// Package recordercontrol implements the on-prem Directory's server-side of the
// RecorderControl.StreamAssignments RPC defined in
// internal/shared/proto/kaivue/v1/recorder_control.proto.
//
// Architecture:
//
//	Admin UI / API → Store (SQLite) → EventBus → Handler → NDJSON stream → Recorder
//
// The package mirrors the cloud-plane implementation in internal/cloud/recordercontrol
// but is backed by the on-prem Directory SQLite database rather than Postgres,
// and operates in a single-tenant context (the on-prem site).
//
// Boundary rules (depguard):
//   - MUST NOT import internal/recorder
//   - MAY import internal/shared for proto types and shared primitives
package recordercontrol

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// CameraRow is the SQLite-backed representation of a camera assignment.
type CameraRow struct {
	CameraID      string
	RecorderID    string
	Name          string
	CredentialRef string
	ConfigJSON    string
	ConfigVersion int64
	CreatedAt     time.Time
	UpdatedAt     time.Time
}

// Store persists camera assignments into the Directory's SQLite database and
// provides the query seams consumed by Handler.
type Store struct {
	db *sql.DB
}

// NewStore creates a Store backed by the given database connection.
// The caller is responsible for running migrations (directory/db.Open handles this).
func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

// ListCamerasForRecorder returns all cameras assigned to recorderID.
// Returns an empty (non-nil) slice when no cameras are assigned.
func (s *Store) ListCamerasForRecorder(ctx context.Context, recorderID string) ([]CameraRow, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT camera_id, recorder_id, name, credential_ref, config_json, config_version, created_at, updated_at
		FROM assigned_cameras
		WHERE recorder_id = ?
		ORDER BY camera_id
	`, recorderID)
	if err != nil {
		return nil, fmt.Errorf("recordercontrol/store: list cameras: %w", err)
	}
	defer rows.Close()

	var out []CameraRow
	for rows.Next() {
		var r CameraRow
		var createdAt, updatedAt string
		if err := rows.Scan(
			&r.CameraID, &r.RecorderID, &r.Name, &r.CredentialRef,
			&r.ConfigJSON, &r.ConfigVersion, &createdAt, &updatedAt,
		); err != nil {
			return nil, fmt.Errorf("recordercontrol/store: scan camera: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		out = append(out, r)
	}
	if out == nil {
		out = []CameraRow{}
	}
	return out, rows.Err()
}

// GetCamera returns a single camera by ID.
func (s *Store) GetCamera(ctx context.Context, cameraID string) (CameraRow, error) {
	var r CameraRow
	var createdAt, updatedAt string
	err := s.db.QueryRowContext(ctx, `
		SELECT camera_id, recorder_id, name, credential_ref, config_json, config_version, created_at, updated_at
		FROM assigned_cameras
		WHERE camera_id = ?
	`, cameraID).Scan(
		&r.CameraID, &r.RecorderID, &r.Name, &r.CredentialRef,
		&r.ConfigJSON, &r.ConfigVersion, &createdAt, &updatedAt,
	)
	if err != nil {
		return CameraRow{}, fmt.Errorf("recordercontrol/store: get camera %s: %w", cameraID, err)
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
	return r, nil
}

// InsertCamera adds a new camera assignment. Returns an error if the camera_id
// already exists.
func (s *Store) InsertCamera(ctx context.Context, row CameraRow) error {
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO assigned_cameras (camera_id, recorder_id, name, credential_ref, config_json, config_version)
		VALUES (?, ?, ?, ?, ?, ?)
	`, row.CameraID, row.RecorderID, row.Name, row.CredentialRef, row.ConfigJSON, row.ConfigVersion)
	if err != nil {
		return fmt.Errorf("recordercontrol/store: insert camera %s: %w", row.CameraID, err)
	}
	return nil
}

// UpdateCamera updates an existing camera's config. It bumps config_version
// and updated_at automatically.
func (s *Store) UpdateCamera(ctx context.Context, row CameraRow) error {
	res, err := s.db.ExecContext(ctx, `
		UPDATE assigned_cameras
		SET recorder_id = ?, name = ?, credential_ref = ?, config_json = ?,
		    config_version = config_version + 1, updated_at = CURRENT_TIMESTAMP
		WHERE camera_id = ?
	`, row.RecorderID, row.Name, row.CredentialRef, row.ConfigJSON, row.CameraID)
	if err != nil {
		return fmt.Errorf("recordercontrol/store: update camera %s: %w", row.CameraID, err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return fmt.Errorf("recordercontrol/store: camera %s not found", row.CameraID)
	}
	return nil
}

// DeleteCamera removes a camera assignment. Returns the number of rows affected.
func (s *Store) DeleteCamera(ctx context.Context, cameraID string) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM assigned_cameras WHERE camera_id = ?`, cameraID)
	if err != nil {
		return 0, fmt.Errorf("recordercontrol/store: delete camera %s: %w", cameraID, err)
	}
	return res.RowsAffected()
}

// RecorderExists checks if a recorder is enrolled (exists in the recorders table).
func (s *Store) RecorderExists(ctx context.Context, recorderID string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM recorders WHERE recorder_id = ?`, recorderID).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("recordercontrol/store: check recorder %s: %w", recorderID, err)
	}
	return count > 0, nil
}
