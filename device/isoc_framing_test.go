package device_test

import (
	"context"
	"testing"
	"time"

	"github.com/DualSenseClient/VIIPER/internal/server/api"
	"github.com/DualSenseClient/VIIPER/internal/server/api/handler"
	"github.com/DualSenseClient/VIIPER/usbip"
	"github.com/DualSenseClient/VIIPER/viiperclient"
	"github.com/DualSenseClient/VIIPER/virtualbus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	viiperTesting "github.com/DualSenseClient/VIIPER/_testing"

	_ "github.com/DualSenseClient/VIIPER/internal/registry" // Register devices
)

// Isochronous CMD_SUBMITs carry one 16-byte packet descriptor per packet after
// the transfer buffer. The server must consume them: leaving them unread
// desyncs the TCP stream and the next header parses as garbage (surfaced as
// "unsupported cmd 0", killing the connection). DualSense is the only VIIPER
// device with isochronous endpoints, so this exercises a full audio OUT + mic
// IN exchange followed by control + interrupt traffic on the same stream.
func TestIsocFramingKeepsStreamInSync(t *testing.T) {
	s := viiperTesting.NewTestServer(t)
	defer s.UsbServer.Close() // nolint
	defer s.ApiServer.Close() // nolint

	r := s.ApiServer.Router()
	r.Register("bus/{id}/add", handler.BusDeviceAdd(s.UsbServer, s.ApiServer))
	r.RegisterStream("bus/{busId}/{deviceid}", api.DeviceStreamHandler(s.UsbServer))

	require.NoError(t, s.ApiServer.Start())
	b, err := virtualbus.NewWithBusID(1)
	require.NoError(t, err)
	defer b.Close() // nolint
	require.NoError(t, s.UsbServer.AddBus(b))

	c := viiperclient.New(s.ApiServer.Addr())
	stream, addResp, err := c.AddDeviceAndConnect(context.Background(), b.BusID(), "dualsense", nil)
	require.NoError(t, err)
	require.NotNil(t, stream)
	require.NotNil(t, addResp)
	if stream != nil {
		defer stream.Close() //nolint:errcheck
	}

	usbipClient := viiperTesting.NewUsbIpClient(t, s.UsbServer.Addr())
	var devs []viiperTesting.Device
	require.Eventually(t, func() bool {
		list, err := usbipClient.ListDevices()
		if err != nil {
			return false
		}
		devs = list
		return len(devs) == 1
	}, 1*time.Second, 10*time.Millisecond)

	imp, err := usbipClient.AttachDevice(devs[0].BusID)
	require.NoError(t, err)
	require.NotNil(t, imp.Conn)
	defer imp.Conn.Close() // nolint

	setup := [8]byte{0x80, 0x06, 0x00, 0x01, 0x00, 0x00, 0x12, 0x00}

	// Peers may leave number_of_packets unset (0xFFFFFFFF) on non-isochronous
	// endpoints; the server must ignore the field there and stay in sync.
	require.NoError(t, usbipClient.SubmitJunkPackets(imp.Conn, usbip.DirIn, 0, nil, &setup, 0xFFFFFFFF))

	// CLEAR_FEATURE(ENDPOINT_HALT) on the speaker endpoint is host pipe-reset
	// hygiene (audio stream stop); the server ACKs it as a noop.
	clearHalt := [8]byte{0x02, 0x01, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00}
	require.NoError(t, usbipClient.Submit(imp.Conn, usbip.DirOut, 0, nil, &clearHalt))

	// Speaker OUT (EP1): payload + 1 descriptor. Must not error or desync.
	outPayload := make([]byte, 16)
	for i := range outPayload {
		outPayload[i] = byte(i)
	}
	_, outDescs, err := usbipClient.SubmitIso(imp.Conn, usbip.DirOut, 1, outPayload, []uint32{16})
	require.NoError(t, err)
	require.Len(t, outDescs, 1)
	assert.Equal(t, uint32(0), outDescs[0].Status)
	assert.Equal(t, uint32(16), outDescs[0].ActualLength)

	// Mic IN (EP2): empty queue serves one silent 192B frame + 1 descriptor.
	mic, micDescs, err := usbipClient.SubmitIso(imp.Conn, usbip.DirIn, 2, nil, []uint32{192})
	require.NoError(t, err)
	require.Len(t, mic, 192)
	require.Len(t, micDescs, 1)
	assert.Equal(t, uint32(0), micDescs[0].Status)
	assert.Equal(t, uint32(192), micDescs[0].ActualLength)

	// A multi-packet URB echoes one descriptor per packet; without frame
	// pacing only the first packet carries device data here.
	micMulti, multiDescs, err := usbipClient.SubmitIso(imp.Conn, usbip.DirIn, 2, nil, []uint32{192, 192, 192, 192})
	require.NoError(t, err)
	require.Len(t, multiDescs, 4)
	require.Len(t, micMulti, 192)
	assert.Equal(t, uint32(192), multiDescs[0].ActualLength)

	// Plain control + interrupt traffic on the same stream proves framing held.
	require.NoError(t, usbipClient.Submit(imp.Conn, usbip.DirIn, 0, nil, &setup))
	require.NoError(t, usbipClient.Submit(imp.Conn, usbip.DirIn, 4, nil, nil))
}
