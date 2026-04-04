package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// User represents a user record in the database.
type User struct {
	ID                string `json:"id"`
	Username          string `json:"username"`
	PasswordHash      string `json:"-"`
	Role              string `json:"role"`
	RoleID            string `json:"role_id"`
	CameraPermissions string `json:"camera_permissions"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

// CreateUser inserts a new user into the database.
// If u.ID is empty, a new UUID is generated.
// If u.Role is empty, it defaults to "viewer".
func (d *DB) CreateUser(u *User) error {
	if u.ID == "" {
		u.ID = uuid.New().String()
	}
	if u.Role == "" {
		u.Role = "viewer"
	}

	now := time.Now().UTC().Format(timeFormat)
	u.CreatedAt = now
	u.UpdatedAt = now

	_, err := d.Exec(`
		INSERT INTO users (id, username, password_hash, role, role_id, camera_permissions, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		u.ID, u.Username, u.PasswordHash, u.Role, u.RoleID, u.CameraPermissions,
		u.CreatedAt, u.UpdatedAt,
	)
	return err
}

// GetUser retrieves a user by ID. Returns ErrNotFound if no match.
func (d *DB) GetUser(id string) (*User, error) {
	u := &User{}
	err := d.QueryRow(`
		SELECT id, username, password_hash, role, COALESCE(role_id, ''), camera_permissions, created_at, updated_at
		FROM users WHERE id = ?`, id,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.RoleID, &u.CameraPermissions,
		&u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// GetUserByUsername retrieves a user by username. Returns ErrNotFound if no match.
func (d *DB) GetUserByUsername(username string) (*User, error) {
	u := &User{}
	err := d.QueryRow(`
		SELECT id, username, password_hash, role, COALESCE(role_id, ''), camera_permissions, created_at, updated_at
		FROM users WHERE username = ?`, username,
	).Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.RoleID, &u.CameraPermissions,
		&u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return u, nil
}

// ListUsers returns all users ordered by username.
func (d *DB) ListUsers() ([]*User, error) {
	rows, err := d.Query(`
		SELECT id, username, password_hash, role, COALESCE(role_id, ''), camera_permissions, created_at, updated_at
		FROM users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []*User
	for rows.Next() {
		u := &User{}
		if err := rows.Scan(&u.ID, &u.Username, &u.PasswordHash, &u.Role, &u.RoleID,
			&u.CameraPermissions, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// UpdateUser updates an existing user. Returns ErrNotFound if no match.
func (d *DB) UpdateUser(u *User) error {
	u.UpdatedAt = time.Now().UTC().Format(timeFormat)

	res, err := d.Exec(`
		UPDATE users SET username = ?, password_hash = ?, role = ?, role_id = ?,
			camera_permissions = ?, updated_at = ?
		WHERE id = ?`,
		u.Username, u.PasswordHash, u.Role, u.RoleID, u.CameraPermissions, u.UpdatedAt, u.ID,
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

// DeleteUser deletes a user by ID. Returns ErrNotFound if no match.
func (d *DB) DeleteUser(id string) error {
	res, err := d.Exec("DELETE FROM users WHERE id = ?", id)
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

// CountUsers returns the total number of users in the database.
func (d *DB) CountUsers() (int, error) {
	var count int
	err := d.QueryRow("SELECT COUNT(*) FROM users").Scan(&count)
	return count, err
}
