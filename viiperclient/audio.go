package viiperclient

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"time"
)

// Audio stream framing (route bus/{busId}/{deviceid}/audio, server to
// feeder): u16 LE length N followed by N bytes of speaker PCM (exact
// host-written EP1 OUT payloads, 4ch S16LE @48kHz, up to 392B).
// N == AudioResetSentinel carries a speaker-reset barrier event with no
// payload (emitted once per streaming generation).
const (
	AudioMaxFrame      = 392
	AudioResetSentinel = 0xFFFF
)

// AudioStream is a feeder connection to a device's speaker PCM stream.
type AudioStream struct {
	conn   net.Conn
	BusID  uint32
	DevID  string
	closed bool
}

// OpenAudioStream connects to an existing device's speaker PCM stream.
// Fails for device types without audio (server closes with "not supported").
func (c *Client) OpenAudioStream(ctx context.Context, busID uint32, devID string) (*AudioStream, error) {
	conn, err := c.dialStream(ctx, fmt.Sprintf("bus/%d/%s/audio\x00", busID, devID))
	if err != nil {
		return nil, err
	}
	return &AudioStream{conn: conn, BusID: busID, DevID: devID}, nil
}

// ReadFrame reads one speaker frame. reset is true for barrier events
// (pcm is nil then). Either a frame or a barrier is returned per call.
func (s *AudioStream) ReadFrame() (pcm []byte, reset bool, err error) {
	if s.closed {
		return nil, false, fmt.Errorf("stream closed")
	}
	var hdr [2]byte
	if _, err := io.ReadFull(s.conn, hdr[:]); err != nil {
		return nil, false, err
	}
	n := binary.LittleEndian.Uint16(hdr[:])
	if n == AudioResetSentinel {
		return nil, true, nil
	}
	if n > AudioMaxFrame {
		return nil, false, fmt.Errorf("speaker frame too large: %d", n)
	}
	pcm = make([]byte, n)
	if _, err := io.ReadFull(s.conn, pcm); err != nil {
		return nil, false, err
	}
	return pcm, false, nil
}

// SetReadDeadline sets the read deadline for the underlying connection.
func (s *AudioStream) SetReadDeadline(t time.Time) error {
	return s.conn.SetReadDeadline(t)
}

// Close closes the audio stream connection.
func (s *AudioStream) Close() error {
	if s.closed {
		return nil
	}
	s.closed = true
	return s.conn.Close()
}
