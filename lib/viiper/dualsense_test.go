package main

import (
	"testing"

	"github.com/DualSenseClient/VIIPER/device"
	"github.com/DualSenseClient/VIIPER/device/dualsense"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestApplyRawInputMetadata(t *testing.T) {
	t.Run("invalid metadata leaves state untouched", func(t *testing.T) {
		s := &dualsense.InputState{}
		applyRawInputMetadata(s, false, false, 42, []byte{1, 2, 3})
		assert.False(t, s.PhysicalMetadataValid)
		assert.Equal(t, uint32(0), s.PhysicalSensorTimestamp)
	})

	t.Run("valid metadata stamps all fields", func(t *testing.T) {
		s := &dualsense.InputState{}
		metadata := make([]byte, dualsense.InputStatePhysicalMetadataSize)
		for i := range metadata {
			metadata[i] = byte(i + 1)
		}

		applyRawInputMetadata(s, true, false, 0xDEADBEEF, metadata)

		assert.True(t, s.PhysicalMetadataValid)
		assert.False(t, s.PhysicalMetadataEdgeLayout)
		assert.Equal(t, uint32(0xDEADBEEF), s.PhysicalSensorTimestamp)
		want := [dualsense.InputStatePhysicalMetadataSize]byte{}
		for i := range want {
			want[i] = byte(i + 1)
		}
		assert.Equal(t, want, s.PhysicalInputMetadata)
	})

	t.Run("edge layout flag propagates", func(t *testing.T) {
		s := &dualsense.InputState{}
		applyRawInputMetadata(s, true, true, 0, nil)
		assert.True(t, s.PhysicalMetadataValid)
		assert.True(t, s.PhysicalMetadataEdgeLayout)
	})

	t.Run("short metadata leaves remaining bytes zero", func(t *testing.T) {
		s := &dualsense.InputState{}
		applyRawInputMetadata(s, true, false, 7, []byte{0xAA, 0xBB})
		assert.True(t, s.PhysicalMetadataValid)
		assert.Equal(t, uint32(7), s.PhysicalSensorTimestamp)
		assert.Equal(t, byte(0xAA), s.PhysicalInputMetadata[0])
		assert.Equal(t, byte(0xBB), s.PhysicalInputMetadata[1])
		assert.Equal(t, byte(0), s.PhysicalInputMetadata[2])
	})
}

func TestIsRegisteredDualSenseType(t *testing.T) {
	assert.True(t, isRegisteredDualSenseType(dualsense.DeviceTypeCombinedAudioDuplexV5))
	assert.True(t, isRegisteredDualSenseType(dualsense.DeviceTypeCombinedAudioDuplexV5RawInputEvents))
	assert.True(t, isRegisteredDualSenseType(dualsense.DeviceTypeEdgeGamepadOnlyV5RawInput))
	assert.True(t, isRegisteredDualSenseType("DUALSENSECOMBINEDAUDIODUPLEXV5"))
	assert.False(t, isRegisteredDualSenseType(""))
	assert.False(t, isRegisteredDualSenseType("dualsensebogus"))
	assert.False(t, isRegisteredDualSenseType("ns2pro"))
}

func TestBuildFromRegistryCreatesEveryRegisteredAlias(t *testing.T) {
	for _, deviceType := range []string{
		dualsense.DeviceTypeCombinedAudioDuplexV5,
		dualsense.DeviceTypeAudioOnlyDuplexV5,
		dualsense.DeviceTypeGamepadOnlyV5,
		dualsense.DeviceTypeCombinedAudioDuplexV5Events,
		dualsense.DeviceTypeAudioOnlyDuplexV5Events,
		dualsense.DeviceTypeCombinedAudioDuplexV5RawInputEvents,
		dualsense.DeviceTypeAudioOnlyDuplexV5RawInputEvents,
		dualsense.DeviceTypeGamepadOnlyV5RawInput,
		dualsense.DeviceTypeEdgeCombinedAudioDuplexV5,
		dualsense.DeviceTypeEdgeAudioOnlyDuplexV5,
		dualsense.DeviceTypeEdgeGamepadOnlyV5,
		dualsense.DeviceTypeEdgeCombinedAudioDuplexV5Events,
		dualsense.DeviceTypeEdgeCombinedAudioDuplexV5RawInputEvents,
		dualsense.DeviceTypeEdgeGamepadOnlyV5RawInput,
	} {
		dev, release, err := buildFromRegistry(deviceType)(&device.CreateOptions{})
		require.NoError(t, err, deviceType)
		assert.Equal(t, deviceType, dev.VIIPERDeviceType())
		release()
	}
}

func deviceSerial(t *testing.T, dev *dualsense.DualSense) string {
	t.Helper()
	telemetry := dev.GetDeviceSpecificArgs()
	serial, ok := telemetry["serial_number"].(string)
	require.True(t, ok, "serial_number missing from device telemetry")
	return serial
}

func TestBuildFromRegistryReleasedIdentityIsReusable(t *testing.T) {
	build := buildFromRegistry(dualsense.DeviceTypeGamepadOnlyV5RawInput)

	dev1, release1, err := build(&device.CreateOptions{})
	require.NoError(t, err)
	serial1 := deviceSerial(t, dev1)
	release1()

	dev2, release2, err := build(&device.CreateOptions{})
	require.NoError(t, err)
	defer release2()
	assert.Equal(t, serial1, deviceSerial(t, dev2))
}

func TestBuildFromRegistryHeldIdentityCollides(t *testing.T) {
	build := buildFromRegistry(dualsense.DeviceTypeGamepadOnlyV5RawInput)

	dev1, release1, err := build(&device.CreateOptions{})
	require.NoError(t, err)
	defer release1()

	dev2, release2, err := build(&device.CreateOptions{})
	require.NoError(t, err)
	defer release2()

	assert.NotEqual(t, deviceSerial(t, dev1), deviceSerial(t, dev2))
}
