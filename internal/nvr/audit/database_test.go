//go:build integration

package audit

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"

	_ "modernc.org/sqlite"
)

// TestDBInsertFailureDuringSegmentComplete verifies that when the database is
// closed before the segment-complete callback fires, the recording file exists
// on disk but the DB insert fails, creating an orphaned file.
func TestDBInsertFailureDuringSegmentComplete(t *testing.T) {
	database := newTestDB(t)
	recordDir := newTestRecordDir(t)
	strm, sub, desc := newTestStream(t)

	// Create a camera so foreign-key constraints are satisfied.
	cam := &db.Camera{Name: "test-cam"}
	err := database.CreateCamera(cam)
	require.NoError(t, err)

	var mu sync.Mutex
	var insertErr error
	var segmentPath string

	rec := startRecorder(t, strm, recordDir, func(path string, duration time.Duration) {
		mu.Lock()
		defer mu.Unlock()
		segmentPath = path

		// Attempt to insert a recording into the (already closed) database.
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
		insertErr = database.InsertRecording(recording)
	})
	_ = rec

	// Close the database before frames trigger segment completion.
	database.Close()

	// Write enough frames to trigger at least one segment complete (120 frames
	// = 4s at 30fps, with 1s segment duration).
	startNTP := time.Now()
	writeH264Frames(sub, desc, 120, startNTP)

	// Wait for recorder to flush segments and fire callbacks.
	time.Sleep(3 * time.Second)

	mu.Lock()
	capturedPath := segmentPath
	capturedErr := insertErr
	mu.Unlock()

	// The file should exist on disk even though DB insert failed.
	files, err := dirFiles(recordDir)
	require.NoError(t, err)
	hasFiles := len(files) > 0
	assert.True(t, hasFiles, "expected recording files on disk after writing 120 frames")

	// The DB insert should have failed because we closed the connection.
	if capturedPath != "" {
		assert.Error(t, capturedErr, "expected DB insert to fail on closed database")
		assert.True(t, fileExists(capturedPath), "segment file should exist on disk despite DB failure")
	}

	testReport.Add(Finding{
		Scenario: "db-insert-failure-during-segment-complete",
		Layer:    "database",
		Severity: SeverityRecoverable,
		Description: fmt.Sprintf("Closed DB before segment-complete callback. "+
			"Files on disk: %d. Callback fired: %v. Insert error: %v.",
			len(files), capturedPath != "", capturedErr),
		Reproduction: "Create DB and camera, start recorder with OnSegmentComplete that inserts recording, " +
			"close DB, write 120 H264 frames (4s at 30fps with 1s segments), wait for callback.",
		DataImpact: "Recording file exists on disk but has no corresponding database row. " +
			"The file is orphaned and invisible to queries, timeline, and playback.",
		Recovery: "A reconciliation pass can scan the recording directory for files not present " +
			"in the database and re-insert them. The fMP4 file is self-contained and playable.",
	})
}

// TestDBFragmentIndexingFailure verifies that a recording inserted without
// fragment metadata is flagged as unindexed and that HLS seeking is unavailable
// until fragments are backfilled.
func TestDBFragmentIndexingFailure(t *testing.T) {
	database := newTestDB(t)

	// Create a camera for FK constraint.
	cam := &db.Camera{Name: "test-cam-indexing"}
	err := database.CreateCamera(cam)
	require.NoError(t, err)

	// Insert a recording manually without any fragments.
	rec := &db.Recording{
		CameraID:   cam.ID,
		StreamID:   "main",
		StartTime:  time.Now().Add(-5 * time.Second).UTC().Format("2006-01-02T15:04:05.000Z"),
		EndTime:    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		DurationMs: 5000,
		FilePath:   "/tmp/test-recording.mp4",
		FileSize:   1024,
		Format:     "fmp4",
	}
	err = database.InsertRecording(rec)
	require.NoError(t, err)
	require.NotZero(t, rec.ID, "InsertRecording should populate ID")

	// Verify the recording exists.
	fetched, err := database.GetRecording(rec.ID)
	require.NoError(t, err)
	assert.Equal(t, rec.ID, fetched.ID)

	// HasFragments should return false since we never inserted any.
	hasFrags, err := database.HasFragments(rec.ID)
	require.NoError(t, err)
	assert.False(t, hasFrags, "recording should have no fragments")

	// GetUnindexedRecordings should include this recording.
	unindexed, err := database.GetUnindexedRecordings()
	require.NoError(t, err)
	assert.NotEmpty(t, unindexed, "expected at least one unindexed recording")

	found := false
	for _, u := range unindexed {
		if u.ID == rec.ID {
			found = true
			break
		}
	}
	assert.True(t, found, "our recording should appear in unindexed list")

	testReport.Add(Finding{
		Scenario: "db-fragment-indexing-failure",
		Layer:    "database",
		Severity: SeverityRecoverable,
		Description: fmt.Sprintf("Inserted recording ID=%d without fragments. "+
			"HasFragments=%v. Appears in GetUnindexedRecordings=%v.",
			rec.ID, hasFrags, found),
		Reproduction: "Insert a Recording via InsertRecording without calling InsertFragments. " +
			"Check HasFragments (false) and GetUnindexedRecordings (includes recording).",
		DataImpact: "HLS seeking is unavailable for unindexed recordings. The file plays from " +
			"the beginning but cannot seek to arbitrary positions because fragment byte " +
			"offsets are unknown. Timeline display works (start/end times exist) but " +
			"scrubbing is broken.",
		Recovery: "A background indexer can parse fMP4 files listed in GetUnindexedRecordings, " +
			"extract fragment offsets, and call InsertFragments to backfill. " +
			"No data loss — the recording file is complete.",
	})
}

// TestDBLocked verifies behavior when a second connection holds a write
// transaction, testing SQLite WAL mode concurrency characteristics.
func TestDBLocked(t *testing.T) {
	// Create a DB in a temp directory (don't use newTestDB so we control the path).
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "lock-test.db")

	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	// Create a camera for FK constraints.
	cam := &db.Camera{Name: "test-cam-lock"}
	err = database.CreateCamera(cam)
	require.NoError(t, err)

	// Open a raw second connection to the same database file to create lock
	// contention. Use the same driver and WAL pragmas.
	rawDB, err := sql.Open("sqlite", dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { rawDB.Close() })

	_, err = rawDB.Exec("PRAGMA journal_mode = WAL")
	require.NoError(t, err)

	// Begin a write transaction on the raw connection and hold it.
	ctx := context.Background()
	tx, err := rawDB.BeginTx(ctx, nil)
	require.NoError(t, err)

	// Execute a write in the raw transaction to acquire a write lock.
	_, err = tx.Exec("INSERT INTO schema_migrations (version) VALUES (99999)")
	require.NoError(t, err)

	// Now attempt an insert via the main DB connection while the lock is held.
	rec := &db.Recording{
		CameraID:   cam.ID,
		StreamID:   "main",
		StartTime:  time.Now().Add(-2 * time.Second).UTC().Format("2006-01-02T15:04:05.000Z"),
		EndTime:    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
		DurationMs: 2000,
		FilePath:   "/tmp/test-locked.mp4",
		FileSize:   512,
		Format:     "fmp4",
	}

	insertErr := database.InsertRecording(rec)

	// Roll back the raw transaction to release the lock.
	tx.Rollback()

	// In WAL mode, readers do not block writers and writers do not block readers.
	// However, only one writer can proceed at a time. The behavior depends on the
	// busy timeout — SQLite may retry or return SQLITE_BUSY.
	// Document whatever actually happens.
	walAllowsConcurrent := insertErr == nil

	// If the insert failed under lock, verify it succeeds after lock release.
	var retryErr error
	if insertErr != nil {
		rec2 := &db.Recording{
			CameraID:   cam.ID,
			StreamID:   "main",
			StartTime:  time.Now().Add(-2 * time.Second).UTC().Format("2006-01-02T15:04:05.000Z"),
			EndTime:    time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
			DurationMs: 2000,
			FilePath:   "/tmp/test-after-unlock.mp4",
			FileSize:   512,
			Format:     "fmp4",
		}
		retryErr = database.InsertRecording(rec2)
		assert.NoError(t, retryErr, "insert should succeed after lock is released")
	}

	testReport.Add(Finding{
		Scenario: "db-locked",
		Layer:    "database",
		Severity: SeverityRecoverable,
		Description: fmt.Sprintf("Held write transaction on second connection. "+
			"Insert on primary connection: err=%v. WAL concurrent write succeeded=%v. "+
			"Retry after unlock: err=%v.",
			insertErr, walAllowsConcurrent, retryErr),
		Reproduction: "Open two connections to the same SQLite file with WAL mode. " +
			"Begin a write transaction on connection 2 (INSERT into schema_migrations). " +
			"Attempt InsertRecording on connection 1 while lock is held.",
		DataImpact: "If SQLITE_BUSY is returned, the segment-complete callback fails silently " +
			"and the recording becomes an orphaned file (same as db-insert-failure). " +
			"If WAL allows concurrent writes, no data loss occurs.",
		Recovery: "Configure a busy timeout (PRAGMA busy_timeout) so SQLite retries automatically " +
			"instead of returning SQLITE_BUSY immediately. A reconciliation scan can recover " +
			"any orphaned files.",
	})
}
