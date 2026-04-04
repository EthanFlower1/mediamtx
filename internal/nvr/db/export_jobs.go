package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ExportJob represents an asynchronous clip export job.
type ExportJob struct {
	ID          string  `json:"id"`
	CameraID    string  `json:"camera_id"`
	StartTime   string  `json:"start_time"`
	EndTime     string  `json:"end_time"`
	Status      string  `json:"status"` // pending, processing, completed, failed, cancelled
	Progress    float64 `json:"progress"`
	OutputPath  string  `json:"output_path"`
	Error       string  `json:"error"`
	CreatedAt   string  `json:"created_at"`
	CompletedAt string  `json:"completed_at,omitempty"`
}

// CreateExportJob inserts a new export job into the database.
// If job.ID is empty, a new UUID is generated.
func (d *DB) CreateExportJob(job *ExportJob) error {
	if job.ID == "" {
		job.ID = uuid.New().String()
	}
	if job.Status == "" {
		job.Status = "pending"
	}
	if job.CreatedAt == "" {
		job.CreatedAt = time.Now().UTC().Format(timeFormat)
	}

	_, err := d.Exec(`
		INSERT INTO export_jobs (id, camera_id, start_time, end_time, status, progress, output_path, error, created_at, completed_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		job.ID, job.CameraID, job.StartTime, job.EndTime, job.Status,
		job.Progress, job.OutputPath, job.Error, job.CreatedAt, job.CompletedAt,
	)
	return err
}

// GetExportJob retrieves an export job by its ID. Returns ErrNotFound if no match.
func (d *DB) GetExportJob(id string) (*ExportJob, error) {
	job := &ExportJob{}
	err := d.QueryRow(`
		SELECT id, camera_id, start_time, end_time, status, progress, output_path, error, created_at, completed_at
		FROM export_jobs WHERE id = ?`, id,
	).Scan(
		&job.ID, &job.CameraID, &job.StartTime, &job.EndTime, &job.Status,
		&job.Progress, &job.OutputPath, &job.Error, &job.CreatedAt, &job.CompletedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return job, nil
}

// ListExportJobs returns export jobs, optionally filtered by camera ID and/or status.
func (d *DB) ListExportJobs(cameraID, status string) ([]*ExportJob, error) {
	query := `SELECT id, camera_id, start_time, end_time, status, progress, output_path, error, created_at, completed_at
		FROM export_jobs WHERE 1=1`
	var args []interface{}

	if cameraID != "" {
		query += " AND camera_id = ?"
		args = append(args, cameraID)
	}
	if status != "" {
		query += " AND status = ?"
		args = append(args, status)
	}
	query += " ORDER BY created_at DESC"

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*ExportJob
	for rows.Next() {
		job := &ExportJob{}
		if err := rows.Scan(
			&job.ID, &job.CameraID, &job.StartTime, &job.EndTime, &job.Status,
			&job.Progress, &job.OutputPath, &job.Error, &job.CreatedAt, &job.CompletedAt,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}

// UpdateExportJobStatus updates the status and progress of an export job.
func (d *DB) UpdateExportJobStatus(id, status string, progress float64, errMsg string) error {
	var completedAt string
	if status == "completed" || status == "failed" || status == "cancelled" {
		completedAt = time.Now().UTC().Format(timeFormat)
	}

	res, err := d.Exec(`
		UPDATE export_jobs
		SET status = ?, progress = ?, error = ?, completed_at = ?
		WHERE id = ?`,
		status, progress, errMsg, completedAt, id,
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

// UpdateExportJobOutput sets the output path for a completed export job.
func (d *DB) UpdateExportJobOutput(id, outputPath string) error {
	res, err := d.Exec(`UPDATE export_jobs SET output_path = ? WHERE id = ?`, outputPath, id)
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

// DeleteExportJob deletes an export job by its ID. Returns ErrNotFound if no match.
func (d *DB) DeleteExportJob(id string) error {
	res, err := d.Exec("DELETE FROM export_jobs WHERE id = ?", id)
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

// GetPendingExportJobs returns export jobs with status 'pending', oldest first.
func (d *DB) GetPendingExportJobs() ([]*ExportJob, error) {
	rows, err := d.Query(`
		SELECT id, camera_id, start_time, end_time, status, progress, output_path, error, created_at, completed_at
		FROM export_jobs WHERE status = 'pending'
		ORDER BY created_at ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var jobs []*ExportJob
	for rows.Next() {
		job := &ExportJob{}
		if err := rows.Scan(
			&job.ID, &job.CameraID, &job.StartTime, &job.EndTime, &job.Status,
			&job.Progress, &job.OutputPath, &job.Error, &job.CreatedAt, &job.CompletedAt,
		); err != nil {
			return nil, err
		}
		jobs = append(jobs, job)
	}
	return jobs, rows.Err()
}
