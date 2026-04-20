package legacydb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Tour represents a camera tour — an ordered list of cameras that cycle
// automatically with a configurable dwell time.
type Tour struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	CameraIDs    []string `json:"camera_ids"`
	DwellSeconds int      `json:"dwell_seconds"`
	CreatedAt    string   `json:"created_at"`
	UpdatedAt    string   `json:"updated_at"`
}

// CreateTour inserts a new tour into the database.
func (d *DB) CreateTour(name string, cameraIDs []string, dwellSeconds int) (*Tour, error) {
	id := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	cameraJSON, err := json.Marshal(cameraIDs)
	if err != nil {
		return nil, fmt.Errorf("marshal camera_ids: %w", err)
	}

	_, err = d.Exec(
		"INSERT INTO tours (id, name, camera_ids, dwell_seconds, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)",
		id, name, string(cameraJSON), dwellSeconds, now, now,
	)
	if err != nil {
		return nil, err
	}

	return &Tour{
		ID: id, Name: name, CameraIDs: cameraIDs,
		DwellSeconds: dwellSeconds, CreatedAt: now, UpdatedAt: now,
	}, nil
}

// ListTours returns all tours ordered by name.
func (d *DB) ListTours() ([]Tour, error) {
	rows, err := d.Query("SELECT id, name, camera_ids, dwell_seconds, created_at, updated_at FROM tours ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tours []Tour
	for rows.Next() {
		var t Tour
		var cameraJSON string
		if err := rows.Scan(&t.ID, &t.Name, &cameraJSON, &t.DwellSeconds, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(cameraJSON), &t.CameraIDs); err != nil {
			t.CameraIDs = []string{}
		}
		tours = append(tours, t)
	}
	return tours, rows.Err()
}

// GetTour retrieves a single tour by ID. Returns ErrNotFound if no tour with
// that ID exists.
func (d *DB) GetTour(id string) (*Tour, error) {
	var t Tour
	var cameraJSON string
	err := d.QueryRow(
		"SELECT id, name, camera_ids, dwell_seconds, created_at, updated_at FROM tours WHERE id = ?", id,
	).Scan(&t.ID, &t.Name, &cameraJSON, &t.DwellSeconds, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(cameraJSON), &t.CameraIDs); err != nil {
		t.CameraIDs = []string{}
	}
	return &t, nil
}

// UpdateTour replaces the name, camera list, and dwell time of an existing tour.
func (d *DB) UpdateTour(id, name string, cameraIDs []string, dwellSeconds int) error {
	now := time.Now().UTC().Format(time.RFC3339)
	cameraJSON, err := json.Marshal(cameraIDs)
	if err != nil {
		return err
	}
	_, err = d.Exec(
		"UPDATE tours SET name = ?, camera_ids = ?, dwell_seconds = ?, updated_at = ? WHERE id = ?",
		name, string(cameraJSON), dwellSeconds, now, id,
	)
	return err
}

// DeleteTour deletes a tour by ID.
func (d *DB) DeleteTour(id string) error {
	_, err := d.Exec("DELETE FROM tours WHERE id = ?", id)
	return err
}
