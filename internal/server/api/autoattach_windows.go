//go:build windows

package api

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"unsafe"

	"github.com/DualSenseClient/VIIPER/usbip"
	"golang.org/x/sys/windows"
)

var (
	setupapi                             = windows.NewLazySystemDLL("setupapi.dll")
	procSetupDiGetClassDevsW             = setupapi.NewProc("SetupDiGetClassDevsW")
	procSetupDiEnumDeviceInterfaces      = setupapi.NewProc("SetupDiEnumDeviceInterfaces")
	procSetupDiGetDeviceInterfaceDetailW = setupapi.NewProc("SetupDiGetDeviceInterfaceDetailW")
	procSetupDiDestroyDeviceInfoList     = setupapi.NewProc("SetupDiDestroyDeviceInfoList")
)

const (
	DigcfPresent         = 0x00000002
	DigcfDeviceInterface = 0x00000010
)

type SpDeviceInterfaceData struct {
	CbSize             uint32
	InterfaceClassGUID windows.GUID
	Flags              uint32
	Reserved           uintptr
}

type SpDeviceInterfaceDetailData struct {
	CbSize     uint32
	DevicePath [1]uint16
}

// Device GUID from usbip-win2 driver
var deviceGUID = windows.GUID{
	Data1: 0xB4030C06,
	Data2: 0xDC5F,
	Data3: 0x4FCC,
	Data4: [8]byte{0x87, 0xEB, 0xE5, 0x51, 0x5A, 0x09, 0x35, 0xC0},
}

const (
	niMaxHost = 1025
	niMaxServ = 32
)

// PLUGIN_HARDWARE structures from usbip-win2.
// attachIOCTL matches 0.9.8.0 (serial + wsk_events); attachIOCTL077 matches
// 0.9.7.7. SERIAL_BUFSZ is 16 in both.
// Pad covers the C++ imported_device_location tail padding: its 1025-byte
// host leaves the base at 1093 bytes, padded to 1096, so serial starts at
// 1100 and the whole 0.9.8.0 struct is 1120 bytes (verified with gcc).
type attachIOCTL struct {
	Size       uint32
	PortOutput int32
	BusID      [32]byte
	Service    [niMaxServ]byte
	Host       [niMaxHost]byte
	Pad        [3]byte
	Serial     [16]byte
	WskEvents  bool
}

type attachIOCTL077 struct {
	Size       uint32
	PortOutput int32
	BusID      [32]byte
	Service    [niMaxServ]byte
	Host       [niMaxHost]byte
}

const (
	fileDeviceUnknown   = 0x00000022
	methodBuffered      = 0
	fileReadData        = 0x0001
	fileWriteData       = 0x0002
	ioctlPluginHardware = (fileDeviceUnknown << 16) | ((fileReadData | fileWriteData) << 14) | (0x800 << 2) | methodBuffered
)

func attachLocalhostClientImpl(ctx context.Context, deviceExportMeta *usbip.ExportMeta, usbipServerPort uint16, useNativeIOCTL bool, wskEvents bool, logger *slog.Logger) (AutoAttachResult, error) {
	if useNativeIOCTL {
		// Never hide a native ABI mismatch behind usbip.exe. A mismatch means
		// the userspace and loaded driver are not a valid pair and must
		// be repaired/rebooted before VIIPER creates devices.
		return attachViaIOCTL(ctx, deviceExportMeta, usbipServerPort, wskEvents, logger)
	}
	return attachViaCommand(ctx, deviceExportMeta, usbipServerPort, logger)
}

func attachViaIOCTL(_ context.Context, deviceExportMeta *usbip.ExportMeta, usbipServerPort uint16, wskEvents bool, logger *slog.Logger) (AutoAttachResult, error) {
	logger.Info("Auto-attaching localhost client via native IOCTL",
		"busID", deviceExportMeta.BusID,
		"deviceID", deviceExportMeta.DevID)

	if usbipServerPort == 0 {
		return AutoAttachResult{}, fmt.Errorf("argumentValidation: invalid TCP port number (0)")
	}

	devicePath, err := getDeviceInterfacePath(&deviceGUID)
	if err != nil {
		return AutoAttachResult{}, fmt.Errorf("discovery: %w", err)
	}

	logger.Debug("Found usbip-win2 device", "path", devicePath)

	busID := fmt.Sprintf("%d-%d", deviceExportMeta.BusID, deviceExportMeta.DevID)
	if len(busID) >= len(attachIOCTL{}.BusID) {
		return AutoAttachResult{}, fmt.Errorf("argumentValidation: bus ID too long: %s", busID)
	}

	service := fmt.Sprintf("%d", usbipServerPort)
	if len(service) >= len(attachIOCTL{}.Service) {
		return AutoAttachResult{}, fmt.Errorf("argumentValidation: service string too long: %s", service)
	}

	devicePathUTF16, err := windows.UTF16PtrFromString(devicePath)
	if err != nil {
		return AutoAttachResult{}, fmt.Errorf("open: failed to convert device path: %w", err)
	}

	handle, err := windows.CreateFile(
		devicePathUTF16,
		windows.GENERIC_READ|windows.GENERIC_WRITE,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE,
		nil,
		windows.OPEN_EXISTING,
		windows.FILE_ATTRIBUTE_NORMAL,
		0,
	)
	if err != nil {
		return AutoAttachResult{}, fmt.Errorf("open: failed to open usbip-win2 device: %w", err)
	}
	defer windows.CloseHandle(handle) // nolint

	logger.Debug("Opened device handle")

	// 0.9.8.0 first (serial left empty, wsk_events selects zero-copy vs
	// low-latency); fall back to the 0.9.7.7 layout when the driver rejects
	// the size. One extra failed IOCTL on version mismatch is negligible.
	ioctlData := attachIOCTL{Size: uint32(unsafe.Sizeof(attachIOCTL{})), WskEvents: wskEvents}
	copy(ioctlData.BusID[:], busID)
	copy(ioctlData.Service[:], service)
	copy(ioctlData.Host[:], "localhost")
	port, bytesReturned, err := submitAttachIOCTL(handle,
		unsafe.Pointer(&ioctlData), ioctlData.Size, &ioctlData.PortOutput)
	if err != nil {
		logger.Debug("0.9.8.0 attach failed, retrying with 0.9.7.7 layout", "error", err)
		legacy := attachIOCTL077{Size: uint32(unsafe.Sizeof(attachIOCTL077{}))}
		copy(legacy.BusID[:], busID)
		copy(legacy.Service[:], service)
		copy(legacy.Host[:], "localhost")
		legacyPort, legacyReturned, legacyErr := submitAttachIOCTL(handle,
			unsafe.Pointer(&legacy), legacy.Size, &legacy.PortOutput)
		if legacyErr != nil {
			return AutoAttachResult{}, fmt.Errorf("IOControl: usbip-win2 0.9.8.0/0.9.7.7 native attach failed (repair or reboot USBIP; no command fallback was attempted): %w (legacy: %v)", err, legacyErr)
		}
		if wskEvents {
			logger.Debug("0.9.7.7 driver ignores low-latency receive mode")
		}
		port, bytesReturned = legacyPort, legacyReturned
	}

	logger.Debug("IOCTL completed", "bytesReturned", bytesReturned, "portOutput", port)

	if port <= 0 {
		return AutoAttachResult{}, fmt.Errorf("ResponseValidation: invalid USB port returned: %d", port)
	}

	logger.Info("Successfully attached device via IOCTL",
		"busID", deviceExportMeta.BusID,
		"deviceID", deviceExportMeta.DevID,
		"usbPort", port)

	return AutoAttachResult{
		USBIPPort: port,
	}, nil
}

func submitAttachIOCTL(handle windows.Handle, data unsafe.Pointer, size uint32,
	portOutput *int32) (int32, uint32, error) {
	var bytesReturned uint32
	err := windows.DeviceIoControl(
		handle,
		ioctlPluginHardware,
		(*byte)(data),
		size,
		(*byte)(data),
		8, // usbip-win2 returns only base.size + location.port.
		&bytesReturned,
		nil,
	)
	if err == nil && bytesReturned != 8 {
		return 0, bytesReturned, fmt.Errorf(
			"usbip-win2 returned %d attach bytes; expected 8", bytesReturned)
	}
	return *portOutput, bytesReturned, err
}

func attachViaCommand(ctx context.Context, deviceExportMeta *usbip.ExportMeta, usbipServerPort uint16, logger *slog.Logger) (AutoAttachResult, error) {
	logger.Info("Auto-attaching localhost client", "busID", deviceExportMeta.BusID, "deviceID", deviceExportMeta.DevID)

	cmd := exec.CommandContext(
		ctx,
		resolveUsbipExecutable(),
		"--tcp-port",
		strconv.FormatUint(uint64(usbipServerPort), 10),
		"attach",
		"-r", "localhost",
		"-b", fmt.Sprintf("%d-%d", deviceExportMeta.BusID, deviceExportMeta.DevID),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		logger.Error("Failed to attach device",
			"error", err,
			"port", usbipServerPort,
			"output", string(output))
		return AutoAttachResult{}, err
	}
	logger.Debug("usbip attach output", "output", string(output))

	return AutoAttachResult{}, nil
}

func resolveUsbipExecutable() string {
	// The usbip-win2 installer does not consistently add its directory to
	// PATH for already-running services. Prefer the canonical installation so
	// a stale copy elsewhere cannot pair a different userspace ABI with the
	// installed driver.
	seen := make(map[string]struct{})
	for _, root := range []string{
		os.Getenv("ProgramW6432"),
		os.Getenv("ProgramFiles"),
		os.Getenv("ProgramFiles(x86)"),
	} {
		if root == "" {
			continue
		}

		candidate := filepath.Join(root, "USBip", "usbip.exe")
		key := strings.ToLower(candidate)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}

	if path, err := exec.LookPath("usbip"); err == nil {
		return path
	}

	return "usbip"
}

func getDeviceInterfacePath(guid *windows.GUID) (string, error) {
	r0, _, e1 := syscall.SyscallN(procSetupDiGetClassDevsW.Addr(),
		uintptr(unsafe.Pointer(guid)),
		0,
		0,
		uintptr(DigcfPresent|DigcfDeviceInterface))

	devInfo := windows.Handle(r0)
	if devInfo == windows.InvalidHandle {
		if e1 != 0 {
			return "", fmt.Errorf("discovery: SetupDiGetClassDevsW failed: %w", e1)
		}
		return "", fmt.Errorf("discovery: SetupDiGetClassDevsW failed with invalid handle")
	}
	defer func() {
		_, _, err := syscall.SyscallN(procSetupDiDestroyDeviceInfoList.Addr(), uintptr(devInfo))
		if err != 0 {
			slog.Error("SetupDiDestroyDeviceInfoList failed", "error", err)
		}
	}()

	var interfaceData SpDeviceInterfaceData
	interfaceData.CbSize = uint32(unsafe.Sizeof(interfaceData))

	r1, _, e2 := syscall.SyscallN(procSetupDiEnumDeviceInterfaces.Addr(),
		uintptr(devInfo),
		0,
		uintptr(unsafe.Pointer(guid)),
		0,
		uintptr(unsafe.Pointer(&interfaceData)))

	if r1 == 0 {
		if e2 != 0 {
			return "", fmt.Errorf("discovery: usbip-win2 driver not found: %w", e2)
		}
		return "", fmt.Errorf("discovery: usbip-win2 driver not found")
	}

	var requiredSize uint32
	r2, _, err := syscall.SyscallN(procSetupDiGetDeviceInterfaceDetailW.Addr(),
		uintptr(devInfo),
		uintptr(unsafe.Pointer(&interfaceData)),
		0,
		0,
		uintptr(unsafe.Pointer(&requiredSize)),
		0)
	if r2 == 0 && err != windows.ERROR_INSUFFICIENT_BUFFER {
		return "", fmt.Errorf("discovery: SetupDiGetDeviceInterfaceDetailW (size query) failed: %w", err)
	}
	if requiredSize == 0 {
		return "", fmt.Errorf("discovery: SetupDiGetDeviceInterfaceDetailW (size query) returned invalid required size")
	}

	detailData := make([]byte, requiredSize)
	detailHeader := (*SpDeviceInterfaceDetailData)(unsafe.Pointer(&detailData[0]))
	detailHeader.CbSize = uint32(unsafe.Sizeof(SpDeviceInterfaceDetailData{}))

	r3, _, e3 := syscall.SyscallN(procSetupDiGetDeviceInterfaceDetailW.Addr(),
		uintptr(devInfo),
		uintptr(unsafe.Pointer(&interfaceData)),
		uintptr(unsafe.Pointer(detailHeader)),
		uintptr(requiredSize),
		0,
		0)

	if r3 == 0 {
		if e3 != 0 {
			return "", fmt.Errorf("discovery: SetupDiGetDeviceInterfaceDetailW failed: %w", e3)
		}
		return "", fmt.Errorf("discovery: SetupDiGetDeviceInterfaceDetailW failed")
	}

	path := windows.UTF16PtrToString(&detailHeader.DevicePath[0])
	return path, nil
}

func CheckAutoAttachPrerequisites(useNativeIOCTL bool, logger *slog.Logger) bool {
	if useNativeIOCTL {
		_, err := getDeviceInterfacePath(&deviceGUID)
		if err != nil {
			logger.Warn("Native IOCTL auto-attach prerequisites not met", "error", err)
			logger.Warn("Native IOCTL auto-attach is unavailable until discovery succeeds")
			logger.Info("Install a supported signed usbip-win2 x64 package (0.9.8.0 or 0.9.7.7):")
			logger.Info("  https://github.com/vadimgrn/usbip-win2/releases/tag/v.0.9.8.0")
			return false
		}
		logger.Debug("usbip-win2 driver found")
		return true
	}

	if _, err := exec.LookPath("usbip.exe"); err != nil {
		logger.Warn("USB/IP tool 'usbip.exe' not found in PATH")
		logger.Warn("Auto-attach requires usbip-win2")
		logger.Info("Download and install usbip-win2:")
		logger.Info("  https://github.com/vadimgrn/usbip-win2")
		return false
	}

	logger.Debug("usbip.exe tool found in PATH")
	return true
}
