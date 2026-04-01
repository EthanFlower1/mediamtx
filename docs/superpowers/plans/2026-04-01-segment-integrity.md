# KAI-12: Segment Integrity Verification Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add integrity verification for fMP4 recording segments — validate structural completeness, detect corruption, and enable quarantine of bad segments.

**Architecture:** A new `internal/nvr/integrity/` package contains the core verification pipeline, background scanner, and quarantine logic. Verification runs inline on segment completion, periodically as a background scan, and on-demand via API. Results are stored as `status`/`status_detail`/`verified_at` columns on the existing `recordings` table. SSE events notify clients of corruption and quarantine actions.

**Tech Stack:** Go, SQLite (modernc.org/sqlite), gin HTTP framework, fMP4 box parsing

---

## File Structure

| File                                        | Responsibility                                                                                                  |
| ------------------------------------------- | --------------------------------------------------------------------------------------------------------------- |
| `internal/nvr/integrity/verifier.go`        | Core `VerifySegment()` function, `VerificationResult` type, fMP4 box-walking checks                             |
| `internal/nvr/integrity/verifier_test.go`   | Unit tests with crafted fMP4 fixtures                                                                           |
| `internal/nvr/integrity/scanner.go`         | Background scanner goroutine                                                                                    |
| `internal/nvr/integrity/scanner_test.go`    | Scanner batch/skip logic tests                                                                                  |
| `internal/nvr/integrity/quarantine.go`      | File move/restore operations                                                                                    |
| `internal/nvr/integrity/quarantine_test.go` | File move/restore tests                                                                                         |
| `internal/nvr/db/migrations.go`             | Migration 29: status columns + index                                                                            |
| `internal/nvr/db/recordings.go`             | New queries: `UpdateRecordingStatus`, `GetUnverifiedRecordings`, `GetIntegritySummary`, `GetRecordingsByFilter` |
| `internal/nvr/api/recordings.go`            | New endpoints: verify, integrity summary, quarantine, unquarantine                                              |
| `internal/nvr/api/events.go`                | `PublishSegmentCorrupted`, `PublishSegmentQuarantined` helpers                                                  |
| `internal/nvr/api/router.go`                | Register new routes                                                                                             |
| `internal/nvr/nvr.go`                       | Hook inline verification into `OnSegmentComplete`, start background scanner                                     |

---

### Task 1: Database Migration — Status Columns

**Files:**

- Modify: `internal/nvr/db/migrations.go:426` (after migration 28)
- Modify: `internal/nvr/db/recordings.go:13-24` (Recording struct)

- [ ] **Step 1: Add migration 29 to migrations.go**

Add this entry to the `migrations` slice in `internal/nvr/db/migrations.go`, after the last entry (version 28):

```go
// Migration 29: Recording integrity verification status.
{
    version: 29,
    sql: `
    ALTER TABLE recordings ADD COLUMN status TEXT NOT NULL DEFAULT 'unverified';
    ALTER TABLE recordings ADD COLUMN status_detail TEXT;
    ALTER TABLE recordings ADD COLUMN verified_at TEXT;
    CREATE INDEX idx_recordings_status ON recordings(status);
    `,
},
```

- [ ] **Step 2: Update Recording struct in recordings.go**

Add three fields to the `Recording` struct in `internal/nvr/db/recordings.go`:

```go
type Recording struct {
    ID           int64   `json:"id"`
    CameraID     string  `json:"camera_id"`
    StreamID     string  `json:"stream_id"`
    StartTime    string  `json:"start_time"`
    EndTime      string  `json:"end_time"`
    DurationMs   int64   `json:"duration_ms"`
    FilePath     string  `json:"file_path"`
    FileSize     int64   `json:"file_size"`
    Format       string  `json:"format"`
    InitSize     int64   `json:"init_size"`
    Status       string  `json:"status"`
    StatusDetail *string `json:"status_detail"`
    VerifiedAt   *string `json:"verified_at"`
}
```

`StatusDetail` and `VerifiedAt` are `*string` because they are nullable in the DB.

- [ ] **Step 3: Update all SELECT queries that scan into Recording**

Update `QueryRecordings` (line 164), `GetRecording` (line 227), `QueryRecordingsBestQuality` (line 364), and `GetUnindexedRecordings` (line 106) to include the new columns in their SELECT and Scan calls. For example, `QueryRecordings` becomes:

```go
func (d *DB) QueryRecordings(cameraID string, start, end time.Time) ([]*Recording, error) {
    rows, err := d.Query(`
        SELECT id, camera_id, stream_id, start_time, end_time, duration_ms, file_path, file_size, format, init_size, status, status_detail, verified_at
        FROM recordings
        WHERE camera_id = ? AND end_time > ? AND start_time < ?
        ORDER BY start_time`,
        cameraID, start.UTC().Format(timeFormat), end.UTC().Format(timeFormat),
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var recs []*Recording
    for rows.Next() {
        rec := &Recording{}
        if err := rows.Scan(
            &rec.ID, &rec.CameraID, &rec.StreamID, &rec.StartTime, &rec.EndTime,
            &rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format, &rec.InitSize,
            &rec.Status, &rec.StatusDetail, &rec.VerifiedAt,
        ); err != nil {
            return nil, err
        }
        recs = append(recs, rec)
    }
    return recs, rows.Err()
}
```

Apply the same pattern to `GetRecording`:

```go
func (d *DB) GetRecording(id int64) (*Recording, error) {
    rec := &Recording{}
    err := d.QueryRow(`
        SELECT id, camera_id, stream_id, start_time, end_time, duration_ms, file_path, file_size, format, init_size, status, status_detail, verified_at
        FROM recordings WHERE id = ?`, id,
    ).Scan(
        &rec.ID, &rec.CameraID, &rec.StreamID, &rec.StartTime, &rec.EndTime,
        &rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format, &rec.InitSize,
        &rec.Status, &rec.StatusDetail, &rec.VerifiedAt,
    )
    if errors.Is(err, sql.ErrNoRows) {
        return nil, ErrNotFound
    }
    if err != nil {
        return nil, err
    }
    return rec, nil
}
```

For `GetUnindexedRecordings`, add the new columns to SELECT and Scan:

```go
func (d *DB) GetUnindexedRecordings() ([]*Recording, error) {
    rows, err := d.Query(`
        SELECT r.id, r.camera_id, r.start_time, r.end_time, r.duration_ms, r.file_path, r.file_size, r.format, r.status, r.status_detail, r.verified_at
        FROM recordings r
        LEFT JOIN recording_fragments rf ON rf.recording_id = r.id
        WHERE rf.id IS NULL
        ORDER BY r.start_time DESC`)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var recs []*Recording
    for rows.Next() {
        rec := &Recording{}
        if err := rows.Scan(&rec.ID, &rec.CameraID, &rec.StartTime, &rec.EndTime,
            &rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format,
            &rec.Status, &rec.StatusDetail, &rec.VerifiedAt); err != nil {
            return nil, err
        }
        recs = append(recs, rec)
    }
    return recs, rows.Err()
}
```

For `QueryRecordingsBestQuality`, the inner call to `QueryRecordings` already returns the new fields, so no changes needed to the function body — it reuses `QueryRecordings`.

- [ ] **Step 4: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: Build succeeds with no errors.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/migrations.go internal/nvr/db/recordings.go
git commit -m "feat(integrity): add recording status columns (migration 29)"
```

---

### Task 2: DB Query Methods for Integrity

**Files:**

- Modify: `internal/nvr/db/recordings.go`

- [ ] **Step 1: Add UpdateRecordingStatus method**

Add to `internal/nvr/db/recordings.go`:

```go
// UpdateRecordingStatus sets the integrity verification status for a recording.
func (d *DB) UpdateRecordingStatus(id int64, status string, statusDetail *string, verifiedAt string) error {
    _, err := d.Exec(
        "UPDATE recordings SET status = ?, status_detail = ?, verified_at = ? WHERE id = ?",
        status, statusDetail, verifiedAt, id,
    )
    return err
}
```

- [ ] **Step 2: Add GetUnverifiedRecordings method**

```go
// GetUnverifiedRecordings returns recordings that need verification: either status='unverified'
// or verified_at older than the given cutoff. Results are ordered newest-first, limited to batchSize.
func (d *DB) GetRecordingsNeedingVerification(cutoff time.Time, batchSize int) ([]*Recording, error) {
    rows, err := d.Query(`
        SELECT id, camera_id, stream_id, start_time, end_time, duration_ms, file_path, file_size, format, init_size, status, status_detail, verified_at
        FROM recordings
        WHERE status = 'unverified' OR (verified_at IS NOT NULL AND verified_at < ?)
        ORDER BY start_time DESC
        LIMIT ?`,
        cutoff.UTC().Format(timeFormat), batchSize,
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var recs []*Recording
    for rows.Next() {
        rec := &Recording{}
        if err := rows.Scan(
            &rec.ID, &rec.CameraID, &rec.StreamID, &rec.StartTime, &rec.EndTime,
            &rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format, &rec.InitSize,
            &rec.Status, &rec.StatusDetail, &rec.VerifiedAt,
        ); err != nil {
            return nil, err
        }
        recs = append(recs, rec)
    }
    return recs, rows.Err()
}
```

- [ ] **Step 3: Add GetIntegritySummary method**

```go
// IntegritySummary holds aggregate counts of recording statuses.
type IntegritySummary struct {
    Total       int64 `json:"total"`
    OK          int64 `json:"ok"`
    Corrupted   int64 `json:"corrupted"`
    Quarantined int64 `json:"quarantined"`
    Unverified  int64 `json:"unverified"`
}

// GetIntegritySummary returns aggregate status counts, optionally filtered by camera.
func (d *DB) GetIntegritySummary(cameraID string) (*IntegritySummary, error) {
    var query string
    var args []interface{}
    if cameraID != "" {
        query = `SELECT
            COUNT(*) as total,
            SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END),
            SUM(CASE WHEN status = 'corrupted' THEN 1 ELSE 0 END),
            SUM(CASE WHEN status = 'quarantined' THEN 1 ELSE 0 END),
            SUM(CASE WHEN status = 'unverified' THEN 1 ELSE 0 END)
        FROM recordings WHERE camera_id = ?`
        args = append(args, cameraID)
    } else {
        query = `SELECT
            COUNT(*) as total,
            SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END),
            SUM(CASE WHEN status = 'corrupted' THEN 1 ELSE 0 END),
            SUM(CASE WHEN status = 'quarantined' THEN 1 ELSE 0 END),
            SUM(CASE WHEN status = 'unverified' THEN 1 ELSE 0 END)
        FROM recordings`
    }

    s := &IntegritySummary{}
    err := d.QueryRow(query, args...).Scan(&s.Total, &s.OK, &s.Corrupted, &s.Quarantined, &s.Unverified)
    if err != nil {
        return nil, err
    }
    return s, nil
}
```

- [ ] **Step 4: Add GetRecordingsByFilter for on-demand verify**

```go
// GetRecordingsByFilter returns recordings matching optional camera and time range filters.
func (d *DB) GetRecordingsByFilter(cameraID string, start, end *time.Time) ([]*Recording, error) {
    query := `SELECT id, camera_id, stream_id, start_time, end_time, duration_ms, file_path, file_size, format, init_size, status, status_detail, verified_at FROM recordings WHERE 1=1`
    var args []interface{}

    if cameraID != "" {
        query += " AND camera_id = ?"
        args = append(args, cameraID)
    }
    if start != nil {
        query += " AND end_time > ?"
        args = append(args, start.UTC().Format(timeFormat))
    }
    if end != nil {
        query += " AND start_time < ?"
        args = append(args, end.UTC().Format(timeFormat))
    }
    query += " ORDER BY start_time DESC"

    rows, err := d.Query(query, args...)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var recs []*Recording
    for rows.Next() {
        rec := &Recording{}
        if err := rows.Scan(
            &rec.ID, &rec.CameraID, &rec.StreamID, &rec.StartTime, &rec.EndTime,
            &rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format, &rec.InitSize,
            &rec.Status, &rec.StatusDetail, &rec.VerifiedAt,
        ); err != nil {
            return nil, err
        }
        recs = append(recs, rec)
    }
    return recs, rows.Err()
}
```

- [ ] **Step 5: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: Build succeeds.

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/db/recordings.go
git commit -m "feat(integrity): add DB queries for status updates, verification batch, and integrity summary"
```

---

### Task 3: Core Verification Pipeline

**Files:**

- Create: `internal/nvr/integrity/verifier.go`
- Create: `internal/nvr/integrity/verifier_test.go`

- [ ] **Step 1: Write the test file**

Create `internal/nvr/integrity/verifier_test.go`:

```go
package integrity

import (
    "encoding/binary"
    "os"
    "path/filepath"
    "testing"
)

// makeBox creates an MP4 box with the given 4-char type and payload.
func makeBox(boxType string, payload []byte) []byte {
    size := uint32(8 + len(payload))
    buf := make([]byte, size)
    binary.BigEndian.PutUint32(buf[0:4], size)
    copy(buf[4:8], boxType)
    copy(buf[8:], payload)
    return buf
}

// makeMoov creates a minimal moov box with an mvhd child.
func makeMoov() []byte {
    // mvhd: version(1) + flags(3) + creation(4) + mod(4) + timescale(4) + duration(4) + rest(80) = 100 bytes payload
    mvhdPayload := make([]byte, 100)
    binary.BigEndian.PutUint32(mvhdPayload[8:12], 90000) // timescale
    mvhd := makeBox("mvhd", mvhdPayload)
    return makeBox("moov", mvhd)
}

// makeMoofMdat creates a moof+mdat pair with dummy data of given size.
func makeMoofMdat(mdatPayloadSize int) []byte {
    moof := makeBox("moof", []byte{0, 0, 0, 0}) // minimal moof
    mdatPayload := make([]byte, mdatPayloadSize)
    mdat := makeBox("mdat", mdatPayload)
    result := make([]byte, 0, len(moof)+len(mdat))
    result = append(result, moof...)
    result = append(result, mdat...)
    return result
}

// writeValidFMP4 writes a minimal valid fMP4 file and returns the path.
func writeValidFMP4(t *testing.T, dir string) string {
    t.Helper()
    ftyp := makeBox("ftyp", []byte("isom\x00\x00\x00\x00"))
    moov := makeMoov()
    frag1 := makeMoofMdat(100)
    frag2 := makeMoofMdat(200)

    data := make([]byte, 0, len(ftyp)+len(moov)+len(frag1)+len(frag2))
    data = append(data, ftyp...)
    data = append(data, moov...)
    data = append(data, frag1...)
    data = append(data, frag2...)

    path := filepath.Join(dir, "valid.mp4")
    if err := os.WriteFile(path, data, 0o644); err != nil {
        t.Fatal(err)
    }
    return path
}

func TestVerifySegment_ValidFile(t *testing.T) {
    dir := t.TempDir()
    path := writeValidFMP4(t, dir)

    info, _ := os.Stat(path)
    ftyp := makeBox("ftyp", []byte("isom\x00\x00\x00\x00"))
    moov := makeMoov()
    initSize := int64(len(ftyp) + len(moov))

    rec := RecordingInfo{
        FilePath:      path,
        FileSize:      info.Size(),
        InitSize:      initSize,
        FragmentCount: 2,
        DurationMs:    0, // skip duration check when 0
    }

    result := VerifySegment(rec)
    if result.Status != StatusOK {
        t.Errorf("expected status ok, got %s: %s", result.Status, result.Detail)
    }
}

func TestVerifySegment_MissingFile(t *testing.T) {
    rec := RecordingInfo{
        FilePath: "/nonexistent/file.mp4",
        FileSize: 1000,
    }
    result := VerifySegment(rec)
    if result.Status != StatusCorrupted {
        t.Errorf("expected corrupted, got %s", result.Status)
    }
    if result.Detail != "file missing" {
        t.Errorf("unexpected detail: %s", result.Detail)
    }
}

func TestVerifySegment_SizeMismatch(t *testing.T) {
    dir := t.TempDir()
    path := writeValidFMP4(t, dir)

    rec := RecordingInfo{
        FilePath: path,
        FileSize: 999999, // wrong size
    }
    result := VerifySegment(rec)
    if result.Status != StatusCorrupted {
        t.Errorf("expected corrupted, got %s", result.Status)
    }
}

func TestVerifySegment_InvalidFtyp(t *testing.T) {
    dir := t.TempDir()
    data := makeBox("free", []byte("notftyp!"))
    path := filepath.Join(dir, "bad.mp4")
    os.WriteFile(path, data, 0o644)

    info, _ := os.Stat(path)
    rec := RecordingInfo{
        FilePath: path,
        FileSize: info.Size(),
    }
    result := VerifySegment(rec)
    if result.Status != StatusCorrupted {
        t.Errorf("expected corrupted, got %s", result.Status)
    }
}

func TestVerifySegment_TruncatedFile(t *testing.T) {
    dir := t.TempDir()
    ftyp := makeBox("ftyp", []byte("isom\x00\x00\x00\x00"))
    moov := makeMoov()
    frag := makeMoofMdat(100)

    // Write ftyp + moov + half of fragment (truncated)
    data := make([]byte, 0, len(ftyp)+len(moov)+len(frag)/2)
    data = append(data, ftyp...)
    data = append(data, moov...)
    data = append(data, frag[:len(frag)/2]...)

    path := filepath.Join(dir, "truncated.mp4")
    os.WriteFile(path, data, 0o644)

    info, _ := os.Stat(path)
    rec := RecordingInfo{
        FilePath: path,
        FileSize: info.Size(),
        InitSize: int64(len(ftyp) + len(moov)),
    }
    result := VerifySegment(rec)
    if result.Status != StatusCorrupted {
        t.Errorf("expected corrupted, got %s", result.Status)
    }
}

func TestVerifySegment_TrailingGarbage(t *testing.T) {
    dir := t.TempDir()
    ftyp := makeBox("ftyp", []byte("isom\x00\x00\x00\x00"))
    moov := makeMoov()
    frag := makeMoofMdat(100)

    data := make([]byte, 0, len(ftyp)+len(moov)+len(frag)+10)
    data = append(data, ftyp...)
    data = append(data, moov...)
    data = append(data, frag...)
    data = append(data, []byte("extratrash")...) // trailing garbage

    path := filepath.Join(dir, "trailing.mp4")
    os.WriteFile(path, data, 0o644)

    info, _ := os.Stat(path)
    rec := RecordingInfo{
        FilePath:      path,
        FileSize:      info.Size(),
        InitSize:      int64(len(ftyp) + len(moov)),
        FragmentCount: 1,
    }
    result := VerifySegment(rec)
    if result.Status != StatusCorrupted {
        t.Errorf("expected corrupted, got %s", result.Status)
    }
}

func TestVerifySegment_FragmentCountMismatch(t *testing.T) {
    dir := t.TempDir()
    path := writeValidFMP4(t, dir)

    info, _ := os.Stat(path)
    ftyp := makeBox("ftyp", []byte("isom\x00\x00\x00\x00"))
    moov := makeMoov()

    rec := RecordingInfo{
        FilePath:      path,
        FileSize:      info.Size(),
        InitSize:      int64(len(ftyp) + len(moov)),
        FragmentCount: 5, // file has 2 fragments, DB says 5
    }
    result := VerifySegment(rec)
    if result.Status != StatusCorrupted {
        t.Errorf("expected corrupted, got %s", result.Status)
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/integrity/...`
Expected: Compilation error — package doesn't exist yet.

- [ ] **Step 3: Implement verifier.go**

Create `internal/nvr/integrity/verifier.go`:

```go
// Package integrity provides recording segment integrity verification.
package integrity

import (
    "encoding/binary"
    "fmt"
    "io"
    "math"
    "os"
)

const (
    StatusOK          = "ok"
    StatusCorrupted   = "corrupted"
    StatusQuarantined = "quarantined"
    StatusUnverified  = "unverified"
)

// RecordingInfo contains the DB-side metadata needed for verification.
type RecordingInfo struct {
    FilePath      string
    FileSize      int64
    InitSize      int64
    FragmentCount int
    DurationMs    int64
}

// VerificationResult holds the outcome of verifying a single segment.
type VerificationResult struct {
    Status string // StatusOK or StatusCorrupted
    Detail string // empty if ok, failure reason if corrupted
}

// VerifySegment runs structural and metadata consistency checks on a recording segment.
// Checks are run in order and short-circuit on first failure.
func VerifySegment(rec RecordingInfo) VerificationResult {
    // 1. File existence
    info, err := os.Stat(rec.FilePath)
    if err != nil {
        return VerificationResult{Status: StatusCorrupted, Detail: "file missing"}
    }

    // 2. File size match
    if rec.FileSize > 0 && info.Size() != rec.FileSize {
        return VerificationResult{
            Status: StatusCorrupted,
            Detail: fmt.Sprintf("size mismatch: db=%d file=%d", rec.FileSize, info.Size()),
        }
    }

    f, err := os.Open(rec.FilePath)
    if err != nil {
        return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("cannot open file: %v", err)}
    }
    defer f.Close()

    fileSize := info.Size()

    // 3. ftyp box
    ftypSize, ftypType, err := readBoxHeader(f)
    if err != nil {
        return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("cannot read ftyp: %v", err)}
    }
    if ftypType != "ftyp" {
        return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("invalid ftyp box: got %q", ftypType)}
    }

    // 4. moov box
    if _, err := f.Seek(ftypSize, io.SeekStart); err != nil {
        return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("seek to moov failed: %v", err)}
    }
    moovSize, moovType, err := readBoxHeader(f)
    if err != nil {
        return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("cannot read moov: %v", err)}
    }
    if moovType != "moov" {
        return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("invalid/missing moov box: got %q", moovType)}
    }

    initSize := ftypSize + moovSize

    // 5. Init size consistency
    if rec.InitSize > 0 && initSize != rec.InitSize {
        return VerificationResult{
            Status: StatusCorrupted,
            Detail: fmt.Sprintf("init size mismatch: db=%d file=%d", rec.InitSize, initSize),
        }
    }

    // 6. Fragment walk
    pos := initSize
    fragmentCount := 0
    for pos < fileSize {
        if _, err := f.Seek(pos, io.SeekStart); err != nil {
            return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("seek failed at offset %d: %v", pos, err)}
        }

        moofSize, moofType, err := readBoxHeader(f)
        if err != nil {
            // If we're at EOF exactly, this is fine
            if pos == fileSize {
                break
            }
            return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("cannot read box at offset %d: %v", pos, err)}
        }

        // Skip non-moof boxes (e.g., Mtxi, free)
        if moofType != "moof" {
            if moofSize == 0 {
                break
            }
            pos += moofSize
            continue
        }

        // Read mdat after moof
        mdatPos := pos + moofSize
        if mdatPos >= fileSize {
            return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("truncated: moof at offset %d has no mdat", pos)}
        }
        if _, err := f.Seek(mdatPos, io.SeekStart); err != nil {
            return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("seek to mdat failed at offset %d: %v", mdatPos, err)}
        }
        mdatSize, mdatType, err := readBoxHeader(f)
        if err != nil {
            return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("cannot read mdat at offset %d: %v", mdatPos, err)}
        }
        if mdatType != "mdat" {
            return VerificationResult{Status: StatusCorrupted, Detail: fmt.Sprintf("expected mdat at offset %d, got %q", mdatPos, mdatType)}
        }

        if mdatSize == 0 {
            mdatSize = fileSize - mdatPos
        }

        // Verify mdat doesn't extend past file
        if mdatPos+mdatSize > fileSize {
            return VerificationResult{
                Status: StatusCorrupted,
                Detail: fmt.Sprintf("truncated mdat at offset %d: declares %d bytes but only %d remain", mdatPos, mdatSize, fileSize-mdatPos),
            }
        }

        fragmentCount++
        pos = mdatPos + mdatSize
    }

    // 7. Fragment count match
    if rec.FragmentCount > 0 && fragmentCount != rec.FragmentCount {
        return VerificationResult{
            Status: StatusCorrupted,
            Detail: fmt.Sprintf("fragment count mismatch: db=%d file=%d", rec.FragmentCount, fragmentCount),
        }
    }

    // 8. Duration consistency (skip if DB duration is 0 or not provided)
    // Duration check requires parsing trun entries which we skip in the lightweight verifier.
    // The fragment count and size checks already provide strong consistency guarantees.
    if rec.DurationMs > 0 {
        // We only flag a problem if there are zero fragments but a positive duration
        if fragmentCount == 0 {
            return VerificationResult{
                Status: StatusCorrupted,
                Detail: fmt.Sprintf("duration mismatch: db=%dms but file has 0 fragments", rec.DurationMs),
            }
        }
    }

    // 9. File completeness — check for trailing bytes after last valid box
    if pos != fileSize {
        trailing := fileSize - pos
        return VerificationResult{
            Status: StatusCorrupted,
            Detail: fmt.Sprintf("trailing garbage: %d bytes after last mdat", trailing),
        }
    }

    return VerificationResult{Status: StatusOK}
}

// readBoxHeader reads an MP4 box header (size + type).
func readBoxHeader(r io.ReadSeeker) (size int64, boxType string, err error) {
    var hdr [8]byte
    if _, err := io.ReadFull(r, hdr[:]); err != nil {
        return 0, "", err
    }

    size = int64(binary.BigEndian.Uint32(hdr[0:4]))
    boxType = string(hdr[4:8])

    if size == 1 {
        var ext [8]byte
        if _, err := io.ReadFull(r, ext[:]); err != nil {
            return 0, "", err
        }
        size = int64(binary.BigEndian.Uint64(ext[:]))
    }

    return size, boxType, nil
}

// durationDriftExceeds checks if the drift between two duration values exceeds a threshold percentage.
func durationDriftExceeds(dbMs, fileMs int64, thresholdPct float64) bool {
    if dbMs == 0 {
        return false
    }
    drift := math.Abs(float64(dbMs-fileMs)) / float64(dbMs) * 100
    return drift > thresholdPct
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/integrity/... -v`
Expected: All 7 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/integrity/verifier.go internal/nvr/integrity/verifier_test.go
git commit -m "feat(integrity): add core segment verification pipeline with tests"
```

---

### Task 4: Quarantine Operations

**Files:**

- Create: `internal/nvr/integrity/quarantine.go`
- Create: `internal/nvr/integrity/quarantine_test.go`

- [ ] **Step 1: Write the test file**

Create `internal/nvr/integrity/quarantine_test.go`:

```go
package integrity

import (
    "os"
    "path/filepath"
    "testing"
)

func TestQuarantineFile(t *testing.T) {
    dir := t.TempDir()
    recDir := filepath.Join(dir, "recordings", "cam1", "2026", "04")
    os.MkdirAll(recDir, 0o755)

    filePath := filepath.Join(recDir, "segment.mp4")
    os.WriteFile(filePath, []byte("test data"), 0o644)

    quarantineBase := filepath.Join(dir, "quarantine")
    recordingsBase := filepath.Join(dir, "recordings")

    newPath, err := QuarantineFile(filePath, recordingsBase, quarantineBase)
    if err != nil {
        t.Fatal(err)
    }

    // Original file should be gone
    if _, err := os.Stat(filePath); !os.IsNotExist(err) {
        t.Error("original file should not exist after quarantine")
    }

    // Quarantined file should exist with relative path preserved
    if _, err := os.Stat(newPath); err != nil {
        t.Errorf("quarantined file should exist at %s: %v", newPath, err)
    }

    expected := filepath.Join(quarantineBase, "cam1", "2026", "04", "segment.mp4")
    if newPath != expected {
        t.Errorf("expected quarantine path %s, got %s", expected, newPath)
    }

    // Content should be preserved
    data, _ := os.ReadFile(newPath)
    if string(data) != "test data" {
        t.Errorf("quarantined file content mismatch")
    }
}

func TestUnquarantineFile(t *testing.T) {
    dir := t.TempDir()
    quarantineDir := filepath.Join(dir, "quarantine", "cam1")
    os.MkdirAll(quarantineDir, 0o755)

    quarantinePath := filepath.Join(quarantineDir, "segment.mp4")
    os.WriteFile(quarantinePath, []byte("test data"), 0o644)

    recordingsBase := filepath.Join(dir, "recordings")
    quarantineBase := filepath.Join(dir, "quarantine")

    restoredPath, err := UnquarantineFile(quarantinePath, quarantineBase, recordingsBase)
    if err != nil {
        t.Fatal(err)
    }

    // Quarantined file should be gone
    if _, err := os.Stat(quarantinePath); !os.IsNotExist(err) {
        t.Error("quarantined file should not exist after restore")
    }

    // Restored file should exist
    if _, err := os.Stat(restoredPath); err != nil {
        t.Errorf("restored file should exist at %s: %v", restoredPath, err)
    }

    expected := filepath.Join(recordingsBase, "cam1", "segment.mp4")
    if restoredPath != expected {
        t.Errorf("expected restored path %s, got %s", expected, restoredPath)
    }
}

func TestQuarantineFile_MissingSource(t *testing.T) {
    _, err := QuarantineFile("/nonexistent/file.mp4", "/recordings", "/quarantine")
    if err == nil {
        t.Error("expected error for missing source file")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/integrity/... -run TestQuarantine -v`
Expected: Compilation error — functions don't exist yet.

- [ ] **Step 3: Implement quarantine.go**

Create `internal/nvr/integrity/quarantine.go`:

```go
package integrity

import (
    "fmt"
    "os"
    "path/filepath"
    "strings"
)

// QuarantineFile moves a recording file to the quarantine directory,
// preserving its relative path structure. Returns the new file path.
func QuarantineFile(filePath, recordingsBase, quarantineBase string) (string, error) {
    if _, err := os.Stat(filePath); err != nil {
        return "", fmt.Errorf("source file not found: %w", err)
    }

    relPath, err := filepath.Rel(recordingsBase, filePath)
    if err != nil {
        // Fallback: use the filename only
        relPath = filepath.Base(filePath)
    }
    // Ensure relPath doesn't escape (e.g. "../...")
    if strings.HasPrefix(relPath, "..") {
        relPath = filepath.Base(filePath)
    }

    destPath := filepath.Join(quarantineBase, relPath)
    destDir := filepath.Dir(destPath)

    if err := os.MkdirAll(destDir, 0o755); err != nil {
        return "", fmt.Errorf("create quarantine directory: %w", err)
    }

    if err := os.Rename(filePath, destPath); err != nil {
        return "", fmt.Errorf("move file to quarantine: %w", err)
    }

    return destPath, nil
}

// UnquarantineFile moves a quarantined file back to the recordings directory.
// Returns the restored file path.
func UnquarantineFile(quarantinePath, quarantineBase, recordingsBase string) (string, error) {
    if _, err := os.Stat(quarantinePath); err != nil {
        return "", fmt.Errorf("quarantined file not found: %w", err)
    }

    relPath, err := filepath.Rel(quarantineBase, quarantinePath)
    if err != nil {
        relPath = filepath.Base(quarantinePath)
    }
    if strings.HasPrefix(relPath, "..") {
        relPath = filepath.Base(quarantinePath)
    }

    destPath := filepath.Join(recordingsBase, relPath)
    destDir := filepath.Dir(destPath)

    if err := os.MkdirAll(destDir, 0o755); err != nil {
        return "", fmt.Errorf("create recordings directory: %w", err)
    }

    if err := os.Rename(quarantinePath, destPath); err != nil {
        return "", fmt.Errorf("move file from quarantine: %w", err)
    }

    return destPath, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/integrity/... -run TestQuarantine -v`
Expected: All 3 quarantine tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/integrity/quarantine.go internal/nvr/integrity/quarantine_test.go
git commit -m "feat(integrity): add quarantine file move/restore operations with tests"
```

---

### Task 5: Background Scanner

**Files:**

- Create: `internal/nvr/integrity/scanner.go`
- Create: `internal/nvr/integrity/scanner_test.go`

- [ ] **Step 1: Write the test file**

Create `internal/nvr/integrity/scanner_test.go`:

```go
package integrity

import (
    "context"
    "sync/atomic"
    "testing"
    "time"
)

func TestScanner_RunsVerification(t *testing.T) {
    var callCount atomic.Int32

    s := &Scanner{
        Interval:  50 * time.Millisecond,
        BatchSize: 10,
        FetchFunc: func(cutoff time.Time, batchSize int) ([]ScanItem, error) {
            if callCount.Load() > 0 {
                return nil, nil // only return items on first call
            }
            return []ScanItem{
                {
                    RecordingID: 1,
                    Info: RecordingInfo{
                        FilePath: "/nonexistent/file.mp4",
                        FileSize: 1000,
                    },
                },
            }, nil
        },
        OnResult: func(recordingID int64, result VerificationResult) {
            callCount.Add(1)
            if result.Status != StatusCorrupted {
                t.Errorf("expected corrupted status, got %s", result.Status)
            }
        },
    }

    ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
    defer cancel()

    go s.Run(ctx)

    <-ctx.Done()
    time.Sleep(20 * time.Millisecond) // let goroutine finish

    if callCount.Load() == 0 {
        t.Error("expected at least one verification call")
    }
}

func TestScanner_RespectsContext(t *testing.T) {
    s := &Scanner{
        Interval:  1 * time.Hour, // long interval
        BatchSize: 10,
        FetchFunc: func(cutoff time.Time, batchSize int) ([]ScanItem, error) {
            return nil, nil
        },
        OnResult: func(recordingID int64, result VerificationResult) {},
    }

    ctx, cancel := context.WithCancel(context.Background())
    done := make(chan struct{})
    go func() {
        s.Run(ctx)
        close(done)
    }()

    cancel()

    select {
    case <-done:
        // ok
    case <-time.After(1 * time.Second):
        t.Error("scanner did not stop after context cancellation")
    }
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/integrity/... -run TestScanner -v`
Expected: Compilation error.

- [ ] **Step 3: Implement scanner.go**

Create `internal/nvr/integrity/scanner.go`:

```go
package integrity

import (
    "context"
    "fmt"
    "os"
    "time"
)

// ScanItem pairs a recording ID with the info needed for verification.
type ScanItem struct {
    RecordingID int64
    CameraID    string
    Info        RecordingInfo
}

// Scanner runs periodic background integrity verification.
type Scanner struct {
    Interval  time.Duration
    BatchSize int
    FetchFunc func(cutoff time.Time, batchSize int) ([]ScanItem, error)
    OnResult  func(recordingID int64, result VerificationResult)
}

// Run starts the scanner loop. It blocks until ctx is cancelled.
func (s *Scanner) Run(ctx context.Context) {
    // Run immediately on start, then on interval.
    s.scan()

    ticker := time.NewTicker(s.Interval)
    defer ticker.Stop()

    for {
        select {
        case <-ctx.Done():
            return
        case <-ticker.C:
            s.scan()
        }
    }
}

func (s *Scanner) scan() {
    cutoff := time.Now().Add(-24 * time.Hour)
    items, err := s.FetchFunc(cutoff, s.BatchSize)
    if err != nil {
        fmt.Fprintf(os.Stderr, "NVR: integrity scanner fetch failed: %v\n", err)
        return
    }

    for _, item := range items {
        result := VerifySegment(item.Info)
        s.OnResult(item.RecordingID, result)
    }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/integrity/... -run TestScanner -v`
Expected: Both tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/integrity/scanner.go internal/nvr/integrity/scanner_test.go
git commit -m "feat(integrity): add background integrity scanner with tests"
```

---

### Task 6: SSE Event Helpers

**Files:**

- Modify: `internal/nvr/api/events.go`

- [ ] **Step 1: Add PublishSegmentCorrupted method**

Add to `internal/nvr/api/events.go`:

```go
// PublishSegmentCorrupted publishes a segment-corrupted event.
func (b *EventBroadcaster) PublishSegmentCorrupted(cameraID string, recordingID int64, filePath, detail string) {
    b.Publish(Event{
        Type:    "segment_corrupted",
        Camera:  cameraID,
        Message: fmt.Sprintf("Segment corrupted: %s", detail),
    })
}

// PublishSegmentQuarantined publishes a segment-quarantined event.
func (b *EventBroadcaster) PublishSegmentQuarantined(cameraID string, recordingID int64, quarantinePath string) {
    b.Publish(Event{
        Type:    "segment_quarantined",
        Camera:  cameraID,
        Message: fmt.Sprintf("Segment quarantined to %s", quarantinePath),
    })
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/api/...`
Expected: Build succeeds.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/events.go
git commit -m "feat(integrity): add SSE event helpers for segment corruption and quarantine"
```

---

### Task 7: API Endpoints

**Files:**

- Modify: `internal/nvr/api/recordings.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Add integrity summary endpoint to recordings.go**

Add to `internal/nvr/api/recordings.go`:

```go
// IntegritySummary returns aggregate integrity status counts.
// GET /api/nvr/recordings/integrity?camera_id=X (optional)
func (h *RecordingHandler) IntegritySummary(c *gin.Context) {
    cameraID := c.Query("camera_id")
    if cameraID != "" {
        if !hasCameraPermission(c, cameraID) {
            c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
            return
        }
    }

    summary, err := h.DB.GetIntegritySummary(cameraID)
    if err != nil {
        apiError(c, http.StatusInternalServerError, "failed to get integrity summary", err)
        return
    }

    c.JSON(http.StatusOK, summary)
}
```

- [ ] **Step 2: Add verify endpoint to recordings.go**

Add the `Verify` handler and its supporting `IntegrityHandler` struct. The `IntegrityHandler` needs access to the `EventBroadcaster` for SSE events, so we create a new handler struct:

```go
// IntegrityHandler implements HTTP endpoints for recording integrity operations.
type IntegrityHandler struct {
    DB             *db.DB
    Events         *EventBroadcaster
    RecordingsBase string
    QuarantineBase string
}

// VerifyRequest is the JSON body for the Verify endpoint.
type VerifyRequest struct {
    CameraID string `json:"camera_id"`
    Start    string `json:"start"`
    End      string `json:"end"`
}

// VerifyResponse is the response for the Verify endpoint.
type VerifyResponse struct {
    Total     int                    `json:"total"`
    OK        int                    `json:"ok"`
    Corrupted int                    `json:"corrupted"`
    Results   []VerifyResultEntry    `json:"results"`
}

// VerifyResultEntry is a single result in the verify response.
type VerifyResultEntry struct {
    RecordingID int64  `json:"recording_id"`
    CameraID    string `json:"camera_id"`
    FilePath    string `json:"file_path"`
    Status      string `json:"status"`
    Detail      string `json:"detail,omitempty"`
}
```

Now add the handler file. Since this has more substance, create a new file `internal/nvr/api/integrity.go`:

Actually, to keep the plan simpler and follow existing patterns, add the handlers directly in `recordings.go`:

```go
// Verify triggers integrity verification for recordings matching the given filters.
// POST /api/nvr/recordings/verify
func (h *IntegrityHandler) Verify(c *gin.Context) {
    var req VerifyRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        // Allow empty body — verifies all recordings
        req = VerifyRequest{}
    }

    if req.CameraID != "" {
        if !hasCameraPermission(c, req.CameraID) {
            c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
            return
        }
    }

    var startTime, endTime *time.Time
    if req.Start != "" {
        t, err := time.Parse(time.RFC3339, req.Start)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time"})
            return
        }
        startTime = &t
    }
    if req.End != "" {
        t, err := time.Parse(time.RFC3339, req.End)
        if err != nil {
            c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time"})
            return
        }
        endTime = &t
    }

    recordings, err := h.DB.GetRecordingsByFilter(req.CameraID, startTime, endTime)
    if err != nil {
        apiError(c, http.StatusInternalServerError, "failed to query recordings", err)
        return
    }

    resp := VerifyResponse{
        Total:   len(recordings),
        Results: make([]VerifyResultEntry, 0, len(recordings)),
    }

    now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

    for _, rec := range recordings {
        fragCount := 0
        if frags, err := h.DB.GetFragments(rec.ID); err == nil {
            fragCount = len(frags)
        }

        info := integrity.RecordingInfo{
            FilePath:      rec.FilePath,
            FileSize:      rec.FileSize,
            InitSize:      rec.InitSize,
            FragmentCount: fragCount,
            DurationMs:    rec.DurationMs,
        }

        result := integrity.VerifySegment(info)

        var detail *string
        if result.Detail != "" {
            detail = &result.Detail
        }
        h.DB.UpdateRecordingStatus(rec.ID, result.Status, detail, now)

        entry := VerifyResultEntry{
            RecordingID: rec.ID,
            CameraID:    rec.CameraID,
            FilePath:    rec.FilePath,
            Status:      result.Status,
            Detail:      result.Detail,
        }
        resp.Results = append(resp.Results, entry)

        if result.Status == integrity.StatusOK {
            resp.OK++
        } else {
            resp.Corrupted++
            if h.Events != nil {
                h.Events.PublishSegmentCorrupted(rec.CameraID, rec.ID, rec.FilePath, result.Detail)
            }
        }
    }

    c.JSON(http.StatusOK, resp)
}

// Quarantine moves a recording file to the quarantine directory.
// POST /api/nvr/recordings/:id/quarantine
func (h *IntegrityHandler) Quarantine(c *gin.Context) {
    idStr := c.Param("id")
    id, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid recording id"})
        return
    }

    rec, err := h.DB.GetRecording(id)
    if errors.Is(err, db.ErrNotFound) {
        c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
        return
    }
    if err != nil {
        apiError(c, http.StatusInternalServerError, "failed to get recording", err)
        return
    }

    if !hasCameraPermission(c, rec.CameraID) {
        c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
        return
    }

    if rec.Status == integrity.StatusQuarantined {
        c.JSON(http.StatusConflict, gin.H{"error": "recording is already quarantined"})
        return
    }

    newPath, err := integrity.QuarantineFile(rec.FilePath, h.RecordingsBase, h.QuarantineBase)
    if err != nil {
        apiError(c, http.StatusInternalServerError, "failed to quarantine file", err)
        return
    }

    now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
    detail := fmt.Sprintf("quarantined from %s", rec.FilePath)
    h.DB.UpdateRecordingStatus(id, integrity.StatusQuarantined, &detail, now)
    h.DB.UpdateRecordingFilePath(id, newPath)

    if h.Events != nil {
        h.Events.PublishSegmentQuarantined(rec.CameraID, id, newPath)
    }

    c.JSON(http.StatusOK, gin.H{
        "status":          "quarantined",
        "quarantine_path": newPath,
    })
}

// Unquarantine restores a quarantined recording file and re-verifies it.
// POST /api/nvr/recordings/:id/unquarantine
func (h *IntegrityHandler) Unquarantine(c *gin.Context) {
    idStr := c.Param("id")
    id, err := strconv.ParseInt(idStr, 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid recording id"})
        return
    }

    rec, err := h.DB.GetRecording(id)
    if errors.Is(err, db.ErrNotFound) {
        c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
        return
    }
    if err != nil {
        apiError(c, http.StatusInternalServerError, "failed to get recording", err)
        return
    }

    if !hasCameraPermission(c, rec.CameraID) {
        c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
        return
    }

    if rec.Status != integrity.StatusQuarantined {
        c.JSON(http.StatusConflict, gin.H{"error": "recording is not quarantined"})
        return
    }

    restoredPath, err := integrity.UnquarantineFile(rec.FilePath, h.QuarantineBase, h.RecordingsBase)
    if err != nil {
        apiError(c, http.StatusInternalServerError, "failed to restore file", err)
        return
    }

    h.DB.UpdateRecordingFilePath(id, restoredPath)

    // Re-verify the restored file
    fragCount := 0
    if frags, dbErr := h.DB.GetFragments(id); dbErr == nil {
        fragCount = len(frags)
    }
    info := integrity.RecordingInfo{
        FilePath:      restoredPath,
        FileSize:      rec.FileSize,
        InitSize:      rec.InitSize,
        FragmentCount: fragCount,
        DurationMs:    rec.DurationMs,
    }
    result := integrity.VerifySegment(info)

    now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
    var detail *string
    if result.Detail != "" {
        detail = &result.Detail
    }
    h.DB.UpdateRecordingStatus(id, result.Status, detail, now)

    c.JSON(http.StatusOK, gin.H{
        "status":    result.Status,
        "file_path": restoredPath,
    })
}
```

Note: This code goes in a new file `internal/nvr/api/integrity.go` since `recordings.go` is already substantial.

- [ ] **Step 3: Add import for integrity package**

The new file `internal/nvr/api/integrity.go` needs:

```go
package api

import (
    "errors"
    "fmt"
    "net/http"
    "strconv"
    "time"

    "github.com/gin-gonic/gin"

    "github.com/bluenviron/mediamtx/internal/nvr/db"
    "github.com/bluenviron/mediamtx/internal/nvr/integrity"
)
```

- [ ] **Step 4: Register routes in router.go**

Add the `IntegrityHandler` to `RouterConfig` in `internal/nvr/api/router.go`:

Add a new field to `RouterConfig`:

```go
QuarantineBase string // base path for quarantined files
```

In `RegisterRoutes`, after the `recordingHandler` initialization (around line 73), add:

```go
integrityHandler := &IntegrityHandler{
    DB:             cfg.DB,
    Events:         cfg.Events,
    RecordingsBase: cfg.RecordingsPath,
    QuarantineBase: cfg.QuarantineBase,
}
```

If `QuarantineBase` is empty, default it:

```go
quarantineBase := cfg.QuarantineBase
if quarantineBase == "" {
    quarantineBase = filepath.Join(cfg.RecordingsPath, ".quarantine")
}
integrityHandler := &IntegrityHandler{
    DB:             cfg.DB,
    Events:         cfg.Events,
    RecordingsBase: cfg.RecordingsPath,
    QuarantineBase: quarantineBase,
}
```

Add the import for `path/filepath` to router.go imports.

Then add routes after the existing recordings routes (around line 249):

```go
// Recording integrity.
protected.GET("/recordings/integrity", recordingHandler.IntegritySummary)
protected.POST("/recordings/verify", integrityHandler.Verify)
protected.POST("/recordings/:id/quarantine", integrityHandler.Quarantine)
protected.POST("/recordings/:id/unquarantine", integrityHandler.Unquarantine)
```

- [ ] **Step 5: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: Build succeeds.

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/api/integrity.go internal/nvr/api/recordings.go internal/nvr/api/router.go
git commit -m "feat(integrity): add verify, quarantine, unquarantine, and integrity summary API endpoints"
```

---

### Task 8: Inline Verification Hook & Background Scanner Start

**Files:**

- Modify: `internal/nvr/nvr.go`

- [ ] **Step 1: Add imports**

Add to the imports in `internal/nvr/nvr.go`:

```go
"github.com/bluenviron/mediamtx/internal/nvr/integrity"
```

- [ ] **Step 2: Add inline verification to OnSegmentComplete**

In `internal/nvr/nvr.go`, in the `OnSegmentComplete` method, after the fragment indexing block (around line 903, after `go n.indexRecordingFragments(rec)`), add inline verification:

```go
// Verify segment integrity inline (file is still in page cache).
go func() {
    fragCount := 0
    if frags, err := n.database.GetFragments(rec.ID); err == nil {
        fragCount = len(frags)
    }

    info := integrity.RecordingInfo{
        FilePath:      rec.FilePath,
        FileSize:      rec.FileSize,
        InitSize:      rec.InitSize,
        FragmentCount: fragCount,
        DurationMs:    rec.DurationMs,
    }
    result := integrity.VerifySegment(info)

    now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
    var detail *string
    if result.Detail != "" {
        detail = &result.Detail
    }
    n.database.UpdateRecordingStatus(rec.ID, result.Status, detail, now)

    if result.Status == integrity.StatusCorrupted && n.events != nil {
        n.events.PublishSegmentCorrupted(cam.ID, rec.ID, rec.FilePath, result.Detail)
    }
}()
```

Note: The inline verification runs as a goroutine because `indexRecordingFragments` also runs as a goroutine and populates fragment data. The verification will check whatever fragment count is available at that point. For newly-created segments, the init_size and file_size checks are the most important inline checks — fragment count will be validated by the background scanner once indexing completes.

- [ ] **Step 3: Start background scanner in Initialize**

In `internal/nvr/nvr.go`, add a `scanner` field to the NVR struct:

```go
integrityScanner *integrity.Scanner
```

In the `Initialize` method, after `n.startFragmentBackfill()` (around line 197), add:

```go
// Start background integrity scanner.
n.integrityScanner = &integrity.Scanner{
    Interval:  1 * time.Hour,
    BatchSize: 100,
    FetchFunc: func(cutoff time.Time, batchSize int) ([]integrity.ScanItem, error) {
        recs, err := n.database.GetRecordingsNeedingVerification(cutoff, batchSize)
        if err != nil {
            return nil, err
        }
        items := make([]integrity.ScanItem, 0, len(recs))
        for _, rec := range recs {
            fragCount := 0
            if frags, err := n.database.GetFragments(rec.ID); err == nil {
                fragCount = len(frags)
            }
            items = append(items, integrity.ScanItem{
                RecordingID: rec.ID,
                CameraID:    rec.CameraID,
                Info: integrity.RecordingInfo{
                    FilePath:      rec.FilePath,
                    FileSize:      rec.FileSize,
                    InitSize:      rec.InitSize,
                    FragmentCount: fragCount,
                    DurationMs:    rec.DurationMs,
                },
            })
        }
        return items, nil
    },
    OnResult: func(recordingID int64, result integrity.VerificationResult) {
        now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
        var detail *string
        if result.Detail != "" {
            detail = &result.Detail
        }
        n.database.UpdateRecordingStatus(recordingID, result.Status, detail, now)

        if result.Status == integrity.StatusCorrupted && n.events != nil {
            rec, err := n.database.GetRecording(recordingID)
            if err == nil {
                n.events.PublishSegmentCorrupted(rec.CameraID, recordingID, rec.FilePath, result.Detail)
            }
        }
    },
}
go n.integrityScanner.Run(n.ctx)
```

- [ ] **Step 4: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: Build succeeds.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/nvr.go
git commit -m "feat(integrity): hook inline verification into OnSegmentComplete and start background scanner"
```

---

### Task 9: Final Build Verification & Existing Tests

**Files:** None (verification only)

- [ ] **Step 1: Run all integrity tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/integrity/... -v`
Expected: All tests pass.

- [ ] **Step 2: Run full NVR package build**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: Build succeeds with no errors.

- [ ] **Step 3: Run existing NVR tests to check for regressions**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -v -count=1 2>&1 | head -100`
Expected: Existing tests pass (some may be skipped if they require external dependencies).

- [ ] **Step 4: Run linter if available**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go vet ./internal/nvr/...`
Expected: No vet errors.

- [ ] **Step 5: Commit any fixes if needed**

Only if previous steps revealed issues that needed fixing.
