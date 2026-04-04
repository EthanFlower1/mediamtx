package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// BulkExportJob represents a bulk export job.
type BulkExportJob struct {
	ID          string  `json:"id"`
	Status      string  `json:"status"`
	ZipPath     *string `json:"zip_path,omitempty"`
	Error       *string `json:"error,omitempty"`
	CreatedAt   string  `json:"created_at"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

// BulkExportItem represents a single item in a bulk export job.
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

// CreateBulkExportJob creates a new bulk export job and its items.
func (d *DB) CreateBulkExportJob(job *BulkExportJob, items []*BulkExportItem) error {
	job.ID = uuid.New().String()
	job.Status = "pending"
	job.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`INSERT INTO bulk_export_jobs (id, status, created_at) VALUES (?, ?, ?)`,
		job.ID, job.Status, job.CreatedAt)
	if err != nil {
		return err
	}

	for _, item := range items {
		item.ID = uuid.New().String()
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
	var job BulkExportJob
	err := d.QueryRow(`SELECT id, status, zip_path, error, created_at, completed_at FROM bulk_export_jobs WHERE id = ?`, id).
		Scan(&job.ID, &job.Status, &job.ZipPath, &job.Error, &job.CreatedAt, &job.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return &job, nil
}

// GetBulkExportItems retrieves all items for a bulk export job.
func (d *DB) GetBulkExportItems(jobID string) ([]*BulkExportItem, error) {
	rows, err := d.Query(`SELECT id, job_id, camera_id, camera_name, start_time, end_time, status, file_count, total_bytes, error FROM bulk_export_items WHERE job_id = ?`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*BulkExportItem
	for rows.Next() {
		var item BulkExportItem
		if err := rows.Scan(&item.ID, &item.JobID, &item.CameraID, &item.CameraName, &item.StartTime, &item.EndTime, &item.Status, &item.FileCount, &item.TotalBytes, &item.Error); err != nil {
			return nil, err
		}
		items = append(items, &item)
	}
	return items, rows.Err()
}

// ListBulkExportJobs returns the most recent bulk export jobs.
func (d *DB) ListBulkExportJobs(limit int) ([]*BulkExportJob, error) {
	rows, err := d.Query(`SELECT id, status, zip_path, error, created_at, completed_at FROM bulk_export_jobs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*BulkExportJob
	for rows.Next() {
		var job BulkExportJob
		if err := rows.Scan(&job.ID, &job.Status, &job.ZipPath, &job.Error, &job.CreatedAt, &job.CompletedAt); err != nil {
			return nil, err
		}
		jobs = append(jobs, &job)
	}
	return jobs, rows.Err()
}

// CompleteBulkExportJob updates a job's status, zip path, and error.
func (d *DB) CompleteBulkExportJob(jobID, status string, zipPath, errMsg *string) error {
	completedAt := time.Now().UTC().Format(time.RFC3339)
	_, err := d.Exec(`UPDATE bulk_export_jobs SET status = ?, zip_path = ?, error = ?, completed_at = ? WHERE id = ?`,
		status, zipPath, errMsg, completedAt, jobID)
	return err
}

// UpdateBulkExportItemStatus updates an item's status, file count, and bytes.
func (d *DB) UpdateBulkExportItemStatus(itemID, status string, fileCount int, totalBytes int64, errMsg *string) error {
	_, err := d.Exec(`UPDATE bulk_export_items SET status = ?, file_count = ?, total_bytes = ?, error = ? WHERE id = ?`,
		status, fileCount, totalBytes, errMsg, itemID)
	return err
}

// DeleteBulkExportJob deletes a job and its items.
func (d *DB) DeleteBulkExportJob(jobID string) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(`DELETE FROM bulk_export_items WHERE job_id = ?`, jobID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM bulk_export_jobs WHERE id = ?`, jobID); err != nil {
		return err
	}
	return tx.Commit()
}
