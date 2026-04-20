package legacydb

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// DetectionSchedule represents a time window during which AI detection is
// active for a given camera on a specific day of the week.
type DetectionSchedule struct {
	ID        string `json:"id"`
	CameraID  string `json:"camera_id"`
	DayOfWeek int    `json:"day_of_week"` // 0=Sunday .. 6=Saturday
	StartTime string `json:"start_time"`  // HH:MM
	EndTime   string `json:"end_time"`    // HH:MM
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// ListDetectionSchedules returns all detection schedule entries for a camera,
// ordered by day_of_week and start_time.
func (d *DB) ListDetectionSchedules(cameraID string) ([]*DetectionSchedule, error) {
	rows, err := d.Query(`
		SELECT id, camera_id, day_of_week, start_time, end_time, enabled, created_at, updated_at
		FROM detection_schedules
		WHERE camera_id = ?
		ORDER BY day_of_week, start_time`, cameraID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []*DetectionSchedule
	for rows.Next() {
		s := &DetectionSchedule{}
		if err := rows.Scan(
			&s.ID, &s.CameraID, &s.DayOfWeek, &s.StartTime, &s.EndTime,
			&s.Enabled, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		schedules = append(schedules, s)
	}
	return schedules, rows.Err()
}

// GetDetectionSchedule retrieves a single detection schedule entry by ID.
// Returns ErrNotFound if no match.
func (d *DB) GetDetectionSchedule(id string) (*DetectionSchedule, error) {
	s := &DetectionSchedule{}
	err := d.QueryRow(`
		SELECT id, camera_id, day_of_week, start_time, end_time, enabled, created_at, updated_at
		FROM detection_schedules WHERE id = ?`, id,
	).Scan(
		&s.ID, &s.CameraID, &s.DayOfWeek, &s.StartTime, &s.EndTime,
		&s.Enabled, &s.CreatedAt, &s.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s, nil
}

// UpsertDetectionSchedules replaces all detection schedule entries for a camera
// with the provided set. This runs inside a transaction.
func (d *DB) UpsertDetectionSchedules(cameraID string, schedules []*DetectionSchedule) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.Exec(`DELETE FROM detection_schedules WHERE camera_id = ?`, cameraID); err != nil {
		return err
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	for _, s := range schedules {
		if s.ID == "" {
			s.ID = uuid.New().String()
		}
		s.CameraID = cameraID
		s.CreatedAt = now
		s.UpdatedAt = now

		if _, err := tx.Exec(`
			INSERT INTO detection_schedules
				(id, camera_id, day_of_week, start_time, end_time, enabled, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			s.ID, s.CameraID, s.DayOfWeek, s.StartTime, s.EndTime,
			s.Enabled, s.CreatedAt, s.UpdatedAt,
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

// ReplaceDetectionSchedules is an alias for UpsertDetectionSchedules.
func (d *DB) ReplaceDetectionSchedules(cameraID string, schedules []*DetectionSchedule) error {
	return d.UpsertDetectionSchedules(cameraID, schedules)
}

// ListAllDetectionSchedules returns all detection schedule entries across all
// cameras, ordered by camera_id, day_of_week, start_time.
func (d *DB) ListAllDetectionSchedules() ([]*DetectionSchedule, error) {
	rows, err := d.Query(`
		SELECT id, camera_id, day_of_week, start_time, end_time, enabled, created_at, updated_at
		FROM detection_schedules
		ORDER BY camera_id, day_of_week, start_time`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var schedules []*DetectionSchedule
	for rows.Next() {
		s := &DetectionSchedule{}
		if err := rows.Scan(
			&s.ID, &s.CameraID, &s.DayOfWeek, &s.StartTime, &s.EndTime,
			&s.Enabled, &s.CreatedAt, &s.UpdatedAt,
		); err != nil {
			return nil, err
		}
		schedules = append(schedules, s)
	}
	return schedules, rows.Err()
}

// DeleteDetectionSchedules removes all detection schedules for a camera.
func (d *DB) DeleteDetectionSchedules(cameraID string) error {
	_, err := d.Exec(`DELETE FROM detection_schedules WHERE camera_id = ?`, cameraID)
	return err
}
