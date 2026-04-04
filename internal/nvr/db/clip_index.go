package db

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

	return stats, nil
}

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
}
