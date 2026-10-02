package dualsense

import (
	"context"
	"testing"

	"github.com/DualSenseClient/VIIPER/usbip"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSetInterfaceAlt(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)

	// SET_INTERFACE: bmRequestType 0x01, bRequest 0x0B, wValue alt, wIndex iface.
	_, handled := d.HandleControl(0x01, 0x0B, 1, 2, 0, nil)
	require.True(t, handled)
	d.mtx.Lock()
	assert.Equal(t, uint8(1), d.alts[2])
	d.mtx.Unlock()

	_, handled = d.HandleControl(0x01, 0x0B, 0, 2, 0, nil)
	require.True(t, handled)
	d.mtx.Lock()
	assert.Equal(t, uint8(0), d.alts[2])
	d.mtx.Unlock()

	// Out-of-range interface is accepted but ignored.
	_, handled = d.HandleControl(0x01, 0x0B, 1, 9, 0, nil)
	require.True(t, handled)
}

//nolint:unparam // bm documents USB bmRequestType (always 0xA1 for UAC GET).
func uacGet(t *testing.T, d *DualSense, bm, req uint8, cs, entity uint8) []byte {
	t.Helper()
	wValue := uint16(cs)<<8 | 0 // master channel
	wIndex := uint16(entity)<<8 | 0
	resp, handled := d.HandleControl(bm, req, wValue, wIndex, 64, nil)
	require.True(t, handled, "bm=0x%02X req=0x%02X cs=%d entity=0x%02X", bm, req, cs, entity)
	return resp
}

func uacSet(t *testing.T, d *DualSense, cs, entity uint8, data []byte) {
	t.Helper()
	wValue := uint16(cs)<<8 | 0
	wIndex := uint16(entity)<<8 | 0
	_, handled := d.HandleControl(0x21, 0x01, wValue, wIndex, uint16(len(data)), data)
	require.True(t, handled)
}

func TestUACMuteRoundTrip(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)

	assert.Equal(t, []byte{0x00}, uacGet(t, d, 0xA1, 0x81, 1, 0x02))
	uacSet(t, d, 1, 0x02, []byte{0x01})
	assert.Equal(t, []byte{0x01}, uacGet(t, d, 0xA1, 0x81, 1, 0x02))

	assert.Equal(t, []byte{0x00}, uacGet(t, d, 0xA1, 0x81, 1, 0x05))
	uacSet(t, d, 1, 0x05, []byte{0x01})
	assert.Equal(t, []byte{0x01}, uacGet(t, d, 0xA1, 0x81, 1, 0x05))
}

func TestUACVolumeRanges(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)

	// Defaults: 0 dB cur.
	assert.Equal(t, []byte{0x00, 0x00}, uacGet(t, d, 0xA1, 0x81, 2, 0x02))
	// Speaker ranges mirror the reference.
	assert.Equal(t, []byte{0x00, 0x9C}, uacGet(t, d, 0xA1, 0x82, 2, 0x02))
	assert.Equal(t, []byte{0x00, 0x00}, uacGet(t, d, 0xA1, 0x83, 2, 0x02))
	assert.Equal(t, []byte{0x00, 0x01}, uacGet(t, d, 0xA1, 0x84, 2, 0x02))
	// Mic ranges mirror the reference.
	assert.Equal(t, []byte{0x00, 0x00}, uacGet(t, d, 0xA1, 0x82, 2, 0x05))
	assert.Equal(t, []byte{0x00, 0x30}, uacGet(t, d, 0xA1, 0x83, 2, 0x05))
	assert.Equal(t, []byte{0x7A, 0x00}, uacGet(t, d, 0xA1, 0x84, 2, 0x05))

	// SET_CUR round-trips through GET_CUR.
	uacSet(t, d, 2, 0x02, []byte{0x34, 0x12})
	assert.Equal(t, []byte{0x34, 0x12}, uacGet(t, d, 0xA1, 0x81, 2, 0x02))
}

func TestUACUnknownEntityStalls(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)
	_, handled := d.HandleControl(0xA1, 0x81, 0x0100, 0x0700, 64, nil)
	assert.False(t, handled)
	// HID class traffic on the same bmRequestType bytes still falls through.
	_, handled = d.HandleControl(0x21, 0x09, 0x0302, 0x0003, 48, make([]byte, 48))
	assert.True(t, handled)
}

func TestIsochronousTransfers(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)
	ctx := context.Background()

	// Mic IN returns one silent 1ms frame.
	got := d.HandleTransfer(ctx, 2, usbip.DirIn, nil)
	require.Len(t, got, 192)
	for _, b := range got {
		assert.Zero(t, b)
	}

	// Speaker OUT is absorbed.
	assert.Nil(t, d.HandleTransfer(ctx, 1, usbip.DirOut, make([]byte, 392)))
}

func TestSpeakerSubscriptionExactBytes(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)
	ctx := context.Background()

	ch, unsub := d.SubscribeSpeaker()
	defer unsub()

	frame := make([]byte, 392)
	for i := range frame {
		frame[i] = byte(i)
	}
	// HandleTransfer takes caller scratch: mutate afterwards to prove copy.
	d.HandleTransfer(ctx, 1, usbip.DirOut, frame)
	for i := range frame {
		frame[i] = 0xFF
	}

	select {
	case ev := <-ch:
		assert.False(t, ev.Reset)
		require.Len(t, ev.PCM, 392)
		for i := range ev.PCM {
			assert.Equal(t, byte(i), ev.PCM[i])
		}
	default:
		t.Fatal("no speaker frame delivered")
	}
}

func TestSpeakerSubscriptionSlowDropsOldest(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)
	ctx := context.Background()

	ch, unsub := d.SubscribeSpeaker()
	defer unsub()

	// Overflow the 32-frame buffer without draining.
	for i := 0; i < 40; i++ {
		d.HandleTransfer(ctx, 1, usbip.DirOut, []byte{byte(i), 0xAA})
	}
	// Freshest frames survive; count what remains.
	var last byte
	n := 0
drain:
	for {
		select {
		case ev := <-ch:
			last = ev.PCM[0]
			n++
		default:
			break drain
		}
	}
	assert.Equal(t, 32, n)
	assert.Equal(t, byte(39), last)
}

func TestSpeakerUnsubscribeStops(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)
	ctx := context.Background()

	ch, unsub := d.SubscribeSpeaker()
	unsub()
	unsub() // idempotent
	d.HandleTransfer(ctx, 1, usbip.DirOut, []byte{0x01})
	// Closed channel: receives zero value with ok=false, never a frame.
	_, ok := <-ch
	assert.False(t, ok)
}

func TestResetBarrierOnAltChange(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)

	ch, unsub := d.SubscribeSpeaker()
	defer unsub()

	// No barrier at init.
	select {
	case <-ch:
		t.Fatal("barrier at init")
	default:
	}

	// Speaker alt 0 -> 1 fires exactly one barrier.
	_, handled := d.HandleControl(0x01, 0x0B, 1, 1, 0, nil)
	require.True(t, handled)
	select {
	case ev := <-ch:
		assert.True(t, ev.Reset)
	default:
		t.Fatal("no barrier on speaker alt change")
	}

	// Same value: no barrier.
	_, handled = d.HandleControl(0x01, 0x0B, 1, 1, 0, nil)
	require.True(t, handled)
	select {
	case <-ch:
		t.Fatal("barrier without change")
	default:
	}

	// Mic alt 0 -> 1 fires as well.
	_, handled = d.HandleControl(0x01, 0x0B, 1, 2, 0, nil)
	require.True(t, handled)
	select {
	case ev := <-ch:
		assert.True(t, ev.Reset)
	default:
		t.Fatal("no barrier on mic alt change")
	}

	// Non-audio interface: no barrier.
	_, handled = d.HandleControl(0x01, 0x0B, 1, 3, 0, nil)
	require.True(t, handled)
	select {
	case <-ch:
		t.Fatal("barrier on HID alt change")
	default:
	}
}
