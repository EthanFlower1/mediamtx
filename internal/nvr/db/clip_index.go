package db

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

	return stats, nil
}

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
}
