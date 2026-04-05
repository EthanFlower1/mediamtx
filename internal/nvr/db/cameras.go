package db

import (
	"database/sql"
	"errors"
	"fmt"
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
	EventRetentionDays       int    `json:"event_retention_days"`
	DetectionRetentionDays   int    `json:"detection_retention_days"`
	SupportsPTZ              bool   `json:"supports_ptz"`
	SupportsImaging          bool   `json:"supports_imaging"`
	SupportsEvents           bool   `json:"supports_events"`
	SupportsRelay            bool   `json:"supports_relay"`
	SupportsAudioBackchannel bool   `json:"supports_audio_backchannel"`
	SnapshotURI              string `json:"snapshot_uri,omitempty"`
	SupportsMedia2           bool   `json:"supports_media2"`
	SupportsAnalytics        bool   `json:"supports_analytics"`
	SupportsEdgeRecording    bool   `json:"supports_edge_recording"`
	ServiceCapabilities      string `json:"service_capabilities,omitempty"`
	MotionTimeoutSeconds     int    `json:"motion_timeout_seconds"`
	SubStreamURL             string  `json:"sub_stream_url,omitempty"`
	AIEnabled                bool    `json:"ai_enabled"`
	AIStreamID               string  `json:"ai_stream_id,omitempty"`
	AITrackTimeout           int     `json:"ai_track_timeout"`
	AIConfidence             float64 `json:"ai_confidence"`
	AudioTranscode           bool    `json:"audio_transcode"`
	RecordingStreamID        string  `json:"recording_stream_id,omitempty"`
	StoragePath              string `json:"storage_path"`
	QuotaBytes               int64  `json:"quota_bytes"`
	QuotaWarningPercent      int    `json:"quota_warning_percent"`
	QuotaCriticalPercent     int    `json:"quota_critical_percent"`
	SupportedEventTopics     string `json:"supported_event_topics"`
	DeviceID                 string `json:"device_id,omitempty"`
	ChannelIndex             *int   `json:"channel_index,omitempty"`
	MulticastEnabled         bool   `json:"multicast_enabled"`
	MulticastAddress         string `json:"multicast_address"`
	MulticastPort            int    `json:"multicast_port"`
	MulticastTTL             int    `json:"multicast_ttl"`
	ConfidenceThresholds     string `json:"confidence_thresholds,omitempty"`
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

	if cam.QuotaWarningPercent == 0 {
		cam.QuotaWarningPercent = 80
	}
	if cam.QuotaCriticalPercent == 0 {
		cam.QuotaCriticalPercent = 90
	}

	var deviceID interface{}
	if cam.DeviceID != "" {
		deviceID = cam.DeviceID
	}
	_, err := d.Exec(`
		INSERT INTO cameras (id, name, onvif_endpoint, onvif_username, onvif_password,
			onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
			retention_days, event_retention_days, detection_retention_days,
			supports_ptz, supports_imaging, supports_events,
			supports_relay, supports_audio_backchannel, snapshot_uri,
			supports_media2, supports_analytics, supports_edge_recording,
			service_capabilities,
			motion_timeout_seconds, sub_stream_url, ai_enabled, audio_transcode,
			storage_path, quota_bytes, quota_warning_percent, quota_critical_percent,
			supported_event_topics,
			device_id, channel_index,
			multicast_enabled, multicast_address, multicast_port, multicast_ttl,
			confidence_thresholds,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		cam.ID, cam.Name, cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword,
		cam.ONVIFProfileToken, cam.RTSPURL, cam.PTZCapable, cam.MediaMTXPath,
		cam.Status, cam.Tags, cam.RetentionDays, cam.EventRetentionDays, cam.DetectionRetentionDays,
		cam.SupportsPTZ, cam.SupportsImaging, cam.SupportsEvents,
		cam.SupportsRelay, cam.SupportsAudioBackchannel, cam.SnapshotURI,
		cam.SupportsMedia2, cam.SupportsAnalytics, cam.SupportsEdgeRecording,
		cam.ServiceCapabilities,
		cam.MotionTimeoutSeconds, cam.SubStreamURL, cam.AIEnabled, cam.AudioTranscode,
		cam.StoragePath, cam.QuotaBytes, cam.QuotaWarningPercent, cam.QuotaCriticalPercent,
		cam.SupportedEventTopics,
		deviceID, cam.ChannelIndex,
		cam.MulticastEnabled, cam.MulticastAddress, cam.MulticastPort, cam.MulticastTTL,
		cam.ConfidenceThresholds,
		cam.CreatedAt, cam.UpdatedAt,
	)
	return err
}

// scanCamera scans a camera row into a Camera struct, handling nullable device_id.
func scanCamera(scan func(...interface{}) error) (*Camera, error) {
	cam := &Camera{}
	var deviceID *string
	err := scan(
		&cam.ID, &cam.Name, &cam.ONVIFEndpoint, &cam.ONVIFUsername, &cam.ONVIFPassword,
		&cam.ONVIFProfileToken, &cam.RTSPURL, &cam.PTZCapable, &cam.MediaMTXPath,
		&cam.Status, &cam.Tags, &cam.RetentionDays, &cam.EventRetentionDays, &cam.DetectionRetentionDays,
		&cam.SupportsPTZ, &cam.SupportsImaging, &cam.SupportsEvents,
		&cam.SupportsRelay, &cam.SupportsAudioBackchannel, &cam.SnapshotURI,
		&cam.SupportsMedia2, &cam.SupportsAnalytics, &cam.SupportsEdgeRecording,
		&cam.ServiceCapabilities,
		&cam.MotionTimeoutSeconds, &cam.SubStreamURL, &cam.AIEnabled, &cam.AudioTranscode,
		&cam.StoragePath, &cam.CreatedAt, &cam.UpdatedAt,
		&cam.AIStreamID, &cam.AITrackTimeout, &cam.AIConfidence, &cam.RecordingStreamID,
		&cam.QuotaBytes, &cam.QuotaWarningPercent, &cam.QuotaCriticalPercent,
		&cam.SupportedEventTopics,
		&deviceID, &cam.ChannelIndex,
		&cam.MulticastEnabled, &cam.MulticastAddress, &cam.MulticastPort, &cam.MulticastTTL,
		&cam.ConfidenceThresholds,
	)
	if err != nil {
		return nil, err
	}
	if deviceID != nil {
		cam.DeviceID = *deviceID
	}
	return cam, nil
}

// GetCamera retrieves a camera by its ID. Returns ErrNotFound if no match.
func (d *DB) GetCamera(id string) (*Camera, error) {
	row := d.QueryRow(`
		SELECT id, name, onvif_endpoint, onvif_username, onvif_password,
			onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
			retention_days, event_retention_days, detection_retention_days,
			supports_ptz, supports_imaging, supports_events,
			supports_relay, supports_audio_backchannel, snapshot_uri,
			supports_media2, supports_analytics, supports_edge_recording,
			service_capabilities,
			motion_timeout_seconds, sub_stream_url, ai_enabled, audio_transcode,
			storage_path, created_at, updated_at,
			ai_stream_id, ai_track_timeout, ai_confidence, recording_stream_id,
			quota_bytes, quota_warning_percent, quota_critical_percent,
			supported_event_topics,
			device_id, channel_index,
			multicast_enabled, multicast_address, multicast_port, multicast_ttl,
			confidence_thresholds
		FROM cameras WHERE id = ?`, id)
	cam, err := scanCamera(row.Scan)
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
	row := d.QueryRow(`
		SELECT id, name, onvif_endpoint, onvif_username, onvif_password,
			onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
			retention_days, event_retention_days, detection_retention_days,
			supports_ptz, supports_imaging, supports_events,
			supports_relay, supports_audio_backchannel, snapshot_uri,
			supports_media2, supports_analytics, supports_edge_recording,
			service_capabilities,
			motion_timeout_seconds, sub_stream_url, ai_enabled, audio_transcode,
			storage_path, created_at, updated_at,
			ai_stream_id, ai_track_timeout, ai_confidence, recording_stream_id,
			quota_bytes, quota_warning_percent, quota_critical_percent,
			supported_event_topics,
			device_id, channel_index,
			multicast_enabled, multicast_address, multicast_port, multicast_ttl,
			confidence_thresholds
		FROM cameras WHERE mediamtx_path = ?`, path)
	cam, err := scanCamera(row.Scan)
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
			retention_days, event_retention_days, detection_retention_days,
			supports_ptz, supports_imaging, supports_events,
			supports_relay, supports_audio_backchannel, snapshot_uri,
			supports_media2, supports_analytics, supports_edge_recording,
			service_capabilities,
			motion_timeout_seconds, sub_stream_url, ai_enabled, audio_transcode,
			storage_path, created_at, updated_at,
			ai_stream_id, ai_track_timeout, ai_confidence, recording_stream_id,
			quota_bytes, quota_warning_percent, quota_critical_percent,
			supported_event_topics,
			device_id, channel_index,
			multicast_enabled, multicast_address, multicast_port, multicast_ttl,
			confidence_thresholds
		FROM cameras ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cameras []*Camera
	for rows.Next() {
		cam, err := scanCamera(rows.Scan)
		if err != nil {
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
			event_retention_days = ?, detection_retention_days = ?,
			supports_ptz = ?, supports_imaging = ?, supports_events = ?,
			supports_relay = ?, supports_audio_backchannel = ?, snapshot_uri = ?,
			supports_media2 = ?, supports_analytics = ?, supports_edge_recording = ?,
			service_capabilities = ?,
			motion_timeout_seconds = ?, sub_stream_url = ?, ai_enabled = ?,
			audio_transcode = ?, storage_path = ?,
			quota_bytes = ?, quota_warning_percent = ?, quota_critical_percent = ?,
			supported_event_topics = ?,
			multicast_enabled = ?, multicast_address = ?, multicast_port = ?, multicast_ttl = ?,
			confidence_thresholds = ?,
			updated_at = ?
		WHERE id = ?`,
		cam.Name, cam.ONVIFEndpoint, cam.ONVIFUsername, cam.ONVIFPassword,
		cam.ONVIFProfileToken, cam.RTSPURL, cam.PTZCapable, cam.MediaMTXPath,
		cam.Status, cam.Tags, cam.RetentionDays,
		cam.EventRetentionDays, cam.DetectionRetentionDays,
		cam.SupportsPTZ, cam.SupportsImaging, cam.SupportsEvents,
		cam.SupportsRelay, cam.SupportsAudioBackchannel, cam.SnapshotURI,
		cam.SupportsMedia2, cam.SupportsAnalytics, cam.SupportsEdgeRecording,
		cam.ServiceCapabilities,
		cam.MotionTimeoutSeconds, cam.SubStreamURL, cam.AIEnabled, cam.AudioTranscode,
		cam.StoragePath,
		cam.QuotaBytes, cam.QuotaWarningPercent, cam.QuotaCriticalPercent,
		cam.SupportedEventTopics,
		cam.MulticastEnabled, cam.MulticastAddress, cam.MulticastPort, cam.MulticastTTL,
		cam.ConfidenceThresholds,
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

// UpdateCameraAIConfig updates the AI pipeline configuration fields of a camera.
// Returns ErrNotFound if no match.
func (d *DB) UpdateCameraAIConfig(id string, aiEnabled bool, streamID string, confidence float64, trackTimeout int) error {
	if trackTimeout <= 0 {
		trackTimeout = 5
	}
	if confidence <= 0 {
		confidence = 0.5
	}
	res, err := d.Exec(`
		UPDATE cameras
		SET ai_enabled = ?, ai_stream_id = ?, ai_confidence = ?, ai_track_timeout = ?, updated_at = ?
		WHERE id = ?`,
		aiEnabled, streamID, confidence, trackTimeout,
		time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateCameraRecordingStream updates the recording_stream_id for a camera.
func (d *DB) UpdateCameraRecordingStream(id, streamID string) error {
	res, err := d.Exec(`
		UPDATE cameras SET recording_stream_id = ?, updated_at = ?
		WHERE id = ?`,
		streamID, time.Now().UTC().Format("2006-01-02T15:04:05.000Z"), id)
	if err != nil {
		return err
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateCameraAudioTranscode updates only the audio_transcode field of a camera.
// Returns ErrNotFound if no match.
func (d *DB) UpdateCameraAudioTranscode(id string, audioTranscode bool) error {
	updatedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	res, err := d.Exec(`
		UPDATE cameras SET audio_transcode = ?, updated_at = ?
		WHERE id = ?`,
		audioTranscode, updatedAt, id,
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

// UpdateCameraRetentionPolicy updates all retention-related fields for a camera.
func (d *DB) UpdateCameraRetentionPolicy(id string, retentionDays, eventRetentionDays, detectionRetentionDays int) error {
	updatedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := d.Exec(`
		UPDATE cameras SET retention_days = ?, event_retention_days = ?,
			detection_retention_days = ?, updated_at = ?
		WHERE id = ?`,
		retentionDays, eventRetentionDays, detectionRetentionDays, updatedAt, id,
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

// UpdateCameraMulticast updates only the multicast configuration fields.
func (d *DB) UpdateCameraMulticast(id string, enabled bool, address string, port, ttl int) error {
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := d.Exec(`
		UPDATE cameras
		SET multicast_enabled = ?, multicast_address = ?, multicast_port = ?, multicast_ttl = ?, updated_at = ?
		WHERE id = ?`,
		enabled, address, port, ttl, now, id)
	if err != nil {
		return fmt.Errorf("update multicast config: %w", err)
	}
	rows, _ := res.RowsAffected()
	if rows == 0 {
		return ErrNotFound
	}
	return nil
}

// UpdateCameraConfidenceThresholds updates the per-class confidence thresholds
// for a camera. The thresholds parameter is a JSON string mapping class names
// to minimum confidence values (e.g. {"person":0.6,"car":0.4}).
// Returns ErrNotFound if no match.
func (d *DB) UpdateCameraConfidenceThresholds(id, thresholds string) error {
	updatedAt := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := d.Exec(`
		UPDATE cameras SET confidence_thresholds = ?, updated_at = ?
		WHERE id = ?`,
		thresholds, updatedAt, id,
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
