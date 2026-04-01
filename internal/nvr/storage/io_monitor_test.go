package storage

import (
	"testing"
	"time"
)

func TestPathIOMetrics_AddAndLatest(t *testing.T) {
	m := NewPathIOMetrics(50, 200)

	// No samples yet — Latest returns zero.
	s := m.Latest()
	if s.LatencyMs != 0 {
		t.Fatalf("expected zero latency, got %f", s.LatencyMs)
	}

	m.Add(IOSample{
		Timestamp:    time.Now(),
		LatencyMs:    12.5,
		ThroughputMB: 80.0,
	})

	s = m.Latest()
	if s.LatencyMs != 12.5 {
		t.Fatalf("expected 12.5, got %f", s.LatencyMs)
	}
}

func TestPathIOMetrics_HistoryOrder(t *testing.T) {
	m := NewPathIOMetrics(50, 200)

	for i := 0; i < 5; i++ {
		m.Add(IOSample{
			Timestamp:    time.Now(),
			LatencyMs:    float64(i + 1),
			ThroughputMB: 80.0,
		})
	}

	h := m.History()
	if len(h) != 5 {
		t.Fatalf("expected 5 samples, got %d", len(h))
	}
	// Oldest first.
	if h[0].LatencyMs != 1.0 {
		t.Fatalf("expected oldest=1.0, got %f", h[0].LatencyMs)
	}
	if h[4].LatencyMs != 5.0 {
		t.Fatalf("expected newest=5.0, got %f", h[4].LatencyMs)
	}
}

func TestPathIOMetrics_RingBufferWrap(t *testing.T) {
	m := NewPathIOMetrics(50, 200)

	// Fill beyond ring buffer capacity (360).
	for i := 0; i < 365; i++ {
		m.Add(IOSample{
			Timestamp:    time.Now(),
			LatencyMs:    float64(i),
			ThroughputMB: 80.0,
		})
	}

	h := m.History()
	if len(h) != 360 {
		t.Fatalf("expected 360 samples, got %d", len(h))
	}
	// Oldest should be sample 5 (indices 0-4 evicted).
	if h[0].LatencyMs != 5.0 {
		t.Fatalf("expected oldest=5.0, got %f", h[0].LatencyMs)
	}
	if h[359].LatencyMs != 364.0 {
		t.Fatalf("expected newest=364.0, got %f", h[359].LatencyMs)
	}
}
