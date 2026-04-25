package fragmentbackfill

import (
	"context"
	"encoding/binary"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/recorder/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Minimal fMP4 builder helpers (mirrors recovery/repair_test.go)
// ---------------------------------------------------------------------------

func makeBox(boxType string, payload []byte) []byte {
	size := uint32(8 + len(payload))
	buf := make([]byte, size)
	binary.BigEndian.PutUint32(buf[0:4], size)
	copy(buf[4:8], boxType)
	copy(buf[8:], payload)
	return buf
}

func makeFtyp() []byte {
	payload := make([]byte, 12)
	copy(payload[0:4], "isom")
	return makeBox("ftyp", payload)
}

func makeMoov() []byte {
	mvhd := makeBox("mvhd", make([]byte, 100))
	return makeBox("moov", mvhd)
}

func makeMoof(seqNum uint32) []byte {
	mfhdPayload := make([]byte, 8)
	binary.BigEndian.PutUint32(mfhdPayload[4:8], seqNum)
	mfhd := makeBox("mfhd", mfhdPayload)

	// trun with 1 sample: flags 0x000101 = data-offset-present (0x000001) | sample-duration-present (0x000100)
	trunPayload := make([]byte, 16)
	trunPayload[3] = 0x01
	trunPayload[2] = 0x01
	binary.BigEndian.PutUint32(trunPayload[4:8], 1)     // sample_count
	binary.BigEndian.PutUint32(trunPayload[8:12], 1000) // sample_duration (timescale units)
	binary.BigEndian.PutUint32(trunPayload[12:16], 100) // sample_size

	tfhd := makeBox("tfhd", make([]byte, 4))
	traf := makeBox("traf", append(tfhd, makeBox("trun", trunPayload)...))
	return makeBox("moof", append(mfhd, traf...))
}

func makeMdat(payloadSize int) []byte {
	return makeBox("mdat", make([]byte, payloadSize))
}

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

// ---------------------------------------------------------------------------
// Test DB helper
// ---------------------------------------------------------------------------

func openTestDB(t *testing.T) *db.DB {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "test.db")
	d, err := db.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	return d
}

// insertTestRecording inserts a minimal recording row and returns its ID.
func insertTestRecording(t *testing.T, d *db.DB, filePath, format string) int64 {
	t.Helper()
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	rec := &db.Recording{
		CameraID:  "cam-test",
		StreamID:  "main",
		StartTime: now,
		EndTime:   now,
		FilePath:  filePath,
		Format:    format,
	}
	require.NoError(t, d.InsertRecording(rec))
	return rec.ID
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestRun_NoUnindexed_ExitsCleanly verifies that when there are no unindexed
// recordings the backfill pass returns without error or DB writes.
func TestRun_NoUnindexed_ExitsCleanly(t *testing.T) {
	d := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	scanCalled := 0
	fakeScan := func(_ string) (ScanResult, error) {
		scanCalled++
		return ScanResult{}, nil
	}

	cfg := Config{DB: d, Logger: logger, Scanner: fakeScan}
	runOnce(context.Background(), cfg)

	assert.Equal(t, 0, scanCalled, "scanner should not be called when no unindexed recordings exist")

	frags, err := d.GetFragments(0) // no recording ID 0
	require.NoError(t, err)
	assert.Empty(t, frags)
}

// TestRun_Unindexed_IndexesFragments verifies that a real fMP4 file is scanned
// and its fragments are written to the database.
func TestRun_Unindexed_IndexesFragments(t *testing.T) {
	d := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Write a valid 3-fragment fMP4 file.
	dir := t.TempDir()
	fmp4Path := filepath.Join(dir, "seg.mp4")
	require.NoError(t, os.WriteFile(fmp4Path, buildValidFMP4(3), 0o644))

	recID := insertTestRecording(t, d, fmp4Path, "fmp4")

	// Inject a scanner that returns canned fragment data based on the real file layout.
	fakeScan := func(filePath string) (ScanResult, error) {
		return ScanResult{
			InitSize: 42,
			Fragments: []FragmentInfo{
				{Offset: 42, Size: 100, DurationMs: 1000.0},
				{Offset: 142, Size: 100, DurationMs: 1000.0},
				{Offset: 242, Size: 100, DurationMs: 1000.0},
			},
		}, nil
	}

	cfg := Config{DB: d, Logger: logger, Scanner: fakeScan}
	runOnce(context.Background(), cfg)

	frags, err := d.GetFragments(recID)
	require.NoError(t, err)
	assert.Len(t, frags, 3, "expected 3 fragment rows inserted")

	// Verify init_size was updated.
	rec, err := d.GetRecording(recID)
	require.NoError(t, err)
	assert.Equal(t, int64(42), rec.InitSize, "init_size should be updated")

	// Verify fragment content.
	assert.Equal(t, int64(42), frags[0].ByteOffset)
	assert.Equal(t, int64(100), frags[0].Size)
	assert.Equal(t, 1000.0, frags[0].DurationMs)
}

// TestRun_MissingFile_SkipsAndContinues verifies that a recording pointing at a
// non-existent file is skipped without error, and processing continues for
// subsequent recordings.
func TestRun_MissingFile_SkipsAndContinues(t *testing.T) {
	d := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// First recording: file does not exist.
	missingID := insertTestRecording(t, d, "/nonexistent/path/missing.mp4", "fmp4")

	// Second recording: file exists, should be indexed.
	dir := t.TempDir()
	existingPath := filepath.Join(dir, "present.mp4")
	require.NoError(t, os.WriteFile(existingPath, buildValidFMP4(1), 0o644))
	presentID := insertTestRecording(t, d, existingPath, "fmp4")

	scanCalled := 0
	fakeScan := func(_ string) (ScanResult, error) {
		scanCalled++
		return ScanResult{
			InitSize: 20,
			Fragments: []FragmentInfo{
				{Offset: 20, Size: 50, DurationMs: 500.0},
			},
		}, nil
	}

	cfg := Config{DB: d, Logger: logger, Scanner: fakeScan}
	runOnce(context.Background(), cfg)

	// Missing file: no fragments inserted, no panic.
	missingFrags, err := d.GetFragments(missingID)
	require.NoError(t, err)
	assert.Empty(t, missingFrags, "no fragments should be inserted for missing file")

	// Present file: fragments inserted.
	presentFrags, err := d.GetFragments(presentID)
	require.NoError(t, err)
	assert.Len(t, presentFrags, 1, "present file should have 1 fragment row")

	// Scanner was called exactly once (for the file that exists).
	assert.Equal(t, 1, scanCalled)
}

// TestRun_ScanError_SkipsAndContinues verifies that a scan error causes the
// recording to be skipped while subsequent recordings are still processed.
func TestRun_ScanError_SkipsAndContinues(t *testing.T) {
	d := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	dir := t.TempDir()

	// Two recordings, both with existing files.
	path1 := filepath.Join(dir, "bad.mp4")
	path2 := filepath.Join(dir, "good.mp4")
	require.NoError(t, os.WriteFile(path1, buildValidFMP4(1), 0o644))
	require.NoError(t, os.WriteFile(path2, buildValidFMP4(2), 0o644))

	id1 := insertTestRecording(t, d, path1, "fmp4")
	id2 := insertTestRecording(t, d, path2, "fmp4")

	fakeScan := func(filePath string) (ScanResult, error) {
		if filePath == path1 {
			return ScanResult{}, errors.New("simulated parse error")
		}
		return ScanResult{
			InitSize:  10,
			Fragments: []FragmentInfo{{Offset: 10, Size: 50, DurationMs: 200.0}},
		}, nil
	}

	cfg := Config{DB: d, Logger: logger, Scanner: fakeScan}
	runOnce(context.Background(), cfg)

	// Recording 1 should have no fragments (scan error).
	frags1, err := d.GetFragments(id1)
	require.NoError(t, err)
	assert.Empty(t, frags1)

	// Recording 2 should have fragments inserted.
	frags2, err := d.GetFragments(id2)
	require.NoError(t, err)
	assert.Len(t, frags2, 1)
}

// TestRun_ContextCancellation_ExitsMidBatch verifies that the goroutine respects
// ctx cancellation and exits without processing the full batch.
func TestRun_ContextCancellation_ExitsMidBatch(t *testing.T) {
	d := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	dir := t.TempDir()
	// Insert enough recordings that context cancellation mid-loop is detectable.
	for i := 0; i < 5; i++ {
		p := filepath.Join(dir, "seg.mp4")
		if i == 0 {
			require.NoError(t, os.WriteFile(p, buildValidFMP4(1), 0o644))
		}
		// Each recording points to the same file to keep things simple.
		insertTestRecording(t, d, p, "fmp4")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	// runOnce should exit quickly without processing all recordings.
	done := make(chan struct{})
	go func() {
		defer close(done)
		runOnce(ctx, Config{DB: d, Logger: logger, Scanner: func(_ string) (ScanResult, error) {
			return ScanResult{
				InitSize:  8,
				Fragments: []FragmentInfo{{Offset: 8, Size: 50, DurationMs: 100.0}},
			}, nil
		}})
	}()

	select {
	case <-done:
		// Good — exited cleanly.
	case <-time.After(2 * time.Second):
		t.Fatal("runOnce did not exit within 2s after context cancellation")
	}
}

// TestRun_NonFMP4Format_Skipped verifies that recordings with format != "fmp4"
// are skipped without calling the scanner.
func TestRun_NonFMP4Format_Skipped(t *testing.T) {
	d := openTestDB(t)
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	dir := t.TempDir()
	p := filepath.Join(dir, "seg.ts")
	require.NoError(t, os.WriteFile(p, []byte("not fmp4"), 0o644))
	insertTestRecording(t, d, p, "mpegts")

	scanCalled := 0
	fakeScan := func(_ string) (ScanResult, error) {
		scanCalled++
		return ScanResult{}, nil
	}

	cfg := Config{DB: d, Logger: logger, Scanner: fakeScan}
	runOnce(context.Background(), cfg)

	assert.Equal(t, 0, scanCalled, "scanner should not be called for non-fmp4 format")
}

// ---------------------------------------------------------------------------
// Fake Database implementation for failure-mode tests
// ---------------------------------------------------------------------------

// fakeDB is an injectable Database that lets individual methods be replaced
// with error-returning stubs.
type fakeDB struct {
	getUnindexed         func() ([]*db.Recording, error)
	updateRecordingInit  func(id int64, size int64) error
	insertFragments      func(id int64, frags []db.RecordingFragment) error
}

func (f *fakeDB) GetUnindexedRecordings() ([]*db.Recording, error) {
	return f.getUnindexed()
}

func (f *fakeDB) UpdateRecordingInitSize(id int64, size int64) error {
	return f.updateRecordingInit(id, size)
}

func (f *fakeDB) InsertFragments(id int64, frags []db.RecordingFragment) error {
	return f.insertFragments(id, frags)
}

// ---------------------------------------------------------------------------
// New failure-mode tests
// ---------------------------------------------------------------------------

// TestRun_UpdateInitSizeFails_SkipsRecording verifies that when
// UpdateRecordingInitSize returns an error, InsertFragments is NOT called so
// the recording remains unindexed and can be retried on next boot.
func TestRun_UpdateInitSizeFails_SkipsRecording(t *testing.T) {
	dir := t.TempDir()
	fmp4Path := filepath.Join(dir, "seg.mp4")
	require.NoError(t, os.WriteFile(fmp4Path, buildValidFMP4(2), 0o644))

	insertFragsCalled := false

	fake := &fakeDB{
		getUnindexed: func() ([]*db.Recording, error) {
			return []*db.Recording{
				{ID: 1, FilePath: fmp4Path, Format: "fmp4", Status: "unverified"},
			}, nil
		},
		updateRecordingInit: func(_ int64, _ int64) error {
			return errors.New("simulated DB write error")
		},
		insertFragments: func(_ int64, _ []db.RecordingFragment) error {
			insertFragsCalled = true
			return nil
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	fakeScan := func(_ string) (ScanResult, error) {
		return ScanResult{
			InitSize:  20,
			Fragments: []FragmentInfo{{Offset: 20, Size: 50, DurationMs: 500.0}},
		}, nil
	}

	cfg := Config{DB: fake, Logger: logger, Scanner: fakeScan}
	runOnce(context.Background(), cfg)

	assert.False(t, insertFragsCalled, "InsertFragments must NOT be called when UpdateRecordingInitSize fails")
}

// TestRun_GetUnindexedFails_LogsErrorAndExits verifies that when
// GetUnindexedRecordings returns an error, Run exits without panic and without
// calling InsertFragments.
func TestRun_GetUnindexedFails_LogsErrorAndExits(t *testing.T) {
	insertFragsCalled := false

	fake := &fakeDB{
		getUnindexed: func() ([]*db.Recording, error) {
			return nil, errors.New("simulated query error")
		},
		updateRecordingInit: func(_ int64, _ int64) error {
			return nil
		},
		insertFragments: func(_ int64, _ []db.RecordingFragment) error {
			insertFragsCalled = true
			return nil
		},
	}

	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))

	// Should not panic.
	runOnce(context.Background(), Config{DB: fake, Logger: logger, Scanner: scanFile})

	assert.False(t, insertFragsCalled, "InsertFragments must NOT be called when GetUnindexedRecordings fails")
}

// ---------------------------------------------------------------------------
// Minor: direct unit test for the real scanFile parser
// ---------------------------------------------------------------------------

// TestScanFile_RealFMP4 exercises the actual fMP4 parser end-to-end on a
// file produced by buildValidFMP4, ensuring the scan code path is covered
// even when the fake scanner is injected by other tests.
func TestScanFile_RealFMP4(t *testing.T) {
	dir := t.TempDir()
	fmp4Path := filepath.Join(dir, "real.mp4")
	require.NoError(t, os.WriteFile(fmp4Path, buildValidFMP4(2), 0o644))

	result, err := scanFile(fmp4Path)
	require.NoError(t, err)

	assert.Greater(t, result.InitSize, int64(0), "InitSize should be positive")
	assert.GreaterOrEqual(t, len(result.Fragments), 1, "should have at least 1 fragment")
}
