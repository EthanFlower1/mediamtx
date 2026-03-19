package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// ErrNotFound is returned when a requested record does not exist.
var ErrNotFound = errors.New("not found")

// Camera represents a camera record in the database.
type Camera struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	ONVIFEndpoint     string `json:"onvif_endpoint"`
	ONVIFUsername      string `json:"onvif_username"`
	ONVIFPassword     string `json:"onvif_password"`
	ONVIFProfileToken string `json:"onvif_profile_token"`
	RTSPURL           string `json:"rtsp_url"`
	PTZCapable        bool   `json:"ptz_capable"`
	MediaMTXPath      string `json:"mediamtx_path"`
	Status            string `json:"status"`
	Tags              string `json:"tags"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
}

// CreateCamera inserts a new camera into the database.
// If cam.ID is empty, a new UUID is generated.
// If cam.Status is empty, it defaults to "disconnected".
func (d *DB) CreateCamera(cam *Camera) error {
	if cam.ID == "" {
		cam.ID = uuid.New().String()
	}
	if cam.Status == "" {
		cam.Status = "disconnected"
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	cam.CreatedAt = now
	cam.UpdatedAt = now

	_, err := d.Exec(`
		INSERT INTO cameras (id, name, onvif_endpoint, onvif_username, onvif_password,
			onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cam.ID, cam.Name, cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword,
		cam.ONVIFProfileToken, cam.RTSPURL, cam.PTZCapable, cam.MediaMTXPath,
		cam.Status, cam.Tags, cam.CreatedAt, cam.UpdatedAt,
	)
	return err
}

// GetCamera retrieves a camera by its ID. Returns ErrNotFound if no match.
func (d *DB) GetCamera(id string) (*Camera, error) {
	cam := &Camera{}
	err := d.QueryRow(`
		SELECT id, name, onvif_endpoint, onvif_username, onvif_password,
			onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
			created_at, updated_at
		FROM cameras WHERE id = ?`, id,
	).Scan(
		&cam.ID, &cam.Name, &cam.ONVIFEndpoint, &cam.ONVIFUsername, &cam.ONVIFPassword,
		&cam.ONVIFProfileToken, &cam.RTSPURL, &cam.PTZCapable, &cam.MediaMTXPath,
		&cam.Status, &cam.Tags, &cam.CreatedAt, &cam.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return cam, nil
}

// GetCameraByPath retrieves a camera by its MediaMTX path. Returns ErrNotFound if no match.
func (d *DB) GetCameraByPath(path string) (*Camera, error) {
	cam := &Camera{}
	err := d.QueryRow(`
		SELECT id, name, onvif_endpoint, onvif_username, onvif_password,
			onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
			created_at, updated_at
		FROM cameras WHERE mediamtx_path = ?`, path,
	).Scan(
		&cam.ID, &cam.Name, &cam.ONVIFEndpoint, &cam.ONVIFUsername, &cam.ONVIFPassword,
		&cam.ONVIFProfileToken, &cam.RTSPURL, &cam.PTZCapable, &cam.MediaMTXPath,
		&cam.Status, &cam.Tags, &cam.CreatedAt, &cam.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return cam, nil
}

// ListCameras returns all cameras ordered by name.
func (d *DB) ListCameras() ([]*Camera, error) {
	rows, err := d.Query(`
		SELECT id, name, onvif_endpoint, onvif_username, onvif_password,
			onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
			created_at, updated_at
		FROM cameras ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cameras []*Camera
	for rows.Next() {
		cam := &Camera{}
		if err := rows.Scan(
			&cam.ID, &cam.Name, &cam.ONVIFEndpoint, &cam.ONVIFUsername, &cam.ONVIFPassword,
			&cam.ONVIFProfileToken, &cam.RTSPURL, &cam.PTZCapable, &cam.MediaMTXPath,
			&cam.Status, &cam.Tags, &cam.CreatedAt, &cam.UpdatedAt,
		); err != nil {
			return nil, err
		}
		cameras = append(cameras, cam)
	}
	return cameras, rows.Err()
}

// UpdateCamera updates an existing camera. Returns ErrNotFound if no match.
func (d *DB) UpdateCamera(cam *Camera) error {
	cam.UpdatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	res, err := d.Exec(`
		UPDATE cameras SET name = ?, onvif_endpoint = ?, onvif_username = ?,
			onvif_password = ?, onvif_profile_token = ?, rtsp_url = ?, ptz_capable = ?,
			mediamtx_path = ?, status = ?, tags = ?, updated_at = ?
		WHERE id = ?`,
		cam.Name, cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword,
		cam.ONVIFProfileToken, cam.RTSPURL, cam.PTZCapable, cam.MediaMTXPath,
		cam.Status, cam.Tags, cam.UpdatedAt, cam.ID,
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

// DeleteCamera deletes a camera by its ID. Returns ErrNotFound if no match.
func (d *DB) DeleteCamera(id string) error {
	res, err := d.Exec("DELETE FROM cameras WHERE id = ?", id)
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
