package legacydb

import "fmt"

// NotificationPreference represents a per-user, per-camera, per-event-type
// channel preference (e.g., "user X wants email for motion on camera Y").
type NotificationPreference struct {
	ID        int64  `json:"id"`
	UserID    string `json:"user_id"`
	CameraID  string `json:"camera_id"`  // "*" means all cameras
	EventType string `json:"event_type"` // motion, camera_offline, camera_online, recording_started, recording_stopped
	Channel   string `json:"channel"`    // email, sms, push, slack, teams
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// QuietHours defines when a user should not receive notifications.
type QuietHours struct {
	ID        int64  `json:"id"`
	UserID    string `json:"user_id"`
	Enabled   bool   `json:"enabled"`
	StartTime string `json:"start_time"` // HH:MM (24-hour)
	EndTime   string `json:"end_time"`   // HH:MM (24-hour)
	Timezone  string `json:"timezone"`
	Days      string `json:"days"` // JSON array, e.g. ["mon","tue","wed"]
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

// EscalationRule defines a chain of notifications that fire when an event
// is not acknowledged within a specified time.
type EscalationRule struct {
	ID                     int64  `json:"id"`
	Name                   string `json:"name"`
	EventType              string `json:"event_type"`
	CameraID               string `json:"camera_id"` // "*" means all
	Enabled                bool   `json:"enabled"`
	DelayMinutes           int    `json:"delay_minutes"`
	RepeatCount            int    `json:"repeat_count"`
	RepeatIntervalMinutes  int    `json:"repeat_interval_minutes"`
	EscalationChain        string `json:"escalation_chain"` // JSON array of steps
	CreatedAt              string `json:"created_at"`
	UpdatedAt              string `json:"updated_at"`
}

// --- Notification Preferences ---

// ListNotificationPreferences returns all notification preferences for a user.
func (d *DB) ListNotificationPreferences(userID string) ([]*NotificationPreference, error) {
	rows, err := d.Query(`
		SELECT id, user_id, camera_id, event_type, channel, enabled, created_at, updated_at
		FROM notification_preferences
		WHERE user_id = ?
		ORDER BY camera_id, event_type, channel`, userID)
	if err != nil {
		return nil, fmt.Errorf("list notification prefs: %w", err)
	}
	defer rows.Close()

	var out []*NotificationPreference
	for rows.Next() {
		p := &NotificationPreference{}
		if err := rows.Scan(&p.ID, &p.UserID, &p.CameraID, &p.EventType, &p.Channel, &p.Enabled, &p.CreatedAt, &p.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan notification pref: %w", err)
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// UpsertNotificationPreference creates or updates a notification preference.
func (d *DB) UpsertNotificationPreference(p *NotificationPreference) error {
	_, err := d.Exec(`
		INSERT INTO notification_preferences (user_id, camera_id, event_type, channel, enabled)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (user_id, camera_id, event_type, channel)
		DO UPDATE SET enabled = excluded.enabled,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`,
		p.UserID, p.CameraID, p.EventType, p.Channel, p.Enabled)
	if err != nil {
		return fmt.Errorf("upsert notification pref: %w", err)
	}
	return nil
}

// BulkUpsertNotificationPreferences upserts multiple preferences in a single transaction.
func (d *DB) BulkUpsertNotificationPreferences(prefs []*NotificationPreference) error {
	tx, err := d.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck

	stmt, err := tx.Prepare(`
		INSERT INTO notification_preferences (user_id, camera_id, event_type, channel, enabled)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (user_id, camera_id, event_type, channel)
		DO UPDATE SET enabled = excluded.enabled,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`)
	if err != nil {
		return fmt.Errorf("prepare stmt: %w", err)
	}
	defer stmt.Close()

	for _, p := range prefs {
		if _, err := stmt.Exec(p.UserID, p.CameraID, p.EventType, p.Channel, p.Enabled); err != nil {
			return fmt.Errorf("exec upsert: %w", err)
		}
	}
	return tx.Commit()
}

// --- Quiet Hours ---

// GetQuietHours returns the quiet-hours configuration for a user.
func (d *DB) GetQuietHours(userID string) (*QuietHours, error) {
	row := d.QueryRow(`
		SELECT id, user_id, enabled, start_time, end_time, timezone, days, created_at, updated_at
		FROM notification_quiet_hours
		WHERE user_id = ?`, userID)

	q := &QuietHours{}
	err := row.Scan(&q.ID, &q.UserID, &q.Enabled, &q.StartTime, &q.EndTime, &q.Timezone, &q.Days, &q.CreatedAt, &q.UpdatedAt)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			return &QuietHours{
				UserID:    userID,
				Enabled:   false,
				StartTime: "22:00",
				EndTime:   "07:00",
				Timezone:  "UTC",
				Days:      `["mon","tue","wed","thu","fri","sat","sun"]`,
			}, nil
		}
		return nil, fmt.Errorf("get quiet hours: %w", err)
	}
	return q, nil
}

// UpsertQuietHours creates or updates quiet-hours settings for a user.
func (d *DB) UpsertQuietHours(q *QuietHours) error {
	_, err := d.Exec(`
		INSERT INTO notification_quiet_hours (user_id, enabled, start_time, end_time, timezone, days)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (user_id)
		DO UPDATE SET
			enabled = excluded.enabled,
			start_time = excluded.start_time,
			end_time = excluded.end_time,
			timezone = excluded.timezone,
			days = excluded.days,
			updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')`,
		q.UserID, q.Enabled, q.StartTime, q.EndTime, q.Timezone, q.Days)
	if err != nil {
		return fmt.Errorf("upsert quiet hours: %w", err)
	}
	return nil
}

// --- Escalation Rules ---

// ListEscalationRules returns all escalation rules.
func (d *DB) ListEscalationRules() ([]*EscalationRule, error) {
	rows, err := d.Query(`
		SELECT id, name, event_type, camera_id, enabled, delay_minutes, repeat_count,
			repeat_interval_minutes, escalation_chain, created_at, updated_at
		FROM escalation_rules
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("list escalation rules: %w", err)
	}
	defer rows.Close()

	var out []*EscalationRule
	for rows.Next() {
		r := &EscalationRule{}
		if err := rows.Scan(&r.ID, &r.Name, &r.EventType, &r.CameraID, &r.Enabled, &r.DelayMinutes,
			&r.RepeatCount, &r.RepeatIntervalMinutes, &r.EscalationChain, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, fmt.Errorf("scan escalation rule: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// CreateEscalationRule inserts a new escalation rule.
func (d *DB) CreateEscalationRule(r *EscalationRule) error {
	res, err := d.Exec(`
		INSERT INTO escalation_rules (name, event_type, camera_id, enabled, delay_minutes,
			repeat_count, repeat_interval_minutes, escalation_chain)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		r.Name, r.EventType, r.CameraID, r.Enabled, r.DelayMinutes,
		r.RepeatCount, r.RepeatIntervalMinutes, r.EscalationChain)
	if err != nil {
		return fmt.Errorf("create escalation rule: %w", err)
	}
	id, _ := res.LastInsertId()
	r.ID = id
	return nil
}

// UpdateEscalationRule updates an existing escalation rule.
func (d *DB) UpdateEscalationRule(r *EscalationRule) error {
	res, err := d.Exec(`
		UPDATE escalation_rules SET
			name = ?, event_type = ?, camera_id = ?, enabled = ?,
			delay_minutes = ?, repeat_count = ?, repeat_interval_minutes = ?,
			escalation_chain = ?, updated_at = strftime('%Y-%m-%dT%H:%M:%fZ', 'now')
		WHERE id = ?`,
		r.Name, r.EventType, r.CameraID, r.Enabled, r.DelayMinutes,
		r.RepeatCount, r.RepeatIntervalMinutes, r.EscalationChain, r.ID)
	if err != nil {
		return fmt.Errorf("update escalation rule: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteEscalationRule removes an escalation rule by ID.
func (d *DB) DeleteEscalationRule(id int64) error {
	res, err := d.Exec(`DELETE FROM escalation_rules WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("delete escalation rule: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}
