package usb

import (
	"sync/atomic"
	"time"
)

var durationHistogramBounds = [...]time.Duration{
	10 * time.Microsecond,
	25 * time.Microsecond,
	50 * time.Microsecond,
	100 * time.Microsecond,
	250 * time.Microsecond,
	500 * time.Microsecond,
	time.Millisecond,
	2 * time.Millisecond,
	5 * time.Millisecond,
	10 * time.Millisecond,
	25 * time.Millisecond,
	50 * time.Millisecond,
	100 * time.Millisecond,
}

type durationHistogram struct {
	buckets [len(durationHistogramBounds) + 1]atomic.Uint64
	count   atomic.Uint64
	total   atomic.Uint64
	maximum atomic.Uint64
}

func (h *durationHistogram) record(value time.Duration) {
	if value < 0 {
		value = 0
	}
	nanos := uint64(value)
	h.count.Add(1)
	h.total.Add(nanos)
	for {
		old := h.maximum.Load()
		if nanos <= old || h.maximum.CompareAndSwap(old, nanos) {
			break
		}
	}
	index := len(durationHistogramBounds)
	for candidate, bound := range durationHistogramBounds {
		if value <= bound {
			index = candidate
			break
		}
	}
	h.buckets[index].Add(1)
}

// DurationHistogramSnapshot is a lock-free aggregate view of one hot-path
// histogram. Percentiles are the upper bound of the fixed bucket containing
// the requested rank; Max retains the exact observed maximum.
type DurationHistogramSnapshot struct {
	Count  uint64
	Median time.Duration
	P95    time.Duration
	P99    time.Duration
	P999   time.Duration
	Max    time.Duration
}

func (h *durationHistogram) snapshot() DurationHistogramSnapshot {
	count := h.count.Load()
	return DurationHistogramSnapshot{
		Count:  count,
		Median: h.percentile(count, 500),
		P95:    h.percentile(count, 950),
		P99:    h.percentile(count, 990),
		P999:   h.percentile(count, 999),
		Max:    time.Duration(h.maximum.Load()),
	}
}

// percentile accepts a per-mille target so p99.9 remains integer-only on the
// hot diagnostics path. It returns the fixed bucket's upper bound; the final
// open bucket is represented by the observed maximum.
func (h *durationHistogram) percentile(count, perMille uint64) time.Duration {
	if count == 0 {
		return 0
	}
	target := (count*perMille + 999) / 1000
	var cumulative uint64
	for index := range h.buckets {
		cumulative += h.buckets[index].Load()
		if cumulative < target {
			continue
		}
		if index == len(durationHistogramBounds) {
			return time.Duration(h.maximum.Load())
		}
		return durationHistogramBounds[index]
	}
	return time.Duration(h.maximum.Load())
}
