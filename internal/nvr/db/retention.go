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
