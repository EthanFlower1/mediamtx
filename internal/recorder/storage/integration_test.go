package storage

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	db "github.com/bluenviron/mediamtx/internal/recorder/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_FailoverAndRecovery(t *testing.T) {
	database := newTestDB(t)
	localDir := t.TempDir()
	nasDir := t.TempDir()

	cam := &db.Camera{
		Name:         "NAS Camera",
		RTSPURL:      "rtsp://x",
		StoragePath:  nasDir,
		MediaMTXPath: "nvr/test-cam/main",
	}
	require.NoError(t, database.CreateCamera(cam))

	m := New(database, nil, localDir, ":9997")
	m.checkInterval = 100 * time.Millisecond

	// Initial health check — NAS is healthy.
	m.runHealthCheck()
	assert.True(t, m.GetHealth(nasDir))
	assert.Equal(t, "healthy", m.StorageStatus(cam))

	// Simulate NAS going offline.
	os.RemoveAll(nasDir)
	m.runHealthCheck()
	assert.False(t, m.GetHealth(nasDir))
	assert.Equal(t, "degraded", m.StorageStatus(cam))

	// Simulate a fallback recording.
	os.MkdirAll(nasDir, 0o755)
	fallbackFile := filepath.Join(localDir, "nvr/test-cam/main/2026-03/25/10-00-00-000000.mp4")
	os.MkdirAll(filepath.Dir(fallbackFile), 0o755)
	os.WriteFile(fallbackFile, []byte("recording data"), 0o644)

	rec := &db.Recording{
		CameraID:  cam.ID,
		StartTime: "2026-03-25T10:00:00.000Z",
		EndTime:   "2026-03-25T11:00:00.000Z",
		FilePath:  fallbackFile,
		Format:    "fmp4",
		FileSize:  14,
	}
	require.NoError(t, database.InsertRecording(rec))

	ps := &db.PendingSync{
		RecordingID: rec.ID,
		CameraID:    cam.ID,
		LocalPath:   fallbackFile,
		TargetPath:  filepath.Join(nasDir, "nvr/test-cam/main/2026-03/25/10-00-00-000000.mp4"),
	}
	require.NoError(t, database.InsertPendingSync(ps))

	// NAS comes back online.
	m.runHealthCheck()
	assert.True(t, m.GetHealth(nasDir))

	// Run sync.
	m.runSyncPass()

	// Verify: file synced to NAS.
	_, err := os.Stat(ps.TargetPath)
	require.NoError(t, err)

	// Verify: local file deleted.
	_, err = os.Stat(fallbackFile)
	assert.True(t, os.IsNotExist(err))

	// Verify: recording path updated in DB.
	updated, err := database.GetRecording(rec.ID)
	require.NoError(t, err)
	assert.Equal(t, ps.TargetPath, updated.FilePath)

	// Verify: pending sync cleaned up.
	pending, err := database.ListPendingSyncs("pending")
	require.NoError(t, err)
	assert.Len(t, pending, 0)
}
