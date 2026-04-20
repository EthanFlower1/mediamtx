package legacydb

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// SMTPConfig holds the SMTP server configuration for sending email notifications.
type SMTPConfig struct {
	Host       string `json:"host"`
	Port       int    `json:"port"`
	Username   string `json:"username"`
	Password   string `json:"password,omitempty"`
	FromAddr   string `json:"from_address"`
	TLSEnabled bool   `json:"tls_enabled"`
	UpdatedAt  string `json:"updated_at"`
}

// AlertRule defines a condition that triggers alerts.
type AlertRule struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	RuleType        string  `json:"rule_type"` // disk_usage, camera_offline, recording_gap
	ThresholdValue  float64 `json:"threshold_value"`
	CameraID        string  `json:"camera_id,omitempty"`
	Enabled         bool    `json:"enabled"`
	NotifyEmail     bool    `json:"notify_email"`
	CooldownMinutes int     `json:"cooldown_minutes"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

// Alert represents a triggered alert instance.
type Alert struct {
	ID             string `json:"id"`
	RuleID         string `json:"rule_id"`
	RuleType       string `json:"rule_type"`
	Severity       string `json:"severity"`
	Message        string `json:"message"`
	Details        string `json:"details,omitempty"`
	Acknowledged   bool   `json:"acknowledged"`
	AcknowledgedBy string `json:"acknowledged_by,omitempty"`
	AcknowledgedAt string `json:"acknowledged_at,omitempty"`
	EmailSent      bool   `json:"email_sent"`
	EmailError     string `json:"email_error,omitempty"`
	CreatedAt      string `json:"created_at"`
}

// GetSMTPConfig returns the SMTP configuration. There is always exactly one row.
func (d *DB) GetSMTPConfig() (*SMTPConfig, error) {
	var cfg SMTPConfig
	var tlsInt int
	err := d.QueryRow(`SELECT host, port, username, password, from_address, tls_enabled, updated_at FROM smtp_config WHERE id = 1`).
		Scan(&cfg.Host, &cfg.Port, &cfg.Username, &cfg.Password, &cfg.FromAddr, &tlsInt, &cfg.UpdatedAt)
	if err != nil {
		return nil, err
	}
	cfg.TLSEnabled = tlsInt == 1
	return &cfg, nil
}

// UpdateSMTPConfig updates the SMTP configuration.
func (d *DB) UpdateSMTPConfig(cfg *SMTPConfig) error {
	now := time.Now().UTC().Format(time.RFC3339)
	tlsInt := 0
	if cfg.TLSEnabled {
		tlsInt = 1
	}
	_, err := d.Exec(`UPDATE smtp_config SET host=?, port=?, username=?, password=?, from_address=?, tls_enabled=?, updated_at=? WHERE id=1`,
		cfg.Host, cfg.Port, cfg.Username, cfg.Password, cfg.FromAddr, tlsInt, now)
	return err
}

// CreateAlertRule inserts a new alert rule.
func (d *DB) CreateAlertRule(r *AlertRule) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	r.CreatedAt = now
	r.UpdatedAt = now

	enabledInt := 0
	if r.Enabled {
		enabledInt = 1
	}
	notifyInt := 0
	if r.NotifyEmail {
		notifyInt = 1
	}

	_, err := d.Exec(`INSERT INTO alert_rules (id, name, rule_type, threshold_value, camera_id, enabled, notify_email, cooldown_minutes, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.RuleType, r.ThresholdValue, r.CameraID, enabledInt, notifyInt, r.CooldownMinutes, r.CreatedAt, r.UpdatedAt)
	return err
}

// UpdateAlertRule updates an existing alert rule.
func (d *DB) UpdateAlertRule(r *AlertRule) error {
	now := time.Now().UTC().Format(time.RFC3339)
	r.UpdatedAt = now

	enabledInt := 0
	if r.Enabled {
		enabledInt = 1
	}
	notifyInt := 0
	if r.NotifyEmail {
		notifyInt = 1
	}

	res, err := d.Exec(`UPDATE alert_rules SET name=?, rule_type=?, threshold_value=?, camera_id=?, enabled=?, notify_email=?, cooldown_minutes=?, updated_at=? WHERE id=?`,
		r.Name, r.RuleType, r.ThresholdValue, r.CameraID, enabledInt, notifyInt, r.CooldownMinutes, r.UpdatedAt, r.ID)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteAlertRule removes an alert rule and cascades to alerts.
func (d *DB) DeleteAlertRule(id string) error {
	res, err := d.Exec("DELETE FROM alert_rules WHERE id = ?", id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListAlertRules returns all alert rules ordered by creation time.
func (d *DB) ListAlertRules() ([]*AlertRule, error) {
	rows, err := d.Query(`SELECT id, name, rule_type, threshold_value, camera_id, enabled, notify_email, cooldown_minutes, created_at, updated_at FROM alert_rules ORDER BY created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*AlertRule
	for rows.Next() {
		var r AlertRule
		var enabledInt, notifyInt int
		if err := rows.Scan(&r.ID, &r.Name, &r.RuleType, &r.ThresholdValue, &r.CameraID, &enabledInt, &notifyInt, &r.CooldownMinutes, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabledInt == 1
		r.NotifyEmail = notifyInt == 1
		rules = append(rules, &r)
	}
	return rules, rows.Err()
}

// GetAlertRule retrieves a single alert rule by ID.
func (d *DB) GetAlertRule(id string) (*AlertRule, error) {
	var r AlertRule
	var enabledInt, notifyInt int
	err := d.QueryRow(`SELECT id, name, rule_type, threshold_value, camera_id, enabled, notify_email, cooldown_minutes, created_at, updated_at FROM alert_rules WHERE id = ?`, id).
		Scan(&r.ID, &r.Name, &r.RuleType, &r.ThresholdValue, &r.CameraID, &enabledInt, &notifyInt, &r.CooldownMinutes, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.Enabled = enabledInt == 1
	r.NotifyEmail = notifyInt == 1
	return &r, nil
}

// ListEnabledAlertRules returns only enabled alert rules.
func (d *DB) ListEnabledAlertRules() ([]*AlertRule, error) {
	rows, err := d.Query(`SELECT id, name, rule_type, threshold_value, camera_id, enabled, notify_email, cooldown_minutes, created_at, updated_at FROM alert_rules WHERE enabled = 1`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var rules []*AlertRule
	for rows.Next() {
		var r AlertRule
		var enabledInt, notifyInt int
		if err := rows.Scan(&r.ID, &r.Name, &r.RuleType, &r.ThresholdValue, &r.CameraID, &enabledInt, &notifyInt, &r.CooldownMinutes, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.Enabled = enabledInt == 1
		r.NotifyEmail = notifyInt == 1
		rules = append(rules, &r)
	}
	return rules, rows.Err()
}

// CreateAlert inserts a new alert.
func (d *DB) CreateAlert(a *Alert) error {
	if a.ID == "" {
		a.ID = uuid.New().String()
	}
	if a.CreatedAt == "" {
		a.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}

	ackedInt := 0
	if a.Acknowledged {
		ackedInt = 1
	}
	emailInt := 0
	if a.EmailSent {
		emailInt = 1
	}

	_, err := d.Exec(`INSERT INTO alerts (id, rule_id, rule_type, severity, message, details, acknowledged, acknowledged_by, acknowledged_at, email_sent, email_error, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ID, a.RuleID, a.RuleType, a.Severity, a.Message, a.Details, ackedInt, a.AcknowledgedBy, a.AcknowledgedAt, emailInt, a.EmailError, a.CreatedAt)
	return err
}

// ListAlerts returns alerts with optional filtering, ordered by creation time descending.
func (d *DB) ListAlerts(acknowledged *bool, limit int) ([]*Alert, error) {
	query := `SELECT id, rule_id, rule_type, severity, message, details, acknowledged, acknowledged_by, acknowledged_at, email_sent, email_error, created_at FROM alerts`
	var args []interface{}

	if acknowledged != nil {
		ackedInt := 0
		if *acknowledged {
			ackedInt = 1
		}
		query += " WHERE acknowledged = ?"
		args = append(args, ackedInt)
	}

	query += " ORDER BY created_at DESC"

	if limit > 0 {
		query += " LIMIT ?"
		args = append(args, limit)
	}

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var alerts []*Alert
	for rows.Next() {
		var a Alert
		var ackedInt, emailInt int
		if err := rows.Scan(&a.ID, &a.RuleID, &a.RuleType, &a.Severity, &a.Message, &a.Details, &ackedInt, &a.AcknowledgedBy, &a.AcknowledgedAt, &emailInt, &a.EmailError, &a.CreatedAt); err != nil {
			return nil, err
		}
		a.Acknowledged = ackedInt == 1
		a.EmailSent = emailInt == 1
		alerts = append(alerts, &a)
	}
	return alerts, rows.Err()
}

// AcknowledgeAlert marks an alert as acknowledged.
func (d *DB) AcknowledgeAlert(id, username string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	res, err := d.Exec(`UPDATE alerts SET acknowledged = 1, acknowledged_by = ?, acknowledged_at = ? WHERE id = ?`,
		username, now, id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateAlertEmailStatus updates the email delivery status for an alert.
func (d *DB) UpdateAlertEmailStatus(id string, sent bool, emailErr string) error {
	emailInt := 0
	if sent {
		emailInt = 1
	}
	_, err := d.Exec(`UPDATE alerts SET email_sent = ?, email_error = ? WHERE id = ?`, emailInt, emailErr, id)
	return err
}

// GetLatestAlertForRule returns the most recent alert for a given rule, or nil if none exist.
func (d *DB) GetLatestAlertForRule(ruleID string) (*Alert, error) {
	var a Alert
	var ackedInt, emailInt int
	err := d.QueryRow(`SELECT id, rule_id, rule_type, severity, message, details, acknowledged, acknowledged_by, acknowledged_at, email_sent, email_error, created_at
		FROM alerts WHERE rule_id = ? ORDER BY created_at DESC LIMIT 1`, ruleID).
		Scan(&a.ID, &a.RuleID, &a.RuleType, &a.Severity, &a.Message, &a.Details, &ackedInt, &a.AcknowledgedBy, &a.AcknowledgedAt, &emailInt, &a.EmailError, &a.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	a.Acknowledged = ackedInt == 1
	a.EmailSent = emailInt == 1
	return &a, nil
}
