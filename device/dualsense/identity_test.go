package dualsense

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/DualSenseClient/VIIPER/device"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func resetIdentityRegistry() {
	identityMu.Lock()
	serials = map[string]struct{}{}
	macs = map[string]struct{}{}
	identityMu.Unlock()
}

func TestAcquireIdentityPatchesShellColorIntoSerial(t *testing.T) {
	resetIdentityRegistry()

	o := &device.CreateOptions{DeviceSpecific: `{"serial_number":"E55700GTD1190A500","shell_color":"Z1"}`}
	lease, err := AcquireIdentity(o, false)
	require.NoError(t, err)
	defer lease.Release()

	var meta MetaState
	require.NoError(t, json.Unmarshal([]byte(o.DeviceSpecific), &meta))
	assert.Equal(t, "E557Z1GTD1190A500", meta.SerialNumber)
	assert.Equal(t, lease.serial, meta.SerialNumber)
}

func TestAcquireIdentityDedupesAndReleaseFreesSlot(t *testing.T) {
	resetIdentityRegistry()

	// The default serial carries the default shell color code ("01").
	defaultSerial := "E55701GTD1190A500"

	first, err := AcquireIdentity(nil, false)
	require.NoError(t, err)
	assert.Equal(t, defaultSerial, first.serial)

	second, err := AcquireIdentity(nil, false)
	require.NoError(t, err)
	assert.Equal(t, "E55701GTD1190A501", second.serial)

	first.Release()
	third, err := AcquireIdentity(nil, false)
	require.NoError(t, err)
	assert.Equal(t, defaultSerial, third.serial)
	third.Release()
	second.Release()
}

func TestAcquireIdentityDedupesMAC(t *testing.T) {
	resetIdentityRegistry()

	first, err := AcquireIdentity(nil, false)
	require.NoError(t, err)
	defer first.Release()

	o := &device.CreateOptions{DeviceSpecific: `{"mac_address":"` + DefaultMACAddressDS + `"}`}
	second, err := AcquireIdentity(o, false)
	require.NoError(t, err)
	defer second.Release()

	var meta MetaState
	require.NoError(t, json.Unmarshal([]byte(o.DeviceSpecific), &meta))
	assert.NotEqual(t, DefaultMACAddressDS, meta.MACAddress)
	assert.True(t, strings.HasSuffix(meta.MACAddress, "01"))
}

func TestAcquireIdentityEdgeUsesEdgeDefaults(t *testing.T) {
	resetIdentityRegistry()

	o := &device.CreateOptions{}
	lease, err := AcquireIdentity(o, true)
	require.NoError(t, err)
	defer lease.Release()

	assert.Equal(t, "E55E01GTD1190A500", lease.serial)
	assert.Equal(t, DefaultMACAddressDSEdge, lease.mac)

	var meta MetaState
	require.NoError(t, json.Unmarshal([]byte(o.DeviceSpecific), &meta))
	assert.Equal(t, "E55E01GTD1190A500", meta.SerialNumber)
	assert.Equal(t, DefaultMACAddressDSEdge, meta.MACAddress)
}
