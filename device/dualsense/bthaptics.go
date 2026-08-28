package dualsense

import (
	"encoding/binary"
	"errors"
	"hash/crc32"
	"io"
)

const (
	BluetoothHapticsSampleSize = 64
	BluetoothHapticsSampleRate = 3000

	// BluetoothCombinedHapticsReportID carries the current output state and
	// haptics sample in one HID transaction. V5 uses only this atomic carrier.
	BluetoothCombinedHapticsReportID   = 0x36
	BluetoothCombinedHapticsReportSize = 398
	BluetoothCombinedStateSize         = 63
	BluetoothCombinedHapticsOffset     = 78
	BluetoothCombinedSpeakerOffset     = 142
	// CombinedReportReference exposes the five packet-0x11 buffer fields as a 16-127
	// setting. Its default is 64, which retains a noticeably delayed haptics
	// queue when the virtual USB stream is already paced in realtime. Keep the
	// stream clock unchanged, but request the smallest documented queue from
	// the physical controller.
	BluetoothCombinedLowLatencyBufferLength = 16

	USBHapticsAudioSampleRate     = 48000
	USBHapticsAudioChannels       = 4
	USBHapticsAudioBytesPerSample = 2
	USBHapticsAudioFrameSize      = USBHapticsAudioChannels * USBHapticsAudioBytesPerSample
	USBHapticsAudioPacketFrames   = USBHapticsAudioSampleRate / 1000
	USBHapticsAudioPacketSize     = USBHapticsAudioPacketFrames * USBHapticsAudioFrameSize
	// The captured hardware descriptor advertises 392 bytes even though a
	// nominal 1 ms 48 kHz, four-channel S16 packet carries 384 bytes.
	USBHapticsAudioMaxPacketSize = 392
	USBHapticsAudioDownsample    = USBHapticsAudioSampleRate / BluetoothHapticsSampleRate

	BluetoothOutputReportID   = 0x31
	BluetoothOutputReportSize = 78

	bluetoothHapticsCRCSeed = 0xEADA2D49
)

var ErrInvalidBluetoothHapticsSample = errors.New("dualsense bluetooth haptics sample must be exactly 64 bytes")
var ErrInvalidUSBOutputReport = errors.New("dualsense USB output report must be report 0x02 with at least 48 bytes")

// defaultBluetoothCombinedState is the vDS default DualSense output state.
// The remaining 16 bytes of the 63-byte state are intentionally zero. A game
// output report replaces the first 47 bytes when one is available.
var defaultBluetoothCombinedState = [BluetoothCombinedStateSize]byte{
	0xfd, 0xf7, 0x00, 0x00, 0x7f, 0x64, 0xff, 0x09, 0x00, 0x0f, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	0x00, 0x0a, 0x07, 0x00, 0x00, 0x02, 0x01, 0x00, 0xff, 0xd7, 0x00,
}

// BuildBluetoothCombinedHapticsReport builds the V5 combined carrier.
//
// Real DualSense Bluetooth traffic combines state, haptics, and speaker data
// into this report. VIIPER owns the virtual USB haptics stream, while
// DS4Windows injects its already-encoded 200-byte Opus frame before forwarding
// the report to a physical controller. Leaving the speaker block empty here is
// intentional: fabricated Opus padding can make the controller reject the
// entire haptics packet.
func BuildBluetoothCombinedHapticsReport(sequence uint8, packetSequence uint8, sample []byte, rawOutputReport []byte) ([]byte, error) {
	report := make([]byte, BluetoothCombinedHapticsReportSize)
	if err := BuildBluetoothCombinedHapticsReportInto(
		sequence, packetSequence, sample, rawOutputReport, report,
	); err != nil {
		return nil, err
	}
	return report, nil
}

// BuildBluetoothCombinedHapticsReportInto is the allocation-free media path.
// The destination remains owned by the caller and may be a field in the
// complete OutputState snapshot queued to the framed writer.
func BuildBluetoothCombinedHapticsReportInto(sequence uint8,
	packetSequence uint8, sample []byte, rawOutputReport []byte,
	destination []byte) error {
	if len(sample) != BluetoothHapticsSampleSize {
		return ErrInvalidBluetoothHapticsSample
	}
	if len(destination) < BluetoothCombinedHapticsReportSize {
		return io.ErrShortBuffer
	}
	report := destination[:BluetoothCombinedHapticsReportSize]
	clear(report)
	report[0] = BluetoothCombinedHapticsReportID
	report[1] = (sequence & 0x0F) << 4

	// Packet 0x11 starts the Bluetooth audio/haptics stream. This is the same
	// header and 64-byte interval contract used by vDS.
	report[2] = 0x91
	report[3] = 0x07
	report[4] = 0xFE
	report[5] = BluetoothCombinedLowLatencyBufferLength
	report[6] = BluetoothCombinedLowLatencyBufferLength
	report[7] = BluetoothCombinedLowLatencyBufferLength
	report[8] = BluetoothCombinedLowLatencyBufferLength
	report[9] = BluetoothCombinedLowLatencyBufferLength
	report[10] = packetSequence

	// Packet 0x10 is the 63-byte DualSense output state. Start from vDS's
	// known-good state, then retain the game's native USB effect payload.
	state := defaultBluetoothCombinedState
	if len(rawOutputReport) >= OutputReportSize && rawOutputReport[0] == ReportIDOutput {
		copy(state[:OutputReportSize-1], rawOutputReport[1:OutputReportSize])
	}
	report[11] = 0x90
	report[12] = BluetoothCombinedStateSize
	copy(report[13:13+BluetoothCombinedStateSize], state[:])

	// Packet 0x12 is the 64-byte signed 8-bit stereo haptics payload.
	report[76] = 0x92
	report[77] = BluetoothHapticsSampleSize
	copy(report[BluetoothCombinedHapticsOffset:BluetoothCombinedHapticsOffset+BluetoothHapticsSampleSize], sample)

	// Packet 0x13 is the optional 200-byte Opus speaker lane. It is explicitly
	// empty here; zero-filled bytes masquerading as Opus cause the controller to
	// reject the whole packet on some firmware revisions.
	report[BluetoothCombinedSpeakerOffset] = 0x93
	report[BluetoothCombinedSpeakerOffset+1] = 0

	binary.LittleEndian.PutUint32(report[BluetoothCombinedHapticsReportSize-4:], dualSenseBluetoothCRC32(report[:BluetoothCombinedHapticsReportSize-4]))
	return nil
}

// BuildBluetoothOutputReportFromUSBOutput maps a native USB DualSense output
// report 0x02 into the Bluetooth report 0x31 shape used by Sony HID-over-BT.
//
// HIDMaestro's DualSense profiles describe this as USB bytes 1-47
// ("effectPayload") shifted to Bluetooth bytes 3-49, with byte 1 carrying the
// rolling BT tag, byte 2 carrying the BT flag 0x10, and bytes 74-77 carrying a
// Sony CRC32 over prefix [0xA2, 0x31] plus bytes 1-73.
func BuildBluetoothOutputReportFromUSBOutput(sequence uint8, usbReport []byte) ([]byte, error) {
	report := make([]byte, BluetoothOutputReportSize)
	if err := BuildBluetoothOutputReportFromUSBOutputInto(sequence, usbReport,
		report); err != nil {
		return nil, err
	}
	return report, nil
}

// BuildBluetoothOutputReportFromUSBOutputInto maps into caller-owned storage
// and computes the Sony CRC incrementally without assembling a temporary
// prefix buffer.
func BuildBluetoothOutputReportFromUSBOutputInto(sequence uint8,
	usbReport []byte, destination []byte) error {
	if len(usbReport) < OutputReportSize || usbReport[0] != ReportIDOutput {
		return ErrInvalidUSBOutputReport
	}
	if len(destination) < BluetoothOutputReportSize {
		return io.ErrShortBuffer
	}
	report := destination[:BluetoothOutputReportSize]
	clear(report)
	report[0] = BluetoothOutputReportID
	report[1] = (sequence & 0x0F) << 4
	report[2] = 0x10
	copy(report[3:50], usbReport[1:OutputReportSize])

	prefix := [...]byte{0xA2, BluetoothOutputReportID}
	crc := dualSenseBluetoothCRC32Update(bluetoothHapticsCRCSeed, prefix[:])
	crc = dualSenseBluetoothCRC32Update(crc, report[1:74])
	binary.LittleEndian.PutUint32(report[74:78], crc)
	return nil
}

func dualSenseBluetoothCRC32(data []byte) uint32 {
	return dualSenseBluetoothCRC32Update(bluetoothHapticsCRCSeed, data)
}

// dualSenseBluetoothCRC32Update is the incremental IEEE update used by Sony's
// seeded Bluetooth CRC. It is table-driven (one lookup per byte) and keeps
// caller-owned report arrays on the stack or in their preallocated slot; the
// standard library's architecture dispatch currently makes its byte slice
// escape under the supported Windows Go toolchain.
func dualSenseBluetoothCRC32Update(crc uint32, data []byte) uint32 {
	crc = ^crc
	for _, value := range data {
		crc = crc32.IEEETable[byte(crc)^value] ^ (crc >> 8)
	}
	return ^crc
}
