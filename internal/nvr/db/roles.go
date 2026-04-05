package db

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Permission constants for granular access control.
const (
	PermViewLive    = "view_live"
	PermViewPlayback = "view_playback"
	PermExport      = "export"
	PermPTZControl  = "ptz_control"
	PermAdmin       = "admin"
)

// AllPermissions is the complete list of valid permissions.
var AllPermissions = []string{
	PermViewLive,
	PermViewPlayback,
	PermExport,
	PermPTZControl,
	PermAdmin,
}

// Role represents a named role with a set of permissions.
type Role struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
	IsSystem    bool     `json:"is_system"`
	CreatedAt   string   `json:"created_at"`
	UpdatedAt   string   `json:"updated_at"`
}

// UserCameraPermission represents per-camera permission overrides for a user.
type UserCameraPermission struct {
	ID          string   `json:"id"`
	UserID      string   `json:"user_id"`
	CameraID    string   `json:"camera_id"`
	Permissions []string `json:"permissions"`
}

// CreateRole inserts a new role into the database.
func (d *DB) CreateRole(r *Role) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}

	now := time.Now().UTC().Format(timeFormat)
	r.CreatedAt = now
	r.UpdatedAt = now

	permsJSON, err := json.Marshal(r.Permissions)
	if err != nil {
		return err
	}

	_, err = d.Exec(`
		INSERT INTO roles (id, name, description, permissions, is_system, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.Description, string(permsJSON), boolToInt(r.IsSystem),
		r.CreatedAt, r.UpdatedAt,
	)
	return err
}

// GetRole retrieves a role by ID. Returns ErrNotFound if no match.
func (d *DB) GetRole(id string) (*Role, error) {
	r := &Role{}
	var permsJSON string
	var isSystem int
	err := d.QueryRow(`
		SELECT id, name, description, permissions, is_system, created_at, updated_at
		FROM roles WHERE id = ?`, id,
	).Scan(&r.ID, &r.Name, &r.Description, &permsJSON, &isSystem,
		&r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.IsSystem = isSystem != 0
	if err := json.Unmarshal([]byte(permsJSON), &r.Permissions); err != nil {
		r.Permissions = []string{}
	}
	return r, nil
}

// GetRoleByName retrieves a role by name. Returns ErrNotFound if no match.
func (d *DB) GetRoleByName(name string) (*Role, error) {
	r := &Role{}
	var permsJSON string
	var isSystem int
	err := d.QueryRow(`
		SELECT id, name, description, permissions, is_system, created_at, updated_at
		FROM roles WHERE name = ?`, name,
	).Scan(&r.ID, &r.Name, &r.Description, &permsJSON, &isSystem,
		&r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.IsSystem = isSystem != 0
	if err := json.Unmarshal([]byte(permsJSON), &r.Permissions); err != nil {
		r.Permissions = []string{}
	}
	return r, nil
}

// ListRoles returns all roles ordered by name.
func (d *DB) ListRoles() ([]*Role, error) {
	rows, err := d.Query(`
		SELECT id, name, description, permissions, is_system, created_at, updated_at
		FROM roles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []*Role
	for rows.Next() {
		r := &Role{}
		var permsJSON string
		var isSystem int
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &permsJSON, &isSystem,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.IsSystem = isSystem != 0
		if err := json.Unmarshal([]byte(permsJSON), &r.Permissions); err != nil {
			r.Permissions = []string{}
		}
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

// UpdateRole updates an existing role. System roles cannot be modified.
func (d *DB) UpdateRole(r *Role) error {
	// Check if this is a system role.
	existing, err := d.GetRole(r.ID)
	if err != nil {
		return err
	}
	if existing.IsSystem {
		return errors.New("cannot modify system role")
	}

	r.UpdatedAt = time.Now().UTC().Format(timeFormat)

	permsJSON, err := json.Marshal(r.Permissions)
	if err != nil {
		return err
	}

	res, err := d.Exec(`
		UPDATE roles SET name = ?, description = ?, permissions = ?, updated_at = ?
		WHERE id = ?`,
		r.Name, r.Description, string(permsJSON), r.UpdatedAt, r.ID,
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

// DeleteRole deletes a role by ID. System roles cannot be deleted.
func (d *DB) DeleteRole(id string) error {
	existing, err := d.GetRole(id)
	if err != nil {
		return err
	}
	if existing.IsSystem {
		return errors.New("cannot delete system role")
	}

	res, err := d.Exec("DELETE FROM roles WHERE id = ?", id)
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

// SetUserCameraPermissions sets the per-camera permissions for a user on a
// specific camera. If permissions is nil or empty, it removes the override.
func (d *DB) SetUserCameraPermissions(userID, cameraID string, permissions []string) error {
	if len(permissions) == 0 {
		_, err := d.Exec(
			"DELETE FROM user_camera_permissions WHERE user_id = ? AND camera_id = ?",
			userID, cameraID,
		)
		return err
	}

	permsJSON, err := json.Marshal(permissions)
	if err != nil {
		return err
	}

	id := uuid.New().String()
	_, err = d.Exec(`
		INSERT INTO user_camera_permissions (id, user_id, camera_id, permissions)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(user_id, camera_id) DO UPDATE SET permissions = excluded.permissions`,
		id, userID, cameraID, string(permsJSON),
	)
	return err
}

// GetUserCameraPermissions returns all per-camera permission overrides for a user.
func (d *DB) GetUserCameraPermissions(userID string) ([]UserCameraPermission, error) {
	rows, err := d.Query(`
		SELECT id, user_id, camera_id, permissions
		FROM user_camera_permissions WHERE user_id = ?`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var perms []UserCameraPermission
	for rows.Next() {
		var p UserCameraPermission
		var permsJSON string
		if err := rows.Scan(&p.ID, &p.UserID, &p.CameraID, &permsJSON); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(permsJSON), &p.Permissions); err != nil {
			p.Permissions = []string{}
		}
		perms = append(perms, p)
	}
	return perms, rows.Err()
}

// GetUserCameraPermission returns the per-camera permissions for a user on a
// specific camera. Returns ErrNotFound if no override exists.
func (d *DB) GetUserCameraPermission(userID, cameraID string) (*UserCameraPermission, error) {
	var p UserCameraPermission
	var permsJSON string
	err := d.QueryRow(`
		SELECT id, user_id, camera_id, permissions
		FROM user_camera_permissions WHERE user_id = ? AND camera_id = ?`,
		userID, cameraID,
	).Scan(&p.ID, &p.UserID, &p.CameraID, &permsJSON)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(permsJSON), &p.Permissions); err != nil {
		p.Permissions = []string{}
	}
	return &p, nil
}

// DeleteUserCameraPermissions removes all per-camera permission overrides for a user.
func (d *DB) DeleteUserCameraPermissions(userID string) error {
	_, err := d.Exec("DELETE FROM user_camera_permissions WHERE user_id = ?", userID)
	return err
}
