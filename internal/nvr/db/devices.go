package db

import (
	"database/sql"
	"errors"
	"time"

	"github.com/google/uuid"
)

// Device represents a physical ONVIF device that may have multiple channels.
type Device struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	Manufacturer    string `json:"manufacturer"`
	Model           string `json:"model"`
	FirmwareVersion string `json:"firmware_version"`
	ONVIFEndpoint   string `json:"onvif_endpoint"`
	ONVIFUsername   string `json:"onvif_username"`
	ONVIFPassword   string `json:"-"`
	ChannelCount    int    `json:"channel_count"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

func (d *DB) CreateDevice(dev *Device) error {
	if dev.ID == "" {
		dev.ID = uuid.New().String()
	}
	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	dev.CreatedAt = now
	dev.UpdatedAt = now
	if dev.ChannelCount < 1 {
		dev.ChannelCount = 1
	}
	_, err := d.Exec(`
		INSERT INTO devices (id, name, manufacturer, model, firmware_version,
			onvif_endpoint, onvif_username, onvif_password, channel_count,
			created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		dev.ID, dev.Name, dev.Manufacturer, dev.Model, dev.FirmwareVersion,
		dev.ONVIFEndpoint, dev.ONVIFUsername, dev.ONVIFPassword, dev.ChannelCount,
		dev.CreatedAt, dev.UpdatedAt,
	)
	return err
}

func (d *DB) GetDevice(id string) (*Device, error) {
	dev := &Device{}
	err := d.QueryRow(`
		SELECT id, name, manufacturer, model, firmware_version,
			onvif_endpoint, onvif_username, onvif_password, channel_count,
			created_at, updated_at
		FROM devices WHERE id = ?`, id,
	).Scan(
		&dev.ID, &dev.Name, &dev.Manufacturer, &dev.Model, &dev.FirmwareVersion,
		&dev.ONVIFEndpoint, &dev.ONVIFUsername, &dev.ONVIFPassword, &dev.ChannelCount,
		&dev.CreatedAt, &dev.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return dev, nil
}

func (d *DB) ListDevices() ([]*Device, error) {
	rows, err := d.Query(`
		SELECT id, name, manufacturer, model, firmware_version,
			onvif_endpoint, onvif_username, onvif_password, channel_count,
			created_at, updated_at
		FROM devices ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var devices []*Device
	for rows.Next() {
		dev := &Device{}
		if err := rows.Scan(
			&dev.ID, &dev.Name, &dev.Manufacturer, &dev.Model, &dev.FirmwareVersion,
			&dev.ONVIFEndpoint, &dev.ONVIFUsername, &dev.ONVIFPassword, &dev.ChannelCount,
			&dev.CreatedAt, &dev.UpdatedAt,
		); err != nil {
			return nil, err
		}
		devices = append(devices, dev)
	}
	return devices, rows.Err()
}

func (d *DB) UpdateDevice(dev *Device) error {
	dev.UpdatedAt = time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	res, err := d.Exec(`
		UPDATE devices SET name = ?, manufacturer = ?, model = ?, firmware_version = ?,
			onvif_endpoint = ?, onvif_username = ?, onvif_password = ?,
			channel_count = ?, updated_at = ?
		WHERE id = ?`,
		dev.Name, dev.Manufacturer, dev.Model, dev.FirmwareVersion,
		dev.ONVIFEndpoint, dev.ONVIFUsername, dev.ONVIFPassword,
		dev.ChannelCount, dev.UpdatedAt, dev.ID,
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

func (d *DB) DeleteDevice(id string) error {
	res, err := d.Exec("DELETE FROM devices WHERE id = ?", id)
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

func (d *DB) ListCamerasByDevice(deviceID string) ([]*Camera, error) {
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
		FROM cameras WHERE device_id = ? ORDER BY channel_index`, deviceID,
	)
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
