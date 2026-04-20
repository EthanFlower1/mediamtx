package legacydb

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

// Notification represents a persisted system notification.
type Notification struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Severity  string    `json:"severity"`
	Camera    string    `json:"camera"`
	Message   string    `json:"message"`
	CreatedAt time.Time `json:"created_at"`
	ReadAt    *string   `json:"read_at"`
	Archived  bool      `json:"archived"`
}

// NotificationFilter specifies query parameters for listing notifications.
type NotificationFilter struct {
	UserID   string
	Camera   string
	Type     string
	Severity string
	Query    string  // free-text search across message and camera
	Read     *bool   // nil = all, true = read only, false = unread only
	Archived bool    // false = inbox, true = archived
	Since    *time.Time
	Until    *time.Time
	Limit    int
	Offset   int
}

// InsertNotification persists a new notification. It assigns a UUID if the ID
// is empty.
func (d *DB) InsertNotification(n *Notification) error {
	if n.ID == "" {
		n.ID = uuid.New().String()
	}
	if n.CreatedAt.IsZero() {
		n.CreatedAt = time.Now().UTC()
	}

	_, err := d.Exec(
		`INSERT INTO notifications (id, type, severity, camera, message, created_at)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		n.ID, n.Type, n.Severity, n.Camera, n.Message,
		n.CreatedAt.Format(time.RFC3339),
	)
	return err
}

// ListNotifications retrieves notifications with per-user read/archive state.
func (d *DB) ListNotifications(f NotificationFilter) ([]*Notification, int, error) {
	// Build the WHERE clause dynamically.
	var conditions []string
	var args []interface{}

	// Always join on the read-state table with a LEFT JOIN so we can
	// filter on read/archived while still returning notifications the
	// user hasn't interacted with yet.

	// Archived filter
	if f.Archived {
		conditions = append(conditions, "COALESCE(rs.archived, 0) = 1")
	} else {
		conditions = append(conditions, "COALESCE(rs.archived, 0) = 0")
	}

	// Read filter
	if f.Read != nil {
		if *f.Read {
			conditions = append(conditions, "rs.read_at IS NOT NULL AND rs.read_at != ''")
		} else {
			conditions = append(conditions, "(rs.read_at IS NULL OR rs.read_at = '')")
		}
	}

	if f.Camera != "" {
		conditions = append(conditions, "n.camera = ?")
		args = append(args, f.Camera)
	}

	if f.Type != "" {
		conditions = append(conditions, "n.type = ?")
		args = append(args, f.Type)
	}

	if f.Severity != "" {
		conditions = append(conditions, "n.severity = ?")
		args = append(args, f.Severity)
	}

	if f.Query != "" {
		conditions = append(conditions, "(n.message LIKE ? OR n.camera LIKE ?)")
		q := "%" + f.Query + "%"
		args = append(args, q, q)
	}

	if f.Since != nil {
		conditions = append(conditions, "n.created_at >= ?")
		args = append(args, f.Since.Format(time.RFC3339))
	}

	if f.Until != nil {
		conditions = append(conditions, "n.created_at <= ?")
		args = append(args, f.Until.Format(time.RFC3339))
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	baseQuery := fmt.Sprintf(`
		FROM notifications n
		LEFT JOIN notification_read_state rs
			ON rs.notification_id = n.id AND rs.user_id = ?
		%s`, whereClause)

	// Count total matching rows.
	countArgs := append([]interface{}{f.UserID}, args...)
	var total int
	err := d.QueryRow("SELECT COUNT(*) "+baseQuery, countArgs...).Scan(&total)
	if err != nil {
		return nil, 0, fmt.Errorf("count notifications: %w", err)
	}

	// Fetch the page.
	selectQuery := fmt.Sprintf(`
		SELECT n.id, n.type, n.severity, n.camera, n.message, n.created_at,
		       rs.read_at, COALESCE(rs.archived, 0)
		%s
		ORDER BY n.created_at DESC
		LIMIT ? OFFSET ?`, baseQuery)

	selectArgs := append(countArgs, f.Limit, f.Offset)
	rows, err := d.Query(selectQuery, selectArgs...)
	if err != nil {
		return nil, 0, fmt.Errorf("query notifications: %w", err)
	}
	defer rows.Close()

	var results []*Notification
	for rows.Next() {
		var n Notification
		var createdStr string
		var readAt *string
		var archived int

		if err := rows.Scan(&n.ID, &n.Type, &n.Severity, &n.Camera, &n.Message,
			&createdStr, &readAt, &archived); err != nil {
			return nil, 0, fmt.Errorf("scan notification: %w", err)
		}

		if t, err := time.Parse(time.RFC3339, createdStr); err == nil {
			n.CreatedAt = t
		}
		if readAt != nil && *readAt != "" {
			n.ReadAt = readAt
		}
		n.Archived = archived == 1
		results = append(results, &n)
	}

	if results == nil {
		results = []*Notification{}
	}

	return results, total, nil
}

// UnreadNotificationCount returns the count of non-archived, unread notifications.
func (d *DB) UnreadNotificationCount(userID string) (int, error) {
	var count int
	err := d.QueryRow(`
		SELECT COUNT(*)
		FROM notifications n
		LEFT JOIN notification_read_state rs
			ON rs.notification_id = n.id AND rs.user_id = ?
		WHERE COALESCE(rs.archived, 0) = 0
		  AND (rs.read_at IS NULL OR rs.read_at = '')
	`, userID).Scan(&count)
	return count, err
}

// MarkNotificationsRead sets the read_at timestamp for the given notification IDs.
func (d *DB) MarkNotificationsRead(userID string, ids []string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, id := range ids {
		_, err := d.Exec(`
			INSERT INTO notification_read_state (notification_id, user_id, read_at, archived)
			VALUES (?, ?, ?, 0)
			ON CONFLICT(notification_id, user_id) DO UPDATE SET read_at = ?
		`, id, userID, now, now)
		if err != nil {
			return fmt.Errorf("mark read %s: %w", id, err)
		}
	}
	return nil
}

// MarkNotificationsUnread clears the read_at for the given notification IDs.
func (d *DB) MarkNotificationsUnread(userID string, ids []string) error {
	for _, id := range ids {
		_, err := d.Exec(`
			INSERT INTO notification_read_state (notification_id, user_id, read_at, archived)
			VALUES (?, ?, '', 0)
			ON CONFLICT(notification_id, user_id) DO UPDATE SET read_at = ''
		`, id, userID)
		if err != nil {
			return fmt.Errorf("mark unread %s: %w", id, err)
		}
	}
	return nil
}

// MarkAllNotificationsRead marks all non-archived notifications as read.
func (d *DB) MarkAllNotificationsRead(userID string) (int, error) {
	now := time.Now().UTC().Format(time.RFC3339)

	// Insert read state for all notifications that don't have one.
	_, err := d.Exec(`
		INSERT OR IGNORE INTO notification_read_state (notification_id, user_id, read_at, archived)
		SELECT n.id, ?, ?, 0
		FROM notifications n
		LEFT JOIN notification_read_state rs
			ON rs.notification_id = n.id AND rs.user_id = ?
		WHERE rs.notification_id IS NULL
	`, userID, now, userID)
	if err != nil {
		return 0, err
	}

	// Update existing unread ones.
	result, err := d.Exec(`
		UPDATE notification_read_state
		SET read_at = ?
		WHERE user_id = ? AND (read_at IS NULL OR read_at = '') AND archived = 0
	`, now, userID)
	if err != nil {
		return 0, err
	}

	count, _ := result.RowsAffected()
	return int(count), nil
}

// ArchiveNotifications moves the specified notifications to the archive.
func (d *DB) ArchiveNotifications(userID string, ids []string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	for _, id := range ids {
		_, err := d.Exec(`
			INSERT INTO notification_read_state (notification_id, user_id, read_at, archived)
			VALUES (?, ?, ?, 1)
			ON CONFLICT(notification_id, user_id) DO UPDATE SET archived = 1, read_at = CASE WHEN read_at = '' THEN ? ELSE read_at END
		`, id, userID, now, now)
		if err != nil {
			return fmt.Errorf("archive %s: %w", id, err)
		}
	}
	return nil
}

// RestoreNotifications moves notifications out of the archive.
func (d *DB) RestoreNotifications(userID string, ids []string) error {
	for _, id := range ids {
		_, err := d.Exec(`
			UPDATE notification_read_state SET archived = 0
			WHERE notification_id = ? AND user_id = ?
		`, id, userID)
		if err != nil {
			return fmt.Errorf("restore %s: %w", id, err)
		}
	}
	return nil
}

// DeleteNotifications permanently removes notifications (and their read state).
func (d *DB) DeleteNotifications(userID string, ids []string) error {
	// Only admins can delete; we delete the notification row which cascades
	// to the read_state rows for all users.
	for _, id := range ids {
		_, err := d.Exec(`DELETE FROM notifications WHERE id = ?`, id)
		if err != nil {
			return fmt.Errorf("delete %s: %w", id, err)
		}
	}
	return nil
}
