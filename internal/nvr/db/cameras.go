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
	ID                       string `json:"id"`
	Name                     string `json:"name"`
	ONVIFEndpoint            string `json:"onvif_endpoint"`
	ONVIFUsername             string `json:"onvif_username"`
	ONVIFPassword            string `json:"-"`
	ONVIFProfileToken        string `json:"onvif_profile_token"`
	RTSPURL                  string `json:"rtsp_url"`
	PTZCapable               bool   `json:"ptz_capable"`
	MediaMTXPath             string `json:"mediamtx_path"`
	Status                   string `json:"status"`
	Tags                     string `json:"tags"`
	RetentionDays            int    `json:"retention_days"`
	SupportsPTZ              bool   `json:"supports_ptz"`
	SupportsImaging          bool   `json:"supports_imaging"`
	SupportsEvents           bool   `json:"supports_events"`
	SupportsRelay            bool   `json:"supports_relay"`
	SupportsAudioBackchannel bool   `json:"supports_audio_backchannel"`
	SnapshotURI              string `json:"snapshot_uri,omitempty"`
	SupportsMedia2           bool   `json:"supports_media2"`
	SupportsAnalytics        bool   `json:"supports_analytics"`
	SupportsEdgeRecording    bool   `json:"supports_edge_recording"`
	MotionTimeoutSeconds     int    `json:"motion_timeout_seconds"`
	SubStreamURL             string `json:"sub_stream_url,omitempty"`
	AIEnabled                bool   `json:"ai_enabled"`
	CreatedAt                string `json:"created_at"`
	UpdatedAt                string `json:"updated_at"`
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
			retention_days, supports_ptz, supports_imaging, supports_events,
			supports_relay, supports_audio_backchannel, snapshot_uri,
			supports_media2, supports_analytics, supports_edge_recording,
			motion_timeout_seconds, sub_stream_url, ai_enabled, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cam.ID, cam.Name, cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword,
		cam.ONVIFProfileToken, cam.RTSPURL, cam.PTZCapable, cam.MediaMTXPath,
		cam.Status, cam.Tags, cam.RetentionDays,
		cam.SupportsPTZ, cam.SupportsImaging, cam.SupportsEvents,
		cam.SupportsRelay, cam.SupportsAudioBackchannel, cam.SnapshotURI,
		cam.SupportsMedia2, cam.SupportsAnalytics, cam.SupportsEdgeRecording,
		cam.MotionTimeoutSeconds, cam.SubStreamURL, cam.AIEnabled,
		cam.CreatedAt, cam.UpdatedAt,
	)
	return err
}

// GetCamera retrieves a camera by its ID. Returns ErrNotFound if no match.
func (d *DB) GetCamera(id string) (*Camera, error) {
	cam := &Camera{}
	err := d.QueryRow(`
		SELECT id, name, onvif_endpoint, onvif_username, onvif_password,
			onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
			retention_days, supports_ptz, supports_imaging, supports_events,
			supports_relay, supports_audio_backchannel, snapshot_uri,
			supports_media2, supports_analytics, supports_edge_recording,
			motion_timeout_seconds, sub_stream_url, ai_enabled, created_at, updated_at
		FROM cameras WHERE id = ?`, id,
	).Scan(
		&cam.ID, &cam.Name, &cam.ONVIFEndpoint, &cam.ONVIFUsername, &cam.ONVIFPassword,
		&cam.ONVIFProfileToken, &cam.RTSPURL, &cam.PTZCapable, &cam.MediaMTXPath,
		&cam.Status, &cam.Tags, &cam.RetentionDays,
		&cam.SupportsPTZ, &cam.SupportsImaging, &cam.SupportsEvents,
		&cam.SupportsRelay, &cam.SupportsAudioBackchannel, &cam.SnapshotURI,
		&cam.SupportsMedia2, &cam.SupportsAnalytics, &cam.SupportsEdgeRecording,
		&cam.MotionTimeoutSeconds, &cam.SubStreamURL, &cam.AIEnabled,
		&cam.CreatedAt, &cam.UpdatedAt,
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
			retention_days, supports_ptz, supports_imaging, supports_events,
			supports_relay, supports_audio_backchannel, snapshot_uri,
			supports_media2, supports_analytics, supports_edge_recording,
			motion_timeout_seconds, sub_stream_url, ai_enabled, created_at, updated_at
		FROM cameras WHERE mediamtx_path = ?`, path,
	).Scan(
		&cam.ID, &cam.Name, &cam.ONVIFEndpoint, &cam.ONVIFUsername, &cam.ONVIFPassword,
		&cam.ONVIFProfileToken, &cam.RTSPURL, &cam.PTZCapable, &cam.MediaMTXPath,
		&cam.Status, &cam.Tags, &cam.RetentionDays,
		&cam.SupportsPTZ, &cam.SupportsImaging, &cam.SupportsEvents,
		&cam.SupportsRelay, &cam.SupportsAudioBackchannel, &cam.SnapshotURI,
		&cam.SupportsMedia2, &cam.SupportsAnalytics, &cam.SupportsEdgeRecording,
		&cam.MotionTimeoutSeconds, &cam.SubStreamURL, &cam.AIEnabled,
		&cam.CreatedAt, &cam.UpdatedAt,
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
			retention_days, supports_ptz, supports_imaging, supports_events,
			supports_relay, supports_audio_backchannel, snapshot_uri,
			supports_media2, supports_analytics, supports_edge_recording,
			motion_timeout_seconds, sub_stream_url, ai_enabled, created_at, updated_at
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
			&cam.Status, &cam.Tags, &cam.RetentionDays,
			&cam.SupportsPTZ, &cam.SupportsImaging, &cam.SupportsEvents,
			&cam.SupportsRelay, &cam.SupportsAudioBackchannel, &cam.SnapshotURI,
			&cam.SupportsMedia2, &cam.SupportsAnalytics, &cam.SupportsEdgeRecording,
			&cam.MotionTimeoutSeconds, &cam.SubStreamURL, &cam.AIEnabled,
			&cam.CreatedAt, &cam.UpdatedAt,
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
			mediamtx_path = ?, status = ?, tags = ?, retention_days = ?,
			supports_ptz = ?, supports_imaging = ?, supports_events = ?,
			supports_relay = ?, supports_audio_backchannel = ?, snapshot_uri = ?,
			supports_media2 = ?, supports_analytics = ?, supports_edge_recording = ?,
			motion_timeout_seconds = ?, sub_stream_url = ?, ai_enabled = ?,
			updated_at = ?
		WHERE id = ?`,
		cam.Name, cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword,
		cam.ONVIFProfileToken, cam.RTSPURL, cam.PTZCapable, cam.MediaMTXPath,
		cam.Status, cam.Tags, cam.RetentionDays,
		cam.SupportsPTZ, cam.SupportsImaging, cam.SupportsEvents,
		cam.SupportsRelay, cam.SupportsAudioBackchannel, cam.SnapshotURI,
		cam.SupportsMedia2, cam.SupportsAnalytics, cam.SupportsEdgeRecording,
		cam.MotionTimeoutSeconds, cam.SubStreamURL, cam.AIEnabled,
		cam.UpdatedAt, cam.ID,
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

// UpdateCameraRetention updates only the retention_days field of a camera.
// Returns ErrNotFound if no match.
func (d *DB) UpdateCameraRetention(id string, retentionDays int) error {
	updatedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	res, err := d.Exec(`
		UPDATE cameras SET retention_days = ?, updated_at = ?
		WHERE id = ?`,
		retentionDays, updatedAt, id,
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

// UpdateCameraMotionTimeout updates only the motion_timeout_seconds field of a camera.
// Returns ErrNotFound if no match.
func (d *DB) UpdateCameraMotionTimeout(id string, seconds int) error {
	updatedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	res, err := d.Exec(`
		UPDATE cameras SET motion_timeout_seconds = ?, updated_at = ?
		WHERE id = ?`,
		seconds, updatedAt, id,
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

// UpdateCameraAIConfig updates only the ai_enabled and sub_stream_url fields
// of a camera. Returns ErrNotFound if no match.
func (d *DB) UpdateCameraAIConfig(id string, aiEnabled bool, subStreamURL string) error {
	updatedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	res, err := d.Exec(`
		UPDATE cameras SET ai_enabled = ?, sub_stream_url = ?, updated_at = ?
		WHERE id = ?`,
		aiEnabled, subStreamURL, updatedAt, id,
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
