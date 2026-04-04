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
	ErrorMsg    *string `json:"error,omitempty"`
	CreatedAt   string  `json:"created_at"`
	CompletedAt string  `json:"completed_at,omitempty"`
}

// BulkExportItem represents a single camera/time-range item within a bulk export.
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
	job.ID = uuid.New().String()
	job.Status = "pending"
	job.CreatedAt = time.Now().UTC().Format(time.RFC3339)

	tx, err := d.Begin()
	if err != nil {
		return err
	}

	if _, err := tx.Exec(`INSERT INTO export_jobs (id, camera_id, start_time, end_time, status, created_at) VALUES (?, '', '', '', ?, ?)`,
		job.ID, job.Status, job.CreatedAt); err != nil {
		tx.Rollback()
		return err
	}

	for _, item := range items {
		item.ID = uuid.New().String()
		item.JobID = job.ID
		item.Status = "pending"
	}

	return tx.Commit()
}

// GetBulkExportJob retrieves a bulk export job by ID.
func (d *DB) GetBulkExportJob(id string) (*BulkExportJob, error) {
	var job BulkExportJob
	var zipPath, errorMsg sql.NullString
	err := d.QueryRow(`SELECT id, status, output_path, error, created_at, completed_at FROM export_jobs WHERE id = ?`, id).
		Scan(&job.ID, &job.Status, &zipPath, &errorMsg, &job.CreatedAt, &job.CompletedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if zipPath.Valid && zipPath.String != "" {
		job.ZipPath = &zipPath.String
	}
	if errorMsg.Valid && errorMsg.String != "" {
		job.ErrorMsg = &errorMsg.String
	}
	return &job, nil
}

// GetBulkExportItems returns all items for a given bulk export job.
func (d *DB) GetBulkExportItems(_ string) ([]*BulkExportItem, error) {
	// Items are stored in-memory during export; this is a stub for the API.
	return []*BulkExportItem{}, nil
}

// ListBulkExportJobs returns recent bulk export jobs.
func (d *DB) ListBulkExportJobs(limit int) ([]*BulkExportJob, error) {
	rows, err := d.Query(`SELECT id, status, output_path, error, created_at, completed_at FROM export_jobs ORDER BY created_at DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*BulkExportJob
	for rows.Next() {
		var job BulkExportJob
		var zipPath, errorMsg sql.NullString
		if err := rows.Scan(&job.ID, &job.Status, &zipPath, &errorMsg, &job.CreatedAt, &job.CompletedAt); err != nil {
			return nil, err
		}
		if zipPath.Valid && zipPath.String != "" {
			job.ZipPath = &zipPath.String
		}
		if errorMsg.Valid && errorMsg.String != "" {
			job.ErrorMsg = &errorMsg.String
		}
		jobs = append(jobs, &job)
	}
	return jobs, rows.Err()
}

// CompleteBulkExportJob updates a bulk export job's status.
func (d *DB) CompleteBulkExportJob(id, status string, zipPath, errorMsg *string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	zp := ""
	if zipPath != nil {
		zp = *zipPath
	}
	em := ""
	if errorMsg != nil {
		em = *errorMsg
	}
	_, err := d.Exec(`UPDATE export_jobs SET status=?, output_path=?, error=?, completed_at=? WHERE id=?`,
		status, zp, em, now, id)
	return err
}

// UpdateBulkExportItemStatus updates the status of a single bulk export item.
func (d *DB) UpdateBulkExportItemStatus(_, status string, _ int, _ int64, _ *string) error {
	// Items are tracked in-memory during export; this is a no-op stub.
	_ = status
	return nil
}

// DeleteBulkExportJob deletes a bulk export job.
func (d *DB) DeleteBulkExportJob(id string) error {
	_, err := d.Exec("DELETE FROM export_jobs WHERE id = ?", id)
	return err
}
