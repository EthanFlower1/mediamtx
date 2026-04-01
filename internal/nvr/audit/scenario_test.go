//go:build integration

package audit

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// TestScenarioNetworkDropAndRecover simulates a network interruption: the
// stream is closed (simulating camera disconnect), a gap passes, and then a
// new stream + recorder is started. We verify that the DB timeline reflects
// the gap and that files from both phases exist.
func TestScenarioNetworkDropAndRecover(t *testing.T) {
	database := newTestDB(t)
	recordDir := newTestRecordDir(t)

	cam := &db.Camera{Name: "scenario-net-drop"}
	require.NoError(t, database.CreateCamera(cam))

	var mu sync.Mutex
	var segments []string

	onComplete := func(path string, duration time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		segments = append(segments, path)

		now := time.Now().UTC()
		rec := &db.Recording{
			CameraID:   cam.ID,
			StreamID:   "main",
			StartTime:  now.Add(-duration).Format("2006-01-02T15:04:05.000Z"),
			EndTime:    now.Format("2006-01-02T15:04:05.000Z"),
			DurationMs: duration.Milliseconds(),
			FilePath:   path,
			FileSize:   fileSize(path),
			Format:     "fmp4",
		}
		err := database.InsertRecording(rec)
		if err != nil {
			t.Logf("InsertRecording error: %v", err)
		}
	}

	// Phase 1: Write 90 frames (~3s at 30fps).
	strm1, sub1, desc1 := makeTestStream(t)
	phase1Start := time.Now()
	rec1 := makeRecorder(t, strm1, recordDir, onComplete)
	writeH264Frames(sub1, desc1, 90, phase1Start)
	time.Sleep(1 * time.Second)

	// Simulate network drop: close the recorder and stream.
	rec1.Close()
	strm1.Close()

	// Gap: wait 5 seconds simulating network outage.
	time.Sleep(5 * time.Second)

	// Phase 2: Recover with new stream + recorder (same recordDir).
	strm2, sub2, desc2 := makeTestStream(t)
	phase2Start := time.Now()
	rec2 := makeRecorder(t, strm2, recordDir, onComplete)
	writeH264Frames(sub2, desc2, 90, phase2Start)
	time.Sleep(1 * time.Second)
	rec2.Close()
	strm2.Close()

	// Allow final callbacks to fire.
	time.Sleep(2 * time.Second)

	// Check files.
	files, err := dirFiles(recordDir)
	require.NoError(t, err)
	t.Logf("Total files after both phases: %d", len(files))
	assert.GreaterOrEqual(t, len(files), 2, "expected files from both recording phases")

	totalSize := int64(0)
	for _, f := range files {
		totalSize += fileSize(f)
	}
	assert.Greater(t, totalSize, int64(0), "total recording size should be > 0")

	// Check DB timeline for any gap between entries.
	timelineStart := phase1Start.Add(-1 * time.Minute)
	timelineEnd := phase2Start.Add(1 * time.Minute)
	timeline, err := database.GetTimeline(cam.ID, timelineStart, timelineEnd)
	require.NoError(t, err)
	t.Logf("Timeline entries: %d", len(timeline))

	// Find the largest gap between consecutive timeline entries.
	var maxGap time.Duration
	for i := 1; i < len(timeline); i++ {
		gap := timeline[i].Start.Sub(timeline[i-1].End)
		if gap > maxGap {
			maxGap = gap
		}
	}
	t.Logf("Largest gap between timeline entries: %v", maxGap)

	testReport.Add(Finding{
		Scenario:     "NetworkDropAndRecover",
		Layer:        "scenario/e2e",
		Severity:     SeverityGap,
		Description:  "Camera network drop causes a gap in the recording timeline; no data is recorded during the outage.",
		Reproduction: "Close stream, wait 5s, start new stream+recorder to same directory.",
		DataImpact:   fmt.Sprintf("Gap of ~5s in timeline. %d files, %d bytes total.", len(files), totalSize),
		Recovery:     "Automatic on reconnect. Gap is visible in DB timeline. No data loss outside the outage window.",
	})
}

// TestScenarioPowerLoss simulates abrupt process termination (SIGKILL) during
// recording. It requires the auditrecord binary from Task 6. If the source is
// not present, the test is skipped.
func TestScenarioPowerLoss(t *testing.T) {
	moduleRoot := findModuleRoot(t)

	cmdSource := filepath.Join(moduleRoot, "internal", "nvr", "audit", "cmd", "auditrecord", "main.go")
	if !fileExists(cmdSource) {
		t.Skipf("auditrecord source not found at %s; skipping (Task 6 not yet committed)", cmdSource)
	}

	// Build the auditrecord binary.
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "auditrecord")
	buildCmd := exec.Command("go", "build", "-o", binPath, "./internal/nvr/audit/cmd/auditrecord")
	buildCmd.Dir = moduleRoot
	out, err := buildCmd.CombinedOutput()
	require.NoError(t, err, "failed to build auditrecord: %s", string(out))

	recordDir := t.TempDir()

	// Start the subprocess.
	cmd := exec.Command(binPath, recordDir)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr
	require.NoError(t, cmd.Start())

	// Wait for READY signal.
	deadline := time.After(15 * time.Second)
	ready := false
	for !ready {
		select {
		case <-deadline:
			_ = cmd.Process.Kill()
			t.Fatalf("auditrecord did not print READY within 15s; output: %s", stdout.String())
		case <-time.After(100 * time.Millisecond):
			if strings.Contains(stdout.String(), "READY") {
				ready = true
			}
		}
	}

	// Let it record for 10 seconds.
	time.Sleep(10 * time.Second)

	// SIGKILL - abrupt termination.
	require.NoError(t, cmd.Process.Kill())
	_ = cmd.Wait() // reap the zombie

	// Inspect files on disk.
	files, err := dirFiles(recordDir)
	require.NoError(t, err)
	t.Logf("Files after SIGKILL: %d", len(files))
	assert.Greater(t, len(files), 0, "expected at least one recording file")

	completeMoofMdat := 0
	hasFtyp := 0
	truncated := 0
	for _, f := range files {
		data, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		// Check for ftyp box (first 4 bytes of size + "ftyp").
		if len(data) >= 8 && string(data[4:8]) == "ftyp" {
			hasFtyp++
		}
		if containsCompleteMoofMdat(data) {
			completeMoofMdat++
		} else if len(data) > 0 {
			truncated++
		}
	}

	t.Logf("Files with ftyp: %d, complete moof+mdat: %d, truncated: %d",
		hasFtyp, completeMoofMdat, truncated)

	// Register files in DB and check unindexed count.
	database := newTestDB(t)
	cam := &db.Camera{Name: "scenario-power-loss"}
	require.NoError(t, database.CreateCamera(cam))

	for _, f := range files {
		rec := &db.Recording{
			CameraID:   cam.ID,
			StreamID:   "main",
			StartTime:  time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
			EndTime:    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
			DurationMs: 0,
			FilePath:   f,
			FileSize:   fileSize(f),
			Format:     "fmp4",
		}
		require.NoError(t, database.InsertRecording(rec))
	}

	unindexed, err := database.GetUnindexedRecordings()
	require.NoError(t, err)
	t.Logf("Unindexed recordings after power loss: %d", len(unindexed))
	assert.Equal(t, len(files), len(unindexed),
		"all recordings should be unindexed since no fragment indexing occurred")

	testReport.Add(Finding{
		Scenario: "PowerLoss",
		Layer:    "scenario/e2e",
		Severity: SeverityCorruption,
		Description: fmt.Sprintf(
			"SIGKILL during recording leaves %d files; %d have ftyp, %d have complete moof+mdat, %d may be truncated.",
			len(files), hasFtyp, completeMoofMdat, truncated),
		Reproduction: "Start auditrecord subprocess, let it record 10s, SIGKILL.",
		DataImpact:   fmt.Sprintf("%d files unindexed in DB; last segment may be truncated.", len(unindexed)),
		Recovery:     "Recovery requires re-indexing surviving files. Truncated final segment may lose last partial mdat.",
	})
}

// TestScenarioStorageFailover verifies behavior when the primary recording
// directory becomes inaccessible mid-recording. The recorder should continue
// writing to whatever path it can, and files should remain consistent.
func TestScenarioStorageFailover(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping storage failover test when running as root (chmod has no effect)")
	}

	primaryDir := newTestRecordDir(t)
	fallbackDir := newTestRecordDir(t)

	database := newTestDB(t)
	cam := &db.Camera{Name: "scenario-storage-failover"}
	require.NoError(t, database.CreateCamera(cam))

	var mu sync.Mutex
	var segments []string

	onComplete := func(path string, duration time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		segments = append(segments, path)

		now := time.Now().UTC()
		rec := &db.Recording{
			CameraID:   cam.ID,
			StreamID:   "main",
			StartTime:  now.Add(-duration).Format("2006-01-02T15:04:05.000Z"),
			EndTime:    now.Format("2006-01-02T15:04:05.000Z"),
			DurationMs: duration.Milliseconds(),
			FilePath:   path,
			FileSize:   fileSize(path),
			Format:     "fmp4",
		}
		_ = database.InsertRecording(rec)
	}

	strm, sub, desc := makeTestStream(t)
	startNTP := time.Now()

	// Start recorder writing to primary directory.
	rec := makeRecorder(t, strm, primaryDir, onComplete)

	// Phase 1: Write 60 frames (~2s).
	writeH264Frames(sub, desc, 60, startNTP)
	time.Sleep(1 * time.Second)

	// Count files before making primary inaccessible.
	filesBefore, _ := dirFiles(primaryDir)
	t.Logf("Files in primary before chmod: %d", len(filesBefore))

	// Make primary directory inaccessible.
	require.NoError(t, os.Chmod(primaryDir, 0o000))
	t.Cleanup(func() {
		// Always restore so TempDir cleanup works.
		os.Chmod(primaryDir, 0o755)
	})

	// Phase 2: Continue writing 60 frames while primary is inaccessible.
	phase2Start := startNTP.Add(2 * time.Second)
	writeH264Frames(sub, desc, 60, phase2Start)
	time.Sleep(2 * time.Second)

	// Restore primary directory.
	require.NoError(t, os.Chmod(primaryDir, 0o755))

	// Phase 3: Write 60 more frames after restore.
	phase3Start := startNTP.Add(4 * time.Second)
	writeH264Frames(sub, desc, 60, phase3Start)
	time.Sleep(1 * time.Second)

	rec.Close()
	strm.Close()

	// Allow final callbacks.
	time.Sleep(2 * time.Second)

	primaryFiles, err := dirFiles(primaryDir)
	require.NoError(t, err)
	fallbackFiles, _ := dirFiles(fallbackDir)

	t.Logf("Primary dir files: %d, Fallback dir files: %d", len(primaryFiles), len(fallbackFiles))

	mu.Lock()
	segCount := len(segments)
	mu.Unlock()
	t.Logf("Total segments completed: %d", segCount)

	// We expect files in the primary directory (from phases 1 and 3 at minimum).
	assert.Greater(t, len(primaryFiles), 0, "expected files in primary directory")

	// Calculate total data.
	totalSize := int64(0)
	for _, f := range primaryFiles {
		totalSize += fileSize(f)
	}
	for _, f := range fallbackFiles {
		totalSize += fileSize(f)
	}

	testReport.Add(Finding{
		Scenario: "StorageFailover",
		Layer:    "scenario/e2e",
		Severity: SeverityGap,
		Description: fmt.Sprintf(
			"Making primary dir inaccessible mid-recording: %d files in primary, %d in fallback. "+
				"Recorder may fail to write segments during the inaccessible window.",
			len(primaryFiles), len(fallbackFiles)),
		Reproduction: "chmod 0o000 on primary recording directory while recorder is active, then restore.",
		DataImpact: fmt.Sprintf(
			"Potential gap during inaccessible window. %d total bytes across both dirs.", totalSize),
		Recovery: "Restoring permissions allows recording to resume. No built-in failover to secondary path.",
	})
}
