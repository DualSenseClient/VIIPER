package usb

import (
	"bytes"
	"context"
	"encoding/binary"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
	"time"

	usbdesc "github.com/Alia5/VIIPER/usb"
	"github.com/Alia5/VIIPER/usbip"
	"github.com/Alia5/VIIPER/virtualbus"
	"github.com/stretchr/testify/require"
)

func makeIsoDescriptorReadStream() []byte {
	const packetCount = 8
	stream := make([]byte, urbHdrSize+packetCount*usbip.IsoPacketDescriptorSize)
	binary.BigEndian.PutUint32(stream[urbHdrOffsetCommand:urbHdrOffsetCommand+4],
		usbip.CmdSubmitCode)
	binary.BigEndian.PutUint32(stream[urbHdrOffsetDir:urbHdrOffsetDir+4], usbip.DirOut)
	binary.BigEndian.PutUint32(stream[urbHdrOffsetEp:urbHdrOffsetEp+4], 1)
	binary.BigEndian.PutUint32(stream[urbHdrOffsetPackets:urbHdrOffsetPackets+4],
		uint32(packetCount))
	for index := 0; index < packetCount; index++ {
		offset := urbHdrSize + index*usbip.IsoPacketDescriptorSize
		binary.BigEndian.PutUint32(stream[offset:offset+4], uint32(index*192))
		binary.BigEndian.PutUint32(stream[offset+4:offset+8], 192)
		binary.BigEndian.PutUint32(stream[offset+8:offset+12], uint32(index+1))
		binary.BigEndian.PutUint32(stream[offset+12:offset+16], 0xffffff98)
	}
	return stream
}

func readLoadedURBScratch(
	reader *bytes.Reader,
	stream []byte,
	header []byte,
	descriptorWire []byte,
	packets []usbip.IsoPacketDescriptor,
) error {
	reader.Reset(stream)
	if err := usbip.ReadExactly(reader, header); err != nil {
		return err
	}
	return readIsoPacketDescriptors(reader, descriptorWire, packets)
}

func TestLoadedURBHeaderAndIsoDescriptorReadAllocatesZero(t *testing.T) {
	const packetCount = 8
	stream := makeIsoDescriptorReadStream()
	reader := bytes.NewReader(stream)
	var header [urbHdrSize]byte
	var descriptorWire [packetCount * usbip.IsoPacketDescriptorSize]byte
	var packets [packetCount]usbip.IsoPacketDescriptor

	require.NoError(t, readLoadedURBScratch(
		reader, stream, header[:], descriptorWire[:], packets[:],
	))
	require.Equal(t, usbip.IsoPacketDescriptor{
		Offset: 7 * 192, Length: 192, ActualLength: 8, Status: -104,
	}, packets[7])

	allocations := testing.AllocsPerRun(1000, func() {
		if err := readLoadedURBScratch(
			reader, stream, header[:], descriptorWire[:], packets[:],
		); err != nil {
			panic(err)
		}
	})
	require.Zero(t, allocations)
}

func BenchmarkLoadedURBHeaderAndIsoDescriptorRead(b *testing.B) {
	const packetCount = 8
	stream := makeIsoDescriptorReadStream()
	reader := bytes.NewReader(stream)
	var header [urbHdrSize]byte
	var descriptorWire [packetCount * usbip.IsoPacketDescriptorSize]byte
	var packets [packetCount]usbip.IsoPacketDescriptor

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := readLoadedURBScratch(
			reader, stream, header[:], descriptorWire[:], packets[:],
		); err != nil {
			b.Fatal(err)
		}
	}
}

func TestLoadedURBDecodeAndEndpointClockAllocatesZero(t *testing.T) {
	const packetCount = 8
	stream := makeIsoDescriptorReadStream()
	reader := bytes.NewReader(stream)
	var header [urbHdrSize]byte
	var descriptorWire [packetCount * usbip.IsoPacketDescriptorSize]byte
	var packets [packetCount]usbip.IsoPacketDescriptor

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clock := realtimeEndpointClock{}
	wake := make(chan struct{})
	trigger := make(chan struct{})
	senderDone := make(chan struct{})
	go func() {
		defer close(senderDone)
		for range trigger {
			wake <- struct{}{}
		}
	}()
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	allocations := testing.AllocsPerRun(1000, func() {
		if err := readLoadedURBScratch(
			reader, stream, header[:], descriptorWire[:], packets[:],
		); err != nil {
			panic(err)
		}
		trigger <- struct{}{}
		if result := clock.WaitUntil(ctx, wake, timer, time.Now().Add(time.Hour)); result != endpointWaitWake {
			panic("unexpected endpoint clock result")
		}
	})
	close(trigger)
	<-senderDone
	require.Zero(t, allocations)
}

func BenchmarkLoadedURBDecodeAndEndpointClock(b *testing.B) {
	const packetCount = 8
	stream := makeIsoDescriptorReadStream()
	reader := bytes.NewReader(stream)
	var header [urbHdrSize]byte
	var descriptorWire [packetCount * usbip.IsoPacketDescriptorSize]byte
	var packets [packetCount]usbip.IsoPacketDescriptor

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	clock := realtimeEndpointClock{}
	wake := make(chan struct{})
	trigger := make(chan struct{})
	senderDone := make(chan struct{})
	go func() {
		defer close(senderDone)
		for range trigger {
			wake <- struct{}{}
		}
	}()
	timer := time.NewTimer(time.Hour)
	if !timer.Stop() {
		<-timer.C
	}
	defer timer.Stop()

	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if err := readLoadedURBScratch(
			reader, stream, header[:], descriptorWire[:], packets[:],
		); err != nil {
			b.Fatal(err)
		}
		trigger <- struct{}{}
		if result := clock.WaitUntil(ctx, wake, timer, time.Now().Add(time.Hour)); result != endpointWaitWake {
			b.Fatal("unexpected endpoint clock result")
		}
	}
	b.StopTimer()
	close(trigger)
	<-senderDone
}

func TestUrbStreamRejectsMalformedAddressBeforePayloadRead(t *testing.T) {
	tests := []struct {
		name      string
		direction uint32
		endpoint  uint32
		wantError string
	}{
		{name: "direction", direction: 2, endpoint: 1, wantError: "invalid URB direction 2"},
		{name: "endpoint", direction: usbip.DirOut, endpoint: 16, wantError: "invalid URB endpoint 16"},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			dev := &altSettingTestDevice{desc: &usbdesc.Descriptor{}}
			bus := virtualbus.New(uint32(252 + index))
			defer bus.Close() //nolint:errcheck
			_, err := bus.Add(dev)
			require.NoError(t, err)

			server := New(ServerConfig{}, slog.New(slog.NewTextHandler(io.Discard, nil)), nil)
			require.NoError(t, server.AddBus(bus))
			serverConn, clientConn := net.Pipe()
			defer serverConn.Close() //nolint:errcheck
			defer clientConn.Close() //nolint:errcheck
			require.NoError(t, clientConn.SetDeadline(time.Now().Add(time.Second)))
			errCh := make(chan error, 1)
			go func() { errCh <- server.handleUrbStream(serverConn, dev) }()

			// A valid-sized header declares a maximum-sized OUT payload but sends
			// none. Address validation must return immediately; waiting for payload
			// would both allocate unnecessarily and desynchronize the next header.
			cmd := usbip.CmdSubmit{
				Basic: usbip.HeaderBasic{
					Command: usbip.CmdSubmitCode,
					Seqnum:  91,
					Dir:     test.direction,
					Ep:      test.endpoint,
				},
				TransferBufferLen: maximumTransferSize,
				NumberOfPackets:   -1,
			}
			require.NoError(t, cmd.Write(clientConn))
			select {
			case streamErr := <-errCh:
				require.Error(t, streamErr)
				require.True(t, strings.Contains(streamErr.Error(), test.wantError),
					"error %q does not contain %q", streamErr, test.wantError)
			case <-time.After(250 * time.Millisecond):
				t.Fatal("command reader waited for malformed-request payload")
			}
			require.Zero(t, dev.transferCalls)
		})
	}
}
