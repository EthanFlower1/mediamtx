package legacydb

import (
	"database/sql"
	"errors"
	"time"
)

// PendingSync represents a file sync operation that is pending, in-progress, or failed.
type PendingSync struct {
	ID            int64  `json:"id"`
	RecordingID   int64  `json:"recording_id"`
	CameraID      string `json:"camera_id"`
	LocalPath     string `json:"local_path"`
	TargetPath    string `json:"target_path"`
	Status        string `json:"status"`
	Attempts      int    `json:"attempts"`
	ErrorMessage  string `json:"error_message"`
	CreatedAt     string `json:"created_at"`
	LastAttemptAt string `json:"last_attempt_at"`
}

// InsertPendingSync inserts a new pending sync record. Defaults Status to "pending",
// sets CreatedAt to now, and populates ps.ID with the auto-generated value.
func (d *DB) InsertPendingSync(ps *PendingSync) error {
	if ps.Status == "" {
		ps.Status = "pending"
	}
	ps.CreatedAt = time.Now().UTC().Format(timeFormat)

	res, err := d.Exec(`
		INSERT INTO pending_syncs (recording_id, camera_id, local_path, target_path, status, attempts, error_message, created_at, last_attempt_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		ps.RecordingID, ps.CameraID, ps.LocalPath, ps.TargetPath,
		ps.Status, ps.Attempts, ps.ErrorMessage, ps.CreatedAt, ps.LastAttemptAt,
	)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	ps.ID = id
	return nil
}

// ListPendingSyncs returns all pending sync records with the given status,
// ordered by created_at ASC. Nullable fields are handled with COALESCE.
func (d *DB) ListPendingSyncs(status string) ([]*PendingSync, error) {
	rows, err := d.Query(`
		SELECT id, recording_id, camera_id, local_path, target_path, status, attempts,
			COALESCE(error_message, ''), created_at, COALESCE(last_attempt_at, '')
		FROM pending_syncs
		WHERE status = ?
		ORDER BY created_at ASC`, status)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var syncs []*PendingSync
	for rows.Next() {
		ps := &PendingSync{}
		if err := rows.Scan(
			&ps.ID, &ps.RecordingID, &ps.CameraID, &ps.LocalPath, &ps.TargetPath,
			&ps.Status, &ps.Attempts, &ps.ErrorMessage, &ps.CreatedAt, &ps.LastAttemptAt,
		); err != nil {
			return nil, err
		}
		syncs = append(syncs, ps)
	}
	return syncs, rows.Err()
}

// SetPendingSyncStatus updates the status of a pending sync record by ID.
// It does not increment the attempt counter.
func (d *DB) SetPendingSyncStatus(id int64, status string) error {
	res, err := d.Exec("UPDATE pending_syncs SET status = ? WHERE id = ?", status, id)
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

// RecordPendingSyncFailure increments the attempt counter, sets the status and
// error message, and records the last_attempt_at timestamp.
func (d *DB) RecordPendingSyncFailure(id int64, status, errorMsg string) error {
	now := time.Now().UTC().Format(timeFormat)
	res, err := d.Exec(`
		UPDATE pending_syncs
		SET status = ?, error_message = ?, attempts = attempts + 1, last_attempt_at = ?
		WHERE id = ?`,
		status, errorMsg, now, id,
	)
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

// GetPendingSync retrieves a single pending sync record by ID.
// Returns ErrNotFound if no record exists.
func (d *DB) GetPendingSync(id int64) (*PendingSync, error) {
	ps := &PendingSync{}
	err := d.QueryRow(`
		SELECT id, recording_id, camera_id, local_path, target_path, status, attempts,
			COALESCE(error_message, ''), created_at, COALESCE(last_attempt_at, '')
		FROM pending_syncs WHERE id = ?`, id,
	).Scan(
		&ps.ID, &ps.RecordingID, &ps.CameraID, &ps.LocalPath, &ps.TargetPath,
		&ps.Status, &ps.Attempts, &ps.ErrorMessage, &ps.CreatedAt, &ps.LastAttemptAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return ps, nil
}

// DeletePendingSync deletes a pending sync record by ID.
// Returns ErrNotFound if no record exists.
func (d *DB) DeletePendingSync(id int64) error {
	res, err := d.Exec("DELETE FROM pending_syncs WHERE id = ?", id)
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

// FileIsReferenced returns true if the given file path is referenced by any
// recording (via file_path) or pending sync (via local_path).
func (d *DB) FileIsReferenced(filePath string) bool {
	var count int
	err := d.QueryRow(`
		SELECT COUNT(*) FROM (
			SELECT 1 FROM recordings WHERE file_path = ?
			UNION ALL
			SELECT 1 FROM pending_syncs WHERE local_path = ?
		)`, filePath, filePath,
	).Scan(&count)
	if err != nil {
		return false
	}
	return count > 0
}

// PendingSyncCountByCamera returns a map of camera_id to count of pending sync
// records where status is 'pending' or 'syncing'.
func (d *DB) PendingSyncCountByCamera() (map[string]int, error) {
	rows, err := d.Query(`
		SELECT camera_id, COUNT(*)
		FROM pending_syncs
		WHERE status IN ('pending', 'syncing')
		GROUP BY camera_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := make(map[string]int)
	for rows.Next() {
		var cameraID string
		var count int
		if err := rows.Scan(&cameraID, &count); err != nil {
			return nil, err
		}
		counts[cameraID] = count
	}
	return counts, rows.Err()
}

// ListPendingSyncsByCamera returns all pending sync records for a specific camera
// where status is 'pending', ordered by created_at ASC.
func (d *DB) ListPendingSyncsByCamera(cameraID string) ([]*PendingSync, error) {
	rows, err := d.Query(`
		SELECT id, recording_id, camera_id, local_path, target_path, status, attempts,
			COALESCE(error_message, ''), created_at, COALESCE(last_attempt_at, '')
		FROM pending_syncs
		WHERE camera_id = ? AND status = 'pending'
		ORDER BY created_at ASC`, cameraID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var syncs []*PendingSync
	for rows.Next() {
		ps := &PendingSync{}
		if err := rows.Scan(
			&ps.ID, &ps.RecordingID, &ps.CameraID, &ps.LocalPath, &ps.TargetPath,
			&ps.Status, &ps.Attempts, &ps.ErrorMessage, &ps.CreatedAt, &ps.LastAttemptAt,
		); err != nil {
			return nil, err
		}
		syncs = append(syncs, ps)
	}
	return syncs, rows.Err()
}
