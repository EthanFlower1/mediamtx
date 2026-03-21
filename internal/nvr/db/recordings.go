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

// CameraStorage holds aggregate storage statistics for a single camera.
type CameraStorage struct {
	CameraID     string `json:"camera_id"`
	CameraName   string `json:"camera_name"`
	TotalBytes   int64  `json:"total_bytes"`
	SegmentCount int64  `json:"segment_count"`
}

// GetStoragePerCamera returns total storage used and segment count per camera.
func (d *DB) GetStoragePerCamera() ([]CameraStorage, error) {
	rows, err := d.Query(`
		SELECT r.camera_id, COALESCE(c.name, ''), COALESCE(SUM(r.file_size), 0), COUNT(*)
		FROM recordings r
		LEFT JOIN cameras c ON c.id = r.camera_id
		GROUP BY r.camera_id
		ORDER BY COALESCE(c.name, '')`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []CameraStorage
	for rows.Next() {
		var cs CameraStorage
		if err := rows.Scan(&cs.CameraID, &cs.CameraName, &cs.TotalBytes, &cs.SegmentCount); err != nil {
			return nil, err
		}
		results = append(results, cs)
	}
	return results, rows.Err()
}

// DeleteRecordingsByDateRange deletes all recordings for a camera whose
// end_time is before the given cutoff. It returns the list of file paths
// that were deleted (so the caller can remove files from disk).
func (d *DB) DeleteRecordingsByDateRange(cameraID string, before time.Time) ([]string, error) {
	beforeStr := before.UTC().Format(timeFormat)

	// Collect file paths first.
	rows, err := d.Query(
		`SELECT file_path FROM recordings WHERE camera_id = ? AND end_time < ?`,
		cameraID, beforeStr,
	)
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

	// Delete the records.
	if len(paths) > 0 {
		_, err = d.Exec(
			`DELETE FROM recordings WHERE camera_id = ? AND end_time < ?`,
			cameraID, beforeStr,
		)
		if err != nil {
			return nil, err
		}
	}

	return paths, nil
}

// GetTotalRecordingSize returns total bytes of recordings for a camera.
func (d *DB) GetTotalRecordingSize(cameraID string) (int64, error) {
	var total int64
	err := d.QueryRow(
		`SELECT COALESCE(SUM(file_size), 0) FROM recordings WHERE camera_id = ?`,
		cameraID,
	).Scan(&total)
	return total, err
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
