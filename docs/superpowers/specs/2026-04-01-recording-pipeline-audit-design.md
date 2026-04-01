# KAI-5: Recording Pipeline Data Loss Audit — Design Spec

## Overview

Audit the MediaMTX NVR recording pipeline under adverse conditions to identify and document all data loss scenarios. Produces an automated test harness (`internal/nvr/audit/`) and a structured findings report.

## Approach

**Hybrid: Layer-level + End-to-End scenario tests.**

- Layer-level tests isolate each pipeline component under fault injection to pinpoint exactly where data loss occurs.
- E2E scenario tests verify the full pipeline (RTSP source → Recorder → fMP4 → disk → DB indexing → storage manager) under realistic failure conditions.
- All tests gated behind `//go:build integration` tag.

## Pipeline Under Test

```
RTSP Source → stream.Stream → Recorder (supervisor) → recorderInstance
  → formatFMP4 (tracks, segments, parts) → disk segments
  → OnSegmentComplete callback → DB insert (recordings, fragments)
  → Fragment backfill (async indexing) → Storage manager (health/failover)
```

Key characteristics:

- Recorder supervisor restarts failed instances with 2s pause
- fMP4 parts written every 1s (configurable), segments closed at 1h
- Fragment backfill indexes fMP4 files asynchronously (newest-first)
- Storage manager checks health every 30s with `.nvr_health_check` file
- NTP drift tolerance of 5s for timestamp discontinuities

## Package Structure

```
internal/nvr/audit/
├── audit_test.go          # Shared helpers, RTSP test source, setup/teardown
├── stream_test.go         # Layer: camera-side network failures
├── recorder_test.go       # Layer: recorder/format fault injection
├── storage_test.go        # Layer: storage I/O failures
├── database_test.go       # Layer: SQLite transaction failures
├── lifecycle_test.go      # Layer: process signals (SIGTERM, SIGKILL)
├── scenario_test.go       # E2E: full pipeline under failure conditions
└── findings.go            # Structured types for audit findings
```

## Layer-Level Tests

### stream_test.go — Camera-side network failures

| Test                   | Failure Mode                         | Verifies                                                                                  |
| ---------------------- | ------------------------------------ | ----------------------------------------------------------------------------------------- |
| `TestStreamDisconnect` | Kill RTSP source mid-recording       | Current segment properly closed (valid fMP4), DB entry gets end time, supervisor restarts |
| `TestStreamStall`      | Source stops sending, TCP stays open | Read timeout triggers, segment closed cleanly, no zombie goroutines                       |
| `TestStreamReconnect`  | Source disconnects, returns after 5s | New segment starts, gap accurately reflected in DB timeline                               |

### recorder_test.go — Recorder/format layer

| Test                         | Failure Mode                                          | Verifies                                                                |
| ---------------------------- | ----------------------------------------------------- | ----------------------------------------------------------------------- |
| `TestSegmentBoundaryFailure` | Error during segment close (mock file close)          | Partial segment state, whether prior completed part data is recoverable |
| `TestOOMPressure`            | Record with constrained `GOMEMLIMIT`                  | Behavior under GC pressure, segment write correctness                   |
| `TestLargePartSize`          | High-bitrate data exceeding `MaxPartSize` in one part | Part split correctly, no truncation                                     |

### storage_test.go — Storage-side failures

| Test                          | Failure Mode                                  | Verifies                                                           |
| ----------------------------- | --------------------------------------------- | ------------------------------------------------------------------ |
| `TestDiskFull`                | Fill temp directory to capacity mid-recording | Current segment state, error propagation, storage manager failover |
| `TestStoragePathUnavailable`  | Remove recording directory mid-write          | Error handling, failover to alternate path, data loss in gap       |
| `TestStoragePermissionDenied` | Change directory to read-only mid-recording   | Error surfaces, recording state consistency                        |

### database_test.go — SQLite failures

| Test                                       | Failure Mode                            | Verifies                                                     |
| ------------------------------------------ | --------------------------------------- | ------------------------------------------------------------ |
| `TestDBInsertFailureDuringSegmentComplete` | DB returns error on `InsertRecording`   | Orphaned file on disk with no DB entry, recovery possibility |
| `TestDBFragmentIndexingFailure`            | Fail during `InsertFragments`           | Recording exists but fragments missing, backfill recovery    |
| `TestDBLocked`                             | Hold write lock during segment complete | Retry behavior, whether data is buffered or lost             |

### lifecycle_test.go — Process signals

| Test                   | Failure Mode                       | Verifies                                                                                            |
| ---------------------- | ---------------------------------- | --------------------------------------------------------------------------------------------------- |
| `TestGracefulShutdown` | SIGTERM during active recording    | Segments properly closed, DB consistent, no data loss                                               |
| `TestSIGKILL`          | `kill -9` subprocess mid-recording | fMP4 file integrity (readable up to last moof+mdat?), DB state (incomplete entries?), recovery path |
| `TestRestartRecovery`  | SIGKILL then fresh start           | Orphaned file detection, fragment backfill re-indexes, timeline has accurate gap                    |

## End-to-End Scenario Tests

### scenario_test.go

| Test                                | Scenario                                                    | Verifies                                                                                              |
| ----------------------------------- | ----------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `TestScenarioNetworkDropAndRecover` | Record 10s, kill source, wait 5s, restart, record 10s       | Two valid segments, accurate 5s gap in timeline, both playable via HLS, fragment index covers both    |
| `TestScenarioDiskFullRecovery`      | Record until disk fills, clear space, verify resume         | Pre-full segment valid/recoverable, new segment starts cleanly, DB timeline reflects gap              |
| `TestScenarioPowerLoss`             | Fork subprocess, record 10s, SIGKILL, run recovery          | Last segment readable to last complete moof+mdat, DB state after backfill recovery, timeline accuracy |
| `TestScenarioStorageFailover`       | Primary + fallback paths, make primary unavailable, restore | No gap during failover, files consolidated after restore                                              |
| `TestScenarioOOMKill`               | Fork subprocess with `GOMEMLIMIT`, high-bitrate stream      | Segment integrity, whether OOM kill leaves corruption                                                 |

## Findings Report

### Structured types (`findings.go`)

```go
type Finding struct {
    Scenario     string // e.g., "disk_full"
    Layer        string // e.g., "storage", "recorder", "database"
    Severity     string // "data_loss", "corruption", "gap", "recoverable"
    Description  string // What happens
    Reproduction string // How to trigger
    DataImpact   string // What data is lost/corrupted
    Recovery     string // How to recover, if possible
}
```

### Output

- `testdata/audit_findings.json` — machine-readable findings from each test run
- `docs/recording-audit-report.md` — aggregated markdown report generated by `TestGenerateReport`
- Report satisfies KAI-5 acceptance criteria: test matrix, reproduction steps, severity categorization

## Test Infrastructure

### RTSP Test Source

Use a minimal test RTSP server that generates synthetic H.264 + AAC frames. Leverage existing test helpers in the codebase if available, otherwise use ffmpeg to stream a test pattern to a local RTSP endpoint.

### Subprocess Testing (for SIGKILL/power loss)

Tests that need SIGKILL will:

1. Build a small test binary that sets up the recording pipeline
2. Run it as a subprocess via `exec.Command`
3. Send `SIGKILL` after a delay
4. Inspect the filesystem and DB state from the parent test process

### Temp Directory Management

Each test gets an isolated temp directory for recordings and SQLite DB. Cleaned up on teardown unless `AUDIT_KEEP_ARTIFACTS=1` is set (for manual inspection).

## Failure Mode Categories

Findings will be categorized using these severity levels:

| Severity        | Definition                                                                       |
| --------------- | -------------------------------------------------------------------------------- |
| **data_loss**   | Recorded data is permanently lost with no recovery path                          |
| **corruption**  | File or DB is left in an inconsistent state that requires manual intervention    |
| **gap**         | Recording has a time gap but data before and after is intact                     |
| **recoverable** | Data appears lost but can be recovered via backfill, re-indexing, or file repair |

## Out of Scope

- Fixes for discovered issues (separate tickets)
- MPEG-TS format testing (fMP4 is the primary format; MPEG-TS can be audited separately)
- Network partition simulation requiring external tools (tc, iptables)
- Multi-node / distributed storage scenarios
