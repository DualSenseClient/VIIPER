package dualsense

import (
	"bytes"
	"testing"

	"github.com/DualSenseClient/VIIPER/usb"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func reportBytes(t *testing.T, edge bool) []byte {
	t.Helper()
	d, err := new(nil, edge)
	require.NoError(t, err)
	desc := d.GetDescriptor()
	require.NotNil(t, desc)
	require.Len(t, desc.Interfaces, 1)
	require.NotNil(t, desc.Interfaces[0].HID)
	rb, err := desc.Interfaces[0].HID.ReportBytes()
	require.NoError(t, err)
	return []byte(rb)
}

// Report descriptors must match DS5Dongle reference lengths:
// DS 321B, Edge 437B.
func TestReportDescriptorLengths(t *testing.T) {
	assert.Len(t, reportBytes(t, false), 321)
	assert.Len(t, reportBytes(t, true), 437)
}

// Report 0x02 output count: DS 47 (0x2F), Edge 63 (0x3F).
func TestReportDescriptorOutputCount(t *testing.T) {
	ds := reportBytes(t, false)
	assert.True(t, bytes.Contains(ds, []byte{0x85, 0x02, 0x09, 0x23, 0x95, 0x2F}))
	dse := reportBytes(t, true)
	assert.True(t, bytes.Contains(dse, []byte{0x85, 0x02, 0x09, 0x23, 0x95, 0x3F}))
}

// Report 0xF2 feature count: DS 15 (0x0F), Edge 52 (0x34).
func TestReportDescriptorF2Count(t *testing.T) {
	ds := reportBytes(t, false)
	assert.True(t, bytes.Contains(ds, []byte{0x85, 0xF2, 0x09, 0x32, 0x95, 0x0F}))
	dse := reportBytes(t, true)
	assert.True(t, bytes.Contains(dse, []byte{0x85, 0xF2, 0x09, 0x32, 0x95, 0x34}))
}

// Edge-only reports must exist on Edge and be absent on DS.
func TestReportDescriptorEdgeOnly(t *testing.T) {
	ds := reportBytes(t, false)
	dse := reportBytes(t, true)
	for _, id := range []byte{0x60, 0x61, 0x68, 0x70, 0x7B} {
		assert.True(t, bytes.Contains(dse, []byte{0x85, id}), "edge missing 0x%02X", id)
		assert.False(t, bytes.Contains(ds, []byte{0x85, id}), "ds unexpectedly has 0x%02X", id)
	}
	// DS-only tail must exist on both (Edge reuses 0xF6-0xF9).
	for _, id := range []byte{0xF6, 0xF9} {
		assert.True(t, bytes.Contains(ds, []byte{0x85, id}))
		assert.True(t, bytes.Contains(dse, []byte{0x85, id}))
	}
}

func TestDeviceDescriptorFields(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)
	dev := d.GetDescriptor().Device
	assert.Equal(t, uint16(DefaultVID), dev.IDVendor)
	assert.Equal(t, uint16(DefaultPIDDS), dev.IDProduct)
	assert.Equal(t, uint8(0x03), dev.ISerialNumber)
	assert.Equal(t, uint16(0x0100), dev.BcdDevice)

	de, err := new(nil, true)
	require.NoError(t, err)
	assert.Equal(t, uint16(DefaultPIDDSEdge), de.GetDescriptor().Device.IDProduct)

	cfg := d.GetDescriptor().Configuration
	assert.Equal(t, uint8(0xC0), cfg.BMAttributes)
	assert.Equal(t, uint8(0xFA), cfg.BMaxPower)
}

func TestHIDEndpointsInterval1(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)
	eps := d.GetDescriptor().Interfaces[0].Endpoints
	require.Len(t, eps, 2)
	for _, ep := range eps {
		assert.Equal(t, uint8(1), ep.BInterval)
	}
}

func TestSerialStringPerDevice(t *testing.T) {
	d, err := new(nil, false)
	require.NoError(t, err)
	s := d.GetDescriptor().Strings
	assert.Equal(t, "Sony Interactive Entertainment", s[1])
	assert.Equal(t, "DualSense Wireless Controller", s[2])
	assert.Equal(t, DefaultSerialNumberDS, s[3])

	// Index 0 must encode to the en-US LANGID descriptor (bLength 4,
	// type STRING, wLangID 0x0409). A lookalike rune here breaks string
	// fetching on the wire.
	assert.Equal(t, []byte{4, 3, 0x09, 0x04}, usb.EncodeStringDescriptor(s[0]))

	de, err := new(nil, true)
	require.NoError(t, err)
	se := de.GetDescriptor().Strings
	assert.Equal(t, "DualSense Edge Wireless Controller", se[2])
	assert.Equal(t, DefaultSerialNumberDSEdge, se[3])

	// Per-device maps must not alias: creating an Edge must not rewrite DS strings.
	assert.Equal(t, "DualSense Wireless Controller", d.GetDescriptor().Strings[2])
}
