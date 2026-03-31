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

	// Camera 1: open event (should NOT be consolidated).
	cam1 := &Camera{Name: "cam-open", MediaMTXPath: "cam-open"}
	require.NoError(t, d.CreateCamera(cam1))

	event1 := &MotionEvent{
		CameraID:  cam1.ID,
		StartedAt: time.Now().Add(-2 * time.Hour).UTC().Format(timeFormat),
		EventType: "ai_detection",
	}
	require.NoError(t, d.InsertMotionEvent(event1))
	det1 := &Detection{
		MotionEventID: event1.ID,
		FrameTime:     time.Now().Add(-2 * time.Hour).UTC().Format(timeFormat),
		Class:         "car",
		Confidence:    0.8,
		BoxX: 0.1, BoxY: 0.2, BoxW: 0.3, BoxH: 0.4,
	}
	require.NoError(t, d.InsertDetection(det1))

	// Camera 2: recently-closed event (should NOT be consolidated).
	cam2 := &Camera{Name: "cam-recent", MediaMTXPath: "cam-recent"}
	require.NoError(t, d.CreateCamera(cam2))

	event2 := &MotionEvent{
		CameraID:  cam2.ID,
		StartedAt: time.Now().Add(-10 * time.Minute).UTC().Format(timeFormat),
		EventType: "ai_detection",
	}
	require.NoError(t, d.InsertMotionEvent(event2))
	require.NoError(t, d.EndMotionEvent(cam2.ID, time.Now().Add(-5*time.Minute).UTC().Format(timeFormat)))
	det2 := &Detection{
		MotionEventID: event2.ID,
		FrameTime:     time.Now().Add(-8 * time.Minute).UTC().Format(timeFormat),
		Class:         "dog",
		Confidence:    0.7,
		BoxX: 0.1, BoxY: 0.2, BoxW: 0.3, BoxH: 0.4,
	}
	require.NoError(t, d.InsertDetection(det2))

	// Consolidation with 1-hour threshold should skip both.
	count, err := d.ConsolidateClosedEvents(1 * time.Hour)
	require.NoError(t, err)
	assert.Equal(t, 0, count)

	// Both events should still have their detections.
	dets1, _ := d.ListDetectionsByEvent(event1.ID)
	assert.Len(t, dets1, 1)
	dets2, _ := d.ListDetectionsByEvent(event2.ID)
	assert.Len(t, dets2, 1)
}

func TestDeleteRecordingsWithoutEvents(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test-cam"}
	require.NoError(t, d.CreateCamera(cam))

	now := time.Now().UTC()
	fiveDaysAgo := now.AddDate(0, 0, -5)

	recNoEvent := &Recording{
		CameraID:  cam.ID,
		StartTime: fiveDaysAgo.Format(timeFormat),
		EndTime:   fiveDaysAgo.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/no-event.mp4",
		FileSize:  1000,
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(recNoEvent))

	recWithEvent := &Recording{
		CameraID:  cam.ID,
		StartTime: fiveDaysAgo.Add(1 * time.Hour).Format(timeFormat),
		EndTime:   fiveDaysAgo.Add(1*time.Hour + 10*time.Minute).Format(timeFormat),
		FilePath:  "/tmp/with-event.mp4",
		FileSize:  2000,
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(recWithEvent))

	event := &MotionEvent{
		CameraID:  cam.ID,
		StartedAt: fiveDaysAgo.Add(1*time.Hour + 2*time.Minute).Format(timeFormat),
		EventType: "ai_detection",
	}
	require.NoError(t, d.InsertMotionEvent(event))
	endStr := fiveDaysAgo.Add(1*time.Hour + 5*time.Minute).Format(timeFormat)
	require.NoError(t, d.EndMotionEvent(cam.ID, endStr))

	cutoff := now.AddDate(0, 0, -3)
	paths, err := d.DeleteRecordingsWithoutEvents(cam.ID, cutoff)
	require.NoError(t, err)
	assert.Equal(t, []string{"/tmp/no-event.mp4"}, paths)

	recs, err := d.QueryRecordings(cam.ID, fiveDaysAgo, now)
	require.NoError(t, err)
	assert.Len(t, recs, 1)
	assert.Equal(t, "/tmp/with-event.mp4", recs[0].FilePath)
}

func TestDeleteRecordingsWithEvents(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test-cam"}
	require.NoError(t, d.CreateCamera(cam))

	now := time.Now().UTC()
	oneYearAgo := now.AddDate(-1, -1, 0)

	rec := &Recording{
		CameraID:  cam.ID,
		StartTime: oneYearAgo.Format(timeFormat),
		EndTime:   oneYearAgo.Add(10 * time.Minute).Format(timeFormat),
		FilePath:  "/tmp/old-event.mp4",
		FileSize:  3000,
		Format:    "fmp4",
	}
	require.NoError(t, d.InsertRecording(rec))

	event := &MotionEvent{
		CameraID:  cam.ID,
		StartedAt: oneYearAgo.Add(2 * time.Minute).Format(timeFormat),
		EventType: "ai_detection",
	}
	require.NoError(t, d.InsertMotionEvent(event))
	require.NoError(t, d.EndMotionEvent(cam.ID, oneYearAgo.Add(5*time.Minute).Format(timeFormat)))

	cutoff := now.AddDate(-1, 0, 0)
	paths, err := d.DeleteRecordingsWithEvents(cam.ID, cutoff)
	require.NoError(t, err)
	assert.Equal(t, []string{"/tmp/old-event.mp4"}, paths)
}

func TestDeleteMotionEventsBefore(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "test-cam", MediaMTXPath: "cameras/test-cam"}
	require.NoError(t, d.CreateCamera(cam))

	now := time.Now().UTC()
	old := now.AddDate(0, -2, 0)

	// Use separate cameras to avoid EndMotionEvent closing both events.
	cam2 := &Camera{Name: "test-cam-2", MediaMTXPath: "cameras/test-cam-2"}
	require.NoError(t, d.CreateCamera(cam2))

	event := &MotionEvent{
		CameraID:      cam.ID,
		StartedAt:     old.Format(timeFormat),
		EventType:     "ai_detection",
		ObjectClass:   "person",
		Confidence:    0.9,
		ThumbnailPath: "/tmp/thumb-old.jpg",
	}
	require.NoError(t, d.InsertMotionEvent(event))
	require.NoError(t, d.EndMotionEvent(cam.ID, old.Add(5*time.Minute).Format(timeFormat)))

	recent := &MotionEvent{
		CameraID:  cam2.ID,
		StartedAt: now.Add(-1 * time.Hour).Format(timeFormat),
		EventType: "motion",
	}
	require.NoError(t, d.InsertMotionEvent(recent))
	require.NoError(t, d.EndMotionEvent(cam2.ID, now.Add(-50*time.Minute).Format(timeFormat)))

	cutoff := now.AddDate(0, -1, 0)
	thumbs, deleted, err := d.DeleteMotionEventsBefore(cam.ID, cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(1), deleted)
	assert.Equal(t, []string{"/tmp/thumb-old.jpg"}, thumbs)

	events, err := d.QueryMotionEvents(cam2.ID, now.Add(-2*time.Hour), now)
	require.NoError(t, err)
	assert.Len(t, events, 1)
}
