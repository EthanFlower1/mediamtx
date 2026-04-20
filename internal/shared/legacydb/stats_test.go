package legacydb

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetRecordingStats_NoRecordings(t *testing.T) {
	d := newTestDB(t)

	stats, err := d.GetRecordingStats("")
	require.NoError(t, err)
	require.Empty(t, stats)
}

func TestGetRecordingStats_SingleCamera(t *testing.T) {
	d := newTestDB(t)
	cam := createTestCameraForRecordings(t, d)

	require.NoError(t, d.InsertRecording(&Recording{
		CameraID:   cam.ID,
		StartTime:  "2025-01-15T10:00:00.000Z",
		EndTime:    "2025-01-15T11:00:00.000Z",
		DurationMs: 3600000,
		FilePath:   "/recordings/a.mp4",
		FileSize:   500000,
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID:   cam.ID,
		StartTime:  "2025-01-15T11:00:00.000Z",
		EndTime:    "2025-01-15T12:00:00.000Z",
		DurationMs: 3600000,
		FilePath:   "/recordings/b.mp4",
		FileSize:   600000,
	}))

	stats, err := d.GetRecordingStats("")
	require.NoError(t, err)
	require.Len(t, stats, 1)

	s := stats[0]
	require.Equal(t, cam.ID, s.CameraID)
	require.Equal(t, "RecCam", s.CameraName)
	require.Equal(t, int64(1100000), s.TotalBytes)
	require.Equal(t, int64(2), s.SegmentCount)
	require.Equal(t, int64(7200000), s.TotalRecordedMs)
	require.Equal(t, "2025-01-15T10:00:00.000Z", s.OldestRecording)
	require.Equal(t, "2025-01-15T12:00:00.000Z", s.NewestRecording)
}

func TestGetRecordingStats_FilterByCamera(t *testing.T) {
	d := newTestDB(t)
	cam1 := createTestCameraForRecordings(t, d)
	cam2 := &Camera{Name: "OtherCam", RTSPURL: "rtsp://192.168.1.21/stream", MediaMTXPath: "cameras/othercam"}
	require.NoError(t, d.CreateCamera(cam2))

	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam1.ID, StartTime: "2025-01-15T10:00:00.000Z",
		EndTime: "2025-01-15T11:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/c1.mp4", FileSize: 100000,
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam2.ID, StartTime: "2025-01-15T10:00:00.000Z",
		EndTime: "2025-01-15T11:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/c2.mp4", FileSize: 200000,
	}))

	stats, err := d.GetRecordingStats(cam1.ID)
	require.NoError(t, err)
	require.Len(t, stats, 1)
	require.Equal(t, cam1.ID, stats[0].CameraID)
}

func TestGetRecordingGaps_NoRecordings(t *testing.T) {
	d := newTestDB(t)

	gaps, err := d.GetRecordingGaps("nonexistent", 2000)
	require.NoError(t, err)
	require.Empty(t, gaps)
}

func TestGetRecordingGaps_NoGaps(t *testing.T) {
	d := newTestDB(t)
	cam := createTestCameraForRecordings(t, d)

	// Two contiguous recordings (no gap).
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T10:00:00.000Z",
		EndTime: "2025-01-15T11:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/g1.mp4", FileSize: 100,
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T11:00:00.000Z",
		EndTime: "2025-01-15T12:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/g2.mp4", FileSize: 100,
	}))

	gaps, err := d.GetRecordingGaps(cam.ID, 2000)
	require.NoError(t, err)
	require.Empty(t, gaps)
}

func TestGetRecordingGaps_WithGaps(t *testing.T) {
	d := newTestDB(t)
	cam := createTestCameraForRecordings(t, d)

	// Three recordings with a 5-minute gap between second and third.
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T10:00:00.000Z",
		EndTime: "2025-01-15T11:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/h1.mp4", FileSize: 100,
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T11:00:00.000Z",
		EndTime: "2025-01-15T12:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/h2.mp4", FileSize: 100,
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T12:05:00.000Z",
		EndTime: "2025-01-15T13:00:00.000Z", DurationMs: 3300000,
		FilePath: "/recordings/h3.mp4", FileSize: 100,
	}))

	gaps, err := d.GetRecordingGaps(cam.ID, 2000)
	require.NoError(t, err)
	require.Len(t, gaps, 1)
	require.Equal(t, "2025-01-15T12:00:00.000Z", gaps[0].Start)
	require.Equal(t, "2025-01-15T12:05:00.000Z", gaps[0].End)
	require.Equal(t, int64(300000), gaps[0].DurationMs)
}

func TestGetRecordingGaps_SmallGapFiltered(t *testing.T) {
	d := newTestDB(t)
	cam := createTestCameraForRecordings(t, d)

	// Two recordings with a 1-second gap (below 2000ms threshold).
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T10:00:00.000Z",
		EndTime: "2025-01-15T11:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/s1.mp4", FileSize: 100,
	}))
	require.NoError(t, d.InsertRecording(&Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T11:00:01.000Z",
		EndTime: "2025-01-15T12:00:00.000Z", DurationMs: 3599000,
		FilePath: "/recordings/s2.mp4", FileSize: 100,
	}))

	gaps, err := d.GetRecordingGaps(cam.ID, 2000)
	require.NoError(t, err)
	require.Empty(t, gaps)
}
