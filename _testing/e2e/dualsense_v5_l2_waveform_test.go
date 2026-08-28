package e2e_bench_test

import (
	"encoding/binary"
	"fmt"
	"net"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/DualSenseClient/VIIPER/_testing/e2e/sdl"
	"github.com/DualSenseClient/VIIPER/device/dualsense"
	"github.com/DualSenseClient/VIIPER/viiperclient"
)

const (
	l2WaveformOptIn            = "VIIPER_E2E_L2_WAVEFORM"
	l2PhysicalReportInterval   = time.Millisecond
	l2PhysicalChangeInterval   = 4 * time.Millisecond
	l2PhysicalPeakHold         = 2450 * time.Millisecond
	l2WaveformReleaseQuiet     = 100 * time.Millisecond
	l2WaveformObservationLimit = 128
	l2WaveformSentLimit        = 64
)

type l2WaveformPhase uint8

const (
	l2WaveformPress l2WaveformPhase = iota
	l2WaveformHold
	l2WaveformRelease
)

type l2WaveformPoint struct {
	raw      uint8
	duration time.Duration
	phase    l2WaveformPhase
}

// This trace mirrors the physical USB DualSense capture used to investigate
// the Resonance L2 report: values changed at roughly four-millisecond
// intervals, remained at 255 for approximately 2.45 seconds, then returned to
// zero monotonically. Each point is retransmitted at the physical one-
// millisecond report cadence for its duration.
var l2PhysicalWaveform = [...]l2WaveformPoint{
	{raw: 11, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 25, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 37, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 51, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 64, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 77, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 90, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 103, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 116, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 129, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 142, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 155, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 168, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 181, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 194, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 207, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 220, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 233, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 246, duration: l2PhysicalChangeInterval, phase: l2WaveformPress},
	{raw: 255, duration: l2PhysicalPeakHold, phase: l2WaveformHold},
	{raw: 242, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 226, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 211, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 196, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 181, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 166, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 151, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 136, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 121, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 106, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 91, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 76, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 61, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 46, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 31, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 16, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 4, duration: l2PhysicalChangeInterval, phase: l2WaveformRelease},
	{raw: 0, duration: l2WaveformReleaseQuiet, phase: l2WaveformRelease},
}

var l2PeakThenLowerHoldWaveform = [...]l2WaveformPoint{
	{raw: 255, duration: 20 * time.Millisecond, phase: l2WaveformPress},
	{raw: 40, duration: 250 * time.Millisecond, phase: l2WaveformHold},
	{raw: 0, duration: l2WaveformReleaseQuiet, phase: l2WaveformRelease},
}

type l2SentSample struct {
	raw     uint8
	digital bool
	at      time.Duration
}

type l2ObservedSample struct {
	axis          int16
	raw           uint8
	released      bool
	beforeRelease bool
	at            time.Duration
}

type l2WaveformResult struct {
	sent                     [l2WaveformSentLimit]l2SentSample
	observed                 [l2WaveformObservationLimit]l2ObservedSample
	sentCount                int
	observedCount            int
	sentFrames               int
	observationOverflow      int
	missingAnalogSamples     int
	unexpectedAnalogSamples  int
	monotonicViolations      int
	earlyZeroes              int
	prematureDigitalReleases int
	staleRepresses           int
	peakMissing              bool
	releaseMissing           bool
	sdlDropped               uint64
	started                  time.Time
	finalZeroSent            bool
}

type l2RawHIDFields struct {
	l2                   uint8
	digitalButtons       uint8
	sequence             uint8
	packetSequence       uint32
	sensorTimestamp      uint32
	touchTimestamp       uint8
	rightTriggerFeedback uint8
	leftTriggerFeedback  uint8
	hostTimestamp        uint32
	effectModes          uint8
	timer2               uint32
	battery              uint8
	connectState         uint8
	headsetStatus        uint8
}

type l2RawHIDComparison struct {
	peak    l2RawHIDFields
	lower   l2RawHIDFields
	release l2RawHIDFields
}

func (r *l2WaveformResult) reset(started time.Time) {
	*r = l2WaveformResult{started: started}
}

func (r *l2WaveformResult) recordSent(raw uint8, at time.Time) {
	if r.sentCount < len(r.sent) {
		r.sent[r.sentCount] = l2SentSample{
			raw: raw, digital: raw != 0, at: at.Sub(r.started),
		}
		r.sentCount++
	}
	if raw == 0 {
		r.finalZeroSent = true
	}
}

func (r *l2WaveformResult) recordObserved(observation triggerObservation,
	neutral int16) {
	if r.observedCount >= len(r.observed) {
		r.observationOverflow++
		return
	}
	raw := l2RawFromSDLAxis(neutral, observation.axis)
	r.observed[r.observedCount] = l2ObservedSample{
		axis:          observation.axis,
		raw:           raw,
		released:      observation.released,
		beforeRelease: !r.finalZeroSent,
		at:            observation.at.Sub(r.started),
	}
	r.observedCount++
}

func (r *l2WaveformResult) analyze(expected []l2WaveformPoint) {
	nextExpected := 0
	peakSeen := false
	releaseSeen := false
	var previous uint8
	for index := 0; index < r.observedCount; index++ {
		observed := r.observed[index]
		if observed.released && observed.beforeRelease {
			r.earlyZeroes++
			// SDL exposes the trigger consumer state as an axis. Neutral is the
			// consumer-observed digital-inactive edge for this diagnostic.
			r.prematureDigitalReleases++
		}

		if releaseSeen && observed.raw != 0 {
			r.staleRepresses++
		}
		if observed.raw == 0 {
			releaseSeen = true
		}

		if !peakSeen {
			if index != 0 && observed.raw < previous {
				r.monotonicViolations++
			}
			if observed.raw == 255 {
				peakSeen = true
			}
		} else if observed.raw > previous {
			r.monotonicViolations++
		}
		previous = observed.raw

		matched := -1
		for expectedIndex := nextExpected; expectedIndex < len(expected); expectedIndex++ {
			if l2RawNear(observed.raw, expected[expectedIndex].raw) {
				matched = expectedIndex
				break
			}
		}
		if matched < 0 {
			r.unexpectedAnalogSamples++
			continue
		}
		r.missingAnalogSamples += matched - nextExpected
		nextExpected = matched + 1
	}
	if nextExpected < len(expected) {
		r.missingAnalogSamples += len(expected) - nextExpected
	}
	r.peakMissing = !peakSeen
	r.releaseMissing = !releaseSeen
}

func (r *l2WaveformResult) valid() bool {
	return r.observationOverflow == 0 && r.missingAnalogSamples == 0 &&
		r.unexpectedAnalogSamples == 0 && r.monotonicViolations == 0 &&
		r.earlyZeroes == 0 && r.prematureDigitalReleases == 0 &&
		r.staleRepresses == 0 && !r.peakMissing && !r.releaseMissing &&
		r.sdlDropped == 0
}

func l2RawNear(left, right uint8) bool {
	difference := int(left) - int(right)
	if difference < 0 {
		difference = -difference
	}
	return difference <= 1
}

func l2RawFromSDLAxis(neutral, axis int16) uint8 {
	span := int(sdl.JoystickAxisMax) - int(neutral)
	if span <= 0 || axis <= neutral {
		return 0
	}
	if axis >= int16(sdl.JoystickAxisMax) {
		return 255
	}
	value := (int(axis)-int(neutral))*255 + span/2
	value /= span
	if value < 0 {
		return 0
	}
	if value > 255 {
		return 255
	}
	return uint8(value)
}

type l2WaveformRig struct {
	harness        *dualSenseHarness
	deviceID       string
	stream         *viiperclient.DeviceStream
	writer         *v5FrameWriter
	gamepad        *sdl.Gamepad
	observer       *triggerObserver
	outputCounters *v5OutputCounters
	outputDone     chan struct{}
	timer          *time.Timer
	closeOnce      sync.Once
}

func newL2WaveformRig(tb testing.TB) *l2WaveformRig {
	tb.Helper()
	existingGamepads, err := gamepadSet()
	if err != nil {
		tb.Fatalf("snapshot SDL3 gamepads: %v", err)
	}
	rig := &l2WaveformRig{harness: newDualSenseHarness(tb)}
	tb.Cleanup(func() { rig.close(tb) })

	deviceInfo, err := rig.harness.client.DeviceAdd(rig.harness.busID,
		dualsense.DeviceTypeCombinedAudioDuplexV5RawInputEvents, nil)
	if err != nil {
		tb.Fatalf("DeviceAdd(DualSense V5 raw-input events): %v", err)
	}
	rig.deviceID = deviceInfo.DevID
	rig.stream, err = rig.harness.client.OpenStream(rig.harness.ctx,
		rig.harness.busID, rig.deviceID)
	if err != nil {
		tb.Fatalf("OpenStream: %v", err)
	}
	rig.outputCounters = &v5OutputCounters{}
	rig.outputDone = make(chan struct{})
	go drainV5Output(rig.stream, rig.outputCounters, rig.outputDone)

	rig.gamepad, err = waitForVirtualPS5(existingGamepads)
	if err != nil {
		tb.Fatalf("actual SDL3 virtual PS5 discovery failed: %v", err)
	}
	rig.writer = newV5FrameWriter(rig.stream, false, true)
	if !rig.writer.enqueue(dualsense.InputState{}) {
		tb.Fatalf("enqueue initial neutral state: %v", rig.writer.terminalErr)
	}
	if err := rig.writer.waitInputResult(); err != nil {
		tb.Fatalf("write initial neutral state: %v", err)
	}
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		sdl.UpdateGamepads()
		if rig.gamepad.GetAxis(sdl.GamepadAxisLeftTrigger) <= 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	rig.observer, err = startGamepadAxisObserver(rig.gamepad,
		sdl.GamepadAxisLeftTrigger)
	if err != nil {
		tb.Fatalf("install SDL3 left-trigger axis watch: %v", err)
	}
	rig.timer = time.NewTimer(time.Hour)
	if !rig.timer.Stop() {
		<-rig.timer.C
	}
	tb.Logf("SDL3 L2 consumer: name=%q type=%s vid=%04x pid=%04x neutral-axis=%d",
		rig.gamepad.Name(), rig.gamepad.Type().Name(), rig.gamepad.Vendor(),
		rig.gamepad.Product(), rig.observer.neutral)
	return rig
}

func (r *l2WaveformRig) close(tb testing.TB) {
	tb.Helper()
	r.closeOnce.Do(func() {
		if r.observer != nil {
			r.observer.close()
		}
		if r.timer != nil {
			r.timer.Stop()
		}
		if r.writer != nil {
			r.writer.stopAndWait()
		}
		if r.stream != nil {
			_ = r.stream.Close()
		}
		if r.outputDone != nil {
			select {
			case <-r.outputDone:
			case <-time.After(time.Second):
				tb.Log("V5 output reader did not stop within one second")
			}
		}
		if r.gamepad != nil {
			r.gamepad.Close()
		}
		if r.deviceID != "" && r.harness != nil {
			if _, err := r.harness.client.DeviceRemove(r.harness.busID, r.deviceID); err != nil {
				tb.Logf("DeviceRemove(%s): %v", r.deviceID, err)
			}
		}
		if r.harness != nil {
			r.harness.close(tb)
		}
	})
}

func (r *l2WaveformRig) replay(result *l2WaveformResult,
	waveform []l2WaveformPoint) error {
	drainObservations(r.observer)
	started := time.Now()
	result.reset(started)
	nextDeadline := started
	var physicalTimestamp uint32
	peakReached := false
	for pointIndex := range waveform {
		point := waveform[pointIndex]
		result.recordSent(point.raw, time.Now())
		reports := max(1, int(point.duration/l2PhysicalReportInterval))
		for report := 0; report < reports; report++ {
			physicalTimestamp += 3000
			status := capturedL2TriggerStatus(point, peakReached, report)
			state := capturedL2RawInputState(point.raw, status,
				physicalTimestamp)
			if point.raw == 255 {
				peakReached = true
			}
			if !r.writer.enqueue(state) {
				return r.writer.terminalErr
			}
			if err := r.writer.waitInputResult(); err != nil {
				return err
			}
			result.sentFrames++
			r.collect(result)
			nextDeadline = r.waitForNextPhysicalReport(nextDeadline)
			r.collect(result)
		}
	}
	result.analyze(waveform)
	return nil
}

func capturedL2TriggerStatus(point l2WaveformPoint, peakReached bool,
	report int) byte {
	if point.raw == 0 {
		return 0x09
	}
	if point.raw == 255 {
		if !peakReached && report == 0 {
			return 0x28
		}
		return 0x29
	}
	if peakReached || point.phase == l2WaveformRelease {
		return 0x29
	}
	switch {
	case point.raw <= 55:
		return 0x02
	case point.raw <= 90:
		return 0x12
	case point.raw <= 129:
		return 0x13
	case point.raw <= 168:
		return 0x14
	case point.raw <= 205:
		return 0x15
	case point.raw <= 220:
		return 0x26
	case point.raw <= 233:
		return 0x27
	default:
		return 0x28
	}
}

func capturedL2RawInputState(l2, status byte,
	physicalTimestamp uint32) dualsense.InputState {
	state := dualsense.InputState{
		L2:                      l2,
		PhysicalMetadataValid:   true,
		PhysicalSensorTimestamp: physicalTimestamp,
	}
	if l2 != 0 {
		state.Buttons = dualsense.ButtonL2
	}
	state.PhysicalInputMetadata[1] = 0x09
	state.PhysicalInputMetadata[2] = status
	state.PhysicalInputMetadata[7] = 0x20
	binary.LittleEndian.PutUint32(state.PhysicalInputMetadata[8:12],
		physicalTimestamp+2)
	state.PhysicalInputMetadata[12] = 0x25
	state.PhysicalInputMetadata[13] = 0x01
	state.PhysicalInputMetadata[14] = 0xA5
	return state
}

func (r *l2WaveformRig) collect(result *l2WaveformResult) {
	for {
		observation, ok := r.observer.pollTriggerObservation()
		if !ok {
			return
		}
		result.recordObserved(observation, r.observer.neutral)
	}
}

func (r *l2WaveformRig) waitForNextPhysicalReport(previous time.Time) time.Time {
	next := previous.Add(l2PhysicalReportInterval)
	now := time.Now()
	if !now.Before(next) {
		if now.Sub(next) >= l2PhysicalReportInterval {
			return now
		}
		return next
	}
	resetTimer(r.timer, next.Sub(now))
	<-r.timer.C
	return next
}

func configureIsolatedL2Waveform(tb testing.TB) {
	tb.Helper()
	if !envBool(l2WaveformOptIn) {
		tb.Skipf("set %s=1 to create an isolated in-process VIIPER bus and virtual DualSense", l2WaveformOptIn)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		tb.Fatalf("reserve isolated API listen address: %v", err)
	}
	apiAddress := listener.Addr().String()
	if err := listener.Close(); err != nil {
		tb.Fatalf("release isolated API listen address: %v", err)
	}
	tb.Setenv("VIIPER_E2E_EXTERNAL", "false")
	tb.Setenv("VIIPER_E2E_API_ADDR", apiAddress)
	tb.Setenv("VIIPER_E2E_API_LISTEN", apiAddress)
	tb.Setenv("VIIPER_E2E_USB_LISTEN", "127.0.0.1:0")
	tb.Setenv("VIIPER_E2E_PASSWORD", "")
}

func TestDualSenseV5L2PhysicalWaveformSDL(t *testing.T) {
	configureIsolatedL2Waveform(t)
	if err := sdl.Init(sdl.InitFlagGamepad); err != nil {
		t.Fatalf("SDL3 gamepad initialization failed: %v", err)
	}
	t.Cleanup(sdl.Quit)
	rig := newL2WaveformRig(t)
	var physical, lowerHold l2WaveformResult
	if err := rig.replay(&physical, l2PhysicalWaveform[:]); err != nil {
		t.Fatalf("replay physical L2 waveform over V5: %v", err)
	}
	if err := rig.replay(&lowerHold, l2PeakThenLowerHoldWaveform[:]); err != nil {
		t.Fatalf("replay peak-then-lower-held L2 waveform over V5: %v", err)
	}
	dropped := rig.observer.close()
	physical.sdlDropped = dropped
	lowerHold.sdlDropped = dropped
	t.Log("physical-capture waveform")
	reportL2Waveform(t, &physical)
	t.Log("peak-then-lower-held waveform (raw 40 remains analog-active and digitally pressed)")
	reportL2Waveform(t, &lowerHold)
	rawComparison := captureL2RawHIDComparison(t)
	reportL2RawHIDComparison(t, rawComparison)
	validateL2RawHIDComparison(t, rawComparison)
	if !physical.valid() {
		t.Fatalf("virtual physical-capture L2 waveform correctness failure: %s", physical.summary())
	}
	if !lowerHold.valid() {
		t.Fatalf("virtual lower-held L2 waveform correctness failure: %s", lowerHold.summary())
	}
	if invalid := rig.outputCounters.invalidFrames.Load(); invalid != 0 {
		t.Fatalf("V5 output reader detected %d invalid frames", invalid)
	}
}

// Benchmark_DualSenseV5_L2PhysicalWaveformSDL is intentionally opt-in and is
// normally run with -benchtime=1x. Its measured operation is one complete
// physical-cadence V5 waveform observed through SDL3, not an internal callback.
func Benchmark_DualSenseV5_L2PhysicalWaveformSDL(b *testing.B) {
	configureIsolatedL2Waveform(b)
	if err := sdl.Init(sdl.InitFlagGamepad); err != nil {
		b.Fatalf("SDL3 gamepad initialization failed: %v", err)
	}
	b.Cleanup(sdl.Quit)
	rig := newL2WaveformRig(b)
	var result l2WaveformResult
	b.ReportAllocs()
	b.ResetTimer()
	for iteration := 0; iteration < b.N; iteration++ {
		if iteration != 0 {
			b.StopTimer()
			b.Fatalf("the fixed SDL event watch is one-shot; run this diagnostic with -benchtime=1x")
		}
		if err := rig.replay(&result, l2PhysicalWaveform[:]); err != nil {
			b.StopTimer()
			b.Fatalf("replay physical L2 waveform over V5: %v", err)
		}
	}
	b.StopTimer()
	result.sdlDropped = rig.observer.close()
	b.ReportMetric(float64(result.sentFrames), "v5-input-frames")
	b.ReportMetric(float64(result.observedCount), "sdl-axis-events")
	b.ReportMetric(float64(result.missingAnalogSamples), "missing-analog-samples")
	b.ReportMetric(float64(result.earlyZeroes), "early-zeroes")
	b.ReportMetric(float64(result.prematureDigitalReleases), "premature-releases")
	b.ReportMetric(float64(result.staleRepresses), "stale-represses")
	reportL2Waveform(b, &result)
	rawComparison := captureL2RawHIDComparison(b)
	reportL2RawHIDComparison(b, rawComparison)
	validateL2RawHIDComparison(b, rawComparison)
	if !result.valid() {
		b.Errorf("virtual L2 waveform correctness failure: %s", result.summary())
	}
}

func reportL2Waveform(tb testing.TB, result *l2WaveformResult) {
	tb.Helper()
	tb.Logf("V5 sent L2 sequence (raw,digital,time): %s", result.sentSequence())
	tb.Logf("SDL3 observed LEFT trigger sequence (axis,raw,time): %s", result.observedSequence())
	tb.Logf("L2 waveform result: %s", result.summary())
}

func captureL2RawHIDComparison(tb testing.TB) l2RawHIDComparison {
	tb.Helper()
	device, err := dualsense.New(nil)
	if err != nil {
		tb.Fatalf("create isolated DualSense encoder for raw report comparison: %v", err)
	}
	var peak, lower, release [dualsense.InputReportSize]byte
	state := capturedL2RawInputState(255, 0x29, 0x11111111)
	device.UpdateInputState(&state)
	if n := device.BuildInputReportInto(peak[:]); n != len(peak) {
		tb.Fatalf("build peak raw virtual HID report: got %d bytes, want %d", n, len(peak))
	}
	time.Sleep(2 * time.Millisecond)
	state = capturedL2RawInputState(40, 0x29, 0x22222222)
	device.UpdateInputState(&state)
	if n := device.BuildInputReportInto(lower[:]); n != len(lower) {
		tb.Fatalf("build lower-held raw virtual HID report: got %d bytes, want %d", n, len(lower))
	}
	time.Sleep(2 * time.Millisecond)
	state = capturedL2RawInputState(0, 0x09, 0x33333333)
	device.UpdateInputState(&state)
	if n := device.BuildInputReportInto(release[:]); n != len(release) {
		tb.Fatalf("build release raw virtual HID report: got %d bytes, want %d", n, len(release))
	}
	return l2RawHIDComparison{
		peak:    decodeL2RawHIDFields(peak[:]),
		lower:   decodeL2RawHIDFields(lower[:]),
		release: decodeL2RawHIDFields(release[:]),
	}
}

func decodeL2RawHIDFields(report []byte) l2RawHIDFields {
	return l2RawHIDFields{
		l2:                   report[5],
		digitalButtons:       report[9],
		sequence:             report[7],
		packetSequence:       binary.LittleEndian.Uint32(report[12:16]),
		sensorTimestamp:      binary.LittleEndian.Uint32(report[28:32]),
		touchTimestamp:       report[41],
		rightTriggerFeedback: report[42],
		leftTriggerFeedback:  report[43],
		hostTimestamp:        binary.LittleEndian.Uint32(report[44:48]),
		effectModes:          report[48],
		timer2:               binary.LittleEndian.Uint32(report[49:53]),
		battery:              report[53],
		connectState:         report[54],
		headsetStatus:        report[55],
	}
}

func reportL2RawHIDComparison(tb testing.TB, comparison l2RawHIDComparison) {
	tb.Helper()
	tb.Log("physical USB reference: byte[7] increments; uint32 byte[12:16] increments; " +
		"uint32 byte[28:32] advances and wraps; uint32 byte[49:53] advances; byte[54]=0x08")
	reportL2RawHIDFields(tb, "peak", comparison.peak)
	reportL2RawHIDFields(tb, "lower-held", comparison.lower)
	reportL2RawHIDFields(tb, "release", comparison.release)
}

func reportL2RawHIDFields(tb testing.TB, label string, fields l2RawHIDFields) {
	tb.Helper()
	tb.Logf("VIIPER raw virtual HID %s: L2 byte[5]=%d digital byte[9]=0x%02x "+
		"sequence byte[7]=%d packet-sequence byte[12:16]=%d sensor-timestamp byte[28:32]=%d "+
		"touch-timestamp byte[41]=%d trigger-feedback byte[42:44]=%02x/%02x "+
		"host-timestamp byte[44:48]=%d effect-modes byte[48]=0x%02x "+
		"timer2 byte[49:53]=%d battery byte[53]=0x%02x connect byte[54]=0x%02x "+
		"headset/filter byte[55]=0x%02x", label,
		fields.l2, fields.digitalButtons, fields.sequence, fields.packetSequence,
		fields.sensorTimestamp, fields.touchTimestamp, fields.rightTriggerFeedback,
		fields.leftTriggerFeedback, fields.hostTimestamp, fields.effectModes,
		fields.timer2, fields.battery, fields.connectState, fields.headsetStatus)
}

func validateL2RawHIDComparison(tb testing.TB, comparison l2RawHIDComparison) {
	tb.Helper()
	states := [...]struct {
		name    string
		fields  l2RawHIDFields
		l2      uint8
		digital bool
		seq     uint32
	}{
		{name: "peak", fields: comparison.peak, l2: 255, digital: true, seq: 1},
		{name: "lower-held", fields: comparison.lower, l2: 40, digital: true, seq: 2},
		{name: "release", fields: comparison.release, l2: 0, digital: false, seq: 3},
	}
	for _, state := range states {
		digital := state.fields.digitalButtons&byte(dualsense.ButtonL2>>8) != 0
		if state.fields.l2 != state.l2 || digital != state.digital ||
			state.fields.sequence != uint8(state.seq) ||
			state.fields.packetSequence != state.seq {
			tb.Fatalf("%s raw L2/counter mismatch: %+v", state.name, state.fields)
		}
		if state.fields.touchTimestamp != 0 ||
			state.fields.connectState != 0x08 || state.fields.headsetStatus != 0xA5 {
			tb.Fatalf("%s raw fixed-field mismatch: %+v", state.name, state.fields)
		}
		wantStatus := uint8(0x29)
		if state.name == "release" {
			wantStatus = 0x09
		}
		if state.fields.rightTriggerFeedback != 0x09 ||
			state.fields.leftTriggerFeedback != wantStatus ||
			state.fields.effectModes != 0x20 {
			tb.Fatalf("%s raw physical trigger metadata mismatch: %+v",
				state.name, state.fields)
		}
	}
	if comparison.peak.sensorTimestamp != 0x11111111 ||
		comparison.lower.sensorTimestamp != 0x22222222 ||
		comparison.release.sensorTimestamp != 0x33333333 ||
		comparison.peak.timer2 != 0x11111113 ||
		comparison.lower.timer2 != 0x22222224 ||
		comparison.release.timer2 != 0x33333335 {
		tb.Fatalf("raw physical controller clocks were not preserved: %+v",
			comparison)
	}
}

func (r *l2WaveformResult) sentSequence() string {
	var builder strings.Builder
	for index := 0; index < r.sentCount; index++ {
		if index != 0 {
			builder.WriteString(" -> ")
		}
		sample := r.sent[index]
		fmt.Fprintf(&builder, "%d/%t@%s", sample.raw, sample.digital,
			sample.at.Round(time.Microsecond))
	}
	return builder.String()
}

func (r *l2WaveformResult) observedSequence() string {
	var builder strings.Builder
	for index := 0; index < r.observedCount; index++ {
		if index != 0 {
			builder.WriteString(" -> ")
		}
		sample := r.observed[index]
		fmt.Fprintf(&builder, "%d/%d@%s", sample.axis, sample.raw,
			sample.at.Round(time.Microsecond))
	}
	return builder.String()
}

func (r *l2WaveformResult) summary() string {
	return fmt.Sprintf("sent-frames=%d sent-changes=%d observed=%d missing=%d unexpected=%d monotonic-violations=%d early-zeroes=%d premature-digital-releases=%d stale-represses=%d peak-missing=%t release-missing=%t trace-overflow=%d sdl-dropped=%d",
		r.sentFrames, r.sentCount, r.observedCount, r.missingAnalogSamples,
		r.unexpectedAnalogSamples, r.monotonicViolations, r.earlyZeroes,
		r.prematureDigitalReleases, r.staleRepresses, r.peakMissing,
		r.releaseMissing, r.observationOverflow, r.sdlDropped)
}

func TestL2WaveformAnalyzerDetectsPrematureReleaseAndStaleRepress(t *testing.T) {
	var result l2WaveformResult
	started := time.Now()
	result.reset(started)
	for index := range l2PhysicalWaveform {
		result.recordSent(l2PhysicalWaveform[index].raw, started)
	}
	result.observed[0] = l2ObservedSample{raw: 11}
	result.observed[1] = l2ObservedSample{raw: 0, released: true, beforeRelease: true}
	result.observed[2] = l2ObservedSample{raw: 255}
	result.observed[3] = l2ObservedSample{raw: 0, released: true}
	result.observedCount = 4
	result.analyze(l2PhysicalWaveform[:])
	if result.earlyZeroes != 1 || result.prematureDigitalReleases != 1 ||
		result.staleRepresses != 1 || result.monotonicViolations == 0 {
		t.Fatalf("premature-release analyzer did not classify the synthetic fault: %s",
			result.summary())
	}
}
