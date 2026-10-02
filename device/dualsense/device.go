package dualsense

import (
	"context"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math"
	"net"
	"sync"
	"time"

	"github.com/DualSenseClient/VIIPER/device"
	"github.com/DualSenseClient/VIIPER/usb"
	"github.com/DualSenseClient/VIIPER/usbip"
)

type DualSense struct {
	inputCh    chan *InputState
	inputState *InputState
	metaState  *MetaState

	outputFunc func(OutputState)
	descriptor usb.Descriptor

	subcommand [2]byte

	seqCounter    uint8
	timestampBase time.Time
	edge          bool

	// Per-finger touch tracking IDs. Bumped on rising edge (not-touched →
	// touched) and mirrored into the contact byte, like real hardware.
	// Guarded by mtx.
	touchTracking [2]uint8
	lastTouchHeld [2]bool

	mtx sync.Mutex
}

func New(o *device.CreateOptions) (*DualSense, error) {
	return new(o, false)
}
func NewEdge(o *device.CreateOptions) (*DualSense, error) {
	return new(o, true)
}

func new(o *device.CreateOptions, edge bool) (*DualSense, error) {
	metaState := &MetaState{
		SerialNumber:       DefaultSerialNumberDS,
		MACAddress:         DefaultMACAddressDS,
		Board:              DefaultBoardStringDS,
		BuildTime:          DefaultBuildTime,
		BatteryStatus:      DefaultBatteryStatus,
		TemperatureCelsius: DefaultTemperature,
		BatteryVoltage:     DefaultVoltage,
		ShellColor:         DefaultShellColor,
	}
	if edge {
		metaState.SerialNumber = DefaultSerialNumberDSEdge
		metaState.MACAddress = DefaultMACAddressDSEdge
		metaState.Board = DefaultBoardStringEdge
	}
	if o != nil && o.DeviceSpecific != "" {
		var newMeta MetaState
		err := json.Unmarshal([]byte(o.DeviceSpecific), &newMeta)
		if err != nil {
			return nil, fmt.Errorf("invalid JSON payload: %w", err)
		}
		if newMeta.SerialNumber != "" {
			metaState.SerialNumber = newMeta.SerialNumber
		}
		if newMeta.MACAddress != "" {
			metaState.MACAddress = newMeta.MACAddress
		}
		if newMeta.Board != "" {
			metaState.Board = newMeta.Board
		}
		if !newMeta.BuildTime.IsZero() {
			metaState.BuildTime = newMeta.BuildTime
		}
		if newMeta.BatteryStatus != 0 {
			metaState.BatteryStatus = newMeta.BatteryStatus
		}
		if newMeta.TemperatureCelsius != 0 {
			metaState.TemperatureCelsius = newMeta.TemperatureCelsius
		}
		if newMeta.BatteryVoltage != 0 {
			metaState.BatteryVoltage = newMeta.BatteryVoltage
		}
		metaState.ShellColor = newMeta.ShellColor
	}

	devDesc := baseDeviceDescriptor()
	devDesc.IDProduct = DefaultPIDDS
	product := "DualSense Wireless Controller"
	ifaces := []usb.InterfaceConfig{dsInterface}
	if edge {
		devDesc.IDProduct = DefaultPIDDSEdge
		product = "DualSense Edge Wireless Controller"
		ifaces = []usb.InterfaceConfig{dseInterface}
	}

	if o != nil {
		if o.IDVendor != nil {
			devDesc.IDVendor = *o.IDVendor
		}
		if o.IDProduct != nil {
			devDesc.IDProduct = *o.IDProduct
		}
	}

	// Strings are per-device: the map must be cloned so one device never
	// mutates the shared template, and index 3 carries this unit's serial.
	strings := map[uint8]string{
		0: "\u0409", // LangID: en-US (0x0409)
		1: "Sony Interactive Entertainment",
		2: product,
		3: metaState.SerialNumber,
	}

	d := &DualSense{
		descriptor: usb.Descriptor{
			Device:        devDesc,
			Configuration: baseConfiguration(),
			Interfaces:    ifaces,
			Strings:       strings,
		},
		metaState: metaState,
		edge:      edge,
	}

	slog.Info("DualSense device instantiated",
		"edge", edge,
		"vid", d.descriptor.Device.IDVendor,
		"pid", d.descriptor.Device.IDProduct,
		"interfaces", len(d.descriptor.Interfaces))

	d.inputState = NewInputState()
	d.inputCh = make(chan *InputState, 1)
	d.inputCh <- d.inputState
	d.timestampBase = time.Now()

	return d, nil
}

func (d *DualSense) SetMetaState(meta MetaState) {
	d.mtx.Lock()
	defer d.mtx.Unlock()
	d.metaState = &meta
}

func (d *DualSense) SetOutputCallback(f func(OutputState)) {
	d.outputFunc = f
}

func (d *DualSense) UpdateInputState(state *InputState) {
	d.mtx.Lock()
	if state.Touch1Active && !d.lastTouchHeld[0] {
		d.touchTracking[0]++
	}
	if state.Touch2Active && !d.lastTouchHeld[1] {
		d.touchTracking[1]++
	}
	d.lastTouchHeld[0] = state.Touch1Active
	d.lastTouchHeld[1] = state.Touch2Active
	d.inputState = state
	d.mtx.Unlock()
	select {
	case <-d.inputCh:
	default:
	}
	d.inputCh <- state
}

func (d *DualSense) GetDescriptor() *usb.Descriptor {
	return &d.descriptor
}

func (d *DualSense) GetDeviceSpecificArgs() map[string]any {
	var res map[string]any
	d.mtx.Lock()
	defer d.mtx.Unlock()

	bytes, err := json.Marshal(d.metaState)
	if err != nil {
		return map[string]any{}
	}
	err = json.Unmarshal(bytes, &res)
	if err != nil {
		return map[string]any{}
	}
	return res
}

func (d *DualSense) HandleTransfer(ctx context.Context, ep uint32, dir uint32, out []byte) []byte {
	if dir == usbip.DirIn {
		switch ep {
		case 4:
			select {
			case <-ctx.Done():
				if errors.Is(ctx.Err(), context.DeadlineExceeded) {
					d.mtx.Lock()
					is := d.inputState
					ms := *d.metaState
					d.mtx.Unlock()
					return d.buildUSBInputReport(is, &ms)
				}
				return nil
			case is := <-d.inputCh:
				d.mtx.Lock()
				ms := *d.metaState
				d.mtx.Unlock()
				return d.buildUSBInputReport(is, &ms)
			}
		default:
			return nil
		}
	}

	if dir == usbip.DirOut && ep == 3 {
		if len(out) >= 48 && out[0] == ReportIDOutput {
			if d.outputFunc != nil {
				d.outputFunc(parseOutputReport(out))
			}
		}
	}

	return nil
}

func (d *DualSense) HandleControl(bmRequestType, bRequest uint8, wValue, wIndex, wLength uint16, data []byte) ([]byte, bool) {
	reportType := uint8(wValue >> 8)
	reportID := uint8(wValue & 0xFF)

	switch bmRequestType {
	case hidClassIN:
		switch bRequest {
		case hidGetReport:
			if reportType == reportTypeInput && reportID == ReportIDInput {
				d.mtx.Lock()
				is := *d.inputState
				ms := *d.metaState
				d.mtx.Unlock()
				b := d.buildUSBInputReport(&is, &ms)
				if wLength > 0 && int(wLength) < len(b) {
					b = b[:wLength]
				}
				return b, true
			}
			if reportType == reportTypeFeature {
				if b := d.getFeatureReport(reportID); b != nil {
					if wLength > 0 && int(wLength) < len(b) {
						b = b[:wLength]
					}
					return b, true
				}
			}
		case hidGetIdle:
			return []byte{0x00}, true
		case hidGetProtocol:
			return []byte{0x01}, true
		}
	case hidClassOUT:
		if bRequest == hidSetReport {
			switch {
			case reportType == reportTypeFeature && reportID == featureIDCommand && len(data) >= 3:
				d.subcommand[0] = data[1]
				d.subcommand[1] = data[2]
				return nil, true
			case reportType == reportTypeFeature:
				return nil, true
			case reportType == reportTypeOutput && reportID == ReportIDOutput && len(data) >= 48:
				if d.outputFunc != nil {
					d.outputFunc(parseOutputReport(data))
				}
				return nil, true
			}
		}
	}

	slog.Warn("DualSense control request unhandled",
		"bmRequestType", bmRequestType,
		"bRequest", bRequest,
		"reportType", reportType,
		"reportID", reportID,
		"wIndex", wIndex,
		"wLength", wLength,
		"dataLen", len(data))

	return nil, false
}

var featureGetHandlers = map[byte]func(*DualSense) []byte{
	featureIDCalibration:     (*DualSense).featureReportCalibration,
	featureIDPairing:         (*DualSense).featureReportPairing,
	featureIDFirmware:        (*DualSense).featureReportFirmware,
	featureIDCommandResponse: (*DualSense).featureReportCommandResponse,
}

// dsFeatureLengths maps every DS feature report ID to its total GET length
// (report ID + payload), derived from the HID report descriptor counts.
var dsFeatureLengths = map[byte]int{
	0x05: 41, 0x08: 48, 0x09: 20, 0x0A: 27, 0x0B: 42, 0x0C: 42,
	0x20: 64, 0x21: 5, 0x22: 64,
	0x80: 64, 0x81: 64, 0x82: 10, 0x83: 64, 0x84: 64, 0x85: 3,
	0xA0: 2, 0xE0: 64,
	0xF0: 64, 0xF1: 64, 0xF2: 16, 0xF4: 64, 0xF5: 4,
	0xF6: 64, 0xF7: 64, 0xF8: 64, 0xF9: 64,
}

// edgeFeatureLengths overrides/additions for the Edge descriptor:
// 0xF2 is wider and the profile block is Edge-only.
var edgeFeatureLengths = map[byte]int{
	0xF2: 53,
	0x60: 64, 0x61: 64, 0x62: 64, 0x63: 64, 0x64: 64, 0x65: 64,
	0x68: 64,
	0x70: 64, 0x71: 64, 0x72: 64, 0x73: 64, 0x74: 64, 0x75: 64,
	0x76: 64, 0x77: 64, 0x78: 64, 0x79: 64, 0x7A: 64, 0x7B: 64,
}

// getFeatureReport serves a GET_REPORT for any descriptor-listed feature ID.
// IDs with real content use their handler; the rest return a zero stub of
// exact length with the report ID set. Edge profile reports (0x60-0x7B)
// are static stubs — without a backing physical controller there is no
// unlock handshake or NAK-until-ready sequence to perform.
func (d *DualSense) getFeatureReport(id byte) []byte {
	if fn, ok := featureGetHandlers[id]; ok {
		return fn(d)
	}
	n, ok := dsFeatureLengths[id]
	if d.edge {
		if en, found := edgeFeatureLengths[id]; found {
			n, ok = en, true
		}
	}
	if !ok {
		return nil
	}
	b := make([]byte, n)
	b[0] = id
	return b
}

// parseOutputReport decodes the 48B USB output report 0x02 (report ID +
// 47B payload, matching DS5Dongle SetStateData) into feeder semantics.
func parseOutputReport(out []byte) OutputState {
	var feedback OutputState
	if len(out) < 48 {
		return feedback
	}
	feedback.Flags0 = out[1]
	feedback.Flags1 = out[2]
	feedback.RumbleSmall = out[3]
	feedback.RumbleLarge = out[4]
	feedback.VolumeHeadphones = out[5]
	feedback.VolumeSpeaker = out[6]
	feedback.VolumeMic = out[7]
	feedback.AudioControl = out[8]
	feedback.MuteLightMode = out[9]
	feedback.MuteControl = out[10]
	copy(feedback.TriggerRight[:], out[11:22])
	copy(feedback.TriggerLeft[:], out[22:33])
	feedback.HostTimestamp = binary.LittleEndian.Uint32(out[33:37])
	feedback.MotorPower = out[37]
	feedback.AudioControl2 = out[38]
	feedback.Flags3 = out[39]
	feedback.HapticFilter = out[40]
	feedback.UnkByte = out[41]
	feedback.LightFade = out[42]
	feedback.LightBrightness = out[43]
	feedback.PlayerLeds = out[44]
	feedback.LedRed = out[45]
	feedback.LedGreen = out[46]
	feedback.LedBlue = out[47]
	return feedback
}

func (d *DualSense) featureReportCalibration() []byte {
	report := make([]byte, 41)
	report[0] = featureIDCalibration

	for i, v := range [17]int16{
		0, 0, 0,
		8192, -8192, 8192, -8192, 8192, -8192,
		500, 500,
		8192, -8192, 8192, -8192, 8192, -8192,
	} {
		binary.LittleEndian.PutUint16(report[1+i*2:], uint16(v))
	}

	report[35] = 0x0B // TODO:
	return report
}

func (d *DualSense) featureReportPairing() []byte {
	report := make([]byte, 20)
	report[0] = featureIDPairing

	d.mtx.Lock()
	mac := d.metaState.MACAddress
	d.mtx.Unlock()

	if hw, err := net.ParseMAC(mac); err == nil && len(hw) == 6 {
		for i := range 6 {
			report[1+i] = hw[5-i]
		}
	}

	// TODO:
	report[7] = 0x08
	report[8] = 0x25
	report[10] = 0x1E
	report[12] = 0xEE
	report[13] = 0x74
	report[14] = 0xD0
	report[15] = 0xBC
	return report
}

func (d *DualSense) featureReportFirmware() []byte {
	report := make([]byte, 64)
	report[0] = featureIDFirmware

	d.mtx.Lock()
	bt := d.metaState.BuildTime
	d.mtx.Unlock()

	copy(report[1:12], bt.Format("Jan 02 2006"))
	copy(report[12:20], bt.Format("15:04:05"))

	report[20] = HardwareType
	report[21] = 0x01 // TODO: unknown
	report[22] = 0x44 // TODO: put in CONST!!! // build revision from real device

	binary.LittleEndian.PutUint32(report[24:28], HwInfo)

	// TODO: unknown
	report[28] = 0x36
	report[31] = 0x01
	report[32] = 0xC1
	report[33] = 0xC8

	binary.LittleEndian.PutUint16(report[44:46], FirmwareVersion)

	// TODO: unknown
	report[48] = 0x14
	report[52] = 0x0B
	report[54] = 0x01
	report[56] = 0x06
	return report
}

func (d *DualSense) featureReportCommandResponse() []byte {
	report := make([]byte, 64)
	report[0] = featureIDCommandResponse

	d.mtx.Lock()
	sub := d.subcommand
	serial := d.metaState.SerialNumber
	voltage := d.metaState.BatteryVoltage
	temp := d.metaState.TemperatureCelsius
	d.mtx.Unlock()

	switch sub[0] {
	case subcmdSerial:
		copy(report[3:21], serial)
	case subcmdStatus:
		// nvs locked
		report[1] = 0x01
		report[4] = 0x01
	case subcmdSensors:
		vRaw := uint16(math.Round(voltage * 1000))
		report[4] = byte(vRaw)
		report[5] = byte(vRaw >> 8)
		tRaw := uint16(math.Max(0, math.Min(4095, math.Round((2470.0-temp*26.0)/0.78125))))
		report[6] = byte(tRaw)
		report[7] = byte(tRaw >> 8)
	default:
		slog.Warn("DualSense: unknown sub-command for featureIDCommandResponse",
			"sub0", sub[0], "sub1", sub[1])
		report[1] = 0x01
	}
	return report
}

// buildUSBInputReport encodes the 64B USB input report 0x01.
//
// Populated from feeder state: sticks, triggers, seq, buttons, gyro/accel,
// sensor timestamp, touch contacts, battery status.
//
// Deliberately left zero (need a real-hardware capture to fill correctly,
// not sample constants): b[11:16] touch timestamps, b[32], b[41:48] trigger
// effect state echo, b[49] reserved marker (kept at the historical 0x10),
// b[50:52], b[54:63] audio/headset flags and reserved tail.
func (d *DualSense) buildUSBInputReport(s *InputState, m *MetaState) []byte {
	b := make([]byte, InputReportSize)

	b[0] = ReportIDInput

	b[1] = uint8(int16(s.LX) + 128)
	b[2] = uint8(int16(s.LY) + 128)
	b[3] = uint8(int16(s.RX) + 128)
	b[4] = uint8(int16(s.RY) + 128)

	b[5] = s.L2
	b[6] = s.R2

	d.seqCounter++
	b[7] = d.seqCounter

	usbDPad := uint8(DPadUSBNeutral)
	switch {
	case s.DPad&DPadUp != 0 && s.DPad&DPadRight != 0:
		usbDPad = DPadUSBUpRight
	case s.DPad&DPadUp != 0 && s.DPad&DPadLeft != 0:
		usbDPad = DPadUSBUpLeft
	case s.DPad&DPadDown != 0 && s.DPad&DPadRight != 0:
		usbDPad = DPadUSBDownRight
	case s.DPad&DPadDown != 0 && s.DPad&DPadLeft != 0:
		usbDPad = DPadUSBDownLeft
	case s.DPad&DPadUp != 0:
		usbDPad = DPadUSBUp
	case s.DPad&DPadDown != 0:
		usbDPad = DPadUSBDown
	case s.DPad&DPadLeft != 0:
		usbDPad = DPadUSBLeft
	case s.DPad&DPadRight != 0:
		usbDPad = DPadUSBRight
	}
	b[8] = (usbDPad & DPadMask) | (uint8(s.Buttons) & 0xF0)
	b[9] = uint8(s.Buttons >> 8)
	b[10] = uint8(s.Buttons >> 16)

	binary.LittleEndian.PutUint16(b[16:18], uint16(s.GyroX))
	binary.LittleEndian.PutUint16(b[18:20], uint16(s.GyroY))
	binary.LittleEndian.PutUint16(b[20:22], uint16(s.GyroZ))

	binary.LittleEndian.PutUint16(b[22:24], uint16(s.AccelX))
	binary.LittleEndian.PutUint16(b[24:26], uint16(s.AccelY))
	binary.LittleEndian.PutUint16(b[26:28], uint16(s.AccelZ))

	ts := uint32(time.Since(d.timestampBase).Microseconds() * 3)
	binary.LittleEndian.PutUint32(b[28:32], ts)

	d.mtx.Lock()
	trk := d.touchTracking
	d.mtx.Unlock()

	touch1 := trk[0] & 0x7F
	if !s.Touch1Active {
		touch1 |= TouchInactiveMask
	}
	b[33] = touch1
	encodeTouchCoords(b[34:37], s.Touch1X, s.Touch1Y)

	touch2 := trk[1] & 0x7F
	if !s.Touch2Active {
		touch2 |= TouchInactiveMask
	}
	b[37] = touch2
	encodeTouchCoords(b[38:41], s.Touch2X, s.Touch2Y)

	b[49] = 0x10
	b[53] = m.BatteryStatus

	return b
}
