package db

import (
	"database/sql"
	"errors"
	"time"
)

// StorageQuota represents a global or per-path storage quota.
type StorageQuota struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	QuotaBytes     int64  `json:"quota_bytes"`
	WarningPercent int    `json:"warning_percent"`
	CriticalPercent int   `json:"critical_percent"`
	Enabled        bool   `json:"enabled"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// QuotaStatus represents the current quota status for a camera or global scope.
type QuotaStatus struct {
	CameraID       string  `json:"camera_id,omitempty"`
	CameraName     string  `json:"camera_name,omitempty"`
	QuotaBytes     int64   `json:"quota_bytes"`
	UsedBytes      int64   `json:"used_bytes"`
	UsedPercent    float64 `json:"used_percent"`
	Status         string  `json:"status"` // "ok", "warning", "critical", "exceeded"
	WarningPercent int     `json:"warning_percent"`
	CriticalPercent int    `json:"critical_percent"`
}

// UpsertStorageQuota creates or updates a storage quota row.
func (d *DB) UpsertStorageQuota(q *StorageQuota) error {
	now := time.Now().UTC().Format(timeFormat)
	if q.WarningPercent == 0 {
		q.WarningPercent = 80
	}
	if q.CriticalPercent == 0 {
		q.CriticalPercent = 90
	}

	_, err := d.Exec(`
		INSERT INTO storage_quotas (id, name, quota_bytes, warning_percent, critical_percent, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			name = excluded.name,
			quota_bytes = excluded.quota_bytes,
			warning_percent = excluded.warning_percent,
			critical_percent = excluded.critical_percent,
			enabled = excluded.enabled,
			updated_at = excluded.updated_at`,
		q.ID, q.Name, q.QuotaBytes, q.WarningPercent, q.CriticalPercent, q.Enabled, now, now)
	return err
}

// GetStorageQuota retrieves a storage quota by ID.
func (d *DB) GetStorageQuota(id string) (*StorageQuota, error) {
	q := &StorageQuota{}
	err := d.QueryRow(`
		SELECT id, name, quota_bytes, warning_percent, critical_percent, enabled, created_at, updated_at
		FROM storage_quotas WHERE id = ?`, id).Scan(
		&q.ID, &q.Name, &q.QuotaBytes, &q.WarningPercent, &q.CriticalPercent, &q.Enabled, &q.CreatedAt, &q.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return q, nil
}

// ListStorageQuotas returns all storage quotas.
func (d *DB) ListStorageQuotas() ([]*StorageQuota, error) {
	rows, err := d.Query(`
		SELECT id, name, quota_bytes, warning_percent, critical_percent, enabled, created_at, updated_at
		FROM storage_quotas ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var quotas []*StorageQuota
	for rows.Next() {
		q := &StorageQuota{}
		if err := rows.Scan(&q.ID, &q.Name, &q.QuotaBytes, &q.WarningPercent, &q.CriticalPercent, &q.Enabled, &q.CreatedAt, &q.UpdatedAt); err != nil {
			return nil, err
		}
		quotas = append(quotas, q)
	}
	return quotas, rows.Err()
}

// DeleteStorageQuota deletes a storage quota by ID.
func (d *DB) DeleteStorageQuota(id string) error {
	res, err := d.Exec(`DELETE FROM storage_quotas WHERE id = ?`, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetCameraStorageUsage returns the total bytes used by a specific camera.
func (d *DB) GetCameraStorageUsage(cameraID string) (int64, error) {
	var total int64
	err := d.QueryRow(`
		SELECT COALESCE(SUM(file_size), 0) FROM recordings WHERE camera_id = ?`,
		cameraID).Scan(&total)
	return total, err
}

// GetTotalStorageUsage returns total bytes used across all recordings.
func (d *DB) GetTotalStorageUsage() (int64, error) {
	var total int64
	err := d.QueryRow(`SELECT COALESCE(SUM(file_size), 0) FROM recordings`).Scan(&total)
	return total, err
}

// DeleteOldestRecordingsWithoutEvents deletes the oldest recordings (by end_time)
// for a camera that have no overlapping motion events, up to bytesToFree bytes.
// Returns the file paths of deleted recordings for disk cleanup.
func (d *DB) DeleteOldestRecordingsWithoutEvents(cameraID string, bytesToFree int64) ([]string, int64, error) {
	rows, err := d.Query(`
		SELECT r.id, r.file_path, r.file_size FROM recordings r
		WHERE r.camera_id = ?
		AND NOT EXISTS (
			SELECT 1 FROM motion_events me
			WHERE me.camera_id = r.camera_id
			AND me.started_at < r.end_time
			AND (me.ended_at IS NULL OR me.ended_at > r.start_time)
		)
		ORDER BY r.end_time ASC`, cameraID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var ids []string
	var paths []string
	var freed int64

	for rows.Next() && freed < bytesToFree {
		var id, path string
		var size int64
		if err := rows.Scan(&id, &path, &size); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
		paths = append(paths, path)
		freed += size
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	for _, id := range ids {
		if _, err := d.Exec(`DELETE FROM recordings WHERE id = ?`, id); err != nil {
			return paths, freed, err
		}
	}

	return paths, freed, nil
}

// DeleteOldestRecordingsWithEvents deletes the oldest recordings (by end_time)
// for a camera that DO have overlapping motion events, up to bytesToFree bytes.
// Returns the file paths of deleted recordings for disk cleanup.
func (d *DB) DeleteOldestRecordingsWithEvents(cameraID string, bytesToFree int64) ([]string, int64, error) {
	rows, err := d.Query(`
		SELECT r.id, r.file_path, r.file_size FROM recordings r
		WHERE r.camera_id = ?
		AND EXISTS (
			SELECT 1 FROM motion_events me
			WHERE me.camera_id = r.camera_id
			AND me.started_at < r.end_time
			AND (me.ended_at IS NULL OR me.ended_at > r.start_time)
		)
		ORDER BY r.end_time ASC`, cameraID)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var ids []string
	var paths []string
	var freed int64

	for rows.Next() && freed < bytesToFree {
		var id, path string
		var size int64
		if err := rows.Scan(&id, &path, &size); err != nil {
			return nil, 0, err
		}
		ids = append(ids, id)
		paths = append(paths, path)
		freed += size
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	for _, id := range ids {
		if _, err := d.Exec(`DELETE FROM recordings WHERE id = ?`, id); err != nil {
			return paths, freed, err
		}
	}

	return paths, freed, nil
}

// UpdateCameraQuota updates only the quota-related fields of a camera.
func (d *DB) UpdateCameraQuota(id string, quotaBytes int64, warningPercent, criticalPercent int) error {
	updatedAt := time.Now().UTC().Format(timeFormat)
	res, err := d.Exec(`
		UPDATE cameras SET quota_bytes = ?, quota_warning_percent = ?, quota_critical_percent = ?, updated_at = ?
		WHERE id = ?`,
		quotaBytes, warningPercent, criticalPercent, updatedAt, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
