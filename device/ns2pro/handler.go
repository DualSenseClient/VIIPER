package ns2pro

import (
	"fmt"
	"io"
	"log/slog"
	"net"
	"sync"

	"github.com/DualSenseClient/VIIPER/device"
	"github.com/DualSenseClient/VIIPER/internal/server/api"
	"github.com/DualSenseClient/VIIPER/usb"
)

func init() {
	api.RegisterDevice("ns2pro", &handler{})
}

type handler struct{}

var (
	serialsMu sync.Mutex
	serials   = map[string]struct{}{}
)

func (h *handler) CreateDevice(o *device.CreateOptions) (usb.Device, error) {
	if o == nil {
		o = &device.CreateOptions{}
	}
	lease, err := AcquireIdentity(o)
	if err != nil {
		return nil, err
	}

	result, err := New(o)
	if err != nil {
		lease.Release()
	}
	return result, err
}

func (h *handler) StreamHandler() api.StreamHandlerFunc {
	return func(conn net.Conn, devPtr *usb.Device, logger *slog.Logger) error {
		defer func() {
			if devPtr == nil || *devPtr == nil {
				return
			}
			ns2, ok := (*devPtr).(*NS2Pro)
			if !ok {
				slog.Warn("device is not ns2pro on disconnect")
				return
			}
			serial := ns2.serialNumber()
			if serial == "" {
				return
			}
			serialsMu.Lock()
			delete(serials, serial)
			serialsMu.Unlock()
			slog.Debug("ns2pro disconnected, serial released", "serial", serial)
		}()

		if devPtr == nil || *devPtr == nil {
			return fmt.Errorf("nil device")
		}
		ns2, ok := (*devPtr).(*NS2Pro)
		if !ok {
			return fmt.Errorf("device is not ns2pro")
		}

		clearOutputCallback := ns2.SetOutputCallback(func(feedback OutputState) {
			data, err := feedback.MarshalBinary()
			if err != nil {
				logger.Error("failed to marshal ns2pro feedback", "error", err)
				return
			}
			if _, err := conn.Write(data); err != nil {
				logger.Error("failed to send ns2pro feedback", "error", err)
			}
		})
		defer clearOutputCallback()

		buf := make([]byte, InputWireSize)
		for {
			if _, err := io.ReadFull(conn, buf); err != nil {
				if err == io.EOF {
					logger.Info("client disconnected")
					return nil
				}
				return fmt.Errorf("read input state: %w", err)
			}

			var state InputState
			if err := state.UnmarshalBinary(buf); err != nil {
				return fmt.Errorf("unmarshal input state: %w", err)
			}
			ns2.UpdateInputState(state)
		}
	}
}
