package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// EvidenceExport represents a chain-of-custody evidence export record.
type EvidenceExport struct {
	ID         string `json:"id"`
	CameraID   string `json:"camera_id"`
	CameraName string `json:"camera_name"`
	StartTime  string `json:"start_time"`
	EndTime    string `json:"end_time"`
	ExportedBy string `json:"exported_by"`
	ExportedAt string `json:"exported_at"`
	SHA256Hash string `json:"sha256_hash"`
	ZipPath    string `json:"zip_path"`
	Notes      string `json:"notes"`
}

// CreateEvidenceExport inserts a new evidence export record.
func (d *DB) CreateEvidenceExport(e *EvidenceExport) error {
	if e.ID == "" {
		e.ID = uuid.New().String()
	}
	if e.ExportedAt == "" {
		e.ExportedAt = time.Now().UTC().Format(timeFormat)
	}

	_, err := d.Exec(`
		INSERT INTO evidence_exports (id, camera_id, camera_name, start_time, end_time,
			exported_by, exported_at, sha256_hash, zip_path, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		e.ID, e.CameraID, e.CameraName, e.StartTime, e.EndTime,
		e.ExportedBy, e.ExportedAt, e.SHA256Hash, e.ZipPath, e.Notes,
	)
	return err
}

// GetEvidenceExport retrieves an evidence export by ID.
func (d *DB) GetEvidenceExport(id string) (*EvidenceExport, error) {
	e := &EvidenceExport{}
	err := d.QueryRow(`
		SELECT id, camera_id, camera_name, start_time, end_time,
			exported_by, exported_at, sha256_hash, zip_path, notes
		FROM evidence_exports WHERE id = ?`, id,
	).Scan(
		&e.ID, &e.CameraID, &e.CameraName, &e.StartTime, &e.EndTime,
		&e.ExportedBy, &e.ExportedAt, &e.SHA256Hash, &e.ZipPath, &e.Notes,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return e, nil
}

// ListEvidenceExports returns evidence exports, optionally filtered by camera ID.
func (d *DB) ListEvidenceExports(cameraID string) ([]*EvidenceExport, error) {
	var rows *sql.Rows
	var err error

	if cameraID != "" {
		rows, err = d.Query(`
			SELECT id, camera_id, camera_name, start_time, end_time,
				exported_by, exported_at, sha256_hash, zip_path, notes
			FROM evidence_exports WHERE camera_id = ?
			ORDER BY exported_at DESC`, cameraID)
	} else {
		rows, err = d.Query(`
			SELECT id, camera_id, camera_name, start_time, end_time,
				exported_by, exported_at, sha256_hash, zip_path, notes
			FROM evidence_exports ORDER BY exported_at DESC`)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var exports []*EvidenceExport
	for rows.Next() {
		e := &EvidenceExport{}
		if err := rows.Scan(
			&e.ID, &e.CameraID, &e.CameraName, &e.StartTime, &e.EndTime,
			&e.ExportedBy, &e.ExportedAt, &e.SHA256Hash, &e.ZipPath, &e.Notes,
		); err != nil {
			return nil, err
		}
		exports = append(exports, e)
	}
	return exports, rows.Err()
}
