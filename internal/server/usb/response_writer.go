package usb

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/Alia5/VIIPER/usbip"
)

const isoPacketDescriptorSize = 16

// responseWriter is the sole owner of a USB/IP connection's response stream.
// Callers build a complete response in endpoint-owned memory before calling
// write. The send lock consequently protects only one write-full operation;
// report encoding, ISO descriptor construction, and device work never execute
// while socket ownership is held.
type responseWriter struct {
	mu      sync.Mutex
	dst     io.Writer
	batcher *batchingWriter

	responseQueueWait durationHistogram
	sendLockWait      durationHistogram
	socketWrite       durationHistogram
}

// USBIPResponseDiagnostics contains aggregate serialization-stage latency for
// one attached USB/IP response stream.
type USBIPResponseDiagnostics struct {
	ResponseQueueWait DurationHistogramSnapshot
	SendLockWait      DurationHistogramSnapshot
	SocketWrite       DurationHistogramSnapshot
}

func (w *responseWriter) snapshot() USBIPResponseDiagnostics {
	return USBIPResponseDiagnostics{
		ResponseQueueWait: w.responseQueueWait.snapshot(),
		SendLockWait:      w.sendLockWait.snapshot(),
		SocketWrite:       w.socketWrite.snapshot(),
	}
}

func newResponseWriter(dst io.Writer, batcher *batchingWriter) *responseWriter {
	return &responseWriter{dst: dst, batcher: batcher}
}

func (w *responseWriter) write(packet []byte, flush bool, readyAt time.Time) error {
	_, err := w.writeIf(packet, flush, readyAt, nil)
	return err
}

// writeIf acquires send ownership and then calls claim immediately before the
// first byte can be emitted. Endpoint workers use this hand-off to make unlink
// ordering exact without holding a queue lock during socket I/O: either unlink
// cancels the logical response first, or the response owns the stream first
// and RET_UNLINK necessarily follows it.
func (w *responseWriter) writeIf(
	packet []byte,
	flush bool,
	readyAt time.Time,
	claim func() bool,
) (bool, error) {
	return w.writeIfThen(packet, flush, readyAt, claim, nil)
}

// writeIfThen is writeIf with an ownership-completion callback. afterWrite is
// invoked while send ownership is still held, after writeFull (and any
// required flush) has finished but before another response can validate
// state. The callback must be fixed-cost and must not perform I/O. This lets a
// successful interrupt presentation advance its device version before a
// waiting control GET_REPORT is allowed to validate a snapshot.
func (w *responseWriter) writeIfThen(
	packet []byte,
	flush bool,
	readyAt time.Time,
	claim func() bool,
	afterWrite func(success bool),
) (bool, error) {
	lockStarted := time.Now()
	if !readyAt.IsZero() {
		w.responseQueueWait.record(lockStarted.Sub(readyAt))
	}
	w.mu.Lock()
	lockedAt := time.Now()
	w.sendLockWait.record(lockedAt.Sub(lockStarted))
	if claim != nil && !claim() {
		if afterWrite != nil {
			afterWrite(false)
		}
		w.mu.Unlock()
		return false, nil
	}

	writeStarted := time.Now()
	err := writeFull(w.dst, packet)
	if err == nil && flush && w.batcher != nil {
		err = w.batcher.Flush()
	}
	writeCompleted := time.Now()
	if afterWrite != nil {
		afterWrite(err == nil)
	}
	w.mu.Unlock()

	w.socketWrite.record(writeCompleted.Sub(writeStarted))
	if err != nil {
		return true, fmt.Errorf("write USB/IP response: %w", err)
	}
	return true, nil
}

func writeFull(w io.Writer, packet []byte) error {
	for len(packet) > 0 {
		n, err := w.Write(packet)
		if n > 0 {
			packet = packet[n:]
		}
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrNoProgress
		}
	}
	return nil
}

// buildRetSubmitPacket serializes one complete RET_SUBMIT into dst. The
// returned slice remains owned by the caller and must not be reused until
// responseWriter.write returns.
func buildRetSubmitPacket(
	dst []byte,
	seq uint32,
	status int32,
	actualLen uint32,
	respData []byte,
	isoPackets []usbip.IsoPacketDescriptor,
	isIso bool,
) []byte {
	total := retSubmitHeaderSize + len(respData)
	if isIso {
		total += len(isoPackets) * isoPacketDescriptorSize
	}
	if cap(dst) < total {
		dst = make([]byte, total)
	} else {
		dst = dst[:total]
		clear(dst)
	}

	binary.BigEndian.PutUint32(dst[0:4], usbip.RetSubmitCode)
	binary.BigEndian.PutUint32(dst[4:8], seq)
	// devid, direction, and endpoint are zero in USB/IP return headers.
	binary.BigEndian.PutUint32(dst[20:24], uint32(status))
	binary.BigEndian.PutUint32(dst[24:28], actualLen)
	packetCount := int32(-1)
	if isIso {
		packetCount = int32(len(isoPackets))
	}
	binary.BigEndian.PutUint32(dst[32:36], uint32(packetCount))

	offset := retSubmitHeaderSize
	copy(dst[offset:], respData)
	offset += len(respData)
	if isIso {
		for _, packet := range isoPackets {
			binary.BigEndian.PutUint32(dst[offset:offset+4], packet.Offset)
			binary.BigEndian.PutUint32(dst[offset+4:offset+8], packet.Length)
			binary.BigEndian.PutUint32(dst[offset+8:offset+12], packet.ActualLength)
			binary.BigEndian.PutUint32(dst[offset+12:offset+16], uint32(packet.Status))
			offset += isoPacketDescriptorSize
		}
	}
	return dst
}

func buildRetUnlinkPacket(dst []byte, seq uint32, status int32) []byte {
	if cap(dst) < retSubmitHeaderSize {
		dst = make([]byte, retSubmitHeaderSize)
	} else {
		dst = dst[:retSubmitHeaderSize]
		clear(dst)
	}
	binary.BigEndian.PutUint32(dst[0:4], usbip.RetUnlinkCode)
	binary.BigEndian.PutUint32(dst[4:8], seq)
	binary.BigEndian.PutUint32(dst[20:24], uint32(status))
	return dst
}
