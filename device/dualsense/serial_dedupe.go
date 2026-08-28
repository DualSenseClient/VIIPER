package dualsense

import (
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/DualSenseClient/VIIPER/device"
	"github.com/DualSenseClient/VIIPER/usb"
)

var (
	identityMu sync.Mutex
	serials    = map[string]struct{}{}
	macs       = map[string]struct{}{}
)

// IdentityLease represents a reserved device identity registration.
type IdentityLease struct {
	serial string
	mac    string
}

// Release frees the identity slots reserved by AcquireIdentity.
func (l *IdentityLease) Release() {
	if l == nil {
		return
	}
	releaseIdentitySlots(l.serial, l.mac)
}

// ReleaseDeviceIdentity frees the identity slots currently reserved for dev.
// It exists for creators that construct devices through the registered device
// handlers (which acquire identity internally without returning a lease), such
// as libVIIPER.
func ReleaseDeviceIdentity(dev usb.Device) {
	dse, ok := dev.(*DualSense)
	if !ok {
		return
	}
	dse.metaMu.Lock()
	serial := dse.metaState.SerialNumber
	mac := dse.metaState.MACAddress
	dse.metaMu.Unlock()
	releaseIdentitySlots(serial, mac)
}

func releaseIdentitySlots(serial, mac string) {
	identityMu.Lock()
	delete(serials, serial)
	delete(macs, mac)
	identityMu.Unlock()
}

// AcquireIdentity resolves the device identity from the metadata encoded in
// o.DeviceSpecific: the serial's color-code bytes are patched from ShellColor
// and colliding serials/MACs are suffixed until unique. The resolved meta
// state is marshaled back into o.DeviceSpecific for device construction.
// The caller must invoke Lease.Release once the device is destroyed.
func AcquireIdentity(o *device.CreateOptions, edge bool) (*IdentityLease, error) {
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

	defaultSerial := DefaultSerialNumberDS
	defaultMAC := DefaultMACAddressDS
	if edge {
		defaultSerial = DefaultSerialNumberDSEdge
		defaultMAC = DefaultMACAddressDSEdge
	}

	serial := metaState.SerialNumber
	if serial == "" {
		serial = defaultSerial
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
			serial = defaultSerial
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
		mac = defaultMAC
	}
	if _, ok := macs[mac]; ok {
		if len(mac) < 2 {
			mac = defaultMAC
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
		releaseIdentitySlots(serial, mac)
		return nil, fmt.Errorf("marshal meta state: %w", err)
	}
	o.DeviceSpecific = string(b)
	return &IdentityLease{serial: serial, mac: mac}, nil
}
