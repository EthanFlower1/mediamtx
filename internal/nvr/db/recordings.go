package db

import (
	"database/sql"
	"errors"
	"time"
)

const timeFormat = "2006-01-02T15:04:05.000Z"

// Recording represents a recording metadata record in the database.
type Recording struct {
	ID         int64  `json:"id"`
	CameraID   string `json:"camera_id"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	DurationMs int64  `json:"duration_ms"`
	FilePath   string `json:"file_path"`
	FileSize   int64  `json:"file_size"`
	Format     string `json:"format"`
}

// TimeRange represents a contiguous time range with a start and end.
type TimeRange struct {
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
}

// InsertRecording inserts a new recording into the database. The ID field is
// populated with the auto-generated value after insertion.
func (d *DB) InsertRecording(rec *Recording) error {
	if rec.Format == "" {
		rec.Format = "fmp4"
	}

	res, err := d.Exec(`
		INSERT INTO recordings (camera_id, start_time, end_time, duration_ms, file_path, file_size, format)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.CameraID, rec.StartTime, rec.EndTime, rec.DurationMs,
		rec.FilePath, rec.FileSize, rec.Format,
	)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	rec.ID = id
	return nil
}

// QueryRecordings returns recordings for a camera that overlap the given time
// range. Overlap logic: end_time > start AND start_time < end.
func (d *DB) QueryRecordings(cameraID string, start, end time.Time) ([]*Recording, error) {
	rows, err := d.Query(`
		SELECT id, camera_id, start_time, end_time, duration_ms, file_path, file_size, format
		FROM recordings
		WHERE camera_id = ? AND end_time > ? AND start_time < ?
		ORDER BY start_time`,
		cameraID, start.UTC().Format(timeFormat), end.UTC().Format(timeFormat),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []*Recording
	for rows.Next() {
		rec := &Recording{}
		if err := rows.Scan(
			&rec.ID, &rec.CameraID, &rec.StartTime, &rec.EndTime,
			&rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format,
		); err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	return recs, rows.Err()
}

// GetTimeline returns time ranges of recordings for a camera within the given
// window. Each TimeRange corresponds to a single recording's span.
func (d *DB) GetTimeline(cameraID string, start, end time.Time) ([]TimeRange, error) {
	rows, err := d.Query(`
		SELECT start_time, end_time
		FROM recordings
		WHERE camera_id = ? AND end_time > ? AND start_time < ?
		ORDER BY start_time`,
		cameraID, start.UTC().Format(timeFormat), end.UTC().Format(timeFormat),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var ranges []TimeRange
	for rows.Next() {
		var startStr, endStr string
		if err := rows.Scan(&startStr, &endStr); err != nil {
			return nil, err
		}
		s, err := time.Parse(timeFormat, startStr)
		if err != nil {
			return nil, err
		}
		e, err := time.Parse(timeFormat, endStr)
		if err != nil {
			return nil, err
		}
		ranges = append(ranges, TimeRange{Start: s, End: e})
	}
	return ranges, rows.Err()
}

// GetRecording retrieves a recording by its ID. Returns ErrNotFound if no match.
func (d *DB) GetRecording(id int64) (*Recording, error) {
	rec := &Recording{}
	err := d.QueryRow(`
		SELECT id, camera_id, start_time, end_time, duration_ms, file_path, file_size, format
		FROM recordings WHERE id = ?`, id,
	).Scan(
		&rec.ID, &rec.CameraID, &rec.StartTime, &rec.EndTime,
		&rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// DeleteRecordingByPath deletes a recording by its file path.
func (d *DB) DeleteRecordingByPath(filePath string) error {
	res, err := d.Exec("DELETE FROM recordings WHERE file_path = ?", filePath)
	if err != nil {
		return err
	}

	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
