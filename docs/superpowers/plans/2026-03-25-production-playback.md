# Production Playback Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix playback performance and reliability by pre-indexing fMP4 fragments in SQLite, rewriting HLS playlist generation to use the index, and fixing client-side seeking/gap bugs.

**Architecture:** Add a `recording_fragments` table that stores per-fragment byte offsets, sizes, and real durations. The recorder populates it on segment complete. HLS playlist generation switches from file scanning to DB queries. Client-side PlaybackController bugs (snap-back, error handling, gap snapping) are fixed independently.

**Tech Stack:** Go (server), SQLite, fMP4/ISO BMFF parsing, Flutter/Dart (client), media_kit (HLS player)

---

### Task 1: Add recording_fragments DB migration and model

**Files:**
- Modify: `internal/nvr/db/migrations.go:214-218` (add migration version 16)
- Modify: `internal/nvr/db/recordings.go` (add RecordingFragment struct and DB methods)

- [ ] **Step 1: Add migration version 16**

In `internal/nvr/db/migrations.go`, add after the version 15 entry (line 217):

```go
{
    version: 16,
    sql: `
CREATE TABLE recording_fragments (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    recording_id INTEGER NOT NULL,
    fragment_index INTEGER NOT NULL,
    byte_offset INTEGER NOT NULL,
    size INTEGER NOT NULL,
    duration_ms REAL NOT NULL,
    is_keyframe INTEGER NOT NULL DEFAULT 1,
    timestamp_ms INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE CASCADE,
    UNIQUE(recording_id, fragment_index)
);
CREATE INDEX idx_fragments_recording ON recording_fragments(recording_id, fragment_index);
ALTER TABLE recordings ADD COLUMN init_size INTEGER NOT NULL DEFAULT 0;
`,
},
```

- [ ] **Step 2: Add RecordingFragment struct and DB methods**

In `internal/nvr/db/recordings.go`, add after the `Recording` struct (line 21):

```go
// RecordingFragment represents a single moof+mdat fragment within an fMP4 recording.
type RecordingFragment struct {
    ID            int64   `json:"id"`
    RecordingID   int64   `json:"recording_id"`
    FragmentIndex int     `json:"fragment_index"`
    ByteOffset    int64   `json:"byte_offset"`
    Size          int64   `json:"size"`
    DurationMs    float64 `json:"duration_ms"`
    IsKeyframe    bool    `json:"is_keyframe"`
    TimestampMs   int64   `json:"timestamp_ms"`
}

// InsertFragments bulk-inserts fragment metadata for a recording using INSERT OR IGNORE
// to ensure idempotency.
func (d *DB) InsertFragments(recordingID int64, fragments []RecordingFragment) error {
    tx, err := d.Begin()
    if err != nil {
        return err
    }
    defer tx.Rollback()

    stmt, err := tx.Prepare(`
        INSERT OR IGNORE INTO recording_fragments
        (recording_id, fragment_index, byte_offset, size, duration_ms, is_keyframe, timestamp_ms)
        VALUES (?, ?, ?, ?, ?, ?, ?)`)
    if err != nil {
        return err
    }
    defer stmt.Close()

    for _, f := range fragments {
        _, err := stmt.Exec(f.RecordingID, f.FragmentIndex, f.ByteOffset, f.Size,
            f.DurationMs, f.IsKeyframe, f.TimestampMs)
        if err != nil {
            return err
        }
    }

    return tx.Commit()
}

// UpdateRecordingInitSize sets the init_size (ftyp + moov byte length) for a recording.
func (d *DB) UpdateRecordingInitSize(recordingID int64, initSize int64) error {
    _, err := d.Exec("UPDATE recordings SET init_size = ? WHERE id = ?", initSize, recordingID)
    return err
}

// GetFragments returns all fragments for a recording, ordered by fragment_index.
func (d *DB) GetFragments(recordingID int64) ([]RecordingFragment, error) {
    rows, err := d.Query(`
        SELECT id, recording_id, fragment_index, byte_offset, size, duration_ms, is_keyframe, timestamp_ms
        FROM recording_fragments
        WHERE recording_id = ?
        ORDER BY fragment_index`, recordingID)
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var frags []RecordingFragment
    for rows.Next() {
        var f RecordingFragment
        if err := rows.Scan(&f.ID, &f.RecordingID, &f.FragmentIndex, &f.ByteOffset,
            &f.Size, &f.DurationMs, &f.IsKeyframe, &f.TimestampMs); err != nil {
            return nil, err
        }
        frags = append(frags, f)
    }
    return frags, rows.Err()
}

// HasFragments checks whether a recording has been indexed (has fragment rows).
func (d *DB) HasFragments(recordingID int64) (bool, error) {
    var count int
    err := d.QueryRow("SELECT COUNT(*) FROM recording_fragments WHERE recording_id = ?", recordingID).Scan(&count)
    return count > 0, err
}

// GetUnindexedRecordings returns recording IDs that have no fragments, newest first.
func (d *DB) GetUnindexedRecordings() ([]*Recording, error) {
    rows, err := d.Query(`
        SELECT r.id, r.camera_id, r.start_time, r.end_time, r.duration_ms, r.file_path, r.file_size, r.format
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
            &rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format); err != nil {
            return nil, err
        }
        recs = append(recs, rec)
    }
    return recs, rows.Err()
}
```

Also add `InitSize` field to the `Recording` struct:

```go
type Recording struct {
    ID         int64  `json:"id"`
    CameraID   string `json:"camera_id"`
    StartTime  string `json:"start_time"`
    EndTime    string `json:"end_time"`
    DurationMs int64  `json:"duration_ms"`
    FilePath   string `json:"file_path"`
    FileSize   int64  `json:"file_size"`
    Format     string `json:"format"`
    InitSize   int64  `json:"init_size"`
}
```

Update all existing `Scan` calls in `recordings.go` that read from `recordings` to also scan `init_size`:
- `QueryRecordings` (line 72): add `&rec.InitSize` to Scan
- `GetRecording` (line 122): add `&rec.InitSize` to Scan
- `GetUnindexedRecordings`: already includes it above
- Update the SELECT columns in each query to include `init_size`

- [ ] **Step 3: Verify migration runs**

Run: `go build ./...`
Expected: Compiles successfully. Migration will auto-run on next server start.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/db/migrations.go internal/nvr/db/recordings.go
git commit -m "feat: add recording_fragments table and DB methods for fragment indexing"
```

---

### Task 2: Extract real fragment durations from fMP4 trun boxes

**Files:**
- Modify: `internal/nvr/api/hls.go:24-28,193-283` (enhance scanFragments to extract timing)

- [ ] **Step 1: Update fragmentInfo to include duration**

In `internal/nvr/api/hls.go`, replace the `fragmentInfo` struct (lines 24-28):

```go
// fragmentInfo describes a single moof+mdat pair inside an fMP4 file.
type fragmentInfo struct {
    offset     int64
    size       int64
    durationMs float64 // actual duration in milliseconds, from trun/tfhd
}
```

- [ ] **Step 2: Add trun/tfhd/mvhd parsing helpers**

Add these functions in `internal/nvr/api/hls.go` after `readBoxHeader`:

```go
// readTimescale reads the timescale from the mvhd box inside moov.
// It seeks through moov's children looking for mvhd.
func readTimescale(f io.ReadSeeker, moovStart, moovSize int64) (uint32, error) {
    pos := moovStart + 8 // skip moov container header to scan children
    end := moovStart + moovSize
    for pos < end {
        if _, err := f.Seek(pos, io.SeekStart); err != nil {
            return 0, err
        }
        boxSize, boxType, err := readBoxHeader(f)
        if err != nil {
            return 0, err
        }
        if boxType == "mvhd" {
            // mvhd: version(1) + flags(3) + creation(4/8) + modification(4/8) + timescale(4)
            var ver [1]byte
            if _, err := io.ReadFull(f, ver[:]); err != nil {
                return 0, err
            }
            // Skip flags (3 bytes)
            if _, err := f.Seek(3, io.SeekCurrent); err != nil {
                return 0, err
            }
            if ver[0] == 0 {
                // Skip creation_time(4) + modification_time(4)
                if _, err := f.Seek(8, io.SeekCurrent); err != nil {
                    return 0, err
                }
            } else {
                // Skip creation_time(8) + modification_time(8)
                if _, err := f.Seek(16, io.SeekCurrent); err != nil {
                    return 0, err
                }
            }
            var ts [4]byte
            if _, err := io.ReadFull(f, ts[:]); err != nil {
                return 0, err
            }
            return binary.BigEndian.Uint32(ts[:]), nil
        }
        if boxSize == 0 {
            break
        }
        pos += boxSize
    }
    return 0, fmt.Errorf("mvhd not found in moov")
}

// readFragmentDuration reads the total sample duration from a moof box.
// It looks for traf > tfhd (default_sample_duration) and trun (per-sample durations).
func readFragmentDuration(f io.ReadSeeker, moofStart, moofSize int64, timescale uint32) (float64, error) {
    pos := moofStart + 8 // skip moof header
    end := moofStart + moofSize

    var defaultSampleDuration uint32
    var totalDuration uint64

    for pos < end {
        if _, err := f.Seek(pos, io.SeekStart); err != nil {
            return 0, err
        }
        boxSize, boxType, err := readBoxHeader(f)
        if err != nil {
            return 0, err
        }

        if boxType == "traf" {
            // Parse traf children inline
            trafEnd := pos + boxSize
            childPos := pos + 8 // skip traf header
            for childPos < trafEnd {
                if _, err := f.Seek(childPos, io.SeekStart); err != nil {
                    return 0, err
                }
                childSize, childType, err := readBoxHeader(f)
                if err != nil {
                    break
                }

                if childType == "tfhd" {
                    // version(1) + flags(3)
                    var vf [4]byte
                    if _, err := io.ReadFull(f, vf[:]); err != nil {
                        break
                    }
                    flags := uint32(vf[1])<<16 | uint32(vf[2])<<8 | uint32(vf[3])
                    // Skip track_id (4 bytes)
                    if _, err := f.Seek(4, io.SeekCurrent); err != nil {
                        break
                    }
                    // Skip optional fields based on flags
                    if flags&0x000001 != 0 { // base-data-offset-present
                        if _, err := f.Seek(8, io.SeekCurrent); err != nil {
                            break
                        }
                    }
                    if flags&0x000002 != 0 { // sample-description-index-present
                        if _, err := f.Seek(4, io.SeekCurrent); err != nil {
                            break
                        }
                    }
                    if flags&0x000008 != 0 { // default-sample-duration-present
                        var dur [4]byte
                        if _, err := io.ReadFull(f, dur[:]); err != nil {
                            break
                        }
                        defaultSampleDuration = binary.BigEndian.Uint32(dur[:])
                    }
                }

                if childType == "trun" {
                    var vf [4]byte
                    if _, err := io.ReadFull(f, vf[:]); err != nil {
                        break
                    }
                    flags := uint32(vf[1])<<16 | uint32(vf[2])<<8 | uint32(vf[3])
                    var sc [4]byte
                    if _, err := io.ReadFull(f, sc[:]); err != nil {
                        break
                    }
                    sampleCount := binary.BigEndian.Uint32(sc[:])

                    // Skip optional data-offset (4 bytes) and first-sample-flags (4 bytes)
                    if flags&0x000001 != 0 {
                        if _, err := f.Seek(4, io.SeekCurrent); err != nil {
                            break
                        }
                    }
                    if flags&0x000004 != 0 {
                        if _, err := f.Seek(4, io.SeekCurrent); err != nil {
                            break
                        }
                    }

                    hasDuration := flags&0x000100 != 0
                    hasSize := flags&0x000200 != 0
                    hasFlags := flags&0x000400 != 0
                    hasCTO := flags&0x000800 != 0

                    for i := uint32(0); i < sampleCount; i++ {
                        if hasDuration {
                            var d [4]byte
                            if _, err := io.ReadFull(f, d[:]); err != nil {
                                break
                            }
                            totalDuration += uint64(binary.BigEndian.Uint32(d[:]))
                        } else {
                            totalDuration += uint64(defaultSampleDuration)
                        }
                        if hasSize {
                            if _, err := f.Seek(4, io.SeekCurrent); err != nil {
                                break
                            }
                        }
                        if hasFlags {
                            if _, err := f.Seek(4, io.SeekCurrent); err != nil {
                                break
                            }
                        }
                        if hasCTO {
                            if _, err := f.Seek(4, io.SeekCurrent); err != nil {
                                break
                            }
                        }
                    }
                }

                if childSize == 0 {
                    break
                }
                childPos += childSize
            }
        }

        if boxSize == 0 {
            break
        }
        pos += boxSize
    }

    if timescale == 0 {
        return 0, fmt.Errorf("timescale is zero")
    }

    durationMs := float64(totalDuration) / float64(timescale) * 1000.0
    return durationMs, nil
}
```

- [ ] **Step 3: Update scanFragments to return real durations**

Replace `scanFragments` (lines 196-283) to also extract timescale and per-fragment durations:

```go
func scanFragments(filePath string) (initSize int64, fragments []fragmentInfo, err error) {
    f, err := os.Open(filePath)
    if err != nil {
        return 0, nil, err
    }
    defer f.Close()

    info, err := f.Stat()
    if err != nil {
        return 0, nil, err
    }
    fileSize := info.Size()

    // Read ftyp box.
    ftypSize, ftypType, err := readBoxHeader(f)
    if err != nil {
        return 0, nil, fmt.Errorf("reading ftyp header: %w", err)
    }
    if ftypType != "ftyp" {
        return 0, nil, fmt.Errorf("expected ftyp box, got %q", ftypType)
    }

    // Seek past ftyp body.
    if _, err := f.Seek(ftypSize, io.SeekStart); err != nil {
        return 0, nil, err
    }

    // Read moov box.
    moovSize, moovType, err := readBoxHeader(f)
    if err != nil {
        return 0, nil, fmt.Errorf("reading moov header: %w", err)
    }
    if moovType != "moov" {
        return 0, nil, fmt.Errorf("expected moov box, got %q", moovType)
    }

    initSize = ftypSize + moovSize

    // Extract timescale from mvhd inside moov.
    timescale, err := readTimescale(f, ftypSize, moovSize)
    if err != nil {
        // Fall back to 1000 (millisecond timescale) if mvhd not found.
        timescale = 1000
    }

    // Scan moof+mdat pairs.
    pos := initSize
    for pos < fileSize {
        if _, err := f.Seek(pos, io.SeekStart); err != nil {
            return 0, nil, err
        }

        moofSize, moofType, err := readBoxHeader(f)
        if err != nil {
            break
        }
        if moofType != "moof" {
            if moofSize == 0 {
                break
            }
            pos += moofSize
            continue
        }

        // Read fragment duration from traf/trun inside this moof.
        durationMs, durErr := readFragmentDuration(f, pos, moofSize, timescale)
        if durErr != nil {
            durationMs = 1000.0 // fallback to 1s
        }

        // Read the mdat box header that follows moof.
        if _, err := f.Seek(pos+moofSize, io.SeekStart); err != nil {
            break
        }
        mdatSize, mdatType, err := readBoxHeader(f)
        if err != nil {
            break
        }
        if mdatType != "mdat" {
            pos += moofSize
            continue
        }

        if mdatSize == 0 {
            mdatSize = fileSize - (pos + moofSize)
        }

        fragments = append(fragments, fragmentInfo{
            offset:     pos,
            size:       moofSize + mdatSize,
            durationMs: durationMs,
        })

        pos += moofSize + mdatSize
    }

    return initSize, fragments, nil
}
```

- [ ] **Step 4: Verify it compiles**

Run: `go build ./...`
Expected: Compiles successfully.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/api/hls.go
git commit -m "feat: extract real fragment durations from fMP4 trun/tfhd boxes"
```

---

### Task 3: Index fragments on segment complete

**Files:**
- Modify: `internal/nvr/nvr.go:506-564` (add indexing after recording insert)

- [ ] **Step 1: Add fragment indexing to OnSegmentComplete**

Replace `OnSegmentComplete` in `internal/nvr/nvr.go` (lines 506-564):

```go
func (n *NVR) OnSegmentComplete(filePath string, duration time.Duration) {
    cameras, err := n.database.ListCameras()
    if err != nil {
        return
    }

    var cam *db.Camera
    for _, c := range cameras {
        if c.MediaMTXPath != "" && strings.Contains(filePath, c.MediaMTXPath) {
            cam = c
            break
        }
    }
    if cam == nil {
        return
    }

    var fileSize int64
    if info, err := os.Stat(filePath); err == nil {
        fileSize = info.Size()
    }

    format := "fmp4"
    if strings.HasSuffix(filePath, ".ts") {
        format = "mpegts"
    }

    now := time.Now().UTC()
    start := now.Add(-duration)

    rec := &db.Recording{
        CameraID:   cam.ID,
        StartTime:  start.Format("2006-01-02T15:04:05.000Z"),
        EndTime:    now.Format("2006-01-02T15:04:05.000Z"),
        DurationMs: duration.Milliseconds(),
        FilePath:   filePath,
        FileSize:   fileSize,
        Format:     format,
    }

    var insertErr error
    for attempt := 0; attempt < 3; attempt++ {
        insertErr = n.database.InsertRecording(rec)
        if insertErr == nil {
            break
        }
        fmt.Fprintf(os.Stderr, "NVR: recording insert attempt %d/3 failed: %v\n", attempt+1, insertErr)
        if attempt < 2 {
            time.Sleep(1 * time.Second)
        }
    }
    if insertErr != nil {
        fmt.Fprintf(os.Stderr, "NVR: failed to insert recording after 3 attempts for %s: %v\n", filePath, insertErr)
        return
    }

    // Index fragments for fMP4 files.
    if format == "fmp4" {
        go n.indexRecordingFragments(rec)
    }
}

// indexRecordingFragments scans an fMP4 file and stores fragment metadata in the DB.
func (n *NVR) indexRecordingFragments(rec *db.Recording) {
    initSize, fragments, err := api.ScanFragments(rec.FilePath)
    if err != nil {
        fmt.Fprintf(os.Stderr, "NVR: failed to scan fragments for %s: %v\n", rec.FilePath, err)
        return
    }

    if err := n.database.UpdateRecordingInitSize(rec.ID, initSize); err != nil {
        fmt.Fprintf(os.Stderr, "NVR: failed to update init_size for recording %d: %v\n", rec.ID, err)
    }

    dbFrags := buildDBFragments(rec.ID, fragments)

    if err := n.database.InsertFragments(rec.ID, dbFrags); err != nil {
        fmt.Fprintf(os.Stderr, "NVR: failed to insert fragments for recording %d: %v\n", rec.ID, err)
    }
}
```

Note: `scanFragments` must be exported as `ScanFragments` in `hls.go` so `nvr.go` can call it. Also export the `fragmentInfo` fields:

```go
// In hls.go, rename:
// scanFragments -> ScanFragments (exported)
// fragmentInfo fields -> Offset, Size, DurationMs (exported)

type FragmentInfo struct {
    Offset     int64
    Size       int64
    DurationMs float64
}

func ScanFragments(filePath string) (initSize int64, fragments []FragmentInfo, err error) {
```

Update all references in `hls.go` (`ServePlaylist`) to use the new exported names.

Also add a shared helper in `nvr.go` to convert `api.FragmentInfo` to `db.RecordingFragment` (used by both Task 3 and Task 4):

```go
// buildDBFragments converts scanned fragment info into DB fragment records.
func buildDBFragments(recordingID int64, fragments []api.FragmentInfo) []db.RecordingFragment {
    dbFrags := make([]db.RecordingFragment, len(fragments))
    var cumulativeMs float64
    for i, f := range fragments {
        dbFrags[i] = db.RecordingFragment{
            RecordingID:   recordingID,
            FragmentIndex: i,
            ByteOffset:    f.Offset,
            Size:          f.Size,
            DurationMs:    f.DurationMs,
            IsKeyframe:    true,
            TimestampMs:   int64(cumulativeMs),
        }
        cumulativeMs += f.DurationMs
    }
    return dbFrags
}
```

- [ ] **Step 2: Add import for api package in nvr.go**

Add `"github.com/bluenviron/mediamtx/internal/nvr/api"` to the imports in `nvr.go`.

- [ ] **Step 3: Verify it compiles**

Run: `go build ./...`
Expected: Compiles successfully.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/nvr.go internal/nvr/api/hls.go
git commit -m "feat: index fMP4 fragments on segment complete"
```

---

### Task 4: Background migration for existing recordings

**Files:**
- Create: `internal/nvr/fragment_backfill.go`
- Modify: `internal/nvr/nvr.go` (start backfill goroutine on init)

- [ ] **Step 1: Create the backfill worker**

Create `internal/nvr/fragment_backfill.go`:

```go
package nvr

import (
    "fmt"
    "os"
    "time"

    "github.com/bluenviron/mediamtx/internal/nvr/api"
    "github.com/bluenviron/mediamtx/internal/nvr/db"
)

// startFragmentBackfill runs a background goroutine that indexes any recordings
// that don't yet have fragment metadata. It processes newest-first so recent
// playback benefits immediately.
func (n *NVR) startFragmentBackfill() {
    go func() {
        // Small delay to let the server finish starting up.
        time.Sleep(5 * time.Second)

        recs, err := n.database.GetUnindexedRecordings()
        if err != nil {
            fmt.Fprintf(os.Stderr, "NVR: fragment backfill query failed: %v\n", err)
            return
        }

        if len(recs) == 0 {
            return
        }

        fmt.Fprintf(os.Stderr, "NVR: starting fragment backfill for %d recordings\n", len(recs))

        indexed := 0
        for _, rec := range recs {
            if rec.Format != "fmp4" {
                continue
            }

            // Check file exists.
            if _, err := os.Stat(rec.FilePath); os.IsNotExist(err) {
                continue
            }

            initSize, fragments, err := api.ScanFragments(rec.FilePath)
            if err != nil {
                fmt.Fprintf(os.Stderr, "NVR: backfill scan failed for %s: %v\n", rec.FilePath, err)
                continue
            }

            if err := n.database.UpdateRecordingInitSize(rec.ID, initSize); err != nil {
                fmt.Fprintf(os.Stderr, "NVR: backfill init_size update failed for recording %d: %v\n", rec.ID, err)
            }

            dbFrags := buildDBFragments(rec.ID, fragments)

            if err := n.database.InsertFragments(rec.ID, dbFrags); err != nil {
                fmt.Fprintf(os.Stderr, "NVR: backfill insert failed for recording %d: %v\n", rec.ID, err)
                continue
            }

            indexed++
            if indexed%100 == 0 {
                fmt.Fprintf(os.Stderr, "NVR: fragment backfill progress: %d/%d\n", indexed, len(recs))
            }
        }

        fmt.Fprintf(os.Stderr, "NVR: fragment backfill complete: indexed %d recordings\n", indexed)
    }()
}
```

- [ ] **Step 2: Start backfill in NVR initialization**

In `internal/nvr/nvr.go`, in the NVR initialization function (after the database is opened and routes are registered), add:

```go
n.startFragmentBackfill()
```

Find the appropriate location — likely at the end of `Initialize()` or `Start()` method, after `n.database` is set.

- [ ] **Step 3: Verify it compiles**

Run: `go build ./...`
Expected: Compiles successfully.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/fragment_backfill.go internal/nvr/nvr.go
git commit -m "feat: add background fragment backfill for existing recordings"
```

---

### Task 5: Rewrite HLS playlist generation to use fragment index

**Files:**
- Modify: `internal/nvr/api/hls.go:35-121` (rewrite ServePlaylist)

- [ ] **Step 1: Rewrite ServePlaylist to use DB-backed fragments**

Replace `ServePlaylist` in `internal/nvr/api/hls.go` (lines 35-121):

```go
func (h *HLSHandler) ServePlaylist(c *gin.Context) {
    cameraID := c.Param("cameraId")
    if cameraID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "cameraId is required"})
        return
    }

    if !hasCameraPermission(c, cameraID) {
        c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
        return
    }

    dateStr := c.Query("date")
    date, err := time.ParseInLocation("2006-01-02", dateStr, time.Now().Location())
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date, expected YYYY-MM-DD"})
        return
    }

    start := date
    end := date.Add(24 * time.Hour)

    recordings, err := h.DB.QueryRecordings(cameraID, start, end)
    if err != nil {
        apiError(c, http.StatusInternalServerError, "failed to query recordings", err)
        return
    }

    if len(recordings) == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "no recordings found for this date"})
        return
    }

    token := c.Query("token")
    if token == "" {
        if auth := c.GetHeader("Authorization"); strings.HasPrefix(auth, "Bearer ") {
            token = strings.TrimPrefix(auth, "Bearer ")
        }
    }

    // Two-pass approach: collect segment data first to compute maxDuration,
    // then write complete playlist with correct TARGETDURATION header.
    type playlistEntry struct {
        line string
    }
    var entries []playlistEntry
    var maxDuration float64
    first := true

    for _, rec := range recordings {
        // Try DB-backed fragments first.
        fragments, dbErr := h.DB.GetFragments(rec.ID)
        if dbErr != nil || len(fragments) == 0 {
            // Fallback: scan file directly (un-indexed recording).
            initSize, scannedFrags, scanErr := ScanFragments(rec.FilePath)
            if scanErr != nil || len(scannedFrags) == 0 {
                continue
            }

            segmentURL := segmentURLFromFilePath(rec.FilePath, h.RecordingsPath, token)
            if !first {
                entries = append(entries, playlistEntry{"#EXT-X-DISCONTINUITY\n"})
            }
            first = false
            entries = append(entries, playlistEntry{fmt.Sprintf("#EXT-X-MAP:URI=\"%s\",BYTERANGE=\"%d@0\"\n", segmentURL, initSize)})
            for _, frag := range scannedFrags {
                durSec := frag.DurationMs / 1000.0
                if durSec > maxDuration {
                    maxDuration = durSec
                }
                entries = append(entries,
                    playlistEntry{fmt.Sprintf("#EXTINF:%.6f,\n", durSec)},
                    playlistEntry{fmt.Sprintf("#EXT-X-BYTERANGE:%d@%d\n", frag.Size, frag.Offset)},
                    playlistEntry{segmentURL + "\n"},
                )
            }
            continue
        }

        // DB-backed path: use pre-computed fragment metadata.
        segmentURL := segmentURLFromFilePath(rec.FilePath, h.RecordingsPath, token)
        if !first {
            entries = append(entries, playlistEntry{"#EXT-X-DISCONTINUITY\n"})
        }
        first = false

        initSize := rec.InitSize
        if initSize == 0 {
            initSize = 1024
        }
        entries = append(entries, playlistEntry{fmt.Sprintf("#EXT-X-MAP:URI=\"%s\",BYTERANGE=\"%d@0\"\n", segmentURL, initSize)})

        for _, frag := range fragments {
            durSec := frag.DurationMs / 1000.0
            if durSec > maxDuration {
                maxDuration = durSec
            }
            entries = append(entries,
                playlistEntry{fmt.Sprintf("#EXTINF:%.6f,\n", durSec)},
                playlistEntry{fmt.Sprintf("#EXT-X-BYTERANGE:%d@%d\n", frag.Size, frag.ByteOffset)},
                playlistEntry{segmentURL + "\n"},
            )
        }
    }

    // Write complete playlist with correct header.
    targetDur := int(maxDuration) + 1
    if targetDur < 1 {
        targetDur = 2
    }

    var b strings.Builder
    b.WriteString("#EXTM3U\n")
    b.WriteString("#EXT-X-VERSION:7\n")
    b.WriteString(fmt.Sprintf("#EXT-X-TARGETDURATION:%d\n", targetDur))
    b.WriteString("#EXT-X-PLAYLIST-TYPE:VOD\n")
    for _, e := range entries {
        b.WriteString(e.line)
    }
    b.WriteString("#EXT-X-ENDLIST\n")

    c.Header("Content-Type", "application/vnd.apple.mpegurl")
    c.Header("Cache-Control", "no-cache")
    c.String(http.StatusOK, b.String())
}
```

- [ ] **Step 2: Verify it compiles**

Run: `go build ./...`
Expected: Compiles successfully.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/hls.go
git commit -m "feat: rewrite HLS playlist to use fragment index with real durations"
```

---

### Task 6: Fix timeline endpoint timezone bug

**Files:**
- Modify: `internal/nvr/api/recordings.go:108-109` (fix timezone parsing)

- [ ] **Step 1: Fix Timeline endpoint to use local timezone**

In `internal/nvr/api/recordings.go`, change line 109 from:

```go
date, err := time.Parse("2006-01-02", dateStr)
```

To:

```go
date, err := time.ParseInLocation("2006-01-02", dateStr, time.Now().Location())
```

- [ ] **Step 2: Also fix MotionEvents endpoint**

In `internal/nvr/api/recordings.go`, change line 180 from:

```go
date, err := time.Parse("2006-01-02", dateStr)
```

To:

```go
date, err := time.ParseInLocation("2006-01-02", dateStr, time.Now().Location())
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./...`
Expected: Compiles successfully.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/recordings.go
git commit -m "fix: use local timezone for Timeline and MotionEvents endpoints"
```

---

### Task 7: Fix PlaybackController seeking race condition and error handling

**Files:**
- Modify: `clients/flutter/lib/screens/playback/playback_controller.dart:189-211,269-317`

- [ ] **Step 1: Fix seek() with try/finally and debounce**

Replace the `seek` method (lines 189-211):

```dart
Future<void> seek(Duration wallClockTarget) async {
    final clamped = Duration(
      milliseconds: wallClockTarget.inMilliseconds.clamp(0, _maxPosition.inMilliseconds),
    );
    final snapped = _snapToSegment(clamped);

    _isSeeking = true;
    _position = snapped;
    _lastSeekTarget = snapped;
    notifyListeners();

    try {
      if (_players.isEmpty) {
        await _openPlayers();
      }

      final playerPos = _wallClockToPlayer(snapped);
      for (final p in _players.values) {
        await p.seek(playerPos);
      }
    } catch (e) {
      debugPrint('Playback seek error: $e');
      _error = e.toString();
    } finally {
      _isSeeking = false;
      // Debounce: ignore stale position events for 150ms after seek completes.
      _seekDebounceUntil = DateTime.now().add(const Duration(milliseconds: 150));
    }
  }
```

Add these fields to the class (after line 18):

```dart
Duration? _lastSeekTarget;
DateTime _seekDebounceUntil = DateTime(2000);
```

- [ ] **Step 2: Fix position stream listener with validation**

Replace the position listener in `_openPlayers()` (lines 302-307):

```dart
_positionSub = primary.stream.position.listen((playerPos) {
    if (_disposed || _isSeeking) return;
    // Ignore stale position events during post-seek debounce window.
    if (DateTime.now().isBefore(_seekDebounceUntil)) return;

    final wallClock = _playerToWallClock(playerPos);

    // Discard position jumps backward >2s that weren't user-initiated (snap-back protection).
    if (_lastSeekTarget == null &&
        _position - wallClock > const Duration(seconds: 2)) {
      return;
    }

    _lastSeekTarget = null;
    _position = wallClock;
    notifyListeners();
  });
```

- [ ] **Step 3: Fix _openPlayers() error handling**

Replace `_openPlayers()` (lines 269-317):

```dart
Future<void> _openPlayers() async {
    final token = await getAccessToken();
    _error = null;

    for (final camId in _selectedCameraIds) {
      final url = playbackService.playlistUrl(
        cameraId: camId,
        date: _dateKey,
        token: token,
      );

      try {
        final player = Player();
        player.setRate(_speed);
        _players[camId] = player;
        _videoControllers[camId] = VideoController(player);

        player.stream.error.listen((error) {
          if (_disposed) return;
          _error = error;
          debugPrint('Playback player error: $error');
          notifyListeners();
        });

        await player.open(Media(url), play: false);
      } catch (e) {
        debugPrint('Failed to open player for camera $camId: $e');
        // Remove failed player but continue with others.
        _players.remove(camId)?.dispose();
        _videoControllers.remove(camId);
      }
    }

    if (_players.isEmpty) {
      _error = 'Failed to open any camera for playback';
      notifyListeners();
      return;
    }

    _positionSub?.cancel();
    _completedSub?.cancel();
    final primary = _players.values.first;

    _positionSub = primary.stream.position.listen((playerPos) {
      if (_disposed || _isSeeking) return;
      if (DateTime.now().isBefore(_seekDebounceUntil)) return;

      final wallClock = _playerToWallClock(playerPos);

      if (_lastSeekTarget == null &&
          _position - wallClock > const Duration(seconds: 2)) {
        return;
      }

      _lastSeekTarget = null;
      _position = wallClock;
      notifyListeners();
    });

    _completedSub = primary.stream.completed.listen((completed) {
      if (_disposed || !completed) return;
      _isPlaying = false;
      notifyListeners();
    });

    notifyListeners();
  }
```

- [ ] **Step 4: Verify it compiles**

Run from `clients/flutter/`: `flutter analyze`
Expected: No errors.

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/screens/playback/playback_controller.dart
git commit -m "fix: seek race condition, error handling, and snap-back protection"
```

---

### Task 8: Fix gap snapping boundary logic

**Files:**
- Modify: `clients/flutter/lib/screens/playback/playback_controller.dart:319-333`

- [ ] **Step 1: Fix _snapToSegment**

Replace `_snapToSegment` (lines 319-333):

```dart
Duration _snapToSegment(Duration position) {
    if (_segments.isEmpty) return position;
    final posTime = _dayStart.add(position);

    // Check if inside any segment (inclusive end boundary).
    for (final seg in _segments) {
      if (!posTime.isBefore(seg.startTime) && !posTime.isAfter(seg.endTime)) {
        return position;
      }
    }

    // In a gap: snap to nearest segment boundary (prev end or next start).
    Duration? nearestBefore;
    Duration? nearestAfter;

    for (final seg in _segments) {
      if (!seg.endTime.isAfter(posTime)) {
        nearestBefore = seg.endTime.difference(_dayStart);
      }
      if (seg.startTime.isAfter(posTime) && nearestAfter == null) {
        nearestAfter = seg.startTime.difference(_dayStart);
      }
    }

    // Prefer snapping forward to next segment start.
    if (nearestAfter != null) return nearestAfter;
    // If past all segments, snap to last segment end.
    if (nearestBefore != null) return nearestBefore;
    return position;
  }
```

- [ ] **Step 2: Fix _wallClockToPlayer inclusive boundary**

In `_wallClockToPlayer` (line 120), change:

```dart
if (wallClock >= entry.wallStart && wallClock < entry.wallEnd) {
```

To:

```dart
if (wallClock >= entry.wallStart && wallClock <= entry.wallEnd) {
```

- [ ] **Step 3: Verify it compiles**

Run from `clients/flutter/`: `flutter analyze`
Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/screens/playback/playback_controller.dart
git commit -m "fix: gap snapping boundary logic with inclusive end and nearest-snap"
```

---

### Task 9: Add playlist caching

**Files:**
- Modify: `internal/nvr/api/hls.go` (add LRU cache)

- [ ] **Step 1: Add a simple cache to HLSHandler**

Add a cache struct and modify `HLSHandler`:

```go
import (
    "sync"
    // ... existing imports
)

type playlistCacheEntry struct {
    playlist  string
    cachedAt  time.Time
}

type HLSHandler struct {
    DB             *db.DB
    RecordingsPath string

    mu    sync.Mutex
    cache map[string]*playlistCacheEntry // key: "cameraId:date"
}

const playlistCacheTTL = 30 * time.Second
```

- [ ] **Step 2: Add cache lookup/store in ServePlaylist**

At the start of `ServePlaylist`, after extracting `cameraID` and `dateStr`, add cache lookup:

```go
cacheKey := cameraID + ":" + dateStr

h.mu.Lock()
if h.cache == nil {
    h.cache = make(map[string]*playlistCacheEntry)
}
if entry, ok := h.cache[cacheKey]; ok {
    // For past dates, cache indefinitely. For today, use TTL.
    isToday := dateStr == time.Now().Format("2006-01-02")
    if !isToday || time.Since(entry.cachedAt) < playlistCacheTTL {
        h.mu.Unlock()
        c.Header("Content-Type", "application/vnd.apple.mpegurl")
        c.Header("Cache-Control", "no-cache")
        c.String(http.StatusOK, entry.playlist)
        return
    }
}
h.mu.Unlock()
```

At the end, before returning the response, store in cache:

```go
playlist := final.String()

h.mu.Lock()
h.cache[cacheKey] = &playlistCacheEntry{
    playlist: playlist,
    cachedAt: time.Now(),
}
h.mu.Unlock()

c.Header("Content-Type", "application/vnd.apple.mpegurl")
c.Header("Cache-Control", "no-cache")
c.String(http.StatusOK, playlist)
```

- [ ] **Step 3: Add cache invalidation method**

```go
// InvalidateCache clears the playlist cache for a camera+date combo.
// Called when a new recording segment completes.
func (h *HLSHandler) InvalidateCache(cameraID, date string) {
    h.mu.Lock()
    defer h.mu.Unlock()
    if h.cache != nil {
        delete(h.cache, cameraID+":"+date)
    }
}
```

**Important:** The `NVR` struct currently has no `hlsHandler` field. You must:
1. Add `hlsHandler *api.HLSHandler` field to the `NVR` struct in `nvr.go`
2. Store the reference when creating `HLSHandler` in the NVR initialization (where `RegisterRoutes` is called — the handler is currently created inline at ~line 499)

Then call this from `nvr.go` `OnSegmentComplete` after the recording is inserted:

```go
if n.hlsHandler != nil {
    dateStr := start.Format("2006-01-02")
    n.hlsHandler.InvalidateCache(cam.ID, dateStr)
}
```

- [ ] **Step 4: Verify it compiles**

Run: `go build ./...`
Expected: Compiles successfully.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/api/hls.go internal/nvr/nvr.go
git commit -m "feat: add LRU playlist cache with TTL and invalidation"
```

---

### Task 10: Add motion intensity timeline endpoint

**Files:**
- Create: `internal/nvr/db/motion_events_intensity.go`
- Modify: `internal/nvr/api/recordings.go` (add Intensity handler)
- Modify: `internal/nvr/api/router.go` (add route)

- [ ] **Step 1: Add bucketed intensity query**

Create `internal/nvr/db/motion_events_intensity.go`:

```go
package db

import "time"

// IntensityBucket holds the event count for a time bucket.
type IntensityBucket struct {
    BucketStart time.Time `json:"bucket_start"`
    Count       int       `json:"count"`
}

// GetMotionIntensity returns event counts bucketed by the given interval.
func (d *DB) GetMotionIntensity(cameraID string, start, end time.Time, bucketSeconds int) ([]IntensityBucket, error) {
    rows, err := d.Query(`
        SELECT
            strftime('%s', started_at) / ? * ? as bucket_epoch,
            COUNT(*) as count
        FROM motion_events
        WHERE camera_id = ?
            AND started_at >= ?
            AND started_at < ?
        GROUP BY bucket_epoch
        ORDER BY bucket_epoch`,
        bucketSeconds, bucketSeconds,
        cameraID,
        start.UTC().Format(timeFormat),
        end.UTC().Format(timeFormat),
    )
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var buckets []IntensityBucket
    for rows.Next() {
        var epochSec int64
        var count int
        if err := rows.Scan(&epochSec, &count); err != nil {
            return nil, err
        }
        buckets = append(buckets, IntensityBucket{
            BucketStart: time.Unix(epochSec, 0).UTC(),
            Count:       count,
        })
    }
    return buckets, rows.Err()
}
```

- [ ] **Step 2: Add Intensity handler**

Add to `internal/nvr/api/recordings.go`:

```go
// Intensity returns motion event counts bucketed by time interval.
// Query params: camera_id (required), date (YYYY-MM-DD), bucket_seconds (default 60).
func (h *RecordingHandler) Intensity(c *gin.Context) {
    cameraID := c.Query("camera_id")
    if cameraID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id is required"})
        return
    }

    if !hasCameraPermission(c, cameraID) {
        c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
        return
    }

    dateStr := c.Query("date")
    date, err := time.ParseInLocation("2006-01-02", dateStr, time.Now().Location())
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date, expected YYYY-MM-DD"})
        return
    }

    bucketSeconds := 60
    if bs := c.Query("bucket_seconds"); bs != "" {
        if v, err := strconv.Atoi(bs); err == nil && v > 0 {
            bucketSeconds = v
        }
    }

    start := date
    end := date.Add(24 * time.Hour)

    buckets, err := h.DB.GetMotionIntensity(cameraID, start, end, bucketSeconds)
    if err != nil {
        apiError(c, http.StatusInternalServerError, "failed to query intensity", err)
        return
    }

    if buckets == nil {
        buckets = []db.IntensityBucket{}
    }

    c.JSON(http.StatusOK, buckets)
}
```

- [ ] **Step 3: Add route**

In `internal/nvr/api/router.go`, add after line 198 (`protected.GET("/timeline", ...)`):

```go
protected.GET("/timeline/intensity", recordingHandler.Intensity)
```

- [ ] **Step 4: Verify it compiles**

Run: `go build ./...`
Expected: Compiles successfully.

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/motion_events_intensity.go internal/nvr/api/recordings.go internal/nvr/api/router.go
git commit -m "feat: add motion intensity timeline endpoint with configurable bucketing"
```

---

### Task 11: Add bookmarks CRUD (server)

**Files:**
- Create: `internal/nvr/db/bookmarks.go`
- Create: `internal/nvr/api/bookmarks.go`
- Modify: `internal/nvr/db/migrations.go` (add migration 17)
- Modify: `internal/nvr/api/router.go` (add routes)

- [ ] **Step 1: Add migration version 17**

In `internal/nvr/db/migrations.go`, add:

```go
{
    version: 17,
    sql: `
CREATE TABLE bookmarks (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    timestamp TEXT NOT NULL,
    label TEXT NOT NULL,
    created_by TEXT,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ', 'now')),
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);
CREATE INDEX idx_bookmarks_camera_time ON bookmarks(camera_id, timestamp);
CREATE INDEX idx_bookmarks_timestamp ON bookmarks(timestamp);
`,
},
```

- [ ] **Step 2: Create bookmarks DB layer**

Create `internal/nvr/db/bookmarks.go`:

```go
package db

import (
    "database/sql"
    "errors"
    "time"
)

type Bookmark struct {
    ID        int64  `json:"id"`
    CameraID  string `json:"camera_id"`
    Timestamp string `json:"timestamp"`
    Label     string `json:"label"`
    CreatedBy string `json:"created_by,omitempty"`
    CreatedAt string `json:"created_at"`
}

func (d *DB) InsertBookmark(b *Bookmark) error {
    if b.CreatedAt == "" {
        b.CreatedAt = time.Now().UTC().Format(timeFormat)
    }
    res, err := d.Exec(`
        INSERT INTO bookmarks (camera_id, timestamp, label, created_by, created_at)
        VALUES (?, ?, ?, ?, ?)`,
        b.CameraID, b.Timestamp, b.Label, b.CreatedBy, b.CreatedAt)
    if err != nil {
        return err
    }
    id, err := res.LastInsertId()
    if err != nil {
        return err
    }
    b.ID = id
    return nil
}

func (d *DB) GetBookmarks(cameraID string, start, end time.Time) ([]Bookmark, error) {
    rows, err := d.Query(`
        SELECT id, camera_id, timestamp, label, COALESCE(created_by, ''), created_at
        FROM bookmarks
        WHERE camera_id = ? AND timestamp >= ? AND timestamp < ?
        ORDER BY timestamp`,
        cameraID, start.UTC().Format(timeFormat), end.UTC().Format(timeFormat))
    if err != nil {
        return nil, err
    }
    defer rows.Close()

    var bookmarks []Bookmark
    for rows.Next() {
        var b Bookmark
        if err := rows.Scan(&b.ID, &b.CameraID, &b.Timestamp, &b.Label, &b.CreatedBy, &b.CreatedAt); err != nil {
            return nil, err
        }
        bookmarks = append(bookmarks, b)
    }
    return bookmarks, rows.Err()
}

func (d *DB) UpdateBookmark(id int64, label string) error {
    res, err := d.Exec("UPDATE bookmarks SET label = ? WHERE id = ?", label, id)
    if err != nil {
        return err
    }
    n, _ := res.RowsAffected()
    if n == 0 {
        return ErrNotFound
    }
    return nil
}

func (d *DB) DeleteBookmark(id int64) error {
    res, err := d.Exec("DELETE FROM bookmarks WHERE id = ?", id)
    if err != nil {
        return err
    }
    n, _ := res.RowsAffected()
    if n == 0 {
        return ErrNotFound
    }
    return nil
}

func (d *DB) GetBookmark(id int64) (*Bookmark, error) {
    b := &Bookmark{}
    err := d.QueryRow(`
        SELECT id, camera_id, timestamp, label, COALESCE(created_by, ''), created_at
        FROM bookmarks WHERE id = ?`, id).
        Scan(&b.ID, &b.CameraID, &b.Timestamp, &b.Label, &b.CreatedBy, &b.CreatedAt)
    if errors.Is(err, sql.ErrNoRows) {
        return nil, ErrNotFound
    }
    return b, err
}
```

- [ ] **Step 3: Create bookmarks API handler**

Create `internal/nvr/api/bookmarks.go`:

```go
package api

import (
    "errors"
    "net/http"
    "strconv"
    "time"

    "github.com/gin-gonic/gin"

    "github.com/bluenviron/mediamtx/internal/nvr/db"
)

type BookmarkHandler struct {
    DB *db.DB
}

type CreateBookmarkRequest struct {
    CameraID  string `json:"camera_id" binding:"required"`
    Timestamp string `json:"timestamp" binding:"required"`
    Label     string `json:"label" binding:"required"`
}

func (h *BookmarkHandler) Create(c *gin.Context) {
    var req CreateBookmarkRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id, timestamp, and label are required"})
        return
    }

    if !hasCameraPermission(c, req.CameraID) {
        c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
        return
    }

    username, _ := c.Get("username")
    b := &db.Bookmark{
        CameraID:  req.CameraID,
        Timestamp: req.Timestamp,
        Label:     req.Label,
        CreatedBy: username.(string),
    }

    if err := h.DB.InsertBookmark(b); err != nil {
        apiError(c, http.StatusInternalServerError, "failed to create bookmark", err)
        return
    }

    c.JSON(http.StatusCreated, b)
}

func (h *BookmarkHandler) List(c *gin.Context) {
    cameraID := c.Query("camera_id")
    if cameraID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id is required"})
        return
    }

    if !hasCameraPermission(c, cameraID) {
        c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
        return
    }

    dateStr := c.Query("date")
    date, err := time.ParseInLocation("2006-01-02", dateStr, time.Now().Location())
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid date"})
        return
    }

    bookmarks, err := h.DB.GetBookmarks(cameraID, date, date.Add(24*time.Hour))
    if err != nil {
        apiError(c, http.StatusInternalServerError, "failed to query bookmarks", err)
        return
    }

    if bookmarks == nil {
        bookmarks = []db.Bookmark{}
    }

    c.JSON(http.StatusOK, bookmarks)
}

type UpdateBookmarkRequest struct {
    Label string `json:"label" binding:"required"`
}

func (h *BookmarkHandler) Update(c *gin.Context) {
    id, err := strconv.ParseInt(c.Param("id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark id"})
        return
    }

    var req UpdateBookmarkRequest
    if err := c.ShouldBindJSON(&req); err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "label is required"})
        return
    }

    if err := h.DB.UpdateBookmark(id, req.Label); err != nil {
        if errors.Is(err, db.ErrNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"error": "bookmark not found"})
            return
        }
        apiError(c, http.StatusInternalServerError, "failed to update bookmark", err)
        return
    }

    c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

func (h *BookmarkHandler) Delete(c *gin.Context) {
    id, err := strconv.ParseInt(c.Param("id"), 10, 64)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid bookmark id"})
        return
    }

    if err := h.DB.DeleteBookmark(id); err != nil {
        if errors.Is(err, db.ErrNotFound) {
            c.JSON(http.StatusNotFound, gin.H{"error": "bookmark not found"})
            return
        }
        apiError(c, http.StatusInternalServerError, "failed to delete bookmark", err)
        return
    }

    c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
```

- [ ] **Step 4: Add routes**

In `internal/nvr/api/router.go`, add handler initialization and routes:

After `savedClipHandler` initialization (~line 96):

```go
bookmarkHandler := &BookmarkHandler{
    DB: cfg.DB,
}
```

After the saved-clips routes (~line 206):

```go
// Bookmarks.
protected.GET("/bookmarks", bookmarkHandler.List)
protected.POST("/bookmarks", bookmarkHandler.Create)
protected.PUT("/bookmarks/:id", bookmarkHandler.Update)
protected.DELETE("/bookmarks/:id", bookmarkHandler.Delete)
```

- [ ] **Step 5: Verify it compiles**

Run: `go build ./...`
Expected: Compiles successfully.

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/db/migrations.go internal/nvr/db/bookmarks.go internal/nvr/api/bookmarks.go internal/nvr/api/router.go
git commit -m "feat: add bookmarks CRUD API and database layer"
```

---

### Task 12: Add Flutter bookmark model, provider, and timeline layer

**Files:**
- Create: `clients/flutter/lib/models/bookmark.dart`
- Create: `clients/flutter/lib/providers/bookmarks_provider.dart`
- Create: `clients/flutter/lib/screens/playback/timeline/bookmark_layer.dart`
- Modify: `clients/flutter/lib/screens/playback/timeline/composable_timeline.dart` (add layer)
- Modify: `clients/flutter/lib/screens/playback/playback_screen.dart` (fetch bookmarks)
- Modify: `clients/flutter/lib/screens/playback/playback_controller.dart` (add bookmark skip)

- [ ] **Step 1: Create Bookmark model**

Create `clients/flutter/lib/models/bookmark.dart`:

```dart
class Bookmark {
  final int id;
  final String cameraId;
  final DateTime timestamp;
  final String label;
  final String? createdBy;
  final DateTime createdAt;

  const Bookmark({
    required this.id,
    required this.cameraId,
    required this.timestamp,
    required this.label,
    this.createdBy,
    required this.createdAt,
  });

  factory Bookmark.fromJson(Map<String, dynamic> json) {
    return Bookmark(
      id: json['id'] as int,
      cameraId: json['camera_id'] as String,
      timestamp: DateTime.parse(json['timestamp'] as String),
      label: json['label'] as String,
      createdBy: json['created_by'] as String?,
      createdAt: DateTime.parse(json['created_at'] as String),
    );
  }
}
```

- [ ] **Step 2: Create bookmarks provider**

Create `clients/flutter/lib/providers/bookmarks_provider.dart`:

```dart
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/bookmark.dart';
import '../services/api_client.dart';

final bookmarksProvider = FutureProvider.family<List<Bookmark>, ({String cameraId, String date})>(
  (ref, params) async {
    final client = ref.read(apiClientProvider);
    final response = await client.get('/api/nvr/bookmarks', queryParameters: {
      'camera_id': params.cameraId,
      'date': params.date,
    });
    final list = response.data as List;
    return list.map((e) => Bookmark.fromJson(e as Map<String, dynamic>)).toList();
  },
);
```

- [ ] **Step 3: Create bookmark timeline layer**

Create `clients/flutter/lib/screens/playback/timeline/bookmark_layer.dart`:

```dart
import 'package:flutter/material.dart';
import '../../../models/bookmark.dart';
import 'timeline_viewport.dart';

class BookmarkLayer extends StatelessWidget {
  final TimelineViewport viewport;
  final List<Bookmark> bookmarks;
  final DateTime dayStart;

  const BookmarkLayer({
    super.key,
    required this.viewport,
    required this.bookmarks,
    required this.dayStart,
  });

  @override
  Widget build(BuildContext context) {
    return CustomPaint(
      painter: _BookmarkPainter(
        viewport: viewport,
        bookmarks: bookmarks,
        dayStart: dayStart,
      ),
      size: Size.infinite,
    );
  }
}

class _BookmarkPainter extends CustomPainter {
  final TimelineViewport viewport;
  final List<Bookmark> bookmarks;
  final DateTime dayStart;

  _BookmarkPainter({
    required this.viewport,
    required this.bookmarks,
    required this.dayStart,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()
      ..color = Colors.amber
      ..style = PaintingStyle.fill;

    for (final bookmark in bookmarks) {
      final seconds = bookmark.timestamp.difference(dayStart).inSeconds.toDouble();
      final x = viewport.timeToPixel(seconds, size.width);
      if (x < 0 || x > size.width) continue;

      // Draw a small triangle marker.
      final path = Path()
        ..moveTo(x, size.height)
        ..lineTo(x - 5, size.height - 10)
        ..lineTo(x + 5, size.height - 10)
        ..close();
      canvas.drawPath(path, paint);

      // Draw a thin vertical line.
      canvas.drawLine(
        Offset(x, 0),
        Offset(x, size.height - 10),
        Paint()
          ..color = Colors.amber.withOpacity(0.4)
          ..strokeWidth = 1,
      );
    }
  }

  @override
  bool shouldRepaint(covariant _BookmarkPainter oldDelegate) =>
      bookmarks != oldDelegate.bookmarks || viewport != oldDelegate.viewport;
}
```

- [ ] **Step 4: Add bookmark layer to ComposableTimeline**

In `clients/flutter/lib/screens/playback/timeline/composable_timeline.dart`, add the bookmark layer in the Stack alongside the other layers. Import the bookmark layer and add the widget where the other layers are stacked (between the event layer and playhead layer):

```dart
BookmarkLayer(
  viewport: _viewport,
  bookmarks: widget.bookmarks,
  dayStart: _dayStart,
),
```

Add `bookmarks` parameter to the ComposableTimeline widget's constructor:

```dart
final List<Bookmark> bookmarks;
```

- [ ] **Step 5: Add bookmark skip to PlaybackController**

In `playback_controller.dart`, add a `_bookmarks` field and skip methods:

```dart
List<Bookmark> _bookmarks = [];
List<Bookmark> get bookmarks => _bookmarks;

void setBookmarks(List<Bookmark> b) {
    _bookmarks = b;
    notifyListeners();
  }

void skipToNextBookmark() {
    final t = _findNext(_bookmarks, _dayStart, _position, (b) => b.timestamp);
    if (t != null) seek(t);
  }

void skipToPreviousBookmark() {
    final t = _findPrev(_bookmarks, _dayStart, _position, (b) => b.timestamp);
    if (t != null) seek(t);
  }
```

- [ ] **Step 6: Wire bookmarks into PlaybackScreen**

In `playback_screen.dart`, fetch bookmarks from the provider and pass to controller and timeline.

- [ ] **Step 7: Verify it compiles**

Run from `clients/flutter/`: `flutter analyze`
Expected: No errors.

- [ ] **Step 8: Commit**

```bash
git add clients/flutter/lib/models/bookmark.dart \
  clients/flutter/lib/providers/bookmarks_provider.dart \
  clients/flutter/lib/screens/playback/timeline/bookmark_layer.dart \
  clients/flutter/lib/screens/playback/timeline/composable_timeline.dart \
  clients/flutter/lib/screens/playback/playback_controller.dart \
  clients/flutter/lib/screens/playback/playback_screen.dart
git commit -m "feat: add bookmark timeline layer with navigation"
```

---

### Task 13: Add Flutter motion intensity layer

**Files:**
- Create: `clients/flutter/lib/providers/timeline_intensity_provider.dart`
- Create: `clients/flutter/lib/screens/playback/timeline/intensity_layer.dart`
- Modify: `clients/flutter/lib/screens/playback/timeline/composable_timeline.dart` (add layer)
- Modify: `clients/flutter/lib/screens/playback/playback_screen.dart` (fetch intensity)

- [ ] **Step 1: Create intensity provider**

Create `clients/flutter/lib/providers/timeline_intensity_provider.dart`:

```dart
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../services/api_client.dart';

class IntensityBucket {
  final DateTime bucketStart;
  final int count;

  const IntensityBucket({required this.bucketStart, required this.count});

  factory IntensityBucket.fromJson(Map<String, dynamic> json) {
    return IntensityBucket(
      bucketStart: DateTime.parse(json['bucket_start'] as String),
      count: json['count'] as int,
    );
  }
}

final intensityProvider = FutureProvider.family<List<IntensityBucket>, ({String cameraId, String date, int bucketSeconds})>(
  (ref, params) async {
    final client = ref.read(apiClientProvider);
    final response = await client.get('/api/nvr/timeline/intensity', queryParameters: {
      'camera_id': params.cameraId,
      'date': params.date,
      'bucket_seconds': params.bucketSeconds.toString(),
    });
    final list = response.data as List;
    return list.map((e) => IntensityBucket.fromJson(e as Map<String, dynamic>)).toList();
  },
);
```

- [ ] **Step 2: Create intensity timeline layer**

Create `clients/flutter/lib/screens/playback/timeline/intensity_layer.dart`:

```dart
import 'package:flutter/material.dart';
import '../../../providers/timeline_intensity_provider.dart';
import 'timeline_viewport.dart';

class IntensityLayer extends StatelessWidget {
  final TimelineViewport viewport;
  final List<IntensityBucket> buckets;
  final int bucketSeconds;
  final DateTime dayStart;

  const IntensityLayer({
    super.key,
    required this.viewport,
    required this.buckets,
    required this.bucketSeconds,
    required this.dayStart,
  });

  @override
  Widget build(BuildContext context) {
    return CustomPaint(
      painter: _IntensityPainter(
        viewport: viewport,
        buckets: buckets,
        bucketSeconds: bucketSeconds,
        dayStart: dayStart,
      ),
      size: Size.infinite,
    );
  }
}

class _IntensityPainter extends CustomPainter {
  final TimelineViewport viewport;
  final List<IntensityBucket> buckets;
  final int bucketSeconds;
  final DateTime dayStart;

  _IntensityPainter({
    required this.viewport,
    required this.buckets,
    required this.bucketSeconds,
    required this.dayStart,
  });

  @override
  void paint(Canvas canvas, Size size) {
    if (buckets.isEmpty) return;

    final maxCount = buckets.map((b) => b.count).reduce((a, b) => a > b ? a : b);
    if (maxCount == 0) return;

    for (final bucket in buckets) {
      final startSec = bucket.bucketStart.difference(dayStart).inSeconds.toDouble();
      final endSec = startSec + bucketSeconds;

      final x1 = viewport.timeToPixel(startSec, size.width);
      final x2 = viewport.timeToPixel(endSec, size.width);

      if (x2 < 0 || x1 > size.width) continue;

      final intensity = bucket.count / maxCount;
      final barHeight = size.height * 0.6 * intensity;

      final paint = Paint()
        ..color = Colors.red.withOpacity(0.15 + 0.35 * intensity)
        ..style = PaintingStyle.fill;

      canvas.drawRect(
        Rect.fromLTRB(x1, size.height - barHeight, x2, size.height),
        paint,
      );
    }
  }

  @override
  bool shouldRepaint(covariant _IntensityPainter oldDelegate) =>
      buckets != oldDelegate.buckets || viewport != oldDelegate.viewport;
}
```

- [ ] **Step 3: Add intensity layer to ComposableTimeline**

In `composable_timeline.dart`, add the intensity layer before the recording layer in the Stack:

```dart
IntensityLayer(
  viewport: _viewport,
  buckets: widget.intensityBuckets,
  bucketSeconds: widget.intensityBucketSeconds,
  dayStart: _dayStart,
),
```

Add parameters to ComposableTimeline:

```dart
final List<IntensityBucket> intensityBuckets;
final int intensityBucketSeconds;
```

- [ ] **Step 4: Wire into PlaybackScreen**

In `playback_screen.dart`, compute bucket seconds from viewport zoom level, fetch intensity data, and pass to ComposableTimeline.

- [ ] **Step 5: Verify it compiles**

Run from `clients/flutter/`: `flutter analyze`
Expected: No errors.

- [ ] **Step 6: Commit**

```bash
git add clients/flutter/lib/providers/timeline_intensity_provider.dart \
  clients/flutter/lib/screens/playback/timeline/intensity_layer.dart \
  clients/flutter/lib/screens/playback/timeline/composable_timeline.dart \
  clients/flutter/lib/screens/playback/playback_screen.dart
git commit -m "feat: add motion intensity heatmap layer to timeline"
```

---

### Task 14: Add cross-day continuous playback

**Files:**
- Modify: `clients/flutter/lib/screens/playback/playback_controller.dart` (add continuous mode)

- [ ] **Step 1: Add continuous playback support**

Add to PlaybackController:

```dart
bool _continuousMode = true;
bool get continuousMode => _continuousMode;

void setContinuousMode(bool enabled) {
    _continuousMode = enabled;
    notifyListeners();
  }
```

Modify the completed stream listener in `_openPlayers()` to auto-advance:

```dart
_completedSub = primary.stream.completed.listen((completed) {
    if (_disposed || !completed) return;
    if (_continuousMode) {
      // Auto-advance to next day.
      final nextDay = DateTime(_selectedDate.year, _selectedDate.month, _selectedDate.day + 1);
      setSelectedDate(nextDay);
      // PlaybackScreen will react to the date change, fetch new segments,
      // and call play() to resume.
    } else {
      _isPlaying = false;
      notifyListeners();
    }
  });
```

- [ ] **Step 2: Add forward/back day buttons to PlaybackScreen**

In `playback_screen.dart`, add date navigation buttons next to the date picker:

```dart
IconButton(
  icon: const Icon(Icons.chevron_left),
  onPressed: () {
    final prev = DateTime(
      ctrl.selectedDate.year,
      ctrl.selectedDate.month,
      ctrl.selectedDate.day - 1,
    );
    ctrl.setSelectedDate(prev);
  },
),
// ... existing date picker ...
IconButton(
  icon: const Icon(Icons.chevron_right),
  onPressed: () {
    final next = DateTime(
      ctrl.selectedDate.year,
      ctrl.selectedDate.month,
      ctrl.selectedDate.day + 1,
    );
    ctrl.setSelectedDate(next);
  },
),
```

- [ ] **Step 3: Verify it compiles**

Run from `clients/flutter/`: `flutter analyze`
Expected: No errors.

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/screens/playback/playback_controller.dart \
  clients/flutter/lib/screens/playback/playback_screen.dart
git commit -m "feat: add cross-day continuous playback with date navigation"
```

---

### Task 15: Add thumbnail preview endpoint (server)

**Files:**
- Modify: `internal/nvr/api/hls.go` (add thumbnail handler)
- Modify: `internal/nvr/api/router.go` (add route)

- [ ] **Step 1: Add thumbnail handler**

Add to `internal/nvr/api/hls.go`:

```go
// ServeThumbnail extracts a single JPEG frame from the nearest keyframe to the
// requested timestamp. Uses ffmpeg to decode the fragment.
//
// GET /vod/thumbnail?camera_id=X&time=RFC3339
func (h *HLSHandler) ServeThumbnail(c *gin.Context) {
    cameraID := c.Query("camera_id")
    if cameraID == "" {
        c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id is required"})
        return
    }

    if !hasCameraPermission(c, cameraID) {
        c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
        return
    }

    timeStr := c.Query("time")
    t, err := time.Parse(time.RFC3339, timeStr)
    if err != nil {
        c.JSON(http.StatusBadRequest, gin.H{"error": "invalid time, expected RFC3339"})
        return
    }

    // Find the recording that contains this timestamp.
    recs, err := h.DB.QueryRecordings(cameraID, t.Add(-1*time.Second), t.Add(1*time.Second))
    if err != nil || len(recs) == 0 {
        c.JSON(http.StatusNotFound, gin.H{"error": "no recording at this time"})
        return
    }

    rec := recs[0]

    // Use ffmpeg to extract a single frame.
    startTime, _ := time.Parse("2006-01-02T15:04:05.000Z", rec.StartTime)
    offset := t.Sub(startTime)

    cmd := exec.CommandContext(c.Request.Context(),
        "ffmpeg",
        "-ss", fmt.Sprintf("%.3f", offset.Seconds()),
        "-i", rec.FilePath,
        "-frames:v", "1",
        "-f", "mjpeg",
        "-q:v", "5",
        "pipe:1",
    )

    var stdout bytes.Buffer
    cmd.Stdout = &stdout

    if err := cmd.Run(); err != nil {
        apiError(c, http.StatusInternalServerError, "failed to extract thumbnail", err)
        return
    }

    c.Header("Content-Type", "image/jpeg")
    c.Header("Cache-Control", "public, max-age=86400")
    c.Data(http.StatusOK, "image/jpeg", stdout.Bytes())
}
```

Add imports: `"bytes"`, `"os/exec"`.

- [ ] **Step 2: Add route**

In `router.go`, after the HLS playlist route (line 235):

```go
protected.GET("/vod/thumbnail", cfg.HLSHandler.ServeThumbnail)
```

- [ ] **Step 3: Verify it compiles**

Run: `go build ./...`
Expected: Compiles successfully.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/hls.go internal/nvr/api/router.go
git commit -m "feat: add thumbnail extraction endpoint using ffmpeg"
```
