package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestGetMotionIntensityByType(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	defer d.Close()

	cam := &Camera{Name: "TestCam", RTSPURL: "rtsp://test"}
	require.NoError(t, d.CreateCamera(cam))

	// Insert motion and line_crossing events.
	require.NoError(t, d.InsertMotionEvent(&MotionEvent{
		CameraID: cam.ID, StartedAt: "2026-04-03T10:00:00.000Z", EventType: "motion",
	}))
	require.NoError(t, d.InsertMotionEvent(&MotionEvent{
		CameraID: cam.ID, StartedAt: "2026-04-03T10:00:30.000Z", EventType: "line_crossing",
	}))

	start, _ := time.Parse(time.RFC3339, "2026-04-03T00:00:00Z")
	end, _ := time.Parse(time.RFC3339, "2026-04-04T00:00:00Z")

	// All types — should return 2 events total.
	all, err := d.GetMotionIntensityByType(cam.ID, start, end, 60, "")
	require.NoError(t, err)
	total := 0
	for _, b := range all {
		total += b.Count
	}
	require.Equal(t, 2, total)

	// Filtered to line_crossing only.
	lc, err := d.GetMotionIntensityByType(cam.ID, start, end, 60, "line_crossing")
	require.NoError(t, err)
	total = 0
	for _, b := range lc {
		total += b.Count
	}
	require.Equal(t, 1, total)
}
