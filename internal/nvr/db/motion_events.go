package db

import (
	"time"
)

// MotionEvent represents a motion detection event for a camera.
type MotionEvent struct {
	ID            int64   `json:"id"`
	CameraID      string  `json:"camera_id"`
	StartedAt     string  `json:"started_at"`
	EndedAt       *string `json:"ended_at"`
	ThumbnailPath string  `json:"thumbnail_path,omitempty"`
}

// InsertMotionEvent inserts a new motion event into the database.
func (d *DB) InsertMotionEvent(event *MotionEvent) error {
	res, err := d.Exec(`
		INSERT INTO motion_events (camera_id, started_at, ended_at, thumbnail_path)
		VALUES (?, ?, ?, ?)`,
		event.CameraID, event.StartedAt, event.EndedAt, event.ThumbnailPath,
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
// events for the given camera.
func (d *DB) EndMotionEvent(cameraID string, endTime string) error {
	_, err := d.Exec(`
		UPDATE motion_events
		SET ended_at = ?
		WHERE camera_id = ? AND ended_at IS NULL`,
		endTime, cameraID,
	)
	return err
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

// QueryMotionEvents returns motion events for a camera that overlap the given
// time range. An event overlaps if started_at < end AND
// (ended_at IS NULL OR ended_at > start).
func (d *DB) QueryMotionEvents(cameraID string, start, end time.Time) ([]*MotionEvent, error) {
	rows, err := d.Query(`
		SELECT id, camera_id, started_at, ended_at, COALESCE(thumbnail_path, '')
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
		if err := rows.Scan(&ev.ID, &ev.CameraID, &ev.StartedAt, &ev.EndedAt, &ev.ThumbnailPath); err != nil {
			return nil, err
		}
		events = append(events, ev)
	}
	return events, rows.Err()
}
