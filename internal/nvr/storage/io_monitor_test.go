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
	r := m.Evaluate()
	if r.Prev != IOStateHealthy || r.Curr != IOStateHealthy {
		t.Fatalf("expected healthy->healthy, got %s->%s", r.Prev, r.Curr)
	}
}

func TestPathIOMetrics_EvaluateSlow(t *testing.T) {
	m := NewPathIOMetrics(50, 200)
	for i := 0; i < 5; i++ {
		m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 75, ThroughputMB: 13})
	}
	r := m.Evaluate()
	if r.Curr != IOStateSlow {
		t.Fatalf("expected slow, got %s", r.Curr)
	}
	if r.AvgMs != 75.0 {
		t.Fatalf("expected avg 75.0, got %f", r.AvgMs)
	}
}

func TestPathIOMetrics_EvaluateCritical(t *testing.T) {
	m := NewPathIOMetrics(50, 200)
	for i := 0; i < 5; i++ {
		m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 250, ThroughputMB: 4})
	}
	r := m.Evaluate()
	if r.Curr != IOStateCritical {
		t.Fatalf("expected critical, got %s", r.Curr)
	}
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
	r := m.Evaluate()
	if r.Prev != IOStateSlow || r.Curr != IOStateHealthy {
		t.Fatalf("expected slow->healthy, got %s->%s", r.Prev, r.Curr)
	}
}

func TestPathIOMetrics_EvaluateIgnoresSingleSpike(t *testing.T) {
	m := NewPathIOMetrics(50, 200)
	for i := 0; i < 4; i++ {
		m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 10, ThroughputMB: 100})
	}
	m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 300, ThroughputMB: 3})
	r := m.Evaluate()
	if r.Curr != IOStateSlow {
		t.Fatalf("expected slow (single spike diluted), got %s", r.Curr)
	}
}

func TestPathIOMetrics_EvaluateFewerThanWindow(t *testing.T) {
	m := NewPathIOMetrics(50, 200)
	m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 250, ThroughputMB: 4})
	m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 250, ThroughputMB: 4})
	r := m.Evaluate()
	if r.Curr != IOStateCritical {
		t.Fatalf("expected critical with fewer than window samples, got %s", r.Curr)
	}
}

func TestIOMonitor_RecordAndGetStatus(t *testing.T) {
	mon := NewIOMonitor(50, 200)

	mon.Record("/data", IOSample{
		Timestamp:    time.Now(),
		LatencyMs:    12.0,
		ThroughputMB: 83.0,
	})

	status := mon.GetStatus()
	if len(status) != 1 {
		t.Fatalf("expected 1 path, got %d", len(status))
	}
	ps, ok := status["/data"]
	if !ok {
		t.Fatal("expected /data in status")
	}
	if ps.State != IOStateHealthy {
		t.Fatalf("expected healthy, got %s", ps.State)
	}
	if ps.Latest.LatencyMs != 12.0 {
		t.Fatalf("expected 12.0, got %f", ps.Latest.LatencyMs)
	}
}

func TestIOMonitor_UpdateThresholds(t *testing.T) {
	mon := NewIOMonitor(50, 200)
	mon.Record("/data", IOSample{Timestamp: time.Now(), LatencyMs: 10, ThroughputMB: 100})

	err := mon.UpdateThresholds("/data", 75, 300)
	if err != nil {
		t.Fatal(err)
	}

	status := mon.GetStatus()
	if status["/data"].WarnMs != 75 || status["/data"].CritMs != 300 {
		t.Fatalf("thresholds not updated: %+v", status["/data"])
	}
}

func TestIOMonitor_UpdateThresholds_UnknownPath(t *testing.T) {
	mon := NewIOMonitor(50, 200)
	err := mon.UpdateThresholds("/nonexistent", 75, 300)
	if err == nil {
		t.Fatal("expected error for unknown path")
	}
}

func TestBenchmarkPath(t *testing.T) {
	dir := t.TempDir()
	sample, err := BenchmarkPath(dir)
	if err != nil {
		t.Fatal(err)
	}
	if sample.LatencyMs <= 0 {
		t.Fatalf("expected positive latency, got %f", sample.LatencyMs)
	}
	if sample.ThroughputMB <= 0 {
		t.Fatalf("expected positive throughput, got %f", sample.ThroughputMB)
	}
	if sample.Timestamp.IsZero() {
		t.Fatal("expected non-zero timestamp")
	}
}

func TestBenchmarkPath_InvalidDir(t *testing.T) {
	_, err := BenchmarkPath("/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error for invalid dir")
	}
}
