package dualsense

import (
	"encoding/json"
	"fmt"
	"strings"

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
}

type dsedgehandler struct {
	audioOnly   bool
	gamepadOnly bool
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
	if _, ok := serials[serial]; ok {
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

	b, err := json.Marshal(metaState)
	if err != nil {
		return nil, fmt.Errorf("marshal meta state: %w", err)
	}
	o.DeviceSpecific = string(b)

	dse, err := new(o, true)
	if err != nil {
		return nil, err
	}
	if h.audioOnly {
		dse.descriptor = makeAudioOnlyDescriptor(true)
		dse.deviceType = DeviceTypeEdgeAudioOnlyDuplexV5
	} else if h.gamepadOnly {
		dse.descriptor = makeGamepadOnlyDescriptor(true)
		dse.deviceType = DeviceTypeEdgeGamepadOnlyV5
	}
	return dse, nil
}

func (h *dsedgehandler) StreamHandler() api.StreamHandlerFunc {
	return dualSenseV5StreamHandler("DualSense Edge")
}

func (h *dsedgehandler) UpdateMetaState(meta string, dev *usb.Device) error {
	dse, ok := (*dev).(*DualSense)
	if !ok {
		return fmt.Errorf("%w: expected DualSenseEdge", device.ErrWrongDeviceType)
	}
	dse.mtx.Lock()
	current := *dse.metaState
	dse.mtx.Unlock()
	if err := json.Unmarshal([]byte(meta), &current); err != nil {
		return fmt.Errorf("unmarshal meta state: %w", err)
	}
	dse.SetMetaState(current)
	return nil
}
