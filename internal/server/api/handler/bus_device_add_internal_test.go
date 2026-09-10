package handler

import (
	"context"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "github.com/DualSenseClient/VIIPER/device/xbox360"
	th "github.com/DualSenseClient/VIIPER/internal/_testing"
	"github.com/DualSenseClient/VIIPER/internal/server/api"
	usbs "github.com/DualSenseClient/VIIPER/internal/server/usb"
	"github.com/DualSenseClient/VIIPER/usbip"
	"github.com/DualSenseClient/VIIPER/viiperclient"
	"github.com/DualSenseClient/VIIPER/virtualbus"
)

func TestBusDeviceAddReturnsNativeAutoAttachMetadata(t *testing.T) {
	const ownerSerial = "DS4W123456789AB"
	type attachCall struct {
		busID  uint32
		devID  uint32
		native bool
	}
	attachCalls := make(chan attachCall, 1)
	originalAutoAttach := attachLocalhostClientWithResult
	t.Cleanup(func() { attachLocalhostClientWithResult = originalAutoAttach })
	attachLocalhostClientWithResult = func(_ context.Context, meta *usbip.ExportMeta, _ uint16, native bool, _ bool, _ *slog.Logger) (api.AutoAttachResult, error) {
		attachCalls <- attachCall{busID: meta.BusID, devID: meta.DevID, native: native}
		return api.AutoAttachResult{
			USBIPPort:        7,
			USBIPOwnerSerial: ownerSerial,
		}, nil
	}

	addr, _, done := th.StartAPIServer(t, func(r *api.Router, s *usbs.Server, apiSrv *api.Server) {
		apiSrv.Config().AutoAttachLocalClient = true
		apiSrv.Config().AutoAttachWindowsNative = true

		bus, err := virtualbus.NewWithBusID(80251)
		require.NoError(t, err)
		require.NoError(t, s.AddBus(bus))

		r.Register("bus/{id}/add", BusDeviceAdd(s, apiSrv))
	})
	defer done()

	client := viiperclient.NewTransport(addr)
	response, err := client.Do("bus/{id}/add", `{"type":"xbox360"}`, map[string]string{"id": "80251"})
	require.NoError(t, err)
	assert.JSONEq(t, `{
		"busId": 80251,
		"devId": "1",
		"deviceSpecific": {"subType": 1},
		"vid": "0x045e",
		"pid": "0x028e",
		"type": "xbox360",
		"usbipPort": 7,
		"usbipOwnerSerial": "DS4W123456789AB"
	}`, response)

	call := <-attachCalls
	require.Equal(t, uint32(80251), call.busID)
	require.Equal(t, uint32(1), call.devID)
	require.True(t, call.native)
}
