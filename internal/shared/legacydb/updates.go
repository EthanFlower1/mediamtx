package legacydb

import "time"

// UpdateRecord represents a single entry in the system update history.
type UpdateRecord struct {
	ID                int    `json:"id"`
	FromVersion       string `json:"from_version"`
	ToVersion         string `json:"to_version"`
	Status            string `json:"status"` // pending, downloading, applying, completed, failed, rolled_back
	StartedAt         string `json:"started_at"`
	CompletedAt       string `json:"completed_at,omitempty"`
	ErrorMessage      string `json:"error_message,omitempty"`
	InitiatedBy       string `json:"initiated_by"`
	SHA256Checksum    string `json:"sha256_checksum,omitempty"`
	RollbackAvailable bool   `json:"rollback_available"`
}

// InsertUpdateRecord creates a new update history entry and returns its ID.
func (d *DB) InsertUpdateRecord(rec *UpdateRecord) (int64, error) {
	result, err := d.Exec(`
		INSERT INTO update_history (from_version, to_version, status, started_at, initiated_by, sha256_checksum, rollback_available)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		rec.FromVersion, rec.ToVersion, rec.Status, rec.StartedAt,
		rec.InitiatedBy, rec.SHA256Checksum, boolToInt(rec.RollbackAvailable),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// UpdateUpdateRecord updates the status, error, and completion time of an update record.
func (d *DB) UpdateUpdateRecord(id int64, status string, errMsg string, rollbackAvailable bool) error {
	completedAt := ""
	if status == "completed" || status == "failed" || status == "rolled_back" {
		completedAt = time.Now().UTC().Format(time.RFC3339)
	}
	_, err := d.Exec(`
		UPDATE update_history
		SET status = ?, error_message = ?, completed_at = ?, rollback_available = ?
		WHERE id = ?`,
		status, errMsg, completedAt, boolToInt(rollbackAvailable), id,
	)
	return err
}

// ListUpdateHistory returns update history entries, most recent first.
func (d *DB) ListUpdateHistory(limit int) ([]*UpdateRecord, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.Query(`
		SELECT id, from_version, to_version, status, started_at,
		       COALESCE(completed_at, ''), COALESCE(error_message, ''),
		       initiated_by, sha256_checksum, rollback_available
		FROM update_history
		ORDER BY started_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var records []*UpdateRecord
	for rows.Next() {
		var r UpdateRecord
		var rb int
		if err := rows.Scan(&r.ID, &r.FromVersion, &r.ToVersion, &r.Status,
			&r.StartedAt, &r.CompletedAt, &r.ErrorMessage,
			&r.InitiatedBy, &r.SHA256Checksum, &rb); err != nil {
			return nil, err
		}
		r.RollbackAvailable = rb != 0
		records = append(records, &r)
	}
	if records == nil {
		records = []*UpdateRecord{}
	}
	return records, rows.Err()
}

// GetLatestUpdateRecord returns the most recent update record, or nil if none exist.
func (d *DB) GetLatestUpdateRecord() (*UpdateRecord, error) {
	records, err := d.ListUpdateHistory(1)
	if err != nil {
		return nil, err
	}
	if len(records) == 0 {
		return nil, nil
	}
	return records[0], nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
