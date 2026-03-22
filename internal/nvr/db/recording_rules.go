package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// RecordingRule represents a recording rule record in the database.
type RecordingRule struct {
	ID               string `json:"id"`
	CameraID         string `json:"camera_id"`
	Name             string `json:"name"`
	Mode             string `json:"mode"`
	Days             string `json:"days"`
	StartTime        string `json:"start_time"`
	EndTime          string `json:"end_time"`
	PostEventSeconds int    `json:"post_event_seconds"`
	Enabled          bool   `json:"enabled"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

// CreateRecordingRule inserts a new recording rule into the database.
// A new UUID and timestamps are automatically generated.
func (d *DB) CreateRecordingRule(rule *RecordingRule) error {
	if rule.ID == "" {
		rule.ID = uuid.New().String()
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	rule.CreatedAt = now
	rule.UpdatedAt = now

	_, err := d.Exec(`
		INSERT INTO recording_rules (id, camera_id, name, mode, days, start_time,
			end_time, post_event_seconds, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		rule.ID, rule.CameraID, rule.Name, rule.Mode, rule.Days, rule.StartTime,
		rule.EndTime, rule.PostEventSeconds, rule.Enabled, rule.CreatedAt, rule.UpdatedAt,
	)
	return err
}

// GetRecordingRule retrieves a recording rule by its ID. Returns ErrNotFound if no match.
func (d *DB) GetRecordingRule(id string) (*RecordingRule, error) {
	rule := &RecordingRule{}
	err := d.QueryRow(`
		SELECT id, camera_id, name, mode, days, start_time, end_time,
			post_event_seconds, enabled, created_at, updated_at
		FROM recording_rules WHERE id = ?`, id,
	).Scan(
		&rule.ID, &rule.CameraID, &rule.Name, &rule.Mode, &rule.Days, &rule.StartTime,
		&rule.EndTime, &rule.PostEventSeconds, &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return rule, nil
}

// ListRecordingRules returns all recording rules for a given camera, ordered by created_at.
func (d *DB) ListRecordingRules(cameraID string) ([]*RecordingRule, error) {
	rows, err := d.Query(`
		SELECT id, camera_id, name, mode, days, start_time, end_time,
			post_event_seconds, enabled, created_at, updated_at
		FROM recording_rules WHERE camera_id = ? ORDER BY created_at`, cameraID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*RecordingRule
	for rows.Next() {
		rule := &RecordingRule{}
		if err := rows.Scan(
			&rule.ID, &rule.CameraID, &rule.Name, &rule.Mode, &rule.Days, &rule.StartTime,
			&rule.EndTime, &rule.PostEventSeconds, &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// ListAllEnabledRecordingRules returns all enabled recording rules across all cameras.
func (d *DB) ListAllEnabledRecordingRules() ([]*RecordingRule, error) {
	rows, err := d.Query(`
		SELECT id, camera_id, name, mode, days, start_time, end_time,
			post_event_seconds, enabled, created_at, updated_at
		FROM recording_rules WHERE enabled = 1 ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*RecordingRule
	for rows.Next() {
		rule := &RecordingRule{}
		if err := rows.Scan(
			&rule.ID, &rule.CameraID, &rule.Name, &rule.Mode, &rule.Days, &rule.StartTime,
			&rule.EndTime, &rule.PostEventSeconds, &rule.Enabled, &rule.CreatedAt, &rule.UpdatedAt,
		); err != nil {
			return nil, err
		}
		rules = append(rules, rule)
	}
	return rules, rows.Err()
}

// UpdateRecordingRule updates an existing recording rule. Returns ErrNotFound if no match.
func (d *DB) UpdateRecordingRule(rule *RecordingRule) error {
	rule.UpdatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	res, err := d.Exec(`
		UPDATE recording_rules SET camera_id = ?, name = ?, mode = ?, days = ?,
			start_time = ?, end_time = ?, post_event_seconds = ?, enabled = ?,
			updated_at = ?
		WHERE id = ?`,
		rule.CameraID, rule.Name, rule.Mode, rule.Days, rule.StartTime,
		rule.EndTime, rule.PostEventSeconds, rule.Enabled, rule.UpdatedAt, rule.ID,
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

// CountRecordingRules returns the total number of recording rules and how many
// are enabled, using a single aggregate query instead of per-camera lookups.
func (d *DB) CountRecordingRules() (total int, enabled int, err error) {
	err = d.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE WHEN enabled = 1 THEN 1 ELSE 0 END), 0) FROM recording_rules`).Scan(&total, &enabled)
	return
}

// CountCamerasWithRules returns the number of distinct cameras that have at
// least one enabled recording rule.
func (d *DB) CountCamerasWithRules() (int, error) {
	var count int
	err := d.QueryRow(`SELECT COUNT(DISTINCT camera_id) FROM recording_rules WHERE enabled = 1`).Scan(&count)
	return count, err
}

// DeleteRecordingRule deletes a recording rule by its ID. Returns ErrNotFound if no match.
func (d *DB) DeleteRecordingRule(id string) error {
	res, err := d.Exec("DELETE FROM recording_rules WHERE id = ?", id)
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
