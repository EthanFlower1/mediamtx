# Performance Graphs Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add CPU and memory usage graphs to a new Performance tab in Settings, backed by an in-memory ring buffer collecting metrics every 10 seconds.

**Architecture:** New `metrics` package with a ring buffer collector, enhanced `/system/metrics` API returning history, `fl_chart` line graphs in a Flutter Performance panel.

**Tech Stack:** Go (runtime, /proc parsing), Flutter, fl_chart, Riverpod

**Spec:** `docs/superpowers/specs/2026-03-29-performance-graphs-design.md`

---

## File Structure

```
internal/nvr/
├── metrics/
│   ├── collector.go          # CREATE — ring buffer + system metrics sampling
│   └── collector_test.go     # CREATE — tests
├── api/
│   ├── system.go             # MODIFY — enhance Metrics endpoint with history
│   └── router.go             # MODIFY — pass collector to SystemHandler
├── nvr.go                    # MODIFY — start/stop collector

clients/flutter/
├── pubspec.yaml              # MODIFY — add fl_chart
├── lib/providers/
│   └── settings_provider.dart  # MODIFY — add metrics history provider
├── lib/screens/settings/
│   ├── performance_panel.dart  # CREATE — chart widgets
│   └── settings_screen.dart    # MODIFY — add Performance tab
```

---

### Task 1: Metrics Collector (Ring Buffer)

**Files:**
- Create: `internal/nvr/metrics/collector.go`
- Create: `internal/nvr/metrics/collector_test.go`

- [ ] **Step 1: Create collector.go**

```go
// internal/nvr/metrics/collector.go
package metrics

import (
	"bufio"
	"os"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Sample is a single metrics snapshot.
type Sample struct {
	Timestamp  int64   `json:"t"`
	CPUPercent float64 `json:"cpu"`
	MemPercent float64 `json:"mem"`
	MemAllocMB float64 `json:"alloc"`
	MemSysMB   float64 `json:"sys"`
	Goroutines int     `json:"gr"`
}

// Collector samples system metrics at a fixed interval and stores them
// in a ring buffer.
type Collector struct {
	mu       sync.RWMutex
	samples  []Sample
	maxSize  int
	pos      int // next write position
	count    int // total samples written (for "is full" check)
	interval time.Duration
	stopCh   chan struct{}
	wg       sync.WaitGroup

	// Previous CPU jiffies for delta calculation (Linux only).
	prevTotal uint64
	prevIdle  uint64
}

// New creates a collector with the given buffer size and sample interval.
func New(maxSize int, interval time.Duration) *Collector {
	return &Collector{
		samples:  make([]Sample, maxSize),
		maxSize:  maxSize,
		interval: interval,
		stopCh:   make(chan struct{}),
	}
}

// Start begins background metric collection.
func (c *Collector) Start() {
	c.wg.Add(1)
	go func() {
		defer c.wg.Done()
		ticker := time.NewTicker(c.interval)
		defer ticker.Stop()

		// Take an initial sample immediately.
		c.collect()

		for {
			select {
			case <-ticker.C:
				c.collect()
			case <-c.stopCh:
				return
			}
		}
	}()
}

// Stop halts collection and waits for the goroutine to exit.
func (c *Collector) Stop() {
	close(c.stopCh)
	c.wg.Wait()
}

// History returns all samples ordered oldest-first.
func (c *Collector) History() []Sample {
	c.mu.RLock()
	defer c.mu.RUnlock()

	n := c.count
	if n > c.maxSize {
		n = c.maxSize
	}
	result := make([]Sample, n)

	if c.count <= c.maxSize {
		// Buffer not yet full — samples are 0..pos-1.
		copy(result, c.samples[:n])
	} else {
		// Buffer wrapped — oldest is at pos, read pos..end then 0..pos-1.
		tail := c.maxSize - c.pos
		copy(result[:tail], c.samples[c.pos:])
		copy(result[tail:], c.samples[:c.pos])
	}
	return result
}

// Current returns the most recent sample.
func (c *Collector) Current() Sample {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.count == 0 {
		return Sample{}
	}
	idx := c.pos - 1
	if idx < 0 {
		idx = c.maxSize - 1
	}
	return c.samples[idx]
}

func (c *Collector) collect() {
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)

	s := Sample{
		Timestamp:  time.Now().Unix(),
		CPUPercent: c.readCPUPercent(),
		MemPercent: readMemPercent(),
		MemAllocMB: float64(ms.Alloc) / (1024 * 1024),
		MemSysMB:   float64(ms.Sys) / (1024 * 1024),
		Goroutines: runtime.NumGoroutine(),
	}

	c.mu.Lock()
	c.samples[c.pos] = s
	c.pos = (c.pos + 1) % c.maxSize
	c.count++
	c.mu.Unlock()
}

// readCPUPercent reads /proc/stat on Linux and calculates CPU usage
// as a percentage since the last sample. Returns 0 on non-Linux or error.
func (c *Collector) readCPUPercent() float64 {
	f, err := os.Open("/proc/stat")
	if err != nil {
		return 0
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	if !scanner.Scan() {
		return 0
	}
	line := scanner.Text()
	if !strings.HasPrefix(line, "cpu ") {
		return 0
	}

	fields := strings.Fields(line)
	if len(fields) < 5 {
		return 0
	}

	var total, idle uint64
	for i := 1; i < len(fields); i++ {
		v, _ := strconv.ParseUint(fields[i], 10, 64)
		total += v
		if i == 4 { // idle is the 4th value (0-indexed field 4)
			idle = v
		}
	}

	cpuPercent := 0.0
	if c.prevTotal > 0 {
		totalDelta := total - c.prevTotal
		idleDelta := idle - c.prevIdle
		if totalDelta > 0 {
			cpuPercent = float64(totalDelta-idleDelta) / float64(totalDelta) * 100
		}
	}

	c.prevTotal = total
	c.prevIdle = idle
	return cpuPercent
}

// readMemPercent reads /proc/meminfo on Linux. Returns 0 on non-Linux or error.
func readMemPercent() float64 {
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		// Fallback: use Go runtime stats as rough estimate.
		var ms runtime.MemStats
		runtime.ReadMemStats(&ms)
		// This is just process memory, not system-wide. Return 0 for accuracy.
		return 0
	}
	defer f.Close()

	var memTotal, memAvailable uint64
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "MemTotal:") {
			memTotal = parseMemInfoKB(line)
		} else if strings.HasPrefix(line, "MemAvailable:") {
			memAvailable = parseMemInfoKB(line)
		}
		if memTotal > 0 && memAvailable > 0 {
			break
		}
	}

	if memTotal == 0 {
		return 0
	}
	return float64(memTotal-memAvailable) / float64(memTotal) * 100
}

func parseMemInfoKB(line string) uint64 {
	fields := strings.Fields(line)
	if len(fields) < 2 {
		return 0
	}
	v, _ := strconv.ParseUint(fields[1], 10, 64)
	return v
}
```

- [ ] **Step 2: Write collector tests**

```go
// internal/nvr/metrics/collector_test.go
package metrics

import (
	"testing"
	"time"
)

func TestCollectorBasic(t *testing.T) {
	c := New(10, 100*time.Millisecond)
	c.Start()
	time.Sleep(350 * time.Millisecond) // ~3 samples + initial
	c.Stop()

	history := c.History()
	if len(history) < 3 {
		t.Fatalf("expected at least 3 samples, got %d", len(history))
	}

	// Verify timestamps are increasing.
	for i := 1; i < len(history); i++ {
		if history[i].Timestamp < history[i-1].Timestamp {
			t.Errorf("timestamps not increasing: [%d]=%d, [%d]=%d",
				i-1, history[i-1].Timestamp, i, history[i].Timestamp)
		}
	}

	// Verify current returns the last sample.
	cur := c.Current()
	last := history[len(history)-1]
	if cur.Timestamp != last.Timestamp {
		t.Errorf("Current().Timestamp = %d, want %d", cur.Timestamp, last.Timestamp)
	}
}

func TestCollectorRingBufferWrap(t *testing.T) {
	c := New(5, 50*time.Millisecond)
	c.Start()
	time.Sleep(400 * time.Millisecond) // ~8 samples, buffer wraps
	c.Stop()

	history := c.History()
	if len(history) != 5 {
		t.Fatalf("expected 5 samples (buffer full), got %d", len(history))
	}

	// Verify ordered oldest-first.
	for i := 1; i < len(history); i++ {
		if history[i].Timestamp < history[i-1].Timestamp {
			t.Errorf("not ordered oldest-first at index %d", i)
		}
	}
}

func TestCollectorEmpty(t *testing.T) {
	c := New(10, time.Hour) // won't tick
	history := c.History()
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d", len(history))
	}

	cur := c.Current()
	if cur.Timestamp != 0 {
		t.Errorf("expected zero current, got timestamp %d", cur.Timestamp)
	}
}
```

- [ ] **Step 3: Run tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/metrics/ -v -count=1`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/metrics/
git commit -m "feat(metrics): add ring buffer collector with CPU and memory sampling"
```

---

### Task 2: Enhance Metrics API Endpoint

**Files:**
- Modify: `internal/nvr/api/system.go`
- Modify: `internal/nvr/api/router.go`
- Modify: `internal/nvr/nvr.go`

- [ ] **Step 1: Add Collector field to SystemHandler**

In `internal/nvr/api/system.go`, add to the `SystemHandler` struct:

```go
Collector *metrics.Collector // may be nil if not started
```

Add import: `"github.com/bluenviron/mediamtx/internal/nvr/metrics"`

- [ ] **Step 2: Enhance the Metrics handler**

Replace the existing `Metrics` function in `system.go` with:

```go
func (h *SystemHandler) Metrics(c *gin.Context) {
	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	var cameraCount int
	if h.ConfigDB != nil {
		cameras, err := h.ConfigDB.ListCameras()
		if err == nil {
			cameraCount = len(cameras)
		}
	}

	current := gin.H{
		"cpu_percent":  0.0,
		"mem_percent":  0.0,
		"mem_alloc_mb": float64(m.Alloc) / (1024 * 1024),
		"mem_sys_mb":   float64(m.Sys) / (1024 * 1024),
		"goroutines":   runtime.NumGoroutine(),
	}

	var history []metrics.Sample
	if h.Collector != nil {
		cur := h.Collector.Current()
		current["cpu_percent"] = cur.CPUPercent
		current["mem_percent"] = cur.MemPercent
		history = h.Collector.History()
	}

	c.JSON(http.StatusOK, gin.H{
		// Legacy fields for backward compatibility.
		"cpu_goroutines":  runtime.NumGoroutine(),
		"mem_alloc_bytes": m.Alloc,
		"mem_sys_bytes":   m.Sys,
		"mem_gc_count":    m.NumGC,
		"uptime_seconds":  time.Since(h.StartedAt).Seconds(),
		"camera_count":    cameraCount,
		// New structured fields.
		"current": current,
		"history": history,
	})
}
```

- [ ] **Step 3: Add Collector to RouterConfig and pass to SystemHandler**

In `internal/nvr/api/router.go`:

Add to `RouterConfig` struct:
```go
Collector *metrics.Collector
```

Add import: `"github.com/bluenviron/mediamtx/internal/nvr/metrics"`

In the `systemHandler` instantiation, add:
```go
Collector: cfg.Collector,
```

- [ ] **Step 4: Start collector in NVR and pass to routes**

In `internal/nvr/nvr.go`:

Add field to NVR struct:
```go
metricsCollector *metrics.Collector
```

Add import: `"github.com/bluenviron/mediamtx/internal/nvr/metrics"`

In `Initialize()`, after database setup (before the WebSocket server start), add:
```go
// Start metrics collector (1 hour of history at 10-second intervals).
n.metricsCollector = metrics.New(360, 10*time.Second)
n.metricsCollector.Start()
```

In `Close()`, add before other cleanup:
```go
if n.metricsCollector != nil {
    n.metricsCollector.Stop()
}
```

In `RegisterRoutes()` (the function that builds the RouterConfig), add the collector:
```go
Collector: n.metricsCollector,
```

Find where `RegisterRoutes` is called and where the `RouterConfig` is built — it may be in `nvr.go` itself. Read the file to find the exact location and add the field.

- [ ] **Step 5: Verify**

Run: `go build .`
Run: `go test ./internal/nvr/... -count=1 2>&1 | tail -12`
Expected: pass

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/api/system.go internal/nvr/api/router.go internal/nvr/nvr.go
git commit -m "feat(api): enhance /system/metrics with ring buffer history"
```

---

### Task 3: Add fl_chart and Performance Panel

**Files:**
- Modify: `clients/flutter/pubspec.yaml`
- Create: `clients/flutter/lib/screens/settings/performance_panel.dart`

- [ ] **Step 1: Add fl_chart dependency**

In `clients/flutter/pubspec.yaml`, add to the `dependencies` section:

```yaml
  fl_chart: ^0.69.2
```

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter pub get`

- [ ] **Step 2: Create performance_panel.dart**

Create `clients/flutter/lib/screens/settings/performance_panel.dart`.

This is a `ConsumerStatefulWidget` called `PerformancePanel` that:

**State:**
```dart
Map<String, dynamic>? _current;
List<dynamic> _history = [];
Timer? _refreshTimer;
bool _loading = true;
```

**initState:** Call `_fetchMetrics()` then start `Timer.periodic(Duration(seconds: 10), (_) => _fetchMetrics())`.

**dispose:** Cancel the timer.

**_fetchMetrics:**
```dart
Future<void> _fetchMetrics() async {
  final api = ref.read(apiClientProvider);
  if (api == null) return;
  try {
    final res = await api.get<dynamic>('/system/metrics');
    final data = res.data as Map<String, dynamic>;
    if (mounted) {
      setState(() {
        _current = data['current'] as Map<String, dynamic>?;
        _history = data['history'] as List<dynamic>? ?? [];
        _loading = false;
      });
    }
  } catch (_) {
    if (mounted) setState(() => _loading = false);
  }
}
```

**build:** Returns a `SingleChildScrollView` with:

1. **CPU & Memory chart** in a `_SectionCard`:
   - Title: "CPU & MEMORY USAGE"
   - `SizedBox(height: 200)` containing a `LineChart` from fl_chart
   - Two lines: CPU% (orange/accent) and Memory% (green)
   - Y-axis: 0-100, X-axis: timestamps from history
   - Legend row below chart: orange dot + "CPU", green dot + "Memory"

2. **Process Memory chart** in a `_SectionCard`:
   - Title: "PROCESS MEMORY"
   - `SizedBox(height: 200)` containing a `LineChart`
   - One line: heap alloc MB (accent)
   - Y-axis: 0-max MB (auto-scaled)

3. **Current stats** in a `_SectionCard`:
   - Key-value rows: CPU USAGE, MEMORY USAGE, GO HEAP, GOROUTINES

**Chart styling (fl_chart):**
```dart
LineChartData(
  backgroundColor: NvrColors.bgSecondary,
  gridData: FlGridData(
    show: true,
    drawVerticalLine: false,
    getDrawingHorizontalLine: (value) => FlLine(
      color: NvrColors.border,
      strokeWidth: 0.5,
    ),
  ),
  titlesData: FlTitlesData(
    leftTitles: AxisTitles(
      sideTitles: SideTitles(
        showTitles: true,
        reservedSize: 40,
        getTitlesWidget: (value, meta) => Text(
          '${value.toInt()}%',
          style: NvrTypography.monoLabel,
        ),
      ),
    ),
    bottomTitles: AxisTitles(
      sideTitles: SideTitles(
        showTitles: true,
        reservedSize: 22,
        interval: (_history.length / 6).ceilToDouble().clamp(1, 100),
        getTitlesWidget: (value, meta) {
          final idx = value.toInt();
          if (idx < 0 || idx >= _history.length) return const SizedBox.shrink();
          final ts = DateTime.fromMillisecondsSinceEpoch(
            (_history[idx]['t'] as num).toInt() * 1000,
          );
          return Text(
            '${ts.hour.toString().padLeft(2, '0')}:${ts.minute.toString().padLeft(2, '0')}',
            style: NvrTypography.monoLabel,
          );
        },
      ),
    ),
    topTitles: const AxisTitles(sideTitles: SideTitles(showTitles: false)),
    rightTitles: const AxisTitles(sideTitles: SideTitles(showTitles: false)),
  ),
  borderData: FlBorderData(show: false),
  lineBarsData: [
    LineChartBarData(
      spots: _history.asMap().entries.map((e) =>
        FlSpot(e.key.toDouble(), (e.value['cpu'] as num).toDouble()),
      ).toList(),
      color: NvrColors.accent,
      barWidth: 2,
      dotData: const FlDotData(show: false),
      belowBarData: BarAreaData(
        show: true,
        color: NvrColors.accent.withValues(alpha: 0.1),
      ),
    ),
    LineChartBarData(
      spots: _history.asMap().entries.map((e) =>
        FlSpot(e.key.toDouble(), (e.value['mem'] as num).toDouble()),
      ).toList(),
      color: const Color(0xFF22c55e),
      barWidth: 2,
      dotData: const FlDotData(show: false),
      belowBarData: BarAreaData(
        show: true,
        color: const Color(0xFF22c55e).withValues(alpha: 0.1),
      ),
    ),
  ],
  minY: 0,
  maxY: 100,
)
```

Use `_SectionCard` pattern from the settings screen (Container with bgSecondary, border, 8px radius, 12px padding, header text).

**Imports:**
```dart
import 'dart:async';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:fl_chart/fl_chart.dart';
import '../../providers/auth_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
```

- [ ] **Step 3: Verify**

Run: `flutter analyze lib/screens/settings/performance_panel.dart`
Expected: No errors

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/pubspec.yaml clients/flutter/pubspec.lock clients/flutter/lib/screens/settings/performance_panel.dart
git commit -m "feat(flutter): add Performance panel with CPU and memory charts"
```

---

### Task 4: Add Performance Tab to Settings Screen

**Files:**
- Modify: `clients/flutter/lib/screens/settings/settings_screen.dart`

- [ ] **Step 1: Add import**

```dart
import 'performance_panel.dart';
```

- [ ] **Step 2: Add tab to _sections**

Change the `_sections` list from:
```dart
static const _sections = ['System', 'Storage', 'Users', 'Backups', 'Audit Log'];
```
to:
```dart
static const _sections = ['System', 'Storage', 'Performance', 'Users', 'Backups', 'Audit Log'];
```

- [ ] **Step 3: Add case to _buildContent switch**

Update the switch in `_buildContent()`. Insert the new case and shift existing indices:

```dart
Widget _buildContent() {
  switch (_selectedSection) {
    case 0:
      return const _SystemPanel();
    case 1:
      return const StoragePanel();
    case 2:
      return const PerformancePanel();
    case 3:
      return const UserManagementScreen();
    case 4:
      return const BackupPanel();
    case 5:
      return const AuditPanel();
    default:
      return const SizedBox.shrink();
  }
}
```

- [ ] **Step 4: Verify**

Run: `flutter analyze lib/screens/settings/settings_screen.dart`
Expected: No errors

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/screens/settings/settings_screen.dart
git commit -m "feat(flutter): add Performance tab to Settings screen"
```

---

### Task 5: End-to-End Verification

- [ ] **Step 1: Run all Go tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -count=1`
Expected: all pass

- [ ] **Step 2: Build**

Run: `go build .`
Expected: builds

- [ ] **Step 3: Flutter analyze**

Run: `cd clients/flutter && flutter analyze lib/`
Expected: no errors

- [ ] **Step 4: Manual smoke test**

1. Start server, wait 30+ seconds for some samples to accumulate
2. Open Settings → Performance tab
3. Verify CPU & Memory chart shows lines updating
4. Verify Process Memory chart shows heap usage
5. Verify current stats show numeric values
6. Wait 10 seconds → verify charts update with new data point
7. `curl localhost:9997/api/nvr/system/metrics | jq .history | jq length` → should be >0

- [ ] **Step 5: Commit**

```bash
git add -A
git commit -m "test: verify performance graphs end-to-end"
```
