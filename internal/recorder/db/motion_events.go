package db

import (
	"strings"
	"time"
)

// MotionEvent represents a motion detection event for a camera.
type MotionEvent struct {
	ID            int64   `json:"id"`
	CameraID      string  `json:"camera_id"`
	StartedAt     string  `json:"started_at"`
	EndedAt       *string `json:"ended_at"`
	ThumbnailPath string  `json:"thumbnail_path,omitempty"`
	EventType     string  `json:"event_type"`
	ObjectClass      string  `json:"object_class"`
	Confidence       float64 `json:"confidence"`
	Embedding        []byte  `json:"-"`
	DetectionSummary string  `json:"detection_summary,omitempty"`
	Metadata         *string `json:"metadata,omitempty"`
}

// InsertMotionEvent inserts a new motion event into the database.
func (d *DB) InsertMotionEvent(event *MotionEvent) error {
	if event.EventType == "" {
		event.EventType = "motion"
	}
	res, err := d.Exec(`
		INSERT INTO motion_events (camera_id, started_at, ended_at, thumbnail_path, event_type, object_class, confidence, metadata)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		event.CameraID, event.StartedAt, event.EndedAt, event.ThumbnailPath, event.EventType,
		event.ObjectClass, event.Confidence, event.Metadata,
	)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	event.ID = id
	return nil
}

// EndMotionEvent sets ended_at on all open (ended_at IS NULL) motion
// events for the given camera. This closes events of ALL types — use
// EndMotionEventByType to close only a specific event type.
func (d *DB) EndMotionEvent(cameraID string, endTime string) error {
	_, err := d.Exec(`
		UPDATE motion_events
		SET ended_at = ?
		WHERE camera_id = ? AND ended_at IS NULL`,
		endTime, cameraID,
	)
	return err
}

// EndMotionEventByType sets ended_at on open events matching the given
// camera AND event_type. This prevents closing unrelated concurrent events.
func (d *DB) EndMotionEventByType(cameraID, eventType, endTime string) error {
	_, err := d.Exec(`
		UPDATE motion_events
		SET ended_at = ?
		WHERE camera_id = ? AND event_type = ? AND ended_at IS NULL`,
		endTime, cameraID, eventType,
	)
	return err
}

// HasOpenMotionEvent returns true if there is an open (ended_at IS NULL)
// motion event for the given camera.
func (d *DB) HasOpenMotionEvent(cameraID string) bool {
	var count int
	err := d.QueryRow(`SELECT COUNT(*) FROM motion_events WHERE camera_id = ? AND ended_at IS NULL`, cameraID).Scan(&count)
	return err == nil && count > 0
}

// CloseOrphanedMotionEvents sets ended_at on all open events, using their
// started_at + 30 seconds as the end time. Called on startup to clean up
// events that were never closed due to a server crash or restart.
func (d *DB) CloseOrphanedMotionEvents() error {
	_, err := d.Exec(`
		UPDATE motion_events
		SET ended_at = datetime(started_at, '+30 seconds')
		WHERE ended_at IS NULL`)
	return err
}

// CloseStaleMotionEvents closes any motion events that have been open for
// longer than maxAge. This handles cases where the camera stops sending
// motion=false (connection lost, subscription dropped, etc.).
func (d *DB) CloseStaleMotionEvents(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge).UTC().Format(timeFormat)
	res, err := d.Exec(`
		UPDATE motion_events
		SET ended_at = datetime(started_at, '+' || CAST(? AS TEXT) || ' seconds')
		WHERE ended_at IS NULL AND started_at < ?`,
		int(maxAge.Seconds()), cutoff,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CloseStaleMotionEventsForCamera closes any open motion events for the given
// camera that have been open longer than maxAge.
func (d *DB) CloseStaleMotionEventsForCamera(cameraID string, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().Add(-maxAge).UTC().Format(timeFormat)
	res, err := d.Exec(`
		UPDATE motion_events
		SET ended_at = ?
		WHERE camera_id = ? AND ended_at IS NULL AND started_at < ?`,
		cutoff, cameraID, cutoff,
	)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// QueryMotionEvents returns motion events for a camera that overlap the given
// time range. An event overlaps if started_at < end AND
// (ended_at IS NULL OR ended_at > start).
func (d *DB) QueryMotionEvents(cameraID string, start, end time.Time) ([]*MotionEvent, error) {
	rows, err := d.Query(`
		SELECT id, camera_id, started_at, ended_at, COALESCE(thumbnail_path, ''), COALESCE(event_type, 'motion'),
		       COALESCE(object_class, ''), COALESCE(confidence, 0), metadata
		FROM motion_events
		WHERE camera_id = ?
		  AND started_at < ?
		  AND (ended_at IS NULL OR ended_at > ?)
		ORDER BY started_at`,
		cameraID,
		end.UTC().Format(timeFormat),
		start.UTC().Format(timeFormat),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*MotionEvent
	for rows.Next() {
		ev := &MotionEvent{}
		if err := rows.Scan(&ev.ID, &ev.CameraID, &ev.StartedAt, &ev.EndedAt, &ev.ThumbnailPath, &ev.EventType,
			&ev.ObjectClass, &ev.Confidence, &ev.Metadata); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// QueryMotionEventsByClass returns motion events for a camera that overlap the
// given time range, optionally filtered by object class. When objectClass is
// empty the filter is skipped and all events are returned.
func (d *DB) QueryMotionEventsByClass(cameraID, objectClass string, start, end time.Time) ([]*MotionEvent, error) {
	query := `
		SELECT id, camera_id, started_at, ended_at, COALESCE(thumbnail_path, ''), COALESCE(event_type, 'motion'),
		       COALESCE(object_class, ''), COALESCE(confidence, 0), metadata
		FROM motion_events
		WHERE camera_id = ?
		  AND started_at < ?
		  AND (ended_at IS NULL OR ended_at > ?)`

	args := []interface{}{
		cameraID,
		end.UTC().Format(timeFormat),
		start.UTC().Format(timeFormat),
	}

	if objectClass != "" {
		query += ` AND object_class = ?`
		args = append(args, objectClass)
	}

	query += ` ORDER BY started_at`

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*MotionEvent
	for rows.Next() {
		ev := &MotionEvent{}
		if err := rows.Scan(&ev.ID, &ev.CameraID, &ev.StartedAt, &ev.EndedAt, &ev.ThumbnailPath, &ev.EventType,
			&ev.ObjectClass, &ev.Confidence, &ev.Metadata); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}

// QueryEvents returns events for a camera that overlap the given time range,
// optionally filtered by event type. When eventTypes is nil or empty, all
// types are returned.
func (d *DB) QueryEvents(cameraID string, start, end time.Time, eventTypes []string) ([]*MotionEvent, error) {
	query := `
		SELECT id, camera_id, started_at, ended_at, COALESCE(thumbnail_path, ''),
		       COALESCE(event_type, 'motion'), COALESCE(object_class, ''),
		       COALESCE(confidence, 0), metadata
		FROM motion_events
		WHERE camera_id = ?
		  AND started_at < ?
		  AND (ended_at IS NULL OR ended_at > ?)`

	args := []interface{}{
		cameraID,
		end.UTC().Format(timeFormat),
		start.UTC().Format(timeFormat),
	}

	if len(eventTypes) > 0 {
		placeholders := make([]string, len(eventTypes))
		for i, et := range eventTypes {
			placeholders[i] = "?"
			args = append(args, et)
		}
		query += ` AND event_type IN (` + strings.Join(placeholders, ",") + `)`
	}

	query += ` ORDER BY started_at`

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*MotionEvent
	for rows.Next() {
		ev := &MotionEvent{}
		if err := rows.Scan(&ev.ID, &ev.CameraID, &ev.StartedAt, &ev.EndedAt,
			&ev.ThumbnailPath, &ev.EventType, &ev.ObjectClass, &ev.Confidence,
			&ev.Metadata); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}
