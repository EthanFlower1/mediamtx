package db

import "time"

// DetectionEvent represents an aggregated detection event — a group of
// consecutive raw detections of the same class in the same zone, merged
// when the gap between detections is within a configurable tolerance.
type DetectionEvent struct {
	ID             int64   `json:"id"`
	CameraID       string  `json:"camera_id"`
	ZoneID         string  `json:"zone_id"`
	Class          string  `json:"class"`
	StartTime      string  `json:"start_time"`
	EndTime        string  `json:"end_time"`
	PeakConfidence float64 `json:"peak_confidence"`
	ThumbnailPath  string  `json:"thumbnail_path,omitempty"`
	DetectionCount int     `json:"detection_count"`
}

// InsertDetectionEvent inserts a new aggregated detection event.
func (d *DB) InsertDetectionEvent(ev *DetectionEvent) error {
	res, err := d.Exec(`
		INSERT INTO detection_events (camera_id, zone_id, class, start_time, end_time,
			peak_confidence, thumbnail_path, detection_count)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		ev.CameraID, ev.ZoneID, ev.Class, ev.StartTime, ev.EndTime,
		ev.PeakConfidence, ev.ThumbnailPath, ev.DetectionCount,
	)
	if err != nil {
		return err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	ev.ID = id
	return nil
}

// UpdateDetectionEvent updates end_time, peak_confidence, and detection_count
// on an existing aggregated detection event (used by the aggregator to extend
// an ongoing event).
func (d *DB) UpdateDetectionEvent(ev *DetectionEvent) error {
	_, err := d.Exec(`
		UPDATE detection_events
		SET end_time = ?, peak_confidence = ?, detection_count = ?, thumbnail_path = ?
		WHERE id = ?`,
		ev.EndTime, ev.PeakConfidence, ev.DetectionCount, ev.ThumbnailPath, ev.ID,
	)
	return err
}

// QueryDetectionEvents returns aggregated detection events for a camera within
// the given time range, optionally filtered by class. When class is empty all
// classes are returned.
func (d *DB) QueryDetectionEvents(cameraID, class string, start, end time.Time) ([]*DetectionEvent, error) {
	query := `
		SELECT id, camera_id, zone_id, class, start_time, end_time,
			peak_confidence, thumbnail_path, detection_count
		FROM detection_events
		WHERE camera_id = ?
		  AND start_time < ?
		  AND end_time > ?`

	args := []interface{}{
		cameraID,
		end.UTC().Format(timeFormat),
		start.UTC().Format(timeFormat),
	}

	if class != "" {
		query += ` AND class = ?`
		args = append(args, class)
	}

	query += ` ORDER BY start_time DESC LIMIT 500`

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*DetectionEvent
	for rows.Next() {
		ev := &DetectionEvent{}
		if err := rows.Scan(
			&ev.ID, &ev.CameraID, &ev.ZoneID, &ev.Class, &ev.StartTime,
			&ev.EndTime, &ev.PeakConfidence, &ev.ThumbnailPath, &ev.DetectionCount,
		); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// GetLatestDetectionEvent returns the most recent open (or most recent)
// detection event for the given camera, class, and zone. It is used by the
// aggregator to decide whether to extend an existing event or start a new one.
func (d *DB) GetLatestDetectionEvent(cameraID, class, zoneID string) (*DetectionEvent, error) {
	ev := &DetectionEvent{}
	err := d.QueryRow(`
		SELECT id, camera_id, zone_id, class, start_time, end_time,
			peak_confidence, thumbnail_path, detection_count
		FROM detection_events
		WHERE camera_id = ? AND class = ? AND zone_id = ?
		ORDER BY end_time DESC
		LIMIT 1`,
		cameraID, class, zoneID,
	).Scan(
		&ev.ID, &ev.CameraID, &ev.ZoneID, &ev.Class, &ev.StartTime,
		&ev.EndTime, &ev.PeakConfidence, &ev.ThumbnailPath, &ev.DetectionCount,
	)
	if err != nil {
		return nil, err
	}
	return ev, nil
}

// DeleteDetectionEventsOlderThan removes aggregated detection events whose
// end_time is older than the given cutoff. Returns the number of rows deleted.
func (d *DB) DeleteDetectionEventsOlderThan(cutoff time.Time) (int64, error) {
	res, err := d.Exec(`
		DELETE FROM detection_events WHERE end_time < ?`,
		cutoff.UTC().Format(timeFormat),
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
