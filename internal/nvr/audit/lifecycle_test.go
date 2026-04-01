//go:build integration

package audit

import (
	"bufio"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// findModuleRoot walks up from the caller's directory to find the directory
// containing go.mod, which is the module root.
func findModuleRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	require.True(t, ok, "runtime.Caller failed")

	dir := filepath.Dir(file)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		require.NotEqual(t, dir, parent, "go.mod not found")
		dir = parent
	}
}

// containsCompleteMoofMdat walks fMP4 boxes in data and returns true if it
// finds at least one complete moof box followed by a complete mdat box.
func containsCompleteMoofMdat(data []byte) bool {
	foundMoof := false
	offset := 0

	for offset+8 <= len(data) {
		boxSize := int(binary.BigEndian.Uint32(data[offset : offset+4]))
		boxType := string(data[offset+4 : offset+8])

		// A box size of 0 means the box extends to the end of the file.
		if boxSize == 0 {
			boxSize = len(data) - offset
		}
		// A size of 1 means the real size is in the next 8 bytes (64-bit).
		if boxSize == 1 && offset+16 <= len(data) {
			boxSize = int(binary.BigEndian.Uint64(data[offset+8 : offset+16]))
		}
		if boxSize < 8 || offset+boxSize > len(data) {
			// Incomplete box; stop walking.
			return false
		}

		switch boxType {
		case "moof":
			foundMoof = true
		case "mdat":
			if foundMoof {
				return true
			}
		}

		offset += boxSize
	}
	return false
}

// startsWithFtyp returns true if data begins with an ftyp box.
func startsWithFtyp(data []byte) bool {
	if len(data) < 8 {
		return false
	}
	return string(data[4:8]) == "ftyp"
}

// --------------------------------------------------------------------------
// TestGracefulShutdown verifies that calling rec.Close() (graceful shutdown)
// produces valid fMP4 files that start with an ftyp box.
// --------------------------------------------------------------------------
func TestGracefulShutdown(t *testing.T) {
	recordDir := newTestRecordDir(t)
	strm, sub, desc := newTestStream(t)

	var mu sync.Mutex
	var completedSegments []string

	rec := makeRecorder(t, strm, recordDir, func(path string, duration time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		completedSegments = append(completedSegments, path)
	})

	// Write 90 frames (3s at 30fps).
	startNTP := time.Now()
	writeH264Frames(sub, desc, 90, startNTP)

	// Allow recorder time to flush segments.
	time.Sleep(2 * time.Second)

	// Graceful shutdown.
	rec.Close()

	// Inspect files on disk.
	files, err := dirFiles(recordDir)
	require.NoError(t, err)
	require.NotEmpty(t, files, "expected recording files after writing 90 frames")

	allValid := true
	for _, f := range files {
		data, err := os.ReadFile(f)
		require.NoError(t, err)
		if !startsWithFtyp(data) {
			allValid = false
			t.Logf("file %s does not start with ftyp box", f)
		}
	}

	assert.True(t, allValid, "all files should start with a valid ftyp box after graceful shutdown")

	severity := SeverityRecoverable
	desc2 := "Graceful shutdown via rec.Close() produces valid fMP4 files starting with ftyp."
	if !allValid {
		severity = SeverityCorruption
		desc2 = "Graceful shutdown produced files missing ftyp box header."
	}

	testReport.Add(Finding{
		Scenario:     "Graceful Shutdown",
		Layer:        "recorder",
		Severity:     severity,
		Description:  desc2,
		Reproduction: "Start recorder, write 90 frames (3s), call rec.Close()",
		DataImpact:   "None if ftyp present; corrupted container if missing",
		Recovery:     "Files are playable; no recovery needed",
	})
}

// --------------------------------------------------------------------------
// TestSIGKILL builds the auditrecord binary, starts it as a subprocess,
// lets it record for 5 seconds, sends SIGKILL, and inspects the resulting
// files for fMP4 validity.
// --------------------------------------------------------------------------
func TestSIGKILL(t *testing.T) {
	moduleRoot := findModuleRoot(t)

	// Build the subprocess binary.
	binPath := filepath.Join(t.TempDir(), "auditrecord")
	buildCmd := exec.Command("go", "build", "-o", binPath,
		"./internal/nvr/audit/cmd/auditrecord")
	buildCmd.Dir = moduleRoot
	buildOut, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "go build failed: %s", string(buildOut))

	recordDir := t.TempDir()

	// Start the subprocess.
	cmd := exec.Command(binPath, recordDir)
	cmd.Dir = moduleRoot

	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	cmd.Stderr = os.Stderr

	require.NoError(t, cmd.Start())

	// Wait for "READY" signal.
	scanner := bufio.NewScanner(stdout)
	ready := false
	for scanner.Scan() {
		if strings.TrimSpace(scanner.Text()) == "READY" {
			ready = true
			break
		}
	}
	require.True(t, ready, "subprocess never printed READY")

	// Let it record for 5 seconds.
	time.Sleep(5 * time.Second)

	// Send SIGKILL.
	require.NoError(t, cmd.Process.Signal(syscall.SIGKILL))

	// Wait for the process to exit (ignore error since SIGKILL causes non-zero exit).
	_ = cmd.Wait()

	// Inspect files on disk.
	files, err := dirFiles(recordDir)
	require.NoError(t, err)
	require.NotEmpty(t, files, "expected recording files after 5s of recording")

	hasFtyp := false
	hasCompleteMoofMdat := false

	for _, f := range files {
		data, err := os.ReadFile(f)
		require.NoError(t, err)

		if startsWithFtyp(data) {
			hasFtyp = true
		}
		if containsCompleteMoofMdat(data) {
			hasCompleteMoofMdat = true
		}
	}

	// After SIGKILL we expect at least some files to have ftyp (completed
	// segments) and possibly incomplete trailing data.
	severity := SeverityRecoverable
	description := "SIGKILL leaves recording files on disk. "
	if hasFtyp {
		description += "At least one file starts with valid ftyp. "
	} else {
		severity = SeverityDataLoss
		description += "No file starts with ftyp; all data may be unrecoverable. "
	}
	if hasCompleteMoofMdat {
		description += "At least one file has complete moof+mdat pairs (recoverable fragments)."
	} else {
		if severity != SeverityDataLoss {
			severity = SeverityGap
		}
		description += "No complete moof+mdat pairs found; last segment is truncated."
	}

	testReport.Add(Finding{
		Scenario:     "SIGKILL",
		Layer:        "recorder",
		Severity:     severity,
		Description:  description,
		Reproduction: "Build auditrecord, start subprocess, wait 5s, send SIGKILL",
		DataImpact:   "In-flight segment may be truncated or missing mdat",
		Recovery:     "Previously completed segments are intact; truncated segment needs repair or discard",
	})

	t.Logf("SIGKILL result: hasFtyp=%v hasCompleteMoofMdat=%v files=%d",
		hasFtyp, hasCompleteMoofMdat, len(files))
}

// --------------------------------------------------------------------------
// TestRestartRecovery simulates a start-record-stop cycle, inserts segment
// metadata into the database, then checks whether recordings without indexed
// fragments can be identified for backfill.
// --------------------------------------------------------------------------
func TestRestartRecovery(t *testing.T) {
	database := newTestDB(t)
	recordDir := newTestRecordDir(t)
	strm, sub, desc := newTestStream(t)

	// Create a camera so foreign-key constraints are satisfied.
	cam := &db.Camera{Name: "lifecycle-test-cam"}
	require.NoError(t, database.CreateCamera(cam))

	var mu sync.Mutex
	var segments []struct {
		path     string
		duration time.Duration
	}

	rec := makeRecorder(t, strm, recordDir, func(path string, duration time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		segments = append(segments, struct {
			path     string
			duration time.Duration
		}{path, duration})

		// Insert recording into DB (without fragments — simulating no indexing).
		recording := &db.Recording{
			CameraID:   cam.ID,
			StreamID:   "main",
			StartTime:  time.Now().Add(-duration).UTC().Format("2006-01-02T15:04:05.000Z"),
			EndTime:    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
			DurationMs: duration.Milliseconds(),
			FilePath:   path,
			FileSize:   fileSize(path),
			Format:     "fmp4",
		}
		require.NoError(t, database.InsertRecording(recording))
	})

	// Write 120 frames (4s at 30fps) — should produce multiple segments
	// with 1s segment duration.
	startNTP := time.Now()
	writeH264Frames(sub, desc, 120, startNTP)

	// Wait for segments to be flushed.
	time.Sleep(3 * time.Second)

	rec.Close()

	mu.Lock()
	segCount := len(segments)
	mu.Unlock()

	require.Greater(t, segCount, 0, "expected at least one completed segment")

	// Query the DB for recordings that have no fragments indexed.
	// This simulates what a recovery/backfill process would do on restart.
	rows, err := database.Query(`
		SELECT r.id, r.file_path
		FROM recordings r
		LEFT JOIN recording_fragments rf ON rf.recording_id = r.id
		WHERE rf.id IS NULL
	`)
	require.NoError(t, err)
	defer rows.Close()

	var unindexed []struct {
		id   int64
		path string
	}
	for rows.Next() {
		var entry struct {
			id   int64
			path string
		}
		require.NoError(t, rows.Scan(&entry.id, &entry.path))
		unindexed = append(unindexed, entry)
	}
	require.NoError(t, rows.Err())

	// All recordings should be unindexed since we never called InsertFragments.
	assert.Equal(t, segCount, len(unindexed),
		"all recordings should be unindexed (no fragments)")

	// Verify the unindexed files still exist on disk.
	allExist := true
	for _, u := range unindexed {
		if !fileExists(u.path) {
			allExist = false
			t.Logf("unindexed recording file missing: %s", u.path)
		}
	}
	assert.True(t, allExist, "all unindexed recording files should exist on disk")

	severity := SeverityRecoverable
	desc2 := "After restart, recordings without fragments can be identified via LEFT JOIN query. "
	if allExist {
		desc2 += "All unindexed files exist on disk and can be re-indexed."
	} else {
		severity = SeverityGap
		desc2 += "Some unindexed recording files are missing from disk."
	}

	testReport.Add(Finding{
		Scenario:     "Restart Recovery",
		Layer:        "database",
		Severity:     severity,
		Description:  desc2,
		Reproduction: "Start recorder, write 120 frames (4s), close, query DB for recordings without fragments",
		DataImpact:   "Recordings exist on disk but are not fragment-indexed until backfill runs",
		Recovery:     "Backfill process can scan unindexed recordings and call InsertFragments to restore random-access",
	})

	t.Logf("RestartRecovery: %d segments, %d unindexed, allExist=%v",
		segCount, len(unindexed), allExist)
}
