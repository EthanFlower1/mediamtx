package legacydb

import "time"

// ConnectionEvent records a camera connection state transition.
type ConnectionEvent struct {
	ID           int64  `json:"id"`
	CameraID     string `json:"camera_id"`
	State        string `json:"state"`
	ErrorMessage string `json:"error_message,omitempty"`
	CreatedAt    string `json:"created_at"`
}

// InsertConnectionEvent records a connection state change.
func (d *DB) InsertConnectionEvent(cameraID, state, errMsg string) error {
	_, err := d.Exec(
		`INSERT INTO connection_events (camera_id, state, error_message, created_at)
		 VALUES (?, ?, ?, ?)`,
		cameraID, state, errMsg, time.Now().UTC().Format("2006-01-02T15:04:05.000Z"),
	)
	return err
}

// ListConnectionEvents returns connection history for a camera, most recent first.
// limit=0 returns all events.
func (d *DB) ListConnectionEvents(cameraID string, limit int) ([]*ConnectionEvent, error) {
	query := `SELECT id, camera_id, state, COALESCE(error_message, ''), created_at
	          FROM connection_events
	          WHERE camera_id = ?
	          ORDER BY created_at DESC`
	args := []interface{}{cameraID}
	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}
	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var events []*ConnectionEvent
	for rows.Next() {
		e := &ConnectionEvent{}
		if err := rows.Scan(&e.ID, &e.CameraID, &e.State, &e.ErrorMessage, &e.CreatedAt); err != nil {
			return nil, err
		}
		events = append(events, e)
	}
	return events, rows.Err()
}

// PruneConnectionEvents removes events older than the given duration.
func (d *DB) PruneConnectionEvents(maxAge time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-maxAge).Format("2006-01-02T15:04:05.000Z")
	result, err := d.Exec(`DELETE FROM connection_events WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ConnectionSummary provides uptime statistics for a camera.
type ConnectionSummary struct {
	CameraID       string `json:"camera_id"`
	TotalEvents    int    `json:"total_events"`
	LastState      string `json:"last_state"`
	LastChangeAt   string `json:"last_change_at,omitempty"`
	ErrorCount     int    `json:"error_count"`
	ConnectedCount int    `json:"connected_count"`
}

// GetConnectionSummary returns aggregate connection stats for a camera.
func (d *DB) GetConnectionSummary(cameraID string) (*ConnectionSummary, error) {
	s := &ConnectionSummary{CameraID: cameraID}

	err := d.QueryRow(
		`SELECT COUNT(*) FROM connection_events WHERE camera_id = ?`,
		cameraID,
	).Scan(&s.TotalEvents)
	if err != nil {
		return nil, err
	}

	_ = d.QueryRow(
		`SELECT state, created_at FROM connection_events
		 WHERE camera_id = ? ORDER BY created_at DESC LIMIT 1`,
		cameraID,
	).Scan(&s.LastState, &s.LastChangeAt)

	_ = d.QueryRow(
		`SELECT COUNT(*) FROM connection_events WHERE camera_id = ? AND state = 'error'`,
		cameraID,
	).Scan(&s.ErrorCount)

	_ = d.QueryRow(
		`SELECT COUNT(*) FROM connection_events WHERE camera_id = ? AND state = 'connected'`,
		cameraID,
	).Scan(&s.ConnectedCount)

	return s, nil
}
