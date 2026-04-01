# KAI-6: Graceful Recording Recovery

## Overview

Resume recording after crash/restart without segment corruption or gaps. On startup, detect incomplete fMP4 segments left by a previous crash, attempt repair by truncating to the last complete moof+mdat pair, and reconcile the database so repaired segments are indexed and playable.

## Startup Sequence

Recovery runs after DB migrations but before recorders start and before fragment backfill:

```
DB open → migrations → recovery scan → repair → reconcile → fragment backfill → start recorders
```

This ordering ensures:
- Recorders don't create files that collide with ones being repaired
- Fragment backfill operates on clean, repaired files
- Repaired segments are immediately available for playback

## Recovery Scanner

### Candidate Discovery

Two sources of incomplete segments:

1. **Orphaned files** — Walk recording directories, find `.mp4` files with no matching `file_path` in the `recordings` table. These exist when the process crashed before `OnSegmentComplete` fired.
2. **Unindexed DB entries** — Query recordings where the `recording_fragments` table has zero rows for that recording ID and status is not `quarantined`. These were inserted into the DB but never had fragments indexed (crash between insert and fragment scan).

### Filtering

- Skip files smaller than 8 bytes (cannot contain a valid ftyp box)
- Skip files that don't start with an ftyp box (not valid fMP4, log warning)
- Skip files in `.quarantine/` or `.recovery_failed/` directories

### Configuration

```go
type RecoveryConfig struct {
    RecordPaths []string // Recording directories to scan (derived from path configs)
    Enabled     bool     // Default: true
}
```

No new user-facing configuration — recovery is always enabled when NVR is active. The scanner derives recording directories from existing `RecordPath` configurations.

## fMP4 Repair

### Box Walking Algorithm

```
func RepairSegment(path string) (RepairResult, error):
  1. Open file, stat for size
  2. Read first 8 bytes, validate ftyp box type
  3. Walk ftyp box (skip by size)
  4. Walk moov box (validate type, skip by size)
  5. Record offset after moov as "content start"
  6. Walk remaining boxes from content start:
     - For each box, read 8-byte header (size + type)
     - If size == 1, read extended 64-bit size from next 8 bytes
     - Track pairs: expect moof followed by mdat
     - If box extends beyond EOF: this is the truncation point
     - Record offset of end of last complete mdat as "safe truncation point"
  7. If truncation needed:
     - Truncate file to safe truncation point
     - Return RepairResult{Repaired: true, ...}
  8. If file already complete (no truncation needed):
     - Return RepairResult{AlreadyComplete: true}
  9. If no complete moof+mdat pairs after moov:
     - Return RepairResult{Unrecoverable: true}
```

### RepairResult

```go
type RepairResult struct {
    Repaired           bool   // File was truncated to recover data
    AlreadyComplete    bool   // File was already structurally complete
    Unrecoverable      bool   // No complete fragments; file cannot be repaired
    OriginalSize       int64  // File size before repair
    NewSize            int64  // File size after repair (== OriginalSize if not repaired)
    FragmentsRecovered int    // Number of complete moof+mdat pairs found
    Detail             string // Human-readable description of what happened
}
```

### Edge Cases

| Scenario | Behavior |
|----------|----------|
| File has ftyp + moov but no moof/mdat | Unrecoverable — no media data |
| File truncated inside moov box | Unrecoverable — can't parse structure |
| File has complete fragments + partial trailing moof | Truncate after last complete mdat |
| File has complete fragments + partial trailing mdat | Truncate after the moof preceding the partial mdat (drop the incomplete pair) |
| File ends cleanly after last mdat | AlreadyComplete — no repair needed |
| File has unexpected box type (not moof/mdat) after moov | Stop walking, truncate at that point, log warning |

## DB Reconciliation

### Orphaned Files (no DB entry)

For each repaired orphaned file:

1. Parse filename using `recordstore.DecodePath` to extract path name and start time
2. Estimate duration from the repaired file's fragment data (sum of moof durations via `api.ScanFragments`)
3. Get file size after repair
4. Match to a camera by path name lookup
5. Insert recording entry:
   ```go
   rec := &db.Recording{
       CameraID:   cameraID,
       StreamID:   streamID,
       StartTime:  startTime,
       EndTime:    startTime.Add(duration),
       DurationMs: duration.Milliseconds(),
       FilePath:   filePath,
       FileSize:   repairedSize,
       Format:     "fmp4",
   }
   ```
6. Fragment indexing happens in the subsequent backfill phase

### Existing DB Entries (stale data)

For recordings found via the unindexed query:

1. Re-stat the file — if missing, mark recording as `corrupted` with detail `"file missing after crash"`
2. If file exists, run `RepairSegment` on it
3. If repaired: update `file_size` in DB to match new size
4. If unrecoverable: mark as `corrupted` with detail from RepairResult
5. Clear any stale fragment rows so backfill re-indexes from scratch

### Unrecoverable Files

Files that cannot be repaired:

1. If KAI-12 integrity columns exist: set `status = 'corrupted'`, `status_detail` = repair failure reason
2. If KAI-12 is not yet deployed: move file to `{recordDir}/.recovery_failed/` with original relative path preserved, log warning
3. Never delete files automatically — leave that to the admin or the retention cleaner

## Resume Without Gaps

No special logic needed. The recorder already generates timestamp-based filenames for new segments. After recovery completes and recorders start, new segments get new filenames. The gap between the last recovered frame and the first new frame reflects the actual outage duration. The DB timeline accurately represents this gap.

## Code Organization

### New Package: `internal/nvr/recovery/`

| File | Purpose |
|------|---------|
| `recovery.go` | `Run()` entry point, orchestrates scan → repair → reconcile |
| `scanner.go` | Directory walking, orphan detection, candidate filtering |
| `repair.go` | fMP4 box walking, truncation, `RepairSegment()` function |
| `reconcile.go` | DB insert/update for recovered segments |
| `repair_test.go` | Unit tests for repair with crafted fMP4 fixtures |
| `scanner_test.go` | Unit tests for candidate discovery |
| `reconcile_test.go` | Unit tests for DB operations |
| `recovery_test.go` | Integration test: record → SIGKILL → recover → verify |

### Integration Points

| File | Change |
|------|--------|
| `internal/nvr/nvr.go` | Call `recovery.Run()` during `Initialize()`, before backfill goroutine starts |
| `internal/nvr/db/recordings.go` | Add `GetOrphanedFiles()` query, `GetUnindexedRecordings()` query, `UpdateRecordingFileSize()` |
| `internal/nvr/db/migrations.go` | No new migrations — uses existing schema (leverages KAI-12 status columns if present) |

### Dependencies

- `internal/recordstore` — path parsing and segment discovery
- `internal/nvr/db` — database queries
- `internal/nvr/api` — `ScanFragments()` for duration estimation after repair
- No new external dependencies

## Logging

All recovery actions logged at appropriate levels:

| Event | Level |
|-------|-------|
| Recovery scan started | Info |
| Orphaned file found | Info |
| Segment repaired successfully | Info (with original/new size, fragments recovered) |
| Segment already complete | Debug |
| Segment unrecoverable | Warn (with detail) |
| File moved to recovery_failed | Warn |
| DB entry reconciled | Debug |
| Recovery scan complete (summary) | Info (total scanned, repaired, unrecoverable, already complete) |

## Test Strategy

### Unit Tests

**repair_test.go** — Craft minimal fMP4 files in memory:

| Test | Input | Expected |
|------|-------|----------|
| `TestRepairCompleteFile` | Valid ftyp + moov + 2x(moof+mdat) | AlreadyComplete = true |
| `TestRepairTruncatedMdat` | Valid structure, last mdat truncated mid-way | Repaired = true, file truncated before incomplete mdat |
| `TestRepairTruncatedMoof` | Valid structure, partial moof at end | Repaired = true, file truncated before incomplete moof |
| `TestRepairNoFragments` | ftyp + moov only | Unrecoverable = true |
| `TestRepairTruncatedMoov` | ftyp + partial moov | Unrecoverable = true |
| `TestRepairEmptyFile` | 0 bytes | Error returned |
| `TestRepairTooSmall` | 4 bytes | Error returned |
| `TestRepairNotFMP4` | Random bytes | Error returned |

**scanner_test.go** — Temp directories with planted files:

| Test | Setup | Expected |
|------|-------|----------|
| `TestScanFindsOrphans` | Files on disk, empty DB | All files returned as candidates |
| `TestScanSkipsIndexed` | Files on disk, matching DB entries with fragments | No candidates |
| `TestScanFindsUnindexed` | DB entries with no fragments | Entries returned as candidates |
| `TestScanSkipsQuarantined` | Files in .quarantine/ | Skipped |
| `TestScanSkipsSmallFiles` | Files < 8 bytes | Skipped |

**reconcile_test.go** — Mock DB:

| Test | Input | Expected |
|------|-------|----------|
| `TestReconcileOrphanedFile` | Repaired file, no DB entry | New recording inserted |
| `TestReconcileUnindexedEntry` | DB entry, repaired file | file_size updated |
| `TestReconcileUnrecoverable` | Unrecoverable result | Status set to corrupted (if KAI-12) or file moved |
| `TestReconcileMissingFile` | DB entry, file deleted | Status set to corrupted |

### Integration Test

**recovery_test.go** — Full pipeline:

1. Build `auditrecord` subprocess (reuse KAI-5 infrastructure)
2. Start recording for 5 seconds
3. SIGKILL the process
4. Run `recovery.Run()` against the recording directory and DB
5. Verify: repaired file has valid ftyp, complete moof+mdat pairs, correct truncation
6. Verify: DB has recording entry with correct file size
7. Run fragment backfill, verify fragments indexed
8. Start a new recorder to same directory, verify no filename collision
