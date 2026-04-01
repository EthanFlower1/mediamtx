# KAI-6: Graceful Recording Recovery Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** On startup, detect incomplete fMP4 segments from a previous crash, repair them by truncating to the last complete moof+mdat pair, and reconcile the database so repaired segments are indexed and playable.

**Architecture:** A new `internal/nvr/recovery/` package with three components: scanner (finds incomplete segments), repair (box-walking truncation), and reconcile (DB sync). Called synchronously in `NVR.Initialize()` after DB open but before fragment backfill and recorder start.

**Tech Stack:** Go stdlib (`os`, `io`, `encoding/binary`), existing `internal/nvr/db` and `internal/nvr/api` packages, `testify` for tests.

---

## File Structure

| File                                      | Responsibility                                                       |
| ----------------------------------------- | -------------------------------------------------------------------- |
| `internal/nvr/recovery/repair.go`         | `RepairSegment()` — fMP4 box walking and file truncation             |
| `internal/nvr/recovery/repair_test.go`    | Unit tests with crafted fMP4 byte fixtures                           |
| `internal/nvr/recovery/scanner.go`        | `ScanForCandidates()` — find orphaned files and unindexed DB entries |
| `internal/nvr/recovery/scanner_test.go`   | Unit tests with temp dirs and mock DB                                |
| `internal/nvr/recovery/reconcile.go`      | `Reconcile()` — insert/update DB entries for recovered segments      |
| `internal/nvr/recovery/reconcile_test.go` | Unit tests for DB operations                                         |
| `internal/nvr/recovery/recovery.go`       | `Run()` — orchestrates scan → repair → reconcile                     |
| `internal/nvr/recovery/recovery_test.go`  | Integration test: end-to-end recovery pipeline                       |
| `internal/nvr/db/recordings.go`           | Add `GetAllRecordingPaths()` and `UpdateRecordingFileSize()` queries |
| `internal/nvr/nvr.go`                     | Call `recovery.Run()` in `Initialize()` at line ~200                 |

---

### Task 1: fMP4 Repair — RepairSegment Function

**Files:**

- Create: `internal/nvr/recovery/repair.go`
- Create: `internal/nvr/recovery/repair_test.go`

- [ ] **Step 1: Write test helpers to craft fMP4 byte fixtures**

Create helper functions that build minimal valid fMP4 byte slices for testing. These build the box structures that `ScanFragments` (in `internal/nvr/api/hls.go:409`) already parses.

```go
// internal/nvr/recovery/repair_test.go
package recovery

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// makeBox creates an ISO BMFF box with the given 4-char type and payload.
func makeBox(boxType string, payload []byte) []byte {
	size := uint32(8 + len(payload))
	buf := make([]byte, size)
	binary.BigEndian.PutUint32(buf[0:4], size)
	copy(buf[4:8], boxType)
	copy(buf[8:], payload)
	return buf
}

// makeFtyp returns a minimal ftyp box.
func makeFtyp() []byte {
	// ftyp payload: major_brand(4) + minor_version(4) + compatible_brands(4)
	payload := make([]byte, 12)
	copy(payload[0:4], "isom")
	return makeBox("ftyp", payload)
}

// makeMoov returns a minimal moov box.
func makeMoov() []byte {
	// Minimal mvhd inside moov. 108 bytes is the standard mvhd v0 size.
	mvhd := makeBox("mvhd", make([]byte, 100))
	return makeBox("moov", mvhd)
}

// makeMoof returns a minimal moof box with a mfhd + trun.
func makeMoof(seqNum uint32) []byte {
	// mfhd: version(1) + flags(3) + sequence_number(4) = 8 bytes
	mfhdPayload := make([]byte, 8)
	binary.BigEndian.PutUint32(mfhdPayload[4:8], seqNum)
	mfhd := makeBox("mfhd", mfhdPayload)

	// trun with 1 sample: version(1) + flags(3) + sample_count(4) + sample_duration(4) + sample_size(4)
	trunPayload := make([]byte, 16)
	// flags: 0x000101 = sample-duration-present + sample-size-present
	trunPayload[3] = 0x01
	trunPayload[2] = 0x01
	binary.BigEndian.PutUint32(trunPayload[4:8], 1) // sample_count
	binary.BigEndian.PutUint32(trunPayload[8:12], 1000) // sample_duration
	binary.BigEndian.PutUint32(trunPayload[12:16], 100) // sample_size

	tfhd := makeBox("tfhd", make([]byte, 4)) // minimal tfhd
	traf := makeBox("traf", append(tfhd, makeBox("trun", trunPayload)...))

	return makeBox("moof", append(mfhd, traf...))
}

// makeMdat returns an mdat box with the given payload size.
func makeMdat(payloadSize int) []byte {
	return makeBox("mdat", make([]byte, payloadSize))
}

// buildValidFMP4 creates a complete, valid fMP4 file with n fragments.
func buildValidFMP4(numFragments int) []byte {
	var data []byte
	data = append(data, makeFtyp()...)
	data = append(data, makeMoov()...)
	for i := 0; i < numFragments; i++ {
		data = append(data, makeMoof(uint32(i+1))...)
		data = append(data, makeMdat(256)...)
	}
	return data
}

// writeTempFile writes data to a temp file and returns the path.
func writeTempFile(t *testing.T, dir string, data []byte) string {
	t.Helper()
	path := filepath.Join(dir, "test.mp4")
	require.NoError(t, os.WriteFile(path, data, 0o644))
	return path
}
```

- [ ] **Step 2: Write failing test for complete file (no repair needed)**

```go
func TestRepairCompleteFile(t *testing.T) {
	dir := t.TempDir()
	data := buildValidFMP4(2)
	path := writeTempFile(t, dir, data)

	result, err := RepairSegment(path)
	require.NoError(t, err)
	assert.True(t, result.AlreadyComplete)
	assert.False(t, result.Repaired)
	assert.False(t, result.Unrecoverable)
	assert.Equal(t, int64(len(data)), result.OriginalSize)
	assert.Equal(t, 2, result.FragmentsRecovered)
}
```

- [ ] **Step 3: Run test to verify it fails**

Run: `cd <worktree> && go test ./internal/nvr/recovery/ -run TestRepairCompleteFile -v`
Expected: FAIL — `RepairSegment` not defined

- [ ] **Step 4: Write RepairSegment implementation**

```go
// internal/nvr/recovery/repair.go
package recovery

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
)

// RepairResult describes the outcome of repairing an fMP4 segment.
type RepairResult struct {
	Repaired           bool   // File was truncated to recover data
	AlreadyComplete    bool   // File was already structurally complete
	Unrecoverable      bool   // No complete fragments; file cannot be repaired
	OriginalSize       int64  // File size before repair
	NewSize            int64  // File size after repair (== OriginalSize if not repaired)
	FragmentsRecovered int    // Number of complete moof+mdat pairs found
	Detail             string // Human-readable description of what happened
}

// RepairSegment inspects an fMP4 file and truncates it to the last complete
// moof+mdat pair if the file is incomplete. This recovers data from segments
// that were being written when the process was killed.
func RepairSegment(path string) (RepairResult, error) {
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		return RepairResult{}, fmt.Errorf("open file: %w", err)
	}
	defer f.Close()

	info, err := f.Stat()
	if err != nil {
		return RepairResult{}, fmt.Errorf("stat file: %w", err)
	}
	fileSize := info.Size()

	if fileSize < 8 {
		return RepairResult{}, fmt.Errorf("file too small (%d bytes)", fileSize)
	}

	// Validate ftyp box.
	ftypSize, ftypType, err := readBoxHeader(f)
	if err != nil {
		return RepairResult{}, fmt.Errorf("reading ftyp: %w", err)
	}
	if ftypType != "ftyp" {
		return RepairResult{}, fmt.Errorf("not an fMP4 file: first box is %q, expected ftyp", ftypType)
	}

	// Skip to moov box.
	if _, err := f.Seek(ftypSize, io.SeekStart); err != nil {
		return RepairResult{}, err
	}

	moovSize, moovType, err := readBoxHeader(f)
	if err != nil {
		return RepairResult{Unrecoverable: true, OriginalSize: fileSize,
			Detail: "truncated before moov box"}, nil
	}
	if moovType != "moov" {
		return RepairResult{Unrecoverable: true, OriginalSize: fileSize,
			Detail: fmt.Sprintf("expected moov box, got %q", moovType)}, nil
	}

	initEnd := ftypSize + moovSize
	if initEnd > fileSize {
		return RepairResult{Unrecoverable: true, OriginalSize: fileSize,
			Detail: "moov box extends beyond file"}, nil
	}

	// Walk moof+mdat pairs.
	pos := initEnd
	lastCompleteEnd := initEnd // end of last complete moof+mdat pair
	fragments := 0

	for pos < fileSize {
		if _, err := f.Seek(pos, io.SeekStart); err != nil {
			break
		}

		moofSize, moofType, err := readBoxHeader(f)
		if err != nil {
			break // truncated box header — stop walking
		}
		if moofType != "moof" {
			break // unexpected box type — stop walking
		}
		if pos+moofSize > fileSize {
			break // moof extends beyond file
		}

		// Read mdat header after moof.
		if _, err := f.Seek(pos+moofSize, io.SeekStart); err != nil {
			break
		}
		mdatSize, mdatType, err := readBoxHeader(f)
		if err != nil {
			break // truncated mdat header
		}
		if mdatType != "mdat" {
			break // expected mdat after moof
		}

		// Handle mdat with size 0 (extends to EOF).
		if mdatSize == 0 {
			mdatSize = fileSize - (pos + moofSize)
		}

		pairEnd := pos + moofSize + mdatSize
		if pairEnd > fileSize {
			break // mdat extends beyond file — incomplete pair
		}

		fragments++
		lastCompleteEnd = pairEnd
		pos = pairEnd
	}

	result := RepairResult{
		OriginalSize:       fileSize,
		FragmentsRecovered: fragments,
	}

	if fragments == 0 {
		result.Unrecoverable = true
		result.Detail = "no complete moof+mdat pairs after moov"
		return result, nil
	}

	if lastCompleteEnd == fileSize {
		result.AlreadyComplete = true
		result.NewSize = fileSize
		result.Detail = fmt.Sprintf("file complete with %d fragments", fragments)
		return result, nil
	}

	// Truncate the file to the last complete pair.
	if err := f.Truncate(lastCompleteEnd); err != nil {
		return RepairResult{}, fmt.Errorf("truncate file: %w", err)
	}

	result.Repaired = true
	result.NewSize = lastCompleteEnd
	result.Detail = fmt.Sprintf("truncated from %d to %d bytes, recovered %d fragments",
		fileSize, lastCompleteEnd, fragments)
	return result, nil
}

// readBoxHeader reads an ISO BMFF box header (size + type). Handles both
// 32-bit and 64-bit extended sizes.
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
```

- [ ] **Step 5: Run test to verify it passes**

Run: `cd <worktree> && go test ./internal/nvr/recovery/ -run TestRepairCompleteFile -v`
Expected: PASS

- [ ] **Step 6: Write failing tests for truncation and edge cases**

```go
func TestRepairTruncatedMdat(t *testing.T) {
	dir := t.TempDir()
	data := buildValidFMP4(2)
	// Truncate 50 bytes off the end (into the second mdat).
	truncated := data[:len(data)-50]
	path := writeTempFile(t, dir, truncated)

	result, err := RepairSegment(path)
	require.NoError(t, err)
	assert.True(t, result.Repaired)
	assert.Equal(t, 1, result.FragmentsRecovered)
	assert.Less(t, result.NewSize, result.OriginalSize)

	// Verify the file was actually truncated on disk.
	info, err := os.Stat(path)
	require.NoError(t, err)
	assert.Equal(t, result.NewSize, info.Size())
}

func TestRepairTruncatedMoof(t *testing.T) {
	dir := t.TempDir()
	// Build 1 complete fragment, then append a partial moof.
	data := buildValidFMP4(1)
	partialMoof := makeMoof(2)[:10] // only first 10 bytes of moof
	data = append(data, partialMoof...)
	path := writeTempFile(t, dir, data)

	result, err := RepairSegment(path)
	require.NoError(t, err)
	assert.True(t, result.Repaired)
	assert.Equal(t, 1, result.FragmentsRecovered)
}

func TestRepairNoFragments(t *testing.T) {
	dir := t.TempDir()
	// ftyp + moov only, no moof/mdat.
	var data []byte
	data = append(data, makeFtyp()...)
	data = append(data, makeMoov()...)
	path := writeTempFile(t, dir, data)

	result, err := RepairSegment(path)
	require.NoError(t, err)
	assert.True(t, result.Unrecoverable)
	assert.Equal(t, 0, result.FragmentsRecovered)
}

func TestRepairTruncatedMoov(t *testing.T) {
	dir := t.TempDir()
	ftyp := makeFtyp()
	moov := makeMoov()
	// Truncate moov in half.
	data := append(ftyp, moov[:len(moov)/2]...)
	path := writeTempFile(t, dir, data)

	result, err := RepairSegment(path)
	require.NoError(t, err)
	assert.True(t, result.Unrecoverable)
}

func TestRepairEmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, []byte{})

	_, err := RepairSegment(path)
	assert.Error(t, err)
}

func TestRepairNotFMP4(t *testing.T) {
	dir := t.TempDir()
	path := writeTempFile(t, dir, []byte("this is not an fmp4 file at all"))

	_, err := RepairSegment(path)
	assert.Error(t, err)
}
```

- [ ] **Step 7: Run all repair tests**

Run: `cd <worktree> && go test ./internal/nvr/recovery/ -run TestRepair -v`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/recovery/repair.go internal/nvr/recovery/repair_test.go
git commit -m "feat(recovery): add fMP4 segment repair with box-walking truncation"
```

---

### Task 2: Recovery Scanner — Find Incomplete Segments

**Files:**

- Create: `internal/nvr/recovery/scanner.go`
- Create: `internal/nvr/recovery/scanner_test.go`

- [ ] **Step 1: Write the Candidate type and scanner interface**

```go
// internal/nvr/recovery/scanner.go
package recovery

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Candidate represents a segment file that may need repair.
type Candidate struct {
	FilePath  string // Absolute path to the file on disk
	HasDBEntry bool  // Whether a recording row exists in the DB
	RecordingID int64 // DB recording ID (0 if no DB entry)
}

// DBQuerier abstracts the database queries needed by the scanner.
type DBQuerier interface {
	// GetAllRecordingPaths returns all file_path values from the recordings table.
	GetAllRecordingPaths() (map[string]int64, error)
	// GetUnindexedRecordingPaths returns recording IDs and paths that have no fragments
	// and are not quarantined.
	GetUnindexedRecordingPaths() (map[string]int64, error)
}

// ScanForCandidates finds fMP4 files that may need recovery. It combines:
// 1. Orphaned files: .mp4 files on disk with no DB entry
// 2. Unindexed DB entries: recordings with no fragment rows
func ScanForCandidates(recordDirs []string, db DBQuerier) ([]Candidate, error) {
	// Get all known recording paths from DB.
	knownPaths, err := db.GetAllRecordingPaths()
	if err != nil {
		return nil, fmt.Errorf("query recording paths: %w", err)
	}

	// Get unindexed recordings (DB entry exists but no fragments).
	unindexedPaths, err := db.GetUnindexedRecordingPaths()
	if err != nil {
		return nil, fmt.Errorf("query unindexed recordings: %w", err)
	}

	var candidates []Candidate

	// Walk recording directories to find orphaned files.
	for _, dir := range recordDirs {
		err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return nil // skip inaccessible paths
			}
			if info.IsDir() {
				// Skip quarantine and recovery_failed directories.
				base := filepath.Base(path)
				if base == ".quarantine" || base == ".recovery_failed" {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".mp4") {
				return nil
			}
			if info.Size() < 8 {
				return nil // too small to be valid fMP4
			}

			absPath, _ := filepath.Abs(path)

			// Check if this file is known to the DB.
			if _, known := knownPaths[absPath]; !known {
				candidates = append(candidates, Candidate{
					FilePath:   absPath,
					HasDBEntry: false,
				})
			}
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walk %s: %w", dir, err)
		}
	}

	// Add unindexed DB entries as candidates.
	for path, recID := range unindexedPaths {
		// Only include if the file still exists on disk.
		if _, err := os.Stat(path); err == nil {
			candidates = append(candidates, Candidate{
				FilePath:    path,
				HasDBEntry:  true,
				RecordingID: recID,
			})
		}
	}

	return candidates, nil
}
```

- [ ] **Step 2: Write failing test for orphan detection**

```go
// internal/nvr/recovery/scanner_test.go
package recovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockDB implements DBQuerier for testing.
type mockDB struct {
	allPaths       map[string]int64
	unindexedPaths map[string]int64
}

func (m *mockDB) GetAllRecordingPaths() (map[string]int64, error) {
	if m.allPaths == nil {
		return map[string]int64{}, nil
	}
	return m.allPaths, nil
}

func (m *mockDB) GetUnindexedRecordingPaths() (map[string]int64, error) {
	if m.unindexedPaths == nil {
		return map[string]int64{}, nil
	}
	return m.unindexedPaths, nil
}

func TestScanFindsOrphans(t *testing.T) {
	dir := t.TempDir()
	// Create two .mp4 files — neither in DB.
	f1 := filepath.Join(dir, "cam1", "2026", "04", "01", "12-00-00.mp4")
	f2 := filepath.Join(dir, "cam1", "2026", "04", "01", "12-01-00.mp4")
	require.NoError(t, os.MkdirAll(filepath.Dir(f1), 0o755))
	require.NoError(t, os.WriteFile(f1, buildValidFMP4(1), 0o644))
	require.NoError(t, os.WriteFile(f2, buildValidFMP4(1), 0o644))

	db := &mockDB{}
	candidates, err := ScanForCandidates([]string{dir}, db)
	require.NoError(t, err)
	assert.Len(t, candidates, 2)
	for _, c := range candidates {
		assert.False(t, c.HasDBEntry)
	}
}

func TestScanSkipsIndexed(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "cam1", "segment.mp4")
	require.NoError(t, os.MkdirAll(filepath.Dir(fpath), 0o755))
	require.NoError(t, os.WriteFile(fpath, buildValidFMP4(1), 0o644))
	absPath, _ := filepath.Abs(fpath)

	db := &mockDB{
		allPaths: map[string]int64{absPath: 1},
	}
	candidates, err := ScanForCandidates([]string{dir}, db)
	require.NoError(t, err)
	assert.Len(t, candidates, 0)
}

func TestScanFindsUnindexed(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "cam1", "segment.mp4")
	require.NoError(t, os.MkdirAll(filepath.Dir(fpath), 0o755))
	require.NoError(t, os.WriteFile(fpath, buildValidFMP4(1), 0o644))
	absPath, _ := filepath.Abs(fpath)

	db := &mockDB{
		allPaths:       map[string]int64{absPath: 42},
		unindexedPaths: map[string]int64{absPath: 42},
	}
	candidates, err := ScanForCandidates([]string{dir}, db)
	require.NoError(t, err)
	assert.Len(t, candidates, 1)
	assert.True(t, candidates[0].HasDBEntry)
	assert.Equal(t, int64(42), candidates[0].RecordingID)
}

func TestScanSkipsQuarantined(t *testing.T) {
	dir := t.TempDir()
	qdir := filepath.Join(dir, ".quarantine", "cam1")
	require.NoError(t, os.MkdirAll(qdir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(qdir, "seg.mp4"), buildValidFMP4(1), 0o644))

	db := &mockDB{}
	candidates, err := ScanForCandidates([]string{dir}, db)
	require.NoError(t, err)
	assert.Len(t, candidates, 0)
}

func TestScanSkipsSmallFiles(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "tiny.mp4")
	require.NoError(t, os.WriteFile(fpath, []byte("tiny"), 0o644))

	db := &mockDB{}
	candidates, err := ScanForCandidates([]string{dir}, db)
	require.NoError(t, err)
	assert.Len(t, candidates, 0)
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `cd <worktree> && go test ./internal/nvr/recovery/ -run TestScan -v`
Expected: FAIL — `ScanForCandidates` not defined (until step 1 code is written)

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd <worktree> && go test ./internal/nvr/recovery/ -run TestScan -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/recovery/scanner.go internal/nvr/recovery/scanner_test.go
git commit -m "feat(recovery): add scanner to find incomplete segments on startup"
```

---

### Task 3: DB Queries for Recovery

**Files:**

- Modify: `internal/nvr/db/recordings.go` (add two new methods after line ~475)

- [ ] **Step 1: Write GetAllRecordingPaths query**

Add to `internal/nvr/db/recordings.go`:

```go
// GetAllRecordingPaths returns a map of file_path → recording ID for all recordings.
// Used by the recovery scanner to identify orphaned files on disk.
func (d *DB) GetAllRecordingPaths() (map[string]int64, error) {
	rows, err := d.Query("SELECT id, file_path FROM recordings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := make(map[string]int64)
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		paths[path] = id
	}
	return paths, rows.Err()
}
```

- [ ] **Step 2: Write GetUnindexedRecordingPaths query**

Add to `internal/nvr/db/recordings.go`:

```go
// GetUnindexedRecordingPaths returns a map of file_path → recording ID for recordings
// that have no fragment rows and are not quarantined. Used by recovery to find
// recordings that were inserted but never indexed (crash between insert and fragment scan).
func (d *DB) GetUnindexedRecordingPaths() (map[string]int64, error) {
	rows, err := d.Query(`
		SELECT r.id, r.file_path
		FROM recordings r
		LEFT JOIN recording_fragments rf ON rf.recording_id = r.id
		WHERE rf.id IS NULL AND r.status != 'quarantined'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := make(map[string]int64)
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		paths[path] = id
	}
	return paths, rows.Err()
}
```

- [ ] **Step 3: Write UpdateRecordingFileSize query**

Add to `internal/nvr/db/recordings.go`:

```go
// UpdateRecordingFileSize updates the file_size for a recording after repair.
func (d *DB) UpdateRecordingFileSize(id int64, fileSize int64) error {
	_, err := d.Exec("UPDATE recordings SET file_size = ? WHERE id = ?", fileSize, id)
	return err
}
```

- [ ] **Step 4: Verify compilation**

Run: `cd <worktree> && go build ./internal/nvr/db/`
Expected: Success

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/recordings.go
git commit -m "feat(db): add recovery queries for orphaned and unindexed recordings"
```

---

### Task 4: Reconciliation Logic

**Files:**

- Create: `internal/nvr/recovery/reconcile.go`
- Create: `internal/nvr/recovery/reconcile_test.go`

- [ ] **Step 1: Write Reconciler interface and Reconcile function**

```go
// internal/nvr/recovery/reconcile.go
package recovery

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Reconciler abstracts the database operations needed for reconciliation.
type Reconciler interface {
	InsertRecording(cameraID, streamID string, startTime, endTime time.Time, durationMs, fileSize int64, filePath, format string) (int64, error)
	UpdateRecordingFileSize(id int64, fileSize int64) error
	UpdateRecordingStatus(id int64, status string, detail *string, verifiedAt string) error
	MatchCameraFromPath(filePath string) (cameraID string, streamID string, ok bool)
}

// ReconcileResult summarizes the reconciliation outcome.
type ReconcileResult struct {
	Inserted      int
	Updated       int
	MarkedCorrupt int
	Skipped       int
}

// RepairOutcome pairs a candidate with its repair result.
type RepairOutcome struct {
	Candidate Candidate
	Result    RepairResult
}

// Reconcile updates the database for each repaired segment.
// - Orphaned files (no DB entry): insert a new recording.
// - Existing entries with stale data: update file_size.
// - Unrecoverable files: mark as corrupted.
func Reconcile(outcomes []RepairOutcome, rec Reconciler) (ReconcileResult, error) {
	var summary ReconcileResult

	for _, o := range outcomes {
		if o.Result.Unrecoverable {
			if o.Candidate.HasDBEntry {
				detail := o.Result.Detail
				now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
				if err := rec.UpdateRecordingStatus(o.Candidate.RecordingID, "corrupted", &detail, now); err != nil {
					return summary, fmt.Errorf("mark corrupted recording %d: %w", o.Candidate.RecordingID, err)
				}
				summary.MarkedCorrupt++
			} else {
				// No DB entry and unrecoverable — move to .recovery_failed/.
				if err := moveToRecoveryFailed(o.Candidate.FilePath); err != nil {
					fmt.Fprintf(os.Stderr, "recovery: failed to move unrecoverable file %s: %v\n", o.Candidate.FilePath, err)
				}
				summary.MarkedCorrupt++
			}
			continue
		}

		if o.Candidate.HasDBEntry {
			// Existing DB entry — update file_size if repaired.
			if o.Result.Repaired {
				if err := rec.UpdateRecordingFileSize(o.Candidate.RecordingID, o.Result.NewSize); err != nil {
					return summary, fmt.Errorf("update file size for recording %d: %w", o.Candidate.RecordingID, err)
				}
				summary.Updated++
			} else {
				summary.Skipped++
			}
		} else {
			// Orphaned file — insert new recording.
			cameraID, streamID, ok := rec.MatchCameraFromPath(o.Candidate.FilePath)
			if !ok {
				summary.Skipped++
				continue
			}

			// Estimate duration from fragments. Use 1s per fragment as rough estimate
			// since actual duration requires parsing trun boxes (done later by backfill).
			estimatedDuration := time.Duration(o.Result.FragmentsRecovered) * time.Second
			now := time.Now().UTC()
			start := now.Add(-estimatedDuration)

			size := o.Result.NewSize
			if size == 0 {
				size = o.Result.OriginalSize
			}

			_, err := rec.InsertRecording(cameraID, streamID, start, now,
				estimatedDuration.Milliseconds(), size, o.Candidate.FilePath, "fmp4")
			if err != nil {
				return summary, fmt.Errorf("insert recovered recording %s: %w", o.Candidate.FilePath, err)
			}
			summary.Inserted++
		}
	}

	return summary, nil
}

// moveToRecoveryFailed moves an unrecoverable file to a .recovery_failed directory
// adjacent to its parent recording directory.
func moveToRecoveryFailed(filePath string) error {
	dir := filepath.Dir(filePath)
	base := filepath.Base(filePath)

	// Walk up to find the recording root (directory containing "nvr/").
	recoveryDir := dir
	if idx := strings.Index(dir, "nvr/"); idx >= 0 {
		root := dir[:idx+4] // includes "nvr/"
		rel, _ := filepath.Rel(root, dir)
		recoveryDir = filepath.Join(root, ".recovery_failed", rel)
	} else {
		recoveryDir = filepath.Join(dir, ".recovery_failed")
	}

	if err := os.MkdirAll(recoveryDir, 0o755); err != nil {
		return err
	}
	return os.Rename(filePath, filepath.Join(recoveryDir, base))
}
```

- [ ] **Step 2: Add missing filepath import**

The `moveToRecoveryFailed` function uses `filepath`. Ensure the import block includes:

```go
import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)
```

- [ ] **Step 3: Write failing tests for reconciliation**

```go
// internal/nvr/recovery/reconcile_test.go
package recovery

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockReconciler struct {
	inserted   []string // file paths inserted
	updated    map[int64]int64 // recording ID → new file size
	corrupted  map[int64]string // recording ID → detail
	cameraMap  map[string][2]string // path substring → [cameraID, streamID]
}

func newMockReconciler() *mockReconciler {
	return &mockReconciler{
		updated:   make(map[int64]int64),
		corrupted: make(map[int64]string),
		cameraMap: make(map[string][2]string),
	}
}

func (m *mockReconciler) InsertRecording(cameraID, streamID string, startTime, endTime time.Time, durationMs, fileSize int64, filePath, format string) (int64, error) {
	m.inserted = append(m.inserted, filePath)
	return int64(len(m.inserted)), nil
}

func (m *mockReconciler) UpdateRecordingFileSize(id int64, fileSize int64) error {
	m.updated[id] = fileSize
	return nil
}

func (m *mockReconciler) UpdateRecordingStatus(id int64, status string, detail *string, verifiedAt string) error {
	d := ""
	if detail != nil {
		d = *detail
	}
	m.corrupted[id] = d
	return nil
}

func (m *mockReconciler) MatchCameraFromPath(filePath string) (string, string, bool) {
	for substr, ids := range m.cameraMap {
		if len(filePath) > 0 && len(substr) > 0 && contains(filePath, substr) {
			return ids[0], ids[1], true
		}
	}
	return "", "", false
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexStr(s, substr) >= 0)
}

func indexStr(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestReconcileOrphanedFile(t *testing.T) {
	rec := newMockReconciler()
	rec.cameraMap["cam1"] = [2]string{"camera-1", "main"}

	outcomes := []RepairOutcome{{
		Candidate: Candidate{FilePath: "/recordings/nvr/cam1/seg.mp4", HasDBEntry: false},
		Result:    RepairResult{Repaired: true, NewSize: 1024, FragmentsRecovered: 3},
	}}

	result, err := Reconcile(outcomes, rec)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Inserted)
	assert.Len(t, rec.inserted, 1)
}

func TestReconcileUnindexedEntry(t *testing.T) {
	rec := newMockReconciler()

	outcomes := []RepairOutcome{{
		Candidate: Candidate{FilePath: "/recordings/seg.mp4", HasDBEntry: true, RecordingID: 42},
		Result:    RepairResult{Repaired: true, NewSize: 2048, OriginalSize: 3000, FragmentsRecovered: 5},
	}}

	result, err := Reconcile(outcomes, rec)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Updated)
	assert.Equal(t, int64(2048), rec.updated[42])
}

func TestReconcileUnrecoverableWithDB(t *testing.T) {
	rec := newMockReconciler()

	outcomes := []RepairOutcome{{
		Candidate: Candidate{FilePath: "/recordings/seg.mp4", HasDBEntry: true, RecordingID: 99},
		Result:    RepairResult{Unrecoverable: true, Detail: "no complete moof+mdat pairs"},
	}}

	result, err := Reconcile(outcomes, rec)
	require.NoError(t, err)
	assert.Equal(t, 1, result.MarkedCorrupt)
	assert.Contains(t, rec.corrupted[99], "no complete moof+mdat pairs")
}

func TestReconcileAlreadyComplete(t *testing.T) {
	rec := newMockReconciler()

	outcomes := []RepairOutcome{{
		Candidate: Candidate{FilePath: "/recordings/seg.mp4", HasDBEntry: true, RecordingID: 10},
		Result:    RepairResult{AlreadyComplete: true, FragmentsRecovered: 5},
	}}

	result, err := Reconcile(outcomes, rec)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Skipped)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd <worktree> && go test ./internal/nvr/recovery/ -run TestReconcile -v`
Expected: All PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/recovery/reconcile.go internal/nvr/recovery/reconcile_test.go
git commit -m "feat(recovery): add reconciliation logic for repaired segments"
```

---

### Task 5: Recovery Orchestrator — Run Function

**Files:**

- Create: `internal/nvr/recovery/recovery.go`
- Create: `internal/nvr/recovery/recovery_test.go`

- [ ] **Step 1: Write the Run function that orchestrates scan → repair → reconcile**

```go
// internal/nvr/recovery/recovery.go
package recovery

import (
	"fmt"
	"os"
)

// Logger abstracts logging for the recovery system.
type Logger interface {
	Log(level, format string, args ...interface{})
}

// stdLogger writes to stderr.
type stdLogger struct{}

func (l *stdLogger) Log(level, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(os.Stderr, "NVR recovery [%s]: %s\n", level, msg)
}

// RunConfig holds the parameters for a recovery run.
type RunConfig struct {
	RecordDirs []string
	DB         DBQuerier
	Reconciler Reconciler
	Logger     Logger
}

// RunResult summarizes the full recovery run.
type RunResult struct {
	Scanned       int
	Repaired      int
	AlreadyOK     int
	Unrecoverable int
	Reconcile     ReconcileResult
}

// Run performs startup recovery: scan for incomplete segments, repair them,
// and reconcile the database. This should be called synchronously during
// NVR initialization, before fragment backfill and recorder startup.
func Run(cfg RunConfig) (RunResult, error) {
	if cfg.Logger == nil {
		cfg.Logger = &stdLogger{}
	}

	cfg.Logger.Log("info", "starting recovery scan across %d directories", len(cfg.RecordDirs))

	// Phase 1: Scan.
	candidates, err := ScanForCandidates(cfg.RecordDirs, cfg.DB)
	if err != nil {
		return RunResult{}, fmt.Errorf("scan: %w", err)
	}

	if len(candidates) == 0 {
		cfg.Logger.Log("info", "no incomplete segments found")
		return RunResult{}, nil
	}

	cfg.Logger.Log("info", "found %d candidate segments", len(candidates))

	// Phase 2: Repair.
	var outcomes []RepairOutcome
	var result RunResult
	result.Scanned = len(candidates)

	for _, c := range candidates {
		repairResult, err := RepairSegment(c.FilePath)
		if err != nil {
			cfg.Logger.Log("warn", "repair failed for %s: %v", c.FilePath, err)
			result.Unrecoverable++
			continue
		}

		outcomes = append(outcomes, RepairOutcome{
			Candidate: c,
			Result:    repairResult,
		})

		switch {
		case repairResult.Repaired:
			result.Repaired++
			cfg.Logger.Log("info", "repaired %s: %s", c.FilePath, repairResult.Detail)
		case repairResult.AlreadyComplete:
			result.AlreadyOK++
			cfg.Logger.Log("debug", "already complete: %s", c.FilePath)
		case repairResult.Unrecoverable:
			result.Unrecoverable++
			cfg.Logger.Log("warn", "unrecoverable: %s — %s", c.FilePath, repairResult.Detail)
		}
	}

	// Phase 3: Reconcile.
	reconcileResult, err := Reconcile(outcomes, cfg.Reconciler)
	if err != nil {
		return result, fmt.Errorf("reconcile: %w", err)
	}
	result.Reconcile = reconcileResult

	cfg.Logger.Log("info", "recovery complete: scanned=%d repaired=%d ok=%d unrecoverable=%d inserted=%d updated=%d corrupt=%d",
		result.Scanned, result.Repaired, result.AlreadyOK, result.Unrecoverable,
		reconcileResult.Inserted, reconcileResult.Updated, reconcileResult.MarkedCorrupt)

	return result, nil
}
```

- [ ] **Step 2: Write integration test using temp files and mock DB**

```go
// internal/nvr/recovery/recovery_test.go
package recovery

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunEndToEnd(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "nvr", "cam1", "main")
	require.NoError(t, os.MkdirAll(subdir, 0o755))

	// Create a complete file (should be skipped or marked OK).
	completeData := buildValidFMP4(3)
	completePath := filepath.Join(subdir, "complete.mp4")
	require.NoError(t, os.WriteFile(completePath, completeData, 0o644))

	// Create a truncated file (should be repaired).
	truncatedData := buildValidFMP4(2)
	// Chop off last 30 bytes to make the second mdat incomplete.
	truncatedData = truncatedData[:len(truncatedData)-30]
	truncatedPath := filepath.Join(subdir, "truncated.mp4")
	require.NoError(t, os.WriteFile(truncatedPath, truncatedData, 0o644))

	// Create an unrecoverable file (ftyp + moov, no fragments).
	var noFragData []byte
	noFragData = append(noFragData, makeFtyp()...)
	noFragData = append(noFragData, makeMoov()...)
	noFragPath := filepath.Join(subdir, "nofrag.mp4")
	require.NoError(t, os.WriteFile(noFragPath, noFragData, 0o644))

	// All files are orphans (not in DB).
	db := &mockDB{}
	rec := newMockReconciler()
	rec.cameraMap["cam1"] = [2]string{"camera-1", "main"}

	result, err := Run(RunConfig{
		RecordDirs: []string{dir},
		DB:         db,
		Reconciler: rec,
	})
	require.NoError(t, err)

	assert.Equal(t, 3, result.Scanned)
	assert.Equal(t, 1, result.Repaired)      // truncated.mp4
	assert.Equal(t, 1, result.AlreadyOK)     // complete.mp4
	assert.Equal(t, 1, result.Unrecoverable) // nofrag.mp4

	// The complete and truncated files should have been inserted as new recordings.
	assert.Equal(t, 2, result.Reconcile.Inserted)

	// Verify the truncated file was actually repaired on disk.
	info, err := os.Stat(truncatedPath)
	require.NoError(t, err)
	assert.Less(t, info.Size(), int64(len(truncatedData)))
}

func TestRunNoRecordDirs(t *testing.T) {
	db := &mockDB{}
	rec := newMockReconciler()

	result, err := Run(RunConfig{
		RecordDirs: []string{},
		DB:         db,
		Reconciler: rec,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.Scanned)
}

func TestRunEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	db := &mockDB{}
	rec := newMockReconciler()

	result, err := Run(RunConfig{
		RecordDirs: []string{dir},
		DB:         db,
		Reconciler: rec,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.Scanned)
}
```

- [ ] **Step 3: Run tests**

Run: `cd <worktree> && go test ./internal/nvr/recovery/ -run TestRun -v`
Expected: All PASS

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/recovery/recovery.go internal/nvr/recovery/recovery_test.go
git commit -m "feat(recovery): add Run orchestrator for scan-repair-reconcile pipeline"
```

---

### Task 6: Integrate Recovery into NVR Startup

**Files:**

- Modify: `internal/nvr/nvr.go` (lines ~37-70 for struct, ~200 for startup sequence)

- [ ] **Step 1: Add the recovery adapter type**

Create a file for the NVR-side adapter that bridges the recovery interfaces to the real DB. Add to `internal/nvr/recovery_adapter.go`:

```go
// internal/nvr/recovery_adapter.go
package nvr

import (
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/recovery"
)

// recoveryDBAdapter implements recovery.DBQuerier using the real DB.
type recoveryDBAdapter struct {
	db *db.DB
}

func (a *recoveryDBAdapter) GetAllRecordingPaths() (map[string]int64, error) {
	return a.db.GetAllRecordingPaths()
}

func (a *recoveryDBAdapter) GetUnindexedRecordingPaths() (map[string]int64, error) {
	return a.db.GetUnindexedRecordingPaths()
}

// recoveryReconcileAdapter implements recovery.Reconciler using the real DB and NVR.
type recoveryReconcileAdapter struct {
	db  *db.DB
	nvr *NVR
}

func (a *recoveryReconcileAdapter) InsertRecording(cameraID, streamID string, startTime, endTime time.Time, durationMs, fileSize int64, filePath, format string) (int64, error) {
	rec := &db.Recording{
		CameraID:   cameraID,
		StreamID:   streamID,
		StartTime:  startTime.UTC().Format("2006-01-02T15:04:05.000Z"),
		EndTime:    endTime.UTC().Format("2006-01-02T15:04:05.000Z"),
		DurationMs: durationMs,
		FilePath:   filePath,
		FileSize:   fileSize,
		Format:     format,
	}
	if err := a.db.InsertRecording(rec); err != nil {
		return 0, err
	}
	return rec.ID, nil
}

func (a *recoveryReconcileAdapter) UpdateRecordingFileSize(id int64, fileSize int64) error {
	return a.db.UpdateRecordingFileSize(id, fileSize)
}

func (a *recoveryReconcileAdapter) UpdateRecordingStatus(id int64, status string, detail *string, verifiedAt string) error {
	return a.db.UpdateRecordingStatus(id, status, detail, verifiedAt)
}

func (a *recoveryReconcileAdapter) MatchCameraFromPath(filePath string) (cameraID string, streamID string, ok bool) {
	// Extract camera ID from path convention: .../nvr/<camera-id>/...
	if idx := strings.Index(filePath, "nvr/"); idx >= 0 {
		rest := filePath[idx+4:]
		parts := strings.SplitN(rest, "/", 2)
		if len(parts) >= 1 {
			candidate := parts[0]
			stream := "main"
			if tildeIdx := strings.Index(candidate, "~"); tildeIdx > 0 {
				stream = candidate[tildeIdx+1:]
				candidate = candidate[:tildeIdx]
			}
			if c, err := a.db.GetCamera(candidate); err == nil && c != nil {
				return c.ID, stream, true
			}
		}
	}

	// Fallback: match against known camera paths.
	cameras, err := a.db.ListCameras()
	if err != nil {
		return "", "", false
	}
	for _, c := range cameras {
		if c.MediaMTXPath != "" && strings.Contains(filePath, c.MediaMTXPath) {
			return c.ID, "main", true
		}
	}
	return "", "", false
}
```

- [ ] **Step 2: Hook recovery into NVR.Initialize()**

In `internal/nvr/nvr.go`, add the recovery import and call. Insert the recovery call **after** line 199 (`n.syncAudioTranscodeState()`) and **before** line 201 (`n.startFragmentBackfill()`):

Add to imports:

```go
"github.com/bluenviron/mediamtx/internal/nvr/recovery"
```

Insert between `syncAudioTranscodeState()` and `startFragmentBackfill()`:

```go
	// Run startup recovery: detect and repair incomplete segments from crashes.
	if n.RecordingsPath != "" {
		recoveryCfg := recovery.RunConfig{
			RecordDirs: []string{n.RecordingsPath},
			DB:         &recoveryDBAdapter{db: n.database},
			Reconciler: &recoveryReconcileAdapter{db: n.database, nvr: n},
		}
		if result, err := recovery.Run(recoveryCfg); err != nil {
			log.Printf("NVR: recovery scan failed: %v", err)
		} else if result.Scanned > 0 {
			log.Printf("NVR: recovery complete — scanned=%d repaired=%d unrecoverable=%d",
				result.Scanned, result.Repaired, result.Unrecoverable)
		}
	}
```

- [ ] **Step 3: Verify compilation**

Run: `cd <worktree> && go build ./internal/nvr/`
Expected: Success

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/recovery_adapter.go internal/nvr/nvr.go
git commit -m "feat(nvr): integrate recovery scanner into startup before backfill"
```

---

### Task 7: Run Full Test Suite and Fix Issues

**Files:**

- All files from tasks 1-6

- [ ] **Step 1: Run all recovery package tests**

Run: `cd <worktree> && go test ./internal/nvr/recovery/ -v`
Expected: All PASS

- [ ] **Step 2: Run NVR package compilation check**

Run: `cd <worktree> && go build ./internal/nvr/...`
Expected: Success

- [ ] **Step 3: Run existing NVR tests to check for regressions**

Run: `cd <worktree> && go test ./internal/nvr/... -v -count=1 -timeout 120s 2>&1 | tail -50`
Expected: All PASS (or expected skips for integration tests)

- [ ] **Step 4: Run full project build**

Run: `cd <worktree> && go build ./...`
Expected: Success

- [ ] **Step 5: Fix any issues found**

If any tests fail, fix them before proceeding.

- [ ] **Step 6: Final commit if any fixes were needed**

```bash
git add -A
git commit -m "fix(recovery): resolve test and compilation issues"
```
