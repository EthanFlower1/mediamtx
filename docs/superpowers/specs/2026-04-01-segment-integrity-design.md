# KAI-12: Recording Segment Integrity Verification

## Overview

Add integrity verification for recording segments to detect corruption, validate structural completeness, and enable quarantine of bad segments. Verification runs inline at segment completion, as a periodic background scan, and on-demand via API.

## Data Model

### Schema Changes

New migration adds three columns to the `recordings` table:

```sql
ALTER TABLE recordings ADD COLUMN status TEXT NOT NULL DEFAULT 'unverified';
ALTER TABLE recordings ADD COLUMN status_detail TEXT;
ALTER TABLE recordings ADD COLUMN verified_at TEXT;
CREATE INDEX idx_recordings_status ON recordings(status);
```

**Status values:**

| Status        | Meaning                                            |
| ------------- | -------------------------------------------------- |
| `unverified`  | Not yet checked (default, covers existing records) |
| `ok`          | Passed all checks                                  |
| `corrupted`   | Failed one or more checks                          |
| `quarantined` | File moved to quarantine directory                 |

- `status_detail` — Human-readable failure reason (e.g., `"truncated: expected 4521984 bytes, got 2097152"`). Null when status is `ok` or `unverified`.
- `verified_at` — RFC3339 timestamp of last verification. Used by background scanner to skip recently-verified segments.

### Quarantine Directory

- Configurable path, defaults to `{recordPath}/.quarantine/`
- Files moved with original relative path preserved (e.g., `.quarantine/cam1/2026/04/01/12-00-00.mp4`)
- DB `file_path` updated to quarantine location, status set to `quarantined`

## Verification Pipeline

### Check Sequence

A single `verifySegment(recording Recording) VerificationResult` function runs these checks in order, short-circuiting on first failure:

1. **File existence** — `os.Stat` the file path. Fail: `"file missing"`
2. **File size match** — Compare stat size to DB `file_size`. Fail: `"size mismatch: db=X file=Y"`
3. **ftyp box** — Read first 8 bytes, validate box type is `ftyp`. Fail: `"invalid ftyp box"`
4. **moov box** — Read next box header after ftyp, validate type is `moov`, size is reasonable. Fail: `"invalid/missing moov box"`
5. **Init size consistency** — Check `ftyp_size + moov_size` matches DB `init_size`. Fail: `"init size mismatch: db=X file=Y"`
6. **Fragment walk** — Starting after init, iterate moof+mdat pairs. Validate each box has correct type and size doesn't exceed remaining file. Fail: `"unexpected box type 'X' at offset Y"` or `"truncated mdat at offset Y"`
7. **Fragment count match** — Compare walked fragment count to DB `recording_fragments` count. Fail: `"fragment count mismatch: db=X file=Y"`
8. **Duration consistency** — Sum fragment durations from file, compare to DB `duration_ms`. Fail if drift > 5%. Fail: `"duration mismatch: db=Xms file=Yms"`
9. **File completeness** — After last mdat, remaining bytes should be 0. Fail: `"trailing garbage: X bytes after last mdat"`

Short-circuits on first failure since later checks may be meaningless if structure is broken.

### Return Type

```go
type VerificationResult struct {
    Status       string   // "ok" or "corrupted"
    Detail       string   // empty if ok, failure reason if corrupted
    ChecksFailed []string // which check(s) failed
}
```

## Execution Modes

### Inline Verification

Hooked into the `OnSegmentComplete` callback path. After a segment is finalized and its DB record is inserted, `verifySegment()` runs on it.

- Runs synchronously in the callback goroutine (fast — just header reads, no media decode)
- File is still in page cache, so I/O is essentially free
- Updates `status`, `status_detail`, `verified_at` in DB
- Emits SSE event immediately if corrupted

### Background Scanner

A goroutine on a configurable interval (default: 1 hour). Each scan:

1. Query recordings where `status = 'unverified'` OR `verified_at` older than 24 hours, ordered by `start_time DESC`, batch size 100
2. Run `verifySegment()` on each
3. Update DB with results
4. Emit SSE events for any newly-detected corruption

Rate-limited: one segment at a time with small delay between checks to avoid I/O storms.

Catches: pre-existing segments, segments where inline verification was skipped (process crash), and re-validates old segments that may have suffered post-write disk corruption.

### On-Demand API

```
POST /api/nvr/recordings/verify
  Body: { "camera_id": "cam1", "start": "...", "end": "..." }
  Response: { "total": 50, "ok": 48, "corrupted": 2, "results": [...] }
```

Triggers immediate verification of matching segments. Same `verifySegment()` function.

## Quarantine API

```
POST /api/nvr/recordings/{id}/quarantine
  Response: { "status": "quarantined", "quarantine_path": "..." }

POST /api/nvr/recordings/{id}/unquarantine
  Response: { "status": "ok", "file_path": "..." }
```

- Quarantine moves the file to the quarantine directory and updates DB
- Unquarantine moves it back, re-runs verification, sets status based on result

## SSE Events

Two new event types on the existing SSE stream:

### `segment_corrupted`

Emitted when verification detects corruption (inline or background).

```json
{
  "type": "segment_corrupted",
  "camera_id": "cam1",
  "recording_id": 142,
  "file_path": "/recordings/cam1/2026/04/01/12-00-00.mp4",
  "detail": "truncated: expected 4521984 bytes, got 2097152",
  "timestamp": "2026-04-01T12:01:05Z"
}
```

### `segment_quarantined`

Emitted when a segment is quarantined via API.

```json
{
  "type": "segment_quarantined",
  "camera_id": "cam1",
  "recording_id": 142,
  "quarantine_path": "/recordings/.quarantine/cam1/2026/04/01/12-00-00.mp4",
  "timestamp": "2026-04-01T12:05:00Z"
}
```

## API Extensions

### Extended Recording Response

Existing `GET /api/nvr/recordings` gains three fields per recording:

```json
{
  "status": "ok",
  "status_detail": null,
  "verified_at": "2026-04-01T12:01:05Z"
}
```

### Integrity Summary

```
GET /api/nvr/recordings/integrity
  Query: ?camera_id=cam1 (optional)
  Response: {
    "total": 500,
    "ok": 485,
    "corrupted": 3,
    "quarantined": 1,
    "unverified": 11
  }
```

## Code Organization

### New Package: `internal/nvr/integrity/`

| File                 | Purpose                                                     |
| -------------------- | ----------------------------------------------------------- |
| `verifier.go`        | Core `verifySegment()` function, `VerificationResult` type  |
| `scanner.go`         | Background scanner goroutine (interval loop, batch queries) |
| `quarantine.go`      | File move/restore operations                                |
| `verifier_test.go`   | Unit tests with crafted fMP4 fixtures                       |
| `scanner_test.go`    | Background scanner tests                                    |
| `quarantine_test.go` | File move/restore tests                                     |

### Integration Points

| File                             | Change                                                                            |
| -------------------------------- | --------------------------------------------------------------------------------- |
| `internal/nvr/db/recordings.go`  | `UpdateRecordingStatus`, `GetUnverifiedRecordings`, `GetIntegritySummary` queries |
| `internal/nvr/db/migrations.go`  | New migration for status columns                                                  |
| `internal/nvr/api/recordings.go` | `/verify`, `/integrity`, `/{id}/quarantine`, `/{id}/unquarantine` endpoints       |
| `internal/nvr/api/events.go`     | `segment_corrupted` and `segment_quarantined` event types                         |
| `internal/core/path.go`          | Hook inline verification into `OnSegmentComplete` callback                        |

### Test Strategy

- **Unit tests** for `verifier.go`: Craft minimal valid fMP4 files in-memory, create variants with each corruption type
- **Unit tests** for `scanner.go`: Mock DB, verify batch processing and skip logic
- **Unit tests** for `quarantine.go`: Temp directories, verify file move/restore and path preservation
- **Integration test**: Record a real segment, verify it passes; truncate it, verify corruption detected
