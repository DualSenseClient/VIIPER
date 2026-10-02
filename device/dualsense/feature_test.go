package dualsense

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// descriptorFeatureCounts extracts reportID -> payload count for every
// Feature main item in a HID report descriptor. IDs without an explicit
// ReportCount inherit the running count, mirroring HID semantics.
func descriptorFeatureCounts(t *testing.T, edge bool) map[byte]int {
	t.Helper()
	d, err := new(nil, edge)
	require.NoError(t, err)
	rb, err := d.GetDescriptor().Interfaces[0].HID.ReportBytes()
	require.NoError(t, err)
	b := []byte(rb)

	out := map[byte]int{}
	var pending *byte
	lastCount := 0
	for i := 0; i < len(b); {
		switch b[i] {
		case 0x85: // Report ID
			require.Less(t, i+1, len(b))
			id := b[i+1]
			pending = &id
			i += 2
		case 0x95: // Report Count
			require.Less(t, i+1, len(b))
			lastCount = int(b[i+1])
			i += 2
		case 0xB1: // Feature main
			require.NotNil(t, pending, "feature main without report ID")
			out[*pending] = lastCount
			pending = nil
			i += 2
		case 0x81, 0x91: // Input/Output main: not a feature
			pending = nil
			i += 2
		default:
			i++
		}
	}
	return out
}

// Every feature ID in the descriptor must resolve to a GET response of
// exactly count+1 bytes on its variant.
func TestFeatureTableMatchesDescriptor(t *testing.T) {
	for _, edge := range []bool{false, true} {
		d, err := new(nil, edge)
		require.NoError(t, err)
		for id, count := range descriptorFeatureCounts(t, edge) {
			b := d.getFeatureReport(id)
			require.NotNil(t, b, "edge=%v id=0x%02X", edge, id)
			assert.Len(t, b, count+1, "edge=%v id=0x%02X", edge, id)
			assert.Equal(t, id, b[0])
		}
	}
}

// Unknown IDs must not resolve (host sees a stall, as before).
func TestFeatureUnknownStalls(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)
	assert.Nil(t, d.getFeatureReport(0xFF))
	assert.Nil(t, d.getFeatureReport(0x04))
	de, err := new(nil, true)
	require.NoError(t, err)
	assert.Nil(t, de.getFeatureReport(0xFF))
	// Edge-only IDs stall on DS.
	assert.Nil(t, d.getFeatureReport(0x70))
	assert.NotNil(t, de.getFeatureReport(0x70))
}
