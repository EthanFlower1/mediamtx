package notifications

import (
	"sync"
	"sync/atomic"
	"time"
)

// MetricsCollector tracks per-channel delivery metrics. It uses atomic
// counters so it can be read by a Prometheus scraper without locks.
//
// Channel adapters call the Record* methods after each delivery
// attempt. The Dispatcher calls RecordDLQ when a message exhausts
// retries.
//
// Label cardinality: channel x message_type — bounded by design
// (channels are a small, known set).
type MetricsCollector struct {
	mu      sync.RWMutex
	buckets map[string]*channelMetrics // keyed by channel name
}

type channelMetrics struct {
	Sent    atomic.Int64
	Failed  atomic.Int64
	Retried atomic.Int64
	DLQ     atomic.Int64

	// latencyMu guards latencies. We keep the last 100 samples for
	// a simple percentile approximation. A proper histogram will
	// come with prometheus/client_golang integration (KAI-421).
	latencyMu sync.Mutex
	latencies []time.Duration
}

// NewMetricsCollector creates an empty collector.
func NewMetricsCollector() *MetricsCollector {
	return &MetricsCollector{buckets: make(map[string]*channelMetrics)}
}

func (mc *MetricsCollector) bucket(channel string) *channelMetrics {
	mc.mu.RLock()
	b, ok := mc.buckets[channel]
	mc.mu.RUnlock()
	if ok {
		return b
	}
	mc.mu.Lock()
	defer mc.mu.Unlock()
	// Double-check after write lock.
	if b, ok = mc.buckets[channel]; ok {
		return b
	}
	b = &channelMetrics{latencies: make([]time.Duration, 0, 100)}
	mc.buckets[channel] = b
	return b
}

// RecordSent increments the sent counter for a channel.
func (mc *MetricsCollector) RecordSent(channel string, count int64) {
	mc.bucket(channel).Sent.Add(count)
}

// RecordFailed increments the failed counter for a channel.
func (mc *MetricsCollector) RecordFailed(channel string, count int64) {
	mc.bucket(channel).Failed.Add(count)
}

// RecordRetried increments the retried counter for a channel.
func (mc *MetricsCollector) RecordRetried(channel string, count int64) {
	mc.bucket(channel).Retried.Add(count)
}

// RecordDLQ increments the dead-letter counter for a channel.
func (mc *MetricsCollector) RecordDLQ(channel string, count int64) {
	mc.bucket(channel).DLQ.Add(count)
}

// RecordLatency records a delivery latency sample.
func (mc *MetricsCollector) RecordLatency(channel string, d time.Duration) {
	b := mc.bucket(channel)
	b.latencyMu.Lock()
	defer b.latencyMu.Unlock()
	if len(b.latencies) >= 100 {
		// Ring buffer: overwrite oldest.
		copy(b.latencies, b.latencies[1:])
		b.latencies[99] = d
	} else {
		b.latencies = append(b.latencies, d)
	}
}

// ChannelSnapshot is a point-in-time view of one channel's counters.
type ChannelSnapshot struct {
	Channel string `json:"channel"`
	Sent    int64  `json:"sent"`
	Failed  int64  `json:"failed"`
	Retried int64  `json:"retried"`
	DLQ     int64  `json:"dlq"`
}

// Snapshot returns all channel metrics as a slice.
func (mc *MetricsCollector) Snapshot() []ChannelSnapshot {
	mc.mu.RLock()
	defer mc.mu.RUnlock()
	out := make([]ChannelSnapshot, 0, len(mc.buckets))
	for name, b := range mc.buckets {
		out = append(out, ChannelSnapshot{
			Channel: name,
			Sent:    b.Sent.Load(),
			Failed:  b.Failed.Load(),
			Retried: b.Retried.Load(),
			DLQ:     b.DLQ.Load(),
		})
	}
	return out
}

// PrometheusTextFormat emits the metrics in Prometheus text exposition
// format. This is a stopgap until prometheus/client_golang is fully
// wired (KAI-421). It follows the same pattern as apiserver/metrics.go.
func (mc *MetricsCollector) PrometheusTextFormat() string {
	mc.mu.RLock()
	defer mc.mu.RUnlock()

	var buf []byte
	appendCounter := func(name, help, channel string, val int64) {
		buf = append(buf, "# HELP "+name+" "+help+"\n"...)
		buf = append(buf, "# TYPE "+name+" counter\n"...)
		buf = append(buf, name+`{channel="`+channel+`"} `...)
		buf = append(buf, intToBytes(val)...)
		buf = append(buf, '\n')
	}

	for name, b := range mc.buckets {
		appendCounter("kaivue_notifications_sent_total",
			"Total notifications sent successfully.", name, b.Sent.Load())
		appendCounter("kaivue_notifications_failed_total",
			"Total notification delivery failures.", name, b.Failed.Load())
		appendCounter("kaivue_notifications_retried_total",
			"Total notification delivery retries.", name, b.Retried.Load())
		appendCounter("kaivue_notifications_dlq_total",
			"Total notifications moved to dead-letter queue.", name, b.DLQ.Load())
	}
	return string(buf)
}

// intToBytes converts an int64 to decimal ASCII bytes.
func intToBytes(n int64) []byte {
	if n == 0 {
		return []byte("0")
	}
	var buf [20]byte
	i := len(buf)
	neg := n < 0
	if neg {
		n = -n
	}
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return buf[i:]
}
