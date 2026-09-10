package main

/*
#include <stdint.h>
#include <stdlib.h>

typedef struct {
	char* addr; // default "0.0.0.0:3241"
	uint64_t connection_timeout_ms; // default 30000 (30s)
	uint64_t device_handler_connect_timeout_ms; // default 5000 (5s)
	uint32_t write_batch_flush_interval_ms; // default 1 (1ms)
} USBServerConfig;

typedef uintptr_t USBServerHandle;

typedef enum {
    VIIPER_LOG_DEBUG = -4,
    VIIPER_LOG_INFO  = 0,
    VIIPER_LOG_WARN  = 4,
    VIIPER_LOG_ERROR = 8,
} VIIPERLogLevel;

typedef void (*VIIPERLogCallback)(VIIPERLogLevel level, const char* message);

static void viiper_call_log(VIIPERLogCallback fn, VIIPERLogLevel level, const char* msg) {
	fn(level, msg);
}
*/
import "C"

import (
	"encoding/json"
	"log/slog"
	"runtime/cgo"
	"time"
	"unsafe"

	"github.com/DualSenseClient/VIIPER/internal/server/usb"
)

// NewUSBServer creates a new USB server with the given configuration and returns a handle to it.
// The server will run in the background and can be stopped by calling CloseUSBServer with the returned handle.
// @param config Server configuration
// @param outHandle Output parameter for the created server handle
// @param logCallback Optional callback function for log messages from the USB server
//
//export NewUSBServer
func NewUSBServer(config *C.USBServerConfig, outHandle *C.USBServerHandle, logCallback C.VIIPERLogCallback) bool {
	addr := C.GoString(config.addr)
	connectionTimeout := time.Duration(config.connection_timeout_ms) * time.Millisecond
	busCleanupTimeout := time.Duration(config.device_handler_connect_timeout_ms) * time.Millisecond
	writeBatchFlushInterval := time.Duration(config.write_batch_flush_interval_ms) * time.Millisecond

	if addr == "" {
		addr = ":3241"
	}
	if connectionTimeout == 0 {
		connectionTimeout = 30 * time.Second
	}
	if busCleanupTimeout == 0 {
		busCleanupTimeout = 5 * time.Second
	}

	var logger *slog.Logger
	if logCallback != nil {
		logger = slog.New(&funcLogHandler{
			func(level slog.Level, msg string) {
				if logCallback == nil {
					return
				}
				cMsg := C.CString(msg)
				defer C.free(unsafe.Pointer(cMsg))
				C.viiper_call_log(logCallback, C.VIIPERLogLevel(level), cMsg)
			},
		})
	} else {
		logger = slog.New(slog.DiscardHandler)
	}
	slog.SetDefault(logger)

	s := usb.New(usb.ServerConfig{
		Addr:                    addr,
		ConnectionTimeout:       connectionTimeout,
		BusCleanupTimeout:       busCleanupTimeout,
		WriteBatchFlushInterval: writeBatchFlushInterval,
	}, logger, nil)

	readyChan := s.Ready()
	errChan := make(chan error, 1)

	go func() {
		errChan <- s.ListenAndServe()
	}()

	select {
	case <-readyChan:
		wrapper := &usbServerHandleWrapper{
			s:             s,
			deviceHandles: make(map[uint32][]deviceHandle),
		}
		wrapper.useNativeIOCTL.Store(true)
		*outHandle = C.USBServerHandle(cgo.NewHandle(wrapper))
		return true
	case err := <-errChan:
		logger.Error("NewUSBServer: ListenAndServe failed", "error", err)
		return false
	}
}

// SetUSBAutoAttachWindowsNative controls how devices created with
// autoAttachLocalhost are attached on this machine on Windows: via the native
// usbip-win2 IOCTL (true, default) or by shelling out to usbip.exe (false).
// This mirrors the standalone app's api.auto-attach-windows-native option and
// has no effect on non-Windows hosts, where the usbip CLI is always used.
// @param serverHandle Handle to the USB server.
// @param useNativeIOCTL True to attach via the native IOCTL.
//
//export SetUSBAutoAttachWindowsNative
func SetUSBAutoAttachWindowsNative(serverHandle C.USBServerHandle, useNativeIOCTL bool) bool {
	sh := cgo.Handle(serverHandle)
	shw, ok := sh.Value().(*usbServerHandleWrapper)
	if !ok {
		return false
	}
	shw.useNativeIOCTL.Store(useNativeIOCTL)
	return true
}

// SetUSBAutoAttachLowLatency selects the usbip-win2 0.9.8.0 receive mode for
// native auto-attach on Windows: true for low-latency (wsk_events), false
// for zero-copy (default). Has no effect on 0.9.7.7 drivers, where the flag
// is ignored, or on non-native transports.
// @param serverHandle Handle to the USB server.
// @param useLowLatency True for low-latency receive mode.
//
//export SetUSBAutoAttachLowLatency
func SetUSBAutoAttachLowLatency(serverHandle C.USBServerHandle, useLowLatency bool) bool {
	sh := cgo.Handle(serverHandle)
	shw, ok := sh.Value().(*usbServerHandleWrapper)
	if !ok {
		return false
	}
	shw.useLowLatency.Store(useLowLatency)
	return true
}

// GetUSBEndpointDiagnostics writes aggregate USB/IP endpoint scheduling
// diagnostics for all currently attached connections to buffer as a JSON
// object. Call it once with buffer = NULL to obtain the required size
// including the null terminator, then again with an adequately sized buffer;
// nothing is written when the buffer is NULL or too small. Returns 0 for an
// invalid handle.
// @param handle Handle to the USB server.
// @param buffer Output buffer for the null-terminated JSON. Pass NULL to query the required size.
// @param bufferSize Size of the output buffer in bytes.
//
//export GetUSBEndpointDiagnostics
func GetUSBEndpointDiagnostics(handle C.USBServerHandle, buffer *C.char, bufferSize C.size_t) C.size_t {
	h := cgo.Handle(handle)
	hw, ok := h.Value().(*usbServerHandleWrapper)
	if !ok {
		return 0
	}
	payload, err := json.Marshal(hw.s.EndpointDiagnosticsSnapshot())
	if err != nil {
		return 0
	}
	var dst []byte
	if buffer != nil {
		dst = unsafe.Slice((*byte)(unsafe.Pointer(buffer)), bufferSize)
	}
	return C.size_t(writeTelemetry(dst, payload))
}

// CloseUSBServer closes the USB server associated with the given handle.
// Automatically removes busses and devices associated with the server.
// @param handle Handle to the USB server to close.
//
//export CloseUSBServer
func CloseUSBServer(handle C.USBServerHandle) bool {
	h := cgo.Handle(handle)
	hw, ok := h.Value().(*usbServerHandleWrapper)
	if !ok {
		return false
	}
	hw.mtx.Lock()
	defer hw.mtx.Unlock()

	for busID, dhs := range hw.deviceHandles {
		for _, dh := range dhs {
			h := cgo.Handle(dh)
			if w, ok := h.Value().(*deviceHandleWrapper); ok && w.releaseIdentity != nil {
				w.releaseIdentity()
			}
			h.Delete()
		}
		delete(hw.deviceHandles, busID)
	}
	hw.deviceHandles = nil

	if err := hw.s.Close(); err != nil {
		return false
	}
	h.Delete()
	return true
}
