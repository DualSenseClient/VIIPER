package main

import (
	"encoding/json"
	"testing"

	"github.com/DualSenseClient/VIIPER/internal/server/usb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEndpointDiagnosticsSnapshotMarshalsWithoutConnections(t *testing.T) {
	snapshot := (&usb.Server{}).EndpointDiagnosticsSnapshot()

	payload, err := json.Marshal(snapshot)
	require.NoError(t, err)

	var decoded map[string]any
	require.NoError(t, json.Unmarshal(payload, &decoded))
	connections, ok := decoded["Connections"].([]any)
	require.True(t, ok, "Connections missing from diagnostics JSON")
	assert.Empty(t, connections)
	assert.NotZero(t, decoded["CapturedAt"])
}
