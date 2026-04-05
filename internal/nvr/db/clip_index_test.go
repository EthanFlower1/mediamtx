package db

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEmbeddingStats(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "stats-cam", RetentionDays: 7}
	require.NoError(t, d.CreateCamera(cam))

	// Create an event with detections.
	twoHoursAgo := time.Now().Add(-2 * time.Hour).UTC()
	event := &MotionEvent{
		CameraID:    cam.ID,
		StartedAt:   twoHoursAgo.Format(timeFormat),
		EventType:   "ai_detection",
		ObjectClass: "person",
		Confidence:  0.9,
	}
	require.NoError(t, d.InsertMotionEvent(event))
	endedAt := twoHoursAgo.Add(5 * time.Minute).Format(timeFormat)
	require.NoError(t, d.EndMotionEvent(cam.ID, endedAt))

	// Detection without embedding.
	det1 := &Detection{
		MotionEventID: event.ID,
		FrameTime:     twoHoursAgo.Add(1 * time.Minute).Format(timeFormat),
		Class:         "person",
		Confidence:    0.85,
		BoxX: 0.1, BoxY: 0.2, BoxW: 0.3, BoxH: 0.4,
	}
	require.NoError(t, d.InsertDetection(det1))

	// Detection with embedding.
	det2 := &Detection{
		MotionEventID: event.ID,
		FrameTime:     twoHoursAgo.Add(2 * time.Minute).Format(timeFormat),
		Class:         "person",
		Confidence:    0.92,
		BoxX: 0.1, BoxY: 0.2, BoxW: 0.3, BoxH: 0.4,
		Embedding: []byte{1, 2, 3, 4},
	}
	require.NoError(t, d.InsertDetection(det2))

	stats, err := d.GetEmbeddingStats()
	require.NoError(t, err)

	assert.Equal(t, int64(2), stats.DetectionTotal)
	assert.Equal(t, int64(1), stats.DetectionWithEmbedding)
	assert.Equal(t, int64(1), stats.EventTotal) // one closed event
	assert.Equal(t, int64(0), stats.EventWithEmbedding)
	assert.NotEmpty(t, stats.OldestEmbedding)
	assert.NotEmpty(t, stats.NewestEmbedding)
}

func TestCleanOrphanedEmbeddings(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "orphan-cam", RetentionDays: 7}
	require.NoError(t, d.CreateCamera(cam))

	// Create an old event (ended 48 hours ago).
	oldTime := time.Now().Add(-48 * time.Hour).UTC()
	oldEvent := &MotionEvent{
		CameraID:    cam.ID,
		StartedAt:   oldTime.Format(timeFormat),
		EventType:   "ai_detection",
		ObjectClass: "person",
		Confidence:  0.9,
	}
	require.NoError(t, d.InsertMotionEvent(oldEvent))
	oldEndedAt := oldTime.Add(5 * time.Minute).Format(timeFormat)
	require.NoError(t, d.EndMotionEvent(cam.ID, oldEndedAt))

	// Add embedding to event.
	_, err := d.Exec(`UPDATE motion_events SET embedding = ? WHERE id = ?`,
		[]byte{1, 2, 3, 4}, oldEvent.ID)
	require.NoError(t, err)

	// Detection with embedding on old event.
	det := &Detection{
		MotionEventID: oldEvent.ID,
		FrameTime:     oldTime.Add(1 * time.Minute).Format(timeFormat),
		Class:         "person",
		Confidence:    0.85,
		BoxX: 0.1, BoxY: 0.2, BoxW: 0.3, BoxH: 0.4,
		Embedding: []byte{5, 6, 7, 8},
	}
	require.NoError(t, d.InsertDetection(det))

	// Create a recent event (ended 1 hour ago).
	recentTime := time.Now().Add(-1 * time.Hour).UTC()
	recentEvent := &MotionEvent{
		CameraID:    cam.ID,
		StartedAt:   recentTime.Format(timeFormat),
		EventType:   "ai_detection",
		ObjectClass: "car",
		Confidence:  0.8,
	}
	require.NoError(t, d.InsertMotionEvent(recentEvent))
	recentEndedAt := recentTime.Add(5 * time.Minute).Format(timeFormat)
	// Close the first event's "ended_at" was already set; we need to set
	// ended_at directly for the second event since EndMotionEvent closes all.
	_, err = d.Exec(`UPDATE motion_events SET ended_at = ? WHERE id = ?`,
		recentEndedAt, recentEvent.ID)
	require.NoError(t, err)

	// Add embedding to recent event.
	_, err = d.Exec(`UPDATE motion_events SET embedding = ? WHERE id = ?`,
		[]byte{9, 10, 11, 12}, recentEvent.ID)
	require.NoError(t, err)

	// Detection with embedding on recent event.
	det2 := &Detection{
		MotionEventID: recentEvent.ID,
		FrameTime:     recentTime.Add(1 * time.Minute).Format(timeFormat),
		Class:         "car",
		Confidence:    0.8,
		BoxX: 0.1, BoxY: 0.2, BoxW: 0.3, BoxH: 0.4,
		Embedding: []byte{13, 14, 15, 16},
	}
	require.NoError(t, d.InsertDetection(det2))

	// Clean embeddings older than 24 hours.
	cutoff := time.Now().Add(-24 * time.Hour)
	cleared, err := d.CleanOrphanedEmbeddings(cutoff)
	require.NoError(t, err)
	assert.Equal(t, int64(2), cleared) // 1 detection + 1 event embedding

	// Verify old embeddings are gone.
	stats, err := d.GetEmbeddingStats()
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.DetectionWithEmbedding) // recent detection kept
	assert.Equal(t, int64(1), stats.EventWithEmbedding)     // recent event kept
}

func TestClearAllEmbeddings(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "clear-cam", RetentionDays: 7}
	require.NoError(t, d.CreateCamera(cam))

	event := &MotionEvent{
		CameraID:    cam.ID,
		StartedAt:   time.Now().Add(-1 * time.Hour).UTC().Format(timeFormat),
		EventType:   "ai_detection",
		ObjectClass: "person",
		Confidence:  0.9,
	}
	require.NoError(t, d.InsertMotionEvent(event))
	require.NoError(t, d.EndMotionEvent(cam.ID, time.Now().UTC().Format(timeFormat)))

	// Add embedding to event.
	_, err := d.Exec(`UPDATE motion_events SET embedding = ? WHERE id = ?`,
		[]byte{1, 2, 3, 4}, event.ID)
	require.NoError(t, err)

	det := &Detection{
		MotionEventID: event.ID,
		FrameTime:     time.Now().Add(-30 * time.Minute).UTC().Format(timeFormat),
		Class:         "person",
		Confidence:    0.85,
		BoxX: 0.1, BoxY: 0.2, BoxW: 0.3, BoxH: 0.4,
		Embedding: []byte{5, 6, 7, 8},
	}
	require.NoError(t, d.InsertDetection(det))

	cleared, err := d.ClearAllEmbeddings()
	require.NoError(t, err)
	assert.Equal(t, int64(2), cleared)

	stats, err := d.GetEmbeddingStats()
	require.NoError(t, err)
	assert.Equal(t, int64(0), stats.DetectionWithEmbedding)
	assert.Equal(t, int64(0), stats.EventWithEmbedding)
}
