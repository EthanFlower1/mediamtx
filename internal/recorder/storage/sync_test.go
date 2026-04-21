package storage

import (
	"os"
	"path/filepath"
	"testing"

	db "github.com/bluenviron/mediamtx/internal/recorder/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T) *db.DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func TestSyncWorker_ProcessSync(t *testing.T) {
	database := newTestDB(t)
	localDir := t.TempDir()
	targetDir := t.TempDir()

	cam := &db.Camera{Name: "test", RTSPURL: "rtsp://x", StoragePath: targetDir}
	require.NoError(t, database.CreateCamera(cam))

	localFile := filepath.Join(localDir, "test.mp4")
	require.NoError(t, os.WriteFile(localFile, []byte("fake mp4 data"), 0o644))

	rec := &db.Recording{
		CameraID:  cam.ID,
		StartTime: "2026-03-25T10:00:00.000Z",
		EndTime:   "2026-03-25T11:00:00.000Z",
		FilePath:  localFile,
		FileSize:  13,
		Format:    "fmp4",
	}
	require.NoError(t, database.InsertRecording(rec))

	targetFile := filepath.Join(targetDir, "test.mp4")
	ps := &db.PendingSync{
		RecordingID: rec.ID,
		CameraID:    cam.ID,
		LocalPath:   localFile,
		TargetPath:  targetFile,
	}
	require.NoError(t, database.InsertPendingSync(ps))

	m := &Manager{db: database, maxRetries: 10, health: make(map[string]bool)}
	m.processSync(ps)

	_, err := os.Stat(targetFile)
	require.NoError(t, err)

	_, err = os.Stat(localFile)
	assert.True(t, os.IsNotExist(err))

	updated, err := database.GetRecording(rec.ID)
	require.NoError(t, err)
	assert.Equal(t, targetFile, updated.FilePath)

	pending, err := database.ListPendingSyncs("pending")
	require.NoError(t, err)
	assert.Len(t, pending, 0)
}

func TestSyncWorker_ProcessSync_TargetUnreachable(t *testing.T) {
	database := newTestDB(t)
	localDir := t.TempDir()

	cam := &db.Camera{Name: "test", RTSPURL: "rtsp://x"}
	require.NoError(t, database.CreateCamera(cam))

	localFile := filepath.Join(localDir, "test.mp4")
	require.NoError(t, os.WriteFile(localFile, []byte("data"), 0o644))

	rec := &db.Recording{
		CameraID:  cam.ID,
		StartTime: "2026-03-25T10:00:00.000Z",
		EndTime:   "2026-03-25T11:00:00.000Z",
		FilePath:  localFile,
		Format:    "fmp4",
	}
	require.NoError(t, database.InsertRecording(rec))

	ps := &db.PendingSync{
		RecordingID: rec.ID,
		CameraID:    cam.ID,
		LocalPath:   localFile,
		TargetPath:  "/dev/null/impossible/test.mp4",
	}
	require.NoError(t, database.InsertPendingSync(ps))

	m := &Manager{db: database, maxRetries: 10, health: make(map[string]bool)}
	m.processSync(ps)

	_, err := os.Stat(localFile)
	require.NoError(t, err)

	// After failure, status should be back to "pending" (RecordPendingSyncFailure sets it to "pending")
	// BUT the initial SetPendingSyncStatus sets it to "syncing", then RecordPendingSyncFailure resets to "pending"
	// So check for "pending" status with error message
	pending, err := database.ListPendingSyncs("pending")
	require.NoError(t, err)
	assert.Len(t, pending, 1)
	assert.NotEmpty(t, pending[0].ErrorMessage)
}
