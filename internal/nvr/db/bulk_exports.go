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
	TotalItems  int     `json:"total_items"`
	ZipPath     *string `json:"zip_path,omitempty"`
	Error       *string `json:"error,omitempty"`
	CreatedAt   string  `json:"created_at"`
	CompletedAt *string `json:"completed_at,omitempty"`
}

// BulkExportItem represents one camera/time-range within a bulk export job.
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

// CreateBulkExportJob inserts a new bulk export job and its items.
func (d *DB) CreateBulkExportJob(job *BulkExportJob, items []*BulkExportItem) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(timeFormat)
	job.CreatedAt = now
	job.Status = "pending"
	job.TotalItems = len(items)

	tx, err := d.Begin()
	if err != nil {
		return err
	}

	_, err = tx.Exec(`
		INSERT INTO bulk_export_jobs (id, status, total_items, created_at)
		VALUES (?, ?, ?, ?)`,
		job.ID, job.Status, job.TotalItems, job.CreatedAt,
	)
	if err != nil {
		tx.Rollback()
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
			item.ID, item.JobID, item.CameraID, item.CameraName, item.StartTime, item.EndTime, item.Status,
		)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	return tx.Commit()
}

// GetBulkExportJob retrieves a bulk export job by ID.
func (d *DB) GetBulkExportJob(id string) (*BulkExportJob, error) {
	job := &BulkExportJob{}
	var zipPath, errMsg, completedAt sql.NullString
	err := d.QueryRow(`
		SELECT id, status, total_items, zip_path, error, created_at, completed_at
		FROM bulk_export_jobs WHERE id = ?`, id,
	).Scan(&job.ID, &job.Status, &job.TotalItems, &zipPath, &errMsg, &job.CreatedAt, &completedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if zipPath.Valid {
		job.ZipPath = &zipPath.String
	}
	if errMsg.Valid {
		job.Error = &errMsg.String
	}
	if completedAt.Valid {
		job.CompletedAt = &completedAt.String
	}
	return job, nil
}

// GetBulkExportItems retrieves all items for a bulk export job.
func (d *DB) GetBulkExportItems(jobID string) ([]*BulkExportItem, error) {
	rows, err := d.Query(`
		SELECT id, job_id, camera_id, camera_name, start_time, end_time, status, file_count, total_bytes, error
		FROM bulk_export_items WHERE job_id = ?`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var items []*BulkExportItem
	for rows.Next() {
		item := &BulkExportItem{}
		var errMsg sql.NullString
		if err := rows.Scan(&item.ID, &item.JobID, &item.CameraID, &item.CameraName,
			&item.StartTime, &item.EndTime, &item.Status, &item.FileCount, &item.TotalBytes, &errMsg); err != nil {
			return nil, err
		}
		if errMsg.Valid {
			item.Error = &errMsg.String
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// ListBulkExportJobs returns the most recent bulk export jobs.
func (d *DB) ListBulkExportJobs(limit int) ([]*BulkExportJob, error) {
	rows, err := d.Query(`
		SELECT id, status, total_items, zip_path, error, created_at, completed_at
		FROM bulk_export_jobs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*BulkExportJob
	for rows.Next() {
		job := &BulkExportJob{}
		var zipPath, errMsg, completedAt sql.NullString
		if err := rows.Scan(&job.ID, &job.Status, &job.TotalItems, &zipPath, &errMsg, &job.CreatedAt, &completedAt); err != nil {
			return nil, err
		}
		if zipPath.Valid {
			job.ZipPath = &zipPath.String
		}
		if errMsg.Valid {
			job.Error = &errMsg.String
		}
		if completedAt.Valid {
			job.CompletedAt = &completedAt.String
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// CompleteBulkExportJob updates the status of a bulk export job.
func (d *DB) CompleteBulkExportJob(id, status string, zipPath, errMsg *string) error {
	now := time.Now().UTC().Format(timeFormat)
	_, err := d.Exec(`
		UPDATE bulk_export_jobs SET status = ?, zip_path = ?, error = ?, completed_at = ?
		WHERE id = ?`,
		status, zipPath, errMsg, now, id,
	)
	return err
}

// UpdateBulkExportItemStatus updates the status and progress of a single item.
func (d *DB) UpdateBulkExportItemStatus(itemID, status string, fileCount int, totalBytes int64, errMsg *string) error {
	_, err := d.Exec(`
		UPDATE bulk_export_items SET status = ?, file_count = ?, total_bytes = ?, error = ?
		WHERE id = ?`,
		status, fileCount, totalBytes, errMsg, itemID,
	)
	return err
}

// DeleteBulkExportJob deletes a bulk export job and its items (cascade).
func (d *DB) DeleteBulkExportJob(id string) error {
	// Delete items first (in case FK cascade isn't set up).
	_, _ = d.Exec("DELETE FROM bulk_export_items WHERE job_id = ?", id)
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
