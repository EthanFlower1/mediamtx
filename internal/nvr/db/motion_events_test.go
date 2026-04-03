package db

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestInsertMotionEventWithMetadata(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	defer d.Close()

	// Insert a camera first.
	cam := &Camera{Name: "TestCam", RTSPURL: "rtsp://test"}
	require.NoError(t, d.CreateCamera(cam))

	metadata := `{"direction":"LeftToRight"}`
	event := &MotionEvent{
		CameraID:  cam.ID,
		StartedAt: "2026-04-03T10:00:00.000Z",
		EventType: "line_crossing",
		Metadata:  &metadata,
	}
	err = d.InsertMotionEvent(event)
	require.NoError(t, err)
	require.NotZero(t, event.ID)
}

func TestQueryEvents(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	defer d.Close()

	cam := &Camera{Name: "TestCam", RTSPURL: "rtsp://test"}
	require.NoError(t, d.CreateCamera(cam))

	// Insert events of different types.
	for _, et := range []string{"motion", "tampering", "line_crossing", "intrusion"} {
		require.NoError(t, d.InsertMotionEvent(&MotionEvent{
			CameraID:  cam.ID,
			StartedAt: "2026-04-03T10:00:00.000Z",
			EventType: et,
		}))
	}

	start, _ := time.Parse(time.RFC3339, "2026-04-03T00:00:00Z")
	end, _ := time.Parse(time.RFC3339, "2026-04-04T00:00:00Z")

	// Query all types.
	all, err := d.QueryEvents(cam.ID, start, end, nil)
	require.NoError(t, err)
	require.Len(t, all, 4)

	// Query single type.
	lc, err := d.QueryEvents(cam.ID, start, end, []string{"line_crossing"})
	require.NoError(t, err)
	require.Len(t, lc, 1)
	require.Equal(t, "line_crossing", lc[0].EventType)

	// Query multiple types.
	multi, err := d.QueryEvents(cam.ID, start, end, []string{"motion", "intrusion"})
	require.NoError(t, err)
	require.Len(t, multi, 2)
}
