package dualsense

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func neutralReport(t *testing.T, d *DualSense) []byte {
	t.Helper()
	d.mtx.Lock()
	is := *d.inputState
	ms := *d.metaState
	d.mtx.Unlock()
	b := d.buildUSBInputReport(&is, &ms)
	require.Len(t, b, InputReportSize)
	return b
}

func TestInputReportBasics(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)
	b := neutralReport(t, d)
	assert.Equal(t, uint8(ReportIDInput), b[0])
	// Neutral sticks center at 128.
	assert.Equal(t, uint8(128), b[1])
	assert.Equal(t, uint8(128), b[2])
	// Neutral hat, no buttons.
	assert.Equal(t, uint8(DPadUSBNeutral), b[8]&DPadMask)
	assert.Equal(t, uint8(0), b[9])
	assert.Equal(t, uint8(0), b[10])
	// Untouched contacts carry the inactive mask.
	assert.Equal(t, TouchInactiveMask, b[33])
	assert.Equal(t, TouchInactiveMask, b[37])
	// Battery status from meta.
	assert.Equal(t, uint8(DefaultBatteryStatus), b[53])
}

func TestInputReportButtonsAndTriggers(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)
	s := NewInputState()
	s.Buttons = ButtonCross | ButtonL1 | ButtonPS
	s.DPad = DPadUp
	s.L2 = 0xFF
	d.UpdateInputState(s)
	b := neutralReport(t, d)
	assert.Equal(t, uint8(0xFF), b[5])
	assert.Equal(t, uint8(DPadUSBUp), b[8]&DPadMask)
	assert.NotZero(t, b[9])
	assert.NotZero(t, b[10])
}

func TestInputReportTouchTracking(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)

	// Feeder-supplied tracking IDs flow verbatim into the contact bytes.
	s := NewInputState()
	s.Touch1Active = true
	s.Touch1Tracking = 0x2A
	s.Touch1X, s.Touch1Y = 100, 200
	d.UpdateInputState(s)
	b := neutralReport(t, d)
	assert.Equal(t, uint8(0x2A), b[33])
	assert.Equal(t, uint8(100), b[34])

	// Release keeps the supplied ID under the inactive mask.
	s.Touch1Active = false
	d.UpdateInputState(s)
	b = neutralReport(t, d)
	assert.Equal(t, TouchInactiveMask|0x2A, b[33])

	// Untouched second finger stays at never-touched 0x80.
	assert.Equal(t, TouchInactiveMask, b[37])

	// Tracking survives the wire round-trip.
	wire, err := s.MarshalBinary()
	require.NoError(t, err)
	require.Len(t, wire, InputStateSize)
	var back InputState
	require.NoError(t, back.UnmarshalBinary(wire))
	assert.Equal(t, uint8(0x2A), back.Touch1Tracking)
}

func TestInputReportSeqAdvances(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)
	a := neutralReport(t, d)
	b := neutralReport(t, d)
	assert.Equal(t, a[7]+1, b[7])
}
