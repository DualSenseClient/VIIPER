package dualsense

import (
	"github.com/DualSenseClient/VIIPER/device"
	"github.com/DualSenseClient/VIIPER/internal/server/api"
	"github.com/DualSenseClient/VIIPER/usb"
)

func init() {
	api.RegisterDevice(DeviceTypeEdgeCombinedAudioDuplexV5, &dsedgehandler{})
	api.RegisterDevice(DeviceTypeEdgeAudioOnlyDuplexV5,
		&dsedgehandler{audioOnly: true})
	api.RegisterDevice(DeviceTypeEdgeGamepadOnlyV5,
		&dsedgehandler{gamepadOnly: true})
	api.RegisterDevice(DeviceTypeEdgeCombinedAudioDuplexV5Events,
		&dsedgehandler{micInterfaceEvents: true})
	api.RegisterDevice(DeviceTypeEdgeCombinedAudioDuplexV5RawInputEvents,
		&dsedgehandler{micInterfaceEvents: true, physicalInputMetadata: true})
	api.RegisterDevice(DeviceTypeEdgeGamepadOnlyV5RawInput,
		&dsedgehandler{gamepadOnly: true, physicalInputMetadata: true})
}

type dsedgehandler struct {
	audioOnly             bool
	gamepadOnly           bool
	micInterfaceEvents    bool
	physicalInputMetadata bool
}

func (h *dsedgehandler) CreateDevice(o *device.CreateOptions) (usb.Device, error) {
	if o == nil {
		o = &device.CreateOptions{}
	}
	lease, err := AcquireIdentity(o, true)
	if err != nil {
		return nil, err
	}

	dse, err := new(o, true)
	if err != nil {
		lease.Release()
		return nil, err
	}
	if h.audioOnly {
		dse.descriptor = makeAudioOnlyDescriptor(true)
		dse.deviceType = DeviceTypeEdgeAudioOnlyDuplexV5
	} else if h.gamepadOnly {
		dse.descriptor = makeGamepadOnlyDescriptor(true)
		if h.physicalInputMetadata {
			dse.deviceType = DeviceTypeEdgeGamepadOnlyV5RawInput
		} else {
			dse.deviceType = DeviceTypeEdgeGamepadOnlyV5
		}
	} else if h.physicalInputMetadata {
		dse.deviceType = DeviceTypeEdgeCombinedAudioDuplexV5RawInputEvents
	} else if h.micInterfaceEvents {
		dse.deviceType = DeviceTypeEdgeCombinedAudioDuplexV5Events
	}
	return dse, nil
}

func (h *dsedgehandler) StreamHandler() api.StreamHandlerFunc {
	return dualSenseV5StreamHandler("DualSense Edge", h.micInterfaceEvents,
		h.physicalInputMetadata)
}
