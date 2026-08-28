package usb

import (
	"testing"
	"time"

	usbdesc "github.com/DualSenseClient/VIIPER/usb"
	"github.com/DualSenseClient/VIIPER/usbip"
	"github.com/stretchr/testify/require"
)

func TestUSBServiceIntervalUsesHighSpeedMicroframes(t *testing.T) {
	require.Equal(t, 4*time.Millisecond, usbServiceInterval(3, 6))
	require.Equal(t, time.Millisecond, usbServiceInterval(3, 4))
	require.Equal(t, time.Millisecond, usbServiceInterval(2, 1))
}

func TestValidateIsoSubmissionRejectsAliasingReaderScratch(t *testing.T) {
	desc := &usbdesc.Descriptor{
		Device: usbdesc.DeviceDescriptor{Speed: 3},
		Interfaces: []usbdesc.InterfaceConfig{{
			Endpoints: []usbdesc.EndpointDescriptor{{
				BEndpointAddress: 0x01,
				BMAttributes:     0x09,
				WMaxPacketSize:   192,
				BInterval:        4,
			}},
		}},
	}

	require.NoError(t, validateIsoSubmission(desc, 1, usbip.DirOut, 8,
		[]usbip.IsoPacketDescriptor{{Offset: 0, Length: 4}, {Offset: 4, Length: 4}}))
	require.Error(t, validateIsoSubmission(desc, 1, usbip.DirOut, 8,
		[]usbip.IsoPacketDescriptor{{Offset: 0, Length: 6}, {Offset: 4, Length: 4}}))
	require.Error(t, validateIsoSubmission(desc, 1, usbip.DirOut, 8,
		[]usbip.IsoPacketDescriptor{{Offset: 4, Length: 8}}))
	require.Error(t, validateIsoSubmission(desc, 1, usbip.DirOut, 8,
		[]usbip.IsoPacketDescriptor{{Offset: 0, Length: 0}}))
	require.Error(t, validateIsoSubmission(desc, 1, usbip.DirOut, 193,
		[]usbip.IsoPacketDescriptor{{Offset: 0, Length: 193}}))
}

func TestEndpointServiceCapacityIncludesHighBandwidthTransactions(t *testing.T) {
	endpoint := &usbdesc.EndpointDescriptor{WMaxPacketSize: 192 | 2<<11}
	capacity, err := endpointServiceCapacity(endpoint)
	require.NoError(t, err)
	require.Equal(t, uint32(576), capacity)

	endpoint.WMaxPacketSize = 192 | 3<<11
	_, err = endpointServiceCapacity(endpoint)
	require.Error(t, err)
	endpoint.WMaxPacketSize = 0
	_, err = endpointServiceCapacity(endpoint)
	require.Error(t, err)
}

func TestInterruptSubmissionAcceptsLargeHostBufferButRejectsZero(t *testing.T) {
	desc := testCompositeDescriptor()
	require.NoError(t, validateInterruptSubmission(desc, 4, usbip.DirIn, 255),
		"host receive capacity may exceed one descriptor-sized packet")
	require.Error(t, validateInterruptSubmission(desc, 4, usbip.DirIn, 0))
}
