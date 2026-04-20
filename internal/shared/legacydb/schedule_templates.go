package legacydb

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ScheduleTemplate represents a reusable recording schedule template.
type ScheduleTemplate struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	Mode             string `json:"mode"`
	Days             string `json:"days"`
	StartTime        string `json:"start_time"`
	EndTime          string `json:"end_time"`
	PostEventSeconds int    `json:"post_event_seconds"`
	IsDefault        bool   `json:"is_default"`
	CreatedAt        string `json:"created_at"`
}

// ListScheduleTemplates returns all schedule templates ordered by is_default DESC, name.
func (d *DB) ListScheduleTemplates() ([]*ScheduleTemplate, error) {
	rows, err := d.Query(`
		SELECT id, name, mode, days, start_time, end_time, post_event_seconds, is_default, created_at
		FROM schedule_templates
		ORDER BY is_default DESC, name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var templates []*ScheduleTemplate
	for rows.Next() {
		t := &ScheduleTemplate{}
		if err := rows.Scan(
			&t.ID, &t.Name, &t.Mode, &t.Days, &t.StartTime, &t.EndTime,
			&t.PostEventSeconds, &t.IsDefault, &t.CreatedAt,
		); err != nil {
			return nil, err
		}
		templates = append(templates, t)
	}
	return templates, rows.Err()
}

// GetScheduleTemplate retrieves a schedule template by ID. Returns ErrNotFound if no match.
func (d *DB) GetScheduleTemplate(id string) (*ScheduleTemplate, error) {
	t := &ScheduleTemplate{}
	err := d.QueryRow(`
		SELECT id, name, mode, days, start_time, end_time, post_event_seconds, is_default, created_at
		FROM schedule_templates WHERE id = ?`, id,
	).Scan(
		&t.ID, &t.Name, &t.Mode, &t.Days, &t.StartTime, &t.EndTime,
		&t.PostEventSeconds, &t.IsDefault, &t.CreatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return t, nil
}

// CreateScheduleTemplate inserts a new schedule template. A UUID and timestamp are auto-generated.
func (d *DB) CreateScheduleTemplate(t *ScheduleTemplate) error {
	if t.ID == "" {
		t.ID = uuid.New().String()
	}
	t.CreatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	_, err := d.Exec(`
		INSERT INTO schedule_templates
			(id, name, mode, days, start_time, end_time, post_event_seconds, is_default, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		t.ID, t.Name, t.Mode, t.Days, t.StartTime, t.EndTime,
		t.PostEventSeconds, t.IsDefault, t.CreatedAt,
	)
	return err
}

// UpdateScheduleTemplate updates the mutable fields of a schedule template.
// Returns ErrNotFound if no match.
func (d *DB) UpdateScheduleTemplate(t *ScheduleTemplate) error {
	res, err := d.Exec(`
		UPDATE schedule_templates
		SET name = ?, mode = ?, days = ?, start_time = ?, end_time = ?, post_event_seconds = ?
		WHERE id = ?`,
		t.Name, t.Mode, t.Days, t.StartTime, t.EndTime, t.PostEventSeconds, t.ID,
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

// DeleteScheduleTemplate deletes a non-default schedule template by ID.
// Returns ErrNotFound if no match or if the template is a default.
func (d *DB) DeleteScheduleTemplate(id string) error {
	res, err := d.Exec("DELETE FROM schedule_templates WHERE id = ? AND is_default = 0", id)
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

// CountTemplateUsage returns how many recording rules reference the given template ID.
func (d *DB) CountTemplateUsage(templateID string) (int, error) {
	var count int
	err := d.QueryRow(
		`SELECT COUNT(*) FROM recording_rules WHERE template_id = ?`, templateID,
	).Scan(&count)
	return count, err
}

// SeedDefaultTemplates inserts the five built-in schedule templates when the table is empty.
// It is safe to call on every startup — it is a no-op when templates already exist.
func (d *DB) SeedDefaultTemplates() error {
	var count int
	if err := d.QueryRow(`SELECT COUNT(*) FROM schedule_templates`).Scan(&count); err != nil {
		return err
	}
	if count > 0 {
		return nil
	}

	defaults := []ScheduleTemplate{
		{
			Name:             "24/7 Continuous",
			Mode:             "always",
			Days:             "[0,1,2,3,4,5,6]",
			StartTime:        "00:00",
			EndTime:          "00:00",
			PostEventSeconds: 0,
			IsDefault:        true,
		},
		{
			Name:             "24/7 Motion",
			Mode:             "events",
			Days:             "[0,1,2,3,4,5,6]",
			StartTime:        "00:00",
			EndTime:          "00:00",
			PostEventSeconds: 30,
			IsDefault:        true,
		},
		{
			Name:             "Business Hours",
			Mode:             "always",
			Days:             "[1,2,3,4,5]",
			StartTime:        "08:00",
			EndTime:          "18:00",
			PostEventSeconds: 0,
			IsDefault:        true,
		},
		{
			Name:             "After Hours Motion",
			Mode:             "events",
			Days:             "[0,1,2,3,4,5,6]",
			StartTime:        "18:00",
			EndTime:          "08:00",
			PostEventSeconds: 30,
			IsDefault:        true,
		},
		{
			Name:             "Weekday Only",
			Mode:             "always",
			Days:             "[1,2,3,4,5]",
			StartTime:        "00:00",
			EndTime:          "00:00",
			PostEventSeconds: 0,
			IsDefault:        true,
		},
	}

	for i := range defaults {
		if err := d.CreateScheduleTemplate(&defaults[i]); err != nil {
			return err
		}
	}
	return nil
}
