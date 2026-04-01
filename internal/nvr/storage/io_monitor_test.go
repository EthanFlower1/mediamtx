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

func TestPathIOMetrics_EvaluateHealthy(t *testing.T) {
	m := NewPathIOMetrics(50, 200)
	for i := 0; i < 5; i++ {
		m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 10, ThroughputMB: 100})
	}
	prev, curr := m.Evaluate()
	if prev != IOStateHealthy || curr != IOStateHealthy {
		t.Fatalf("expected healthy->healthy, got %s->%s", prev, curr)
	}
}

func TestPathIOMetrics_EvaluateSlow(t *testing.T) {
	m := NewPathIOMetrics(50, 200)
	for i := 0; i < 5; i++ {
		m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 75, ThroughputMB: 13})
	}
	prev, curr := m.Evaluate()
	if curr != IOStateSlow {
		t.Fatalf("expected slow, got %s", curr)
	}
	_ = prev
}

func TestPathIOMetrics_EvaluateCritical(t *testing.T) {
	m := NewPathIOMetrics(50, 200)
	for i := 0; i < 5; i++ {
		m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 250, ThroughputMB: 4})
	}
	prev, curr := m.Evaluate()
	if curr != IOStateCritical {
		t.Fatalf("expected critical, got %s", curr)
	}
	_ = prev
}

func TestPathIOMetrics_EvaluateRecovery(t *testing.T) {
	m := NewPathIOMetrics(50, 200)
	for i := 0; i < 5; i++ {
		m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 75, ThroughputMB: 13})
	}
	m.Evaluate()

	for i := 0; i < 5; i++ {
		m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 10, ThroughputMB: 100})
	}
	prev, curr := m.Evaluate()
	if prev != IOStateSlow || curr != IOStateHealthy {
		t.Fatalf("expected slow->healthy, got %s->%s", prev, curr)
	}
}

func TestPathIOMetrics_EvaluateIgnoresSingleSpike(t *testing.T) {
	m := NewPathIOMetrics(50, 200)
	for i := 0; i < 4; i++ {
		m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 10, ThroughputMB: 100})
	}
	m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 300, ThroughputMB: 3})
	_, curr := m.Evaluate()
	if curr != IOStateSlow {
		t.Fatalf("expected slow (single spike diluted), got %s", curr)
	}
}

func TestPathIOMetrics_EvaluateFewerThanWindow(t *testing.T) {
	m := NewPathIOMetrics(50, 200)
	m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 250, ThroughputMB: 4})
	m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 250, ThroughputMB: 4})
	_, curr := m.Evaluate()
	if curr != IOStateCritical {
		t.Fatalf("expected critical with fewer than window samples, got %s", curr)
	}
}
