package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef uintptr_t USBServerHandle;

typedef uintptr_t DSDeviceHandle;

#define DS_BUTTON_SQUARE    0x00000010u
#define DS_BUTTON_CROSS     0x00000020u
#define DS_BUTTON_CIRCLE    0x00000040u
#define DS_BUTTON_TRIANGLE  0x00000080u
#define DS_BUTTON_L1        0x00000100u
#define DS_BUTTON_R1        0x00000200u
#define DS_BUTTON_L2        0x00000400u
#define DS_BUTTON_R2        0x00000800u
#define DS_BUTTON_CREATE    0x00001000u
#define DS_BUTTON_OPTIONS   0x00002000u
#define DS_BUTTON_L3        0x00004000u
#define DS_BUTTON_R3        0x00008000u
#define DS_BUTTON_PS        0x00010000u
#define DS_BUTTON_TOUCHPAD  0x00020000u
#define DS_BUTTON_MIC_MUTE  0x00040000u
#define DS_BUTTON_RFN       0x00200000u
#define DS_BUTTON_LFN       0x00100000u
#define DS_BUTTON_R4        0x00800000u
#define DS_BUTTON_L4        0x00400000u

#define DS_DPAD_UP     0x01u
#define DS_DPAD_DOWN   0x02u
#define DS_DPAD_LEFT   0x04u
#define DS_DPAD_RIGHT  0x08u

#define DS_SHELL_COLOR_WHITE                    "00"
#define DS_SHELL_COLOR_BLACK                    "01"
#define DS_SHELL_COLOR_COSMIC_RED               "02"
#define DS_SHELL_COLOR_NOVA_PINK                "03"
#define DS_SHELL_COLOR_GALACTIC_PURPLE          "04"
#define DS_SHELL_COLOR_STARLIGHT_BLUE           "05"
#define DS_SHELL_COLOR_GREY_CAMOUFLAGE          "06"
#define DS_SHELL_COLOR_VOLCANIC_RED             "07"
#define DS_SHELL_COLOR_STERLING_SILVER          "08"
#define DS_SHELL_COLOR_COBALT_BLUE              "09"
#define DS_SHELL_COLOR_CHROMA_TEAL              "10"
#define DS_SHELL_COLOR_CHROMA_INDIGO            "11"
#define DS_SHELL_COLOR_CHROMA_PEARL             "12"
#define DS_SHELL_COLOR_ANNIVERSARY_30TH         "30"
#define DS_SHELL_COLOR_GOD_OF_WAR_RAGNAROK      "Z1"
#define DS_SHELL_COLOR_SPIDER_MAN_2             "Z2"
#define DS_SHELL_COLOR_ASTRO_BOT                "Z3"
#define DS_SHELL_COLOR_FORTNITE                 "Z4"
#define DS_SHELL_COLOR_MONSTER_HUNTER_WILDS     "Z5"
#define DS_SHELL_COLOR_THE_LAST_OF_US           "Z6"
#define DS_SHELL_COLOR_GHOST_OF_YOTEI           "Z7"
#define DS_SHELL_COLOR_ICON_BLUE_LIMITED_EDITION "ZB"
#define DS_SHELL_COLOR_ASTRO_BOT_JOYFUL_EDITION "ZC"
#define DS_SHELL_COLOR_GENSHIN_IMPACT           "ZE"

// Input-report connection status byte flags. The virtual device always reports
// USB data/power; the headphone/mic/mute flags are application-controlled.
#define DS_CONNECTION_HEADPHONE  0x01u
#define DS_CONNECTION_MIC         0x02u
#define DS_CONNECTION_MIC_MUTED   0x04u
#define DS_CONNECTION_USB_DATA    0x08u
#define DS_CONNECTION_USB_POWER   0x10u

// Offsets inside the 48-byte native USB output report (byte 0 = report ID 0x02).
#define DS_OUTPUT_MIC_LED_OFFSET                9
#define DS_OUTPUT_LIGHTBAR_SETUP_OFFSET       42
#define DS_OUTPUT_LIGHTBAR_BRIGHTNESS_OFFSET  43
#define DS_OUTPUT_PLAYER_LEDS_OFFSET          44
#define DS_OUTPUT_LIGHTBAR_OFFSET             45

typedef struct {
	int8_t   LX;
	int8_t   LY;
	int8_t   RX;
	int8_t   RY;
	uint32_t Buttons;
	uint8_t  DPad;
	uint8_t  L2;
	uint8_t  R2;
	uint16_t Touch1X;
	uint16_t Touch1Y;
	uint8_t  Touch1Active;
	uint8_t  Touch1Tracking;
	uint16_t Touch2X;
	uint16_t Touch2Y;
	uint8_t  Touch2Active;
	uint8_t  Touch2Tracking;
	int16_t  GyroX;
	int16_t  GyroY;
	int16_t  GyroZ;
	int16_t  AccelX;
	int16_t  AccelY;
	int16_t  AccelZ;
} DSDeviceState;

typedef struct {
	const char* SerialNumber;       // NULL = use default
	const char* MACAddress;         // NULL = use default
	const char* Board;              // NULL = use default
	uint8_t     BatteryStatus;      // 0 = use default
	double      TemperatureCelsius; // 0 = use default
	double      BatteryVoltage;     // 0 = use default
	const char* ShellColor;         // NULL = use default (2-char code, e.g. "00", "Z1")
	const char* BuildTime;          // NULL = use default (RFC3339 or "YYYY-MM-DD HH:MM:SS")
	uint8_t     ConnectionStatus;   // 0 = use default (DS_CONNECTION_* flags)
} DSMetaState;

// Physical raw-input metadata accompanying the legacy 33-byte input state;
// together they form the 53-byte ...v5rawinput... wire payload.
#define DS_RAW_INPUT_METADATA_SIZE 15

typedef struct {
	uint8_t  Valid;      // 0 = metadata invalid/ignored
	uint8_t  EdgeLayout; // non-zero = metadata normalized from an Edge-layout report
	uint8_t  Reserved[2];
	uint32_t SensorTimestamp; // physical input report bytes 28:32
	uint8_t  PhysicalMetadata[DS_RAW_INPUT_METADATA_SIZE]; // normalized physical report metadata
} DSRawInputMetadata;

typedef void (*DSOutputCallback)(DSDeviceHandle handle, uint8_t rumbleSmall, uint8_t rumbleLarge, uint8_t ledRed, uint8_t ledGreen, uint8_t ledBlue, uint8_t playerLeds);

static void viiper_call_ds_output(DSOutputCallback fn, DSDeviceHandle handle, uint8_t rumbleSmall, uint8_t rumbleLarge, uint8_t ledRed, uint8_t ledGreen, uint8_t ledBlue, uint8_t playerLeds) {
	fn(handle, rumbleSmall, rumbleLarge, ledRed, ledGreen, ledBlue, playerLeds);
}

typedef struct {
	uint8_t RumbleSmall;
	uint8_t RumbleLarge;
	uint8_t LedRed;
	uint8_t LedGreen;
	uint8_t LedBlue;
	uint8_t PlayerLeds;
	uint8_t TriggerR2Mode;
	uint8_t TriggerR2StartResistance;
	uint8_t TriggerR2EffectForce;
	uint8_t TriggerR2RangeForce;
	uint8_t TriggerR2NearReleaseStrength;
	uint8_t TriggerR2NearMiddleStrength;
	uint8_t TriggerR2PressedStrength;
	uint8_t TriggerR2Frequency;
	uint8_t TriggerL2Mode;
	uint8_t TriggerL2StartResistance;
	uint8_t TriggerL2EffectForce;
	uint8_t TriggerL2RangeForce;
	uint8_t TriggerL2NearReleaseStrength;
	uint8_t TriggerL2NearMiddleStrength;
	uint8_t TriggerL2PressedStrength;
	uint8_t TriggerL2Frequency;
	uint8_t RawOutputReport[48];
	uint8_t BluetoothCombinedOutputReport[398];
	uint8_t MicLed;             // 0=off, 1=on, 2=pulse (native report byte 9)
	uint8_t LightbarSetup;      // 0x01 default, 0x02 custom (native report byte 42)
	uint8_t LightbarBrightness; // 0=high, 1=medium, 2=low (native report byte 43)
} DSOutputState;

typedef void (*DSOutputStateCallback)(DSDeviceHandle handle, DSOutputState output);

static void viiper_call_ds_output_state(DSOutputStateCallback fn, DSDeviceHandle handle, DSOutputState output) {
	fn(handle, output);
}

typedef void (*DSRealtimeHapticsCallback)(DSDeviceHandle handle, DSOutputState output);

static void viiper_call_ds_realtime_haptics(DSRealtimeHapticsCallback fn, DSDeviceHandle handle, DSOutputState output) {
	fn(handle, output);
}

typedef void (*DSAtomicAudioHapticsCallback)(DSDeviceHandle handle, DSOutputState output, const uint8_t* speakerPCM, size_t length);

static void viiper_call_ds_atomic_audio_haptics(DSAtomicAudioHapticsCallback fn, DSDeviceHandle handle, DSOutputState output, const uint8_t* speakerPCM, size_t length) {
	fn(handle, output, speakerPCM, length);
}

typedef void (*DSSpeakerResetCallback)(DSDeviceHandle handle);

static void viiper_call_ds_speaker_reset(DSSpeakerResetCallback fn, DSDeviceHandle handle) {
	fn(handle);
}

typedef void (*DSAudioCallback)(DSDeviceHandle handle, const uint8_t* pcm, size_t length);

static void viiper_call_ds_audio(DSAudioCallback fn, DSDeviceHandle handle, const uint8_t* pcm, size_t length) {
	fn(handle, pcm, length);
}

*/
import "C"
import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/cgo"
	"slices"
	"strings"
	"time"
	"unsafe"

	"github.com/DualSenseClient/VIIPER/device"
	"github.com/DualSenseClient/VIIPER/device/dualsense"
	"github.com/DualSenseClient/VIIPER/internal/server/api"
)

// CreateDualSenseDevice creates a new DualSense (non-edge) device on the bus with the given ID on the server associated with the given handle.
// @param serverHandle Handle to the USB server.
// @param outDeviceHandle Output parameter for the created device handle.
// @param busID ID of the bus to add the device to.
// @param autoAttachLocalhost If true, the device will be automatically attached to a USBIP-Client/Driver running on THIS machine.
// @param idVendor Optional USB vendor ID (0 = default).
// @param idProduct Optional USB product ID (0 = default).
// @param meta Optional pointer to initial device metadata. Pass NULL to use defaults.
//
//export CreateDualSenseDevice
func CreateDualSenseDevice(
	serverHandle C.USBServerHandle,
	outDeviceHandle *C.DSDeviceHandle,
	busID uint32,
	autoAttachLocalhost bool,
	idVendor uint16,
	idProduct uint16,
	meta *C.DSMetaState,
) bool {
	return createDualSenseDevice(serverHandle, outDeviceHandle, busID, autoAttachLocalhost, idVendor, idProduct, meta, buildFromConstructor(dualsense.New, false))
}

// CreateDualSenseEdgeDevice creates a new DualSense Edge device on the bus with the given ID on the server associated with the given handle.
// @param serverHandle Handle to the USB server.
// @param outDeviceHandle Output parameter for the created device handle.
// @param busID ID of the bus to add the device to.
// @param autoAttachLocalhost If true, the device will be automatically attached to a USBIP-Client/Driver running on THIS machine.
// @param idVendor Optional USB vendor ID (0 = default).
// @param idProduct Optional USB product ID (0 = default).
// @param meta Optional pointer to initial device metadata. Pass NULL to use defaults.
//
//export CreateDualSenseEdgeDevice
func CreateDualSenseEdgeDevice(
	serverHandle C.USBServerHandle,
	outDeviceHandle *C.DSDeviceHandle,
	busID uint32,
	autoAttachLocalhost bool,
	idVendor uint16,
	idProduct uint16,
	meta *C.DSMetaState,
) bool {
	return createDualSenseDevice(serverHandle, outDeviceHandle, busID, autoAttachLocalhost, idVendor, idProduct, meta, buildFromConstructor(dualsense.NewEdge, true))
}

// CreateDualSenseAudioOnlyDevice creates a DualSense exposing only the audio
// interfaces and no HID gamepad interface.
// @param serverHandle Handle to the USB server.
// @param outDeviceHandle Output parameter for the created device handle.
// @param busID ID of the bus to add the device to.
// @param autoAttachLocalhost If true, the device will be automatically attached to a USBIP-Client/Driver running on THIS machine.
// @param idVendor Optional USB vendor ID (0 = default).
// @param idProduct Optional USB product ID (0 = default).
// @param meta Optional pointer to initial device metadata. Pass NULL to use defaults.
//
//export CreateDualSenseAudioOnlyDevice
func CreateDualSenseAudioOnlyDevice(
	serverHandle C.USBServerHandle,
	outDeviceHandle *C.DSDeviceHandle,
	busID uint32,
	autoAttachLocalhost bool,
	idVendor uint16,
	idProduct uint16,
	meta *C.DSMetaState,
) bool {
	return createDualSenseDevice(serverHandle, outDeviceHandle, busID, autoAttachLocalhost, idVendor, idProduct, meta, buildFromConstructor(dualsense.NewAudioOnly, false))
}

// CreateDualSenseEdgeAudioOnlyDevice creates a DualSense Edge exposing only the audio
// interfaces and no HID gamepad interface.
// @param serverHandle Handle to the USB server.
// @param outDeviceHandle Output parameter for the created device handle.
// @param busID ID of the bus to add the device to.
// @param autoAttachLocalhost If true, the device will be automatically attached to a USBIP-Client/Driver running on THIS machine.
// @param idVendor Optional USB vendor ID (0 = default).
// @param idProduct Optional USB product ID (0 = default).
// @param meta Optional pointer to initial device metadata. Pass NULL to use defaults.
//
//export CreateDualSenseEdgeAudioOnlyDevice
func CreateDualSenseEdgeAudioOnlyDevice(
	serverHandle C.USBServerHandle,
	outDeviceHandle *C.DSDeviceHandle,
	busID uint32,
	autoAttachLocalhost bool,
	idVendor uint16,
	idProduct uint16,
	meta *C.DSMetaState,
) bool {
	return createDualSenseDevice(serverHandle, outDeviceHandle, busID, autoAttachLocalhost, idVendor, idProduct, meta, buildFromConstructor(dualsense.NewEdgeAudioOnly, true))
}

// CreateDualSenseGamepadOnlyDevice creates a DualSense exposing only the HID gamepad interface.
// @param serverHandle Handle to the USB server.
// @param outDeviceHandle Output parameter for the created device handle.
// @param busID ID of the bus to add the device to.
// @param autoAttachLocalhost If true, the device will be automatically attached to a USBIP-Client/Driver running on THIS machine.
// @param idVendor Optional USB vendor ID (0 = default).
// @param idProduct Optional USB product ID (0 = default).
// @param meta Optional pointer to initial device metadata. Pass NULL to use defaults.
//
//export CreateDualSenseGamepadOnlyDevice
func CreateDualSenseGamepadOnlyDevice(
	serverHandle C.USBServerHandle,
	outDeviceHandle *C.DSDeviceHandle,
	busID uint32,
	autoAttachLocalhost bool,
	idVendor uint16,
	idProduct uint16,
	meta *C.DSMetaState,
) bool {
	return createDualSenseDevice(serverHandle, outDeviceHandle, busID, autoAttachLocalhost, idVendor, idProduct, meta, buildFromConstructor(dualsense.NewGamepadOnly, false))
}

// CreateDualSenseEdgeGamepadOnlyDevice creates a DualSense Edge exposing only the HID gamepad interface.
// @param serverHandle Handle to the USB server.
// @param outDeviceHandle Output parameter for the created device handle.
// @param busID ID of the bus to add the device to.
// @param autoAttachLocalhost If true, the device will be automatically attached to a USBIP-Client/Driver running on THIS machine.
// @param idVendor Optional USB vendor ID (0 = default).
// @param idProduct Optional USB product ID (0 = default).
// @param meta Optional pointer to initial device metadata. Pass NULL to use defaults.
//
//export CreateDualSenseEdgeGamepadOnlyDevice
func CreateDualSenseEdgeGamepadOnlyDevice(
	serverHandle C.USBServerHandle,
	outDeviceHandle *C.DSDeviceHandle,
	busID uint32,
	autoAttachLocalhost bool,
	idVendor uint16,
	idProduct uint16,
	meta *C.DSMetaState,
) bool {
	return createDualSenseDevice(serverHandle, outDeviceHandle, busID, autoAttachLocalhost, idVendor, idProduct, meta, buildFromConstructor(dualsense.NewEdgeGamepadOnly, true))
}

func createDualSenseDevice(
	serverHandle C.USBServerHandle,
	outDeviceHandle *C.DSDeviceHandle,
	busID uint32,
	autoAttachLocalhost bool,
	idVendor uint16,
	idProduct uint16,
	meta *C.DSMetaState,
	build dsBuildFunc,
) bool {
	sh := cgo.Handle(serverHandle)
	shw, ok := sh.Value().(*usbServerHandleWrapper)
	if !ok {
		return false
	}
	bus := shw.s.GetBus(busID)
	if bus == nil {
		return false
	}

	opts := &device.CreateOptions{}
	if idVendor != 0 {
		opts.IDVendor = &idVendor
	}
	if idProduct != 0 {
		opts.IDProduct = &idProduct
	}
	if meta != nil {
		goMeta := dualsense.MetaState{
			SerialNumber:       goStringOrEmpty(meta.SerialNumber),
			MACAddress:         goStringOrEmpty(meta.MACAddress),
			Board:              goStringOrEmpty(meta.Board),
			BatteryStatus:      uint8(meta.BatteryStatus),
			TemperatureCelsius: float64(meta.TemperatureCelsius),
			BatteryVoltage:     float64(meta.BatteryVoltage),
			ConnectionStatus:   uint8(meta.ConnectionStatus),
			ShellColor:         goStringOrEmpty(meta.ShellColor),
		}
		if bt, ok := parseDSBuildTime(goStringOrEmpty(meta.BuildTime)); ok {
			goMeta.BuildTime = bt
		}
		b, err := json.Marshal(goMeta)
		if err != nil {
			return false
		}
		opts.DeviceSpecific = string(b)
	}

	d, releaseIdentity, err := build(opts)
	if err != nil {
		return false
	}
	devCtx, err := bus.Add(d)
	if err != nil {
		releaseIdentity()
		return false
	}
	exportMeta := device.GetDeviceMeta(devCtx)
	if exportMeta == nil {
		releaseIdentity()
		return false
	}

	if autoAttachLocalhost {
		err := api.AttachLocalhostClient(
			context.Background(),
			exportMeta,
			shw.s.GetListenPort(),
			shw.useNativeIOCTL.Load(),
			slog.Default(),
		)
		if err != nil {
			slog.Error("failed to auto-attach localhost client", "error", err)
			releaseIdentity()
			return false
		}
	}

	handleWrapper := &deviceHandleWrapper{
		device:          d,
		exportMeta:      exportMeta,
		usbServer:       shw,
		releaseIdentity: releaseIdentity,
	}
	*outDeviceHandle = C.DSDeviceHandle(cgo.NewHandle(handleWrapper))

	shw.mtx.Lock()
	defer shw.mtx.Unlock()
	shw.deviceHandles[busID] = append(shw.deviceHandles[busID], deviceHandle(*outDeviceHandle))
	return true
}

// dsBuildFunc constructs a DualSense device plus the release hook for the
// identity registration reserved during construction.
type dsBuildFunc func(opts *device.CreateOptions) (*dualsense.DualSense, func(), error)

func buildFromConstructor(ctor func(*device.CreateOptions) (*dualsense.DualSense, error),
	edge bool) dsBuildFunc {
	return func(opts *device.CreateOptions) (*dualsense.DualSense, func(), error) {
		lease, err := dualsense.AcquireIdentity(opts, edge)
		if err != nil {
			return nil, nil, err
		}
		d, err := ctor(opts)
		if err != nil {
			lease.Release()
			return nil, nil, err
		}
		return d, lease.Release, nil
	}
}

func buildFromRegistry(deviceType string) dsBuildFunc {
	return func(opts *device.CreateOptions) (*dualsense.DualSense, func(), error) {
		reg := api.GetRegistration(deviceType)
		if reg == nil {
			return nil, nil, fmt.Errorf("unknown device type %q", deviceType)
		}
		dev, err := reg.CreateDevice(opts)
		if err != nil {
			return nil, nil, err
		}
		ds, ok := dev.(*dualsense.DualSense)
		if !ok {
			dualsense.ReleaseDeviceIdentity(dev)
			return nil, nil, fmt.Errorf("device type %q is not a DualSense variant", deviceType)
		}
		return ds, func() { dualsense.ReleaseDeviceIdentity(ds) }, nil
	}
}

// CreateDualSenseDeviceByType creates a DualSense family device selected by a
// registered device type name. In addition to the classic variants covered by
// the dedicated Create* functions, this reaches the events and raw-input
// aliases (e.g. "dualsensecombinedaudioduplexv5rawinputevents").
// @param serverHandle Handle to the USB server.
// @param outDeviceHandle Output parameter for the created device handle.
// @param busID ID of the bus to add the device to.
// @param autoAttachLocalhost If true, the device will be automatically attached to a USBIP-Client/Driver running on THIS machine.
// @param idVendor Optional USB vendor ID (0 = default).
// @param idProduct Optional USB product ID (0 = default).
// @param meta Optional pointer to initial device metadata. Pass NULL to use defaults.
// @param deviceType Registered DualSense device type name (case-insensitive).
//
//export CreateDualSenseDeviceByType
func CreateDualSenseDeviceByType(
	serverHandle C.USBServerHandle,
	outDeviceHandle *C.DSDeviceHandle,
	busID uint32,
	autoAttachLocalhost bool,
	idVendor uint16,
	idProduct uint16,
	meta *C.DSMetaState,
	deviceType *C.char,
) bool {
	name := goStringOrEmpty(deviceType)
	if !isRegisteredDualSenseType(name) {
		return false
	}
	return createDualSenseDevice(serverHandle, outDeviceHandle, busID,
		autoAttachLocalhost, idVendor, idProduct, meta,
		buildFromRegistry(strings.ToLower(name)))
}

func isRegisteredDualSenseType(name string) bool {
	name = strings.ToLower(name)
	return strings.HasPrefix(name, "dualsense") && api.GetRegistration(name) != nil
}

// SetDualSenseDeviceState updates the input state of the DualSense device associated with the given handle.
// @param handle Handle to the DualSense device.
// @param state New input state to set on the device.
//
//export SetDualSenseDeviceState
func SetDualSenseDeviceState(handle C.DSDeviceHandle, state C.DSDeviceState) bool {
	return setDualSenseInputState(handle, toGoInputState(state))
}

// SetDualSenseDeviceStateRaw updates the input state together with physical
// raw-input metadata, mirroring the 53-byte ...v5rawinput... wire payload as
// one atomic unit. Pass NULL (or a struct with Valid = 0) to behave exactly
// like SetDualSenseDeviceState; the presented USB input report then carries
// the neutral metadata fallback.
// @param handle Handle to the DualSense device.
// @param state New input state to set on the device.
// @param raw Optional physical metadata accompanying the state.
//
//export SetDualSenseDeviceStateRaw
func SetDualSenseDeviceStateRaw(handle C.DSDeviceHandle, state C.DSDeviceState,
	raw *C.DSRawInputMetadata) bool {
	s := toGoInputState(state)
	if raw != nil {
		metadata := make([]byte, len(raw.PhysicalMetadata))
		for i, b := range raw.PhysicalMetadata {
			metadata[i] = byte(b)
		}
		applyRawInputMetadata(s, raw.Valid != 0, raw.EdgeLayout != 0,
			uint32(raw.SensorTimestamp), metadata)
	}
	return setDualSenseInputState(handle, s)
}

func applyRawInputMetadata(s *dualsense.InputState, valid, edgeLayout bool,
	sensorTimestamp uint32, physicalMetadata []byte) {
	if !valid {
		return
	}
	s.PhysicalMetadataValid = true
	s.PhysicalMetadataEdgeLayout = edgeLayout
	s.PhysicalSensorTimestamp = sensorTimestamp
	copy(s.PhysicalInputMetadata[:], physicalMetadata)
}

func toGoInputState(state C.DSDeviceState) *dualsense.InputState {
	return &dualsense.InputState{
		LX:             int8(state.LX),
		LY:             int8(state.LY),
		RX:             int8(state.RX),
		RY:             int8(state.RY),
		Buttons:        uint32(state.Buttons),
		DPad:           uint8(state.DPad),
		L2:             uint8(state.L2),
		R2:             uint8(state.R2),
		Touch1X:        uint16(state.Touch1X),
		Touch1Y:        uint16(state.Touch1Y),
		Touch1Active:   state.Touch1Active != 0,
		Touch1Tracking: uint8(state.Touch1Tracking),
		Touch2X:        uint16(state.Touch2X),
		Touch2Y:        uint16(state.Touch2Y),
		Touch2Active:   state.Touch2Active != 0,
		Touch2Tracking: uint8(state.Touch2Tracking),
		GyroX:          int16(state.GyroX),
		GyroY:          int16(state.GyroY),
		GyroZ:          int16(state.GyroZ),
		AccelX:         int16(state.AccelX),
		AccelY:         int16(state.AccelY),
		AccelZ:         int16(state.AccelZ),
	}
}

func setDualSenseInputState(handle C.DSDeviceHandle, s *dualsense.InputState) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	dsDevice, ok := dhw.device.(*dualsense.DualSense)
	if !ok {
		return false
	}
	dsDevice.UpdateInputState(s)
	return true
}

// SetDualSenseMetaState updates the meta (identity/battery/sensor) state of the
// device at runtime. Fields left at their zero value (NULL/0) keep the current
// value, so a partial update only changes what the caller supplies.
// @param handle Handle to the DualSense device.
// @param meta Updated metadata. Pass NULL or a zeroed struct to change nothing.
//
//export SetDualSenseMetaState
func SetDualSenseMetaState(handle C.DSDeviceHandle, meta *C.DSMetaState) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	dsDevice, ok := dhw.device.(*dualsense.DualSense)
	if !ok {
		return false
	}
	if meta == nil {
		return true
	}
	goMeta := dualsense.MetaState{
		SerialNumber:       goStringOrEmpty(meta.SerialNumber),
		MACAddress:         goStringOrEmpty(meta.MACAddress),
		Board:              goStringOrEmpty(meta.Board),
		BatteryStatus:      uint8(meta.BatteryStatus),
		TemperatureCelsius: float64(meta.TemperatureCelsius),
		BatteryVoltage:     float64(meta.BatteryVoltage),
		ConnectionStatus:   uint8(meta.ConnectionStatus),
		ShellColor:         goStringOrEmpty(meta.ShellColor),
	}
	if bt, ok := parseDSBuildTime(goStringOrEmpty(meta.BuildTime)); ok {
		goMeta.BuildTime = bt
	}
	dsDevice.UpdateMetaState(goMeta)
	return true
}

func parseDSBuildTime(s string) (time.Time, bool) {
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

// SetDualSenseOutputCallback sets a callback to be invoked when the host sends output (rumble/LED) commands to the device.
// @param handle Handle to the DualSense device.
// @param callback Callback receiving rumbleSmall, rumbleLarge, ledRed, ledGreen, ledBlue, playerLeds. Pass NULL to clear.
//
//export SetDualSenseOutputCallback
func SetDualSenseOutputCallback(handle C.DSDeviceHandle, cb C.DSOutputCallback) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	dsDevice, ok := dhw.device.(*dualsense.DualSense)
	if !ok {
		return false
	}
	if cb == nil {
		dsDevice.SetOutputCallback(nil)
		return true
	}
	dsDevice.SetOutputCallback(func(out dualsense.OutputState) {
		C.viiper_call_ds_output(cb, handle,
			C.uint8_t(out.RumbleSmall),
			C.uint8_t(out.RumbleLarge),
			C.uint8_t(out.LedRed),
			C.uint8_t(out.LedGreen),
			C.uint8_t(out.LedBlue),
			C.uint8_t(out.PlayerLeds),
		)
	})
	return true
}

// SetDualSenseOutputStateCallback sets a callback to be invoked when the host sends output commands to the device.
// Unlike SetDualSenseOutputCallback this delivers the full output state, including the adaptive trigger blocks for R2 and L2.
// Only one output callback may be active at a time; the most recently installed wins.
// @param handle Handle to the DualSense device.
// @param callback Callback receiving the full output state. Pass NULL to clear.
//
//export SetDualSenseOutputStateCallback
func SetDualSenseOutputStateCallback(handle C.DSDeviceHandle, cb C.DSOutputStateCallback) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	dsDevice, ok := dhw.device.(*dualsense.DualSense)
	if !ok {
		return false
	}
	if cb == nil {
		dsDevice.SetOutputCallback(nil)
		return true
	}
	dsDevice.SetOutputCallback(func(out dualsense.OutputState) {
		C.viiper_call_ds_output_state(cb, handle, toCDSOutputState(out))
	})
	return true
}

func toCDSOutputState(out dualsense.OutputState) C.DSOutputState {
	st := C.DSOutputState{
		RumbleSmall:                  C.uint8_t(out.RumbleSmall),
		RumbleLarge:                  C.uint8_t(out.RumbleLarge),
		LedRed:                       C.uint8_t(out.LedRed),
		LedGreen:                     C.uint8_t(out.LedGreen),
		LedBlue:                      C.uint8_t(out.LedBlue),
		PlayerLeds:                   C.uint8_t(out.PlayerLeds),
		TriggerR2Mode:                C.uint8_t(out.TriggerR2Mode),
		TriggerR2StartResistance:     C.uint8_t(out.TriggerR2StartResistance),
		TriggerR2EffectForce:         C.uint8_t(out.TriggerR2EffectForce),
		TriggerR2RangeForce:          C.uint8_t(out.TriggerR2RangeForce),
		TriggerR2NearReleaseStrength: C.uint8_t(out.TriggerR2NearReleaseStrength),
		TriggerR2NearMiddleStrength:  C.uint8_t(out.TriggerR2NearMiddleStrength),
		TriggerR2PressedStrength:     C.uint8_t(out.TriggerR2PressedStrength),
		TriggerR2Frequency:           C.uint8_t(out.TriggerR2Frequency),
		TriggerL2Mode:                C.uint8_t(out.TriggerL2Mode),
		TriggerL2StartResistance:     C.uint8_t(out.TriggerL2StartResistance),
		TriggerL2EffectForce:         C.uint8_t(out.TriggerL2EffectForce),
		TriggerL2RangeForce:          C.uint8_t(out.TriggerL2RangeForce),
		TriggerL2NearReleaseStrength: C.uint8_t(out.TriggerL2NearReleaseStrength),
		TriggerL2NearMiddleStrength:  C.uint8_t(out.TriggerL2NearMiddleStrength),
		TriggerL2PressedStrength:     C.uint8_t(out.TriggerL2PressedStrength),
		TriggerL2Frequency:           C.uint8_t(out.TriggerL2Frequency),
	}
	fillCDSArray(st.RawOutputReport[:], out.RawOutputReport[:])
	fillCDSArray(st.BluetoothCombinedOutputReport[:], out.BluetoothCombinedOutputReport[:])
	if len(out.RawOutputReport) > int(C.DS_OUTPUT_LIGHTBAR_BRIGHTNESS_OFFSET) {
		st.MicLed = C.uint8_t(out.RawOutputReport[9])
		st.LightbarSetup = C.uint8_t(out.RawOutputReport[42])
		st.LightbarBrightness = C.uint8_t(out.RawOutputReport[43])
	}
	return st
}

func fillCDSArray(dst []C.uint8_t, src []byte) {
	for i, v := range src {
		if i >= len(dst) {
			break
		}
		dst[i] = C.uint8_t(v)
	}
}

// SetDualSenseRealtimeHapticsCallback sets a callback invoked as soon as a rear
// haptics interval completes, before the next 480-frame speaker boundary. This is
// the low-latency haptics lane of the V5 transport; the delivered output state
// carries a fresh BluetoothCombinedOutputReport. Pass NULL to clear.
// @param handle Handle to the DualSense device.
// @param callback Callback receiving the full output state.
//
//export SetDualSenseRealtimeHapticsCallback
func SetDualSenseRealtimeHapticsCallback(handle C.DSDeviceHandle, cb C.DSRealtimeHapticsCallback) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	dsDevice, ok := dhw.device.(*dualsense.DualSense)
	if !ok {
		return false
	}
	if cb == nil {
		dsDevice.SetRealtimeHapticsCallback(nil)
		return true
	}
	dsDevice.SetRealtimeHapticsCallback(func(out dualsense.OutputState) {
		C.viiper_call_ds_realtime_haptics(cb, handle, toCDSOutputState(out))
	})
	return true
}

// SetDualSenseAtomicAudioHapticsCallback sets a callback invoked once per
// 480-frame (10 ms) speaker generation of the V5 transport. Each invocation
// pairs the native feedback output state with exactly that generation's
// speaker PCM: two S16LE channels (front stereo) at 48 kHz, 1920 bytes.
// While installed it supersedes SetDualSenseOutputStateCallback and
// SetDualSenseOutputCallback for audio generations; realtime haptics
// callbacks continue independently. Pass NULL to clear.
// @param handle Handle to the DualSense device.
// @param callback Callback receiving the full output state and its paired PCM buffer. The buffer is only valid during the call.
//
//export SetDualSenseAtomicAudioHapticsCallback
func SetDualSenseAtomicAudioHapticsCallback(handle C.DSDeviceHandle, cb C.DSAtomicAudioHapticsCallback) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	dsDevice, ok := dhw.device.(*dualsense.DualSense)
	if !ok {
		return false
	}
	if cb == nil {
		dsDevice.SetAtomicAudioHapticsCallback(nil)
		return true
	}
	dsDevice.SetAtomicAudioHapticsCallback(func(out dualsense.OutputState, pcm []byte) {
		if len(pcm) == 0 {
			return
		}
		C.viiper_call_ds_atomic_audio_haptics(cb, handle, toCDSOutputState(out),
			(*C.uint8_t)(unsafe.Pointer(&pcm[0])), C.size_t(len(pcm)))
	})
	return true
}

// SetDualSenseSpeakerResetCallback sets a callback invoked when the haptics audio
// interface alternate setting changes or the endpoint is reset. This marks a
// speaker-stream generation barrier, so transport queues should be flushed.
// Pass NULL to clear.
// @param handle Handle to the DualSense device.
// @param callback Callback with no arguments.
//
//export SetDualSenseSpeakerResetCallback
func SetDualSenseSpeakerResetCallback(handle C.DSDeviceHandle, cb C.DSSpeakerResetCallback) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	dsDevice, ok := dhw.device.(*dualsense.DualSense)
	if !ok {
		return false
	}
	if cb == nil {
		dsDevice.SetSpeakerResetCallback(nil)
		return true
	}
	dsDevice.SetSpeakerResetCallback(func() {
		C.viiper_call_ds_speaker_reset(cb, handle)
	})
	return true
}

// SetDualSenseAudioOutCallback sets a callback to be invoked when the host sends haptics/speaker PCM to the device.
// The callback receives the raw bytes written to the haptics audio-out endpoint: four S16LE channels
// (front stereo + rear haptics) at 48 kHz. Pass NULL to clear.
// @param handle Handle to the DualSense device.
// @param callback Callback receiving a PCM buffer. The buffer is only valid during the call.
//
//export SetDualSenseAudioOutCallback
func SetDualSenseAudioOutCallback(handle C.DSDeviceHandle, cb C.DSAudioCallback) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	dsDevice, ok := dhw.device.(*dualsense.DualSense)
	if !ok {
		return false
	}
	if cb == nil {
		dsDevice.SetAudioOutCallback(nil)
		return true
	}
	dsDevice.SetAudioOutCallback(func(pcm []byte) {
		if len(pcm) == 0 {
			return
		}
		C.viiper_call_ds_audio(cb, handle, (*C.uint8_t)(unsafe.Pointer(&pcm[0])), C.size_t(len(pcm)))
	})
	return true
}

// SetDualSenseMicrophonePCM queues a microphone PCM frame captured from the host-facing mic stream.
// The frame must be exactly 1920 bytes (480 frames of two S16LE channels at 48 kHz).
// @param handle Handle to the DualSense device.
// @param data Pointer to the PCM frame.
// @param length Length of the PCM frame.
//
//export SetDualSenseMicrophonePCM
func SetDualSenseMicrophonePCM(handle C.DSDeviceHandle, data *C.uint8_t, length C.size_t) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	dsDevice, ok := dhw.device.(*dualsense.DualSense)
	if !ok {
		return false
	}
	if data == nil || length != dualsense.USBMicrophoneClientFrameSize {
		return false
	}
	frame := C.GoBytes(unsafe.Pointer(data), C.int(length))
	dsDevice.QueueMicrophonePCMFrame(frame)
	return true
}

// RemoveDualSenseDevice removes the DualSense device associated with the given handle from the server.
// @param handle Handle to the DualSense device to remove.
//
//export RemoveDualSenseDevice
func RemoveDualSenseDevice(handle C.DSDeviceHandle) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	if err := dhw.usbServer.s.RemoveDeviceByID(dhw.exportMeta.BusID, fmt.Sprintf("%d", dhw.exportMeta.DevID)); err != nil {
		return false
	}
	if dhw.releaseIdentity != nil {
		dhw.releaseIdentity()
	}

	shw := dhw.usbServer
	busID := dhw.exportMeta.BusID

	shw.mtx.Lock()
	defer shw.mtx.Unlock()
	shw.deviceHandles[busID] = slices.DeleteFunc(shw.deviceHandles[busID], func(h deviceHandle) bool {
		return h == deviceHandle(handle)
	})
	dh.Delete()

	return true
}
