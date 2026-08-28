package dualsense

import (
	"sync/atomic"
	"time"
)

var inputStageBucketLimits = [...]time.Duration{
	5 * time.Microsecond,
	10 * time.Microsecond,
	25 * time.Microsecond,
	50 * time.Microsecond,
	100 * time.Microsecond,
	250 * time.Microsecond,
	500 * time.Microsecond,
	time.Millisecond,
	2 * time.Millisecond,
	4 * time.Millisecond,
	8 * time.Millisecond,
	16 * time.Millisecond,
	32 * time.Millisecond,
	64 * time.Millisecond,
}

// LatencyDistribution is an aggregate fixed-bucket distribution. Percentiles
// are conservative bucket upper bounds; Maximum is the exact observed maximum.
// P99.9 remains zero until at least 1,000 samples have been collected.
type LatencyDistribution struct {
	Count   uint64
	P50     time.Duration
	P95     time.Duration
	P99     time.Duration
	P999    time.Duration
	Maximum time.Duration
}

type inputStageHistogram struct {
	buckets [len(inputStageBucketLimits) + 1]atomic.Uint64
	maximum atomic.Int64
}

func (h *inputStageHistogram) record(duration time.Duration) {
	if duration < 0 {
		duration = 0
	}
	bucket := len(inputStageBucketLimits)
	for index, limit := range inputStageBucketLimits {
		if duration <= limit {
			bucket = index
			break
		}
	}
	h.buckets[bucket].Add(1)
	recordMaximumInt64(&h.maximum, int64(duration))
}

func (h *inputStageHistogram) snapshot() LatencyDistribution {
	var buckets [len(inputStageBucketLimits) + 1]uint64
	var count uint64
	for index := range h.buckets {
		buckets[index] = h.buckets[index].Load()
		count += buckets[index]
	}
	maximum := time.Duration(h.maximum.Load())
	return latencyDistributionFromBuckets(
		buckets[:], inputStageBucketLimits[:], maximum, count,
	)
}

type dualSenseInputTransportTelemetry struct {
	frameReadToDecode inputStageHistogram
	decodeToPublish   inputStageHistogram
	framesValidated   atomic.Uint64
	inputFrames       atomic.Uint64
	lastFrameSequence atomic.Uint32
}

// InputTelemetrySnapshot contains only process-local monotonic stage
// distributions. It deliberately does not combine clocks across DS4Windows
// and VIIPER; end-to-end latency remains the consumer harness's responsibility.
type InputTelemetrySnapshot struct {
	Enabled             bool
	FramesValidated     uint64
	InputFrames         uint64
	LastFrameSequence   uint32
	FrameReadToDecode   LatencyDistribution
	DecodeToPublication LatencyDistribution
	ReceiveToSelected   LatencyDistribution
	ReceiveToPresented  LatencyDistribution
}

// SetInputTelemetryEnabled enables aggregate recording without per-report
// logging. Existing samples remain available when recording is disabled.
func (d *DualSense) SetInputTelemetryEnabled(enabled bool) {
	d.inputTelemetryEnabled.Store(enabled)
}

func (d *DualSense) InputTelemetryState() InputTelemetrySnapshot {
	input := d.input.snapshot()
	queueCount := uint64(0)
	for _, count := range input.QueueAgeBuckets {
		queueCount += count
	}
	queue := latencyDistributionFromBuckets(
		input.QueueAgeBuckets[:], inputQueueAgeBucketLimits[:],
		input.MaximumQueueAge, queueCount,
	)
	selectionCount := uint64(0)
	for _, count := range input.SelectionAgeBuckets {
		selectionCount += count
	}
	selection := latencyDistributionFromBuckets(
		input.SelectionAgeBuckets[:], inputQueueAgeBucketLimits[:],
		input.MaximumSelectionAge, selectionCount,
	)
	return InputTelemetrySnapshot{
		Enabled:             d.inputTelemetryEnabled.Load(),
		FramesValidated:     d.inputTransportTelemetry.framesValidated.Load(),
		InputFrames:         d.inputTransportTelemetry.inputFrames.Load(),
		LastFrameSequence:   d.inputTransportTelemetry.lastFrameSequence.Load(),
		FrameReadToDecode:   d.inputTransportTelemetry.frameReadToDecode.snapshot(),
		DecodeToPublication: d.inputTransportTelemetry.decodeToPublish.snapshot(),
		ReceiveToSelected:   selection,
		ReceiveToPresented:  queue,
	}
}

func latencyDistributionFromBuckets(buckets []uint64,
	limits []time.Duration, maximum time.Duration,
	count uint64) LatencyDistribution {
	result := LatencyDistribution{Count: count, Maximum: maximum}
	if count == 0 {
		return result
	}
	result.P50 = percentileFromBuckets(buckets, limits, maximum, count, 500)
	result.P95 = percentileFromBuckets(buckets, limits, maximum, count, 950)
	result.P99 = percentileFromBuckets(buckets, limits, maximum, count, 990)
	if count >= 1000 {
		result.P999 = percentileFromBuckets(buckets, limits, maximum, count, 999)
	}
	return result
}

func percentileFromBuckets(buckets []uint64, limits []time.Duration,
	maximum time.Duration, count uint64, perThousand uint64) time.Duration {
	target := (count*perThousand + 999) / 1000
	var cumulative uint64
	for index, bucketCount := range buckets {
		cumulative += bucketCount
		if cumulative < target {
			continue
		}
		if index < len(limits) {
			return limits[index]
		}
		return maximum
	}
	return maximum
}
