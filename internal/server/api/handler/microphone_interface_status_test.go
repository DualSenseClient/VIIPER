package handler_test

import (
	"encoding/json"
	"testing"

	"github.com/Alia5/VIIPER/device/dualsense"
	handlerTest "github.com/Alia5/VIIPER/internal/_testing"
	"github.com/Alia5/VIIPER/internal/server/api"
	"github.com/Alia5/VIIPER/internal/server/api/handler"
	"github.com/Alia5/VIIPER/internal/server/usb"
	"github.com/Alia5/VIIPER/viiperclient"
	"github.com/Alia5/VIIPER/virtualbus"
)

func TestBusDeviceMicrophoneInterfaceStatusUsesNarrowDeviceLookup(t *testing.T) {
	const busID = 60321
	var controller *dualsense.DualSense
	addr, _, done := handlerTest.StartAPIServer(t,
		func(r *api.Router, server *usb.Server, _ *api.Server) {
			bus, err := virtualbus.NewWithBusID(busID)
			if err != nil {
				t.Fatalf("create bus: %v", err)
			}
			if err := server.AddBus(bus); err != nil {
				t.Fatalf("add bus: %v", err)
			}
			controller, err = dualsense.New(nil)
			if err != nil {
				t.Fatalf("create DualSense: %v", err)
			}
			if _, err := bus.Add(controller); err != nil {
				t.Fatalf("add DualSense: %v", err)
			}
			r.Register("bus/{busId}/{devId}/microphone-interface",
				handler.BusDeviceMicrophoneInterfaceStatus(server))
		})
	defer done()

	client := viiperclient.NewTransport(addr)
	readStatus := func() map[string]any {
		t.Helper()
		line, err := client.Do("bus/{busId}/{devId}/microphone-interface", nil,
			map[string]string{"busId": "60321", "devId": "1"})
		if err != nil {
			t.Fatalf("status request: %v", err)
		}
		var status map[string]any
		if err := json.Unmarshal([]byte(line), &status); err != nil {
			t.Fatalf("decode status: %v", err)
		}
		return status
	}

	if active, ok := readStatus()["active"].(bool); !ok || active {
		t.Fatalf("initial microphone status was not inactive")
	}
	controller.SetInterfaceAltSetting(dualsense.InterfaceMicrophone, 1)
	status := readStatus()
	if active, ok := status["active"].(bool); !ok || !active {
		t.Fatalf("active alternate setting was not visible: %#v", status)
	}
	if _, broadMetadataLeak := status["serialNumber"]; broadMetadataLeak {
		t.Fatalf("narrow endpoint returned broad metadata: %#v", status)
	}
}
