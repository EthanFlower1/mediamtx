package db

import (
	"fmt"
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

// QueryAuditLog returns audit log entries with pagination and optional filters.
// It returns the entries and the total count of matching entries.
func (d *DB) QueryAuditLog(limit, offset int, userID, action string) ([]*AuditEntry, int, error) {
	var conditions []string
	var args []interface{}

	if userID != "" {
		conditions = append(conditions, "user_id = ?")
		args = append(args, userID)
	}
	if action != "" {
		conditions = append(conditions, "action = ?")
		args = append(args, action)
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

	queryArgs := append(args, limit, offset)
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
