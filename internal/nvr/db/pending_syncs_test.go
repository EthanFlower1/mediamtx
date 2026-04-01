package db

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPendingSyncCRUD(t *testing.T) {
	d := newTestDB(t)

	cam := &Camera{Name: "test", RTSPURL: "rtsp://x"}
	require.NoError(t, d.CreateCamera(cam))
	rec := &Recording{
		CameraID:  cam.ID,
		StartTime: "2026-03-25T10:00:00.000Z",
		EndTime:   "2026-03-25T11:00:00.000Z",
		FilePath:  "./recordings/test.mp4",
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec))

	ps := &PendingSync{
		RecordingID: rec.ID,
		CameraID:    cam.ID,
		LocalPath:   "./recordings/test.mp4",
		TargetPath:  "/mnt/nas1/recordings/test.mp4",
	}
	require.NoError(t, d.InsertPendingSync(ps))
	assert.NotZero(t, ps.ID)

	pending, err := d.ListPendingSyncs("pending")
	require.NoError(t, err)
	assert.Len(t, pending, 1)
	assert.Equal(t, ps.LocalPath, pending[0].LocalPath)

	require.NoError(t, d.SetPendingSyncStatus(ps.ID, "syncing"))
	pending, err = d.ListPendingSyncs("pending")
	require.NoError(t, err)
	assert.Len(t, pending, 0)
	syncing, err := d.ListPendingSyncs("syncing")
	require.NoError(t, err)
	assert.Len(t, syncing, 1)

	require.NoError(t, d.RecordPendingSyncFailure(ps.ID, "failed", "connection refused"))
	failed, err := d.ListPendingSyncs("failed")
	require.NoError(t, err)
	assert.Equal(t, "connection refused", failed[0].ErrorMessage)

	require.NoError(t, d.DeletePendingSync(ps.ID))
	all, err := d.ListPendingSyncs("pending")
	require.NoError(t, err)
	assert.Len(t, all, 0)
}

func TestPendingSyncCascadeDelete(t *testing.T) {
	d := newTestDB(t)
	cam := &Camera{Name: "test", RTSPURL: "rtsp://x"}
	require.NoError(t, d.CreateCamera(cam))
	rec := &Recording{
		CameraID:  cam.ID,
		StartTime: "2026-03-25T10:00:00.000Z",
		EndTime:   "2026-03-25T11:00:00.000Z",
		FilePath:  "./recordings/test.mp4",
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec))
	ps := &PendingSync{
		RecordingID: rec.ID,
		CameraID:    cam.ID,
		LocalPath:   "./recordings/test.mp4",
		TargetPath:  "/mnt/nas/test.mp4",
	}
	require.NoError(t, d.InsertPendingSync(ps))

	require.NoError(t, d.DeleteRecordingByPath(rec.FilePath))
	pending, err := d.ListPendingSyncs("pending")
	require.NoError(t, err)
	assert.Len(t, pending, 0)
}

func TestPendingSyncCountByCamera(t *testing.T) {
	d := newTestDB(t)
	cam := &Camera{Name: "test", RTSPURL: "rtsp://x"}
	require.NoError(t, d.CreateCamera(cam))
	rec := &Recording{
		CameraID:  cam.ID,
		StartTime: "2026-03-25T10:00:00.000Z",
		EndTime:   "2026-03-25T11:00:00.000Z",
		FilePath:  "./recordings/test.mp4",
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec))
	ps := &PendingSync{
		RecordingID: rec.ID,
		CameraID:    cam.ID,
		LocalPath:   "./recordings/test.mp4",
		TargetPath:  "/mnt/nas/test.mp4",
	}
	require.NoError(t, d.InsertPendingSync(ps))

	counts, err := d.PendingSyncCountByCamera()
	require.NoError(t, err)
	assert.Equal(t, 1, counts[cam.ID])
}
