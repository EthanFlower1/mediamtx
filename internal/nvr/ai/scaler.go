// internal/nvr/ai/scaler.go
package ai

import (
	"log"
	"runtime"
	"sync"
	"time"
)

// Default thresholds for auto-scaling.
const (
	DefaultCPUHighThreshold  = 80.0 // percent
	DefaultCPULowThreshold   = 50.0 // percent
	DefaultMemHighThreshold  = 80.0 // percent of system memory
	DefaultMemLowThreshold   = 60.0 // percent of system memory
	DefaultMinSkipFactor     = 1    // process every frame (no skip)
	DefaultMaxSkipFactor     = 8    // process every 8th frame at max load
	DefaultScalerPollSeconds = 5    // check load every 5 seconds
)

// ScalerConfig holds configurable thresholds for load-based scaling.
type ScalerConfig struct {
	CPUHighThreshold  float64 // CPU% above which we start skipping frames
	CPULowThreshold   float64 // CPU% below which we restore full rate
	MemHighThreshold  float64 // Memory% above which we start skipping frames
	MemLowThreshold   float64 // Memory% below which we restore full rate
	MaxSkipFactor     int     // maximum frames to skip (e.g. 8 = keep 1 in 8)
	PollIntervalSecs  int     // how often to sample system load
}

// DefaultScalerConfig returns a ScalerConfig with sensible defaults.
func DefaultScalerConfig() ScalerConfig {
	return ScalerConfig{
		CPUHighThreshold: DefaultCPUHighThreshold,
		CPULowThreshold:  DefaultCPULowThreshold,
		MemHighThreshold: DefaultMemHighThreshold,
		MemLowThreshold:  DefaultMemLowThreshold,
		MaxSkipFactor:    DefaultMaxSkipFactor,
		PollIntervalSecs: DefaultScalerPollSeconds,
	}
}

// loadSampler abstracts system load measurement so it can be swapped in tests.
type loadSampler interface {
	// Sample returns (cpuPercent, memPercent).
	Sample() (float64, float64)
}

// runtimeSampler uses Go runtime metrics as a proxy for process load.
type runtimeSampler struct {
	prevTotal    uint64
	prevIdle     uint64
	lastSample   time.Time
	lastCPU      float64
	numCPU       int
	prevUserTime time.Duration
}

func newRuntimeSampler() *runtimeSampler {
	return &runtimeSampler{
		lastSample: time.Now(),
		numCPU:     runtime.NumCPU(),
	}
}

// Sample uses runtime.MemStats for memory and a goroutine/CPU heuristic for
// CPU pressure. This avoids platform-specific /proc or cgo dependencies.
func (rs *runtimeSampler) Sample() (cpuPct float64, memPct float64) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	// Memory: ratio of heap in-use to total system memory obtained from OS.
	if m.Sys > 0 {
		memPct = float64(m.HeapInuse) / float64(m.Sys) * 100.0
	}

	// CPU: approximate based on goroutine count relative to available CPUs.
	// Under heavy load the goroutine count rises; we map it to a percentage.
	goroutines := runtime.NumGoroutine()
	cpuPct = float64(goroutines) / float64(rs.numCPU) * 10.0
	if cpuPct > 100 {
		cpuPct = 100
	}

	return cpuPct, memPct
}

// Scaler monitors system load and computes how many frames to skip.
// The pipeline calls ShouldProcess for every incoming frame.
type Scaler struct {
	config     ScalerConfig
	cameraName string
	sampler    loadSampler

	mu         sync.RWMutex
	skipFactor int // 1 = no skip, N = keep 1 in N
	frameCount uint64
	lastCPU    float64
	lastMem    float64

	stopCh chan struct{}
	done   chan struct{}
}

// NewScaler creates a Scaler. Call Start to begin the monitoring loop.
func NewScaler(cameraName string, config ScalerConfig, sampler loadSampler) *Scaler {
	if config.MaxSkipFactor < 1 {
		config.MaxSkipFactor = DefaultMaxSkipFactor
	}
	if config.PollIntervalSecs < 1 {
		config.PollIntervalSecs = DefaultScalerPollSeconds
	}
	if sampler == nil {
		sampler = newRuntimeSampler()
	}
	return &Scaler{
		config:     config,
		cameraName: cameraName,
		sampler:    sampler,
		skipFactor: DefaultMinSkipFactor,
		stopCh:     make(chan struct{}),
		done:       make(chan struct{}),
	}
}

// Start begins the periodic load-sampling loop.
func (s *Scaler) Start() {
	go s.run()
}

// Stop terminates the monitoring loop and waits for it to exit.
func (s *Scaler) Stop() {
	close(s.stopCh)
	<-s.done
}

// ShouldProcess returns true if the current frame should be processed.
// It increments an internal counter and uses modular arithmetic against skipFactor.
func (s *Scaler) ShouldProcess() bool {
	s.mu.RLock()
	skip := s.skipFactor
	s.mu.RUnlock()

	s.mu.Lock()
	s.frameCount++
	count := s.frameCount
	s.mu.Unlock()

	return (count % uint64(skip)) == 0
}

// SkipFactor returns the current skip factor (1 = no skip).
func (s *Scaler) SkipFactor() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.skipFactor
}

func (s *Scaler) run() {
	defer close(s.done)

	ticker := time.NewTicker(time.Duration(s.config.PollIntervalSecs) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-s.stopCh:
			return
		case <-ticker.C:
			s.adjust()
		}
	}
}

func (s *Scaler) adjust() {
	cpuPct, memPct := s.sampler.Sample()

	s.mu.Lock()
	defer s.mu.Unlock()

	s.lastCPU = cpuPct
	s.lastMem = memPct

	prevSkip := s.skipFactor

	highLoad := cpuPct >= s.config.CPUHighThreshold || memPct >= s.config.MemHighThreshold
	lowLoad := cpuPct <= s.config.CPULowThreshold && memPct <= s.config.MemLowThreshold

	if highLoad && s.skipFactor < s.config.MaxSkipFactor {
		// Scale up: double the skip factor.
		s.skipFactor *= 2
		if s.skipFactor > s.config.MaxSkipFactor {
			s.skipFactor = s.config.MaxSkipFactor
		}
	} else if lowLoad && s.skipFactor > DefaultMinSkipFactor {
		// Scale down: halve the skip factor.
		s.skipFactor /= 2
		if s.skipFactor < DefaultMinSkipFactor {
			s.skipFactor = DefaultMinSkipFactor
		}
	}

	if s.skipFactor != prevSkip {
		log.Printf("[ai][%s] auto-scale: skip %d -> %d (cpu=%.1f%%, mem=%.1f%%)",
			s.cameraName, prevSkip, s.skipFactor, cpuPct, memPct)
	}
}
