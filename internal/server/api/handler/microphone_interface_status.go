package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/DualSenseClient/VIIPER/internal/server/api"
	apierror "github.com/DualSenseClient/VIIPER/internal/server/api/error"
	"github.com/DualSenseClient/VIIPER/internal/server/usb"
)

type microphoneInterfaceStatusDevice interface {
	GetMicrophoneInterfaceStatus() map[string]any
}

// BusDeviceMicrophoneInterfaceStatus returns only the virtual microphone
// interface state and its microphone-buffer telemetry. It is deliberately
// separate from bus/{id}/list so legacy compatibility polling cannot trigger
// metadata JSON work for every device on the bus.
func BusDeviceMicrophoneInterfaceStatus(s *usb.Server) api.HandlerFunc {
	return func(req *api.Request, res *api.Response, _ *slog.Logger) error {
		busIDText, ok := req.Params["busId"]
		if !ok {
			return apierror.ErrBadRequest("missing busId parameter")
		}
		deviceIDText, ok := req.Params["devId"]
		if !ok {
			return apierror.ErrBadRequest("missing devId parameter")
		}
		busID, err := strconv.ParseUint(busIDText, 10, 32)
		if err != nil {
			return apierror.ErrBadRequest(fmt.Sprintf("invalid busId: %v", err))
		}
		bus := s.GetBus(uint32(busID))
		if bus == nil {
			return apierror.ErrNotFound(fmt.Sprintf("bus %d not found", busID))
		}
		deviceID, err := strconv.ParseUint(deviceIDText, 10, 32)
		if err != nil {
			return apierror.ErrBadRequest(fmt.Sprintf("invalid devId: %v", err))
		}
		dev, found := bus.GetDeviceByID(uint32(deviceID))
		if !found {
			return apierror.ErrNotFound(fmt.Sprintf(
				"device %s not found on bus %d", deviceIDText, busID))
		}
		statusDevice, supported := dev.(microphoneInterfaceStatusDevice)
		if !supported {
			return apierror.ErrBadRequest(fmt.Sprintf(
				"device %s does not expose microphone interface status", deviceIDText))
		}
		payload, err := json.Marshal(statusDevice.GetMicrophoneInterfaceStatus())
		if err != nil {
			return apierror.ErrInternal(fmt.Sprintf(
				"failed to marshal microphone interface status: %v", err))
		}
		res.JSON = string(payload)
		return nil
	}
}
