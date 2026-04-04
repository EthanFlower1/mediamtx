package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// BulkExportJob represents a bulk export operation spanning multiple cameras.
type BulkExportJob struct {
	ID        string  `json:"id"`
	Status    string  `json:"status"`
	ZipPath   *string `json:"zip_path,omitempty"`
	Error     *string `json:"error,omitempty"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

// BulkExportItem represents a single camera time range within a bulk export.
type BulkExportItem struct {
	ID         string  `json:"id"`
	JobID      string  `json:"job_id"`
	CameraID   string  `json:"camera_id"`
	CameraName string  `json:"camera_name"`
	StartTime  string  `json:"start_time"`
	EndTime    string  `json:"end_time"`
	Status     string  `json:"status"`
	FileCount  int     `json:"file_count"`
	TotalBytes int64   `json:"total_bytes"`
	Error      *string `json:"error,omitempty"`
}

// CreateBulkExportJob creates a new bulk export job with its items in a single
// transaction. Both job.ID and each item's ID are generated if empty.
func (d *DB) CreateBulkExportJob(job *BulkExportJob, items []*BulkExportItem) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(timeFormat)
	job.Status = "pending"
	job.CreatedAt = now
	job.UpdatedAt = now

	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO bulk_export_jobs (id, status, created_at, updated_at)
		VALUES (?, ?, ?, ?)`,
		job.ID, job.Status, job.CreatedAt, job.UpdatedAt,
	)
	if err != nil {
		return err
	}

	for _, item := range items {
		if item.ID == "" {
			item.ID = uuid.New().String()
		}
		item.JobID = job.ID
		item.Status = "pending"

		_, err = tx.Exec(`
			INSERT INTO bulk_export_items (id, job_id, camera_id, camera_name, start_time, end_time, status)
			VALUES (?, ?, ?, ?, ?, ?, ?)`,
			item.ID, item.JobID, item.CameraID, item.CameraName,
			item.StartTime, item.EndTime, item.Status,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetBulkExportJob retrieves a bulk export job by ID. Returns ErrNotFound if
// no match.
func (d *DB) GetBulkExportJob(id string) (*BulkExportJob, error) {
	j := &BulkExportJob{}
	err := d.QueryRow(`
		SELECT id, status, zip_path, error, created_at, updated_at
		FROM bulk_export_jobs WHERE id = ?`, id,
	).Scan(&j.ID, &j.Status, &j.ZipPath, &j.Error, &j.CreatedAt, &j.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return j, err
}

// GetBulkExportItems returns all items for a given job ID.
func (d *DB) GetBulkExportItems(jobID string) ([]*BulkExportItem, error) {
	rows, err := d.Query(`
		SELECT id, job_id, camera_id, camera_name, start_time, end_time, status,
			file_count, total_bytes, error
		FROM bulk_export_items WHERE job_id = ?`, jobID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*BulkExportItem
	for rows.Next() {
		item := &BulkExportItem{}
		if err := rows.Scan(&item.ID, &item.JobID, &item.CameraID, &item.CameraName,
			&item.StartTime, &item.EndTime, &item.Status,
			&item.FileCount, &item.TotalBytes, &item.Error); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListBulkExportJobs returns up to limit recent jobs ordered by creation time.
func (d *DB) ListBulkExportJobs(limit int) ([]*BulkExportJob, error) {
	rows, err := d.Query(`
		SELECT id, status, zip_path, error, created_at, updated_at
		FROM bulk_export_jobs ORDER BY created_at DESC LIMIT ?`, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*BulkExportJob
	for rows.Next() {
		j := &BulkExportJob{}
		if err := rows.Scan(&j.ID, &j.Status, &j.ZipPath, &j.Error,
			&j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, j)
	}
	return jobs, rows.Err()
}

// DeleteBulkExportJob deletes a job and its items (via CASCADE).
func (d *DB) DeleteBulkExportJob(id string) error {
	res, err := d.Exec("DELETE FROM bulk_export_jobs WHERE id = ?", id)
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

// CompleteBulkExportJob updates the job status, zip path, and error.
func (d *DB) CompleteBulkExportJob(id, status string, zipPath, errMsg *string) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := d.Exec(`
		UPDATE bulk_export_jobs SET status = ?, zip_path = ?, error = ?, updated_at = ?
		WHERE id = ?`,
		status, zipPath, errMsg, now, id,
	)
	return err
}

// UpdateBulkExportItemStatus updates a single item's status, file count,
// total bytes, and optional error message.
func (d *DB) UpdateBulkExportItemStatus(id, status string, fileCount int, totalBytes int64, errMsg *string) error {
	_, err := d.Exec(`
		UPDATE bulk_export_items SET status = ?, file_count = ?, total_bytes = ?, error = ?
		WHERE id = ?`,
		status, fileCount, totalBytes, errMsg, id,
	)
	return err
}
