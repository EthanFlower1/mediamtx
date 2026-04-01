//go:build integration

package audit

import (
	"fmt"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDiskFull starts a recording and attempts to fill the filesystem.
// On most dev machines there is ample space, so this test skips when >100MB
// is available and records a finding explaining the constraint.
func TestDiskFull(t *testing.T) {
	recordDir := newTestRecordDir(t)
	strm, sub, desc := newTestStream(t)

	var completedSegments []string
	rec := startRecorder(t, strm, recordDir, func(path string, duration time.Duration) {
		completedSegments = append(completedSegments, path)
	})
	_ = rec

	// Write 1s of frames (30 frames at 30fps).
	startNTP := time.Now()
	writeH264Frames(sub, desc, 30, startNTP)

	// Let the recorder flush.
	time.Sleep(500 * time.Millisecond)

	// Check available space on the filesystem hosting recordDir.
	var stat syscall.Statfs_t
	err := syscall.Statfs(recordDir, &stat)
	require.NoError(t, err)

	availableBytes := stat.Bavail * uint64(stat.Bsize)
	const maxAllowed = 100 * 1024 * 1024 // 100 MB

	if availableBytes > maxAllowed {
		testReport.Add(Finding{
			Scenario: "disk-full",
			Layer:    "storage",
			Severity: SeverityDataLoss,
			Description: fmt.Sprintf("Skipped: filesystem has %d MB available (need <100 MB). "+
				"Run inside a constrained tmpfs or disk image to exercise disk-full path.",
				availableBytes/(1024*1024)),
			Reproduction: "Mount a small tmpfs (e.g., 50 MB) and set recordDir to a path on it, " +
				"then write frames until the disk fills.",
			DataImpact: "Unknown — test could not be exercised on this filesystem.",
			Recovery:   "Requires manual investigation on a constrained filesystem.",
		})
		t.Skipf("filesystem has %d MB available; need <100 MB to exercise disk-full path", availableBytes/(1024*1024))
	}

	// Disk is constrained — fill it with a large file.
	fillPath := recordDir + "/fill.bin"
	fillFile, err := os.Create(fillPath)
	require.NoError(t, err)
	defer os.Remove(fillPath)

	// Write zeros in 1MB chunks until we get an error.
	chunk := make([]byte, 1024*1024)
	for {
		_, werr := fillFile.Write(chunk)
		if werr != nil {
			break
		}
	}
	fillFile.Close()

	// Continue writing frames while disk is full.
	writeH264Frames(sub, desc, 90, startNTP.Add(1*time.Second))

	// Let recorder attempt to flush.
	time.Sleep(2 * time.Second)

	// Remove the fill file to allow inspection.
	os.Remove(fillPath)

	files, err := dirFiles(recordDir)
	require.NoError(t, err)

	totalSize := int64(0)
	for _, f := range files {
		totalSize += fileSize(f)
	}

	testReport.Add(Finding{
		Scenario: "disk-full",
		Layer:    "storage",
		Severity: SeverityDataLoss,
		Description: fmt.Sprintf("Wrote 1s (30 frames), filled disk, then wrote 3s more (90 frames). "+
			"Found %d files (%d bytes) on disk after clearing fill file.",
			len(files), totalSize),
		Reproduction: "Start recording on a constrained filesystem (<100 MB free), write 30 frames, " +
			"fill remaining space with a large file, then write 90 more frames.",
		DataImpact: fmt.Sprintf("Segments completed before fill: %d. Files on disk after: %d (%d bytes). "+
			"Frames written during full-disk period are likely lost.",
			len(completedSegments), len(files), totalSize),
		Recovery: "Recorder does not alert on write failures. Manual monitoring of disk space is required. " +
			"Recording may resume automatically once space is freed.",
	})
}

// TestStoragePathUnavailable removes the recording directory mid-recording and
// verifies how the recorder handles the missing path.
func TestStoragePathUnavailable(t *testing.T) {
	recordDir := newTestRecordDir(t)
	strm, sub, desc := newTestStream(t)

	var completedSegments []string
	rec := startRecorder(t, strm, recordDir, func(path string, duration time.Duration) {
		completedSegments = append(completedSegments, path)
	})
	_ = rec

	// Write 1s of frames.
	startNTP := time.Now()
	writeH264Frames(sub, desc, 30, startNTP)

	// Let the recorder flush.
	time.Sleep(500 * time.Millisecond)

	preRemoveFiles, err := dirFiles(recordDir)
	require.NoError(t, err)
	preRemoveCount := len(preRemoveFiles)

	// Remove the entire recording directory.
	err = os.RemoveAll(recordDir)
	require.NoError(t, err)

	// Continue writing frames while directory is gone.
	writeH264Frames(sub, desc, 60, startNTP.Add(1*time.Second))

	// Let recorder attempt to write.
	time.Sleep(2 * time.Second)

	// Check if the directory was recreated by the recorder.
	dirRecreated := false
	if _, serr := os.Stat(recordDir); serr == nil {
		dirRecreated = true
	}

	var postFiles []string
	var postTotalSize int64
	if dirRecreated {
		postFiles, err = dirFiles(recordDir)
		require.NoError(t, err)
		for _, f := range postFiles {
			postTotalSize += fileSize(f)
		}
	}

	assert.Greater(t, preRemoveCount, 0, "expected at least one file before directory removal")

	testReport.Add(Finding{
		Scenario: "storage-path-unavailable",
		Layer:    "storage",
		Severity: SeverityDataLoss,
		Description: fmt.Sprintf("Wrote 1s (30 frames), removed recording directory, then wrote 2s more (60 frames). "+
			"Pre-remove files: %d. Directory recreated: %v. Post-recovery files: %d (%d bytes).",
			preRemoveCount, dirRecreated, len(postFiles), postTotalSize),
		Reproduction: "Start recording, write 30 frames at 30fps, sleep 500ms, " +
			"os.RemoveAll the recording directory, write 60 more frames, sleep 2s.",
		DataImpact: fmt.Sprintf("All %d pre-existing files were destroyed by RemoveAll. "+
			"Directory recreated: %v. Files recovered after removal: %d. "+
			"Data written while path was unavailable is lost.",
			preRemoveCount, dirRecreated, len(postFiles)),
		Recovery: "Recorder may or may not recreate the directory automatically. " +
			"External monitoring should detect missing recording directories and alert operators.",
	})
}

// TestStoragePermissionDenied makes the recording directory inaccessible
// mid-recording and verifies recovery after permissions are restored.
func TestStoragePermissionDenied(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user to exercise permission denial")
	}

	recordDir := newTestRecordDir(t)
	strm, sub, desc := newTestStream(t)

	var completedSegments []string
	rec := startRecorder(t, strm, recordDir, func(path string, duration time.Duration) {
		completedSegments = append(completedSegments, path)
	})
	_ = rec

	// Write 1s of frames.
	startNTP := time.Now()
	writeH264Frames(sub, desc, 30, startNTP)

	// Let the recorder flush.
	time.Sleep(500 * time.Millisecond)

	preChmodFiles, err := dirFiles(recordDir)
	require.NoError(t, err)
	preChmodCount := len(preChmodFiles)

	// Make directory fully inaccessible.
	err = os.Chmod(recordDir, 0o000)
	require.NoError(t, err)

	// Ensure we restore permissions even if the test fails.
	t.Cleanup(func() {
		os.Chmod(recordDir, 0o755)
	})

	// Continue writing frames while directory is inaccessible.
	writeH264Frames(sub, desc, 60, startNTP.Add(1*time.Second))

	// Let recorder attempt to write.
	time.Sleep(2 * time.Second)

	// Restore permissions.
	err = os.Chmod(recordDir, 0o755)
	require.NoError(t, err)

	// Check what survived.
	postFiles, err := dirFiles(recordDir)
	require.NoError(t, err)

	totalSize := int64(0)
	for _, f := range postFiles {
		totalSize += fileSize(f)
	}

	assert.Greater(t, preChmodCount, 0, "expected at least one file before permission change")

	testReport.Add(Finding{
		Scenario: "storage-permission-denied",
		Layer:    "storage",
		Severity: SeverityDataLoss,
		Description: fmt.Sprintf("Wrote 1s (30 frames), chmod 0o000 on recording dir, wrote 2s more (60 frames), "+
			"restored to 0o755. Pre-chmod files: %d. Post-restore files: %d (%d bytes).",
			preChmodCount, len(postFiles), totalSize),
		Reproduction: "Start recording, write 30 frames at 30fps, sleep 500ms, " +
			"os.Chmod(recordDir, 0o000), write 60 more frames, sleep 2s, restore to 0o755.",
		DataImpact: fmt.Sprintf("Pre-chmod data (%d files) may or may not survive depending on open file handles. "+
			"Post-restore: %d files (%d bytes). Frames written during denial period are likely lost.",
			preChmodCount, len(postFiles), totalSize),
		Recovery: "Recorder does not detect or alert on permission errors. " +
			"Recording may resume after permissions are restored if the recorder retries directory creation. " +
			"External monitoring of filesystem permissions is recommended.",
	})
}
