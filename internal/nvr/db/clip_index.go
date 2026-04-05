package db

<<<<<<< HEAD
import "time"

// EmbeddingStats holds statistics about the CLIP embedding index.
type EmbeddingStats struct {
	// Detection embeddings (live, not yet consolidated).
	DetectionTotal         int64 `json:"detection_total"`
	DetectionWithEmbedding int64 `json:"detection_with_embedding"`

	// Consolidated event embeddings.
	EventTotal         int64 `json:"event_total"`
	EventWithEmbedding int64 `json:"event_with_embedding"`

	// Oldest and newest embedding timestamps.
	OldestEmbedding string `json:"oldest_embedding,omitempty"`
	NewestEmbedding string `json:"newest_embedding,omitempty"`
}

// GetEmbeddingStats returns statistics about the CLIP search index.
func (d *DB) GetEmbeddingStats() (*EmbeddingStats, error) {
	stats := &EmbeddingStats{}

	// Detection counts.
	_ = d.QueryRow(`SELECT COUNT(*) FROM detections`).Scan(&stats.DetectionTotal)
	_ = d.QueryRow(`SELECT COUNT(*) FROM detections WHERE embedding IS NOT NULL AND length(embedding) > 0`).
		Scan(&stats.DetectionWithEmbedding)

	// Consolidated event counts.
	_ = d.QueryRow(`SELECT COUNT(*) FROM motion_events WHERE ended_at IS NOT NULL`).Scan(&stats.EventTotal)
	_ = d.QueryRow(`SELECT COUNT(*) FROM motion_events WHERE embedding IS NOT NULL AND length(embedding) > 0`).
		Scan(&stats.EventWithEmbedding)

	// Oldest embedding time (from detections or events).
	var oldestDet, oldestEvt *string
	_ = d.QueryRow(`SELECT MIN(frame_time) FROM detections WHERE embedding IS NOT NULL AND length(embedding) > 0`).
		Scan(&oldestDet)
	_ = d.QueryRow(`SELECT MIN(started_at) FROM motion_events WHERE embedding IS NOT NULL AND length(embedding) > 0`).
		Scan(&oldestEvt)
	stats.OldestEmbedding = minNonEmpty(oldestDet, oldestEvt)

	// Newest embedding time.
	var newestDet, newestEvt *string
	_ = d.QueryRow(`SELECT MAX(frame_time) FROM detections WHERE embedding IS NOT NULL AND length(embedding) > 0`).
		Scan(&newestDet)
	_ = d.QueryRow(`SELECT MAX(started_at) FROM motion_events WHERE embedding IS NOT NULL AND length(embedding) > 0`).
		Scan(&newestEvt)
	stats.NewestEmbedding = maxNonEmpty(newestDet, newestEvt)
=======
// CLIPIndexStats holds statistics about the CLIP embedding index.
type CLIPIndexStats struct {
	// DetectionEmbeddings is the count of detections with non-null embeddings.
	DetectionEmbeddings int64 `json:"detection_embeddings"`
	// DetectionsTotal is the total count of detection rows.
	DetectionsTotal int64 `json:"detections_total"`
	// EventEmbeddings is the count of motion events with non-null embeddings.
	EventEmbeddings int64 `json:"event_embeddings"`
	// EventsTotal is the total count of motion event rows.
	EventsTotal int64 `json:"events_total"`
	// EmbeddingSizeBytes is the total size of all embeddings in bytes.
	EmbeddingSizeBytes int64 `json:"embedding_size_bytes"`
	// OrphanedEmbeddings is the count of detection embeddings whose motion
	// event no longer exists (should normally be 0 due to CASCADE deletes).
	OrphanedEmbeddings int64 `json:"orphaned_embeddings"`
}

// GetCLIPIndexStats returns statistics about the CLIP embedding index including
// embedding counts, total sizes, and orphan detection.
func (d *DB) GetCLIPIndexStats() (*CLIPIndexStats, error) {
	stats := &CLIPIndexStats{}

	_ = d.QueryRow(`SELECT COUNT(*) FROM detections WHERE embedding IS NOT NULL AND length(embedding) > 0`).Scan(&stats.DetectionEmbeddings)
	_ = d.QueryRow(`SELECT COUNT(*) FROM detections`).Scan(&stats.DetectionsTotal)
	_ = d.QueryRow(`SELECT COUNT(*) FROM motion_events WHERE embedding IS NOT NULL AND length(embedding) > 0`).Scan(&stats.EventEmbeddings)
	_ = d.QueryRow(`SELECT COUNT(*) FROM motion_events`).Scan(&stats.EventsTotal)

	// Total embedding size across both tables.
	var detSize, evtSize int64
	_ = d.QueryRow(`SELECT COALESCE(SUM(length(embedding)), 0) FROM detections WHERE embedding IS NOT NULL`).Scan(&detSize)
	_ = d.QueryRow(`SELECT COALESCE(SUM(length(embedding)), 0) FROM motion_events WHERE embedding IS NOT NULL`).Scan(&evtSize)
	stats.EmbeddingSizeBytes = detSize + evtSize

	// Orphaned detections: those whose motion_event_id references a
	// non-existent motion_events row. Normally zero because of CASCADE
	// deletes, but can occur if foreign keys were disabled during a crash.
	_ = d.QueryRow(`
		SELECT COUNT(*) FROM detections d
		WHERE d.embedding IS NOT NULL AND length(d.embedding) > 0
		AND NOT EXISTS (SELECT 1 FROM motion_events me WHERE me.id = d.motion_event_id)
	`).Scan(&stats.OrphanedEmbeddings)
>>>>>>> origin/main

	return stats, nil
}

<<<<<<< HEAD
// CleanOrphanedEmbeddings removes embeddings from detections and events that
// reference motion events which no longer exist or whose recordings have been
// deleted by the retention policy. It clears embeddings on detections belonging
// to events that ended before the cutoff.
func (d *DB) CleanOrphanedEmbeddings(before time.Time) (int64, error) {
	beforeStr := before.UTC().Format(timeFormat)

	// Clear embeddings on detections whose parent events ended before cutoff.
	res, err := d.Exec(`
		UPDATE detections SET embedding = NULL
		WHERE embedding IS NOT NULL
		  AND motion_event_id IN (
			SELECT id FROM motion_events
			WHERE ended_at IS NOT NULL AND ended_at < ?
		  )`, beforeStr)
	if err != nil {
		return 0, err
	}
	detCleared, _ := res.RowsAffected()

	// Clear embeddings on consolidated events that ended before cutoff.
	res, err = d.Exec(`
		UPDATE motion_events SET embedding = NULL
		WHERE embedding IS NOT NULL
		  AND ended_at IS NOT NULL AND ended_at < ?`, beforeStr)
	if err != nil {
		return detCleared, err
	}
	evtCleared, _ := res.RowsAffected()

	return detCleared + evtCleared, nil
}

// ClearAllEmbeddings removes all CLIP embeddings from detections and events.
// Used when triggering a full reindex.
func (d *DB) ClearAllEmbeddings() (int64, error) {
	res1, err := d.Exec(`UPDATE detections SET embedding = NULL WHERE embedding IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	n1, _ := res1.RowsAffected()

	res2, err := d.Exec(`UPDATE motion_events SET embedding = NULL WHERE embedding IS NOT NULL`)
	if err != nil {
		return n1, err
	}
	n2, _ := res2.RowsAffected()

	return n1 + n2, nil
}

func minNonEmpty(a, b *string) string {
	if a == nil && b == nil {
		return ""
	}
	if a == nil {
		return *b
	}
	if b == nil {
		return *a
	}
	if *a < *b {
		return *a
	}
	return *b
}

func maxNonEmpty(a, b *string) string {
	if a == nil && b == nil {
		return ""
	}
	if a == nil {
		return *b
	}
	if b == nil {
		return *a
	}
	if *a > *b {
		return *a
	}
	return *b
=======
// CleanupOrphanedEmbeddings removes detections that reference non-existent
// motion events. Returns the number of cleaned up rows.
func (d *DB) CleanupOrphanedEmbeddings() (int64, error) {
	res, err := d.Exec(`
		DELETE FROM detections
		WHERE NOT EXISTS (SELECT 1 FROM motion_events me WHERE me.id = detections.motion_event_id)
	`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// ClearAllEmbeddings removes all CLIP embeddings from both detections and
// motion_events tables. This is used before a full re-index after a model
// update. Returns the number of detection and event rows affected.
func (d *DB) ClearAllEmbeddings() (detections int64, events int64, err error) {
	res, err := d.Exec(`UPDATE detections SET embedding = NULL WHERE embedding IS NOT NULL`)
	if err != nil {
		return 0, 0, err
	}
	detections, _ = res.RowsAffected()

	res, err = d.Exec(`UPDATE motion_events SET embedding = NULL WHERE embedding IS NOT NULL`)
	if err != nil {
		return detections, 0, err
	}
	events, _ = res.RowsAffected()

	return detections, events, nil
>>>>>>> origin/main
}
