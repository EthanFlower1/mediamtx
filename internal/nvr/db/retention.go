package db

import (
	"encoding/json"
	"time"
)

// DetectionSummaryEntry is a compact representation of a detection for storage
// in the motion_event's detection_summary JSON field after consolidation.
type DetectionSummaryEntry struct {
	FrameTime  string  `json:"t"`
	Class      string  `json:"c"`
	Confidence float64 `json:"cf"`
	BoxX       float64 `json:"x"`
	BoxY       float64 `json:"y"`
	BoxW       float64 `json:"w"`
	BoxH       float64 `json:"h"`
}

// ConsolidateClosedEvents finds closed motion events older than the given
// threshold that still have individual detection rows. For each, it builds a
// compact JSON summary, keeps the best CLIP embedding, stores both on the
// motion_event row, and deletes the individual detection rows.
func (d *DB) ConsolidateClosedEvents(olderThan time.Duration) (int, error) {
	cutoff := time.Now().Add(-olderThan).UTC().Format(timeFormat)

	rows, err := d.Query(`
		SELECT DISTINCT me.id
		FROM motion_events me
		INNER JOIN detections det ON det.motion_event_id = me.id
		WHERE me.ended_at IS NOT NULL
		  AND me.ended_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var eventIDs []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return 0, err
		}
		eventIDs = append(eventIDs, id)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	consolidated := 0
	for _, eventID := range eventIDs {
		if err := d.consolidateEvent(eventID); err != nil {
			continue
		}
		consolidated++
	}
	return consolidated, nil
}

func (d *DB) consolidateEvent(eventID int64) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	detRows, err := tx.Query(`
		SELECT frame_time, class, confidence, box_x, box_y, box_w, box_h, embedding
		FROM detections WHERE motion_event_id = ?
		ORDER BY frame_time`, eventID)
	if err != nil {
		return err
	}

	var entries []DetectionSummaryEntry
	var bestEmbedding []byte
	var bestConfidence float64

	for detRows.Next() {
		var e DetectionSummaryEntry
		var embedding []byte
		if err := detRows.Scan(&e.FrameTime, &e.Class, &e.Confidence,
			&e.BoxX, &e.BoxY, &e.BoxW, &e.BoxH, &embedding); err != nil {
			detRows.Close()
			return err
		}
		entries = append(entries, e)
		if len(embedding) > 0 && e.Confidence > bestConfidence {
			bestEmbedding = embedding
			bestConfidence = e.Confidence
		}
	}
	detRows.Close()
	if err := detRows.Err(); err != nil {
		return err
	}

	if len(entries) == 0 {
		return nil
	}

	summaryJSON, err := json.Marshal(entries)
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`
		UPDATE motion_events SET detection_summary = ?, embedding = ?
		WHERE id = ?`,
		string(summaryJSON), bestEmbedding, eventID); err != nil {
		return err
	}

	if _, err := tx.Exec(
		`DELETE FROM detections WHERE motion_event_id = ?`, eventID); err != nil {
		return err
	}

	return tx.Commit()
}

// DeleteRecordingsWithoutEvents deletes recordings for a camera that ended
// before the cutoff and have NO overlapping motion events. Returns the file
// paths of deleted recordings for disk cleanup.
func (d *DB) DeleteRecordingsWithoutEvents(cameraID string, before time.Time) ([]string, error) {
	beforeStr := before.UTC().Format(timeFormat)

	rows, err := d.Query(`
		SELECT r.file_path FROM recordings r
		WHERE r.camera_id = ? AND r.end_time < ?
		AND NOT EXISTS (
			SELECT 1 FROM motion_events me
			WHERE me.camera_id = r.camera_id
			AND me.started_at < r.end_time
			AND (me.ended_at IS NULL OR me.ended_at > r.start_time)
		)`, cameraID, beforeStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(paths) > 0 {
		_, err = d.Exec(`
			DELETE FROM recordings
			WHERE camera_id = ? AND end_time < ?
			AND NOT EXISTS (
				SELECT 1 FROM motion_events me
				WHERE me.camera_id = recordings.camera_id
				AND me.started_at < recordings.end_time
				AND (me.ended_at IS NULL OR me.ended_at > recordings.start_time)
			)`, cameraID, beforeStr)
		if err != nil {
			return nil, err
		}
	}

	return paths, nil
}

// DeleteRecordingsWithEvents deletes recordings for a camera that ended before
// the cutoff and DO have overlapping motion events. Returns the file paths of
// deleted recordings for disk cleanup.
func (d *DB) DeleteRecordingsWithEvents(cameraID string, before time.Time) ([]string, error) {
	beforeStr := before.UTC().Format(timeFormat)

	rows, err := d.Query(`
		SELECT r.file_path FROM recordings r
		WHERE r.camera_id = ? AND r.end_time < ?
		AND EXISTS (
			SELECT 1 FROM motion_events me
			WHERE me.camera_id = r.camera_id
			AND me.started_at < r.end_time
			AND (me.ended_at IS NULL OR me.ended_at > r.start_time)
		)`, cameraID, beforeStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(paths) > 0 {
		_, err = d.Exec(`
			DELETE FROM recordings
			WHERE camera_id = ? AND end_time < ?
			AND EXISTS (
				SELECT 1 FROM motion_events me
				WHERE me.camera_id = recordings.camera_id
				AND me.started_at < recordings.end_time
				AND (me.ended_at IS NULL OR me.ended_at > recordings.start_time)
			)`, cameraID, beforeStr)
		if err != nil {
			return nil, err
		}
	}

	return paths, nil
}

// DeleteMotionEventsBefore deletes closed motion events for a camera that
// ended before the cutoff. Returns thumbnail paths for disk cleanup and the
// number of deleted events. Associated detections are CASCADE-deleted.
func (d *DB) DeleteMotionEventsBefore(cameraID string, before time.Time) (thumbnailPaths []string, deleted int64, err error) {
	beforeStr := before.UTC().Format(timeFormat)

	thumbRows, err := d.Query(`
		SELECT thumbnail_path FROM motion_events
		WHERE camera_id = ? AND ended_at IS NOT NULL AND ended_at < ?
		AND thumbnail_path != ''`, cameraID, beforeStr)
	if err != nil {
		return nil, 0, err
	}
	defer thumbRows.Close()

	for thumbRows.Next() {
		var p string
		if err := thumbRows.Scan(&p); err != nil {
			return nil, 0, err
		}
		thumbnailPaths = append(thumbnailPaths, p)
	}
	if err := thumbRows.Err(); err != nil {
		return nil, 0, err
	}

	res, err := d.Exec(`
		DELETE FROM motion_events
		WHERE camera_id = ? AND ended_at IS NOT NULL AND ended_at < ?`,
		cameraID, beforeStr)
	if err != nil {
		return nil, 0, err
	}

	deleted, _ = res.RowsAffected()
	return thumbnailPaths, deleted, nil
}

// DeleteStreamRecordingsWithoutEvents deletes recordings for a specific stream
// that ended before the cutoff and have NO overlapping motion events.
func (d *DB) DeleteStreamRecordingsWithoutEvents(cameraID, streamID string, before time.Time) ([]string, error) {
	beforeStr := before.UTC().Format(timeFormat)

	rows, err := d.Query(`
		SELECT r.file_path FROM recordings r
		WHERE r.camera_id = ? AND r.stream_id = ? AND r.end_time < ?
		AND NOT EXISTS (
			SELECT 1 FROM motion_events me
			WHERE me.camera_id = r.camera_id
			AND me.started_at < r.end_time
			AND (me.ended_at IS NULL OR me.ended_at > r.start_time)
		)`, cameraID, streamID, beforeStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(paths) > 0 {
		_, err = d.Exec(`
			DELETE FROM recordings
			WHERE camera_id = ? AND stream_id = ? AND end_time < ?
			AND NOT EXISTS (
				SELECT 1 FROM motion_events me
				WHERE me.camera_id = recordings.camera_id
				AND me.started_at < recordings.end_time
				AND (me.ended_at IS NULL OR me.ended_at > recordings.start_time)
			)`, cameraID, streamID, beforeStr)
		if err != nil {
			return nil, err
		}
	}

	return paths, nil
}

// DeleteStreamRecordingsWithEvents deletes recordings for a specific stream
// that ended before the cutoff and DO have overlapping motion events.
func (d *DB) DeleteStreamRecordingsWithEvents(cameraID, streamID string, before time.Time) ([]string, error) {
	beforeStr := before.UTC().Format(timeFormat)

	rows, err := d.Query(`
		SELECT r.file_path FROM recordings r
		WHERE r.camera_id = ? AND r.stream_id = ? AND r.end_time < ?
		AND EXISTS (
			SELECT 1 FROM motion_events me
			WHERE me.camera_id = r.camera_id
			AND me.started_at < r.end_time
			AND (me.ended_at IS NULL OR me.ended_at > r.start_time)
		)`, cameraID, streamID, beforeStr)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if len(paths) > 0 {
		_, err = d.Exec(`
			DELETE FROM recordings
			WHERE camera_id = ? AND stream_id = ? AND end_time < ?
			AND EXISTS (
				SELECT 1 FROM motion_events me
				WHERE me.camera_id = recordings.camera_id
				AND me.started_at < recordings.end_time
				AND (me.ended_at IS NULL OR me.ended_at > recordings.start_time)
			)`, cameraID, streamID, beforeStr)
		if err != nil {
			return nil, err
		}
	}

	return paths, nil
}

// TableStats holds row count for a database table.
type TableStats struct {
	RowCount int64 `json:"row_count"`
}

// DatabaseStats holds size and per-table statistics for the SQLite database.
type DatabaseStats struct {
	FileSizeBytes int64                `json:"file_size_bytes"`
	Tables        map[string]TableStats `json:"tables"`
}

// GetDatabaseStats returns the database file size and row counts for key tables.
func (d *DB) GetDatabaseStats() (*DatabaseStats, error) {
	stats := &DatabaseStats{
		Tables: make(map[string]TableStats),
	}

	var pageCount, pageSize int64
	_ = d.QueryRow("PRAGMA page_count").Scan(&pageCount)
	_ = d.QueryRow("PRAGMA page_size").Scan(&pageSize)
	stats.FileSizeBytes = pageCount * pageSize

	tables := []string{
		"recordings", "recording_fragments", "motion_events",
		"detections", "audit_log", "pending_syncs", "screenshots",
	}
	for _, table := range tables {
		var count int64
		_ = d.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count)
		stats.Tables[table] = TableStats{RowCount: count}
	}

	return stats, nil
}
