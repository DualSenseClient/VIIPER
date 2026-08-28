package dualsense

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"io"
	"log/slog"
	"net"
	"strings"
	"testing"
)

func makeV5StreamFrame(frameType byte, sequence uint32, payload []byte) []byte {
	frame := make([]byte, StreamFrameHeaderSize+len(payload))
	frame[0] = StreamFrameMagic0
	frame[1] = StreamFrameMagic1
	frame[2] = StreamFrameMagic2
	frame[3] = StreamFrameMagic3
	frame[4] = StreamFrameVersionV5
	frame[5] = frameType
	binary.LittleEndian.PutUint16(frame[6:8], uint16(len(payload)))
	binary.LittleEndian.PutUint32(frame[8:12], sequence)
	copy(frame[StreamFrameHeaderSize:], payload)
	hash := crc32.NewIEEE()
	_, _ = hash.Write(frame[4:12])
	_, _ = hash.Write(payload)
	binary.LittleEndian.PutUint32(frame[12:16], hash.Sum32())
	return frame
}

func newV5ReaderTest(t *testing.T) (*DualSense, net.Conn, <-chan error) {
	return newV5ReaderTestMode(t, false)
}

func newV5ReaderTestMode(t *testing.T,
	physicalInputMetadata bool) (*DualSense, net.Conn, <-chan error) {
	t.Helper()
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	dev.SetInterfaceAltSetting(InterfaceMicrophone, 1)
	server, client := net.Pipe()
	errCh := make(chan error, 1)
	go func() {
		logger := slog.New(slog.NewTextHandler(io.Discard, nil))
		errCh <- readDualSenseV5InputStreamGeneration(server, dev, logger,
			dev.input.currentGeneration(), physicalInputMetadata)
	}()
	return dev, client, errCh
}

func TestDualSenseInputStateLegacyAndRawInputMetadataRoundTrip(t *testing.T) {
	state := *NewInputState()
	state.L2 = 255
	state.Buttons = ButtonL2
	state.PhysicalMetadataValid = true
	state.PhysicalMetadataEdgeLayout = true
	state.PhysicalSensorTimestamp = 0x44332211
	state.PhysicalInputMetadata = [InputStatePhysicalMetadataSize]byte{
		0x7B, 0x09, 0x29, 0x00, 0x00, 0x00, 0x00,
		0x20, 0x80, 0x01, 0x02, 0x03, 0x27, 0x01, 0xA5,
	}

	var raw [InputStateRawSize]byte
	if err := state.MarshalRawInputInto(raw[:]); err != nil {
		t.Fatalf("MarshalRawInputInto: %v", err)
	}
	if raw[InputStateRawFlagsOffset] != InputStatePhysicalMetadataValid|
		InputStatePhysicalMetadataEdgeLayout {
		t.Fatalf("raw-input flags=%#x", raw[InputStateRawFlagsOffset])
	}
	if got := binary.LittleEndian.Uint32(
		raw[InputStatePhysicalSensorOffset:InputStatePhysicalMetadataOffset]); got != state.PhysicalSensorTimestamp {
		t.Fatalf("physical sensor timestamp=%#x, want %#x", got,
			state.PhysicalSensorTimestamp)
	}

	var decoded InputState
	if err := decoded.UnmarshalBinary(raw[:]); err != nil {
		t.Fatalf("UnmarshalBinary raw input: %v", err)
	}
	if !decoded.PhysicalMetadataValid ||
		!decoded.PhysicalMetadataEdgeLayout ||
		decoded.PhysicalSensorTimestamp != state.PhysicalSensorTimestamp ||
		decoded.PhysicalInputMetadata != state.PhysicalInputMetadata {
		t.Fatalf("enhanced metadata changed: got=%+v want=%+v", decoded, state)
	}

	var legacy [InputStateSize]byte
	if err := state.MarshalInto(legacy[:]); err != nil {
		t.Fatalf("MarshalInto legacy: %v", err)
	}
	if err := decoded.UnmarshalBinary(legacy[:]); err != nil {
		t.Fatalf("UnmarshalBinary legacy: %v", err)
	}
	if decoded.PhysicalMetadataValid || decoded.PhysicalMetadataEdgeLayout ||
		decoded.PhysicalSensorTimestamp != 0 ||
		decoded.PhysicalInputMetadata != [InputStatePhysicalMetadataSize]byte{} {
		t.Fatalf("legacy decode retained stale enhanced metadata: %+v", decoded)
	}

	invalidFlags := raw
	invalidFlags[InputStateRawFlagsOffset] =
		InputStatePhysicalMetadataEdgeLayout
	if err := decoded.UnmarshalBinary(invalidFlags[:]); err == nil ||
		!strings.Contains(err.Error(), "flags") {
		t.Fatalf("unknown extension flag result: %v", err)
	}
	for _, size := range []int{InputStateSize - 1, InputStateSize + 1,
		InputStateRawSize - 1, InputStateRawSize + 1} {
		if err := decoded.UnmarshalBinary(make([]byte, size)); err == nil {
			t.Fatalf("accepted input state length %d", size)
		}
	}
	invalidState := state
	invalidState.PhysicalMetadataValid = false
	if err := invalidState.MarshalRawInputInto(raw[:]); err == nil ||
		!strings.Contains(err.Error(), "requires valid") {
		t.Fatalf("marshaled Edge layout without valid metadata: %v", err)
	}
}

func TestReadDualSenseV5RawInputPreservesPhysicalMetadata(t *testing.T) {
	dev, client, errCh := newV5ReaderTestMode(t, true)
	state := *NewInputState()
	state.L2 = 255
	state.Buttons = ButtonL2
	state.PhysicalMetadataValid = true
	state.PhysicalSensorTimestamp = 0x76543210
	state.PhysicalInputMetadata = [InputStatePhysicalMetadataSize]byte{
		0x39, 0x09, 0x29, 0, 0, 0, 0, 0x20,
		0x80, 0, 0, 0, 0x26, 0x01, 0xA5,
	}
	var payload [InputStateRawSize]byte
	if err := state.MarshalRawInputInto(payload[:]); err != nil {
		t.Fatalf("MarshalRawInputInto: %v", err)
	}
	if _, err := client.Write(makeV5StreamFrame(
		StreamFrameInputState, 0, payload[:])); err != nil {
		t.Fatalf("write enhanced state: %v", err)
	}
	_ = client.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("enhanced reader: %v", err)
	}

	dev.input.mu.Lock()
	got := dev.input.previous
	dev.input.mu.Unlock()
	if got.L2 != state.L2 || !got.PhysicalMetadataValid ||
		got.PhysicalSensorTimestamp != state.PhysicalSensorTimestamp ||
		got.PhysicalInputMetadata != state.PhysicalInputMetadata {
		t.Fatalf("reader changed enhanced state: got=%+v want=%+v", got, state)
	}
}

func TestReadDualSenseV5InputAliasesRequireExactPayloadSize(t *testing.T) {
	for _, test := range []struct {
		name              string
		rawInputAlias     bool
		claimedPayloadLen int
		wantExpected      int
	}{
		{
			name:              "legacy-or-events-rejects-raw-input",
			claimedPayloadLen: InputStateRawSize,
			wantExpected:      InputStateSize,
		},
		{
			name:              "raw-input-rejects-legacy",
			rawInputAlias:     true,
			claimedPayloadLen: InputStateSize,
			wantExpected:      InputStateRawSize,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, client, errCh := newV5ReaderTestMode(t, test.rawInputAlias)
			frame := makeV5StreamFrame(StreamFrameInputState, 0,
				make([]byte, test.claimedPayloadLen))
			if _, err := client.Write(frame[:StreamFrameHeaderSize]); err != nil {
				t.Fatalf("write header: %v", err)
			}
			_ = client.Close()
			err := <-errCh
			if err == nil || !strings.Contains(err.Error(),
				fmt.Sprintf("expected %d", test.wantExpected)) {
				t.Fatalf("unexpected payload length result: %v", err)
			}
		})
	}
}

func TestDualSenseInputCapabilityAliasesUseCompatibleContracts(t *testing.T) {
	for _, test := range []struct {
		name        string
		handler     dshandler
		wantType    string
		payloadSize int
	}{
		{
			name:        "existing-events-retains-legacy-input",
			handler:     dshandler{micInterfaceEvents: true},
			wantType:    DeviceTypeCombinedAudioDuplexV5Events,
			payloadSize: InputStateSize,
		},
		{
			name: "raw-input-events",
			handler: dshandler{
				micInterfaceEvents:    true,
				physicalInputMetadata: true,
			},
			wantType:    DeviceTypeCombinedAudioDuplexV5RawInputEvents,
			payloadSize: InputStateRawSize,
		},
		{
			name: "raw-input-gamepad",
			handler: dshandler{
				gamepadOnly:           true,
				physicalInputMetadata: true,
			},
			wantType:    DeviceTypeGamepadOnlyV5RawInput,
			payloadSize: InputStateRawSize,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			created, err := test.handler.CreateDevice(nil)
			if err != nil {
				t.Fatalf("CreateDevice: %v", err)
			}
			defer releaseDualSenseIdentity(&created, "DualSense")
			dev := created.(*DualSense)
			if got := dev.VIIPERDeviceType(); got != test.wantType {
				t.Fatalf("device type=%q, want %q", got, test.wantType)
			}

			server, client := net.Pipe()
			errCh := make(chan error, 1)
			go func() {
				logger := slog.New(slog.NewTextHandler(io.Discard, nil))
				errCh <- readDualSenseV5InputStreamGeneration(
					server, dev, logger, dev.input.currentGeneration(),
					test.handler.physicalInputMetadata)
			}()

			state := *NewInputState()
			state.LX = 19
			var raw [InputStateRawSize]byte
			var payload []byte
			if test.payloadSize == InputStateRawSize {
				state.PhysicalMetadataValid = true
				state.PhysicalSensorTimestamp = 0x10203040
				state.PhysicalInputMetadata[physicalMetadataR2Status] = 0x09
				state.PhysicalInputMetadata[physicalMetadataL2Status] = 0x09
				if err := state.MarshalRawInputInto(raw[:]); err != nil {
					t.Fatalf("MarshalRawInputInto: %v", err)
				}
				payload = raw[:]
			} else {
				if err := state.MarshalInto(raw[:InputStateSize]); err != nil {
					t.Fatalf("MarshalInto: %v", err)
				}
				payload = raw[:InputStateSize]
			}
			if _, err := client.Write(makeV5StreamFrame(
				StreamFrameInputState, 0, payload)); err != nil {
				t.Fatalf("write input: %v", err)
			}
			_ = client.Close()
			if err := <-errCh; err != nil {
				t.Fatalf("reader: %v", err)
			}
			dev.input.mu.Lock()
			got := dev.input.previous
			dev.input.mu.Unlock()
			if got.LX != state.LX ||
				got.PhysicalMetadataValid !=
					(test.payloadSize == InputStateRawSize) {
				t.Fatalf("decoded alias state=%+v", got)
			}
		})
	}

	for _, test := range []struct {
		name     string
		handler  dsedgehandler
		wantType string
	}{
		{
			name:     "Edge-existing-events",
			handler:  dsedgehandler{micInterfaceEvents: true},
			wantType: DeviceTypeEdgeCombinedAudioDuplexV5Events,
		},
		{
			name: "Edge-raw-input-events",
			handler: dsedgehandler{
				micInterfaceEvents:    true,
				physicalInputMetadata: true,
			},
			wantType: DeviceTypeEdgeCombinedAudioDuplexV5RawInputEvents,
		},
		{
			name: "Edge-raw-input-gamepad",
			handler: dsedgehandler{
				gamepadOnly:           true,
				physicalInputMetadata: true,
			},
			wantType: DeviceTypeEdgeGamepadOnlyV5RawInput,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			created, err := test.handler.CreateDevice(nil)
			if err != nil {
				t.Fatalf("CreateDevice: %v", err)
			}
			defer releaseDualSenseIdentity(&created, "DualSense Edge")
			if got := created.(*DualSense).VIIPERDeviceType(); got != test.wantType {
				t.Fatalf("device type=%q, want %q", got, test.wantType)
			}
		})
	}
}

func TestReadDualSenseV5RawInputRejectsInvalidMetadataFlags(t *testing.T) {
	for _, flags := range []byte{
		0x80,
		InputStatePhysicalMetadataEdgeLayout,
	} {
		t.Run(fmt.Sprintf("flags-%02x", flags), func(t *testing.T) {
			_, client, errCh := newV5ReaderTestMode(t, true)
			payload := make([]byte, InputStateRawSize)
			payload[InputStateRawFlagsOffset] = flags
			if _, err := client.Write(makeV5StreamFrame(
				StreamFrameInputState, 0, payload)); err != nil {
				t.Fatalf("write state: %v", err)
			}
			_ = client.Close()
			err := <-errCh
			if err == nil || !strings.Contains(err.Error(), "invalid flags") {
				t.Fatalf("unexpected flags result: %v", err)
			}
		})
	}
}

func TestReadDualSenseV5InputStreamAcceptsInterleavedStateAndMicrophone(t *testing.T) {
	dev, client, errCh := newV5ReaderTest(t)
	state := NewInputState()
	state.LX = 64
	state.Buttons = ButtonCross | ButtonL1
	state.GyroX = -32768
	state.GyroY = 0x4350
	state.AccelZ = -12345
	input, _ := state.MarshalBinary()

	if _, err := client.Write(makeV5StreamFrame(StreamFrameInputState, 0, input)); err != nil {
		t.Fatalf("write state: %v", err)
	}
	for sequence := uint32(1); sequence <= microphoneTargetClientFrames; sequence++ {
		pcm := make([]byte, USBMicrophoneClientFrameSize)
		for index := range pcm {
			pcm[index] = byte(sequence)
		}
		if _, err := client.Write(makeV5StreamFrame(
			StreamFrameMicrophonePCM, sequence, pcm)); err != nil {
			t.Fatalf("write microphone frame %d: %v", sequence, err)
		}
	}
	_ = client.Close()
	if err := <-errCh; err != nil {
		t.Fatalf("reader: %v", err)
	}

	dev.input.mu.Lock()
	got := dev.input.previous
	dev.input.mu.Unlock()
	dev.microphoneMu.Lock()
	queued := dev.microphoneBuffer.State().QueuedBytes
	dev.microphoneMu.Unlock()
	if got.LX != state.LX || got.Buttons != state.Buttons ||
		got.GyroX != state.GyroX || got.GyroY != state.GyroY ||
		got.AccelZ != state.AccelZ {
		t.Fatalf("V5 input changed: got=%+v want=%+v", got, state)
	}
	if queued != USBMicrophoneClientFrameSize*microphoneTargetClientFrames {
		t.Fatalf("queued microphone bytes=%d", queued)
	}
	telemetry := dev.InputTelemetryState()
	if telemetry.FramesValidated != microphoneTargetClientFrames+1 ||
		telemetry.InputFrames != 1 ||
		telemetry.LastFrameSequence != microphoneTargetClientFrames {
		t.Fatalf("V5 sequence correlation telemetry=%+v", telemetry)
	}
}

func TestReadDualSenseV5InputStreamRejectsLegacyVersion(t *testing.T) {
	_, client, errCh := newV5ReaderTest(t)
	frame := makeV5StreamFrame(StreamFrameInputState, 0,
		make([]byte, InputStateSize))
	frame[4] = 0x04
	if _, err := client.Write(frame[:StreamFrameHeaderSize]); err != nil {
		t.Fatalf("write legacy frame: %v", err)
	}
	_ = client.Close()
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "requires V5 stream") {
		t.Fatalf("unexpected legacy version result: %v", err)
	}
}

func TestReadDualSenseV5InputStreamRejectsBadCRC(t *testing.T) {
	_, client, errCh := newV5ReaderTest(t)
	frame := makeV5StreamFrame(StreamFrameInputState, 0,
		make([]byte, InputStateSize))
	frame[12] ^= 0xFF
	if _, err := client.Write(frame); err != nil {
		t.Fatalf("write bad CRC frame: %v", err)
	}
	_ = client.Close()
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "CRC mismatch") {
		t.Fatalf("unexpected CRC result: %v", err)
	}
}

func TestReadDualSenseV5InputStreamRejectsSequenceGap(t *testing.T) {
	_, client, errCh := newV5ReaderTest(t)
	payload := make([]byte, InputStateSize)
	if _, err := client.Write(makeV5StreamFrame(StreamFrameInputState, 7, payload)); err != nil {
		t.Fatalf("write first frame: %v", err)
	}
	if _, err := client.Write(makeV5StreamFrame(StreamFrameInputState, 9, payload)); err != nil {
		t.Fatalf("write skipped frame: %v", err)
	}
	_ = client.Close()
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "sequence mismatch") {
		t.Fatalf("unexpected sequence result: %v", err)
	}
}

func TestReadDualSenseV5InputStreamRejectsInvalidControlBits(t *testing.T) {
	_, client, errCh := newV5ReaderTest(t)
	payload := make([]byte, InputStateSize)
	binary.LittleEndian.PutUint32(payload[4:8], 0x80000000)
	if _, err := client.Write(makeV5StreamFrame(StreamFrameInputState, 0, payload)); err != nil {
		t.Fatalf("write corrupt frame: %v", err)
	}
	_ = client.Close()
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "invalid controls") {
		t.Fatalf("unexpected corrupt input result: %v", err)
	}
}

func TestReadDualSenseV5InputStreamRejectsUnknownFrameType(t *testing.T) {
	_, client, errCh := newV5ReaderTest(t)
	if _, err := client.Write(makeV5StreamFrame(0x7F, 0, nil)); err != nil {
		t.Fatalf("write unknown frame: %v", err)
	}
	_ = client.Close()
	err := <-errCh
	if err == nil || !strings.Contains(err.Error(), "unknown DualSense framed stream") {
		t.Fatalf("unexpected unknown-frame result: %v", err)
	}
}

func TestQueueMicrophonePCMFrameRequiresActiveInterface(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	frame := make([]byte, USBMicrophoneClientFrameSize)
	dev.QueueMicrophonePCMFrame(frame)
	if got := dev.GetDeviceSpecificArgs()["queuedMicrophoneBytes"]; got != 0 {
		t.Fatalf("inactive interface queued microphone PCM: %v", got)
	}
	dev.SetInterfaceAltSetting(InterfaceMicrophone, 1)
	dev.QueueMicrophonePCMFrame(frame)
	if got := dev.GetDeviceSpecificArgs()["queuedMicrophoneBytes"]; got != USBMicrophoneClientFrameSize {
		t.Fatalf("active interface queued bytes=%v", got)
	}
	dev.SetInterfaceAltSetting(InterfaceMicrophone, 0)
	if got := dev.GetDeviceSpecificArgs()["queuedMicrophoneBytes"]; got != 0 {
		t.Fatalf("interface close retained microphone PCM: %v", got)
	}
}

func TestDualSenseUpdateInputStateCopiesState(t *testing.T) {
	dev, err := New(nil)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	state := NewInputState()
	state.Buttons = ButtonTriangle
	dev.UpdateInputState(state)
	state.Buttons = ButtonCircle
	dev.input.mu.Lock()
	got := dev.input.previous.Buttons
	dev.input.mu.Unlock()
	if got != ButtonTriangle {
		t.Fatalf("device retained caller-owned state: got=%#x", got)
	}
}
