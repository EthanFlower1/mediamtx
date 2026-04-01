# KAI-13: Disk I/O Performance Monitoring Design

## Overview

Extend the existing storage manager's health check loop to measure write latency per storage path, detect slow disks via configurable thresholds, and warn before performance impacts recording. Expose metrics via API and SSE events.

## Goals

- Track write latency and throughput per storage path every 30 seconds
- Detect slow disks using configurable thresholds with sliding-window smoothing
- Emit SSE events on state transitions (healthy/slow/critical)
- Expose latency history and current status via REST API
- Warn operators before disk performance degrades recording quality

## Non-Goals

- Per-camera latency breakdowns (derivable from camera-to-path mapping)
- Instrumenting actual recording writes (future enhancement)
- Disk space monitoring (already exists in `GET /api/nvr/system/storage`)
- Auto-failover on slow disk (KAI-9 recording health handles recovery)

## Design

### 1. Latency Measurement

Extend the storage manager's existing 30-second health check loop. Currently it writes and reads a small `.nvr_health_check` file to verify path reachability. We add:

- Write a 1MB test block (`.nvr_io_bench`) and time the write using `time.Since`
- Calculate write latency (ms) and throughput (MB/s)
- Store the sample in a per-path ring buffer
- Remove the test file after measurement

The 1MB block size is large enough to produce meaningful latency numbers but small enough to avoid impacting actual recording I/O. The benchmark runs after the existing health check so path reachability is confirmed first.

```go
type IOSample struct {
    Timestamp    time.Time
    LatencyMs    float64
    ThroughputMB float64
}
```

### 2. Ring Buffer Storage

Each storage path gets a ring buffer holding 360 samples (3 hours at 30s intervals), matching the pattern used by the existing metrics collector.

```go
type PathIOMetrics struct {
    mu       sync.RWMutex
    samples  [360]IOSample
    head     int
    count    int
    state    IOState       // healthy, slow, critical
    warnMs   float64       // default 50
    critMs   float64       // default 200
}
```

### 3. Threshold Evaluation

After each sample is recorded, evaluate the sliding window of the last 5 samples:

- Compute the average latency over the window
- Compare against configurable thresholds:
  - `healthy`: avg < `warn_ms` (default 50ms)
  - `slow`: avg >= `warn_ms` and avg < `critical_ms`
  - `critical`: avg >= `critical_ms` (default 200ms)
- The 5-sample window (2.5 minutes) prevents single-spike false alarms

State transitions trigger SSE events. Thresholds are stored in memory with defaults; configurable via API.

### 4. SSE Events

Publish through the existing `EventBroadcaster` on state transitions:

| Event            | Trigger                  | Payload                                                            |
| ---------------- | ------------------------ | ------------------------------------------------------------------ |
| `disk_slow`      | healthy -> slow          | `{ path, avg_latency_ms, throughput_mbps, warn_threshold_ms }`     |
| `disk_critical`  | any -> critical          | `{ path, avg_latency_ms, throughput_mbps, critical_threshold_ms }` |
| `disk_recovered` | slow/critical -> healthy | `{ path, avg_latency_ms, throughput_mbps }`                        |

### 5. API Endpoints

#### `GET /api/nvr/system/disk-io`

Returns per-path I/O status and history.

```json
{
  "paths": {
    "/recordings": {
      "state": "healthy",
      "latest": {
        "timestamp": "2026-04-01T12:00:00Z",
        "latency_ms": 12.3,
        "throughput_mbps": 81.3
      },
      "thresholds": {
        "warn_ms": 50,
        "critical_ms": 200
      },
      "history": [
        {
          "timestamp": "2026-04-01T11:59:30Z",
          "latency_ms": 11.8,
          "throughput_mbps": 84.7
        }
      ]
    }
  }
}
```

#### `PUT /api/nvr/system/disk-io/thresholds`

Update thresholds for a specific path.

```json
{
  "path": "/recordings",
  "warn_ms": 75,
  "critical_ms": 300
}
```

Returns 200 with updated thresholds, or 404 if path not found.

### 6. Integration Points

| Component                         | Change                                                                   |
| --------------------------------- | ------------------------------------------------------------------------ |
| `internal/nvr/storage/manager.go` | Add IO benchmark to health check loop, ring buffer, threshold evaluation |
| `internal/nvr/api/router.go`      | Register new endpoints                                                   |
| `internal/nvr/api/system.go`      | Implement `disk-io` and `thresholds` handlers                            |
| `internal/nvr/api/events.go`      | Add `disk_slow`, `disk_critical`, `disk_recovered` event types           |

### 7. New Files

| File                                      | Purpose                                                                    |
| ----------------------------------------- | -------------------------------------------------------------------------- |
| `internal/nvr/storage/io_monitor.go`      | `PathIOMetrics` ring buffer, `IOSample` struct, threshold evaluation logic |
| `internal/nvr/storage/io_monitor_test.go` | Unit tests for ring buffer, threshold evaluation, state transitions        |

## Testing Strategy

- **Unit tests**: Ring buffer insert/read, threshold evaluation with various latency patterns, state transition logic (including edge cases like immediate critical, recovery oscillation)
- **Integration tests**: Verify API endpoints return correct structure, SSE events fire on state transitions
- **Manual validation**: Run with a real storage path and confirm metrics appear in API response
