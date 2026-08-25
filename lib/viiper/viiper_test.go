package main

import (
	"encoding/json"
	"testing"

	"github.com/DualSenseClient/VIIPER/device/ns2pro"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func telemetryPayload(t *testing.T) []byte {
	t.Helper()
	dev, err := ns2pro.New(nil)
	require.NoError(t, err)
	payload, err := marshalDeviceTelemetry(dev)
	require.NoError(t, err)
	return payload
}

func TestWriteTelemetryTwoCallPattern(t *testing.T) {
	payload := telemetryPayload(t)

	buf := make([]byte, len(payload)+1)
	buf[0] = 'X'
	written := writeTelemetry(buf, payload)
	require.Equal(t, len(payload)+1, written)

	var telemetry map[string]any
	require.NoError(t, json.Unmarshal(buf[:len(payload)], &telemetry))
	assert.Contains(t, telemetry, "serial_number")
	assert.Equal(t, byte(0), buf[len(payload)])
}

func TestWriteTelemetryUndersizedBufferWritesNothing(t *testing.T) {
	payload := telemetryPayload(t)

	buf := make([]byte, 4)
	buf[0] = 'X'
	written := writeTelemetry(buf, payload)
	assert.Equal(t, len(payload)+1, written)
	assert.Equal(t, byte('X'), buf[0])
}

func TestWriteTelemetryNilBufferReturnsRequiredSize(t *testing.T) {
	payload := telemetryPayload(t)

	assert.Equal(t, len(payload)+1, writeTelemetry(nil, payload))
}
