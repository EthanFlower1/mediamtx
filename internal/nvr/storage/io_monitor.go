package storage

import (
	"fmt"
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

// PathStatus holds the current state and metrics for a single storage path.
type PathStatus struct {
	State   IOState    `json:"state"`
	Latest  IOSample   `json:"latest"`
	WarnMs  float64    `json:"warn_ms"`
	CritMs  float64    `json:"critical_ms"`
	History []IOSample `json:"history"`
}

// IOMonitor manages per-path IOMetrics and coordinates recording and evaluation.
type IOMonitor struct {
	mu            sync.RWMutex
	paths         map[string]*PathIOMetrics
	defaultWarnMs float64
	defaultCritMs float64
}

// NewIOMonitor creates an IOMonitor with default thresholds for new paths.
func NewIOMonitor(defaultWarnMs, defaultCritMs float64) *IOMonitor {
	return &IOMonitor{
		paths:         make(map[string]*PathIOMetrics),
		defaultWarnMs: defaultWarnMs,
		defaultCritMs: defaultCritMs,
	}
}

// Record adds a sample for the given path (creating the PathIOMetrics if needed)
// and evaluates thresholds. Returns (previousState, newState).
func (m *IOMonitor) Record(path string, sample IOSample) (IOState, IOState) {
	m.mu.Lock()
	pm, ok := m.paths[path]
	if !ok {
		pm = NewPathIOMetrics(m.defaultWarnMs, m.defaultCritMs)
		m.paths[path] = pm
	}
	m.mu.Unlock()

	pm.Add(sample)
	return pm.Evaluate()
}

// GetStatus returns the current status of all tracked paths.
func (m *IOMonitor) GetStatus() map[string]PathStatus {
	m.mu.RLock()
	defer m.mu.RUnlock()

	result := make(map[string]PathStatus, len(m.paths))
	for path, pm := range m.paths {
		warnMs, critMs := pm.GetThresholds()
		result[path] = PathStatus{
			State:   pm.GetState(),
			Latest:  pm.Latest(),
			WarnMs:  warnMs,
			CritMs:  critMs,
			History: pm.History(),
		}
	}
	return result
}

// UpdateThresholds changes the warn and critical thresholds for a specific path.
func (m *IOMonitor) UpdateThresholds(path string, warnMs, critMs float64) error {
	m.mu.RLock()
	pm, ok := m.paths[path]
	m.mu.RUnlock()
	if !ok {
		return fmt.Errorf("path %q not found", path)
	}
	pm.SetThresholds(warnMs, critMs)
	return nil
}
