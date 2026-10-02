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

	// First touch: rising edge bumps tracking 0 -> 1.
	s := NewInputState()
	s.Touch1Active = true
	s.Touch1X, s.Touch1Y = 100, 200
	d.UpdateInputState(s)
	b := neutralReport(t, d)
	assert.Equal(t, uint8(1), b[33])
	assert.Equal(t, uint8(100), b[34])

	// Held touch keeps the same tracking ID.
	d.UpdateInputState(s)
	b = neutralReport(t, d)
	assert.Equal(t, uint8(1), b[33])

	// Release keeps the last ID with the inactive mask.
	s.Touch1Active = false
	d.UpdateInputState(s)
	b = neutralReport(t, d)
	assert.Equal(t, TouchInactiveMask|1, b[33])

	// Untouched second finger stays at never-touched 0x80.
	assert.Equal(t, TouchInactiveMask, b[37])
}

func TestInputReportSeqAdvances(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)
	a := neutralReport(t, d)
	b := neutralReport(t, d)
	assert.Equal(t, a[7]+1, b[7])
}
