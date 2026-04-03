package db

import (
	"path/filepath"
	"testing"

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
