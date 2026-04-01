# Performance Graphs

**Date:** 2026-03-29
**Status:** Approved
**Goal:** Add a Performance tab to Settings with CPU and memory usage graphs showing 1 hour of history, backed by an in-memory ring buffer on the server.

---

## Context

The backend already has `GET /system/metrics` returning point-in-time memory and goroutine stats. This feature adds historical collection via a ring buffer and renders the data as line charts in a new Settings tab.

## Backend: Metrics Collector

### Ring Buffer

A background goroutine started in `NVR.Initialize()` samples metrics every 10 seconds and stores them in a thread-safe ring buffer. The buffer holds 360 samples (1 hour). When full, the oldest sample is overwritten.

### MetricsSample Struct

```go
type MetricsSample struct {
    Timestamp  int64   `json:"t"`     // Unix seconds
    CPUPercent float64 `json:"cpu"`   // system CPU usage 0-100
    MemPercent float64 `json:"mem"`   // system RAM usage 0-100
    MemAllocMB float64 `json:"alloc"` // Go heap allocation in MB
    MemSysMB   float64 `json:"sys"`   // Go process memory in MB
    Goroutines int     `json:"gr"`    // active goroutines
}
```

### System Metrics Collection (No External Dependencies)

**Process metrics (cross-platform):**

- `runtime.MemStats.Alloc` → heap allocation
- `runtime.MemStats.Sys` → total process memory
- `runtime.NumGoroutine()` → goroutine count

**System CPU % (Linux):**

- Parse `/proc/stat` to get total and idle CPU jiffies
- Compare two consecutive reads (10s apart) to calculate utilization
- Fallback: return 0 on non-Linux platforms

**System Memory % (Linux):**

- Parse `/proc/meminfo` for MemTotal and MemAvailable
- Calculate: `(total - available) / total * 100`
- Fallback on macOS: use `syscall.Sysctl("hw.memsize")` for total, `runtime.MemStats.Sys` for process usage (less accurate but functional)

### Ring Buffer Implementation

New file: `internal/nvr/metrics/collector.go`

```go
type Collector struct {
    mu      sync.RWMutex
    samples []MetricsSample
    maxSize int
    index   int
    full    bool
    stopCh  chan struct{}
}
```

- `NewCollector(maxSize int) *Collector`
- `Start()` — launches background goroutine sampling every 10s
- `Stop()` — stops the goroutine
- `Samples() []MetricsSample` — returns samples oldest-first (thread-safe copy)
- `Current() MetricsSample` — returns the latest sample

---

## Backend: API

### Enhanced GET /system/metrics

Update the existing endpoint to include the ring buffer history:

```json
{
  "current": {
    "cpu_percent": 12.5,
    "mem_percent": 45.2,
    "mem_alloc_mb": 128.5,
    "mem_sys_mb": 256.0,
    "goroutines": 42
  },
  "history": [
    {
      "t": 1711742400,
      "cpu": 12.5,
      "mem": 45.2,
      "alloc": 128.5,
      "sys": 256.0,
      "gr": 42
    },
    {
      "t": 1711742410,
      "cpu": 13.1,
      "mem": 45.3,
      "alloc": 129.0,
      "sys": 256.0,
      "gr": 43
    }
  ]
}
```

The `history` array contains the full ring buffer ordered oldest-first. Timestamps are Unix seconds for compact JSON.

The existing fields (`cpu_goroutines`, `mem_alloc_bytes`, etc.) remain for backward compatibility. The new `current` and `history` fields are additive.

---

## Flutter: Performance Tab

### Package Addition

Add `fl_chart` to `pubspec.yaml`. This is the most popular Flutter charting library, supports line charts with dark themes, and has no native dependencies.

### Settings Screen: New Tab

Add "Performance" as a new tab in the Settings screen sidebar/pill tabs, after "Audit Log" (6th tab).

Icon: `Icons.show_chart` or `Icons.timeline`

### Tab Layout

```
PERFORMANCE (section header)

┌─────────────────────────────────────────────┐
│  CPU & MEMORY USAGE                         │
│  [Line chart: CPU% orange, Memory% green]   │
│  Y: 0-100%  X: last 1 hour                 │
└─────────────────────────────────────────────┘

┌─────────────────────────────────────────────┐
│  PROCESS MEMORY                             │
│  [Line chart: Heap MB in accent]            │
│  Y: 0-max MB  X: last 1 hour               │
└─────────────────────────────────────────────┘

CURRENT STATS
  CPU USAGE        12.5%
  MEMORY USAGE     45.2%
  GO HEAP          128.5 MB
  GOROUTINES       42
```

### Chart Styling

Match the NVR dark theme:

- Background: NvrColors.bgSecondary
- Grid lines: NvrColors.border (subtle)
- CPU line: NvrColors.accent (#f97316, orange)
- Memory line: #22c55e (green)
- Heap line: NvrColors.accent
- Axis labels: NvrTypography.monoLabel (gray, 9px)
- Tooltip: dark background, accent text
- Chart container: \_SectionCard with 1px border, 8px radius

### Auto-Refresh

Poll `GET /system/metrics` every 10 seconds using a `Timer.periodic`. Dispose the timer on tab/screen dispose. Each poll updates the chart data.

### Data Provider

New provider: `metricsHistoryProvider` — fetches `/system/metrics`, parses `current` and `history`, returns a structured object. The Performance tab watches this provider and triggers a re-fetch on the timer.

---

## Files

### Backend

- Create: `internal/nvr/metrics/collector.go` — ring buffer + system metrics sampling
- Modify: `internal/nvr/api/system.go` — enhance Metrics endpoint with history
- Modify: `internal/nvr/nvr.go` — start/stop collector
- Modify: `internal/nvr/api/router.go` — pass collector to SystemHandler (if needed)

### Flutter

- Modify: `clients/flutter/pubspec.yaml` — add `fl_chart`
- Create: `clients/flutter/lib/screens/settings/performance_panel.dart` — chart widgets
- Modify: `clients/flutter/lib/screens/settings/settings_screen.dart` — add Performance tab
