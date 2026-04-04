package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// BulkExportJob tracks a batch export containing multiple camera/time-range items.
type BulkExportJob struct {
	ID             string  `json:"id"`
	Status         string  `json:"status"` // pending, processing, completed, failed
	TotalItems     int     `json:"total_items"`
	CompletedItems int     `json:"completed_items"`
	FailedItems    int     `json:"failed_items"`
	ZipPath        *string `json:"zip_path"`
	TotalBytes     int64   `json:"total_bytes"`
	ErrorMessage   *string `json:"error_message"`
	CreatedAt      string  `json:"created_at"`
	CompletedAt    *string `json:"completed_at"`
}

// BulkExportItem is a single camera/time-range entry within a bulk export job.
type BulkExportItem struct {
	ID           string  `json:"id"`
	JobID        string  `json:"job_id"`
	CameraID     string  `json:"camera_id"`
	CameraName   string  `json:"camera_name"`
	StartTime    string  `json:"start_time"`
	EndTime      string  `json:"end_time"`
	Status       string  `json:"status"` // pending, completed, failed
	FileCount    int     `json:"file_count"`
	TotalBytes   int64   `json:"total_bytes"`
	ErrorMessage *string `json:"error_message"`
}

// CreateBulkExportJob inserts a new bulk export job with its items in a single transaction.
func (d *DB) CreateBulkExportJob(job *BulkExportJob, items []*BulkExportItem) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	if job.CreatedAt == "" {
		job.CreatedAt = time.Now().UTC().Format(timeFormat)
	}
	job.Status = "pending"
	job.TotalItems = len(items)

	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		INSERT INTO bulk_export_jobs (id, status, total_items, completed_items, failed_items, total_bytes, created_at)
		VALUES (?, ?, ?, 0, 0, 0, ?)`,
		job.ID, job.Status, job.TotalItems, job.CreatedAt,
	)
	if err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO bulk_export_items (id, job_id, camera_id, camera_name, start_time, end_time, status)
		VALUES (?, ?, ?, ?, ?, ?, 'pending')`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, item := range items {
		if item.ID == "" {
			item.ID = uuid.New().String()
		}
		item.JobID = job.ID
		item.Status = "pending"
		_, err = stmt.Exec(item.ID, item.JobID, item.CameraID, item.CameraName, item.StartTime, item.EndTime)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetBulkExportJob retrieves a bulk export job by ID. Returns ErrNotFound if no match.
func (d *DB) GetBulkExportJob(id string) (*BulkExportJob, error) {
	job := &BulkExportJob{}
	err := d.QueryRow(`
		SELECT id, status, total_items, completed_items, failed_items, zip_path, total_bytes, error_message, created_at, completed_at
		FROM bulk_export_jobs WHERE id = ?`, id,
	).Scan(
		&job.ID, &job.Status, &job.TotalItems, &job.CompletedItems, &job.FailedItems,
		&job.ZipPath, &job.TotalBytes, &job.ErrorMessage, &job.CreatedAt, &job.CompletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

// GetBulkExportItems returns all items for a given job.
func (d *DB) GetBulkExportItems(jobID string) ([]*BulkExportItem, error) {
	rows, err := d.Query(`
		SELECT id, job_id, camera_id, camera_name, start_time, end_time, status, file_count, total_bytes, error_message
		FROM bulk_export_items WHERE job_id = ?
		ORDER BY camera_name, start_time`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*BulkExportItem
	for rows.Next() {
		item := &BulkExportItem{}
		if err := rows.Scan(
			&item.ID, &item.JobID, &item.CameraID, &item.CameraName,
			&item.StartTime, &item.EndTime, &item.Status,
			&item.FileCount, &item.TotalBytes, &item.ErrorMessage,
		); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// UpdateBulkExportItemStatus updates the status and file metadata for a single export item,
// and increments the parent job's completed/failed counters accordingly.
func (d *DB) UpdateBulkExportItemStatus(itemID, status string, fileCount int, totalBytes int64, errMsg *string) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		UPDATE bulk_export_items SET status = ?, file_count = ?, total_bytes = ?, error_message = ?
		WHERE id = ?`,
		status, fileCount, totalBytes, errMsg, itemID,
	)
	if err != nil {
		return err
	}

	// Look up the job_id for counter updates.
	var jobID string
	err = tx.QueryRow("SELECT job_id FROM bulk_export_items WHERE id = ?", itemID).Scan(&jobID)
	if err != nil {
		return err
	}

	switch status {
	case "completed":
		_, err = tx.Exec(`
			UPDATE bulk_export_jobs SET completed_items = completed_items + 1, total_bytes = total_bytes + ?
			WHERE id = ?`, totalBytes, jobID)
	case "failed":
		_, err = tx.Exec(`
			UPDATE bulk_export_jobs SET failed_items = failed_items + 1
			WHERE id = ?`, jobID)
	}
	if err != nil {
		return err
	}

	return tx.Commit()
}

// CompleteBulkExportJob marks a job as completed or failed with optional zip path and error.
func (d *DB) CompleteBulkExportJob(jobID, status string, zipPath *string, errMsg *string) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := d.Exec(`
		UPDATE bulk_export_jobs SET status = ?, zip_path = ?, error_message = ?, completed_at = ?
		WHERE id = ?`,
		status, zipPath, errMsg, now, jobID,
	)
	return err
}

// ListBulkExportJobs returns recent bulk export jobs, newest first.
func (d *DB) ListBulkExportJobs(limit int) ([]*BulkExportJob, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := d.Query(`
		SELECT id, status, total_items, completed_items, failed_items, zip_path, total_bytes, error_message, created_at, completed_at
		FROM bulk_export_jobs
		ORDER BY created_at DESC
		LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*BulkExportJob
	for rows.Next() {
		job := &BulkExportJob{}
		if err := rows.Scan(
			&job.ID, &job.Status, &job.TotalItems, &job.CompletedItems, &job.FailedItems,
			&job.ZipPath, &job.TotalBytes, &job.ErrorMessage, &job.CreatedAt, &job.CompletedAt,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// DeleteBulkExportJob deletes a job and its items. Returns ErrNotFound if no match.
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
