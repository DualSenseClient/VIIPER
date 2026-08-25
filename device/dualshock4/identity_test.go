package dualshock4

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DualSenseClient/VIIPER/device"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetIdentityRegistry() {
	serialsMu.Lock()
	serials = map[string]struct{}{}
	serialsMu.Unlock()
}

func TestAcquireIdentityPadsAndDedupesSerial(t *testing.T) {
	resetIdentityRegistry()

	o := &device.CreateOptions{DeviceSpecific: `{"serial_number":"ABC"}`}
	first, err := AcquireIdentity(o)
	require.NoError(t, err)

	var meta MetaState
	require.NoError(t, json.Unmarshal([]byte(o.DeviceSpecific), &meta))
	assert.Equal(t, "0000000000000ABC", meta.SerialNumber)
	assert.Equal(t, "0000000000000ABC", first.serial)

	again := &device.CreateOptions{DeviceSpecific: `{"serial_number":"0000000000000ABC"}`}
	second, err := AcquireIdentity(again)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(second.serial, "01"))
	assert.NotEqual(t, first.serial, second.serial)

	// AcquireIdentity rewrites DeviceSpecific with the resolved identity, so a
	// repeat attempt needs a fresh payload of the original request, and the
	// original lease must be released for its slot to become available again.
	first.Release()
	fresh := &device.CreateOptions{DeviceSpecific: `{"serial_number":"0000000000000ABC"}`}
	third, err := AcquireIdentity(fresh)
	require.NoError(t, err)
	assert.Equal(t, "0000000000000ABC", third.serial)
	third.Release()
	second.Release()
}

func TestAcquireIdentityDefaultsWhenUnset(t *testing.T) {
	resetIdentityRegistry()

	o := &device.CreateOptions{}
	lease, err := AcquireIdentity(o)
	require.NoError(t, err)
	defer lease.Release()

	assert.Equal(t, "1111020BF619A500", lease.serial)
	assert.Contains(t, o.DeviceSpecific, `"serial_number":"1111020BF619A500"`)
}
