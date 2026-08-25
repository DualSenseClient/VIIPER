package ns2pro

import (
	"encoding/json"
	"fmt"

	"github.com/DualSenseClient/VIIPER/device"
)

// IdentityLease represents a reserved device identity registration.
type IdentityLease struct {
	serial string
}

// Release frees the identity slot reserved by AcquireIdentity.
func (l *IdentityLease) Release() {
	if l == nil {
		return
	}
	serialsMu.Lock()
	delete(serials, l.serial)
	serialsMu.Unlock()
}

// AcquireIdentity resolves the device identity from the metadata encoded in
// o.DeviceSpecific: colliding serials are suffixed until unique. The resolved
// meta state is marshaled back into o.DeviceSpecific for device construction.
// The caller must invoke Lease.Release once the device is destroyed.
func AcquireIdentity(o *device.CreateOptions) (*IdentityLease, error) {
	if o == nil {
		o = &device.CreateOptions{}
	}

	metaState := *defaultMetaState()
	if o.DeviceSpecific != "" {
		if err := json.Unmarshal([]byte(o.DeviceSpecific), &metaState); err != nil {
			return nil, fmt.Errorf("invalid device specific JSON: %w", err)
		}
	}
	serial := metaState.SerialNumber
	if serial == "" {
		serial = DefaultSerial
	}
	serialsMu.Lock()
	if _, ok := serials[serial]; ok {
		if len(serial) < 2 {
			serial = DefaultSerial
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
	serialsMu.Unlock()

	b, err := json.Marshal(metaState)
	if err != nil {
		serialsMu.Lock()
		delete(serials, serial)
		serialsMu.Unlock()
		return nil, fmt.Errorf("marshal meta state: %w", err)
	}
	o.DeviceSpecific = string(b)
	return &IdentityLease{serial: serial}, nil
}
