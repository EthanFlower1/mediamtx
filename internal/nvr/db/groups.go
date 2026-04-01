package db

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// CameraGroup represents a named collection of cameras.
type CameraGroup struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	CameraIDs []string `json:"camera_ids"`
	CreatedAt string   `json:"created_at"`
	UpdatedAt string   `json:"updated_at"`
}

// CreateGroup inserts a new camera group and its member camera IDs into the
// database inside a single transaction.
func (d *DB) CreateGroup(name string, cameraIDs []string) (*CameraGroup, error) {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := d.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback()

	_, err = tx.Exec(
		"INSERT INTO camera_groups (id, name, created_at, updated_at) VALUES (?, ?, ?, ?)",
		id, name, now, now,
	)
	if err != nil {
		return nil, fmt.Errorf("insert group: %w", err)
	}

	for i, camID := range cameraIDs {
		_, err = tx.Exec(
			"INSERT INTO camera_group_members (group_id, camera_id, sort_order) VALUES (?, ?, ?)",
			id, camID, i,
		)
		if err != nil {
			return nil, fmt.Errorf("insert member %s: %w", camID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit: %w", err)
	}

	return &CameraGroup{
		ID: id, Name: name, CameraIDs: cameraIDs,
		CreatedAt: now, UpdatedAt: now,
	}, nil
}

// ListGroups returns all camera groups ordered by name, each populated with
// their member camera IDs.
func (d *DB) ListGroups() ([]CameraGroup, error) {
	rows, err := d.Query("SELECT id, name, created_at, updated_at FROM camera_groups ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var groups []CameraGroup
	for rows.Next() {
		var g CameraGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedAt, &g.UpdatedAt); err != nil {
			return nil, err
		}

		memberRows, err := d.Query(
			"SELECT camera_id FROM camera_group_members WHERE group_id = ? ORDER BY sort_order",
			g.ID,
		)
		if err != nil {
			return nil, err
		}
		for memberRows.Next() {
			var camID string
			if err := memberRows.Scan(&camID); err != nil {
				memberRows.Close()
				return nil, err
			}
			g.CameraIDs = append(g.CameraIDs, camID)
		}
		memberRows.Close()
		if g.CameraIDs == nil {
			g.CameraIDs = []string{}
		}
		groups = append(groups, g)
	}
	return groups, rows.Err()
}

// GetGroup retrieves a single camera group by ID, including its member camera
// IDs. Returns ErrNotFound if no group with that ID exists.
func (d *DB) GetGroup(id string) (*CameraGroup, error) {
	var g CameraGroup
	err := d.QueryRow(
		"SELECT id, name, created_at, updated_at FROM camera_groups WHERE id = ?", id,
	).Scan(&g.ID, &g.Name, &g.CreatedAt, &g.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	rows, err := d.Query(
		"SELECT camera_id FROM camera_group_members WHERE group_id = ? ORDER BY sort_order", id,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var camID string
		if err := rows.Scan(&camID); err != nil {
			return nil, err
		}
		g.CameraIDs = append(g.CameraIDs, camID)
	}
	if g.CameraIDs == nil {
		g.CameraIDs = []string{}
	}
	return &g, rows.Err()
}

// UpdateGroup replaces the name and full member list of an existing camera
// group inside a single transaction.
func (d *DB) UpdateGroup(id, name string, cameraIDs []string) error {
	now := time.Now().UTC().Format(time.RFC3339)

	tx, err := d.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	_, err = tx.Exec("UPDATE camera_groups SET name = ?, updated_at = ? WHERE id = ?", name, now, id)
	if err != nil {
		return err
	}

	// Replace all members.
	_, err = tx.Exec("DELETE FROM camera_group_members WHERE group_id = ?", id)
	if err != nil {
		return err
	}
	for i, camID := range cameraIDs {
		_, err = tx.Exec(
			"INSERT INTO camera_group_members (group_id, camera_id, sort_order) VALUES (?, ?, ?)",
			id, camID, i,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// DeleteGroup deletes a camera group by ID. CASCADE on the join table removes
// all member rows automatically.
func (d *DB) DeleteGroup(id string) error {
	_, err := d.Exec("DELETE FROM camera_groups WHERE id = ?", id)
	return err
}
