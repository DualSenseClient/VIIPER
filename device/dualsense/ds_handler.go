package dualsense

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"log/slog"
	"net"

	"github.com/DualSenseClient/VIIPER/device"
	"github.com/DualSenseClient/VIIPER/internal/server/api"
	"github.com/DualSenseClient/VIIPER/usb"
)

func init() {
	api.RegisterDevice(DeviceTypeCombinedAudioDuplexV5, &dshandler{})
	api.RegisterDevice(DeviceTypeAudioOnlyDuplexV5, &dshandler{audioOnly: true})
	api.RegisterDevice(DeviceTypeGamepadOnlyV5, &dshandler{gamepadOnly: true})
}

type dshandler struct {
	audioOnly   bool
	gamepadOnly bool
}

func (h *dshandler) CreateDevice(o *device.CreateOptions) (usb.Device, error) {
	if o == nil {
		o = &device.CreateOptions{}
	}
	lease, err := AcquireIdentity(o, false)
	if err != nil {
		return nil, err
	}

	dse, err := new(o, false)
	if err != nil {
		lease.Release()
		return nil, err
	}
	if h.audioOnly {
		dse.descriptor = makeAudioOnlyDescriptor(false)
		dse.deviceType = DeviceTypeAudioOnlyDuplexV5
	} else if h.gamepadOnly {
		dse.descriptor = makeGamepadOnlyDescriptor(false)
		dse.deviceType = DeviceTypeGamepadOnlyV5
	}
	return dse, nil
}

func (h *dshandler) StreamHandler() api.StreamHandlerFunc {
	return dualSenseV5StreamHandler("DualSense")
}

func dualSenseV5StreamHandler(deviceName string) api.StreamHandlerFunc {
	return func(conn net.Conn, devPtr *usb.Device, logger *slog.Logger) error {
		defer releaseDualSenseIdentity(devPtr, deviceName)
		if devPtr == nil || *devPtr == nil {
			return fmt.Errorf("nil device")
		}
		dse, ok := (*devPtr).(*DualSense)
		if !ok {
			return fmt.Errorf("%w: expected %s", device.ErrWrongDeviceType, deviceName)
		}

		logger.Info(deviceName+" V5 stream configured",
			"microphoneInput", true,
			"speakerOutput", true,
			"frameVersion", StreamFrameVersionV5)

		marshalFeedback := func(feedback OutputState) ([]byte, error) {
			return feedback.MarshalV5Binary()
		}
		writer := newDualSenseOutputWriter(conn, dse.beginSpeakerStream(), logger)
		go writer.Run()
		dse.SetOutputCallback(func(feedback OutputState) {
			data, err := marshalFeedback(feedback)
			if err != nil {
				logger.Error("failed to marshal V5 feedback", "error", err)
				return
			}
			writer.EnqueueControl(StreamFrameOutputState, data)
		})
		dse.SetAtomicAudioHapticsCallback(func(feedback OutputState, speakerPCM []byte) {
			data, err := marshalFeedback(feedback)
			if err != nil {
				logger.Error("failed to marshal V5 atomic audio/haptics feedback", "error", err)
				return
			}
			writer.EnqueueAtomicAudioHaptics(data, speakerPCM)
		})
		dse.SetRealtimeHapticsCallback(func(feedback OutputState) {
			data, err := marshalFeedback(feedback)
			if err != nil {
				logger.Error("failed to marshal V5 realtime haptics feedback", "error", err)
				return
			}
			writer.EnqueueRealtimeHaptics(data)
		})
		dse.SetSpeakerResetCallback(writer.ResetSpeaker)
		defer func() {
			dse.SetOutputCallback(nil)
			dse.SetAtomicAudioHapticsCallback(nil)
			dse.SetRealtimeHapticsCallback(nil)
			dse.SetSpeakerResetCallback(nil)
			writer.Stop()
		}()

		return readDualSenseV5InputStream(conn, dse, logger)
	}
}

func releaseDualSenseIdentity(devPtr *usb.Device, deviceName string) {
	if devPtr == nil || *devPtr == nil {
		return
	}
	dse, ok := (*devPtr).(*DualSense)
	if !ok {
		slog.Warn("unexpected device type on disconnect", "expected", deviceName)
		return
	}
	dse.mtx.Lock()
	serial := dse.metaState.SerialNumber
	mac := dse.metaState.MACAddress
	dse.mtx.Unlock()
	identityMu.Lock()
	delete(serials, serial)
	delete(macs, mac)
	identityMu.Unlock()
	slog.Debug(deviceName+" disconnected, serial/mac released", "serial", serial, "mac", mac)
}

func readDualSenseV5InputStream(conn net.Conn, dse *DualSense, logger *slog.Logger) error {
	header := make([]byte, StreamFrameHeaderSize)
	input := make([]byte, InputStateSize)
	microphonePCM := make([]byte, USBMicrophoneClientFrameSize)
	var expectedSequence uint32
	sequenceInitialized := false
	for {
		if _, err := io.ReadFull(conn, header); err != nil {
			if err == io.EOF {
				logger.Info("client disconnected")
				return nil
			}
			return fmt.Errorf("read stream frame header: %w", err)
		}

		if header[0] != StreamFrameMagic0 ||
			header[1] != StreamFrameMagic1 ||
			header[2] != StreamFrameMagic2 ||
			header[3] != StreamFrameMagic3 {
			return fmt.Errorf("invalid DualSense framed stream magic %02X %02X %02X %02X",
				header[0], header[1], header[2], header[3])
		}
		if header[4] != StreamFrameVersionV5 {
			return fmt.Errorf("DualSense requires V5 stream version 0x%02X; got 0x%02X",
				StreamFrameVersionV5, header[4])
		}

		frameType := header[5]
		payloadLen := int(binary.LittleEndian.Uint16(header[6:8]))

		var payload []byte
		switch frameType {
		case StreamFrameInputState:
			if payloadLen != InputStateSize {
				return fmt.Errorf("invalid framed input state length %d", payloadLen)
			}
			payload = input
		case StreamFrameMicrophonePCM:
			if payloadLen != USBMicrophoneClientFrameSize {
				return fmt.Errorf("invalid microphone pcm frame length %d", payloadLen)
			}
			payload = microphonePCM
		default:
			return fmt.Errorf("unknown DualSense framed stream packet type 0x%02X length %d", frameType, payloadLen)
		}

		if _, err := io.ReadFull(conn, payload); err != nil {
			return fmt.Errorf("read framed packet type 0x%02X: %w", frameType, err)
		}

		sequence := binary.LittleEndian.Uint32(header[8:12])
		if sequenceInitialized && sequence != expectedSequence {
			return fmt.Errorf("DualSense V5 stream sequence mismatch: got %d expected %d", sequence, expectedSequence)
		}
		expectedSequence = sequence + 1
		sequenceInitialized = true

		receivedCRC := binary.LittleEndian.Uint32(header[12:16])
		calculatedCRC := framedStreamCRC(header[4:12], payload)
		if receivedCRC != calculatedCRC {
			return fmt.Errorf("DualSense V5 stream CRC mismatch for sequence %d: got %08X expected %08X", sequence, receivedCRC, calculatedCRC)
		}

		switch frameType {
		case StreamFrameInputState:
			corruptReason := inputStatePayloadCorruptionReason(input)
			if corruptReason != "" {
				return fmt.Errorf("invalid framed input state: %s", corruptReason)
			}
			var state InputState
			if err := state.UnmarshalBinary(input); err != nil {
				return fmt.Errorf("unmarshal framed input state: %w", err)
			}
			dse.UpdateInputState(&state)
		case StreamFrameMicrophonePCM:
			dse.QueueMicrophonePCMFrame(microphonePCM)
		}
	}
}

func framedStreamCRC(headerFields, payload []byte) uint32 {
	hash := crc32.NewIEEE()
	_, _ = hash.Write(headerFields)
	_, _ = hash.Write(payload)
	return hash.Sum32()
}

func inputStatePayloadCorruptionReason(input []byte) string {
	if len(input) < InputStateSize {
		return fmt.Sprintf("invalid length %d", len(input))
	}

	buttons := binary.LittleEndian.Uint32(input[4:8])
	dpad := input[8]
	if buttons&^validDualSenseInputButtons != 0 ||
		dpad&^validDualSenseInputDPad != 0 {
		return fmt.Sprintf("invalid controls buttons=0x%08X dpad=0x%02X", buttons, dpad)
	}

	return ""
}

func isPowerOfTwo(value int) bool {
	return value > 0 && value&(value-1) == 0
}
