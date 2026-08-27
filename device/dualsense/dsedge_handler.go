package dualsense

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/Alia5/VIIPER/device"
	"github.com/Alia5/VIIPER/internal/server/api"
	"github.com/Alia5/VIIPER/usb"
)

func init() {
	api.RegisterDevice(DeviceTypeEdgeCombinedAudioDuplexV5, &dsedgehandler{})
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
	gamepadOnly           bool
	micInterfaceEvents    bool
	physicalInputMetadata bool
}

func (h *dsedgehandler) CreateDevice(o *device.CreateOptions) (usb.Device, error) {
	if o == nil {
		o = &device.CreateOptions{}
	}

	metaState := MetaState{
		ShellColor: DefaultShellColor,
	}
	if o.DeviceSpecific != "" {
		if err := json.Unmarshal([]byte(o.DeviceSpecific), &metaState); err != nil {
			return nil, fmt.Errorf("invalid device specific JSON: %w", err)
		}
	}

	serial := metaState.SerialNumber
	if serial == "" {
		serial = DefaultSerialNumberDSEdge
	}
	if metaState.ShellColor != "" && len(serial) >= 6 {
		code := strings.ToUpper(metaState.ShellColor)
		if len(code) >= 2 {
			serial = serial[:4] + code[:2] + serial[6:]
		}
	}
	identityMu.Lock()
	if _, ok := serials[serial]; ok {
		if len(serial) < 2 {
			serial = DefaultSerialNumberDSEdge
		}
		for i := 1; i < 16; i++ {
			newSerial := fmt.Sprintf("%s%02X", serial[:len(serial)-2], i)
			if _, exists := serials[newSerial]; !exists {
				serial = newSerial
				break
			}
		}
	}
	metaState.SerialNumber = serial
	serials[serial] = struct{}{}

	mac := metaState.MACAddress
	if mac == "" {
		mac = DefaultMACAddressDSEdge
	}
	if _, ok := macs[mac]; ok {
		if len(mac) < 2 {
			mac = DefaultMACAddressDSEdge
		}
		prefix := mac[:len(mac)-2]
		for i := 1; i <= 16; i++ {
			candidate := fmt.Sprintf("%s%02X", prefix, i)
			if _, exists := macs[candidate]; !exists {
				mac = candidate
				break
			}
		}
	}
	metaState.MACAddress = mac
	macs[mac] = struct{}{}
	identityMu.Unlock()

	b, err := json.Marshal(metaState)
	if err != nil {
		identityMu.Lock()
		delete(serials, serial)
		delete(macs, mac)
		identityMu.Unlock()
		return nil, fmt.Errorf("marshal meta state: %w", err)
	}
	o.DeviceSpecific = string(b)

	dse, err := new(o, true)
	if err != nil {
		identityMu.Lock()
		delete(serials, serial)
		delete(macs, mac)
		identityMu.Unlock()
		return nil, err
	}
	if h.gamepadOnly {
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

func (h *dsedgehandler) UpdateMetaState(meta string, dev *usb.Device) error {
	dse, ok := (*dev).(*DualSense)
	if !ok {
		return fmt.Errorf("%w: expected DualSenseEdge", device.ErrWrongDeviceType)
	}
	dse.metaMu.Lock()
	current := *dse.metaState
	dse.metaMu.Unlock()
	if err := json.Unmarshal([]byte(meta), &current); err != nil {
		return fmt.Errorf("unmarshal meta state: %w", err)
	}
	dse.SetMetaState(current)
	return nil
}
