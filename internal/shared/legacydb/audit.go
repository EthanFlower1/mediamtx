package legacydb

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
	"time"
)

// AuditEntry represents a single audit log entry.
type AuditEntry struct {
	ID           int64  `json:"id"`
	UserID       string `json:"user_id"`
	Username     string `json:"username"`
	Action       string `json:"action"`        // "create", "update", "delete", "login", "logout", "login_failed"
	ResourceType string `json:"resource_type"` // "camera", "user", "recording_rule", "system"
	ResourceID   string `json:"resource_id"`
	Details      string `json:"details"`
	IPAddress    string `json:"ip_address"`
	CreatedAt    string `json:"created_at"`
}

// InsertAuditEntry inserts a new audit log entry.
func (d *DB) InsertAuditEntry(entry *AuditEntry) error {
	if entry.CreatedAt == "" {
		entry.CreatedAt = time.Now().UTC().Format(timeFormat)
	}

	res, err := d.Exec(`
		INSERT INTO audit_log (user_id, username, action, resource_type, resource_id, details, ip_address, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		entry.UserID, entry.Username, entry.Action, entry.ResourceType,
		entry.ResourceID, entry.Details, entry.IPAddress, entry.CreatedAt,
	)
	if err != nil {
		return err
	}

	id, err := res.LastInsertId()
	if err != nil {
		return err
	}
	entry.ID = id
	return nil
}

// AuditQueryParams holds all optional filters for querying the audit log.
type AuditQueryParams struct {
	Limit        int
	Offset       int
	UserID       string
	Action       string
	ResourceType string
	Search       string    // free-text search across username, resource_type, resource_id, details, ip_address
	From         time.Time // zero value means no lower bound
	To           time.Time // zero value means no upper bound
}

// QueryAuditLog returns audit log entries with pagination and optional filters.
// It returns the entries and the total count of matching entries.
func (d *DB) QueryAuditLog(limit, offset int, userID, action string) ([]*AuditEntry, int, error) {
	return d.QueryAuditLogAdvanced(AuditQueryParams{
		Limit:  limit,
		Offset: offset,
		UserID: userID,
		Action: action,
	})
}

// QueryAuditLogAdvanced returns audit log entries using the full set of filters.
func (d *DB) QueryAuditLogAdvanced(p AuditQueryParams) ([]*AuditEntry, int, error) {
	var conditions []string
	var args []interface{}

	if p.UserID != "" {
		conditions = append(conditions, "user_id = ?")
		args = append(args, p.UserID)
	}
	if p.Action != "" {
		conditions = append(conditions, "action = ?")
		args = append(args, p.Action)
	}
	if p.ResourceType != "" {
		conditions = append(conditions, "resource_type = ?")
		args = append(args, p.ResourceType)
	}
	if !p.From.IsZero() {
		conditions = append(conditions, "created_at >= ?")
		args = append(args, p.From.UTC().Format(timeFormat))
	}
	if !p.To.IsZero() {
		conditions = append(conditions, "created_at <= ?")
		args = append(args, p.To.UTC().Format(timeFormat))
	}
	if p.Search != "" {
		q := "%" + p.Search + "%"
		conditions = append(conditions,
			"(username LIKE ? OR resource_type LIKE ? OR resource_id LIKE ? OR details LIKE ? OR ip_address LIKE ?)")
		args = append(args, q, q, q, q, q)
	}

	where := ""
	if len(conditions) > 0 {
		where = "WHERE " + strings.Join(conditions, " AND ")
	}

	// Get total count.
	var total int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM audit_log %s", where)
	if err := d.QueryRow(countQuery, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	// Get entries.
	query := fmt.Sprintf(`
		SELECT id, user_id, username, action, resource_type, resource_id, details, ip_address, created_at
		FROM audit_log %s
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?`, where)

	queryArgs := append(args, p.Limit, p.Offset)
	rows, err := d.Query(query, queryArgs...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		e := &AuditEntry{}
		if err := rows.Scan(
			&e.ID, &e.UserID, &e.Username, &e.Action, &e.ResourceType,
			&e.ResourceID, &e.Details, &e.IPAddress, &e.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return entries, total, nil
}

// SecurityActions lists audit actions classified as security events.
// These are retained longer than general audit entries.
var SecurityActions = []string{
	"login_failed",
	"login",
	"logout",
	"permission_change",
	"password_change",
	"user_create",
	"user_delete",
}

// isSecurityAction returns true if the action is a security event.
func isSecurityAction(action string) bool {
	for _, sa := range SecurityActions {
		if action == sa {
			return true
		}
	}
	return false
}

// securityActionPlaceholders returns the SQL placeholder string and args for
// SecurityActions (e.g. "?,?,?" and the slice of interface{} values).
func securityActionPlaceholders() (string, []interface{}) {
	placeholders := make([]string, len(SecurityActions))
	args := make([]interface{}, len(SecurityActions))
	for i, a := range SecurityActions {
		placeholders[i] = "?"
		args[i] = a
	}
	return strings.Join(placeholders, ","), args
}

// DeleteAuditEntriesBefore deletes all audit log entries created before the
// given time. This is used for retention cleanup.
func (d *DB) DeleteAuditEntriesBefore(before time.Time) error {
	_, err := d.Exec("DELETE FROM audit_log WHERE created_at < ?", before.UTC().Format(timeFormat))
	return err
}

// DeleteGeneralAuditEntriesBefore deletes non-security audit entries older
// than the given time. Security events (login, logout, login_failed, etc.)
// are preserved for separate, longer retention.
func (d *DB) DeleteGeneralAuditEntriesBefore(before time.Time) (int64, error) {
	ph, phArgs := securityActionPlaceholders()
	query := fmt.Sprintf(
		"DELETE FROM audit_log WHERE created_at < ? AND action NOT IN (%s)", ph)
	args := append([]interface{}{before.UTC().Format(timeFormat)}, phArgs...)
	res, err := d.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DeleteSecurityAuditEntriesBefore deletes security audit entries older
// than the given time. Security events have a separate (typically longer)
// retention period.
func (d *DB) DeleteSecurityAuditEntriesBefore(before time.Time) (int64, error) {
	ph, phArgs := securityActionPlaceholders()
	query := fmt.Sprintf(
		"DELETE FROM audit_log WHERE created_at < ? AND action IN (%s)", ph)
	args := append([]interface{}{before.UTC().Format(timeFormat)}, phArgs...)
	res, err := d.Exec(query, args...)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// QueryAuditLogByDateRange returns all audit log entries within the given time
// range, with optional action and user filters. Used for export.
func (d *DB) QueryAuditLogByDateRange(from, to time.Time, userID, action string) ([]*AuditEntry, error) {
	var conditions []string
	var args []interface{}

	conditions = append(conditions, "created_at >= ?")
	args = append(args, from.UTC().Format(timeFormat))

	conditions = append(conditions, "created_at <= ?")
	args = append(args, to.UTC().Format(timeFormat))

	if userID != "" {
		conditions = append(conditions, "user_id = ?")
		args = append(args, userID)
	}
	if action != "" {
		conditions = append(conditions, "action = ?")
		args = append(args, action)
	}

	where := "WHERE " + strings.Join(conditions, " AND ")

	query := fmt.Sprintf(`
		SELECT id, user_id, username, action, resource_type, resource_id, details, ip_address, created_at
		FROM audit_log %s
		ORDER BY created_at ASC`, where)

	rows, err := d.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		e := &AuditEntry{}
		if err := rows.Scan(
			&e.ID, &e.UserID, &e.Username, &e.Action, &e.ResourceType,
			&e.ResourceID, &e.Details, &e.IPAddress, &e.CreatedAt,
		); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	return entries, nil
}

// WriteAuditCSV writes audit entries to the given writer in CSV format.
func WriteAuditCSV(w io.Writer, entries []*AuditEntry) error {
	cw := csv.NewWriter(w)
	defer cw.Flush()

	// Header row.
	if err := cw.Write([]string{
		"id", "user_id", "username", "action", "resource_type",
		"resource_id", "details", "ip_address", "created_at",
	}); err != nil {
		return err
	}

	for _, e := range entries {
		if err := cw.Write([]string{
			fmt.Sprintf("%d", e.ID),
			e.UserID,
			e.Username,
			e.Action,
			e.ResourceType,
			e.ResourceID,
			e.Details,
			e.IPAddress,
			e.CreatedAt,
		}); err != nil {
			return err
		}
	}

	return cw.Error()
}
