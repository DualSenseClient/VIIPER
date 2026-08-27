//go:build windows

// dualsense_raw_capture passively records Raw Input HID reports from Sony
// DualSense-class devices. It registers only as a Raw Input sink: it never
// opens a HID device handle and never sends feature or output reports.
package main

import (
	"bufio"
	"encoding/binary"
	"encoding/csv"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	wmInput   = 0x00ff
	wmClose   = 0x0010
	wmDestroy = 0x0002

	rimTypeHID = 2
	ridInput   = 0x10000003
	ridiName   = 0x20000007
	ridiInfo   = 0x2000000b

	ridevRemove    = 0x00000001
	ridevInputSink = 0x00000100

	usagePageGenericDesktop = 0x01
	usageGamepad            = 0x05

	sonyVID          = 0x054c
	dualSensePID     = 0x0ce6
	dualSenseEdgePID = 0x0df2

	maxReportSize = 128 // Includes USB (64 byte) and Bluetooth (78 byte) reports.
)

var (
	user32                      = windows.NewLazySystemDLL("user32.dll")
	kernel32                    = windows.NewLazySystemDLL("kernel32.dll")
	procRegisterClassExW        = user32.NewProc("RegisterClassExW")
	procCreateWindowExW         = user32.NewProc("CreateWindowExW")
	procDefWindowProcW          = user32.NewProc("DefWindowProcW")
	procDestroyWindow           = user32.NewProc("DestroyWindow")
	procPostQuitMessage         = user32.NewProc("PostQuitMessage")
	procPostMessageW            = user32.NewProc("PostMessageW")
	procGetMessageW             = user32.NewProc("GetMessageW")
	procTranslateMessage        = user32.NewProc("TranslateMessage")
	procDispatchMessageW        = user32.NewProc("DispatchMessageW")
	procRegisterRawInputDevices = user32.NewProc("RegisterRawInputDevices")
	procGetRawInputData         = user32.NewProc("GetRawInputData")
	procGetRawInputDeviceList   = user32.NewProc("GetRawInputDeviceList")
	procGetRawInputDeviceInfoW  = user32.NewProc("GetRawInputDeviceInfoW")
	procGetModuleHandleW        = kernel32.NewProc("GetModuleHandleW")
	procQueryPerformanceCounter = kernel32.NewProc("QueryPerformanceCounter")
	procQueryPerformanceFreq    = kernel32.NewProc("QueryPerformanceFrequency")

	activeRecorder *recorder
)

type exactPaths []string

func (p *exactPaths) String() string { return strings.Join(*p, ",") }
func (p *exactPaths) Set(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return errors.New("path must not be empty")
	}
	*p = append(*p, value)
	return nil
}

func (p exactPaths) contains(path string) bool {
	for _, candidate := range p {
		if strings.EqualFold(candidate, path) {
			return true
		}
	}
	return false
}

type point struct {
	X int32
	Y int32
}

type message struct {
	HWND     uintptr
	Message  uint32
	WParam   uintptr
	LParam   uintptr
	Time     uint32
	Pt       point
	LPrivate uint32
}

type windowClassEx struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   uintptr
	Icon       uintptr
	Cursor     uintptr
	Background uintptr
	MenuName   *uint16
	ClassName  *uint16
	IconSmall  uintptr
}

type rawInputDevice struct {
	UsagePage uint16
	Usage     uint16
	Flags     uint32
	Target    uintptr
}

type rawInputDeviceList struct {
	Device uintptr
	Type   uint32
}

type rawInputHeader struct {
	Type   uint32
	Size   uint32
	Device uintptr
	WParam uintptr
}

type rawDeviceInfo struct {
	Size  uint32
	Type  uint32
	Union [24]byte
}

type deviceIdentity struct {
	ID        uint16
	VID       uint16
	PID       uint16
	UsagePage uint16
	Usage     uint16
	Source    string
	Path      string
}

type captureRecord struct {
	Ordinal  uint64
	QPC      int64
	DeviceID uint16
	Size     uint16
	Report   [maxReportSize]byte
}

type recorder struct {
	physicalPaths exactPaths
	viiperPaths   exactPaths

	devicesByHandle map[uintptr]uint16
	devices         []deviceIdentity
	records         []captureRecord
	scratch         [64 * 1024]byte

	total       uint64
	overwritten uint64
	oversize    uint64
	qpcErrors   uint64
	qpcFreq     int64
}

func newRecorder(capacity int, physicalPaths, viiperPaths exactPaths) (*recorder, error) {
	if capacity < 1 {
		return nil, errors.New("capacity must be positive")
	}
	r := &recorder{
		physicalPaths:   physicalPaths,
		viiperPaths:     viiperPaths,
		devicesByHandle: make(map[uintptr]uint16, 8),
		devices:         make([]deviceIdentity, 0, 8),
		records:         make([]captureRecord, capacity),
	}
	if ok, _, callErr := procQueryPerformanceFreq.Call(uintptr(unsafe.Pointer(&r.qpcFreq))); ok == 0 {
		return nil, winCallError("QueryPerformanceFrequency", callErr)
	}
	return r, nil
}

func (r *recorder) enumerateDevices() error {
	var count uint32
	listSize := uint32(unsafe.Sizeof(rawInputDeviceList{}))
	result, _, callErr := procGetRawInputDeviceList.Call(0, uintptr(unsafe.Pointer(&count)), uintptr(listSize))
	if uint32(result) == ^uint32(0) {
		return winCallError("GetRawInputDeviceList(size)", callErr)
	}
	if count == 0 {
		return nil
	}
	list := make([]rawInputDeviceList, count)
	result, _, callErr = procGetRawInputDeviceList.Call(
		uintptr(unsafe.Pointer(&list[0])), uintptr(unsafe.Pointer(&count)), uintptr(listSize))
	if uint32(result) == ^uint32(0) {
		return winCallError("GetRawInputDeviceList(data)", callErr)
	}
	for i := uint32(0); i < uint32(result); i++ {
		if list[i].Type != rimTypeHID {
			continue
		}
		_, _ = r.discoverDevice(list[i].Device)
	}
	return nil
}

func (r *recorder) discoverDevice(handle uintptr) (uint16, bool) {
	if id, ok := r.devicesByHandle[handle]; ok {
		return id, true
	}
	path, err := rawDeviceName(handle)
	if err != nil {
		return 0, false
	}
	info, err := rawHIDInfo(handle)
	if err != nil || info.VID != sonyVID ||
		(info.PID != dualSensePID && info.PID != dualSenseEdgePID) ||
		info.UsagePage != usagePageGenericDesktop || info.Usage != usageGamepad {
		return 0, false
	}
	if len(r.devices) >= 1<<16 {
		return 0, false
	}
	id := uint16(len(r.devices))
	source := "sony-physical-or-unclassified"
	if r.physicalPaths.contains(path) {
		source = "physical"
	}
	if r.viiperPaths.contains(path) {
		source = "viiper"
	}
	identity := deviceIdentity{
		ID: id, VID: info.VID, PID: info.PID,
		UsagePage: info.UsagePage, Usage: info.Usage,
		Source: source, Path: path,
	}
	r.devices = append(r.devices, identity)
	r.devicesByHandle[handle] = id
	fmt.Printf("capture device id=%d source=%s vid=%04x pid=%04x path=%q\n",
		id, source, info.VID, info.PID, path)
	return id, true
}

type hidIdentity struct {
	VID       uint16
	PID       uint16
	UsagePage uint16
	Usage     uint16
}

func rawHIDInfo(handle uintptr) (hidIdentity, error) {
	info := rawDeviceInfo{Size: uint32(unsafe.Sizeof(rawDeviceInfo{}))}
	size := info.Size
	result, _, callErr := procGetRawInputDeviceInfoW.Call(
		handle, ridiInfo, uintptr(unsafe.Pointer(&info)), uintptr(unsafe.Pointer(&size)))
	if uint32(result) == ^uint32(0) {
		return hidIdentity{}, winCallError("GetRawInputDeviceInfoW(info)", callErr)
	}
	return hidIdentity{
		VID:       uint16(binary.LittleEndian.Uint32(info.Union[0:4])),
		PID:       uint16(binary.LittleEndian.Uint32(info.Union[4:8])),
		UsagePage: binary.LittleEndian.Uint16(info.Union[12:14]),
		Usage:     binary.LittleEndian.Uint16(info.Union[14:16]),
	}, nil
}

func rawDeviceName(handle uintptr) (string, error) {
	var chars uint32
	result, _, callErr := procGetRawInputDeviceInfoW.Call(
		handle, ridiName, 0, uintptr(unsafe.Pointer(&chars)))
	if uint32(result) == ^uint32(0) {
		return "", winCallError("GetRawInputDeviceInfoW(name size)", callErr)
	}
	if chars == 0 {
		return "", errors.New("Raw Input device returned an empty path")
	}
	name := make([]uint16, chars+1)
	result, _, callErr = procGetRawInputDeviceInfoW.Call(
		handle, ridiName, uintptr(unsafe.Pointer(&name[0])), uintptr(unsafe.Pointer(&chars)))
	if uint32(result) == ^uint32(0) {
		return "", winCallError("GetRawInputDeviceInfoW(name)", callErr)
	}
	return windows.UTF16ToString(name), nil
}

func (r *recorder) processRawInput(rawHandle uintptr) {
	size := uint32(len(r.scratch))
	headerSize := uint32(unsafe.Sizeof(rawInputHeader{}))
	result, _, _ := procGetRawInputData.Call(
		rawHandle, ridInput, uintptr(unsafe.Pointer(&r.scratch[0])),
		uintptr(unsafe.Pointer(&size)), uintptr(headerSize))
	if uint32(result) == ^uint32(0) || uint32(result) < headerSize+8 {
		return
	}
	header := (*rawInputHeader)(unsafe.Pointer(&r.scratch[0]))
	if header.Type != rimTypeHID {
		return
	}
	deviceID, ok := r.devicesByHandle[header.Device]
	if !ok {
		deviceID, ok = r.discoverDevice(header.Device)
		if !ok {
			return
		}
	}
	offset := int(headerSize)
	reportSize := int(binary.LittleEndian.Uint32(r.scratch[offset : offset+4]))
	reportCount := int(binary.LittleEndian.Uint32(r.scratch[offset+4 : offset+8]))
	offset += 8
	if reportSize <= 0 || reportCount <= 0 || reportSize > (int(result)-offset)/reportCount {
		return
	}
	for i := 0; i < reportCount; i++ {
		report := r.scratch[offset+i*reportSize : offset+(i+1)*reportSize]
		if len(report) > maxReportSize {
			r.oversize++ // Never retain a partial HID report.
			continue
		}
		var qpc int64
		if ok, _, _ := procQueryPerformanceCounter.Call(uintptr(unsafe.Pointer(&qpc))); ok == 0 {
			r.qpcErrors++
		}
		r.appendRecord(qpc, deviceID, report)
	}
}

func (r *recorder) appendRecord(qpc int64, deviceID uint16, report []byte) {
	if r.total >= uint64(len(r.records)) {
		r.overwritten++
	}
	record := &r.records[r.total%uint64(len(r.records))]
	record.Ordinal = r.total
	record.QPC = qpc
	record.DeviceID = deviceID
	record.Size = uint16(len(report))
	copy(record.Report[:], report)
	r.total++
}

func (r *recorder) retained() uint64 {
	if r.total < uint64(len(r.records)) {
		return r.total
	}
	return uint64(len(r.records))
}

func (r *recorder) eachRecord(fn func(*captureRecord) error) error {
	retained := r.retained()
	start := r.total - retained
	for ordinal := start; ordinal < r.total; ordinal++ {
		record := &r.records[ordinal%uint64(len(r.records))]
		if record.Ordinal != ordinal {
			return fmt.Errorf("capture ring corruption at ordinal %d", ordinal)
		}
		if err := fn(record); err != nil {
			return err
		}
	}
	return nil
}

func (r *recorder) writeCSV(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	buffer := bufio.NewWriterSize(file, 1<<20)
	if _, err = fmt.Fprintf(buffer,
		"# dualsense_raw_capture_v1 qpc_frequency=%d total=%d retained=%d overwritten=%d oversize=%d qpc_errors=%d\n",
		r.qpcFreq, r.total, r.retained(), r.overwritten, r.oversize, r.qpcErrors); err != nil {
		return err
	}
	for _, device := range r.devices {
		if _, err = fmt.Fprintf(buffer, "# device id=%d source=%s vid=%04x pid=%04x usage_page=%04x usage=%04x path=%q\n",
			device.ID, device.Source, device.VID, device.PID,
			device.UsagePage, device.Usage, device.Path); err != nil {
			return err
		}
	}
	w := csv.NewWriter(buffer)
	if err = w.Write([]string{
		"ordinal", "qpc", "device_id", "report_size", "report_id",
		"l2_b5", "digital_l2_b9_bit2", "buttons_b9", "sequence_b7",
		"packet_sequence_b12_15_le", "sensor_clock_b28_31_le",
		"metadata_b41_54_hex", "report_b0_63_hex", "complete_report_hex",
	}); err != nil {
		return err
	}
	err = r.eachRecord(func(record *captureRecord) error {
		report := record.Report[:record.Size]
		row := []string{
			strconv.FormatUint(record.Ordinal, 10),
			strconv.FormatInt(record.QPC, 10),
			strconv.FormatUint(uint64(record.DeviceID), 10),
			strconv.Itoa(len(report)), valueU8(report, 0), valueU8(report, 5),
			valueBit(report, 9, 2), valueHexU8(report, 9), valueU8(report, 7),
			valueLE32(report, 12), valueLE32(report, 28), valueHexRange(report, 41, 55),
			valueHexRange(report, 0, 64), hex.EncodeToString(report),
		}
		return w.Write(row)
	})
	w.Flush()
	if err != nil {
		return err
	}
	if err = w.Error(); err != nil {
		return err
	}
	if err = buffer.Flush(); err != nil {
		return err
	}
	return file.Sync()
}

func (r *recorder) writeBinary(path string) error {
	file, err := os.Create(path)
	if err != nil {
		return err
	}
	defer file.Close()
	buffer := bufio.NewWriterSize(file, 1<<20)
	write := func(value any) error { return binary.Write(buffer, binary.LittleEndian, value) }
	if _, err = buffer.Write([]byte{'D', 'S', 'R', 'A', 'W', 'V', '1', 0}); err != nil {
		return err
	}
	for _, value := range []any{
		uint32(1), uint64(r.qpcFreq), r.total, r.retained(), r.overwritten,
		r.oversize, r.qpcErrors, uint32(len(r.devices)), uint32(maxReportSize),
	} {
		if err = write(value); err != nil {
			return err
		}
	}
	for _, device := range r.devices {
		source := []byte(device.Source)
		name := []byte(device.Path)
		for _, value := range []any{
			device.ID, device.VID, device.PID, device.UsagePage, device.Usage,
			uint16(len(source)), uint16(len(name)),
		} {
			if err = write(value); err != nil {
				return err
			}
		}
		if _, err = buffer.Write(source); err != nil {
			return err
		}
		if _, err = buffer.Write(name); err != nil {
			return err
		}
	}
	err = r.eachRecord(func(record *captureRecord) error {
		for _, value := range []any{record.Ordinal, record.QPC, record.DeviceID, record.Size} {
			if writeErr := write(value); writeErr != nil {
				return writeErr
			}
		}
		_, writeErr := buffer.Write(record.Report[:record.Size])
		return writeErr
	})
	if err != nil {
		return err
	}
	if err = buffer.Flush(); err != nil {
		return err
	}
	return file.Sync()
}

func valueU8(report []byte, index int) string {
	if index >= len(report) {
		return ""
	}
	return strconv.FormatUint(uint64(report[index]), 10)
}

func valueHexU8(report []byte, index int) string {
	if index >= len(report) {
		return ""
	}
	return fmt.Sprintf("%02x", report[index])
}

func valueBit(report []byte, index int, bit uint) string {
	if index >= len(report) {
		return ""
	}
	return strconv.FormatUint(uint64((report[index]>>bit)&1), 10)
}

func valueLE32(report []byte, index int) string {
	if index+4 > len(report) {
		return ""
	}
	return strconv.FormatUint(uint64(binary.LittleEndian.Uint32(report[index:index+4])), 10)
}

func valueHexRange(report []byte, start, end int) string {
	if start >= len(report) {
		return ""
	}
	if end > len(report) {
		end = len(report)
	}
	return hex.EncodeToString(report[start:end])
}

func registerRawInput(hwnd uintptr, flags uint32) error {
	device := rawInputDevice{
		UsagePage: usagePageGenericDesktop,
		Usage:     usageGamepad,
		Flags:     flags,
		Target:    hwnd,
	}
	result, _, callErr := procRegisterRawInputDevices.Call(
		uintptr(unsafe.Pointer(&device)), 1, unsafe.Sizeof(device))
	if result == 0 {
		return winCallError("RegisterRawInputDevices", callErr)
	}
	return nil
}

func windowProc(hwnd uintptr, msg uint32, wParam, lParam uintptr) uintptr {
	switch msg {
	case wmInput:
		if activeRecorder != nil {
			activeRecorder.processRawInput(lParam)
		}
	case wmClose:
		procDestroyWindow.Call(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	result, _, _ := procDefWindowProcW.Call(hwnd, uintptr(msg), wParam, lParam)
	return result
}

func createMessageWindow() (uintptr, error) {
	instance, _, callErr := procGetModuleHandleW.Call(0)
	if instance == 0 {
		return 0, winCallError("GetModuleHandleW", callErr)
	}
	className, _ := windows.UTF16PtrFromString("VIIPER_DualSense_Raw_Capture")
	windowName, _ := windows.UTF16PtrFromString("VIIPER passive DualSense Raw Input capture")
	class := windowClassEx{
		Size:      uint32(unsafe.Sizeof(windowClassEx{})),
		WndProc:   syscall.NewCallback(windowProc),
		Instance:  instance,
		ClassName: className,
	}
	atom, _, callErr := procRegisterClassExW.Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		return 0, winCallError("RegisterClassExW", callErr)
	}
	hwndMessage := ^uintptr(2) // HWND_MESSAGE == (HWND)-3.
	hwnd, _, callErr := procCreateWindowExW.Call(
		0, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(windowName)), 0,
		0, 0, 0, 0, hwndMessage, 0, instance, 0)
	if hwnd == 0 {
		return 0, winCallError("CreateWindowExW", callErr)
	}
	return hwnd, nil
}

func postClose(hwnd uintptr) {
	procPostMessageW.Call(hwnd, wmClose, 0, 0)
}

func runMessageLoop() error {
	var msg message
	for {
		result, _, callErr := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(result) == -1 {
			return winCallError("GetMessageW", callErr)
		}
		if result == 0 {
			return nil
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
	}
}

func winCallError(name string, err error) error {
	if err == nil || errors.Is(err, syscall.Errno(0)) {
		return fmt.Errorf("%s failed", name)
	}
	return fmt.Errorf("%s: %w", name, err)
}

func outputPaths(base, format string) (csvPath, binaryPath string) {
	ext := strings.ToLower(filepath.Ext(base))
	stem := strings.TrimSuffix(base, filepath.Ext(base))
	if stem == "" {
		stem = base
	}
	switch format {
	case "csv":
		if ext == ".csv" {
			return base, ""
		}
		return stem + ".csv", ""
	case "bin":
		if ext == ".bin" {
			return "", base
		}
		return "", stem + ".bin"
	default:
		return stem + ".csv", stem + ".bin"
	}
}

func main() {
	// A Win32 window and its GetMessage loop are owned by the creating OS
	// thread. Without this pin, the Go scheduler can move the goroutine between
	// CreateWindowExW and GetMessageW, leaving close/timer messages unserviced.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	var physicalPaths exactPaths
	var viiperPaths exactPaths
	duration := flag.Duration("duration", 0, "capture duration (0 means until Ctrl+C)")
	capacity := flag.Int("capacity", 262144, "fixed report-ring capacity; oldest reports are overwritten when full")
	format := flag.String("format", "csv", "output format: csv, bin, or both")
	out := flag.String("out", "", "output path or basename (default: timestamped file in current directory)")
	flag.Var(&physicalPaths, "physical-path", "exact Raw Input physical-device path to label (repeatable)")
	flag.Var(&viiperPaths, "viiper-path", "exact Raw Input VIIPER-device path to label (repeatable)")
	flag.Parse()

	*format = strings.ToLower(*format)
	if *format != "csv" && *format != "bin" && *format != "both" {
		fmt.Fprintln(os.Stderr, "-format must be csv, bin, or both")
		os.Exit(2)
	}
	if *duration < 0 {
		fmt.Fprintln(os.Stderr, "-duration must not be negative")
		os.Exit(2)
	}
	if *out == "" {
		*out = "dualsense-raw-" + time.Now().Format("20060102-150405")
	}

	r, err := newRecorder(*capacity, physicalPaths, viiperPaths)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	activeRecorder = r
	if err = r.enumerateDevices(); err != nil {
		fmt.Fprintf(os.Stderr, "initial Raw Input enumeration warning: %v\n", err)
	}
	hwnd, err := createMessageWindow()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	if err = registerRawInput(hwnd, ridevInputSink); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	interrupts := make(chan os.Signal, 1)
	signal.Notify(interrupts, os.Interrupt)
	go func() {
		<-interrupts
		postClose(hwnd)
	}()
	var timer *time.Timer
	if *duration > 0 {
		timer = time.AfterFunc(*duration, func() { postClose(hwnd) })
	}
	fmt.Printf("passive Raw Input capture started; capacity=%d duration=%s (Ctrl+C stops)\n", *capacity, duration.String())
	err = runMessageLoop()
	if timer != nil {
		timer.Stop()
	}
	signal.Stop(interrupts)
	_ = registerRawInput(0, ridevRemove)
	activeRecorder = nil
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	csvPath, binaryPath := outputPaths(*out, *format)
	if csvPath != "" {
		if err = r.writeCSV(csvPath); err != nil {
			fmt.Fprintf(os.Stderr, "write CSV: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", csvPath)
	}
	if binaryPath != "" {
		if err = r.writeBinary(binaryPath); err != nil {
			fmt.Fprintf(os.Stderr, "write binary: %v\n", err)
			os.Exit(1)
		}
		fmt.Printf("wrote %s\n", binaryPath)
	}
	fmt.Printf("reports total=%d retained=%d overwritten=%d oversize=%d qpc_errors=%d\n",
		r.total, r.retained(), r.overwritten, r.oversize, r.qpcErrors)
}
