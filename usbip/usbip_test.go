package usbip

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"testing"
)

func TestIsoPacketDescriptorRoundTrip(t *testing.T) {
	want := IsoPacketDescriptor{
		Offset:       384,
		Length:       384,
		ActualLength: 384,
		Status:       -104,
	}

	var wire bytes.Buffer
	if err := want.Write(&wire); err != nil {
		t.Fatalf("Write returned error: %v", err)
	}
	if wire.Len() != 16 {
		t.Fatalf("unexpected descriptor length: got %d want 16", wire.Len())
	}

	var got IsoPacketDescriptor
	if err := got.Read(&wire); err != nil {
		t.Fatalf("Read returned error: %v", err)
	}
	if got != want {
		t.Fatalf("descriptor mismatch: got %#v want %#v", got, want)
	}
	var truncated IsoPacketDescriptor
	if err := truncated.Read(bytes.NewReader(make([]byte, IsoPacketDescriptorSize-1))); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("truncated Read error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestIsoPacketDescriptorDecode(t *testing.T) {
	var wire [IsoPacketDescriptorSize]byte
	binary.BigEndian.PutUint32(wire[0:4], 0x01020304)
	binary.BigEndian.PutUint32(wire[4:8], 0x11121314)
	binary.BigEndian.PutUint32(wire[8:12], 0x21222324)
	binary.BigEndian.PutUint32(wire[12:16], 0xffffff98)

	var descriptor IsoPacketDescriptor
	if err := descriptor.Decode(wire[:]); err != nil {
		t.Fatalf("Decode returned error: %v", err)
	}
	want := IsoPacketDescriptor{
		Offset:       0x01020304,
		Length:       0x11121314,
		ActualLength: 0x21222324,
		Status:       -104,
	}
	if descriptor != want {
		t.Fatalf("descriptor mismatch: got %#v want %#v", descriptor, want)
	}
	if err := descriptor.Decode(wire[:IsoPacketDescriptorSize-1]); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("short Decode error = %v, want io.ErrUnexpectedEOF", err)
	}

	allocations := testing.AllocsPerRun(1000, func() {
		if err := descriptor.Decode(wire[:]); err != nil {
			panic(err)
		}
	})
	if allocations != 0 {
		t.Fatalf("Decode allocations = %v, want 0", allocations)
	}
}
