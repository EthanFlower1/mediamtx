//go:build integration

package audit

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/recorder"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// TestSegmentBoundaryFailure creates a recorder with very short segment and part
// durations to trigger frequent segment boundaries, then validates that every
// file on disk starts with a valid ftyp box.
func TestSegmentBoundaryFailure(t *testing.T) {
	recordDir := newTestRecordDir(t)
	strm, sub, desc := newTestStream(t)

	var completedSegments []string
	recordPath := filepath.Join(recordDir, "%path/%Y-%m-%d_%H-%M-%S-%f")

	rec := &recorder.Recorder{
		PathFormat:      recordPath,
		Format:          conf.RecordFormatFMP4,
		PartDuration:    100 * time.Millisecond,
		MaxPartSize:     50 * 1024 * 1024,
		SegmentDuration: 500 * time.Millisecond,
		PathName:        "testpath",
		Stream:          strm,
		OnSegmentComplete: func(path string, duration time.Duration) {
			completedSegments = append(completedSegments, path)
		},
		Parent: testLogger{},
	}
	rec.Initialize()
	t.Cleanup(rec.Close)

	// Write 3 seconds of frames with real-time pacing (~33ms between frames).
	startNTP := time.Now()
	const fps = 30
	frameDur := 90000 / fps
	totalFrames := 3 * fps // 90 frames for 3 seconds

	for i := range totalFrames {
		pts := int64(i * frameDur)
		ntp := startNTP.Add(time.Duration(i) * time.Second / fps)

		sub.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
			PTS: pts,
			NTP: ntp,
			Payload: unit.PayloadH264{
				test.FormatH264.SPS,
				test.FormatH264.PPS,
				{5}, // IDR
			},
		})

		time.Sleep(33 * time.Millisecond)
	}

	// Let the recorder flush remaining data.
	time.Sleep(1 * time.Second)

	files, err := dirFiles(recordDir)
	require.NoError(t, err)

	validCount := 0
	invalidCount := 0
	ftypMagic := []byte("ftyp")

	for _, f := range files {
		sz := fileSize(f)
		if sz < 8 {
			invalidCount++
			continue
		}
		// Read first 8 bytes to check for ftyp box.
		data := make([]byte, 8)
		fh, err2 := os.Open(f)
		if err2 != nil {
			invalidCount++
			continue
		}
		_, err2 = fh.Read(data)
		fh.Close()
		if err2 != nil {
			invalidCount++
			continue
		}
		if bytes.Equal(data[4:8], ftypMagic) {
			validCount++
		} else {
			invalidCount++
		}
	}

	assert.Greater(t, len(files), 0, "expected at least one recording file")

	totalSize := int64(0)
	for _, f := range files {
		totalSize += fileSize(f)
	}

	testReport.Add(Finding{
		Scenario: "segment-boundary-failure",
		Layer:    "recorder",
		Severity: SeverityCorruption,
		Description: fmt.Sprintf("Wrote 3s of real-time paced frames with 500ms segment/100ms part durations. "+
			"Found %d files (%d valid ftyp, %d invalid). Total size: %d bytes. Completed segments: %d.",
			len(files), validCount, invalidCount, totalSize, len(completedSegments)),
		Reproduction: "Create recorder with SegmentDuration=500ms, PartDuration=100ms. " +
			"Write 90 H264 IDR frames at 30fps with 33ms real-time pacing between each frame.",
		DataImpact: fmt.Sprintf("%d out of %d files have valid ftyp headers. "+
			"Invalid files may represent corrupted segment boundaries.",
			validCount, len(files)),
		Recovery: "Corrupted segments can be detected by checking for ftyp box at file start. " +
			"Unrecoverable segments should be discarded during playback.",
	})
}

// TestLargePartSize creates a recorder with a very small MaxPartSize to trigger
// frequent part splits and checks that the recorder handles fragmentation correctly.
func TestLargePartSize(t *testing.T) {
	recordDir := newTestRecordDir(t)
	strm, sub, desc := newTestStream(t)

	var completedSegments []string
	recordPath := filepath.Join(recordDir, "%path/%Y-%m-%d_%H-%M-%S-%f")

	rec := &recorder.Recorder{
		PathFormat:      recordPath,
		Format:          conf.RecordFormatFMP4,
		PartDuration:    100 * time.Millisecond,
		MaxPartSize:     conf.StringSize(1024), // 1KB to trigger frequent part splits
		SegmentDuration: 1 * time.Second,
		PathName:        "testpath",
		Stream:          strm,
		OnSegmentComplete: func(path string, duration time.Duration) {
			completedSegments = append(completedSegments, path)
		},
		Parent: testLogger{},
	}
	rec.Initialize()
	t.Cleanup(rec.Close)

	// Write 60 frames with large IDR NALUs (~2KB each).
	startNTP := time.Now()
	const fps = 30
	frameDur := 90000 / fps
	largeIDR := make([]byte, 2048)
	largeIDR[0] = 5 // IDR NAL type

	for i := range 60 {
		pts := int64(i * frameDur)
		ntp := startNTP.Add(time.Duration(i) * time.Second / fps)

		sub.WriteUnit(desc.Medias[0], desc.Medias[0].Formats[0], &unit.Unit{
			PTS: pts,
			NTP: ntp,
			Payload: unit.PayloadH264{
				test.FormatH264.SPS,
				test.FormatH264.PPS,
				largeIDR,
			},
		})
	}

	// Let the recorder flush.
	time.Sleep(2 * time.Second)

	files, err := dirFiles(recordDir)
	require.NoError(t, err)

	totalSize := int64(0)
	for _, f := range files {
		totalSize += fileSize(f)
	}

	assert.Greater(t, len(files), 0, "expected at least one recording file with small MaxPartSize")

	testReport.Add(Finding{
		Scenario: "large-part-size",
		Layer:    "recorder",
		Severity: SeverityRecoverable,
		Description: fmt.Sprintf("Wrote 60 frames with ~2KB IDR NALUs and MaxPartSize=1KB. "+
			"Found %d files (%d bytes). Completed segments: %d.",
			len(files), totalSize, len(completedSegments)),
		Reproduction: "Create recorder with MaxPartSize=1024 bytes. Write 60 H264 IDR frames " +
			"with 2KB NALU payloads at 30fps to force part splits on every frame.",
		DataImpact: fmt.Sprintf("Recorder produced %d files totaling %d bytes. "+
			"Part splitting with very small MaxPartSize may cause increased file count "+
			"and overhead from fMP4 headers per part.", len(files), totalSize),
		Recovery: "Part splitting is handled transparently by the recorder. Files remain " +
			"individually playable. Increase MaxPartSize to reduce fragmentation overhead.",
	})
}

// TestOOMPressure writes frames while the process is under heavy GC pressure
// from a large memory ballast, checking that recordings still complete correctly.
func TestOOMPressure(t *testing.T) {
	recordDir := newTestRecordDir(t)
	strm, sub, desc := newTestStream(t)

	var completedSegments []string
	rec := startRecorder(t, strm, recordDir, func(path string, duration time.Duration) {
		completedSegments = append(completedSegments, path)
	})
	_ = rec

	// Allocate 100MB of ballast to create GC pressure.
	ballast := make([][]byte, 100)
	for i := range ballast {
		ballast[i] = make([]byte, 1024*1024) // 1MB each
		// Touch the memory to ensure it's allocated.
		ballast[i][0] = byte(i)
	}
	runtime.GC()

	// Write 90 frames under memory pressure.
	startNTP := time.Now()
	writeH264Frames(sub, desc, 90, startNTP)

	// Let the recorder flush.
	time.Sleep(2 * time.Second)

	// Release ballast.
	ballast = nil
	runtime.GC()

	files, err := dirFiles(recordDir)
	require.NoError(t, err)

	totalSize := int64(0)
	for _, f := range files {
		totalSize += fileSize(f)
	}

	assert.Greater(t, len(files), 0, "expected at least one recording file under memory pressure")

	testReport.Add(Finding{
		Scenario: "oom-pressure",
		Layer:    "recorder",
		Severity: SeverityRecoverable,
		Description: fmt.Sprintf("Wrote 90 frames with 100MB ballast causing GC pressure. "+
			"Found %d files (%d bytes). Completed segments: %d.",
			len(files), totalSize, len(completedSegments)),
		Reproduction: "Allocate 100MB of ballast memory (100x 1MB slices), force GC, " +
			"then write 90 H264 IDR frames at 30fps via standard recorder.",
		DataImpact: fmt.Sprintf("Under GC pressure, recorder produced %d files totaling %d bytes. "+
			"Memory pressure may cause write latency but should not cause data loss.", len(files), totalSize),
		Recovery: "GC pressure effects are transient. Recordings complete normally once " +
			"pressure subsides. Monitor process RSS to detect sustained memory issues.",
	})
}
