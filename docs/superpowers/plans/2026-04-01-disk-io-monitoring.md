# KAI-13: Disk I/O Performance Monitoring Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Track write latency per storage path, detect slow disks via configurable thresholds, and expose metrics via API and SSE events.

**Architecture:** Extend the existing storage manager's 30s health check loop to benchmark I/O by writing a 1MB test block and timing it. Store latency samples in a per-path ring buffer. Evaluate against configurable thresholds using a 5-sample sliding window. Expose via two new API endpoints and SSE events on state transitions.

**Tech Stack:** Go, Gin HTTP framework, existing storage.Manager, existing EventBroadcaster

---

## File Map

| File                                      | Action | Responsibility                                                                 |
| ----------------------------------------- | ------ | ------------------------------------------------------------------------------ |
| `internal/nvr/storage/io_monitor.go`      | Create | IOSample struct, PathIOMetrics ring buffer, IOState type, threshold evaluation |
| `internal/nvr/storage/io_monitor_test.go` | Create | Unit tests for ring buffer, threshold evaluation, state transitions            |
| `internal/nvr/storage/manager.go`         | Modify | Add IOMonitor field, call benchmark in health check, wire up event broadcaster |
| `internal/nvr/api/system.go`              | Modify | Add DiskIO and UpdateDiskIOThresholds handlers to SystemHandler                |
| `internal/nvr/api/router.go`              | Modify | Register two new endpoints under protected system group                        |
| `internal/nvr/api/events.go`              | Modify | Add PublishDiskSlow, PublishDiskCritical, PublishDiskRecovered helpers         |

---

### Task 1: IOSample and PathIOMetrics Ring Buffer

**Files:**

- Create: `internal/nvr/storage/io_monitor.go`
- Create: `internal/nvr/storage/io_monitor_test.go`

- [ ] **Step 1: Write failing test for ring buffer Add and Latest**

```go
// internal/nvr/storage/io_monitor_test.go
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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /path/to/worktree && go test ./internal/nvr/storage/ -run TestPathIOMetrics -v`
Expected: FAIL — `NewPathIOMetrics` and `IOSample` not defined.

- [ ] **Step 3: Implement IOSample, IOState, and PathIOMetrics**

```go
// internal/nvr/storage/io_monitor.go
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /path/to/worktree && go test ./internal/nvr/storage/ -run TestPathIOMetrics -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/storage/io_monitor.go internal/nvr/storage/io_monitor_test.go
git commit -m "feat(storage): add IOSample and PathIOMetrics ring buffer"
```

---

### Task 2: Threshold Evaluation with Sliding Window

**Files:**

- Modify: `internal/nvr/storage/io_monitor.go`
- Modify: `internal/nvr/storage/io_monitor_test.go`

- [ ] **Step 1: Write failing tests for Evaluate**

```go
// Append to internal/nvr/storage/io_monitor_test.go

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
	// Push into slow state.
	for i := 0; i < 5; i++ {
		m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 75, ThroughputMB: 13})
	}
	m.Evaluate()

	// Recover with fast writes.
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
	// 4 fast samples, 1 slow spike.
	for i := 0; i < 4; i++ {
		m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 10, ThroughputMB: 100})
	}
	m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 300, ThroughputMB: 3})
	_, curr := m.Evaluate()
	// Average = (10*4 + 300) / 5 = 68 => slow (above warn=50), but NOT critical.
	if curr != IOStateSlow {
		t.Fatalf("expected slow (single spike diluted), got %s", curr)
	}
}

func TestPathIOMetrics_EvaluateFewerThanWindow(t *testing.T) {
	m := NewPathIOMetrics(50, 200)
	// Only 2 samples — should still evaluate with what we have.
	m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 250, ThroughputMB: 4})
	m.Add(IOSample{Timestamp: time.Now(), LatencyMs: 250, ThroughputMB: 4})
	_, curr := m.Evaluate()
	if curr != IOStateCritical {
		t.Fatalf("expected critical with fewer than window samples, got %s", curr)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /path/to/worktree && go test ./internal/nvr/storage/ -run TestPathIOMetrics_Evaluate -v`
Expected: FAIL — `Evaluate` not defined.

- [ ] **Step 3: Implement Evaluate method**

Add to `internal/nvr/storage/io_monitor.go`:

```go
const slidingWindowSize = 5

// Evaluate computes the average latency over the last slidingWindowSize samples
// and updates the IOState. It returns (previousState, newState) so callers can
// detect transitions and emit events.
func (m *PathIOMetrics) Evaluate() (IOState, IOState) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.count == 0 {
		return m.State, m.State
	}

	// Gather the last N samples (or fewer if not enough yet).
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /path/to/worktree && go test ./internal/nvr/storage/ -run TestPathIOMetrics -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/storage/io_monitor.go internal/nvr/storage/io_monitor_test.go
git commit -m "feat(storage): add threshold evaluation with sliding window"
```

---

### Task 3: IOMonitor Coordinator

**Files:**

- Modify: `internal/nvr/storage/io_monitor.go`
- Modify: `internal/nvr/storage/io_monitor_test.go`

- [ ] **Step 1: Write failing test for IOMonitor**

```go
// Append to internal/nvr/storage/io_monitor_test.go

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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /path/to/worktree && go test ./internal/nvr/storage/ -run TestIOMonitor -v`
Expected: FAIL — `NewIOMonitor` not defined.

- [ ] **Step 3: Implement IOMonitor**

Add to `internal/nvr/storage/io_monitor.go`:

```go
import "fmt"

// PathStatus holds the current state and metrics for a single storage path,
// suitable for JSON serialisation in API responses.
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
// and evaluates thresholds. Returns (previousState, newState) for event emission.
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
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /path/to/worktree && go test ./internal/nvr/storage/ -run TestIOMonitor -v`
Expected: All PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/storage/io_monitor.go internal/nvr/storage/io_monitor_test.go
git commit -m "feat(storage): add IOMonitor coordinator for per-path metrics"
```

---

### Task 4: Wire IO Benchmark into Storage Manager Health Check

**Files:**

- Modify: `internal/nvr/storage/manager.go`
- Modify: `internal/nvr/storage/io_monitor.go` (add BenchmarkPath function)

- [ ] **Step 1: Write failing test for BenchmarkPath**

```go
// Append to internal/nvr/storage/io_monitor_test.go

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
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /path/to/worktree && go test ./internal/nvr/storage/ -run TestBenchmarkPath -v`
Expected: FAIL — `BenchmarkPath` not defined.

- [ ] **Step 3: Implement BenchmarkPath**

Add to `internal/nvr/storage/io_monitor.go`:

```go
import (
	"os"
	"path/filepath"
)

const benchBlockSize = 1 << 20 // 1 MB

// BenchmarkPath writes a 1MB test block to the given directory and returns
// the measured latency and throughput.
func BenchmarkPath(dir string) (IOSample, error) {
	testFile := filepath.Join(dir, ".nvr_io_bench")
	data := make([]byte, benchBlockSize)

	start := time.Now()
	if err := os.WriteFile(testFile, data, 0o644); err != nil {
		return IOSample{}, err
	}
	elapsed := time.Since(start)

	os.Remove(testFile)

	latencyMs := float64(elapsed.Microseconds()) / 1000.0
	throughputMB := float64(benchBlockSize) / (1024 * 1024) / elapsed.Seconds()

	return IOSample{
		Timestamp:    time.Now(),
		LatencyMs:    latencyMs,
		ThroughputMB: throughputMB,
	}, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /path/to/worktree && go test ./internal/nvr/storage/ -run TestBenchmarkPath -v`
Expected: All PASS.

- [ ] **Step 5: Wire into Manager**

Modify `internal/nvr/storage/manager.go`:

Add `IOMonitor` field and `EventPublisher` interface to the Manager struct and constructor:

```go
// At the top of manager.go, add the EventPublisher interface:

// EventPublisher can publish disk I/O events.
type EventPublisher interface {
	PublishDiskSlow(path string, avgLatencyMs, throughputMB, thresholdMs float64)
	PublishDiskCritical(path string, avgLatencyMs, throughputMB, thresholdMs float64)
	PublishDiskRecovered(path string, avgLatencyMs, throughputMB float64)
}
```

Add fields to the Manager struct:

```go
// Add to Manager struct fields:
	ioMonitor  *IOMonitor
	events     EventPublisher
```

Update the New function to initialize IOMonitor:

```go
// In the New function, add before the return:
	ioMonitor:      NewIOMonitor(50, 200),
```

Add a `SetEventPublisher` method:

```go
// SetEventPublisher sets the event publisher for disk I/O state change events.
func (m *Manager) SetEventPublisher(ep EventPublisher) {
	m.events = ep
}
```

Add `GetIOMonitor` accessor:

```go
// GetIOMonitor returns the IOMonitor for API handlers to read status.
func (m *Manager) GetIOMonitor() *IOMonitor {
	return m.ioMonitor
}
```

Add benchmark call inside `evaluateHealth`, after the existing health check succeeds for a path. Insert after `m.health[path] = healthy` and before the failover/recovery checks:

```go
		// Run I/O benchmark on healthy paths.
		if healthy {
			sample, benchErr := BenchmarkPath(path)
			if benchErr != nil {
				log.Printf("[NVR] [storage] I/O benchmark error for %s: %v", path, benchErr)
			} else {
				prev, curr := m.ioMonitor.Record(path, sample)
				if prev != curr && m.events != nil {
					warnMs, critMs := m.ioMonitor.paths[path].GetThresholds()
					switch {
					case curr == IOStateSlow:
						m.events.PublishDiskSlow(path, sample.LatencyMs, sample.ThroughputMB, warnMs)
					case curr == IOStateCritical:
						m.events.PublishDiskCritical(path, sample.LatencyMs, sample.ThroughputMB, critMs)
					case curr == IOStateHealthy:
						m.events.PublishDiskRecovered(path, sample.LatencyMs, sample.ThroughputMB)
					}
				}
			}
		}
```

- [ ] **Step 6: Run all storage tests**

Run: `cd /path/to/worktree && go test ./internal/nvr/storage/ -v`
Expected: All PASS.

- [ ] **Step 7: Commit**

```bash
git add internal/nvr/storage/io_monitor.go internal/nvr/storage/io_monitor_test.go internal/nvr/storage/manager.go
git commit -m "feat(storage): wire I/O benchmark into health check loop"
```

---

### Task 5: SSE Event Helpers

**Files:**

- Modify: `internal/nvr/api/events.go`

- [ ] **Step 1: Add PublishDiskSlow, PublishDiskCritical, PublishDiskRecovered to EventBroadcaster**

Add to `internal/nvr/api/events.go`:

```go
// PublishDiskSlow publishes a disk_slow event when a path crosses the warn threshold.
func (b *EventBroadcaster) PublishDiskSlow(path string, avgLatencyMs, throughputMB, thresholdMs float64) {
	b.Publish(Event{
		Type:    "disk_slow",
		Message: fmt.Sprintf("Disk I/O slow on %s: %.1fms avg (threshold: %.0fms)", path, avgLatencyMs, thresholdMs),
	})
}

// PublishDiskCritical publishes a disk_critical event when a path crosses the critical threshold.
func (b *EventBroadcaster) PublishDiskCritical(path string, avgLatencyMs, throughputMB, thresholdMs float64) {
	b.Publish(Event{
		Type:    "disk_critical",
		Message: fmt.Sprintf("Disk I/O critical on %s: %.1fms avg (threshold: %.0fms)", path, avgLatencyMs, thresholdMs),
	})
}

// PublishDiskRecovered publishes a disk_recovered event when a path returns to healthy.
func (b *EventBroadcaster) PublishDiskRecovered(path string, avgLatencyMs, throughputMB float64) {
	b.Publish(Event{
		Type:    "disk_recovered",
		Message: fmt.Sprintf("Disk I/O recovered on %s: %.1fms avg", path, avgLatencyMs),
	})
}
```

- [ ] **Step 2: Verify compilation**

Run: `cd /path/to/worktree && go build ./internal/nvr/api/...`
Expected: Compiles without errors.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/events.go
git commit -m "feat(api): add disk I/O SSE event helpers"
```

---

### Task 6: API Endpoints

**Files:**

- Modify: `internal/nvr/api/system.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Add StorageManager field to SystemHandler and implement DiskIO handler**

Add to `internal/nvr/api/system.go` — import the storage package and add the field:

```go
// Add to SystemHandler struct:
	StorageMgr *storage.Manager // storage manager for disk I/O metrics (may be nil)
```

Add the import:

```go
	"github.com/bluenviron/mediamtx/internal/nvr/storage"
```

Add the DiskIO endpoint handler:

```go
// DiskIO returns per-path I/O performance status and latency history.
//
//	GET /api/nvr/system/disk-io
func (h *SystemHandler) DiskIO(c *gin.Context) {
	if h.StorageMgr == nil {
		c.JSON(http.StatusOK, gin.H{"paths": map[string]interface{}{}})
		return
	}
	status := h.StorageMgr.GetIOMonitor().GetStatus()
	c.JSON(http.StatusOK, gin.H{"paths": status})
}
```

Add the UpdateDiskIOThresholds endpoint handler:

```go
// UpdateDiskIOThresholds updates warn/critical latency thresholds for a storage path.
//
//	PUT /api/nvr/system/disk-io/thresholds
func (h *SystemHandler) UpdateDiskIOThresholds(c *gin.Context) {
	if h.StorageMgr == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "storage manager not available"})
		return
	}

	var req struct {
		Path   string  `json:"path" binding:"required"`
		WarnMs float64 `json:"warn_ms" binding:"required,gt=0"`
		CritMs float64 `json:"critical_ms" binding:"required,gt=0"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.WarnMs >= req.CritMs {
		c.JSON(http.StatusBadRequest, gin.H{"error": "warn_ms must be less than critical_ms"})
		return
	}

	if err := h.StorageMgr.GetIOMonitor().UpdateThresholds(req.Path, req.WarnMs, req.CritMs); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"path":        req.Path,
		"warn_ms":     req.WarnMs,
		"critical_ms": req.CritMs,
	})
}
```

- [ ] **Step 2: Wire SystemHandler.StorageMgr in router.go**

In `internal/nvr/api/router.go`, add the `StorageMgr` field to the systemHandler initialization (around line 87):

```go
	systemHandler := &SystemHandler{
		Version:        cfg.Version,
		StartedAt:      time.Now(),
		SetupChecker:   cfg.SetupChecker,
		RecordingsPath: cfg.RecordingsPath,
		DB:             cfg.DB,
		Broadcaster:    cfg.Events,
		ConfigDB:       cfg.DB,
		ConfigPath:     cfg.ConfigPath,
		APIAddress:     cfg.APIAddress,
		Collector:      cfg.Collector,
		StorageMgr:     cfg.StorageManager,
	}
```

Register the two new endpoints in the protected system group (after line 309, `protected.GET("/system/metrics", ...)`):

```go
	protected.GET("/system/disk-io", systemHandler.DiskIO)
	protected.PUT("/system/disk-io/thresholds", systemHandler.UpdateDiskIOThresholds)
```

- [ ] **Step 3: Verify compilation**

Run: `cd /path/to/worktree && go build ./...`
Expected: Compiles without errors.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/system.go internal/nvr/api/router.go
git commit -m "feat(api): add disk I/O status and threshold endpoints"
```

---

### Task 7: Wire EventBroadcaster into Storage Manager

**Files:**

- Modify: `internal/nvr/nvr.go` (or wherever Manager.Start is called)

- [ ] **Step 1: Find where Manager is wired up**

Search for `storage.New(` or `StorageManager` being assigned in `internal/nvr/nvr.go` to find the wiring point.

- [ ] **Step 2: Call SetEventPublisher after Manager creation**

After the storage manager is created and before `Start()` is called, add:

```go
	storageMgr.SetEventPublisher(events)
```

Where `events` is the `*EventBroadcaster` instance. The EventBroadcaster satisfies the `EventPublisher` interface because we added the three `PublishDisk*` methods in Task 5.

- [ ] **Step 3: Verify compilation**

Run: `cd /path/to/worktree && go build ./...`
Expected: Compiles without errors.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/nvr.go
git commit -m "feat(nvr): wire event broadcaster into storage manager for disk I/O events"
```

---

### Task 8: Also Monitor Default Recordings Path

**Files:**

- Modify: `internal/nvr/storage/manager.go`

- [ ] **Step 1: Run I/O benchmark on the default recordings path too**

The current `evaluateHealth` only benchmarks paths from cameras with custom `StoragePath`. But the default recordings path (used by cameras without a custom path) should also be monitored.

Add a benchmark of `m.recordingsPath` at the end of `runHealthCheck`, after `m.evaluateHealth(pathCameras)`:

```go
	// Also benchmark the default recordings path.
	if m.recordingsPath != "" {
		sample, err := BenchmarkPath(m.recordingsPath)
		if err != nil {
			log.Printf("[NVR] [storage] I/O benchmark error for default path %s: %v", m.recordingsPath, err)
		} else {
			prev, curr := m.ioMonitor.Record(m.recordingsPath, sample)
			if prev != curr && m.events != nil {
				m.mu.RLock()
				pm := m.ioMonitor.paths[m.recordingsPath]
				m.mu.RUnlock()
				warnMs, critMs := pm.GetThresholds()
				switch {
				case curr == IOStateSlow:
					m.events.PublishDiskSlow(m.recordingsPath, sample.LatencyMs, sample.ThroughputMB, warnMs)
				case curr == IOStateCritical:
					m.events.PublishDiskCritical(m.recordingsPath, sample.LatencyMs, sample.ThroughputMB, critMs)
				case curr == IOStateHealthy:
					m.events.PublishDiskRecovered(m.recordingsPath, sample.LatencyMs, sample.ThroughputMB)
				}
			}
		}
	}
```

- [ ] **Step 2: Verify compilation and tests**

Run: `cd /path/to/worktree && go build ./... && go test ./internal/nvr/storage/ -v`
Expected: Compiles and all tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/storage/manager.go
git commit -m "feat(storage): also benchmark default recordings path for I/O monitoring"
```

---

### Task 9: Extract Event Emission Helper to Reduce Duplication

**Files:**

- Modify: `internal/nvr/storage/manager.go`

- [ ] **Step 1: Extract emitIOEvent helper**

Task 4 and Task 8 both have similar event emission code. Extract a helper method on Manager:

```go
// emitIOEvent publishes an SSE event if the I/O state changed for a path.
func (m *Manager) emitIOEvent(path string, sample IOSample, prev, curr IOState) {
	if prev == curr || m.events == nil {
		return
	}
	m.ioMonitor.mu.RLock()
	pm := m.ioMonitor.paths[path]
	m.ioMonitor.mu.RUnlock()
	if pm == nil {
		return
	}
	warnMs, critMs := pm.GetThresholds()
	switch {
	case curr == IOStateSlow:
		m.events.PublishDiskSlow(path, sample.LatencyMs, sample.ThroughputMB, warnMs)
	case curr == IOStateCritical:
		m.events.PublishDiskCritical(path, sample.LatencyMs, sample.ThroughputMB, critMs)
	case curr == IOStateHealthy:
		m.events.PublishDiskRecovered(path, sample.LatencyMs, sample.ThroughputMB)
	}
}
```

Replace the inline event emission in both `evaluateHealth` (Task 4) and `runHealthCheck` (Task 8) with:

```go
	m.emitIOEvent(path, sample, prev, curr)
```

- [ ] **Step 2: Verify compilation and tests**

Run: `cd /path/to/worktree && go build ./... && go test ./internal/nvr/storage/ -v`
Expected: Compiles and all tests pass.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/storage/manager.go
git commit -m "refactor(storage): extract emitIOEvent helper to reduce duplication"
```

---

### Task 10: Final Build Verification

**Files:** None (verification only)

- [ ] **Step 1: Run full test suite**

Run: `cd /path/to/worktree && go test ./internal/nvr/storage/ -v -count=1`
Expected: All tests pass.

- [ ] **Step 2: Run full build**

Run: `cd /path/to/worktree && go build ./...`
Expected: Clean build, no errors.

- [ ] **Step 3: Verify all new code compiles together**

Run: `cd /path/to/worktree && go vet ./internal/nvr/storage/ ./internal/nvr/api/`
Expected: No issues.
