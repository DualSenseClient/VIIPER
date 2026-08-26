package e2e_bench_test

import (
	"encoding/binary"
	"encoding/json"
	"hash/crc32"
	"io"
	"runtime"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Alia5/VIIPER/device/dualsense"
	"github.com/Alia5/VIIPER/viiperclient"
)

const (
	v5InputQueueCapacity = 64
	v5InputBurstLimit    = 16
	v5MicrophoneInterval = 10 * time.Millisecond
	v5MaxOutputPayload   = 4096
	v5FrameHeaderSize    = 16
	v5InputStateSize     = 33
	v5MicrophoneSize     = 1920
	v5FrameMagic0        = 0x56
	v5FrameMagic1        = 0x50
	v5FrameMagic2        = 0x43
	v5FrameMagic3        = 0x4d
	v5FrameVersion       = 0x05
	v5FrameInput         = 0x01
	v5FrameMicrophone    = 0x02
	v5FrameOutput        = 0x81
	v5FrameAtomicAudio   = 0x83
	v5FrameRealtime      = 0x84
	v5FrameMicInterface  = 0x85
	sdlTriggerPeakAxis   = int16(30000)
)

type triggerObservation struct {
	axis     int16
	released bool
	at       time.Time
}

type expectedTransition struct {
	pressed bool
	sentAt  time.Time
}

type triggerObservationSource interface {
	pollTriggerObservation() (triggerObservation, bool)
}

type transitionResults struct {
	delivered    int
	missed       int
	reordered    int
	falseRepress int
	latencies    []time.Duration
	latencyCount int
	peakAxis     int16
}

const transitionTraceCapacity = 64

const (
	transitionTraceWait uint8 = iota
	transitionTraceQuiet
)

type transitionTraceEntry struct {
	axis          int16
	expectedIndex uint8
	phase         uint8
	released      bool
}

// transitionTrace is fixed storage for diagnosing a correctness failure. It
// is reset and reused for every epoch, and only copied when an epoch changes a
// correctness counter, so tracing does not add timed-loop allocation.
type transitionTrace struct {
	entries   [transitionTraceCapacity]transitionTraceEntry
	count     int
	epoch     int
	remaining int
	timedOut  bool
}

func (t *transitionTrace) reset(epoch int) {
	t.count = 0
	t.epoch = epoch
	t.remaining = 0
	t.timedOut = false
}

func (t *transitionTrace) record(observed triggerObservation, expectedIndex int,
	phase uint8) {
	if t.count >= len(t.entries) {
		return
	}
	t.entries[t.count] = transitionTraceEntry{
		axis:          observed.axis,
		expectedIndex: uint8(expectedIndex),
		phase:         phase,
		released:      observed.released,
	}
	t.count++
}

func newTransitionResults(capacity int) transitionResults {
	return transitionResults{
		latencies: make([]time.Duration, capacity),
		peakAxis:  -32768,
	}
}

func (r *transitionResults) recordDelivered(expected expectedTransition,
	observed triggerObservation) {
	r.delivered++
	if r.latencyCount < len(r.latencies) {
		latency := observed.at.Sub(expected.sentAt)
		if latency < 0 {
			latency = 0
		}
		r.latencies[r.latencyCount] = latency
		r.latencyCount++
	}
}

// classifyObservation advances over one or more expected transitions. A
// release observed before its press is both an undelivered press and a
// reordering. A press while waiting for release is a false re-press and does
// not consume that release expectation.
func classifyObservation(expected []expectedTransition, next int,
	observed triggerObservation, results *transitionResults) int {
	results.peakAxis = max(results.peakAxis, observed.axis)
	for next < len(expected) {
		want := expected[next]
		matches := observed.released
		if want.pressed {
			matches = observed.axis >= sdlTriggerPeakAxis
		}
		if matches {
			results.recordDelivered(want, observed)
			return next + 1
		}
		if want.pressed {
			if observed.released {
				results.missed++
				results.reordered++
				next++
				continue
			}
			// Intermediate nonzero values are useful peak evidence but do not
			// satisfy the epoch until the consumer observes the meaningful peak.
			return next
		}
		if observed.axis >= sdlTriggerPeakAxis {
			results.falseRepress++
			return next
		}
		// Intermediate active values while awaiting release neither consume the
		// release nor invent another transition.
		return next
	}
	if observed.axis >= sdlTriggerPeakAxis {
		results.falseRepress++
	}
	return next
}

type latencyDistribution struct {
	median time.Duration
	p95    time.Duration
	p99    time.Duration
	p999   time.Duration
	max    time.Duration
	count  int
}

func durationPercentile(sorted []time.Duration, percentile float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	index := int(float64(len(sorted)-1)*percentile + 0.5)
	if index < 0 {
		index = 0
	}
	if index >= len(sorted) {
		index = len(sorted) - 1
	}
	return sorted[index]
}

func summarizeLatencies(values []time.Duration) latencyDistribution {
	if len(values) == 0 {
		return latencyDistribution{}
	}
	sort.Slice(values, func(left, right int) bool { return values[left] < values[right] })
	return latencyDistribution{
		median: durationPercentile(values, 0.50),
		p95:    durationPercentile(values, 0.95),
		p99:    durationPercentile(values, 0.99),
		p999:   durationPercentile(values, 0.999),
		max:    values[len(values)-1],
		count:  len(values),
	}
}

func resetTimer(timer *time.Timer, duration time.Duration) {
	if !timer.Stop() {
		select {
		case <-timer.C:
		default:
		}
	}
	timer.Reset(duration)
}

func waitForExpectedTransitions(timer *time.Timer,
	observations triggerObservationSource, expected []expectedTransition,
	timeout time.Duration, results *transitionResults, trace *transitionTrace) {
	next := 0
	resetTimer(timer, timeout)
	for next < len(expected) {
		if observation, ok := observations.pollTriggerObservation(); ok {
			if trace != nil {
				trace.record(observation, next, transitionTraceWait)
			}
			next = classifyObservation(expected, next, observation, results)
			continue
		}
		select {
		case <-timer.C:
			if trace != nil {
				trace.timedOut = true
				trace.remaining = len(expected) - next
			}
			results.missed += len(expected) - next
			return
		default:
			runtime.Gosched()
		}
	}
}

func drainObservations(observations triggerObservationSource) {
	for {
		if _, ok := observations.pollTriggerObservation(); !ok {
			return
		}
	}
}

type v5InputRequest struct {
	state dualsense.InputState
}

// v5FrameWriter is the sole owner of the client-to-server V5 byte stream and
// sequence. Input and microphone producers publish logical work only.
type v5FrameWriter struct {
	stream      *viiperclient.DeviceStream
	input       chan v5InputRequest
	inputResult chan error
	stop        chan struct{}
	done        chan struct{}
	stopOnce    sync.Once
	loaded      bool
	sequence    uint32
	frame       [v5FrameHeaderSize + v5MicrophoneSize]byte
	inputBytes  [v5InputStateSize]byte
	microphone  [v5MicrophoneSize]byte
	terminalErr error
}

func newV5FrameWriter(stream *viiperclient.DeviceStream, loaded bool) *v5FrameWriter {
	w := &v5FrameWriter{
		stream:      stream,
		input:       make(chan v5InputRequest, v5InputQueueCapacity),
		inputResult: make(chan error, v5InputQueueCapacity),
		stop:        make(chan struct{}),
		done:        make(chan struct{}),
		loaded:      loaded,
	}
	// A deterministic, bounded stereo waveform avoids a completely empty
	// microphone source while keeping generation outside the timed loop.
	for frame := 0; frame < 480; frame++ {
		sample := int16((frame%96 - 48) * 192)
		offset := frame * 4
		binary.LittleEndian.PutUint16(w.microphone[offset:offset+2], uint16(sample))
		binary.LittleEndian.PutUint16(w.microphone[offset+2:offset+4], uint16(-sample))
	}
	go w.run()
	return w
}

func (w *v5FrameWriter) enqueue(state dualsense.InputState) bool {
	select {
	case w.input <- v5InputRequest{state: state}:
		return true
	case <-w.done:
		return false
	}
}

func (w *v5FrameWriter) waitInputResult() error {
	select {
	case err := <-w.inputResult:
		return err
	case <-w.done:
		select {
		case err := <-w.inputResult:
			return err
		default:
			return w.terminalErr
		}
	}
}

func (w *v5FrameWriter) stopAndWait() {
	w.stopOnce.Do(func() { close(w.stop) })
	<-w.done
}

func (w *v5FrameWriter) run() {
	defer close(w.done)
	var microphoneTicker *time.Ticker
	var microphoneDue <-chan time.Time
	if w.loaded {
		microphoneTicker = time.NewTicker(v5MicrophoneInterval)
		microphoneDue = microphoneTicker.C
		defer microphoneTicker.Stop()
	}

	burst := 0
	for {
		if burst < v5InputBurstLimit {
			select {
			case request := <-w.input:
				if !w.writeInput(request) {
					return
				}
				burst++
				continue
			default:
			}
		}

		if burst >= v5InputBurstLimit {
			select {
			case <-microphoneDue:
				if err := w.writeFrame(v5FrameMicrophone, w.microphone[:]); err != nil {
					w.terminalErr = err
					return
				}
				burst = 0
				continue
			default:
			}
		}

		select {
		case request := <-w.input:
			if !w.writeInput(request) {
				return
			}
			burst++
		case <-microphoneDue:
			// A due but unstarted media frame never stays ahead of newly
			// available input.
			select {
			case request := <-w.input:
				if !w.writeInput(request) {
					return
				}
				burst++
				continue
			default:
			}
			if err := w.writeFrame(v5FrameMicrophone, w.microphone[:]); err != nil {
				w.terminalErr = err
				return
			}
			burst = 0
		case <-w.stop:
			return
		}
	}
}

func (w *v5FrameWriter) writeInput(request v5InputRequest) bool {
	marshalV5InputState(&request.state, w.inputBytes[:])
	err := w.writeFrame(v5FrameInput, w.inputBytes[:])
	w.inputResult <- err
	if err != nil {
		w.terminalErr = err
		return false
	}
	return true
}

func marshalV5InputState(state *dualsense.InputState, destination []byte) {
	b := destination[:v5InputStateSize]
	b[0] = uint8(state.LX)
	b[1] = uint8(state.LY)
	b[2] = uint8(state.RX)
	b[3] = uint8(state.RY)
	binary.LittleEndian.PutUint32(b[4:8], state.Buttons)
	b[8] = state.DPad
	b[9] = state.L2
	b[10] = state.R2
	binary.LittleEndian.PutUint16(b[11:13], state.Touch1X)
	binary.LittleEndian.PutUint16(b[13:15], state.Touch1Y)
	b[15] = v5TouchStatus(state.Touch1Active, state.Touch1Tracking)
	binary.LittleEndian.PutUint16(b[16:18], state.Touch2X)
	binary.LittleEndian.PutUint16(b[18:20], state.Touch2Y)
	b[20] = v5TouchStatus(state.Touch2Active, state.Touch2Tracking)
	binary.LittleEndian.PutUint16(b[21:23], uint16(state.GyroX))
	binary.LittleEndian.PutUint16(b[23:25], uint16(state.GyroY))
	binary.LittleEndian.PutUint16(b[25:27], uint16(state.GyroZ))
	binary.LittleEndian.PutUint16(b[27:29], uint16(state.AccelX))
	binary.LittleEndian.PutUint16(b[29:31], uint16(state.AccelY))
	binary.LittleEndian.PutUint16(b[31:33], uint16(state.AccelZ))
}

func v5TouchStatus(active bool, tracking uint8) byte {
	if tracking != 0 {
		if active {
			return tracking &^ 0x80
		}
		return tracking | 0x80
	}
	if active {
		// Zero is a valid active tracking ID on USB but the V5 transport uses
		// zero as an older boolean-active representation.
		return 1
	}
	return 0x80
}

func (w *v5FrameWriter) writeFrame(frameType byte, payload []byte) error {
	length := v5FrameHeaderSize + len(payload)
	packet := w.frame[:length]
	header := packet[:v5FrameHeaderSize]
	header[0] = v5FrameMagic0
	header[1] = v5FrameMagic1
	header[2] = v5FrameMagic2
	header[3] = v5FrameMagic3
	header[4] = v5FrameVersion
	header[5] = frameType
	binary.LittleEndian.PutUint16(header[6:8], uint16(len(payload)))
	binary.LittleEndian.PutUint32(header[8:12], w.sequence)
	crc := crc32.Update(0, crc32.IEEETable, header[4:12])
	crc = crc32.Update(crc, crc32.IEEETable, payload)
	binary.LittleEndian.PutUint32(header[12:16], crc)
	copy(packet[v5FrameHeaderSize:], payload)
	w.sequence++
	for len(packet) != 0 {
		n, err := w.stream.Write(packet)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrUnexpectedEOF
		}
		packet = packet[n:]
	}
	return nil
}

type v5OutputCounters struct {
	outputState        atomic.Uint64
	atomicAudio        atomic.Uint64
	realtimeHaptics    atomic.Uint64
	microphoneEvents   atomic.Uint64
	microphoneActive   atomic.Bool
	microphoneInactive atomic.Bool
	invalidFrames      atomic.Uint64
}

func drainV5Output(stream *viiperclient.DeviceStream, counters *v5OutputCounters,
	done chan<- struct{}) {
	defer close(done)
	var header [v5FrameHeaderSize]byte
	var payload [v5MaxOutputPayload]byte
	var expectedSequence uint32
	var sequenceInitialized bool
	for {
		if _, err := io.ReadFull(stream, header[:]); err != nil {
			return
		}
		if header[0] != v5FrameMagic0 || header[1] != v5FrameMagic1 ||
			header[2] != v5FrameMagic2 || header[3] != v5FrameMagic3 ||
			header[4] != v5FrameVersion {
			counters.invalidFrames.Add(1)
			return
		}
		payloadLength := int(binary.LittleEndian.Uint16(header[6:8]))
		if payloadLength > len(payload) {
			counters.invalidFrames.Add(1)
			return
		}
		body := payload[:payloadLength]
		if _, err := io.ReadFull(stream, body); err != nil {
			return
		}
		sequence := binary.LittleEndian.Uint32(header[8:12])
		if sequenceInitialized && sequence != expectedSequence {
			counters.invalidFrames.Add(1)
			return
		}
		expectedSequence = sequence + 1
		sequenceInitialized = true
		crc := crc32.Update(0, crc32.IEEETable, header[4:12])
		crc = crc32.Update(crc, crc32.IEEETable, body)
		if crc != binary.LittleEndian.Uint32(header[12:16]) {
			counters.invalidFrames.Add(1)
			return
		}
		switch header[5] {
		case v5FrameOutput:
			counters.outputState.Add(1)
		case v5FrameAtomicAudio:
			counters.atomicAudio.Add(1)
		case v5FrameRealtime:
			counters.realtimeHaptics.Add(1)
		case v5FrameMicInterface:
			if len(body) != 9 {
				counters.invalidFrames.Add(1)
				return
			}
			counters.microphoneEvents.Add(1)
			if len(body) != 0 && body[0] != 0 {
				counters.microphoneActive.Store(true)
			} else {
				counters.microphoneInactive.Store(true)
			}
		default:
			counters.invalidFrames.Add(1)
			return
		}
	}
}

func TestClassifyObservation(t *testing.T) {
	now := time.Now()
	expected := []expectedTransition{
		{pressed: true, sentAt: now},
		{pressed: false, sentAt: now},
	}

	t.Run("ordered", func(t *testing.T) {
		results := newTransitionResults(2)
		next := classifyObservation(expected, 0,
			triggerObservation{axis: 32767, at: now.Add(time.Millisecond)}, &results)
		next = classifyObservation(expected, next,
			triggerObservation{axis: 0, released: true, at: now.Add(2 * time.Millisecond)}, &results)
		if next != 2 || results.delivered != 2 || results.missed != 0 ||
			results.reordered != 0 || results.falseRepress != 0 {
			t.Fatalf("unexpected ordered results: next=%d results=%+v", next, results)
		}
	})

	t.Run("release-before-press", func(t *testing.T) {
		results := newTransitionResults(2)
		next := classifyObservation(expected, 0,
			triggerObservation{axis: 0, released: true, at: now.Add(time.Millisecond)}, &results)
		if next != 2 || results.delivered != 1 || results.missed != 1 ||
			results.reordered != 1 || results.falseRepress != 0 {
			t.Fatalf("unexpected reordered results: next=%d results=%+v", next, results)
		}
	})

	t.Run("false-repress", func(t *testing.T) {
		results := newTransitionResults(2)
		next := classifyObservation(expected, 0,
			triggerObservation{axis: 32767, at: now.Add(time.Millisecond)}, &results)
		next = classifyObservation(expected, next,
			triggerObservation{axis: 32767, at: now.Add(2 * time.Millisecond)}, &results)
		if next != 1 || results.falseRepress != 1 || results.delivered != 1 {
			t.Fatalf("unexpected false-repress results: next=%d results=%+v", next, results)
		}
	})
}

func TestClassifyObservationRequiresPeakBeforeRelease(t *testing.T) {
	now := time.Now()
	expected := []expectedTransition{
		{pressed: true, sentAt: now},
		{pressed: false, sentAt: now},
	}
	results := newTransitionResults(2)
	next := classifyObservation(expected, 0,
		triggerObservation{axis: -12000, at: now.Add(time.Millisecond)}, &results)
	requirePeak := next == 0 && results.delivered == 0 && results.peakAxis == -12000
	if !requirePeak {
		t.Fatalf("intermediate trigger value incorrectly counted as peak: next=%d results=%+v",
			next, results)
	}
	next = classifyObservation(expected, next,
		triggerObservation{axis: 32767, at: now.Add(2 * time.Millisecond)}, &results)
	next = classifyObservation(expected, next,
		triggerObservation{axis: 0, released: true, at: now.Add(3 * time.Millisecond)}, &results)
	if next != 2 || results.delivered != 2 || results.peakAxis != 32767 {
		t.Fatalf("peak/release sequence not delivered: next=%d results=%+v", next, results)
	}
}

func TestTransitionClassificationAllocatesZero(t *testing.T) {
	now := time.Unix(1, 0)
	expected := [2]expectedTransition{
		{pressed: true, sentAt: now},
		{pressed: false, sentAt: now},
	}
	results := newTransitionResults(2)
	allocations := testing.AllocsPerRun(1000, func() {
		results.delivered = 0
		results.missed = 0
		results.reordered = 0
		results.falseRepress = 0
		results.latencyCount = 0
		results.peakAxis = -32768
		next := classifyObservation(expected[:], 0,
			triggerObservation{axis: 32767, at: now}, &results)
		_ = classifyObservation(expected[:], next,
			triggerObservation{axis: 0, released: true, at: now}, &results)
	})
	if allocations != 0 {
		t.Fatalf("transition classification allocated %.2f times per epoch", allocations)
	}
}

func TestSummarizeLatencies(t *testing.T) {
	values := []time.Duration{10, 1, 9, 2, 8, 3, 7, 4, 6, 5}
	got := summarizeLatencies(values)
	if got.count != 10 || got.median != 6 || got.p95 != 10 ||
		got.p99 != 10 || got.p999 != 10 || got.max != 10 {
		t.Fatalf("unexpected distribution: %+v", got)
	}
}

func TestMappedDiagnosticHelpersAcceptOptionalJSONFields(t *testing.T) {
	values := map[string]any{
		"InputTransitionHighWater": float64(3),
		"count":                    json.Number("10000"),
	}
	if got := mapNumber(values, "inputTransitionHighWater"); got != 3 {
		t.Fatalf("case-insensitive number=%v want=3", got)
	}
	if got := mapNumber(values, "Count"); got != 10000 {
		t.Fatalf("json.Number=%v want=10000", got)
	}
	if got := mapLookup(values, "missing"); got != nil {
		t.Fatalf("missing optional field=%v want nil", got)
	}
}

func TestV5FrameWriterBuildsValidatedFrame(t *testing.T) {
	// The framing calculation itself is tested without opening a VIIPER
	// stream: reproduce the fixed header fields and verify the incremental
	// CRC boundary used by the writer.
	var header [v5FrameHeaderSize]byte
	header[0] = v5FrameMagic0
	header[1] = v5FrameMagic1
	header[2] = v5FrameMagic2
	header[3] = v5FrameMagic3
	header[4] = v5FrameVersion
	header[5] = v5FrameInput
	binary.LittleEndian.PutUint16(header[6:8], v5InputStateSize)
	binary.LittleEndian.PutUint32(header[8:12], 7)
	var payload [v5InputStateSize]byte
	crc := crc32.Update(0, crc32.IEEETable, header[4:12])
	crc = crc32.Update(crc, crc32.IEEETable, payload[:])
	binary.LittleEndian.PutUint32(header[12:16], crc)
	if got := crc32.Update(crc32.Update(0, crc32.IEEETable, header[4:12]),
		crc32.IEEETable, payload[:]); got != binary.LittleEndian.Uint32(header[12:16]) {
		t.Fatalf("frame CRC mismatch: got %08x want %08x", got,
			binary.LittleEndian.Uint32(header[12:16]))
	}
}
