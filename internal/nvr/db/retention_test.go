package db

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func TestConsolidateClosedEvents(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test-cam", RetentionDays: 7}
	require.NoError(t, d.CreateCamera(cam))

	twoHoursAgo := time.Now().Add(-2 * time.Hour).UTC()
	event := &MotionEvent{
		CameraID:    cam.ID,
		StartedAt:   twoHoursAgo.Add(-5 * time.Minute).Format(timeFormat),
		EventType:   "ai_detection",
		ObjectClass: "person",
		Confidence:  0.9,
	}
	require.NoError(t, d.InsertMotionEvent(event))
	endedAt := twoHoursAgo.Format(timeFormat)
	require.NoError(t, d.EndMotionEvent(cam.ID, endedAt))

	det1 := &Detection{
		MotionEventID: event.ID,
		FrameTime:     twoHoursAgo.Add(-4 * time.Minute).Format(timeFormat),
		Class:         "person",
		Confidence:    0.85,
		BoxX: 0.1, BoxY: 0.2, BoxW: 0.3, BoxH: 0.4,
	}
	require.NoError(t, d.InsertDetection(det1))

	det2 := &Detection{
		MotionEventID: event.ID,
		FrameTime:     twoHoursAgo.Add(-3 * time.Minute).Format(timeFormat),
		Class:         "person",
		Confidence:    0.92,
		BoxX: 0.15, BoxY: 0.25, BoxW: 0.3, BoxH: 0.4,
		Embedding: []byte{1, 2, 3, 4},
	}
	require.NoError(t, d.InsertDetection(det2))

	det3 := &Detection{
		MotionEventID: event.ID,
		FrameTime:     twoHoursAgo.Add(-2 * time.Minute).Format(timeFormat),
		Class:         "person",
		Confidence:    0.88,
		BoxX: 0.2, BoxY: 0.3, BoxW: 0.3, BoxH: 0.4,
		Embedding: []byte{5, 6, 7, 8},
	}
	require.NoError(t, d.InsertDetection(det3))

	count, err := d.ConsolidateClosedEvents(1 * time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 1, count)

	dets, err := d.ListDetectionsByEvent(event.ID)
	require.NoError(t, err)
	assert.Empty(t, dets, "individual detections should be deleted after consolidation")

	var summary string
	var embedding []byte
	err = d.QueryRow(
		`SELECT COALESCE(detection_summary, ''), embedding FROM motion_events WHERE id = ?`,
		event.ID,
	).Scan(&summary, &embedding)
	require.NoError(t, err)

	assert.NotEmpty(t, summary)
	assert.Equal(t, []byte{1, 2, 3, 4}, embedding, "should keep embedding from highest confidence detection (det2=0.92)")

	var entries []DetectionSummaryEntry
	require.NoError(t, json.Unmarshal([]byte(summary), &entries))
	assert.Len(t, entries, 3, "summary should contain all 3 detections")
	assert.Equal(t, "person", entries[0].Class)
}

func TestConsolidateClosedEvents_SkipsRecentAndOpen(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test-cam"}
	require.NoError(t, d.CreateCamera(cam))

	event1 := &MotionEvent{
		CameraID:  cam.ID,
		StartedAt: time.Now().Add(-2 * time.Hour).UTC().Format(timeFormat),
		EventType: "ai_detection",
	}
	require.NoError(t, d.InsertMotionEvent(event1))
	det := &Detection{
		MotionEventID: event1.ID,
		FrameTime:     time.Now().Add(-2 * time.Hour).UTC().Format(timeFormat),
		Class:         "car",
		Confidence:    0.8,
		BoxX: 0.1, BoxY: 0.2, BoxW: 0.3, BoxH: 0.4,
	}
	require.NoError(t, d.InsertDetection(det))

	event2 := &MotionEvent{
		CameraID:  cam.ID,
		StartedAt: time.Now().Add(-10 * time.Minute).UTC().Format(timeFormat),
		EventType: "ai_detection",
	}
	require.NoError(t, d.InsertMotionEvent(event2))
	require.NoError(t, d.EndMotionEvent(cam.ID, time.Now().Add(-5*time.Minute).UTC().Format(timeFormat)))
	det2 := &Detection{
		MotionEventID: event2.ID,
		FrameTime:     time.Now().Add(-8 * time.Minute).UTC().Format(timeFormat),
		Class:         "dog",
		Confidence:    0.7,
		BoxX: 0.1, BoxY: 0.2, BoxW: 0.3, BoxH: 0.4,
	}
	require.NoError(t, d.InsertDetection(det2))

	count, err := d.ConsolidateClosedEvents(1 * time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	dets1, _ := d.ListDetectionsByEvent(event1.ID)
	assert.Len(t, dets1, 1)
	dets2, _ := d.ListDetectionsByEvent(event2.ID)
	assert.Len(t, dets2, 1)
}
