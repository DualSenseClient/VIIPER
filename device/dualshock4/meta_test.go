package dualshock4

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeMetaState(t *testing.T) {
	d, err := New(nil)
	require.NoError(t, err)

	before := *d.metaState
	d.MergeMetaState(MetaState{BatteryStatus: 0x05})
	assert.Equal(t, uint8(0x05), d.metaState.BatteryStatus)
	assert.Equal(t, before.SerialNumber, d.metaState.SerialNumber)
	assert.Equal(t, before.Board, d.metaState.Board)
	assert.Equal(t, before.BatteryVoltage, d.metaState.BatteryVoltage)

	// Empty delta changes nothing.
	d.MergeMetaState(MetaState{})
	assert.Equal(t, uint8(0x05), d.metaState.BatteryStatus)

	// Serial merge flows into the MAC-derived feature reports.
	d.MergeMetaState(MetaState{SerialNumber: "2222060BF619A500"})
	b, handled := featureGet(t, d, featureIDIdentity)
	require.True(t, handled)
	s := serialStringToBytes("2222060BF619A500")
	assert.Equal(t, []byte{s[7], s[6], s[5], s[4], s[3], s[2]}, b)
}
