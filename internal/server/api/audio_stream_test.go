package api_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	viiperTesting "github.com/DualSenseClient/VIIPER/_testing"
	"github.com/DualSenseClient/VIIPER/internal/server/api"
	"github.com/DualSenseClient/VIIPER/internal/server/api/handler"
	"github.com/DualSenseClient/VIIPER/usbip"
	"github.com/DualSenseClient/VIIPER/viiperclient"
	"github.com/DualSenseClient/VIIPER/virtualbus"
)

// End to end: host writes speaker PCM via USBIP OUT EP1, the TCP audio
// feeder stream delivers the exact bytes framed.
func TestAudioStream_SpeakerPassthrough(t *testing.T) {
	s := viiperTesting.NewTestServer(t)
	defer s.ApiServer.Close() //nolint:errcheck
	defer s.UsbServer.Close() //nolint:errcheck

	r := s.ApiServer.Router()
	r.Register("bus/{id}/add", handler.BusDeviceAdd(s.UsbServer, s.ApiServer))
	r.RegisterStream("bus/{busId}/{deviceid}", api.DeviceStreamHandler(s.UsbServer))
	r.RegisterStream("bus/{busId}/{deviceid}/audio", api.DeviceAudioStreamHandler(s.UsbServer))

	require.NoError(t, s.ApiServer.Start())
	time.Sleep(50 * time.Millisecond)

	b, err := virtualbus.NewWithBusID(1)
	require.NoError(t, err)
	defer b.Close() //nolint:errcheck
	require.NoError(t, s.UsbServer.AddBus(b))
	time.Sleep(50 * time.Millisecond)

	client := viiperclient.New(s.ApiServer.Addr())
	addResp, err := client.DeviceAdd(b.BusID(), "dualsense", nil)
	require.NoError(t, err)

	audio, err := client.OpenAudioStream(context.Background(), b.BusID(), addResp.DevID)
	require.NoError(t, err)
	defer audio.Close() //nolint:errcheck
	require.NoError(t, audio.SetReadDeadline(time.Now().Add(2*time.Second)))

	usbipClient := viiperTesting.NewUsbIpClient(t, s.UsbServer.Addr())
	devs, err := usbipClient.ListDevices()
	require.NoError(t, err)
	require.Len(t, devs, 1)
	imp, err := usbipClient.AttachDevice(devs[0].BusID)
	require.NoError(t, err)
	if imp != nil && imp.Conn != nil {
		defer imp.Conn.Close() //nolint:errcheck
	}
	time.Sleep(50 * time.Millisecond)

	frame := make([]byte, 384)
	for i := range frame {
		frame[i] = byte(i * 7)
	}
	require.NoError(t, usbipClient.Submit(imp.Conn, usbip.DirOut, 1, frame, nil))

	pcm, reset, err := audio.ReadFrame()
	require.NoError(t, err)
	assert.False(t, reset)
	assert.Equal(t, frame, pcm)
}

// End to end: SET_INTERFACE on the mic interface surfaces a reset barrier
// on the TCP audio stream.
func TestAudioStream_ResetBarrier(t *testing.T) {
	s := viiperTesting.NewTestServer(t)
	defer s.ApiServer.Close() //nolint:errcheck
	defer s.UsbServer.Close() //nolint:errcheck

	r := s.ApiServer.Router()
	r.Register("bus/{id}/add", handler.BusDeviceAdd(s.UsbServer, s.ApiServer))
	r.RegisterStream("bus/{busId}/{deviceid}", api.DeviceStreamHandler(s.UsbServer))
	r.RegisterStream("bus/{busId}/{deviceid}/audio", api.DeviceAudioStreamHandler(s.UsbServer))

	require.NoError(t, s.ApiServer.Start())
	time.Sleep(50 * time.Millisecond)

	b, err := virtualbus.NewWithBusID(1)
	require.NoError(t, err)
	defer b.Close() //nolint:errcheck
	require.NoError(t, s.UsbServer.AddBus(b))
	time.Sleep(50 * time.Millisecond)

	client := viiperclient.New(s.ApiServer.Addr())
	addResp, err := client.DeviceAdd(b.BusID(), "dualsense", nil)
	require.NoError(t, err)

	audio, err := client.OpenAudioStream(context.Background(), b.BusID(), addResp.DevID)
	require.NoError(t, err)
	defer audio.Close() //nolint:errcheck
	require.NoError(t, audio.SetReadDeadline(time.Now().Add(2*time.Second)))

	usbipClient := viiperTesting.NewUsbIpClient(t, s.UsbServer.Addr())
	devs, err := usbipClient.ListDevices()
	require.NoError(t, err)
	require.Len(t, devs, 1)
	imp, err := usbipClient.AttachDevice(devs[0].BusID)
	require.NoError(t, err)
	if imp != nil && imp.Conn != nil {
		defer imp.Conn.Close() //nolint:errcheck
	}
	time.Sleep(50 * time.Millisecond)

	// SET_INTERFACE mic alt 1: bm 0x01, bRequest 0x0B, wValue 1, wIndex 2.
	setup := [8]byte{0x01, 0x0B, 0x01, 0x00, 0x02, 0x00, 0x00, 0x00}
	require.NoError(t, usbipClient.Submit(imp.Conn, usbip.DirOut, 0, nil, &setup))

	_, reset, err := audio.ReadFrame()
	require.NoError(t, err)
	assert.True(t, reset)
}

// Devices without speaker PCM fail the audio route.
func TestAudioStream_UnsupportedDevice(t *testing.T) {
	s := viiperTesting.NewTestServer(t)
	defer s.ApiServer.Close() //nolint:errcheck
	defer s.UsbServer.Close() //nolint:errcheck

	r := s.ApiServer.Router()
	r.Register("bus/{id}/add", handler.BusDeviceAdd(s.UsbServer, s.ApiServer))
	r.RegisterStream("bus/{busId}/{deviceid}", api.DeviceStreamHandler(s.UsbServer))
	r.RegisterStream("bus/{busId}/{deviceid}/audio", api.DeviceAudioStreamHandler(s.UsbServer))

	require.NoError(t, s.ApiServer.Start())
	time.Sleep(50 * time.Millisecond)

	b, err := virtualbus.NewWithBusID(1)
	require.NoError(t, err)
	defer b.Close() //nolint:errcheck
	require.NoError(t, s.UsbServer.AddBus(b))

	client := viiperclient.New(s.ApiServer.Addr())
	addResp, err := client.DeviceAdd(b.BusID(), "xbox360", nil)
	require.NoError(t, err)

	audio, err := client.OpenAudioStream(context.Background(), b.BusID(), addResp.DevID)
	require.NoError(t, err)
	defer audio.Close() //nolint:errcheck
	require.NoError(t, audio.SetReadDeadline(time.Now().Add(2*time.Second)))

	// Server closes with "not supported"; the framed read surfaces EOF.
	_, _, err = audio.ReadFrame()
	require.Error(t, err)
	assert.NotEqual(t, context.DeadlineExceeded, err)
}
