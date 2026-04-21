package legacynvrapi

// audit.go — Audit log endpoint for the legacy NVR API.
//
// GET /api/nvr/audit — Returns paginated audit log entries.
//
// The recorder DB's audit_log table is used (via direct SQL query since there
// is no dedicated ListAuditEntries method yet). Query params:
//   limit  — max rows to return (default 100, max 500)
//   offset — skip this many rows (for pagination)
//   action — filter by action name

import (
	"database/sql"
	"net/http"
	"strconv"
	"time"
)

// auditEntry mirrors a row from the recorder DB's audit_log table.
// Fields are intentionally flexible (using *string for nullable columns).
type auditEntry struct {
	ID        int64   `json:"id"`
	Action    string  `json:"action"`
	UserID    *string `json:"user_id,omitempty"`
	CameraID  *string `json:"camera_id,omitempty"`
	Detail    *string `json:"detail,omitempty"`
	IPAddress *string `json:"ip_address,omitempty"`
	CreatedAt string  `json:"created_at"`
}

func (h *Handlers) auditLog(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	if h.RecDB == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]string{
			"code":    "DB_UNAVAILABLE",
			"message": "recorder database not available",
		})
		return
	}

	// Parse query params.
	q := r.URL.Query()

	limit := 100
	if v := q.Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			if n > 500 {
				n = 500
			}
			limit = n
		}
	}

	offset := 0
	if v := q.Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}

	action := q.Get("action")

	// Build query.
	query := `SELECT id, action, user_id, camera_id, detail, ip_address, created_at
	          FROM audit_log`
	var args []any

	if action != "" {
		query += " WHERE action = ?"
		args = append(args, action)
	}

	query += " ORDER BY created_at DESC LIMIT ? OFFSET ?"
	args = append(args, limit, offset)

	rows, err := h.RecDB.Query(query, args...)
	if err != nil {
		// Table may not exist in all schema versions — return empty list gracefully.
		writeJSON(w, http.StatusOK, []any{})
		return
	}
	defer rows.Close()

	entries := []auditEntry{}
	for rows.Next() {
		var e auditEntry
		var createdAt *string
		if err := rows.Scan(&e.ID, &e.Action, &e.UserID, &e.CameraID, &e.Detail, &e.IPAddress, &createdAt); err != nil {
			if err == sql.ErrNoRows {
				break
			}
			continue
		}
		if createdAt != nil {
			e.CreatedAt = *createdAt
		} else {
			e.CreatedAt = time.Now().UTC().Format(time.RFC3339)
		}
		entries = append(entries, e)
	}

	writeJSON(w, http.StatusOK, entries)
}
