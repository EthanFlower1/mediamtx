// internal/nvr/ai/scaler_test.go
package ai

import (
	"testing"
	"time"
)

// fakeSampler returns fixed CPU/mem values that can be changed between samples.
type fakeSampler struct {
	cpu float64
	mem float64
}

func (f *fakeSampler) Sample() (float64, float64) {
	return f.cpu, f.mem
}

func TestScalerShouldProcess_NoSkip(t *testing.T) {
	sampler := &fakeSampler{cpu: 10, mem: 10}
	s := NewScaler("test-cam", DefaultScalerConfig(), sampler)
	// Default skipFactor = 1, every frame should be processed.
	for i := 0; i < 10; i++ {
		if !s.ShouldProcess() {
			t.Fatalf("frame %d: expected ShouldProcess=true with skip=1", i)
		}
	}
}

func TestScalerShouldProcess_SkipFactor2(t *testing.T) {
	sampler := &fakeSampler{cpu: 10, mem: 10}
	s := NewScaler("test-cam", DefaultScalerConfig(), sampler)
	// Manually set skipFactor to 2.
	s.mu.Lock()
	s.skipFactor = 2
	s.mu.Unlock()

	results := make([]bool, 8)
	for i := 0; i < 8; i++ {
		results[i] = s.ShouldProcess()
	}
	// With skip=2, every 2nd frame is processed.
	// frameCount goes 1,2,3,4,5,6,7,8 -> mod 2 == 0 at 2,4,6,8
	processed := 0
	for _, r := range results {
		if r {
			processed++
		}
	}
	if processed != 4 {
		t.Fatalf("expected 4 processed out of 8 frames (skip=2), got %d: %v", processed, results)
	}
}

func TestScalerAdjust_ScalesUpOnHighCPU(t *testing.T) {
	sampler := &fakeSampler{cpu: 85, mem: 30}
	s := NewScaler("test-cam", DefaultScalerConfig(), sampler)

	if s.SkipFactor() != 1 {
		t.Fatalf("initial skip should be 1, got %d", s.SkipFactor())
	}

	s.adjust()
	if s.SkipFactor() != 2 {
		t.Fatalf("after high CPU adjust, skip should be 2, got %d", s.SkipFactor())
	}

	s.adjust()
	if s.SkipFactor() != 4 {
		t.Fatalf("after second high CPU adjust, skip should be 4, got %d", s.SkipFactor())
	}
}

func TestScalerAdjust_ScalesUpOnHighMem(t *testing.T) {
	sampler := &fakeSampler{cpu: 30, mem: 85}
	s := NewScaler("test-cam", DefaultScalerConfig(), sampler)

	s.adjust()
	if s.SkipFactor() != 2 {
		t.Fatalf("after high mem adjust, skip should be 2, got %d", s.SkipFactor())
	}
}

func TestScalerAdjust_ScalesDownOnLowLoad(t *testing.T) {
	sampler := &fakeSampler{cpu: 85, mem: 30}
	s := NewScaler("test-cam", DefaultScalerConfig(), sampler)

	// Scale up twice: 1 -> 2 -> 4
	s.adjust()
	s.adjust()
	if s.SkipFactor() != 4 {
		t.Fatalf("expected skip=4 after scale-up, got %d", s.SkipFactor())
	}

	// Drop load.
	sampler.cpu = 20
	sampler.mem = 20
	s.adjust()
	if s.SkipFactor() != 2 {
		t.Fatalf("expected skip=2 after scale-down, got %d", s.SkipFactor())
	}

	s.adjust()
	if s.SkipFactor() != 1 {
		t.Fatalf("expected skip=1 after second scale-down, got %d", s.SkipFactor())
	}
}

func TestScalerAdjust_CapsAtMaxSkipFactor(t *testing.T) {
	sampler := &fakeSampler{cpu: 95, mem: 95}
	config := DefaultScalerConfig()
	config.MaxSkipFactor = 4
	s := NewScaler("test-cam", config, sampler)

	for i := 0; i < 10; i++ {
		s.adjust()
	}
	if s.SkipFactor() != 4 {
		t.Fatalf("expected skip capped at 4, got %d", s.SkipFactor())
	}
}

func TestScalerAdjust_NeutralZoneNoChange(t *testing.T) {
	// Load between low and high thresholds: skip should stay constant.
	sampler := &fakeSampler{cpu: 65, mem: 65}
	s := NewScaler("test-cam", DefaultScalerConfig(), sampler)

	// Start at skip=1.
	s.adjust()
	if s.SkipFactor() != 1 {
		t.Fatalf("neutral zone should not change skip from 1, got %d", s.SkipFactor())
	}

	// Manually set to 2, neutral should leave it.
	s.mu.Lock()
	s.skipFactor = 2
	s.mu.Unlock()

	s.adjust()
	if s.SkipFactor() != 2 {
		t.Fatalf("neutral zone should not change skip from 2, got %d", s.SkipFactor())
	}
}

func TestScalerStartStop(t *testing.T) {
	sampler := &fakeSampler{cpu: 10, mem: 10}
	config := DefaultScalerConfig()
	config.PollIntervalSecs = 1
	s := NewScaler("test-cam", config, sampler)

	s.Start()
	time.Sleep(50 * time.Millisecond)
	s.Stop() // should not hang
}
