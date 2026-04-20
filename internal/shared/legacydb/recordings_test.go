package legacydb

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func createTestCameraForRecordings(t *testing.T, d *DB) *Camera {
	t.Helper()
	cam := &Camera{Name: "RecCam", RTSPURL: "rtsp://192.168.1.20/stream"}
	require.NoError(t, d.CreateCamera(cam))
	return cam
}

func TestInsertRecording(t *testing.T) {
	d := newTestDB(t)
	cam := createTestCameraForRecordings(t, d)

	rec := &Recording{
		CameraID:   cam.ID,
		StartTime:  "2025-01-15T10:00:00.000Z",
		EndTime:    "2025-01-15T10:05:00.000Z",
		DurationMs: 300000,
		FilePath:   "/recordings/cam1/2025-01-15T10-00.mp4",
		FileSize:   1048576,
	}
	err := d.InsertRecording(rec)
	require.NoError(t, err)
	require.NotZero(t, rec.ID)
	require.Equal(t, "fmp4", rec.Format)
}

func TestQueryRecordingsOverlap(t *testing.T) {
	d := newTestDB(t)
	cam := createTestCameraForRecordings(t, d)

	// Insert three recordings: 10:00-10:05, 10:05-10:10, 10:15-10:20
	for _, r := range []struct {
		start, end string
	}{
		{"2025-01-15T10:00:00.000Z", "2025-01-15T10:05:00.000Z"},
		{"2025-01-15T10:05:00.000Z", "2025-01-15T10:10:00.000Z"},
		{"2025-01-15T10:15:00.000Z", "2025-01-15T10:20:00.000Z"},
	} {
		require.NoError(t, d.InsertRecording(&Recording{
			CameraID:  cam.ID,
			StartTime: r.start,
			EndTime:   r.end,
			FilePath:  "/recordings/" + r.start,
		}))
	}

	// Query 10:03 - 10:07: should overlap with first two recordings.
	start := time.Date(2025, 1, 15, 10, 3, 0, 0, time.UTC)
	end := time.Date(2025, 1, 15, 10, 7, 0, 0, time.UTC)
	recs, err := d.QueryRecordings(cam.ID, start, end)
	require.NoError(t, err)
	require.Len(t, recs, 2)

	// Query 10:12 - 10:14: should return no recordings (gap).
	start = time.Date(2025, 1, 15, 10, 12, 0, 0, time.UTC)
	end = time.Date(2025, 1, 15, 10, 14, 0, 0, time.UTC)
	recs, err = d.QueryRecordings(cam.ID, start, end)
	require.NoError(t, err)
	require.Empty(t, recs)

	// Query 10:00 - 10:20: should return all three.
	start = time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	end = time.Date(2025, 1, 15, 10, 20, 0, 0, time.UTC)
	recs, err = d.QueryRecordings(cam.ID, start, end)
	require.NoError(t, err)
	require.Len(t, recs, 3)
}

func TestGetTimeline(t *testing.T) {
	d := newTestDB(t)
	cam := createTestCameraForRecordings(t, d)

	require.NoError(t, d.InsertRecording(&Recording{
		CameraID:  cam.ID,
		StartTime: "2025-01-15T10:00:00.000Z",
		EndTime:   "2025-01-15T10:05:00.000Z",
		FilePath:  "/recordings/a",
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID:  cam.ID,
		StartTime: "2025-01-15T10:10:00.000Z",
		EndTime:   "2025-01-15T10:15:00.000Z",
		FilePath:  "/recordings/b",
	}))

	start := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 15, 10, 20, 0, 0, time.UTC)

	ranges, err := d.GetTimeline(cam.ID, start, end)
	require.NoError(t, err)
	require.Len(t, ranges, 2)

	require.Equal(t, time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC), ranges[0].Start)
	require.Equal(t, time.Date(2025, 1, 15, 10, 5, 0, 0, time.UTC), ranges[0].End)
	require.Equal(t, time.Date(2025, 1, 15, 10, 10, 0, 0, time.UTC), ranges[1].Start)
	require.Equal(t, time.Date(2025, 1, 15, 10, 15, 0, 0, time.UTC), ranges[1].End)
}

func TestDeleteRecordingByPath(t *testing.T) {
	d := newTestDB(t)
	cam := createTestCameraForRecordings(t, d)

	rec := &Recording{
		CameraID:  cam.ID,
		StartTime: "2025-01-15T10:00:00.000Z",
		EndTime:   "2025-01-15T10:05:00.000Z",
		FilePath:  "/recordings/to-delete.mp4",
	}
	require.NoError(t, d.InsertRecording(rec))

	require.NoError(t, d.DeleteRecordingByPath("/recordings/to-delete.mp4"))

	// Should not be queryable anymore.
	start := time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC)
	end := time.Date(2025, 1, 15, 10, 5, 0, 0, time.UTC)
	recs, err := d.QueryRecordings(cam.ID, start, end)
	require.NoError(t, err)
	require.Empty(t, recs)

	// Deleting again should return ErrNotFound.
	require.ErrorIs(t, d.DeleteRecordingByPath("/recordings/to-delete.mp4"), ErrNotFound)
}
