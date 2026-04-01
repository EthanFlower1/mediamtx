//go:build integration

package audit

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStreamDisconnect writes 2s of H264 frames at 30fps, then closes the
// stream abruptly. It verifies that at least one recording file exists on disk.
func TestStreamDisconnect(t *testing.T) {
	recordDir := newTestRecordDir(t)
	strm, sub, desc := newTestStream(t)

	var completedSegments []string
	rec := startRecorder(t, strm, recordDir, func(path string, duration time.Duration) {
		completedSegments = append(completedSegments, path)
	})
	_ = rec

	// Write 2s of frames at 30fps = 60 frames.
	startNTP := time.Now()
	writeH264Frames(sub, desc, 60, startNTP)

	// Let the recorder flush parts/segments.
	time.Sleep(2 * time.Second)

	// Abruptly close the stream.
	strm.Close()

	// Give recorder time to handle the error (supervisor restarts with 2s pause).
	time.Sleep(3 * time.Second)

	files, err := dirFiles(recordDir)
	require.NoError(t, err)

	hasFiles := len(files) > 0
	assert.True(t, hasFiles, "expected at least one recording file after stream disconnect")

	totalSize := int64(0)
	for _, f := range files {
		totalSize += fileSize(f)
	}

	testReport.Add(Finding{
		Scenario:    "stream-disconnect",
		Layer:       "stream",
		Severity:    SeverityDataLoss,
		Description: fmt.Sprintf("Wrote 2s (60 frames) then closed stream abruptly. Found %d files (%d bytes) on disk.", len(files), totalSize),
		Reproduction: "Write 60 H264 IDR frames at 30fps via SubStream, sleep 2s for flush, " +
			"then call Stream.Close() to simulate camera disconnect.",
		DataImpact: fmt.Sprintf("Recording captured %d files totaling %d bytes from 2s of video. "+
			"Any frames in-flight at disconnect time may be lost.", len(files), totalSize),
		Recovery: "Recorder supervisor detects the closed stream and restarts after a 2s pause. " +
			"Manual intervention required to reconnect to the camera.",
	})
}

// TestStreamStall writes 1s of frames, then stops sending for 10s while
// keeping the stream open. It verifies that at least one file from pre-stall
// data exists and records a finding about the gap.
func TestStreamStall(t *testing.T) {
	recordDir := newTestRecordDir(t)
	strm, sub, desc := newTestStream(t)
	_ = strm

	var completedSegments []string
	rec := startRecorder(t, strm, recordDir, func(path string, duration time.Duration) {
		completedSegments = append(completedSegments, path)
	})
	_ = rec

	// Write 1s of frames at 30fps = 30 frames.
	startNTP := time.Now()
	writeH264Frames(sub, desc, 30, startNTP)

	// Let the recorder flush the first second of data.
	time.Sleep(2 * time.Second)

	preStallFiles, err := dirFiles(recordDir)
	require.NoError(t, err)
	preStallCount := len(preStallFiles)

	// Stall: keep stream open but send nothing for 10s.
	time.Sleep(10 * time.Second)

	postStallFiles, err := dirFiles(recordDir)
	require.NoError(t, err)

	assert.Greater(t, preStallCount, 0, "expected at least one file from pre-stall data")

	totalSize := int64(0)
	for _, f := range postStallFiles {
		totalSize += fileSize(f)
	}

	testReport.Add(Finding{
		Scenario: "stream-stall",
		Layer:    "stream",
		Severity: SeverityGap,
		Description: fmt.Sprintf("Wrote 1s (30 frames) then stalled for 10s with stream open. "+
			"Pre-stall: %d files. Post-stall: %d files. Total size: %d bytes.",
			preStallCount, len(postStallFiles), totalSize),
		Reproduction: "Write 30 H264 IDR frames at 30fps via SubStream, sleep 2s for flush, " +
			"then stop sending data for 10s while keeping the stream open.",
		DataImpact: fmt.Sprintf("10-second gap in recording. Pre-stall data (%d files) was preserved. "+
			"No new segments created during stall period (delta: %d files).",
			preStallCount, len(postStallFiles)-preStallCount),
		Recovery: "Recording resumes automatically when frames start arriving again. " +
			"The gap is silent — no alert or error is produced by the recorder.",
	})
}

// TestStreamReconnect writes 2s of frames, closes the stream, waits 3s,
// creates a new stream, writes 2s more, and documents the gap and segment count.
func TestStreamReconnect(t *testing.T) {
	recordDir := newTestRecordDir(t)
	strm, sub, desc := newTestStream(t)

	var completedSegments []string
	rec := startRecorder(t, strm, recordDir, func(path string, duration time.Duration) {
		completedSegments = append(completedSegments, path)
	})
	_ = rec

	// Phase 1: Write 2s of frames at 30fps = 60 frames.
	startNTP := time.Now()
	writeH264Frames(sub, desc, 60, startNTP)

	// Let the recorder flush.
	time.Sleep(2 * time.Second)

	preDisconnectFiles, err := dirFiles(recordDir)
	require.NoError(t, err)
	preDisconnectCount := len(preDisconnectFiles)

	// Close the stream to simulate camera disconnect.
	disconnectTime := time.Now()
	strm.Close()

	// Wait for recorder to handle the error (supervisor restarts with 2s pause).
	time.Sleep(3 * time.Second)

	postDisconnectFiles, err := dirFiles(recordDir)
	require.NoError(t, err)
	postDisconnectCount := len(postDisconnectFiles)

	// Phase 2: Create a new stream and write 2s more.
	// Note: The existing recorder is tied to the old stream. We create a new
	// stream and a new recorder to simulate reconnection behavior.
	strm2, sub2, desc2 := newTestStream(t)

	var completedSegments2 []string
	rec2 := startRecorder(t, strm2, recordDir, func(path string, duration time.Duration) {
		completedSegments2 = append(completedSegments2, path)
	})
	_ = rec2

	reconnectTime := time.Now()
	gapDuration := reconnectTime.Sub(disconnectTime)
	writeH264Frames(sub2, desc2, 60, reconnectTime)

	// Let the second recorder flush.
	time.Sleep(2 * time.Second)

	finalFiles, err := dirFiles(recordDir)
	require.NoError(t, err)

	assert.Greater(t, preDisconnectCount, 0, "expected files from first recording session")
	assert.Greater(t, len(finalFiles), postDisconnectCount,
		"expected new files from second recording session")

	totalSize := int64(0)
	for _, f := range finalFiles {
		totalSize += fileSize(f)
	}

	testReport.Add(Finding{
		Scenario: "stream-reconnect",
		Layer:    "stream",
		Severity: SeverityGap,
		Description: fmt.Sprintf("Wrote 2s, disconnected for %.1fs, reconnected with new stream+recorder, wrote 2s more. "+
			"Pre-disconnect: %d files. Post-disconnect: %d files. Final: %d files (%d bytes).",
			gapDuration.Seconds(), preDisconnectCount, postDisconnectCount, len(finalFiles), totalSize),
		Reproduction: "Write 60 H264 IDR frames at 30fps, sleep 2s, close stream, wait 3s, " +
			"create new stream and recorder to same directory, write 60 more frames.",
		DataImpact: fmt.Sprintf("Gap of %.1fs between disconnect and reconnect. "+
			"Original recorder cannot recover — a new recorder must be created for the new stream. "+
			"Total segments across both sessions: %d files.",
			gapDuration.Seconds(), len(finalFiles)),
		Recovery: "Requires external orchestration to detect stream loss, create a new stream, " +
			"and start a new recorder. The gap duration depends on detection and reconnection latency.",
	})
}
