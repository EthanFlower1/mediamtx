package db

import (
	"database/sql"
	"errors"
	"strings"
	"time"
)

const timeFormat = "2006-01-02T15:04:05.000Z"

// Recording represents a recording metadata record in the database.
type Recording struct {
	ID         int64  `json:"id"`
	CameraID   string `json:"camera_id"`
	StreamID   string `json:"stream_id"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	DurationMs int64  `json:"duration_ms"`
	FilePath   string `json:"file_path"`
	FileSize   int64  `json:"file_size"`
	Format         string  `json:"format"`
	InitSize       int64   `json:"init_size"`
	Status         string  `json:"status"`
	StatusDetail   *string `json:"status_detail"`
	VerifiedAt     *string `json:"verified_at"`
	MediaStartTime *string `json:"media_start_time,omitempty"`
}

// RecordingFragment represents a single moof+mdat fragment within an fMP4 recording.
type RecordingFragment struct {
	ID            int64   `json:"id"`
	RecordingID   int64   `json:"recording_id"`
	FragmentIndex int     `json:"fragment_index"`
	ByteOffset    int64   `json:"byte_offset"`
	Size          int64   `json:"size"`
	DurationMs    float64 `json:"duration_ms"`
	IsKeyframe    bool    `json:"is_keyframe"`
	TimestampMs   int64   `json:"timestamp_ms"`
}

// InsertFragments bulk-inserts fragment metadata for a recording using INSERT OR IGNORE
// to ensure idempotency.
func (d *DB) InsertFragments(recordingID int64, fragments []RecordingFragment) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.Prepare(`
        INSERT OR IGNORE INTO recording_fragments
        (recording_id, fragment_index, byte_offset, size, duration_ms, is_keyframe, timestamp_ms)
        VALUES (?, ?, ?, ?, ?, ?, ?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, f := range fragments {
		_, err := stmt.Exec(f.RecordingID, f.FragmentIndex, f.ByteOffset, f.Size,
			f.DurationMs, f.IsKeyframe, f.TimestampMs)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// UpdateRecordingInitSize sets the init_size (ftyp + moov byte length) for a recording.
func (d *DB) UpdateRecordingInitSize(recordingID int64, initSize int64) error {
	_, err := d.Exec("UPDATE recordings SET init_size = ? WHERE id = ?", initSize, recordingID)
	return err
}

// GetFragments returns all fragments for a recording, ordered by fragment_index.
func (d *DB) GetFragments(recordingID int64) ([]RecordingFragment, error) {
	rows, err := d.Query(`
        SELECT id, recording_id, fragment_index, byte_offset, size, duration_ms, is_keyframe, timestamp_ms
        FROM recording_fragments
        WHERE recording_id = ?
        ORDER BY fragment_index`, recordingID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var frags []RecordingFragment
	for rows.Next() {
		var f RecordingFragment
		if err := rows.Scan(&f.ID, &f.RecordingID, &f.FragmentIndex, &f.ByteOffset,
			&f.Size, &f.DurationMs, &f.IsKeyframe, &f.TimestampMs); err != nil {
			return nil, err
		}
		frags = append(frags, f)
	}
	return frags, rows.Err()
}

// HasFragments checks whether a recording has been indexed (has fragment rows).
func (d *DB) HasFragments(recordingID int64) (bool, error) {
	var count int
	err := d.QueryRow("SELECT COUNT(*) FROM recording_fragments WHERE recording_id = ?", recordingID).Scan(&count)
	return count > 0, err
}

// GetUnindexedRecordings returns recording IDs that have no fragments, newest first.
func (d *DB) GetUnindexedRecordings() ([]*Recording, error) {
	rows, err := d.Query(`
        SELECT r.id, r.camera_id, r.stream_id, r.start_time, r.end_time, r.duration_ms, r.file_path, r.file_size, r.format, r.init_size, r.status, r.status_detail, r.verified_at, r.media_start_time
        FROM recordings r
        LEFT JOIN recording_fragments rf ON rf.recording_id = r.id
        WHERE rf.id IS NULL
        ORDER BY r.start_time DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []*Recording
	for rows.Next() {
		rec := &Recording{}
		if err := rows.Scan(&rec.ID, &rec.CameraID, &rec.StreamID, &rec.StartTime, &rec.EndTime,
			&rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format, &rec.InitSize, &rec.Status, &rec.StatusDetail, &rec.VerifiedAt, &rec.MediaStartTime); err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	return recs, rows.Err()
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
		INSERT INTO recordings (camera_id, stream_id, start_time, end_time, duration_ms, file_path, file_size, format)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		rec.CameraID, rec.StreamID, rec.StartTime, rec.EndTime, rec.DurationMs,
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

// CompleteRecording updates an in-progress recording with its final end time,
// duration, and file size. Used when a segment that was registered at creation
// time finishes writing.
func (d *DB) CompleteRecording(filePath string, endTime string, durationMs int64, fileSize int64) error {
	_, err := d.Exec(`
		UPDATE recordings SET end_time = ?, duration_ms = ?, file_size = ?
		WHERE file_path = ?`,
		endTime, durationMs, fileSize, filePath,
	)
	return err
}

// UpdateMediaTimestamps updates a recording's media_start_time, start_time, and end_time
// from NTP-derived media timestamps. This corrects the wall-clock drift from time.Now().
func (d *DB) UpdateMediaTimestamps(recordingID int64, mediaStartTime, startTime, endTime string) error {
	_, err := d.Exec(`
		UPDATE recordings SET media_start_time = ?, start_time = ?, end_time = ?
		WHERE id = ?`,
		mediaStartTime, startTime, endTime, recordingID,
	)
	return err
}

// GetRecordingByFilePath looks up a recording by its file path.
func (d *DB) GetRecordingByFilePath(filePath string) (*Recording, error) {
	row := d.QueryRow(`
		SELECT id, camera_id, stream_id, start_time, end_time, duration_ms, file_path, file_size, format, init_size, status, status_detail, verified_at, media_start_time
		FROM recordings WHERE file_path = ?`, filePath)

	rec := &Recording{}
	err := row.Scan(
		&rec.ID, &rec.CameraID, &rec.StreamID, &rec.StartTime, &rec.EndTime,
		&rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format, &rec.InitSize,
		&rec.Status, &rec.StatusDetail, &rec.VerifiedAt, &rec.MediaStartTime,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return rec, nil
}

// QueryRecordings returns recordings for a camera that overlap the given time
// range. Overlap logic: end_time > start AND start_time < end.
func (d *DB) QueryRecordings(cameraID string, start, end time.Time) ([]*Recording, error) {
	rows, err := d.Query(`
		SELECT id, camera_id, stream_id, start_time, end_time, duration_ms, file_path, file_size, format, init_size, status, status_detail, verified_at, media_start_time
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
			&rec.ID, &rec.CameraID, &rec.StreamID, &rec.StartTime, &rec.EndTime,
			&rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format, &rec.InitSize,
			&rec.Status, &rec.StatusDetail, &rec.VerifiedAt, &rec.MediaStartTime,
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
		SELECT id, camera_id, stream_id, start_time, end_time, duration_ms, file_path, file_size, format, init_size, status, status_detail, verified_at, media_start_time
		FROM recordings WHERE id = ?`, id,
	).Scan(
		&rec.ID, &rec.CameraID, &rec.StreamID, &rec.StartTime, &rec.EndTime,
		&rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format, &rec.InitSize,
		&rec.Status, &rec.StatusDetail, &rec.VerifiedAt, &rec.MediaStartTime,
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

// StreamStorage holds aggregate storage statistics for a single stream.
type StreamStorage struct {
	StreamID     string `json:"stream_id"`
	StreamName   string `json:"stream_name"`
	TotalBytes   int64  `json:"total_bytes"`
	SegmentCount int64  `json:"segment_count"`
}

// GetStoragePerStream returns total storage used and segment count per stream
// for a given camera.
func (d *DB) GetStoragePerStream(cameraID string) ([]StreamStorage, error) {
	rows, err := d.Query(`
		SELECT r.stream_id, COALESCE(cs.name, ''), COALESCE(SUM(r.file_size), 0), COUNT(*)
		FROM recordings r
		LEFT JOIN camera_streams cs ON cs.id = r.stream_id
		WHERE r.camera_id = ?
		GROUP BY r.stream_id
		ORDER BY COALESCE(cs.name, '')`, cameraID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []StreamStorage
	for rows.Next() {
		var ss StreamStorage
		if err := rows.Scan(&ss.StreamID, &ss.StreamName, &ss.TotalBytes, &ss.SegmentCount); err != nil {
			return nil, err
		}
		results = append(results, ss)
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

// UpdateRecordingFilePath updates the file_path column for a recording by ID.
// Returns ErrNotFound if no matching record exists.
func (d *DB) UpdateRecordingFilePath(id int64, filePath string) error {
	res, err := d.Exec("UPDATE recordings SET file_path = ? WHERE id = ?", filePath, id)
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

// QueryRecordingsBestQuality returns recordings for a camera in a time range,
// preferring higher-resolution streams when recordings from multiple streams
// overlap. For overlapping periods, only the recording from the stream with
// the largest resolution (width * height) is returned.
func (d *DB) QueryRecordingsBestQuality(cameraID string, start, end time.Time) ([]*Recording, error) {
	all, err := d.QueryRecordings(cameraID, start, end)
	if err != nil {
		return nil, err
	}
	if len(all) <= 1 {
		return all, nil
	}

	// Build resolution lookup from camera_streams.
	type streamRes struct {
		width, height int
	}
	streamResolutions := make(map[string]streamRes)
	rows, err := d.Query(`SELECT id, width, height FROM camera_streams WHERE camera_id = ?`, cameraID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var id string
			var w, h int
			if rows.Scan(&id, &w, &h) == nil {
				streamResolutions[id] = streamRes{w, h}
			}
		}
	}

	// Score each recording by resolution. Main stream (no ~ in path) gets
	// max resolution; sub-streams get their actual resolution.
	resolutionOf := func(rec *Recording) int {
		idx := strings.LastIndex(rec.FilePath, "~")
		if idx < 0 {
			return 9999 * 9999 // main stream: highest priority
		}
		suffix := rec.FilePath[idx+1:]
		if slashIdx := strings.Index(suffix, "/"); slashIdx > 0 {
			suffix = suffix[:slashIdx]
		}
		for id, res := range streamResolutions {
			if strings.HasPrefix(id, suffix) {
				return res.width * res.height
			}
		}
		return 0
	}

	// Filter: for overlapping recordings, keep highest resolution.
	var result []*Recording
	for _, rec := range all {
		overlaps := false
		for i, existing := range result {
			if existing.EndTime > rec.StartTime && existing.StartTime < rec.EndTime {
				if resolutionOf(rec) > resolutionOf(existing) {
					result[i] = rec
				}
				overlaps = true
				break
			}
		}
		if !overlaps {
			result = append(result, rec)
		}
	}
	return result, nil
}

// UpdateRecordingStatus sets the integrity verification status for a recording.
func (d *DB) UpdateRecordingStatus(id int64, status string, statusDetail *string, verifiedAt string) error {
	_, err := d.Exec(
		"UPDATE recordings SET status = ?, status_detail = ?, verified_at = ? WHERE id = ?",
		status, statusDetail, verifiedAt, id,
	)
	return err
}

// GetAllRecordingPaths returns a map of file_path → recording ID for all recordings.
// Used by the recovery scanner to identify orphaned files on disk.
func (d *DB) GetAllRecordingPaths() (map[string]int64, error) {
	rows, err := d.Query("SELECT id, file_path FROM recordings")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := make(map[string]int64)
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		paths[path] = id
	}
	return paths, rows.Err()
}

// GetUnindexedRecordingPaths returns a map of file_path → recording ID for recordings
// that have no fragment rows and are not quarantined. Used by recovery to find
// recordings that were inserted but never indexed (crash between insert and fragment scan).
func (d *DB) GetUnindexedRecordingPaths() (map[string]int64, error) {
	rows, err := d.Query(`
		SELECT r.id, r.file_path
		FROM recordings r
		LEFT JOIN recording_fragments rf ON rf.recording_id = r.id
		WHERE rf.id IS NULL AND r.status != 'quarantined'`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := make(map[string]int64)
	for rows.Next() {
		var id int64
		var path string
		if err := rows.Scan(&id, &path); err != nil {
			return nil, err
		}
		paths[path] = id
	}
	return paths, rows.Err()
}

// UpdateRecordingFileSize updates the file_size for a recording after repair.
func (d *DB) UpdateRecordingFileSize(id int64, fileSize int64) error {
	_, err := d.Exec("UPDATE recordings SET file_size = ? WHERE id = ?", fileSize, id)
	return err
}

// GetRecordingsNeedingVerification returns recordings that need verification: either status='unverified'
// or verified_at older than the given cutoff. Results are ordered newest-first, limited to batchSize.
func (d *DB) GetRecordingsNeedingVerification(cutoff time.Time, batchSize int) ([]*Recording, error) {
	rows, err := d.Query(`
		SELECT id, camera_id, stream_id, start_time, end_time, duration_ms, file_path, file_size, format, init_size, status, status_detail, verified_at, media_start_time
		FROM recordings
		WHERE status = 'unverified' OR (verified_at IS NOT NULL AND verified_at < ?)
		ORDER BY start_time DESC
		LIMIT ?`,
		cutoff.UTC().Format(timeFormat), batchSize,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []*Recording
	for rows.Next() {
		rec := &Recording{}
		if err := rows.Scan(
			&rec.ID, &rec.CameraID, &rec.StreamID, &rec.StartTime, &rec.EndTime,
			&rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format, &rec.InitSize,
			&rec.Status, &rec.StatusDetail, &rec.VerifiedAt, &rec.MediaStartTime,
		); err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	return recs, rows.Err()
}

// IntegritySummary holds aggregate counts of recording statuses.
type IntegritySummary struct {
	Total       int64 `json:"total"`
	OK          int64 `json:"ok"`
	Corrupted   int64 `json:"corrupted"`
	Quarantined int64 `json:"quarantined"`
	Unverified  int64 `json:"unverified"`
}

// GetIntegritySummary returns aggregate status counts, optionally filtered by camera.
func (d *DB) GetIntegritySummary(cameraID string) (*IntegritySummary, error) {
	var query string
	var args []interface{}
	if cameraID != "" {
		query = `SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'corrupted' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'quarantined' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'unverified' THEN 1 ELSE 0 END)
		FROM recordings WHERE camera_id = ?`
		args = append(args, cameraID)
	} else {
		query = `SELECT
			COUNT(*) as total,
			SUM(CASE WHEN status = 'ok' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'corrupted' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'quarantined' THEN 1 ELSE 0 END),
			SUM(CASE WHEN status = 'unverified' THEN 1 ELSE 0 END)
		FROM recordings`
	}

	s := &IntegritySummary{}
	err := d.QueryRow(query, args...).Scan(&s.Total, &s.OK, &s.Corrupted, &s.Quarantined, &s.Unverified)
	if err != nil {
		return nil, err
	}
	return s, nil
}

// GetRecordingsByFilter returns recordings matching optional camera and time range filters.
func (d *DB) GetRecordingsByFilter(cameraID string, start, end *time.Time) ([]*Recording, error) {
	query := `SELECT id, camera_id, stream_id, start_time, end_time, duration_ms, file_path, file_size, format, init_size, status, status_detail, verified_at, media_start_time FROM recordings WHERE 1=1`
	var args []interface{}

	if cameraID != "" {
		query += " AND camera_id = ?"
		args = append(args, cameraID)
	}
	if start != nil {
		query += " AND end_time > ?"
		args = append(args, start.UTC().Format(timeFormat))
	}
	if end != nil {
		query += " AND start_time < ?"
		args = append(args, end.UTC().Format(timeFormat))
	}
	query += " ORDER BY start_time DESC"

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var recs []*Recording
	for rows.Next() {
		rec := &Recording{}
		if err := rows.Scan(
			&rec.ID, &rec.CameraID, &rec.StreamID, &rec.StartTime, &rec.EndTime,
			&rec.DurationMs, &rec.FilePath, &rec.FileSize, &rec.Format, &rec.InitSize,
			&rec.Status, &rec.StatusDetail, &rec.VerifiedAt, &rec.MediaStartTime,
		); err != nil {
			return nil, err
		}
		recs = append(recs, rec)
	}
	return recs, rows.Err()
}
