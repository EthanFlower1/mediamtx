package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// CameraPermission represents per-camera permission assignments for a user.
type CameraPermission struct {
	ID          string `json:"id"`
	UserID      string `json:"user_id"`
	CameraID    string `json:"camera_id"`
	Permissions string `json:"permissions"` // JSON array: ["view_live","view_playback","export","ptz_control"]
	CreatedAt   string `json:"created_at"`
}

// SetCameraPermission inserts or replaces camera permissions for a user+camera pair.
func (d *DB) SetCameraPermission(cp *CameraPermission) error {
	if cp.ID == "" {
		cp.ID = uuid.New().String()
	}
	cp.CreatedAt = time.Now().UTC().Format(timeFormat)

	_, err := d.Exec(`
		INSERT INTO camera_permissions (id, user_id, camera_id, permissions, created_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, camera_id) DO UPDATE SET
			permissions = excluded.permissions`,
		cp.ID, cp.UserID, cp.CameraID, cp.Permissions, cp.CreatedAt,
	)
	return err
}

// GetCameraPermission retrieves a single camera permission by user+camera.
func (d *DB) GetCameraPermission(userID, cameraID string) (*CameraPermission, error) {
	cp := &CameraPermission{}
	err := d.QueryRow(`
		SELECT id, user_id, camera_id, permissions, created_at
		FROM camera_permissions WHERE user_id = ? AND camera_id = ?`,
		userID, cameraID,
	).Scan(&cp.ID, &cp.UserID, &cp.CameraID, &cp.Permissions, &cp.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return cp, nil
}

// ListCameraPermissions returns all camera permissions for a user.
func (d *DB) ListCameraPermissions(userID string) ([]*CameraPermission, error) {
	rows, err := d.Query(`
		SELECT id, user_id, camera_id, permissions, created_at
		FROM camera_permissions WHERE user_id = ? ORDER BY camera_id`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []*CameraPermission
	for rows.Next() {
		cp := &CameraPermission{}
		if err := rows.Scan(&cp.ID, &cp.UserID, &cp.CameraID, &cp.Permissions, &cp.CreatedAt); err != nil {
			return nil, err
		}
		perms = append(perms, cp)
	}
	return perms, rows.Err()
}

// DeleteCameraPermission removes a specific camera permission by user+camera.
func (d *DB) DeleteCameraPermission(userID, cameraID string) error {
	res, err := d.Exec(
		"DELETE FROM camera_permissions WHERE user_id = ? AND camera_id = ?",
		userID, cameraID,
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

// DeleteAllCameraPermissions removes all camera permissions for a user.
func (d *DB) DeleteAllCameraPermissions(userID string) error {
	_, err := d.Exec("DELETE FROM camera_permissions WHERE user_id = ?", userID)
	return err
}

// SetBulkCameraPermissions replaces all camera permissions for a user in a transaction.
func (d *DB) SetBulkCameraPermissions(userID string, perms []*CameraPermission) error {
	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("DELETE FROM camera_permissions WHERE user_id = ?", userID)
	if err != nil {
		return err
	}

	now := time.Now().UTC().Format(timeFormat)
	for _, cp := range perms {
		if cp.ID == "" {
			cp.ID = uuid.New().String()
		}
		cp.UserID = userID
		cp.CreatedAt = now
		_, err = tx.Exec(`
			INSERT INTO camera_permissions (id, user_id, camera_id, permissions, created_at)
			VALUES (?, ?, ?, ?, ?)`,
			cp.ID, cp.UserID, cp.CameraID, cp.Permissions, cp.CreatedAt,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// UserHasCameraPermission checks if a user has a specific permission for a camera.
// Returns true if the user's role is admin, if user has wildcard camera_permissions ("*"),
// or if the specific permission exists in their camera_permissions table entry.
func (d *DB) UserHasCameraPermission(userID, cameraID, permission string) (bool, error) {
	user, err := d.GetUser(userID)
	if err != nil {
		return false, err
	}

	// Admin role always has all permissions.
	if user.Role == "admin" {
		return true, nil
	}

	// Wildcard camera permissions (legacy support).
	if user.CameraPermissions == "*" {
		return true, nil
	}

	// Check the camera_permissions table for granular per-camera check.
	cp, err := d.GetCameraPermission(userID, cameraID)
	if errors.Is(err, ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	// Parse the permissions JSON array and check for the specific permission.
	var parsedPerms []string
	if err := json.Unmarshal([]byte(cp.Permissions), &parsedPerms); err != nil {
		return false, nil
	}
	for _, p := range parsedPerms {
		if p == permission {
			return true, nil
		}
	}
	return false, nil
}
