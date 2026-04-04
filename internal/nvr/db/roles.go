package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Role represents a custom role with a set of permissions.
type Role struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Permissions string `json:"permissions"` // JSON array of permission strings
	IsSystem    bool   `json:"is_system"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// CreateRole inserts a new role into the database.
func (d *DB) CreateRole(r *Role) error {
	if r.ID == "" {
		r.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format(timeFormat)
	r.CreatedAt = now
	r.UpdatedAt = now

	_, err := d.Exec(`
		INSERT INTO roles (id, name, permissions, is_system, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?)`,
		r.ID, r.Name, r.Permissions, boolToInt(r.IsSystem), r.CreatedAt, r.UpdatedAt,
	)
	return err
}

// GetRole retrieves a role by ID.
func (d *DB) GetRole(id string) (*Role, error) {
	r := &Role{}
	var isSystem int
	err := d.QueryRow(`
		SELECT id, name, permissions, is_system, created_at, updated_at
		FROM roles WHERE id = ?`, id,
	).Scan(&r.ID, &r.Name, &r.Permissions, &isSystem, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.IsSystem = isSystem != 0
	return r, nil
}

// GetRoleByName retrieves a role by name.
func (d *DB) GetRoleByName(name string) (*Role, error) {
	r := &Role{}
	var isSystem int
	err := d.QueryRow(`
		SELECT id, name, permissions, is_system, created_at, updated_at
		FROM roles WHERE name = ?`, name,
	).Scan(&r.ID, &r.Name, &r.Permissions, &isSystem, &r.CreatedAt, &r.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.IsSystem = isSystem != 0
	return r, nil
}

// ListRoles returns all roles ordered by name.
func (d *DB) ListRoles() ([]*Role, error) {
	rows, err := d.Query(`
		SELECT id, name, permissions, is_system, created_at, updated_at
		FROM roles ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var roles []*Role
	for rows.Next() {
		r := &Role{}
		var isSystem int
		if err := rows.Scan(&r.ID, &r.Name, &r.Permissions, &isSystem,
			&r.CreatedAt, &r.UpdatedAt); err != nil {
			return nil, err
		}
		r.IsSystem = isSystem != 0
		roles = append(roles, r)
	}
	return roles, rows.Err()
}

// UpdateRole updates an existing role. System roles cannot be modified.
func (d *DB) UpdateRole(r *Role) error {
	r.UpdatedAt = time.Now().UTC().Format(timeFormat)

	res, err := d.Exec(`
		UPDATE roles SET name = ?, permissions = ?, updated_at = ?
		WHERE id = ? AND is_system = 0`,
		r.Name, r.Permissions, r.UpdatedAt, r.ID,
	)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		// Check if it exists but is a system role.
		var count int
		_ = d.QueryRow("SELECT COUNT(*) FROM roles WHERE id = ?", r.ID).Scan(&count)
		if count > 0 {
			return errors.New("cannot modify system role")
		}
		return ErrNotFound
	}
	return nil
}

// DeleteRole deletes a role by ID. System roles cannot be deleted.
func (d *DB) DeleteRole(id string) error {
	res, err := d.Exec("DELETE FROM roles WHERE id = ? AND is_system = 0", id)
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		var count int
		_ = d.QueryRow("SELECT COUNT(*) FROM roles WHERE id = ?", id).Scan(&count)
		if count > 0 {
			return errors.New("cannot delete system role")
		}
		return ErrNotFound
	}
	return nil
}

// boolToInt is defined in updates.go
