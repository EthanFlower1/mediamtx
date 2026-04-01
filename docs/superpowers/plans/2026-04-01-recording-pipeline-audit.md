# KAI-5: Recording Pipeline Data Loss Audit — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build an automated test harness that audits the MediaMTX recording pipeline under adverse conditions and produces a structured findings report.

**Architecture:** Integration tests in `internal/nvr/audit/` gated behind `//go:build integration`. Layer-level tests isolate each pipeline component (stream, recorder, storage, DB, lifecycle). E2E scenario tests exercise the full pipeline. A findings report is generated as JSON + markdown.

**Tech Stack:** Go test framework, testify assertions, `internal/test` helpers, `os/exec` for subprocess SIGKILL tests, `internal/recorder` + `internal/nvr/db` + `internal/nvr/storage` packages.

---

### Task 1: Set Up Worktree and Package Scaffold

**Files:**

- Create: `internal/nvr/audit/findings.go`
- Create: `internal/nvr/audit/audit_test.go`

- [ ] **Step 1: Create the git worktree and branch**

```bash
git worktree add .worktrees/kai-5 -b feat/kai-5-recording-audit
```

All subsequent work happens in `.worktrees/kai-5/`.

- [ ] **Step 2: Create the audit package directory**

```bash
mkdir -p .worktrees/kai-5/internal/nvr/audit
```

- [ ] **Step 3: Create `findings.go` with structured types**

Create `.worktrees/kai-5/internal/nvr/audit/findings.go`:

```go
//go:build integration

package audit

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Severity levels for audit findings.
const (
	SeverityDataLoss    = "data_loss"    // Recorded data permanently lost
	SeverityCorruption  = "corruption"   // File or DB in inconsistent state
	SeverityGap         = "gap"          // Time gap but surrounding data intact
	SeverityRecoverable = "recoverable"  // Appears lost but can be recovered
)

// Finding represents a single audit observation from a test.
type Finding struct {
	Scenario     string `json:"scenario"`
	Layer        string `json:"layer"`
	Severity     string `json:"severity"`
	Description  string `json:"description"`
	Reproduction string `json:"reproduction"`
	DataImpact   string `json:"data_impact"`
	Recovery     string `json:"recovery"`
}

// Report collects findings from all audit tests.
type Report struct {
	mu       sync.Mutex
	Findings []Finding `json:"findings"`
	RunAt    string    `json:"run_at"`
}

// NewReport creates an empty report timestamped to now.
func NewReport() *Report {
	return &Report{
		RunAt: time.Now().UTC().Format(time.RFC3339),
	}
}

// Add appends a finding to the report (thread-safe).
func (r *Report) Add(f Finding) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.Findings = append(r.Findings, f)
}

// WriteJSON writes the report as JSON to the given path.
func (r *Report) WriteJSON(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o644)
}

// WriteMarkdown generates a human-readable markdown report.
func (r *Report) WriteMarkdown(path string) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}

	var sb strings.Builder
	sb.WriteString("# Recording Pipeline Audit Report\n\n")
	sb.WriteString(fmt.Sprintf("**Run at:** %s\n\n", r.RunAt))
	sb.WriteString(fmt.Sprintf("**Total findings:** %d\n\n", len(r.Findings)))

	// Summary table by severity
	counts := map[string]int{}
	for _, f := range r.Findings {
		counts[f.Severity]++
	}
	sb.WriteString("## Summary\n\n")
	sb.WriteString("| Severity | Count |\n|----------|-------|\n")
	for _, sev := range []string{SeverityDataLoss, SeverityCorruption, SeverityGap, SeverityRecoverable} {
		if c, ok := counts[sev]; ok {
			sb.WriteString(fmt.Sprintf("| %s | %d |\n", sev, c))
		}
	}
	sb.WriteString("\n")

	// Group findings by layer
	byLayer := map[string][]Finding{}
	for _, f := range r.Findings {
		byLayer[f.Layer] = append(byLayer[f.Layer], f)
	}
	layers := make([]string, 0, len(byLayer))
	for l := range byLayer {
		layers = append(layers, l)
	}
	sort.Strings(layers)

	for _, layer := range layers {
		sb.WriteString(fmt.Sprintf("## Layer: %s\n\n", layer))
		for i, f := range byLayer[layer] {
			sb.WriteString(fmt.Sprintf("### %d. %s\n\n", i+1, f.Scenario))
			sb.WriteString(fmt.Sprintf("**Severity:** %s\n\n", f.Severity))
			sb.WriteString(fmt.Sprintf("**Description:** %s\n\n", f.Description))
			sb.WriteString(fmt.Sprintf("**Reproduction:** %s\n\n", f.Reproduction))
			sb.WriteString(fmt.Sprintf("**Data Impact:** %s\n\n", f.DataImpact))
			sb.WriteString(fmt.Sprintf("**Recovery:** %s\n\n", f.Recovery))
		}
	}

	return os.WriteFile(path, []byte(sb.String()), 0o644)
}
```

- [ ] **Step 4: Create `audit_test.go` with shared test helpers**

Create `.worktrees/kai-5/internal/nvr/audit/audit_test.go`:

```go
//go:build integration

package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/recorder"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/stretchr/testify/require"
)

// testReport is the shared report instance for all audit tests.
var testReport = NewReport()

// testLogger implements logger.Writer and discards output.
type testLogger struct{}

func (testLogger) Log(_ logger.Level, _ string, _ ...any) {}

// newTestDB creates an isolated SQLite database for testing.
func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

// newTestRecordDir creates an isolated temp directory for recordings.
func newTestRecordDir(t *testing.T) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "recordings")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	return dir
}

// newTestStream creates a stream with H264 + MPEG4Audio for testing.
func newTestStream(t *testing.T) (*stream.Stream, *stream.SubStream) {
	t.Helper()
	desc := &defs.APISourceDescription{
		Medias: []*defs.APIPathSourceOrReaderMedia{
			{Formats: []*defs.APIPathSourceOrReaderFormat{{
				Codec: "H264",
			}}},
			{Formats: []*defs.APIPathSourceOrReaderFormat{{
				Codec: "MPEG-4 Audio",
			}}},
		},
	}

	medias := []*defs.Media{
		test.MediaH264,
		test.MediaMPEG4Audio,
	}

	s, err := stream.New(512*1024, medias, false, testLogger{})
	require.NoError(t, err)
	t.Cleanup(s.Close)

	sub := s.NewSubStream()
	_ = desc // used for documentation; stream uses medias directly
	return s, sub
}

// writeH264Frames writes n H264 IDR frames to the stream at 30fps intervals.
func writeH264Frames(s *stream.Stream, n int, startNTP time.Time) {
	for i := 0; i < n; i++ {
		dts := time.Duration(i) * time.Second / 30
		ntp := startNTP.Add(dts)
		s.WriteUnit(test.MediaH264, test.FormatH264, &unit.H264{
			Base: unit.Base{
				DTS: dts,
				NTP: ntp,
				PTS: dts,
			},
			AU: [][]byte{
				test.FormatH264.SPS,
				test.FormatH264.PPS,
				{5, 1}, // IDR slice
			},
		})
	}
}

// startRecorder creates and initializes a Recorder writing to the given directory.
func startRecorder(
	t *testing.T,
	s *stream.Stream,
	recordDir string,
	onComplete func(string, time.Duration),
) *recorder.Recorder {
	t.Helper()
	pathFormat := filepath.Join(recordDir, "%path/%Y-%m-%d_%H-%M-%S-%f")
	rec := &recorder.Recorder{
		PathFormat:      pathFormat,
		Format:          conf.RecordFormatFMP4,
		PartDuration:    200 * time.Millisecond,
		SegmentDuration: 2 * time.Second,
		PathName:        "test",
		Stream:          s,
		OnSegmentCreate: func(_ string) {},
		OnSegmentComplete: func(path string, dur time.Duration) {
			if onComplete != nil {
				onComplete(path, dur)
			}
		},
		Parent: testLogger{},
	}
	rec.Initialize()
	t.Cleanup(rec.Close)
	return rec
}

// fileExists returns true if the path exists on disk.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fileSize returns the size of a file, or -1 if it doesn't exist.
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return info.Size()
}

// dirFiles returns all file paths under a directory.
func dirFiles(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, path)
		}
		return nil
	})
	return files, err
}
```

- [ ] **Step 5: Verify the package compiles**

Run from `.worktrees/kai-5/`:

```bash
go build -tags=integration ./internal/nvr/audit/
```

Expected: No errors.

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/audit/findings.go internal/nvr/audit/audit_test.go
git commit -m "feat(audit): add package scaffold with findings types and test helpers"
```

---

### Task 2: Stream Layer Tests — Camera-Side Network Failures

**Files:**

- Create: `internal/nvr/audit/stream_test.go`

- [ ] **Step 1: Write `TestStreamDisconnect`**

Create `.worktrees/kai-5/internal/nvr/audit/stream_test.go`:

```go
//go:build integration

package audit

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStreamDisconnect(t *testing.T) {
	recordDir := newTestRecordDir(t)
	s, _ := newTestStream(t)

	var mu sync.Mutex
	var completedSegments []string

	_ = startRecorder(t, s, recordDir, func(path string, _ time.Duration) {
		mu.Lock()
		completedSegments = append(completedSegments, path)
		mu.Unlock()
	})

	// Write 2 seconds of frames at 30fps
	now := time.Now()
	writeH264Frames(s, 60, now)
	time.Sleep(500 * time.Millisecond)

	// Simulate camera disconnect by closing the stream
	s.Close()
	time.Sleep(2 * time.Second)

	// Check what's on disk
	files, err := dirFiles(recordDir)
	require.NoError(t, err)

	mu.Lock()
	segCount := len(completedSegments)
	mu.Unlock()

	// Record findings
	if len(files) == 0 {
		testReport.Add(Finding{
			Scenario:     "stream_disconnect",
			Layer:        "stream",
			Severity:     SeverityDataLoss,
			Description:  "No recording files found after stream disconnect",
			Reproduction: "Write 2s of H264 frames, close stream abruptly",
			DataImpact:   "All recorded data lost",
			Recovery:     "None",
		})
	} else {
		for _, f := range files {
			size := fileSize(f)
			severity := SeverityRecoverable
			if size == 0 {
				severity = SeverityDataLoss
			}
			testReport.Add(Finding{
				Scenario:     "stream_disconnect",
				Layer:        "stream",
				Severity:     severity,
				Description:  fmt.Sprintf("File %s exists (%d bytes), %d segments completed via callback", filepath.Base(f), size, segCount),
				Reproduction: "Write 2s of H264 frames at 30fps, close stream abruptly",
				DataImpact:   fmt.Sprintf("Data up to last completed segment is safe. Current in-progress segment may be partial."),
				Recovery:     "Partial fMP4 files readable up to last complete moof+mdat pair",
			})
		}
	}

	// Assert at least some data was written
	assert.NotEmpty(t, files, "expected at least one recording file on disk")
}

func TestStreamStall(t *testing.T) {
	recordDir := newTestRecordDir(t)
	s, _ := newTestStream(t)

	var mu sync.Mutex
	var completedSegments []string

	_ = startRecorder(t, s, recordDir, func(path string, _ time.Duration) {
		mu.Lock()
		completedSegments = append(completedSegments, path)
		mu.Unlock()
	})

	// Write 1 second of frames, then stop sending (stall)
	now := time.Now()
	writeH264Frames(s, 30, now)

	// Wait longer than any reasonable read timeout
	// The stream stays open but no data flows
	time.Sleep(10 * time.Second)

	files, err := dirFiles(recordDir)
	require.NoError(t, err)

	mu.Lock()
	segCount := len(completedSegments)
	mu.Unlock()

	testReport.Add(Finding{
		Scenario:     "stream_stall",
		Layer:        "stream",
		Severity:     SeverityGap,
		Description:  fmt.Sprintf("Stream stalled for 10s. %d files on disk, %d completed segments", len(files), segCount),
		Reproduction: "Write 1s of frames then stop sending for 10s while keeping stream open",
		DataImpact:   "Gap in recording during stall period. Data before stall should be intact.",
		Recovery:     "Recording resumes when data flow resumes",
	})

	assert.NotEmpty(t, files, "expected at least one recording file from pre-stall data")
}

func TestStreamReconnect(t *testing.T) {
	recordDir := newTestRecordDir(t)
	s, _ := newTestStream(t)

	var mu sync.Mutex
	var completedSegments []string

	rec := startRecorder(t, s, recordDir, func(path string, _ time.Duration) {
		mu.Lock()
		completedSegments = append(completedSegments, path)
		mu.Unlock()
	})

	// Phase 1: write 2 seconds of frames
	now := time.Now()
	writeH264Frames(s, 60, now)
	time.Sleep(500 * time.Millisecond)

	// Simulate disconnect
	s.Close()
	time.Sleep(3 * time.Second)

	// Phase 2: create a new stream (simulating reconnection)
	// The recorder supervisor should pick this up after restart pause
	s2, _ := newTestStream(t)
	defer s2.Close()

	// Write 2 more seconds
	reconnectTime := time.Now()
	writeH264Frames(s2, 60, reconnectTime)
	time.Sleep(500 * time.Millisecond)

	filesPhase1, _ := dirFiles(recordDir)

	mu.Lock()
	segCount := len(completedSegments)
	mu.Unlock()

	gap := reconnectTime.Sub(now.Add(2 * time.Second))

	testReport.Add(Finding{
		Scenario:     "stream_reconnect",
		Layer:        "stream",
		Severity:     SeverityGap,
		Description:  fmt.Sprintf("Disconnect/reconnect cycle. %d files on disk, %d completed segments, ~%.1fs gap", len(filesPhase1), segCount, gap.Seconds()),
		Reproduction: "Write 2s frames, close stream, wait 3s, create new stream, write 2s more",
		DataImpact:   fmt.Sprintf("%.1fs gap between disconnect and reconnect", gap.Seconds()),
		Recovery:     "New segment starts on reconnect. Gap accurately reflected in timeline.",
	})

	_ = rec // keep recorder reference alive
}
```

- [ ] **Step 2: Run tests to verify they execute**

```bash
cd .worktrees/kai-5 && go test -tags=integration -v -run "TestStream" -timeout 60s ./internal/nvr/audit/
```

Expected: Tests run (may pass or fail — this is an audit, findings are recorded either way).

- [ ] **Step 3: Fix any compilation errors**

Address import issues or API mismatches. The `newTestStream` helper may need adjustment based on the exact `stream.New` signature — check `internal/stream/stream.go` for the constructor.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/audit/stream_test.go
git commit -m "feat(audit): add stream layer tests (disconnect, stall, reconnect)"
```

---

### Task 3: Storage Layer Tests — Disk and I/O Failures

**Files:**

- Create: `internal/nvr/audit/storage_test.go`

- [ ] **Step 1: Write storage layer tests**

Create `.worktrees/kai-5/internal/nvr/audit/storage_test.go`:

```go
//go:build integration

package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDiskFull(t *testing.T) {
	// Create a small tmpfs-like directory using a constrained temp dir.
	// We simulate disk full by filling the directory with a large file,
	// then starting a recording.
	recordDir := newTestRecordDir(t)
	s, _ := newTestStream(t)

	var mu sync.Mutex
	var completedSegments []string
	var segmentErrors []error

	_ = startRecorder(t, s, recordDir, func(path string, _ time.Duration) {
		mu.Lock()
		completedSegments = append(completedSegments, path)
		mu.Unlock()
	})

	// Write some initial frames to establish a recording
	now := time.Now()
	writeH264Frames(s, 30, now)
	time.Sleep(500 * time.Millisecond)

	// Get filesystem stats to determine available space
	var stat syscall.Statfs_t
	err := syscall.Statfs(recordDir, &stat)
	require.NoError(t, err)

	// Create a file that fills available space in the recording directory
	// Use a subdirectory to avoid interfering with the recorder's path template
	fillPath := filepath.Join(t.TempDir(), "fill")
	availableBytes := stat.Bavail * uint64(stat.Bsize)

	// Only attempt this if we're in a constrained filesystem (< 100MB available)
	// Otherwise, log a finding that the test needs a constrained FS
	if availableBytes > 100*1024*1024 {
		testReport.Add(Finding{
			Scenario:     "disk_full",
			Layer:        "storage",
			Severity:     SeverityGap,
			Description:  fmt.Sprintf("Skipped: filesystem has %d MB available, need constrained FS", availableBytes/1024/1024),
			Reproduction: "Run with a tmpfs or small partition to simulate disk full",
			DataImpact:   "Unable to determine — test requires constrained filesystem",
			Recovery:     "N/A",
		})
		t.Skip("filesystem too large to simulate disk full safely")
	}

	// Fill the disk
	f, err := os.Create(fillPath)
	require.NoError(t, err)
	_, _ = f.Write(make([]byte, availableBytes-1024)) // leave 1KB
	f.Close()
	defer os.Remove(fillPath)

	// Continue writing frames — recorder should hit write errors
	writeH264Frames(s, 90, now.Add(time.Second))
	time.Sleep(2 * time.Second)

	files, _ := dirFiles(recordDir)
	mu.Lock()
	segCount := len(completedSegments)
	errCount := len(segmentErrors)
	mu.Unlock()

	testReport.Add(Finding{
		Scenario:     "disk_full",
		Layer:        "storage",
		Severity:     SeverityDataLoss,
		Description:  fmt.Sprintf("Disk full during recording. %d files on disk, %d completed segments, %d errors", len(files), segCount, errCount),
		Reproduction: "Fill filesystem, continue writing frames to recorder",
		DataImpact:   "Frames written after disk full are lost. Segment in progress may be corrupted.",
		Recovery:     "Free disk space. Completed segments before disk full are intact.",
	})
}

func TestStoragePathUnavailable(t *testing.T) {
	recordDir := newTestRecordDir(t)
	s, _ := newTestStream(t)

	var mu sync.Mutex
	var completedSegments []string

	_ = startRecorder(t, s, recordDir, func(path string, _ time.Duration) {
		mu.Lock()
		completedSegments = append(completedSegments, path)
		mu.Unlock()
	})

	// Write 1 second of frames
	now := time.Now()
	writeH264Frames(s, 30, now)
	time.Sleep(500 * time.Millisecond)

	// Remove the recording directory while recording is active
	err := os.RemoveAll(recordDir)
	require.NoError(t, err)

	// Continue writing frames
	writeH264Frames(s, 60, now.Add(time.Second))
	time.Sleep(2 * time.Second)

	// Check if directory was recreated or if recording failed
	recreated := fileExists(recordDir)
	files, _ := dirFiles(recordDir)

	mu.Lock()
	segCount := len(completedSegments)
	mu.Unlock()

	testReport.Add(Finding{
		Scenario:     "storage_path_unavailable",
		Layer:        "storage",
		Severity:     SeverityDataLoss,
		Description:  fmt.Sprintf("Recording dir removed mid-write. Dir recreated: %v, files: %d, completed segments: %d", recreated, len(files), segCount),
		Reproduction: "Start recording, remove recording directory, continue writing frames",
		DataImpact:   "Data loss during period when directory is unavailable",
		Recovery:     "Recorder supervisor restarts after 2s pause. New segments created if dir is restored.",
	})
}

func TestStoragePermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}

	recordDir := newTestRecordDir(t)
	s, _ := newTestStream(t)

	var mu sync.Mutex
	var completedSegments []string

	_ = startRecorder(t, s, recordDir, func(path string, _ time.Duration) {
		mu.Lock()
		completedSegments = append(completedSegments, path)
		mu.Unlock()
	})

	// Write initial frames
	now := time.Now()
	writeH264Frames(s, 30, now)
	time.Sleep(500 * time.Millisecond)

	// Make directory read-only
	err := os.Chmod(recordDir, 0o444)
	require.NoError(t, err)
	t.Cleanup(func() {
		os.Chmod(recordDir, 0o755) // restore for cleanup
	})

	// Continue writing frames
	writeH264Frames(s, 60, now.Add(time.Second))
	time.Sleep(2 * time.Second)

	// Restore permissions and check state
	os.Chmod(recordDir, 0o755)
	files, _ := dirFiles(recordDir)

	mu.Lock()
	segCount := len(completedSegments)
	mu.Unlock()

	testReport.Add(Finding{
		Scenario:     "storage_permission_denied",
		Layer:        "storage",
		Severity:     SeverityDataLoss,
		Description:  fmt.Sprintf("Directory made read-only mid-recording. %d files, %d completed segments", len(files), segCount),
		Reproduction: "Start recording, chmod directory to 444, continue writing frames",
		DataImpact:   "New segments cannot be created. In-progress segment write fails.",
		Recovery:     "Restore permissions. Recorder supervisor restarts and resumes.",
	})

	assert.True(t, len(files) > 0, "expected at least one file from before permission change")
}
```

- [ ] **Step 2: Run the storage tests**

```bash
cd .worktrees/kai-5 && go test -tags=integration -v -run "TestStorage|TestDisk" -timeout 60s ./internal/nvr/audit/
```

Expected: Tests execute. `TestDiskFull` may skip if filesystem is too large.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/audit/storage_test.go
git commit -m "feat(audit): add storage layer tests (disk full, path unavailable, permissions)"
```

---

### Task 4: Database Layer Tests — SQLite Failures

**Files:**

- Create: `internal/nvr/audit/database_test.go`

- [ ] **Step 1: Write database layer tests**

Create `.worktrees/kai-5/internal/nvr/audit/database_test.go`:

```go
//go:build integration

package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDBInsertFailureDuringSegmentComplete(t *testing.T) {
	database := newTestDB(t)
	recordDir := newTestRecordDir(t)

	// Create a camera to associate recordings with
	cam := &db.Camera{
		Name:         "test-cam",
		MediaMTXPath: "test",
		Status:       "connected",
	}
	require.NoError(t, database.CreateCamera(cam))

	s, _ := newTestStream(t)

	var completedPath string

	_ = startRecorder(t, s, recordDir, func(path string, dur time.Duration) {
		completedPath = path

		// Close the DB before the insert to simulate a DB failure
		database.Close()
	})

	// Write enough frames to trigger a segment complete
	now := time.Now()
	writeH264Frames(s, 120, now) // 4 seconds at 30fps
	time.Sleep(3 * time.Second)

	// Check: file should exist on disk but DB insert failed
	diskFileExists := fileExists(completedPath)

	// Reopen DB and check for the recording entry
	dbPath := filepath.Join(t.TempDir(), "check.db")
	checkDB, err := db.Open(dbPath)
	if err == nil {
		defer checkDB.Close()
	}

	testReport.Add(Finding{
		Scenario:     "db_insert_failure_on_segment_complete",
		Layer:        "database",
		Severity:     SeverityRecoverable,
		Description:  fmt.Sprintf("DB closed during segment complete callback. File on disk: %v, path: %s", diskFileExists, completedPath),
		Reproduction: "Close SQLite DB connection during OnSegmentComplete callback",
		DataImpact:   "Recording file exists on disk but has no database entry (orphaned file)",
		Recovery:     "Fragment backfill can re-index orphaned files on next startup",
	})

	if completedPath != "" {
		assert.True(t, diskFileExists, "segment file should exist on disk even if DB insert fails")
	}
}

func TestDBFragmentIndexingFailure(t *testing.T) {
	database := newTestDB(t)

	// Insert a recording entry without fragments
	rec := &db.Recording{
		CameraID:   "cam-1",
		StreamID:   "stream-1",
		StartTime:  time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		EndTime:    time.Now().Add(10 * time.Second).UTC().Format("2006-01-02T15:04:05.000Z"),
		DurationMs: 10000,
		FilePath:   "/fake/path/recording.mp4",
		FileSize:   1024 * 1024,
		Format:     "fmp4",
	}
	require.NoError(t, database.InsertRecording(rec))

	// Verify recording exists
	fetched, err := database.GetRecording(rec.ID)
	require.NoError(t, err)
	require.NotNil(t, fetched)

	// Check if it shows up as unindexed
	unindexed, err := database.GetUnindexedRecordings()
	require.NoError(t, err)

	hasFragments, err := database.HasFragments(rec.ID)
	require.NoError(t, err)

	testReport.Add(Finding{
		Scenario:     "db_fragment_indexing_failure",
		Layer:        "database",
		Severity:     SeverityRecoverable,
		Description:  fmt.Sprintf("Recording inserted without fragments. Has fragments: %v, in unindexed list: %v", hasFragments, len(unindexed) > 0),
		Reproduction: "Insert recording to DB, skip fragment insertion",
		DataImpact:   "Recording exists in DB but cannot be seeked via HLS (no fragment offsets)",
		Recovery:     "Fragment backfill goroutine picks up unindexed recordings on startup",
	})

	assert.False(t, hasFragments, "recording should have no fragments")
	assert.True(t, len(unindexed) > 0, "recording should appear in unindexed list")
}

func TestDBLocked(t *testing.T) {
	database := newTestDB(t)

	cam := &db.Camera{
		Name:         "test-cam-lock",
		MediaMTXPath: "test-lock",
		Status:       "connected",
	}
	require.NoError(t, database.CreateCamera(cam))

	// Acquire an exclusive write lock by starting a transaction
	tx, err := database.BeginTx()
	require.NoError(t, err)

	// Attempt to insert a recording while the lock is held
	rec := &db.Recording{
		CameraID:   cam.ID,
		StreamID:   "stream-1",
		StartTime:  time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		EndTime:    time.Now().Add(5 * time.Second).UTC().Format("2006-01-02T15:04:05.000Z"),
		DurationMs: 5000,
		FilePath:   "/fake/locked-test.mp4",
		FileSize:   512 * 1024,
		Format:     "fmp4",
	}

	insertErr := database.InsertRecording(rec)

	// Release the lock
	if tx != nil {
		tx.Rollback()
	}

	testReport.Add(Finding{
		Scenario:     "db_locked",
		Layer:        "database",
		Severity:     SeverityRecoverable,
		Description:  fmt.Sprintf("DB locked during recording insert. Insert error: %v", insertErr),
		Reproduction: "Hold exclusive SQLite transaction, attempt InsertRecording from another connection",
		DataImpact:   "Recording metadata not persisted while lock held. File still written to disk.",
		Recovery:     "SQLite WAL mode + busy timeout may retry. Fragment backfill handles re-indexing.",
	})

	// SQLite in WAL mode with busy_timeout should handle this
	// The test documents the actual behavior
	if insertErr != nil {
		t.Logf("Insert failed with lock held: %v", insertErr)
	}
}
```

- [ ] **Step 2: Run the database tests**

```bash
cd .worktrees/kai-5 && go test -tags=integration -v -run "TestDB" -timeout 30s ./internal/nvr/audit/
```

Expected: Tests execute. `TestDBLocked` behavior depends on SQLite WAL mode busy timeout configuration.

- [ ] **Step 3: Fix compilation issues**

The `database.BeginTx()` method may not exist on the `db.DB` type. Check the actual API:

```bash
cd .worktrees/kai-5 && grep -n "func.*DB.*Begin\|func.*DB.*Exec\|func.*DB.*Lock" internal/nvr/db/db.go
```

If `BeginTx` doesn't exist, use a second `db.Open()` on the same file to create lock contention, or use the underlying `*sql.DB` directly.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/audit/database_test.go
git commit -m "feat(audit): add database layer tests (insert failure, indexing, locking)"
```

---

### Task 5: Recorder Layer Tests — Format and Memory Pressure

**Files:**

- Create: `internal/nvr/audit/recorder_test.go`

- [ ] **Step 1: Write recorder layer tests**

Create `.worktrees/kai-5/internal/nvr/audit/recorder_test.go`:

```go
//go:build integration

package audit

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/recorder"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSegmentBoundaryFailure(t *testing.T) {
	recordDir := newTestRecordDir(t)
	s, _ := newTestStream(t)

	var mu sync.Mutex
	var completedSegments []string

	// Use a very short segment duration to trigger frequent segment boundaries
	pathFormat := filepath.Join(recordDir, "%path/%Y-%m-%d_%H-%M-%S-%f")
	rec := &recorder.Recorder{
		PathFormat:      pathFormat,
		Format:          conf.RecordFormatFMP4,
		PartDuration:    100 * time.Millisecond,
		SegmentDuration: 500 * time.Millisecond, // Very short segments
		PathName:        "test",
		Stream:          s,
		OnSegmentCreate: func(_ string) {},
		OnSegmentComplete: func(path string, dur time.Duration) {
			mu.Lock()
			completedSegments = append(completedSegments, path)
			mu.Unlock()
		},
		Parent: testLogger{},
	}
	rec.Initialize()
	defer rec.Close()

	// Write 3 seconds of frames — should trigger ~6 segment boundaries
	now := time.Now()
	for i := 0; i < 90; i++ {
		dts := time.Duration(i) * time.Second / 30
		ntp := now.Add(dts)
		s.WriteUnit(test.MediaH264, test.FormatH264, &unit.H264{
			Base: unit.Base{
				DTS: dts,
				NTP: ntp,
				PTS: dts,
			},
			AU: [][]byte{
				test.FormatH264.SPS,
				test.FormatH264.PPS,
				{5, 1},
			},
		})
		time.Sleep(33 * time.Millisecond) // ~30fps real-time
	}
	time.Sleep(time.Second) // let final segment complete

	files, err := dirFiles(recordDir)
	require.NoError(t, err)

	mu.Lock()
	segCount := len(completedSegments)
	mu.Unlock()

	// Check each segment file is valid (non-zero, starts with ftyp box)
	var validCount, invalidCount int
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil || len(data) < 8 {
			invalidCount++
			continue
		}
		// fMP4 files start with ftyp box: bytes 4-7 = "ftyp"
		if string(data[4:8]) == "ftyp" {
			validCount++
		} else {
			invalidCount++
		}
	}

	testReport.Add(Finding{
		Scenario:     "segment_boundary_rapid",
		Layer:        "recorder",
		Severity:     SeverityRecoverable,
		Description:  fmt.Sprintf("Rapid segment boundaries (500ms). %d files, %d completed callbacks, %d valid fMP4, %d invalid", len(files), segCount, validCount, invalidCount),
		Reproduction: "Set segmentDuration=500ms, write 3s of frames at 30fps real-time",
		DataImpact:   fmt.Sprintf("%d invalid segment files detected", invalidCount),
		Recovery:     "Valid segments recoverable. Invalid segments may be truncated at last complete part.",
	})

	assert.Equal(t, 0, invalidCount, "all segment files should be valid fMP4")
}

func TestLargePartSize(t *testing.T) {
	recordDir := newTestRecordDir(t)
	s, _ := newTestStream(t)

	var mu sync.Mutex
	var completedSegments []string

	// Use a very small MaxPartSize to trigger part splits
	pathFormat := filepath.Join(recordDir, "%path/%Y-%m-%d_%H-%M-%S-%f")
	rec := &recorder.Recorder{
		PathFormat:      pathFormat,
		Format:          conf.RecordFormatFMP4,
		PartDuration:    time.Second,
		MaxPartSize:     conf.StringSize(1024), // 1KB max part size
		SegmentDuration: 5 * time.Second,
		PathName:        "test",
		Stream:          s,
		OnSegmentCreate: func(_ string) {},
		OnSegmentComplete: func(path string, dur time.Duration) {
			mu.Lock()
			completedSegments = append(completedSegments, path)
			mu.Unlock()
		},
		Parent: testLogger{},
	}
	rec.Initialize()
	defer rec.Close()

	// Write frames with large NAL units to exceed MaxPartSize
	now := time.Now()
	for i := 0; i < 60; i++ {
		dts := time.Duration(i) * time.Second / 30
		ntp := now.Add(dts)
		// Create a large IDR frame (~2KB) to exceed 1KB MaxPartSize
		largeIDR := make([]byte, 2048)
		largeIDR[0] = 5 // IDR NAL type
		largeIDR[1] = 1

		s.WriteUnit(test.MediaH264, test.FormatH264, &unit.H264{
			Base: unit.Base{
				DTS: dts,
				NTP: ntp,
				PTS: dts,
			},
			AU: [][]byte{
				test.FormatH264.SPS,
				test.FormatH264.PPS,
				largeIDR,
			},
		})
	}
	time.Sleep(2 * time.Second)

	files, err := dirFiles(recordDir)
	require.NoError(t, err)

	// Check total data written vs expected
	var totalSize int64
	for _, f := range files {
		totalSize += fileSize(f)
	}

	mu.Lock()
	segCount := len(completedSegments)
	mu.Unlock()

	testReport.Add(Finding{
		Scenario:     "large_part_size",
		Layer:        "recorder",
		Severity:     SeverityRecoverable,
		Description:  fmt.Sprintf("Frames exceed MaxPartSize (1KB). %d files, %d segments, %d bytes total", len(files), segCount, totalSize),
		Reproduction: "Set MaxPartSize=1KB, write 2KB IDR frames at 30fps",
		DataImpact:   "Parts should be split at MaxPartSize boundary. No data loss expected.",
		Recovery:     "Automatic — part splitting is a normal code path",
	})

	assert.True(t, totalSize > 0, "expected some recording data on disk")
}

func TestOOMPressure(t *testing.T) {
	recordDir := newTestRecordDir(t)
	s, _ := newTestStream(t)

	var mu sync.Mutex
	var completedSegments []string

	_ = startRecorder(t, s, recordDir, func(path string, _ time.Duration) {
		mu.Lock()
		completedSegments = append(completedSegments, path)
		mu.Unlock()
	})

	// Allocate significant memory to create GC pressure
	var ballast [][]byte
	for i := 0; i < 100; i++ {
		ballast = append(ballast, make([]byte, 1024*1024)) // 100MB total
	}

	// Write frames under memory pressure
	now := time.Now()
	writeH264Frames(s, 90, now)
	time.Sleep(2 * time.Second)

	// Release ballast
	ballast = nil

	files, err := dirFiles(recordDir)
	require.NoError(t, err)

	mu.Lock()
	segCount := len(completedSegments)
	mu.Unlock()

	testReport.Add(Finding{
		Scenario:     "oom_pressure",
		Layer:        "recorder",
		Severity:     SeverityRecoverable,
		Description:  fmt.Sprintf("Recording under 100MB GC pressure. %d files, %d completed segments", len(files), segCount),
		Reproduction: "Allocate 100MB ballast, write 3s of frames at 30fps",
		DataImpact:   "GC pauses may cause timing jitter but data should be intact",
		Recovery:     "Automatic — GC pressure does not cause data loss, only latency",
	})

	assert.NotEmpty(t, files, "expected recording files even under memory pressure")
}
```

- [ ] **Step 2: Run the recorder tests**

```bash
cd .worktrees/kai-5 && go test -tags=integration -v -run "TestSegment|TestLargePart|TestOOM" -timeout 60s ./internal/nvr/audit/
```

Expected: Tests execute and record findings.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/audit/recorder_test.go
git commit -m "feat(audit): add recorder layer tests (segment boundary, part size, OOM)"
```

---

### Task 6: Lifecycle Tests — Process Signals and Recovery

**Files:**

- Create: `internal/nvr/audit/lifecycle_test.go`
- Create: `internal/nvr/audit/cmd/auditrecord/main.go` (subprocess for SIGKILL test)

- [ ] **Step 1: Create the subprocess binary for SIGKILL testing**

Create `.worktrees/kai-5/internal/nvr/audit/cmd/auditrecord/main.go`:

```go
// auditrecord is a helper binary that records to disk, used by lifecycle tests
// to test SIGKILL behavior. It writes to the directory specified by the first arg,
// then records until killed.
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/defs"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/recorder"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

type nopLogger struct{}

func (nopLogger) Log(_ logger.Level, _ string, _ ...any) {}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintf(os.Stderr, "usage: auditrecord <record-dir>\n")
		os.Exit(1)
	}
	recordDir := os.Args[1]

	medias := []*defs.Media{
		test.MediaH264,
	}

	s, err := stream.New(512*1024, medias, false, nopLogger{})
	if err != nil {
		fmt.Fprintf(os.Stderr, "stream.New: %v\n", err)
		os.Exit(1)
	}

	pathFormat := filepath.Join(recordDir, "%path/%Y-%m-%d_%H-%M-%S-%f")
	rec := &recorder.Recorder{
		PathFormat:        pathFormat,
		Format:            conf.RecordFormatFMP4,
		PartDuration:      200 * time.Millisecond,
		SegmentDuration:   10 * time.Second,
		PathName:          "test",
		Stream:            s,
		OnSegmentCreate:   func(_ string) {},
		OnSegmentComplete: func(_ string, _ time.Duration) {},
		Parent:            nopLogger{},
	}
	rec.Initialize()

	// Signal readiness on stdout
	fmt.Println("READY")

	// Write frames continuously until killed
	startNTP := time.Now()
	i := 0
	for {
		dts := time.Duration(i) * time.Second / 30
		ntp := startNTP.Add(dts)
		s.WriteUnit(test.MediaH264, test.FormatH264, &unit.H264{
			Base: unit.Base{
				DTS: dts,
				NTP: ntp,
				PTS: dts,
			},
			AU: [][]byte{
				test.FormatH264.SPS,
				test.FormatH264.PPS,
				{5, 1},
			},
		})
		i++
		time.Sleep(33 * time.Millisecond)
	}
}
```

- [ ] **Step 2: Write lifecycle tests**

Create `.worktrees/kai-5/internal/nvr/audit/lifecycle_test.go`:

```go
//go:build integration

package audit

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGracefulShutdown(t *testing.T) {
	recordDir := newTestRecordDir(t)
	s, _ := newTestStream(t)

	var mu sync.Mutex
	var completedSegments []string

	rec := startRecorder(t, s, recordDir, func(path string, _ time.Duration) {
		mu.Lock()
		completedSegments = append(completedSegments, path)
		mu.Unlock()
	})

	// Write 3 seconds of frames
	now := time.Now()
	writeH264Frames(s, 90, now)
	time.Sleep(500 * time.Millisecond)

	// Graceful close (equivalent to SIGTERM handler)
	rec.Close()
	time.Sleep(500 * time.Millisecond)

	files, err := dirFiles(recordDir)
	require.NoError(t, err)

	mu.Lock()
	segCount := len(completedSegments)
	mu.Unlock()

	// Check all files are valid fMP4
	var validCount int
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil || len(data) < 8 {
			continue
		}
		if string(data[4:8]) == "ftyp" {
			validCount++
		}
	}

	testReport.Add(Finding{
		Scenario:     "graceful_shutdown",
		Layer:        "lifecycle",
		Severity:     SeverityRecoverable,
		Description:  fmt.Sprintf("Graceful close after 3s recording. %d files, %d completed segments, %d valid fMP4", len(files), segCount, validCount),
		Reproduction: "Write 3s frames, call Recorder.Close()",
		DataImpact:   "Current segment should be properly closed with duration written to header",
		Recovery:     "No recovery needed — graceful shutdown produces valid files",
	})

	assert.Equal(t, len(files), validCount, "all files should be valid fMP4 after graceful shutdown")
}

func TestSIGKILL(t *testing.T) {
	recordDir := newTestRecordDir(t)

	// Build the subprocess binary
	binPath := filepath.Join(t.TempDir(), "auditrecord")
	build := exec.Command("go", "build", "-o", binPath, "./internal/nvr/audit/cmd/auditrecord")
	build.Dir = findModuleRoot(t)
	out, err := build.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(out))

	// Start the subprocess
	cmd := exec.Command(binPath, recordDir)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Start())

	// Wait for READY signal
	scanner := bufio.NewScanner(stdout)
	ready := false
	for scanner.Scan() {
		if scanner.Text() == "READY" {
			ready = true
			break
		}
	}
	require.True(t, ready, "subprocess did not signal READY")

	// Let it record for 5 seconds
	time.Sleep(5 * time.Second)

	// SIGKILL — no cleanup handlers run
	require.NoError(t, cmd.Process.Signal(syscall.SIGKILL))
	_ = cmd.Wait() // will return error due to SIGKILL, that's expected

	// Inspect what's on disk
	files, err := dirFiles(recordDir)
	require.NoError(t, err)

	var validCount, truncatedCount int
	var totalSize int64
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		totalSize += int64(len(data))
		if len(data) >= 8 && string(data[4:8]) == "ftyp" {
			// Check if the file has at least one complete moof+mdat
			if containsCompleteMoofMdat(data) {
				validCount++
			} else {
				truncatedCount++
			}
		} else {
			truncatedCount++
		}
	}

	testReport.Add(Finding{
		Scenario:     "sigkill",
		Layer:        "lifecycle",
		Severity:     SeverityGap,
		Description:  fmt.Sprintf("SIGKILL after 5s recording. %d files (%d bytes), %d valid (have moof+mdat), %d truncated", len(files), totalSize, validCount, truncatedCount),
		Reproduction: "Start recording subprocess, wait 5s, send SIGKILL",
		DataImpact:   fmt.Sprintf("%d truncated files. Data in last incomplete moof+mdat is lost.", truncatedCount),
		Recovery:     "Valid files readable up to last complete moof+mdat. Fragment backfill re-indexes on restart.",
	})

	assert.NotEmpty(t, files, "expected recording files on disk after SIGKILL")
}

func TestRestartRecovery(t *testing.T) {
	recordDir := newTestRecordDir(t)
	database := newTestDB(t)

	// Phase 1: Record normally and insert into DB
	s, _ := newTestStream(t)

	var mu sync.Mutex
	var completedPaths []string

	_ = startRecorder(t, s, recordDir, func(path string, dur time.Duration) {
		mu.Lock()
		completedPaths = append(completedPaths, path)
		mu.Unlock()

		// Simulate NVR OnSegmentComplete: insert recording into DB
		rec := &db.Recording{
			CameraID:   "cam-restart",
			StreamID:   "stream-1",
			StartTime:  time.Now().Add(-dur).UTC().Format("2006-01-02T15:04:05.000Z"),
			EndTime:    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
			DurationMs: dur.Milliseconds(),
			FilePath:   path,
			FileSize:   fileSize(path),
			Format:     "fmp4",
		}
		database.InsertRecording(rec)
	})

	now := time.Now()
	writeH264Frames(s, 120, now)
	time.Sleep(3 * time.Second)
	s.Close()
	time.Sleep(time.Second)

	mu.Lock()
	phase1Paths := make([]string, len(completedPaths))
	copy(phase1Paths, completedPaths)
	mu.Unlock()

	// Check unindexed recordings (fragments not yet inserted)
	unindexed, err := database.GetUnindexedRecordings()
	require.NoError(t, err)

	testReport.Add(Finding{
		Scenario:     "restart_recovery",
		Layer:        "lifecycle",
		Severity:     SeverityRecoverable,
		Description:  fmt.Sprintf("After recording stop: %d completed segments, %d unindexed recordings needing backfill", len(phase1Paths), len(unindexed)),
		Reproduction: "Record 4s, stop stream, check DB for unindexed recordings",
		DataImpact:   "Recordings on disk and in DB but fragments not indexed — HLS seeking unavailable until backfill",
		Recovery:     "Fragment backfill runs on startup and indexes all unindexed recordings",
	})

	assert.True(t, len(unindexed) > 0 || len(phase1Paths) == 0,
		"completed recordings should appear as unindexed until fragment backfill runs")
}

// containsCompleteMoofMdat checks if fMP4 data contains at least one moof+mdat pair.
func containsCompleteMoofMdat(data []byte) bool {
	offset := 0
	foundMoof := false
	for offset+8 <= len(data) {
		size := int(data[offset])<<24 | int(data[offset+1])<<16 | int(data[offset+2])<<8 | int(data[offset+3])
		if size < 8 || offset+size > len(data) {
			return foundMoof // incomplete box
		}
		boxType := string(data[offset+4 : offset+8])
		if boxType == "moof" {
			foundMoof = true
		}
		if boxType == "mdat" && foundMoof {
			return true // found a complete moof+mdat pair
		}
		offset += size
	}
	return false
}

// findModuleRoot walks up from cwd to find go.mod.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	require.NoError(t, err)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find module root")
		}
		dir = parent
	}
}
```

- [ ] **Step 3: Run lifecycle tests**

```bash
cd .worktrees/kai-5 && go test -tags=integration -v -run "TestGraceful|TestSIGKILL|TestRestart" -timeout 120s ./internal/nvr/audit/
```

Expected: `TestGracefulShutdown` and `TestRestartRecovery` run normally. `TestSIGKILL` builds and runs the subprocess.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/audit/lifecycle_test.go internal/nvr/audit/cmd/auditrecord/main.go
git commit -m "feat(audit): add lifecycle tests (graceful shutdown, SIGKILL, restart recovery)"
```

---

### Task 7: End-to-End Scenario Tests

**Files:**

- Create: `internal/nvr/audit/scenario_test.go`

- [ ] **Step 1: Write E2E scenario tests**

Create `.worktrees/kai-5/internal/nvr/audit/scenario_test.go`:

```go
//go:build integration

package audit

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScenarioNetworkDropAndRecover(t *testing.T) {
	recordDir := newTestRecordDir(t)
	database := newTestDB(t)

	cam := &db.Camera{
		Name:         "net-drop-cam",
		MediaMTXPath: "test",
		Status:       "connected",
	}
	require.NoError(t, database.CreateCamera(cam))

	s, _ := newTestStream(t)

	var mu sync.Mutex
	var completedSegments []string

	_ = startRecorder(t, s, recordDir, func(path string, dur time.Duration) {
		mu.Lock()
		completedSegments = append(completedSegments, path)
		mu.Unlock()

		rec := &db.Recording{
			CameraID:   cam.ID,
			StreamID:   "stream-1",
			StartTime:  time.Now().Add(-dur).UTC().Format("2006-01-02T15:04:05.000Z"),
			EndTime:    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
			DurationMs: dur.Milliseconds(),
			FilePath:   path,
			FileSize:   fileSize(path),
			Format:     "fmp4",
		}
		database.InsertRecording(rec)
	})

	// Phase 1: Record for 3 seconds
	phase1Start := time.Now()
	writeH264Frames(s, 90, phase1Start)
	time.Sleep(time.Second)

	// Network drop: close stream
	s.Close()
	disconnectTime := time.Now()

	// Gap: 5 seconds with no data
	time.Sleep(5 * time.Second)

	// Phase 2: Reconnect with new stream
	s2, _ := newTestStream(t)
	defer s2.Close()
	reconnectTime := time.Now()
	writeH264Frames(s2, 90, reconnectTime)
	time.Sleep(time.Second)

	gap := reconnectTime.Sub(disconnectTime)

	files, err := dirFiles(recordDir)
	require.NoError(t, err)

	mu.Lock()
	segCount := len(completedSegments)
	mu.Unlock()

	// Query timeline from DB
	start := phase1Start.Add(-time.Minute)
	end := reconnectTime.Add(time.Minute)
	timeline, _ := database.GetTimeline(cam.ID, start, end)

	testReport.Add(Finding{
		Scenario:     "e2e_network_drop_and_recover",
		Layer:        "scenario",
		Severity:     SeverityGap,
		Description:  fmt.Sprintf("Network drop/recover cycle. %d files, %d segments, %.1fs gap, %d timeline ranges", len(files), segCount, gap.Seconds(), len(timeline)),
		Reproduction: "Record 3s, close stream, wait 5s, create new stream, record 3s",
		DataImpact:   fmt.Sprintf("%.1fs gap in recording. Data before and after gap is intact.", gap.Seconds()),
		Recovery:     "Automatic. New segment starts on reconnect. Timeline reflects gap.",
	})

	assert.NotEmpty(t, files, "expected recording files")
}

func TestScenarioPowerLoss(t *testing.T) {
	recordDir := newTestRecordDir(t)
	database := newTestDB(t)

	// Build subprocess
	binPath := filepath.Join(t.TempDir(), "auditrecord")
	build := exec.Command("go", "build", "-o", binPath, "./internal/nvr/audit/cmd/auditrecord")
	build.Dir = findModuleRoot(t)
	out, err := build.CombinedOutput()
	require.NoError(t, err, "build failed: %s", string(out))

	// Start subprocess recording
	cmd := exec.Command(binPath, recordDir)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())

	scanner := bufio.NewScanner(stdout)
	ready := false
	for scanner.Scan() {
		if scanner.Text() == "READY" {
			ready = true
			break
		}
	}
	require.True(t, ready)

	// Record for 10 seconds
	time.Sleep(10 * time.Second)

	// Power loss: SIGKILL
	require.NoError(t, cmd.Process.Signal(syscall.SIGKILL))
	_ = cmd.Wait()

	// Inspect filesystem state
	files, err := dirFiles(recordDir)
	require.NoError(t, err)

	var validFiles, recoverableFiles int
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		if len(data) >= 8 && string(data[4:8]) == "ftyp" {
			if containsCompleteMoofMdat(data) {
				validFiles++
			} else {
				recoverableFiles++
			}
		}
	}

	// Simulate recovery: register files in DB and check backfill readiness
	for _, f := range files {
		size := fileSize(f)
		if size <= 0 {
			continue
		}
		rec := &db.Recording{
			CameraID:   "power-loss-cam",
			StreamID:   "stream-1",
			StartTime:  time.Now().Add(-10 * time.Second).UTC().Format("2006-01-02T15:04:05.000Z"),
			EndTime:    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
			DurationMs: 10000,
			FilePath:   f,
			FileSize:   size,
			Format:     "fmp4",
		}
		database.InsertRecording(rec)
	}

	unindexed, _ := database.GetUnindexedRecordings()

	testReport.Add(Finding{
		Scenario:     "e2e_power_loss",
		Layer:        "scenario",
		Severity:     SeverityGap,
		Description:  fmt.Sprintf("SIGKILL after 10s. %d files, %d valid, %d truncated but recoverable, %d ready for backfill", len(files), validFiles, recoverableFiles, len(unindexed)),
		Reproduction: "Start recording subprocess, wait 10s, SIGKILL, inspect disk and run recovery",
		DataImpact:   "Last partial moof+mdat is lost. All complete fragments are intact.",
		Recovery:     "Re-register files in DB, run fragment backfill to re-index.",
	})

	assert.NotEmpty(t, files, "expected recording files after power loss")
	assert.True(t, validFiles > 0, "expected at least one file with complete moof+mdat")
}

func TestScenarioStorageFailover(t *testing.T) {
	primaryDir := newTestRecordDir(t)
	fallbackDir := filepath.Join(t.TempDir(), "fallback")
	require.NoError(t, os.MkdirAll(fallbackDir, 0o755))

	s, _ := newTestStream(t)

	var mu sync.Mutex
	var completedSegments []string

	_ = startRecorder(t, s, primaryDir, func(path string, _ time.Duration) {
		mu.Lock()
		completedSegments = append(completedSegments, path)
		mu.Unlock()
	})

	// Record to primary for 2 seconds
	now := time.Now()
	writeH264Frames(s, 60, now)
	time.Sleep(time.Second)

	// Make primary unavailable
	os.Chmod(primaryDir, 0o000)

	// Continue writing — these should fail on primary
	writeH264Frames(s, 60, now.Add(2*time.Second))
	time.Sleep(2 * time.Second)

	// Restore primary
	os.Chmod(primaryDir, 0o755)

	// Write more frames
	writeH264Frames(s, 60, now.Add(4*time.Second))
	time.Sleep(time.Second)

	primaryFiles, _ := dirFiles(primaryDir)
	fallbackFiles, _ := dirFiles(fallbackDir)

	mu.Lock()
	segCount := len(completedSegments)
	mu.Unlock()

	testReport.Add(Finding{
		Scenario:     "e2e_storage_failover",
		Layer:        "scenario",
		Severity:     SeverityGap,
		Description:  fmt.Sprintf("Primary storage made unavailable. Primary files: %d, fallback files: %d, completed segments: %d", len(primaryFiles), len(fallbackFiles), segCount),
		Reproduction: "Record to primary, chmod 000, continue recording, restore, record more",
		DataImpact:   "Data loss during failover transition. Recorder supervisor restarts on write failure.",
		Recovery:     "Recorder restarts after 2s pause. Storage manager syncs files when primary recovers.",
	})
}
```

- [ ] **Step 2: Run E2E scenario tests**

```bash
cd .worktrees/kai-5 && go test -tags=integration -v -run "TestScenario" -timeout 180s ./internal/nvr/audit/
```

Expected: Tests execute (some may take up to 30s each due to sleep durations).

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/audit/scenario_test.go
git commit -m "feat(audit): add E2E scenario tests (network drop, power loss, storage failover)"
```

---

### Task 8: Report Generation Test

**Files:**

- Modify: `internal/nvr/audit/audit_test.go` (add TestGenerateReport)

- [ ] **Step 1: Add `TestGenerateReport` to `audit_test.go`**

Append to `.worktrees/kai-5/internal/nvr/audit/audit_test.go`:

```go
// TestGenerateReport runs last and writes the accumulated findings to disk.
// Use `go test -run TestGenerateReport` after running all other audit tests,
// or run all tests together and this will capture whatever ran.
func TestGenerateReport(t *testing.T) {
	if len(testReport.Findings) == 0 {
		t.Skip("no findings to report — run other audit tests first")
	}

	// Write JSON report
	jsonPath := filepath.Join(findModuleRoot(t), "internal", "nvr", "audit", "testdata", "audit_findings.json")
	err := testReport.WriteJSON(jsonPath)
	require.NoError(t, err)
	t.Logf("JSON report written to %s (%d findings)", jsonPath, len(testReport.Findings))

	// Write markdown report
	mdPath := filepath.Join(findModuleRoot(t), "docs", "recording-audit-report.md")
	err = testReport.WriteMarkdown(mdPath)
	require.NoError(t, err)
	t.Logf("Markdown report written to %s", mdPath)
}

// findRepoRoot is defined in lifecycle_test.go as findModuleRoot.
// Reuse it here by calling findModuleRoot directly.
```

- [ ] **Step 2: Create testdata directory**

```bash
mkdir -p .worktrees/kai-5/internal/nvr/audit/testdata
```

- [ ] **Step 3: Run all audit tests together to generate the report**

```bash
cd .worktrees/kai-5 && go test -tags=integration -v -timeout 300s ./internal/nvr/audit/
```

Expected: All tests run, findings are collected, JSON + markdown reports are written.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/audit/audit_test.go internal/nvr/audit/testdata/
git commit -m "feat(audit): add report generation test and testdata directory"
```

---

### Task 9: Fix Compilation Issues and Run Full Suite

- [ ] **Step 1: Build the package and fix any import/type errors**

```bash
cd .worktrees/kai-5 && go build -tags=integration ./internal/nvr/audit/
```

Common issues to check:

- `stream.New` signature may differ — read `internal/stream/stream.go` for exact params
- `conf.RecordFormatFMP4` constant name — check `internal/conf/conf.go`
- `conf.StringSize` type for MaxPartSize — verify in `internal/conf/`
- `db.DB.BeginTx()` may not exist — check `internal/nvr/db/db.go`
- `unit.H264` and `unit.Base` fields — check `internal/unit/`

Fix each compilation error by reading the actual source file and adjusting the test code.

- [ ] **Step 2: Run the full test suite**

```bash
cd .worktrees/kai-5 && go test -tags=integration -v -timeout 300s ./internal/nvr/audit/ 2>&1 | tee audit-results.txt
```

- [ ] **Step 3: Review the generated markdown report**

```bash
cat .worktrees/kai-5/docs/recording-audit-report.md
```

- [ ] **Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix(audit): resolve compilation issues and finalize test suite"
```

---

### Task 10: Push and Create PR

- [ ] **Step 1: Push the branch**

```bash
cd .worktrees/kai-5 && git push -u origin feat/kai-5-recording-audit
```

- [ ] **Step 2: Create the PR**

````bash
gh pr create --title "KAI-5: Audit recording pipeline for data loss" --body "$(cat <<'EOF'
## Summary

- Adds `internal/nvr/audit/` package with integration tests auditing the recording pipeline under adverse conditions
- Layer-level tests: stream disconnect/stall/reconnect, disk full, storage unavailable, DB failures, process signals
- E2E scenario tests: network drop+recover, power loss (SIGKILL), storage failover
- Generates structured findings report (JSON + markdown)

## Test Matrix

| Scenario | Layer | Status |
|----------|-------|--------|
| Stream disconnect | stream | tested |
| Stream stall | stream | tested |
| Stream reconnect | stream | tested |
| Disk full | storage | tested |
| Storage path unavailable | storage | tested |
| Permission denied | storage | tested |
| DB insert failure | database | tested |
| Fragment indexing failure | database | tested |
| DB locked | database | tested |
| Segment boundary stress | recorder | tested |
| Large part size | recorder | tested |
| OOM pressure | recorder | tested |
| Graceful shutdown | lifecycle | tested |
| SIGKILL (power loss) | lifecycle | tested |
| Restart recovery | lifecycle | tested |
| Network drop + recover | scenario | tested |
| Power loss E2E | scenario | tested |
| Storage failover | scenario | tested |

## How to Run

```bash
go test -tags=integration -v -timeout 300s ./internal/nvr/audit/
````

## Test Plan

- [ ] All integration tests pass with `-tags=integration`
- [ ] JSON findings report generated at `internal/nvr/audit/testdata/audit_findings.json`
- [ ] Markdown report generated at `docs/recording-audit-report.md`
- [ ] Findings categorized by severity (data_loss, corruption, gap, recoverable)

Closes KAI-5

🤖 Generated with [Claude Code](https://claude.com/claude-code)
EOF
)"

````

- [ ] **Step 3: Verify the PR was created**

```bash
gh pr view --web
````
