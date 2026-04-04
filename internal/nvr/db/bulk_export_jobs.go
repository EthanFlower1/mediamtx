package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// BulkExportJob represents an asynchronous bulk export job.
type BulkExportJob struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	ZipPath     *string `json:"zip_path,omitempty"`
	ErrorMsg    *string `json:"error,omitempty"`
	TotalItems  int     `json:"total_items"`
	CreatedAt   string  `json:"created_at"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

// BulkExportItem represents a single camera/time-range within a bulk export job.
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
	ErrorMsg   *string `json:"error,omitempty"`
}

// CreateBulkExportJob inserts a new bulk export job and its items.
func (d *DB) CreateBulkExportJob(job *BulkExportJob, items []*BulkExportItem) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	job.Status = "pending"
	job.TotalItems = len(items)
	job.CreatedAt = time.Now().UTC().Format(timeFormat)

	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	_, err = tx.Exec(`INSERT INTO bulk_export_jobs (id, status, total_items, created_at) VALUES (?, ?, ?, ?)`,
		job.ID, job.Status, job.TotalItems, job.CreatedAt)
	if err != nil {
		return err
	}

	for _, item := range items {
		if item.ID == "" {
			item.ID = uuid.New().String()
		}
		item.JobID = job.ID
		item.Status = "pending"
		_, err = tx.Exec(`INSERT INTO bulk_export_items (id, job_id, camera_id, camera_name, start_time, end_time, status) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			item.ID, item.JobID, item.CameraID, item.CameraName, item.StartTime, item.EndTime, item.Status)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetBulkExportJob retrieves a bulk export job by ID.
func (d *DB) GetBulkExportJob(id string) (*BulkExportJob, error) {
	job := &BulkExportJob{}
	err := d.QueryRow(`SELECT id, status, zip_path, error_msg, total_items, created_at, completed_at FROM bulk_export_jobs WHERE id = ?`, id).
		Scan(&job.ID, &job.Status, &job.ZipPath, &job.ErrorMsg, &job.TotalItems, &job.CreatedAt, &job.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

// GetBulkExportItems retrieves all items for a given bulk export job.
func (d *DB) GetBulkExportItems(jobID string) ([]*BulkExportItem, error) {
	rows, err := d.Query(`SELECT id, job_id, camera_id, camera_name, start_time, end_time, status, file_count, total_bytes, error_msg FROM bulk_export_items WHERE job_id = ?`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*BulkExportItem
	for rows.Next() {
		item := &BulkExportItem{}
		if err := rows.Scan(&item.ID, &item.JobID, &item.CameraID, &item.CameraName, &item.StartTime, &item.EndTime, &item.Status, &item.FileCount, &item.TotalBytes, &item.ErrorMsg); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListBulkExportJobs returns recent bulk export jobs ordered by creation time.
func (d *DB) ListBulkExportJobs(limit int) ([]*BulkExportJob, error) {
	rows, err := d.Query(`SELECT id, status, zip_path, error_msg, total_items, created_at, completed_at FROM bulk_export_jobs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*BulkExportJob
	for rows.Next() {
		job := &BulkExportJob{}
		if err := rows.Scan(&job.ID, &job.Status, &job.ZipPath, &job.ErrorMsg, &job.TotalItems, &job.CreatedAt, &job.CompletedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// CompleteBulkExportJob updates the status and optionally the zip path or error of a bulk export job.
func (d *DB) CompleteBulkExportJob(id, status string, zipPath *string, errMsg *string) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := d.Exec(`UPDATE bulk_export_jobs SET status = ?, zip_path = ?, error_msg = ?, completed_at = ? WHERE id = ?`,
		status, zipPath, errMsg, now, id)
	return err
}

// UpdateBulkExportItemStatus updates the status of a single bulk export item.
func (d *DB) UpdateBulkExportItemStatus(id, status string, fileCount int, totalBytes int64, errMsg *string) error {
	_, err := d.Exec(`UPDATE bulk_export_items SET status = ?, file_count = ?, total_bytes = ?, error_msg = ? WHERE id = ?`,
		status, fileCount, totalBytes, errMsg, id)
	return err
}

// DeleteBulkExportJob deletes a bulk export job and its items.
func (d *DB) DeleteBulkExportJob(id string) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`DELETE FROM bulk_export_items WHERE job_id = ?`, id); err != nil {
		return err
	}
	res, err := tx.Exec(`DELETE FROM bulk_export_jobs WHERE id = ?`, id)
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
	return tx.Commit()
}
