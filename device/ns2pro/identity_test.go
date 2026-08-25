package ns2pro

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

func TestAcquireIdentityDefaultsWhenUnset(t *testing.T) {
	resetIdentityRegistry()

	o := &device.CreateOptions{}
	lease, err := AcquireIdentity(o)
	require.NoError(t, err)
	defer lease.Release()

	assert.Equal(t, DefaultSerial, lease.serial)
	var meta MetaState
	require.NoError(t, json.Unmarshal([]byte(o.DeviceSpecific), &meta))
	assert.Equal(t, DefaultSerial, meta.SerialNumber)
	assert.Equal(t, BatteryMax, meta.BatteryLevel) // defaults preserved
}

func TestAcquireIdentityDedupesAndReleaseFreesSlot(t *testing.T) {
	resetIdentityRegistry()

	first, err := AcquireIdentity(nil)
	require.NoError(t, err)
	assert.Equal(t, DefaultSerial, first.serial)

	second, err := AcquireIdentity(nil)
	require.NoError(t, err)
	assert.True(t, strings.HasSuffix(second.serial, "01"))
	assert.NotEqual(t, first.serial, second.serial)

	first.Release()
	third, err := AcquireIdentity(nil)
	require.NoError(t, err)
	assert.Equal(t, DefaultSerial, third.serial)
	third.Release()
	second.Release()
}
