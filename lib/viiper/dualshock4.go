package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef uintptr_t USBServerHandle;

typedef uintptr_t DS4DeviceHandle;

#define DS4_BUTTON_SQUARE    0x0010u
#define DS4_BUTTON_CROSS     0x0020u
#define DS4_BUTTON_CIRCLE    0x0040u
#define DS4_BUTTON_TRIANGLE  0x0080u
#define DS4_BUTTON_L1        0x0100u
#define DS4_BUTTON_R1        0x0200u
#define DS4_BUTTON_L2        0x0400u
#define DS4_BUTTON_R2        0x0800u
#define DS4_BUTTON_SHARE     0x1000u
#define DS4_BUTTON_OPTIONS   0x2000u
#define DS4_BUTTON_L3        0x4000u
#define DS4_BUTTON_R3        0x8000u
#define DS4_BUTTON_PS        0x0001u
#define DS4_BUTTON_TOUCHPAD  0x0002u

#define DS4_DPAD_UP        0x00u
#define DS4_DPAD_UP_RIGHT  0x01u
#define DS4_DPAD_RIGHT     0x02u
#define DS4_DPAD_DOWN_RIGHT 0x03u
#define DS4_DPAD_DOWN      0x04u
#define DS4_DPAD_DOWN_LEFT 0x05u
#define DS4_DPAD_LEFT      0x06u
#define DS4_DPAD_UP_LEFT   0x07u
#define DS4_DPAD_NEUTRAL   0x08u

typedef struct {
	int8_t   LX;
	int8_t   LY;
	int8_t   RX;
	int8_t   RY;
	uint16_t Buttons;
	uint8_t  DPad;
	uint8_t  L2;
	uint8_t  R2;
	uint16_t Touch1X;
	uint16_t Touch1Y;
	uint8_t  Touch1Active;
	uint16_t Touch2X;
	uint16_t Touch2Y;
	uint8_t  Touch2Active;
	int16_t  GyroX;
	int16_t  GyroY;
	int16_t  GyroZ;
	int16_t  AccelX;
	int16_t  AccelY;
	int16_t  AccelZ;
} DS4DeviceState;

typedef struct {
	const char* SerialNumber;       // NULL = use default
	const char* Board;              // NULL = use default
	uint8_t     BatteryStatus;      // 0 = use default
	double      TemperatureCelsius; // 0 = use default
	double      BatteryVoltage;     // 0 = use default
	const char* BuildTime;          // NULL = use default (RFC3339 or "YYYY-MM-DD HH:MM:SS")
} DS4MetaState;

typedef void (*DS4OutputCallback)(DS4DeviceHandle handle, uint8_t rumbleSmall, uint8_t rumbleLarge, uint8_t ledRed, uint8_t ledGreen, uint8_t ledBlue, uint8_t flashOn, uint8_t flashOff);

static void viiper_call_ds4_output(DS4OutputCallback fn, DS4DeviceHandle handle, uint8_t rumbleSmall, uint8_t rumbleLarge, uint8_t ledRed, uint8_t ledGreen, uint8_t ledBlue, uint8_t flashOn, uint8_t flashOff) {
	fn(handle, rumbleSmall, rumbleLarge, ledRed, ledGreen, ledBlue, flashOn, flashOff);
}

typedef void (*DS4SpeakerCallback)(DS4DeviceHandle handle, const uint8_t* pcm, size_t length);

static void viiper_call_ds4_speaker(DS4SpeakerCallback fn, DS4DeviceHandle handle, const uint8_t* pcm, size_t length) {
	fn(handle, pcm, length);
}

typedef void (*DS4SpeakerResetCallback)(DS4DeviceHandle handle);

static void viiper_call_ds4_speaker_reset(DS4SpeakerResetCallback fn, DS4DeviceHandle handle) {
	fn(handle);
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
	"unsafe"

	"github.com/DualSenseClient/VIIPER/device"
	"github.com/DualSenseClient/VIIPER/device/dualshock4"
	"github.com/DualSenseClient/VIIPER/internal/server/api"
)

// CreateDS4Device creates a new DualShock 4 device on the bus with the given ID on the server associated with the given handle.
// @param serverHandle Handle to the USB server.
// @param outDeviceHandle Output parameter for the created device handle.
// @param busID ID of the bus to add the device to.
// @param autoAttachLocalhost If true, the device will be automatically attached to a USBIP-Client/Driver running on THIS machine.
// @param idVendor Optional USB vendor ID (0 = default).
// @param idProduct Optional USB product ID (0 = default).
// @param meta Optional pointer to initial device metadata. Pass NULL to use defaults.
//
//export CreateDS4Device
func CreateDS4Device(
	serverHandle C.USBServerHandle,
	outDeviceHandle *C.DS4DeviceHandle,
	busID uint32,
	autoAttachLocalhost bool,
	idVendor uint16,
	idProduct uint16,
	meta *C.DS4MetaState,
) bool {
	return createDS4Device(serverHandle, outDeviceHandle, busID, autoAttachLocalhost, idVendor, idProduct, meta, dualshock4.New)
}

// CreateDS4AudioOnlyDevice creates a DualShock 4 exposing only the audio
// interfaces and no HID gamepad interface.
// @param serverHandle Handle to the USB server.
// @param outDeviceHandle Output parameter for the created device handle.
// @param busID ID of the bus to add the device to.
// @param autoAttachLocalhost If true, the device will be automatically attached to a USBIP-Client/Driver running on THIS machine.
// @param idVendor Optional USB vendor ID (0 = default).
// @param idProduct Optional USB product ID (0 = default).
// @param meta Optional pointer to initial device metadata. Pass NULL to use defaults.
//
//export CreateDS4AudioOnlyDevice
func CreateDS4AudioOnlyDevice(
	serverHandle C.USBServerHandle,
	outDeviceHandle *C.DS4DeviceHandle,
	busID uint32,
	autoAttachLocalhost bool,
	idVendor uint16,
	idProduct uint16,
	meta *C.DS4MetaState,
) bool {
	return createDS4Device(serverHandle, outDeviceHandle, busID, autoAttachLocalhost, idVendor, idProduct, meta, dualshock4.NewAudioOnly)
}

func createDS4Device(
	serverHandle C.USBServerHandle,
	outDeviceHandle *C.DS4DeviceHandle,
	busID uint32,
	autoAttachLocalhost bool,
	idVendor uint16,
	idProduct uint16,
	meta *C.DS4MetaState,
	ctor func(*device.CreateOptions) (*dualshock4.DualShock4, error),
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
		goMeta := dualshock4.MetaState{
			SerialNumber:       goStringOrEmpty(meta.SerialNumber),
			Board:              goStringOrEmpty(meta.Board),
			BatteryStatus:      uint8(meta.BatteryStatus),
			TemperatureCelsius: float64(meta.TemperatureCelsius),
			BatteryVoltage:     float64(meta.BatteryVoltage),
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

	d, err := ctor(opts)
	if err != nil {
		return false
	}
	devCtx, err := bus.Add(d)
	if err != nil {
		return false
	}
	exportMeta := device.GetDeviceMeta(devCtx)
	if exportMeta == nil {
		return false
	}

	if autoAttachLocalhost {
		err := api.AttachLocalhostClient(
			context.Background(),
			exportMeta,
			shw.s.GetListenPort(),
			true,
			slog.Default(),
		)
		if err != nil {
			slog.Error("failed to auto-attach localhost client", "error", err)
			return false
		}
	}

	handleWrapper := &deviceHandleWrapper{
		device:     d,
		exportMeta: exportMeta,
		usbServer:  shw,
	}
	*outDeviceHandle = C.DS4DeviceHandle(cgo.NewHandle(handleWrapper))

	shw.mtx.Lock()
	defer shw.mtx.Unlock()
	shw.deviceHandles[busID] = append(shw.deviceHandles[busID], deviceHandle(*outDeviceHandle))
	return true
}

// SetDS4DeviceState updates the input state of the DualShock 4 device associated with the given handle.
// @param handle Handle to the DS4 device.
// @param state New input state to set on the device.
//
//export SetDS4DeviceState
func SetDS4DeviceState(handle C.DS4DeviceHandle, state C.DS4DeviceState) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	ds4device, ok := dhw.device.(*dualshock4.DualShock4)
	if !ok {
		return false
	}
	s := &dualshock4.InputState{
		LX:           int8(state.LX),
		LY:           int8(state.LY),
		RX:           int8(state.RX),
		RY:           int8(state.RY),
		Buttons:      uint16(state.Buttons),
		DPad:         uint8(state.DPad),
		L2:           uint8(state.L2),
		R2:           uint8(state.R2),
		Touch1X:      uint16(state.Touch1X),
		Touch1Y:      uint16(state.Touch1Y),
		Touch1Active: state.Touch1Active != 0,
		Touch2X:      uint16(state.Touch2X),
		Touch2Y:      uint16(state.Touch2Y),
		Touch2Active: state.Touch2Active != 0,
		GyroX:        int16(state.GyroX),
		GyroY:        int16(state.GyroY),
		GyroZ:        int16(state.GyroZ),
		AccelX:       int16(state.AccelX),
		AccelY:       int16(state.AccelY),
		AccelZ:       int16(state.AccelZ),
	}
	ds4device.UpdateInputState(s)
	return true
}

// SetDS4OutputCallback sets a callback to be invoked when the host sends output (rumble/LED) commands to the device.
// @param handle Handle to the DS4 device.
// @param callback Callback receiving rumbleSmall, rumbleLarge, ledRed, ledGreen, ledBlue, flashOn, flashOff. Pass NULL to clear.
//
//export SetDS4OutputCallback
func SetDS4OutputCallback(handle C.DS4DeviceHandle, cb C.DS4OutputCallback) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	ds4device, ok := dhw.device.(*dualshock4.DualShock4)
	if !ok {
		return false
	}
	if cb == nil {
		ds4device.SetOutputCallback(nil)
		return true
	}
	ds4device.SetOutputCallback(func(out dualshock4.OutputState) {
		C.viiper_call_ds4_output(cb, handle,
			C.uint8_t(out.RumbleSmall),
			C.uint8_t(out.RumbleLarge),
			C.uint8_t(out.LedRed),
			C.uint8_t(out.LedGreen),
			C.uint8_t(out.LedBlue),
			C.uint8_t(out.FlashOn),
			C.uint8_t(out.FlashOff),
		)
	})
	return true
}

// SetDS4SpeakerCallback sets a callback to be invoked when the host sends speaker PCM to the device.
// The callback receives the raw bytes written to the speaker audio-out endpoint: two S16LE
// channels at 32 kHz. Pass NULL to clear.
// @param handle Handle to the DS4 device.
// @param callback Callback receiving a PCM buffer. The buffer is only valid during the call.
//
//export SetDS4SpeakerCallback
func SetDS4SpeakerCallback(handle C.DS4DeviceHandle, cb C.DS4SpeakerCallback) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	ds4device, ok := dhw.device.(*dualshock4.DualShock4)
	if !ok {
		return false
	}
	if cb == nil {
		ds4device.SetSpeakerCallback(nil)
		return true
	}
	ds4device.SetSpeakerCallback(func(pcm []byte) {
		if len(pcm) == 0 {
			return
		}
		C.viiper_call_ds4_speaker(cb, handle, (*C.uint8_t)(unsafe.Pointer(&pcm[0])), C.size_t(len(pcm)))
	})
	return true
}

// SetDS4SpeakerResetCallback sets a callback invoked when the speaker audio
// interface alternate setting changes or the endpoint is reset. This marks a
// speaker-stream generation barrier, so transport queues should be flushed.
// Pass NULL to clear.
// @param handle Handle to the DS4 device.
// @param callback Callback with no arguments.
//
//export SetDS4SpeakerResetCallback
func SetDS4SpeakerResetCallback(handle C.DS4DeviceHandle, cb C.DS4SpeakerResetCallback) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	ds4device, ok := dhw.device.(*dualshock4.DualShock4)
	if !ok {
		return false
	}
	if cb == nil {
		ds4device.SetSpeakerResetCallback(nil)
		return true
	}
	ds4device.SetSpeakerResetCallback(func() {
		C.viiper_call_ds4_speaker_reset(cb, handle)
	})
	return true
}

// SetDS4MicrophonePCM queues a microphone PCM frame captured from the host-facing mic stream.
// The frame must be exactly 320 bytes (160 frames of one S16LE channel at 16 kHz).
// @param handle Handle to the DS4 device.
// @param data Pointer to the PCM frame.
// @param length Length of the PCM frame.
//
//export SetDS4MicrophonePCM
func SetDS4MicrophonePCM(handle C.DS4DeviceHandle, data *C.uint8_t, length C.size_t) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	ds4device, ok := dhw.device.(*dualshock4.DualShock4)
	if !ok {
		return false
	}
	if data == nil || length != dualshock4.USBMicrophoneClientFrameSize {
		return false
	}
	frame := C.GoBytes(unsafe.Pointer(data), C.int(length))
	ds4device.QueueMicrophonePCMFrame(frame)
	return true
}

// SetDS4MetaState updates the meta (identity/battery) state of the device at runtime.
// Fields left at their zero value (NULL/0) keep the current value, so a partial
// update only changes what the caller supplies.
// @param handle Handle to the DS4 device.
// @param meta Updated metadata. Pass NULL or a zeroed struct to change nothing.
//
//export SetDS4MetaState
func SetDS4MetaState(handle C.DS4DeviceHandle, meta *C.DS4MetaState) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	ds4device, ok := dhw.device.(*dualshock4.DualShock4)
	if !ok {
		return false
	}
	if meta == nil {
		return true
	}
	goMeta := dualshock4.MetaState{
		SerialNumber:       goStringOrEmpty(meta.SerialNumber),
		Board:              goStringOrEmpty(meta.Board),
		BatteryStatus:      uint8(meta.BatteryStatus),
		TemperatureCelsius: float64(meta.TemperatureCelsius),
		BatteryVoltage:     float64(meta.BatteryVoltage),
	}
	if bt, ok := parseDSBuildTime(goStringOrEmpty(meta.BuildTime)); ok {
		goMeta.BuildTime = bt
	}
	ds4device.UpdateMetaState(goMeta)
	return true
}

// RemoveDS4Device removes the DualShock 4 device associated with the given handle from the server.
// @param handle Handle to the DS4 device to remove.
//
//export RemoveDS4Device
func RemoveDS4Device(handle C.DS4DeviceHandle) bool {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return false
	}
	if err := dhw.usbServer.s.RemoveDeviceByID(dhw.exportMeta.BusID, fmt.Sprintf("%d", dhw.exportMeta.DevID)); err != nil {
		return false
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
