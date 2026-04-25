// Package adminapi implements the Directory's admin-facing API for managing
// users, roles, recording schedules, retention policies, alert rules, audit
// logs, and export jobs. These are enterprise concerns that belong on the
// management server, not the recording server.
package adminapi

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// Store provides CRUD operations for all admin-managed entities.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store backed by the Directory's SQLite database.
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// DB returns the underlying database handle.
func (s *Store) DB() *sql.DB {
	return s.db
}

const defaultTenant = "local"

// --- Users ------------------------------------------------------------------

// User represents a Directory user.
type User struct {
	ID               string `json:"id"`
	Username         string `json:"username"`
	PasswordHash     string `json:"-"`
	RoleID           string `json:"role_id"`
	CameraPerms      string `json:"camera_permissions"`
	LockedUntil      string `json:"locked_until,omitempty"`
	FailedLogins     int    `json:"failed_login_attempts"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

func (s *Store) CreateUser(ctx context.Context, u User) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO users (id, tenant_id, username, password_hash, role_id, camera_permissions)
		VALUES (?, ?, ?, ?, ?, ?)
	`, u.ID, defaultTenant, u.Username, u.PasswordHash, u.RoleID, u.CameraPerms)
	return err
}

func (s *Store) GetUser(ctx context.Context, id string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, COALESCE(role_id,''), camera_permissions,
		       COALESCE(locked_until,''), failed_login_attempts, created_at, updated_at
		FROM users WHERE id = ?
	`, id).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.RoleID, &u.CameraPerms,
		&u.LockedUntil, &u.FailedLogins, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (s *Store) GetUserByUsername(ctx context.Context, username string) (User, error) {
	var u User
	err := s.db.QueryRowContext(ctx, `
		SELECT id, username, password_hash, COALESCE(role_id,''), camera_permissions,
		       COALESCE(locked_until,''), failed_login_attempts, created_at, updated_at
		FROM users WHERE username = ?
	`, username).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.RoleID, &u.CameraPerms,
		&u.LockedUntil, &u.FailedLogins, &u.CreatedAt, &u.UpdatedAt)
	return u, err
}

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, username, COALESCE(role_id,''), camera_permissions, created_at, updated_at
		FROM users ORDER BY username
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []User
	for rows.Next() {
		var u User
		if err := rows.Scan(&u.ID, &u.Username, &u.RoleID, &u.CameraPerms, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, u User) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE users SET username=?, role_id=?, camera_permissions=?, updated_at=CURRENT_TIMESTAMP
		WHERE id = ?
	`, u.Username, u.RoleID, u.CameraPerms, u.ID)
	return err
}

func (s *Store) DeleteUser(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	return err
}

// --- Roles ------------------------------------------------------------------

// Role represents a permission role.
type Role struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Permissions string `json:"permissions"` // JSON array
	IsSystem    bool   `json:"is_system"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

func (s *Store) CreateRole(ctx context.Context, r Role) error {
	system := 0
	if r.IsSystem {
		system = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO roles (id, tenant_id, name, description, permissions, is_system)
		VALUES (?, ?, ?, ?, ?, ?)
	`, r.ID, defaultTenant, r.Name, r.Description, r.Permissions, system)
	return err
}

func (s *Store) ListRoles(ctx context.Context) ([]Role, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, description, permissions, is_system, created_at, updated_at
		FROM roles ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Role
	for rows.Next() {
		var r Role
		var sys int
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.Permissions, &sys, &r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.IsSystem = sys == 1
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) DeleteRole(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM roles WHERE id = ? AND is_system = 0`, id)
	return err
}

// --- Recording Schedules ----------------------------------------------------

// RecordingSchedule represents a recording policy.
type RecordingSchedule struct {
	ID              string `json:"id"`
	CameraID        string `json:"camera_id,omitempty"`
	Mode            string `json:"mode"` // continuous, motion, schedule, off
	ScheduleCron    string `json:"schedule_cron,omitempty"`
	PreRollSeconds  int    `json:"pre_roll_seconds"`
	PostRollSeconds int    `json:"post_roll_seconds"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

func (s *Store) CreateSchedule(ctx context.Context, rs RecordingSchedule) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO recording_schedules (id, tenant_id, camera_id, mode, schedule_cron, pre_roll_seconds, post_roll_seconds)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, rs.ID, defaultTenant, nilIfEmpty(rs.CameraID), rs.Mode, nilIfEmpty(rs.ScheduleCron), rs.PreRollSeconds, rs.PostRollSeconds)
	return err
}

func (s *Store) ListSchedules(ctx context.Context) ([]RecordingSchedule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(camera_id,''), mode, COALESCE(schedule_cron,''),
		       pre_roll_seconds, post_roll_seconds, created_at, updated_at
		FROM recording_schedules ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RecordingSchedule
	for rows.Next() {
		var rs RecordingSchedule
		if err := rows.Scan(&rs.ID, &rs.CameraID, &rs.Mode, &rs.ScheduleCron,
			&rs.PreRollSeconds, &rs.PostRollSeconds, &rs.CreatedAt, &rs.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, rs)
	}
	return out, rows.Err()
}

func (s *Store) UpdateSchedule(ctx context.Context, rs RecordingSchedule) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE recording_schedules SET mode=?, schedule_cron=?, pre_roll_seconds=?,
		       post_roll_seconds=?, updated_at=CURRENT_TIMESTAMP
		WHERE id = ?
	`, rs.Mode, nilIfEmpty(rs.ScheduleCron), rs.PreRollSeconds, rs.PostRollSeconds, rs.ID)
	return err
}

func (s *Store) DeleteSchedule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM recording_schedules WHERE id = ?`, id)
	return err
}

// --- Retention Policies -----------------------------------------------------

// RetentionPolicy represents a retention configuration.
type RetentionPolicy struct {
	ID              string `json:"id"`
	CameraID        string `json:"camera_id,omitempty"`
	HotDays         int    `json:"hot_days"`
	WarmDays        int    `json:"warm_days"`
	ColdDays        int    `json:"cold_days"`
	DeleteAfterDays int    `json:"delete_after_days"`
	ArchiveTier     string `json:"archive_tier"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

func (s *Store) CreateRetention(ctx context.Context, rp RetentionPolicy) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO retention_policies (id, tenant_id, camera_id, hot_days, warm_days, cold_days, delete_after_days, archive_tier)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, rp.ID, defaultTenant, nilIfEmpty(rp.CameraID), rp.HotDays, rp.WarmDays, rp.ColdDays, rp.DeleteAfterDays, rp.ArchiveTier)
	return err
}

func (s *Store) ListRetention(ctx context.Context) ([]RetentionPolicy, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(camera_id,''), hot_days, warm_days, cold_days,
		       delete_after_days, archive_tier, created_at, updated_at
		FROM retention_policies ORDER BY id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []RetentionPolicy
	for rows.Next() {
		var rp RetentionPolicy
		if err := rows.Scan(&rp.ID, &rp.CameraID, &rp.HotDays, &rp.WarmDays, &rp.ColdDays,
			&rp.DeleteAfterDays, &rp.ArchiveTier, &rp.CreatedAt, &rp.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, rp)
	}
	return out, rows.Err()
}

func (s *Store) DeleteRetention(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM retention_policies WHERE id = ?`, id)
	return err
}

// --- Alert Rules ------------------------------------------------------------

// AlertRule represents a fleet-level alert configuration.
type AlertRule struct {
	ID              string  `json:"id"`
	Name            string  `json:"name"`
	RuleType        string  `json:"rule_type"`
	ThresholdValue  float64 `json:"threshold_value"`
	CameraID        string  `json:"camera_id,omitempty"`
	RecorderID      string  `json:"recorder_id,omitempty"`
	Enabled         bool    `json:"enabled"`
	NotifyEmail     bool    `json:"notify_email"`
	CooldownMinutes int     `json:"cooldown_minutes"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

func (s *Store) CreateAlertRule(ctx context.Context, ar AlertRule) error {
	enabled, notify := 0, 0
	if ar.Enabled {
		enabled = 1
	}
	if ar.NotifyEmail {
		notify = 1
	}
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO alert_rules (id, tenant_id, name, rule_type, threshold_value, camera_id, recorder_id, enabled, notify_email, cooldown_minutes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, ar.ID, defaultTenant, ar.Name, ar.RuleType, ar.ThresholdValue,
		nilIfEmpty(ar.CameraID), nilIfEmpty(ar.RecorderID), enabled, notify, ar.CooldownMinutes)
	return err
}

func (s *Store) ListAlertRules(ctx context.Context) ([]AlertRule, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, rule_type, threshold_value, COALESCE(camera_id,''),
		       COALESCE(recorder_id,''), enabled, notify_email, cooldown_minutes, created_at, updated_at
		FROM alert_rules ORDER BY name
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AlertRule
	for rows.Next() {
		var ar AlertRule
		var enabled, notify int
		if err := rows.Scan(&ar.ID, &ar.Name, &ar.RuleType, &ar.ThresholdValue,
			&ar.CameraID, &ar.RecorderID, &enabled, &notify, &ar.CooldownMinutes,
			&ar.CreatedAt, &ar.UpdatedAt); err != nil {
			return nil, err
		}
		ar.Enabled = enabled == 1
		ar.NotifyEmail = notify == 1
		out = append(out, ar)
	}
	return out, rows.Err()
}

func (s *Store) UpdateAlertRule(ctx context.Context, ar AlertRule) error {
	enabled, notify := 0, 0
	if ar.Enabled {
		enabled = 1
	}
	if ar.NotifyEmail {
		notify = 1
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE alert_rules SET name=?, rule_type=?, threshold_value=?, camera_id=?,
		       recorder_id=?, enabled=?, notify_email=?, cooldown_minutes=?, updated_at=CURRENT_TIMESTAMP
		WHERE id = ?
	`, ar.Name, ar.RuleType, ar.ThresholdValue, nilIfEmpty(ar.CameraID),
		nilIfEmpty(ar.RecorderID), enabled, notify, ar.CooldownMinutes, ar.ID)
	return err
}

func (s *Store) DeleteAlertRule(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM alert_rules WHERE id = ?`, id)
	return err
}

// --- Audit Log --------------------------------------------------------------

// AuditEntry represents a single audit event.
type AuditEntry struct {
	ID           int64  `json:"id"`
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
	RecorderID   string `json:"recorder_id"`
	Action       string `json:"action"`
	ResourceType string `json:"resource_type"`
	ResourceID   string `json:"resource_id"`
	Details      string `json:"details"`
	IPAddress    string `json:"ip_address"`
	CreatedAt    string `json:"created_at"`
}

func (s *Store) InsertAudit(ctx context.Context, e AuditEntry) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO audit_entries (tenant_id, user_id, username, recorder_id, action, resource_type, resource_id, details, ip_address)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, defaultTenant, e.UserID, e.Username, e.RecorderID, e.Action, e.ResourceType, e.ResourceID, e.Details, e.IPAddress)
	return err
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, user_id, username, recorder_id, action, resource_type, resource_id, details, ip_address, created_at
		FROM audit_entries ORDER BY created_at DESC LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AuditEntry
	for rows.Next() {
		var e AuditEntry
		if err := rows.Scan(&e.ID, &e.UserID, &e.Username, &e.RecorderID, &e.Action,
			&e.ResourceType, &e.ResourceID, &e.Details, &e.IPAddress, &e.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- Export Jobs -------------------------------------------------------------

// ExportJob represents a Directory-orchestrated export request.
type ExportJob struct {
	ID           string `json:"id"`
	RecorderID   string `json:"recorder_id"`
	CameraID     string `json:"camera_id"`
	StartTime    string `json:"start_time"`
	EndTime      string `json:"end_time"`
	Format       string `json:"format"`
	Status       string `json:"status"`
	RequestedBy  string `json:"requested_by"`
	ErrorMessage string `json:"error_message,omitempty"`
	DownloadURL  string `json:"download_url,omitempty"`
	CreatedAt    string `json:"created_at"`
	UpdatedAt    string `json:"updated_at"`
}

func (s *Store) CreateExportJob(ctx context.Context, j ExportJob) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO export_jobs (id, tenant_id, recorder_id, camera_id, start_time, end_time, format, requested_by)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, j.ID, defaultTenant, j.RecorderID, j.CameraID, j.StartTime, j.EndTime, j.Format, j.RequestedBy)
	return err
}

func (s *Store) ListExportJobs(ctx context.Context) ([]ExportJob, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, recorder_id, camera_id, start_time, end_time, format, status,
		       requested_by, error_message, download_url, created_at, updated_at
		FROM export_jobs ORDER BY created_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ExportJob
	for rows.Next() {
		var j ExportJob
		if err := rows.Scan(&j.ID, &j.RecorderID, &j.CameraID, &j.StartTime, &j.EndTime,
			&j.Format, &j.Status, &j.RequestedBy, &j.ErrorMessage, &j.DownloadURL,
			&j.CreatedAt, &j.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}

func (s *Store) UpdateExportJobStatus(ctx context.Context, id, status, errorMsg, downloadURL string) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE export_jobs SET status=?, error_message=?, download_url=?, updated_at=CURRENT_TIMESTAMP
		WHERE id = ?
	`, status, errorMsg, downloadURL, id)
	return err
}

func (s *Store) GetExportJob(ctx context.Context, id string) (ExportJob, error) {
	var j ExportJob
	err := s.db.QueryRowContext(ctx, `
		SELECT id, recorder_id, camera_id, start_time, end_time, format, status,
		       requested_by, error_message, download_url, created_at, updated_at
		FROM export_jobs WHERE id = ?
	`, id).Scan(&j.ID, &j.RecorderID, &j.CameraID, &j.StartTime, &j.EndTime,
		&j.Format, &j.Status, &j.RequestedBy, &j.ErrorMessage, &j.DownloadURL,
		&j.CreatedAt, &j.UpdatedAt)
	return j, err
}

// nilIfEmpty returns nil for empty strings (used for nullable FK columns).
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// unused but shows planned recording schedule push shape
var _ = time.Now
var _ = fmt.Sprintf
