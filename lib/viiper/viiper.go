package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef uintptr_t VIIPERDeviceHandle;
*/
import "C"
import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"runtime/cgo"
	"strings"
	"sync"
	"unsafe"

	"github.com/DualSenseClient/VIIPER/internal/server/usb"
	usblib "github.com/DualSenseClient/VIIPER/usb"
	"github.com/DualSenseClient/VIIPER/usbip"
)

func main() {}

func goStringOrEmpty(p *C.char) string {
	if p == nil {
		return ""
	}
	return C.GoString(p)
}

// GetDeviceTelemetry writes the device telemetry (the same DeviceSpecificArgs
// exposed by the bus/{id}/list API route) to buffer as a JSON object. Call it
// once with buffer = NULL to obtain the required size including the null
// terminator, then again with an adequately sized buffer; nothing is written
// when the buffer is NULL or too small. Returns 0 for an invalid handle.
// @param handle Handle to any device created by this library.
// @param buffer Output buffer for the null-terminated JSON. Pass NULL to query the required size.
// @param bufferSize Size of the output buffer in bytes.
//
//export GetDeviceTelemetry
func GetDeviceTelemetry(handle C.VIIPERDeviceHandle, buffer *C.char, bufferSize C.size_t) C.size_t {
	dh := cgo.Handle(handle)
	dhw, ok := dh.Value().(*deviceHandleWrapper)
	if !ok {
		return 0
	}
	dev, ok := dhw.device.(usblib.Device)
	if !ok {
		return 0
	}
	payload, err := marshalDeviceTelemetry(dev)
	if err != nil {
		return 0
	}
	var dst []byte
	if buffer != nil {
		dst = unsafe.Slice((*byte)(unsafe.Pointer(buffer)), bufferSize)
	}
	return C.size_t(writeTelemetry(dst, payload))
}

// marshalDeviceTelemetry renders a device's DeviceSpecificArgs as compact JSON.
func marshalDeviceTelemetry(dev usblib.Device) ([]byte, error) {
	payload, err := json.Marshal(dev.GetDeviceSpecificArgs())
	if err != nil {
		return nil, err
	}
	if len(payload) == 0 {
		return nil, fmt.Errorf("empty telemetry payload")
	}
	return payload, nil
}

// writeTelemetry reports the buffer size required for payload plus its null
// terminator. When dst is large enough the payload and terminator are written;
// otherwise dst is left untouched.
func writeTelemetry(dst, payload []byte) int {
	required := len(payload) + 1
	if len(dst) < required {
		return required
	}
	copy(dst, payload)
	dst[len(payload)] = 0
	return required
}

type deviceHandle cgo.Handle

type usbServerHandleWrapper struct {
	s             *usb.Server
	mtx           sync.Mutex
	deviceHandles map[uint32][]deviceHandle
}

type deviceHandleWrapper struct {
	device     any
	exportMeta *usbip.ExportMeta
	usbServer  *usbServerHandleWrapper
}

// ---

type funcLogHandler struct{ fn func(slog.Level, string) }

func (h *funcLogHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *funcLogHandler) WithAttrs([]slog.Attr) slog.Handler       { return h }
func (h *funcLogHandler) WithGroup(string) slog.Handler            { return h }
func (h *funcLogHandler) Handle(_ context.Context, r slog.Record) error {
	msg := r.Message
	r.Attrs(func(a slog.Attr) bool {
		msg += fmt.Sprintf(" %s=%v", a.Key, a.Value)
		return true
	})
	h.fn(r.Level, strings.TrimSpace(msg))
	return nil
}
