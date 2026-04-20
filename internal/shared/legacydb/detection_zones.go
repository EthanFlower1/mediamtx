package legacydb

import (
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
)

// DetectionZonePoint is a single polygon vertex with normalized coordinates.
type DetectionZonePoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

// DetectionZone is the database representation of a detection zone polygon.
type DetectionZone struct {
	ID          string               `json:"id"`
	CameraID    string               `json:"camera_id"`
	Name        string               `json:"name"`
	Points      []DetectionZonePoint `json:"points"`
	ClassFilter []string             `json:"class_filter"`
	Enabled     bool                 `json:"enabled"`
	CreatedAt   string               `json:"created_at"`
	UpdatedAt   string               `json:"updated_at"`
}

// CreateDetectionZone inserts a new detection zone.
func (d *DB) CreateDetectionZone(z *DetectionZone) error {
	if z.ID == "" {
		z.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	z.CreatedAt = now
	z.UpdatedAt = now

	pointsJSON, err := json.Marshal(z.Points)
	if err != nil {
		return err
	}
	filterJSON, err := json.Marshal(z.ClassFilter)
	if err != nil {
		return err
	}

	_, err = d.Exec(`
		INSERT INTO detection_zones (id, camera_id, name, points, class_filter, enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		z.ID, z.CameraID, z.Name, string(pointsJSON), string(filterJSON),
		z.Enabled, z.CreatedAt, z.UpdatedAt,
	)
	return err
}

// UpdateDetectionZone updates an existing detection zone by ID.
func (d *DB) UpdateDetectionZone(z *DetectionZone) error {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	z.UpdatedAt = now

	pointsJSON, err := json.Marshal(z.Points)
	if err != nil {
		return err
	}
	filterJSON, err := json.Marshal(z.ClassFilter)
	if err != nil {
		return err
	}

	res, err := d.Exec(`
		UPDATE detection_zones
		SET name = ?, points = ?, class_filter = ?, enabled = ?, updated_at = ?
		WHERE id = ?`,
		z.Name, string(pointsJSON), string(filterJSON), z.Enabled, z.UpdatedAt, z.ID,
	)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteDetectionZone removes a detection zone by ID.
func (d *DB) DeleteDetectionZone(id string) error {
	res, err := d.Exec(`DELETE FROM detection_zones WHERE id = ?`, id)
	if err != nil {
		return err
	}
	rows, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// GetDetectionZone returns a single detection zone by ID.
func (d *DB) GetDetectionZone(id string) (*DetectionZone, error) {
	row := d.QueryRow(`
		SELECT id, camera_id, name, points, class_filter, enabled, created_at, updated_at
		FROM detection_zones WHERE id = ?`, id)
	return scanDetectionZone(row)
}

// ListDetectionZones returns all detection zones for a camera.
func (d *DB) ListDetectionZones(cameraID string) ([]*DetectionZone, error) {
	rows, err := d.Query(`
		SELECT id, camera_id, name, points, class_filter, enabled, created_at, updated_at
		FROM detection_zones WHERE camera_id = ? ORDER BY name`, cameraID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var zones []*DetectionZone
	for rows.Next() {
		var z DetectionZone
		var pointsStr, filterStr string
		if err := rows.Scan(&z.ID, &z.CameraID, &z.Name, &pointsStr, &filterStr,
			&z.Enabled, &z.CreatedAt, &z.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(pointsStr), &z.Points); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(filterStr), &z.ClassFilter); err != nil {
			z.ClassFilter = nil // tolerate empty/null
		}
		zones = append(zones, &z)
	}
	return zones, rows.Err()
}

// ListEnabledDetectionZones returns only enabled zones for a camera,
// used by the AI pipeline at detection time.
func (d *DB) ListEnabledDetectionZones(cameraID string) ([]*DetectionZone, error) {
	rows, err := d.Query(`
		SELECT id, camera_id, name, points, class_filter, enabled, created_at, updated_at
		FROM detection_zones WHERE camera_id = ? AND enabled = 1 ORDER BY name`, cameraID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var zones []*DetectionZone
	for rows.Next() {
		var z DetectionZone
		var pointsStr, filterStr string
		if err := rows.Scan(&z.ID, &z.CameraID, &z.Name, &pointsStr, &filterStr,
			&z.Enabled, &z.CreatedAt, &z.UpdatedAt); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(pointsStr), &z.Points); err != nil {
			return nil, err
		}
		if err := json.Unmarshal([]byte(filterStr), &z.ClassFilter); err != nil {
			z.ClassFilter = nil
		}
		zones = append(zones, &z)
	}
	return zones, rows.Err()
}

type zoneScanner interface {
	Scan(dest ...interface{}) error
}

func scanDetectionZone(row zoneScanner) (*DetectionZone, error) {
	var z DetectionZone
	var pointsStr, filterStr string
	err := row.Scan(&z.ID, &z.CameraID, &z.Name, &pointsStr, &filterStr,
		&z.Enabled, &z.CreatedAt, &z.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(pointsStr), &z.Points); err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(filterStr), &z.ClassFilter); err != nil {
		z.ClassFilter = nil
	}
	return &z, nil
}
