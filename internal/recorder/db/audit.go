package db

import (
	"fmt"
	"strings"
	"time"
)

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
