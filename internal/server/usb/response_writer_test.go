package usb

import (
	"encoding/binary"
	"testing"
	"time"

	"github.com/DualSenseClient/VIIPER/usbip"
	"github.com/stretchr/testify/require"
)

func TestResponseWriterEmitsOneContiguousRetSubmit(t *testing.T) {
	recorder := newRecordingWriter()
	writer := newResponseWriter(recorder, nil)
	payload := []byte{0x11, 0x22, 0x33}
	packets := []usbip.IsoPacketDescriptor{{
		Offset: 0, Length: 4, ActualLength: 3, Status: 0,
	}}
	packet := buildRetSubmitPacket(nil, 77, 0, 3, payload, packets, true)

	require.NoError(t, writer.write(packet, true, time.Now()))
	writes := recorder.waitForWrites(t, 1)
	require.Len(t, writes, 1)
	require.Len(t, writes[0].packet, retSubmitHeaderSize+len(payload)+isoPacketDescriptorSize)
	require.Equal(t, uint32(usbip.RetSubmitCode), binary.BigEndian.Uint32(writes[0].packet[0:4]))
	require.Equal(t, uint32(77), binary.BigEndian.Uint32(writes[0].packet[4:8]))
	require.Equal(t, payload, writes[0].packet[retSubmitHeaderSize:retSubmitHeaderSize+len(payload)])
	descriptorOffset := retSubmitHeaderSize + len(payload)
	require.Equal(t, uint32(3), binary.BigEndian.Uint32(
		writes[0].packet[descriptorOffset+8:descriptorOffset+12],
	))
}

type shortWriter struct {
	maximum int
	writes  int
	data    []byte
}

func (w *shortWriter) Write(packet []byte) (int, error) {
	w.writes++
	n := min(len(packet), w.maximum)
	w.data = append(w.data, packet[:n]...)
	return n, nil
}

func TestResponseWriterUsesWriteFullForShortWrites(t *testing.T) {
	destination := &shortWriter{maximum: 7}
	writer := newResponseWriter(destination, nil)
	packet := buildRetUnlinkPacket(nil, 91, errConnReset)

	require.NoError(t, writer.write(packet, true, time.Now()))
	require.Equal(t, packet, destination.data)
	require.Greater(t, destination.writes, 1)
}

func BenchmarkBuildRetSubmitInto(b *testing.B) {
	payload := make([]byte, 64)
	buffer := make([]byte, retSubmitHeaderSize+len(payload))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		buffer = buildRetSubmitPacket(
			buffer, uint32(i), 0, uint32(len(payload)), payload, nil, false,
		)
	}
}
