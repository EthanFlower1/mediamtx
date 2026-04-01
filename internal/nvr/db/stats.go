package db

// RecordingStats holds aggregate recording statistics for a single camera.
type RecordingStats struct {
	CameraID        string `json:"camera_id"`
	CameraName      string `json:"camera_name"`
	TotalBytes      int64  `json:"total_bytes"`
	SegmentCount    int64  `json:"segment_count"`
	TotalRecordedMs int64  `json:"total_recorded_ms"`
	OldestRecording string `json:"oldest_recording"`
	NewestRecording string `json:"newest_recording"`
}

// GetRecordingStats returns aggregate recording statistics per camera.
// If cameraID is non-empty, results are filtered to that camera only.
func (d *DB) GetRecordingStats(cameraID string) ([]RecordingStats, error) {
	query := `
		SELECT r.camera_id, COALESCE(c.name, ''),
			COALESCE(SUM(r.file_size), 0),
			COUNT(*),
			COALESCE(SUM(r.duration_ms), 0),
			MIN(r.start_time),
			MAX(r.end_time)
		FROM recordings r
		LEFT JOIN cameras c ON c.id = r.camera_id`

	var args []interface{}
	if cameraID != "" {
		query += " WHERE r.camera_id = ?"
		args = append(args, cameraID)
	}
	query += " GROUP BY r.camera_id ORDER BY COALESCE(c.name, '')"

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []RecordingStats
	for rows.Next() {
		var s RecordingStats
		if err := rows.Scan(&s.CameraID, &s.CameraName, &s.TotalBytes,
			&s.SegmentCount, &s.TotalRecordedMs, &s.OldestRecording, &s.NewestRecording); err != nil {
			return nil, err
		}
		results = append(results, s)
	}
	return results, rows.Err()
}
