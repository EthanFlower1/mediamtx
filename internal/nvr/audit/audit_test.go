//go:build integration

package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/gortsplib/v5/pkg/description"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/conf"
	"github.com/bluenviron/mediamtx/internal/logger"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/recorder"
	"github.com/bluenviron/mediamtx/internal/stream"
	"github.com/bluenviron/mediamtx/internal/test"
	"github.com/bluenviron/mediamtx/internal/unit"
)

// testReport is a package-level report shared across audit tests.
var testReport = NewReport()

// testLogger implements logger.Writer and discards all output.
type testLogger struct{}

func (testLogger) Log(_ logger.Level, _ string, _ ...any) {}

// newTestDB creates a SQLite database in a temporary directory.
// The database is automatically cleaned up when the test ends.
func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	d, err := db.Open(filepath.Join(dir, "audit.db"))
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

// newTestRecordDir creates a temporary directory for recordings.
func newTestRecordDir(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// makeTestStream creates a Stream with H264 video and MPEG-4 audio medias.
// The caller is responsible for calling strm.Close() — no cleanup is registered.
// Use this when you need to control stream close timing (e.g., disconnect tests).
func makeTestStream(t *testing.T) (*stream.Stream, *stream.SubStream, *description.Session) {
	t.Helper()

	desc := &description.Session{
		Medias: []*description.Media{
			test.UniqueMediaH264(),
			test.UniqueMediaMPEG4Audio(),
		},
	}

	strm := &stream.Stream{
		Desc:              desc,
		WriteQueueSize:    512,
		RTPMaxPayloadSize: 1450,
		Parent:            testLogger{},
	}
	err := strm.Initialize()
	require.NoError(t, err)

	sub := &stream.SubStream{
		Stream:        strm,
		UseRTPPackets: false,
	}
	err = sub.Initialize()
	require.NoError(t, err)

	return strm, sub, desc
}

// newTestStream creates a Stream and registers t.Cleanup to close it.
// Do NOT call strm.Close() manually — use makeTestStream instead if you need
// to control stream close timing.
func newTestStream(t *testing.T) (*stream.Stream, *stream.SubStream, *description.Session) {
	t.Helper()
	strm, sub, desc := makeTestStream(t)
	t.Cleanup(strm.Close)
	return strm, sub, desc
}

// writeH264Frames writes n IDR frames at ~30fps intervals into the stream's
// first media (H264). startNTP is the NTP base time for the first frame.
func writeH264Frames(sub *stream.SubStream, desc *description.Session, n int, startNTP time.Time) {
	const fps = 30
	frameDur := 90000 / fps // PTS ticks per frame at 90kHz clock

	for i := range n {
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
	}
}

// makeRecorder creates and initializes a Recorder writing fMP4 segments.
// The caller is responsible for calling rec.Close() — no cleanup is registered.
// Use this when you need to control the Close timing (e.g., graceful shutdown tests).
func makeRecorder(
	t *testing.T,
	strm *stream.Stream,
	recordDir string,
	onSegmentComplete recorder.OnSegmentCompleteFunc,
) *recorder.Recorder {
	t.Helper()

	recordPath := filepath.Join(recordDir, "%path/%Y-%m-%d_%H-%M-%S-%f")

	rec := &recorder.Recorder{
		PathFormat:        recordPath,
		Format:            conf.RecordFormatFMP4,
		PartDuration:      100 * time.Millisecond,
		MaxPartSize:       50 * 1024 * 1024,
		SegmentDuration:   1 * time.Second,
		PathName:          "testpath",
		Stream:            strm,
		OnSegmentComplete: onSegmentComplete,
		Parent:            testLogger{},
	}
	rec.Initialize()

	return rec
}

// startRecorder creates and starts a Recorder writing fMP4 segments.
// It registers t.Cleanup to close the recorder automatically.
// Do NOT call rec.Close() manually — use makeRecorder instead if you need
// to control the shutdown timing.
func startRecorder(
	t *testing.T,
	strm *stream.Stream,
	recordDir string,
	onSegmentComplete recorder.OnSegmentCompleteFunc,
) *recorder.Recorder {
	t.Helper()
	rec := makeRecorder(t, strm, recordDir, onSegmentComplete)
	t.Cleanup(rec.Close)
	return rec
}

// fileExists reports whether the named file exists.
func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// fileSize returns the size of the named file, or -1 if it cannot be read.
func fileSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return -1
	}
	return info.Size()
}

// dirFiles returns the full paths of all regular files under dir, recursively.
func dirFiles(dir string) ([]string, error) {
	var paths []string
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			paths = append(paths, path)
		}
		return nil
	})
	return paths, err
}

// TestGenerateReport runs last (alphabetically after all other tests) and writes
// the accumulated findings to JSON and Markdown files.
func TestGenerateReport(t *testing.T) {
	if len(testReport.Findings) == 0 {
		t.Skip("no findings to report — run other audit tests first")
	}

	root := findModuleRoot(t)

	jsonPath := filepath.Join(root, "internal", "nvr", "audit", "testdata", "audit_findings.json")
	require.NoError(t, os.MkdirAll(filepath.Dir(jsonPath), 0o755))
	err := testReport.WriteJSON(jsonPath)
	require.NoError(t, err)
	t.Logf("JSON report written to %s (%d findings)", jsonPath, len(testReport.Findings))

	mdPath := filepath.Join(root, "docs", "recording-audit-report.md")
	require.NoError(t, os.MkdirAll(filepath.Dir(mdPath), 0o755))
	err = testReport.WriteMarkdown(mdPath)
	require.NoError(t, err)
	t.Logf("Markdown report written to %s", mdPath)
}
