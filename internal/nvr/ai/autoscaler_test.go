// internal/nvr/ai/autoscaler_test.go
package ai

import (
	"context"
	"testing"
	"time"
)

// fakeLoadReader returns a configurable LoadSnapshot.
type fakeLoadReader struct {
	cpu float64
	mem float64
}

func (f *fakeLoadReader) Read() (LoadSnapshot, error) {
	return LoadSnapshot{
		CPUPercent: f.cpu,
		MemPercent: f.mem,
		Timestamp:  time.Now(),
	}, nil
}

func TestAutoscalerDefaults(t *testing.T) {
	cfg := AutoscaleConfig{Enabled: true}
	cfg = cfg.withDefaults()

	if cfg.CPUHighThreshold != 80 {
		t.Errorf("CPUHighThreshold = %v, want 80", cfg.CPUHighThreshold)
	}
	if cfg.BaseInterval != 500*time.Millisecond {
		t.Errorf("BaseInterval = %v, want 500ms", cfg.BaseInterval)
	}
	if cfg.MaxInterval != 5*time.Second {
		t.Errorf("MaxInterval = %v, want 5s", cfg.MaxInterval)
	}
	if cfg.MinInterval != cfg.BaseInterval {
		t.Errorf("MinInterval = %v, want %v", cfg.MinInterval, cfg.BaseInterval)
	}
}

func TestAutoscalerScalesUpOnHighCPU(t *testing.T) {
	reader := &fakeLoadReader{cpu: 90, mem: 40}
	cfg := AutoscaleConfig{
		Enabled:      true,
		BaseInterval: 500 * time.Millisecond,
		MaxInterval:  4 * time.Second,
	}
	a := newAutoscalerWithReader("test-cam", cfg, reader)

	if a.Interval() != 500*time.Millisecond {
		t.Fatalf("initial interval = %v, want 500ms", a.Interval())
	}

	a.tick()
	if a.Interval() != 1*time.Second {
		t.Errorf("after high CPU tick: interval = %v, want 1s", a.Interval())
	}

	a.tick()
	if a.Interval() != 2*time.Second {
		t.Errorf("after second tick: interval = %v, want 2s", a.Interval())
	}

	a.tick()
	if a.Interval() != 4*time.Second {
		t.Errorf("after third tick: interval = %v, want 4s", a.Interval())
	}

	// Should clamp at max.
	a.tick()
	if a.Interval() != 4*time.Second {
		t.Errorf("after fourth tick: interval = %v, want 4s (max)", a.Interval())
	}
}

func TestAutoscalerScalesUpOnHighMemory(t *testing.T) {
	reader := &fakeLoadReader{cpu: 30, mem: 90}
	cfg := AutoscaleConfig{
		Enabled:      true,
		BaseInterval: 500 * time.Millisecond,
	}
	a := newAutoscalerWithReader("test-cam", cfg, reader)

	a.tick()
	if a.Interval() != 1*time.Second {
		t.Errorf("after high memory tick: interval = %v, want 1s", a.Interval())
	}
}

func TestAutoscalerScalesDownOnLowLoad(t *testing.T) {
	reader := &fakeLoadReader{cpu: 90, mem: 40}
	cfg := AutoscaleConfig{
		Enabled:      true,
		BaseInterval: 500 * time.Millisecond,
		MaxInterval:  4 * time.Second,
	}
	a := newAutoscalerWithReader("test-cam", cfg, reader)

	// Scale up first.
	a.tick()
	a.tick()
	if a.Interval() != 2*time.Second {
		t.Fatalf("after two high ticks: interval = %v, want 2s", a.Interval())
	}

	// Now simulate low load.
	reader.cpu = 30
	reader.mem = 30

	a.tick()
	if a.Interval() != 1*time.Second {
		t.Errorf("after low-load tick: interval = %v, want 1s", a.Interval())
	}

	a.tick()
	if a.Interval() != 500*time.Millisecond {
		t.Errorf("after second low-load tick: interval = %v, want 500ms", a.Interval())
	}

	// Should clamp at min (base).
	a.tick()
	if a.Interval() != 500*time.Millisecond {
		t.Errorf("should clamp at base: interval = %v, want 500ms", a.Interval())
	}
}

func TestAutoscalerHoldsInMiddleZone(t *testing.T) {
	reader := &fakeLoadReader{cpu: 65, mem: 70}
	cfg := AutoscaleConfig{
		Enabled:      true,
		BaseInterval: 500 * time.Millisecond,
	}
	a := newAutoscalerWithReader("test-cam", cfg, reader)

	a.tick()
	if a.Interval() != 500*time.Millisecond {
		t.Errorf("middle-zone tick should not change interval: got %v", a.Interval())
	}
}

func TestAutoscalerRunStopsOnCancel(t *testing.T) {
	reader := &fakeLoadReader{cpu: 30, mem: 30}
	cfg := AutoscaleConfig{
		Enabled:    true,
		PollPeriod: 10 * time.Millisecond,
	}
	a := newAutoscalerWithReader("test-cam", cfg, reader)

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		a.Run(ctx)
		close(done)
	}()

	// Let it run a few ticks.
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("autoscaler did not stop after cancel")
	}
}

func TestAutoscalerLastSnapshot(t *testing.T) {
	reader := &fakeLoadReader{cpu: 42, mem: 67}
	cfg := AutoscaleConfig{Enabled: true}
	a := newAutoscalerWithReader("test-cam", cfg, reader)

	a.tick()
	snap := a.LastSnapshot()
	if snap.CPUPercent != 42 {
		t.Errorf("CPUPercent = %v, want 42", snap.CPUPercent)
	}
	if snap.MemPercent != 67 {
		t.Errorf("MemPercent = %v, want 67", snap.MemPercent)
	}
}
