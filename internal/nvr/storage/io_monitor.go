package storage

import (
	"sync"
	"time"
)

// IOState represents the health state of a storage path's I/O performance.
type IOState string

const (
	IOStateHealthy  IOState = "healthy"
	IOStateSlow     IOState = "slow"
	IOStateCritical IOState = "critical"
)

const ioRingSize = 360 // 3 hours at 30s intervals

const slidingWindowSize = 5

// IOSample holds a single I/O benchmark measurement.
type IOSample struct {
	Timestamp    time.Time `json:"timestamp"`
	LatencyMs    float64   `json:"latency_ms"`
	ThroughputMB float64   `json:"throughput_mbps"`
}

// PathIOMetrics stores a ring buffer of IOSamples for a single storage path
// and evaluates I/O performance against configurable thresholds.
type PathIOMetrics struct {
	mu      sync.RWMutex
	samples [ioRingSize]IOSample
	pos     int
	count   int
	State   IOState
	WarnMs  float64
	CritMs  float64
}

// NewPathIOMetrics creates a PathIOMetrics with the given thresholds.
func NewPathIOMetrics(warnMs, critMs float64) *PathIOMetrics {
	return &PathIOMetrics{
		State:  IOStateHealthy,
		WarnMs: warnMs,
		CritMs: critMs,
	}
}

// Add appends a sample to the ring buffer.
func (m *PathIOMetrics) Add(s IOSample) {
	m.mu.Lock()
	m.samples[m.pos] = s
	m.pos = (m.pos + 1) % ioRingSize
	if m.count < ioRingSize {
		m.count++
	}
	m.mu.Unlock()
}

// Latest returns the most recent sample, or a zero IOSample if empty.
func (m *PathIOMetrics) Latest() IOSample {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.count == 0 {
		return IOSample{}
	}
	last := (m.pos - 1 + ioRingSize) % ioRingSize
	return m.samples[last]
}

// History returns all stored samples in chronological order (oldest first).
func (m *PathIOMetrics) History() []IOSample {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.count == 0 {
		return []IOSample{}
	}
	out := make([]IOSample, m.count)
	if m.count < ioRingSize {
		copy(out, m.samples[:m.count])
	} else {
		n := copy(out, m.samples[m.pos:])
		copy(out[n:], m.samples[:m.pos])
	}
	return out
}

// Evaluate computes the average latency over the last slidingWindowSize samples
// and updates the IOState. Returns (previousState, newState).
func (m *PathIOMetrics) Evaluate() (IOState, IOState) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.count == 0 {
		return m.State, m.State
	}

	n := slidingWindowSize
	if m.count < n {
		n = m.count
	}

	var sum float64
	for i := 0; i < n; i++ {
		idx := (m.pos - 1 - i + ioRingSize) % ioRingSize
		sum += m.samples[idx].LatencyMs
	}
	avg := sum / float64(n)

	prev := m.State
	switch {
	case avg >= m.CritMs:
		m.State = IOStateCritical
	case avg >= m.WarnMs:
		m.State = IOStateSlow
	default:
		m.State = IOStateHealthy
	}
	return prev, m.State
}

// GetState returns the current IOState.
func (m *PathIOMetrics) GetState() IOState {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.State
}

// GetThresholds returns the current warn and critical thresholds.
func (m *PathIOMetrics) GetThresholds() (float64, float64) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.WarnMs, m.CritMs
}

// SetThresholds updates the warn and critical thresholds.
func (m *PathIOMetrics) SetThresholds(warnMs, critMs float64) {
	m.mu.Lock()
	m.WarnMs = warnMs
	m.CritMs = critMs
	m.mu.Unlock()
}
