package dualsense

import (
	"context"
	"testing"

	"github.com/DualSenseClient/VIIPER/usbip"
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

// The input report echoes the host-driven mute LED in b[54] bit 2,
// synthesized from the last output report (the dongle forwards the
// controller bit; the virtual device has no backing controller).
func TestInputReportMuteLightEcho(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)
	ctx := context.Background()

	// 48B output report with Flags1 + MuteLightMode at out[2]/out[9].
	output := func(flags1, mode uint8) []byte {
		out := make([]byte, 48)
		out[0] = ReportIDOutput
		out[2] = flags1
		out[9] = mode
		return out
	}

	// Default off.
	assert.Zero(t, neutralReport(t, d)[54]&0x04)

	// Without AllowMuteLight the mode byte is ignored.
	d.HandleTransfer(ctx, 3, usbip.DirOut, output(0x00, MuteLightOn))
	assert.Zero(t, neutralReport(t, d)[54]&0x04)

	// AllowMuteLight + On lights the bit; Breathing keeps it; Off clears.
	d.HandleTransfer(ctx, 3, usbip.DirOut, output(Flag1AllowMuteLight, MuteLightOn))
	assert.Equal(t, uint8(0x04), neutralReport(t, d)[54]&0x04)
	d.HandleTransfer(ctx, 3, usbip.DirOut, output(Flag1AllowMuteLight, MuteLightBreathing))
	assert.Equal(t, uint8(0x04), neutralReport(t, d)[54]&0x04)
	d.HandleTransfer(ctx, 3, usbip.DirOut, output(Flag1AllowMuteLight, MuteLightOff))
	assert.Zero(t, neutralReport(t, d)[54]&0x04)

	// DoNothing leaves the previous state (on here).
	d.HandleTransfer(ctx, 3, usbip.DirOut, output(Flag1AllowMuteLight, MuteLightOn))
	d.HandleTransfer(ctx, 3, usbip.DirOut, output(Flag1AllowMuteLight, MuteLightDoNothing))
	assert.Equal(t, uint8(0x04), neutralReport(t, d)[54]&0x04)

	// The control-path SET_REPORT output feeds the same synthesis.
	d.HandleTransfer(ctx, 3, usbip.DirOut, output(Flag1AllowMuteLight, MuteLightOff))
	assert.Zero(t, neutralReport(t, d)[54]&0x04)
	_, handled := d.HandleControl(0x21, 0x09, 0x0202, 0x0003, 48,
		output(Flag1AllowMuteLight, MuteLightOn))
	require.True(t, handled)
	assert.Equal(t, uint8(0x04), neutralReport(t, d)[54]&0x04)
}
