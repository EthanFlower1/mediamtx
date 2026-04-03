# KAI-26: Multi-Channel Camera Support Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Handle multi-sensor cameras (e.g., Hanwha PNM-9322VQP) by creating separate Camera records per channel, grouped under a shared Device record.

**Architecture:** A new `devices` table holds shared physical device metadata (ONVIF endpoint, credentials). Each video channel becomes its own Camera with `device_id` + `channel_index`. During ONVIF discovery, profiles are grouped by `VideoSourceConfiguration.SourceToken` to detect channels. The API supports grouped responses via `GET /cameras?group_by=device` and new `/devices` endpoints.

**Tech Stack:** Go, SQLite, Gin HTTP router, ONVIF (onvif-go library)

---

### Task 1: Database Migration — `devices` Table and Camera Columns

**Files:**
- Modify: `internal/nvr/db/migrations.go:454` (add migration 31)

- [ ] **Step 1: Write the failing test**

Create a test that opens a DB (which runs all migrations) and verifies the `devices` table exists and cameras has the new columns.

```go
// File: internal/nvr/db/devices_test.go
package db

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDevicesTableExists(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	// Verify devices table exists by inserting a row.
	_, err := database.Exec(`
		INSERT INTO devices (id, name, onvif_endpoint, onvif_username, onvif_password, channel_count, created_at, updated_at)
		VALUES ('test-dev', 'Test Device', 'http://192.168.1.1', '', '', 2, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`)
	require.NoError(t, err)

	// Verify new camera columns exist.
	_, err = database.Exec(`
		INSERT INTO cameras (id, name, rtsp_url, mediamtx_path, status, device_id, channel_index, created_at, updated_at)
		VALUES ('test-cam', 'Test Camera', 'rtsp://x', 'nvr/test/main', 'disconnected', 'test-dev', 0, '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z')
	`)
	require.NoError(t, err)

	// Verify we can read device_id back.
	var deviceID *string
	err = database.QueryRow("SELECT device_id FROM cameras WHERE id = 'test-cam'").Scan(&deviceID)
	require.NoError(t, err)
	require.NotNil(t, deviceID)
	require.Equal(t, "test-dev", *deviceID)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/db && go test -run TestDevicesTableExists -v`
Expected: FAIL — table "devices" does not exist

- [ ] **Step 3: Write the migration**

In `internal/nvr/db/migrations.go`, add migration 31 after the closing brace of migration 30 (after line 453):

```go
	// Migration 31: Multi-channel camera support (KAI-26).
	{
		version: 31,
		sql: `
		CREATE TABLE IF NOT EXISTS devices (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			manufacturer TEXT NOT NULL DEFAULT '',
			model TEXT NOT NULL DEFAULT '',
			firmware_version TEXT NOT NULL DEFAULT '',
			onvif_endpoint TEXT NOT NULL DEFAULT '',
			onvif_username TEXT NOT NULL DEFAULT '',
			onvif_password TEXT NOT NULL DEFAULT '',
			channel_count INTEGER NOT NULL DEFAULT 1,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL
		);
		ALTER TABLE cameras ADD COLUMN device_id TEXT DEFAULT NULL REFERENCES devices(id);
		ALTER TABLE cameras ADD COLUMN channel_index INTEGER DEFAULT NULL;
		CREATE INDEX IF NOT EXISTS idx_cameras_device ON cameras(device_id);
		`,
	},
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd internal/nvr/db && go test -run TestDevicesTableExists -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/db/migrations.go internal/nvr/db/devices_test.go
git commit -m "feat(db): add devices table and camera device_id/channel_index columns (KAI-26)"
```

---

### Task 2: Device DB Model and CRUD Operations

**Files:**
- Create: `internal/nvr/db/devices.go`
- Test: `internal/nvr/db/devices_test.go`

- [ ] **Step 1: Write the failing tests**

Append to `internal/nvr/db/devices_test.go`:

```go
func TestDeviceCreate(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	dev := &Device{
		Name:            "Front Multi-Sensor",
		Manufacturer:    "Hanwha",
		Model:           "PNM-9322VQP",
		FirmwareVersion: "1.0.0",
		ONVIFEndpoint:   "http://192.168.1.50:80/onvif/device_service",
		ONVIFUsername:    "admin",
		ONVIFPassword:   "encrypted-pass",
		ChannelCount:    4,
	}
	err := database.CreateDevice(dev)
	require.NoError(t, err)
	require.NotEmpty(t, dev.ID)
	require.NotEmpty(t, dev.CreatedAt)
}

func TestDeviceGet(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	dev := &Device{Name: "Test", ONVIFEndpoint: "http://x", ChannelCount: 2}
	require.NoError(t, database.CreateDevice(dev))

	got, err := database.GetDevice(dev.ID)
	require.NoError(t, err)
	require.Equal(t, dev.ID, got.ID)
	require.Equal(t, "Test", got.Name)
	require.Equal(t, 2, got.ChannelCount)
}

func TestDeviceGetNotFound(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	_, err := database.GetDevice("nonexistent")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestDeviceList(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	require.NoError(t, database.CreateDevice(&Device{Name: "Beta", ONVIFEndpoint: "http://b", ChannelCount: 1}))
	require.NoError(t, database.CreateDevice(&Device{Name: "Alpha", ONVIFEndpoint: "http://a", ChannelCount: 2}))

	devices, err := database.ListDevices()
	require.NoError(t, err)
	require.Len(t, devices, 2)
	require.Equal(t, "Alpha", devices[0].Name) // ordered by name
}

func TestDeviceDelete(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	dev := &Device{Name: "ToDelete", ONVIFEndpoint: "http://x", ChannelCount: 1}
	require.NoError(t, database.CreateDevice(dev))

	err := database.DeleteDevice(dev.ID)
	require.NoError(t, err)

	_, err = database.GetDevice(dev.ID)
	require.ErrorIs(t, err, ErrNotFound)
}

func TestDeviceDeleteNotFound(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	err := database.DeleteDevice("nonexistent")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestListCamerasByDevice(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	dev := &Device{Name: "Multi", ONVIFEndpoint: "http://x", ChannelCount: 2}
	require.NoError(t, database.CreateDevice(dev))

	cam1 := &Camera{Name: "Channel 1", RTSPURL: "rtsp://x/ch1", MediaMTXPath: "nvr/c1/main", DeviceID: dev.ID, ChannelIndex: intPtr(0)}
	cam2 := &Camera{Name: "Channel 2", RTSPURL: "rtsp://x/ch2", MediaMTXPath: "nvr/c2/main", DeviceID: dev.ID, ChannelIndex: intPtr(1)}
	require.NoError(t, database.CreateCamera(cam1))
	require.NoError(t, database.CreateCamera(cam2))

	// Also create a standalone camera.
	cam3 := &Camera{Name: "Standalone", RTSPURL: "rtsp://y", MediaMTXPath: "nvr/c3/main"}
	require.NoError(t, database.CreateCamera(cam3))

	cameras, err := database.ListCamerasByDevice(dev.ID)
	require.NoError(t, err)
	require.Len(t, cameras, 2)
	require.Equal(t, dev.ID, cameras[0].DeviceID)
}

func intPtr(i int) *int {
	return &i
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd internal/nvr/db && go test -run "TestDevice|TestListCamerasByDevice" -v`
Expected: FAIL — `Device` type not found, methods undefined

- [ ] **Step 3: Implement the Device model and CRUD**

Create `internal/nvr/db/devices.go`:

```go
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

// CreateDevice inserts a new device into the database.
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

// GetDevice retrieves a device by ID. Returns ErrNotFound if no match.
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

// ListDevices returns all devices ordered by name.
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

// UpdateDevice updates an existing device. Returns ErrNotFound if no match.
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

// DeleteDevice deletes a device by ID. Returns ErrNotFound if no match.
// Caller must delete associated cameras first or rely on cascade logic.
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

// ListCamerasByDevice returns all cameras belonging to a specific device,
// ordered by channel_index.
func (d *DB) ListCamerasByDevice(deviceID string) ([]*Camera, error) {
	rows, err := d.Query(`
		SELECT id, name, onvif_endpoint, onvif_username, onvif_password,
			onvif_profile_token, rtsp_url, ptz_capable, mediamtx_path, status, tags,
			retention_days, event_retention_days, detection_retention_days,
			supports_ptz, supports_imaging, supports_events,
			supports_relay, supports_audio_backchannel, snapshot_uri,
			supports_media2, supports_analytics, supports_edge_recording,
			motion_timeout_seconds, sub_stream_url, ai_enabled, audio_transcode,
			storage_path, created_at, updated_at,
			ai_stream_id, ai_track_timeout, ai_confidence, recording_stream_id,
			quota_bytes, quota_warning_percent, quota_critical_percent,
			device_id, channel_index
		FROM cameras WHERE device_id = ? ORDER BY channel_index`, deviceID,
	)
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
			&cam.Status, &cam.Tags, &cam.RetentionDays, &cam.EventRetentionDays, &cam.DetectionRetentionDays,
			&cam.SupportsPTZ, &cam.SupportsImaging, &cam.SupportsEvents,
			&cam.SupportsRelay, &cam.SupportsAudioBackchannel, &cam.SnapshotURI,
			&cam.SupportsMedia2, &cam.SupportsAnalytics, &cam.SupportsEdgeRecording,
			&cam.MotionTimeoutSeconds, &cam.SubStreamURL, &cam.AIEnabled, &cam.AudioTranscode,
			&cam.StoragePath, &cam.CreatedAt, &cam.UpdatedAt,
			&cam.AIStreamID, &cam.AITrackTimeout, &cam.AIConfidence, &cam.RecordingStreamID,
			&cam.QuotaBytes, &cam.QuotaWarningPercent, &cam.QuotaCriticalPercent,
			&cam.DeviceID, &cam.ChannelIndex,
		); err != nil {
			return nil, err
		}
		cameras = append(cameras, cam)
	}
	return cameras, rows.Err()
}
```

- [ ] **Step 4: Add DeviceID and ChannelIndex to Camera struct**

In `internal/nvr/db/cameras.go`, add two fields to the `Camera` struct after line 50 (`QuotaCriticalPercent`):

```go
	DeviceID             string `json:"device_id,omitempty"`
	ChannelIndex         *int   `json:"channel_index,omitempty"`
```

- [ ] **Step 5: Update Camera SQL queries to include new columns**

In `internal/nvr/db/cameras.go`:

**CreateCamera** — add `device_id, channel_index` to the INSERT column list and values. After the `quota_critical_percent` column in the INSERT (line 85), add `, device_id, channel_index`. Add the corresponding values to the VALUES placeholder and args.

**GetCamera** — add `device_id, channel_index` to the SELECT column list (after `quota_warning_percent, quota_critical_percent`) and to the Scan call.

**GetCameraByPath** — same changes as GetCamera.

**ListCameras** — same changes as GetCamera.

**ListCamerasByDevice** — already includes them (written above).

The column additions follow this pattern — add to the end of each SELECT and Scan:
```
SELECT ..., quota_bytes, quota_warning_percent, quota_critical_percent, device_id, channel_index
...
Scan(..., &cam.QuotaBytes, &cam.QuotaWarningPercent, &cam.QuotaCriticalPercent, &cam.DeviceID, &cam.ChannelIndex)
```

- [ ] **Step 6: Run tests to verify they pass**

Run: `cd internal/nvr/db && go test -run "TestDevice|TestListCamerasByDevice" -v`
Expected: PASS

- [ ] **Step 7: Run all existing DB tests to verify no regression**

Run: `cd internal/nvr/db && go test -v`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/db/devices.go internal/nvr/db/devices_test.go internal/nvr/db/cameras.go
git commit -m "feat(db): add Device model with CRUD and Camera device_id/channel_index fields (KAI-26)"
```

---

### Task 3: ONVIF Discovery — Detect Multi-Channel Cameras

**Files:**
- Modify: `internal/nvr/onvif/discovery.go`
- Test: `internal/nvr/onvif/discovery_test.go` (create)

- [ ] **Step 1: Write the failing test**

Create `internal/nvr/onvif/discovery_test.go`:

```go
package onvif

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGroupProfilesByVideoSource(t *testing.T) {
	profiles := []MediaProfile{
		{Token: "P1", Name: "Main Ch1", Width: 2560, Height: 1440, VideoSourceToken: "VS_1"},
		{Token: "P2", Name: "Sub Ch1", Width: 640, Height: 480, VideoSourceToken: "VS_1"},
		{Token: "P3", Name: "Main Ch2", Width: 2560, Height: 1440, VideoSourceToken: "VS_2"},
		{Token: "P4", Name: "Sub Ch2", Width: 640, Height: 480, VideoSourceToken: "VS_2"},
	}

	channels := GroupProfilesByVideoSource(profiles)
	require.Len(t, channels, 2)

	// Channels should be ordered by video source token.
	assert.Equal(t, "VS_1", channels[0].VideoSourceToken)
	assert.Len(t, channels[0].Profiles, 2)
	assert.Equal(t, "Channel 1", channels[0].Name)

	assert.Equal(t, "VS_2", channels[1].VideoSourceToken)
	assert.Len(t, channels[1].Profiles, 2)
	assert.Equal(t, "Channel 2", channels[1].Name)
}

func TestGroupProfilesByVideoSourceSingleChannel(t *testing.T) {
	profiles := []MediaProfile{
		{Token: "P1", Name: "Main", Width: 1920, Height: 1080, VideoSourceToken: "VS_1"},
		{Token: "P2", Name: "Sub", Width: 640, Height: 480, VideoSourceToken: "VS_1"},
	}

	channels := GroupProfilesByVideoSource(profiles)
	// Single source: should return 1 channel.
	require.Len(t, channels, 1)
	assert.Equal(t, "VS_1", channels[0].VideoSourceToken)
	assert.Len(t, channels[0].Profiles, 2)
}

func TestGroupProfilesByVideoSourceEmptyTokens(t *testing.T) {
	// When profiles have no VideoSourceToken (e.g. from unauthenticated discovery),
	// group them all into a single channel.
	profiles := []MediaProfile{
		{Token: "P1", Name: "Main", Width: 1920, Height: 1080},
		{Token: "P2", Name: "Sub", Width: 640, Height: 480},
	}

	channels := GroupProfilesByVideoSource(profiles)
	require.Len(t, channels, 1)
	assert.Len(t, channels[0].Profiles, 2)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/onvif && go test -run TestGroupProfilesByVideoSource -v`
Expected: FAIL — `GroupProfilesByVideoSource` undefined, `VideoSourceToken` field not found

- [ ] **Step 3: Add VideoSourceToken to MediaProfile**

In `internal/nvr/onvif/discovery.go`, add the field to the `MediaProfile` struct (after line 25, `Height`):

```go
	VideoSourceToken string `json:"video_source_token,omitempty"`
```

- [ ] **Step 4: Add DiscoveredChannel type and GroupProfilesByVideoSource function**

In `internal/nvr/onvif/discovery.go`, add after the `DiscoveredDevice` struct (after line 37):

```go
// DiscoveredChannel represents a single video channel on a multi-sensor device.
type DiscoveredChannel struct {
	VideoSourceToken string         `json:"video_source_token"`
	Name             string         `json:"name"`
	Profiles         []MediaProfile `json:"profiles"`
}
```

Add the `Channels` field to `DiscoveredDevice` (after line 36, `Profiles`):

```go
	Channels         []DiscoveredChannel `json:"channels,omitempty"`
```

Add the grouping function at the end of the file:

```go
// GroupProfilesByVideoSource groups profiles by their VideoSourceToken.
// Returns one DiscoveredChannel per unique video source, sorted by token.
// If all profiles have empty VideoSourceToken, returns a single channel.
func GroupProfilesByVideoSource(profiles []MediaProfile) []DiscoveredChannel {
	groups := make(map[string][]MediaProfile)
	var order []string

	for _, p := range profiles {
		token := p.VideoSourceToken
		if _, seen := groups[token]; !seen {
			order = append(order, token)
		}
		groups[token] = append(groups[token], p)
	}

	sort.Strings(order)

	channels := make([]DiscoveredChannel, 0, len(order))
	for i, token := range order {
		channels = append(channels, DiscoveredChannel{
			VideoSourceToken: token,
			Name:             fmt.Sprintf("Channel %d", i+1),
			Profiles:         groups[token],
		})
	}
	return channels
}
```

Add `"sort"` to the imports.

- [ ] **Step 5: Run test to verify it passes**

Run: `cd internal/nvr/onvif && go test -run TestGroupProfilesByVideoSource -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/onvif/discovery.go internal/nvr/onvif/discovery_test.go
git commit -m "feat(onvif): add VideoSourceToken to MediaProfile and channel grouping (KAI-26)"
```

---

### Task 4: Populate VideoSourceToken During Profile Discovery

**Files:**
- Modify: `internal/nvr/onvif/discovery.go` (`enrichDevice`, `profileToMediaProfile`)
- Modify: `internal/nvr/onvif/device.go` (`profileToMediaProfile`, `ProbeDeviceFull`)
- Modify: `internal/nvr/onvif/media2.go` (`GetProfiles2`)

- [ ] **Step 1: Update profileToMediaProfile to extract VideoSourceToken**

In `internal/nvr/onvif/device.go`, update `profileToMediaProfile` (line 124) to extract the source token:

```go
func profileToMediaProfile(p *onvifgo.Profile) MediaProfile {
	mp := MediaProfile{
		Token: p.Token,
		Name:  p.Name,
	}
	if p.VideoSourceConfiguration != nil {
		mp.VideoSourceToken = p.VideoSourceConfiguration.SourceToken
	}
	if p.VideoEncoderConfiguration != nil {
		mp.VideoCodec = p.VideoEncoderConfiguration.Encoding
		if p.VideoEncoderConfiguration.Resolution != nil {
			mp.Width = p.VideoEncoderConfiguration.Resolution.Width
			mp.Height = p.VideoEncoderConfiguration.Resolution.Height
		}
	}
	if p.AudioEncoderConfiguration != nil {
		mp.AudioCodec = p.AudioEncoderConfiguration.Encoding
	}
	return mp
}
```

- [ ] **Step 2: Update enrichDevice to build Channels**

In `internal/nvr/onvif/discovery.go`, update `enrichDevice` (line 177) to group profiles into channels when multiple video sources are detected. After the profile loop (after line 221), add:

```go
	// Group profiles by video source to detect multi-channel devices.
	if len(dev.Profiles) > 0 {
		channels := GroupProfilesByVideoSource(dev.Profiles)
		if len(channels) > 1 {
			dev.Channels = channels
		}
	}
```

- [ ] **Step 3: Add VideoSourceToken to Media2 profile parsing**

In `internal/nvr/onvif/media2.go`, update the `media2Profile` struct (line 36) to include the video source configuration:

```go
type media2Profile struct {
	Token          string               `xml:"token,attr"`
	Name           string               `xml:"Name"`
	Configurations media2Configurations `xml:"Configurations"`
}

type media2Configurations struct {
	VideoSource  *media2VideoSourceConfig  `xml:"VideoSource"`
	VideoEncoder *media2VideoEncoderConfig `xml:"VideoEncoder"`
	AudioEncoder *media2AudioEncoderConfig `xml:"AudioEncoder"`
}

type media2VideoSourceConfig struct {
	SourceToken string `xml:"SourceToken"`
}
```

In `GetProfiles2` (line 142), extract the source token:

```go
	for _, p := range env.Body.GetProfilesResponse.Profiles {
		mp := MediaProfile{
			Token: p.Token,
			Name:  p.Name,
		}
		if p.Configurations.VideoSource != nil {
			mp.VideoSourceToken = p.Configurations.VideoSource.SourceToken
		}
		if p.Configurations.VideoEncoder != nil {
			mp.VideoCodec = p.Configurations.VideoEncoder.Encoding
			mp.Width = p.Configurations.VideoEncoder.Resolution.Width
			mp.Height = p.Configurations.VideoEncoder.Resolution.Height
		}
		if p.Configurations.AudioEncoder != nil {
			mp.AudioCodec = p.Configurations.AudioEncoder.Encoding
		}
		profiles = append(profiles, mp)
	}
```

- [ ] **Step 4: Update ProbeDeviceFull to include VideoSources in result**

In `internal/nvr/onvif/device.go`, add `VideoSources` to `ProbeResult` (line 60):

```go
type ProbeResult struct {
	Profiles     []MediaProfile    `json:"profiles"`
	VideoSources []*VideoSourceInfo `json:"video_sources,omitempty"`
	SnapshotURI  string            `json:"snapshot_uri,omitempty"`
	Capabilities Capabilities      `json:"capabilities"`
}
```

In `ProbeDeviceFull` (line 69), after getting profiles (line 88), add video source detection:

```go
	// Get video sources for multi-channel detection.
	ctx2 := context.Background()
	sources, err := client.Dev.GetVideoSources(ctx2)
	if err == nil && len(sources) > 0 {
		for _, s := range sources {
			vs := &VideoSourceInfo{Token: s.Token, Framerate: s.Framerate}
			if s.Resolution != nil {
				vs.Width = s.Resolution.Width
				vs.Height = s.Resolution.Height
			}
			result.VideoSources = append(result.VideoSources, vs)
		}
	}
```

- [ ] **Step 5: Run all ONVIF tests**

Run: `cd internal/nvr/onvif && go test -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/onvif/discovery.go internal/nvr/onvif/device.go internal/nvr/onvif/media2.go
git commit -m "feat(onvif): populate VideoSourceToken in profiles and detect multi-channel devices (KAI-26)"
```

---

### Task 5: Device API Endpoints — List, Get, Delete

**Files:**
- Create: `internal/nvr/api/devices.go`
- Modify: `internal/nvr/api/router.go`
- Test: `internal/nvr/api/devices_test.go` (create)

- [ ] **Step 1: Write the failing tests**

Create `internal/nvr/api/devices_test.go`:

```go
package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func setupDeviceTest(t *testing.T) (*DeviceHandler, *db.DB, func()) {
	t.Helper()
	camHandler, cleanup := setupCameraTest(t)
	handler := &DeviceHandler{
		DB:            camHandler.DB,
		YAMLWriter:    camHandler.YAMLWriter,
		EncryptionKey: camHandler.EncryptionKey,
	}
	return handler, camHandler.DB, cleanup
}

func TestDeviceListEmpty(t *testing.T) {
	handler, _, cleanup := setupDeviceTest(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/devices", handler.List)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/devices", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result []interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Empty(t, result)
}

func TestDeviceGetNotFound(t *testing.T) {
	handler, _, cleanup := setupDeviceTest(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/devices/:id", handler.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/devices/nonexistent", nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestDeviceGetWithCameras(t *testing.T) {
	handler, database, cleanup := setupDeviceTest(t)
	defer cleanup()

	// Create a device with two cameras.
	dev := &db.Device{Name: "Multi", Manufacturer: "Hanwha", ONVIFEndpoint: "http://x", ChannelCount: 2}
	require.NoError(t, database.CreateDevice(dev))

	cam1 := &db.Camera{Name: "Ch1", RTSPURL: "rtsp://x/ch1", MediaMTXPath: "nvr/c1/main", DeviceID: dev.ID, ChannelIndex: intPtr(0)}
	cam2 := &db.Camera{Name: "Ch2", RTSPURL: "rtsp://x/ch2", MediaMTXPath: "nvr/c2/main", DeviceID: dev.ID, ChannelIndex: intPtr(1)}
	require.NoError(t, database.CreateCamera(cam1))
	require.NoError(t, database.CreateCamera(cam2))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/devices/:id", handler.Get)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/devices/"+dev.ID, nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	assert.Equal(t, dev.ID, result["id"])
	cameras := result["cameras"].([]interface{})
	assert.Len(t, cameras, 2)
}

func TestDeviceDeleteCascade(t *testing.T) {
	handler, database, cleanup := setupDeviceTest(t)
	defer cleanup()

	dev := &db.Device{Name: "ToDelete", ONVIFEndpoint: "http://x", ChannelCount: 1}
	require.NoError(t, database.CreateDevice(dev))

	cam := &db.Camera{Name: "Ch1", RTSPURL: "rtsp://x/ch1", MediaMTXPath: "nvr/del/main", DeviceID: dev.ID, ChannelIndex: intPtr(0)}
	require.NoError(t, database.CreateCamera(cam))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.DELETE("/devices/:id", handler.Delete)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("DELETE", "/devices/"+dev.ID, nil)
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)

	// Device should be gone.
	_, err := database.GetDevice(dev.ID)
	assert.ErrorIs(t, err, db.ErrNotFound)

	// Camera should also be gone.
	_, err = database.GetCamera(cam.ID)
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func intPtr(i int) *int {
	return &i
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd internal/nvr/api && go test -run "TestDevice" -v`
Expected: FAIL — `DeviceHandler` undefined

- [ ] **Step 3: Implement DeviceHandler**

Create `internal/nvr/api/devices.go`:

```go
package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// DeviceHandler implements HTTP endpoints for physical device management.
type DeviceHandler struct {
	DB            *db.DB
	YAMLWriter    *yamlwriter.Writer
	EncryptionKey []byte
	Scheduler     interface{ RemoveCamera(string) }
}

// deviceWithCameras wraps a Device with its child cameras.
type deviceWithCameras struct {
	db.Device
	Cameras []*db.Camera `json:"cameras"`
}

// List returns all devices with their cameras.
func (h *DeviceHandler) List(c *gin.Context) {
	devices, err := h.DB.ListDevices()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list devices", err)
		return
	}

	result := make([]deviceWithCameras, 0, len(devices))
	for _, dev := range devices {
		cameras, _ := h.DB.ListCamerasByDevice(dev.ID)
		if cameras == nil {
			cameras = []*db.Camera{}
		}
		result = append(result, deviceWithCameras{Device: *dev, Cameras: cameras})
	}
	c.JSON(http.StatusOK, result)
}

// Get returns a single device with its cameras.
func (h *DeviceHandler) Get(c *gin.Context) {
	id := c.Param("id")

	dev, err := h.DB.GetDevice(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve device", err)
		return
	}

	cameras, _ := h.DB.ListCamerasByDevice(dev.ID)
	if cameras == nil {
		cameras = []*db.Camera{}
	}
	c.JSON(http.StatusOK, deviceWithCameras{Device: *dev, Cameras: cameras})
}

// Delete removes a device and all its cameras from the database and YAML config.
func (h *DeviceHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	dev, err := h.DB.GetDevice(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve device", err)
		return
	}

	// Delete all cameras belonging to this device.
	cameras, _ := h.DB.ListCamerasByDevice(id)
	for _, cam := range cameras {
		if cam.MediaMTXPath != "" {
			_ = h.YAMLWriter.RemovePath(cam.MediaMTXPath)
		}
		// Remove sub-stream paths.
		streams, _ := h.DB.ListCameraStreams(cam.ID)
		for i, s := range streams {
			if i > 0 {
				subPath := cameraStreamPath(cam, s.ID)
				_ = h.YAMLWriter.RemovePath(subPath)
			}
		}
		_ = h.DB.DeleteStreamsByCamera(cam.ID)
		_ = h.DB.DeleteCamera(cam.ID)
		if h.Scheduler != nil {
			h.Scheduler.RemoveCamera(cam.ID)
		}
	}

	if err := h.DB.DeleteDevice(id); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete device", err)
		return
	}

	nvrLogInfo("devices", fmt.Sprintf("Deleted device %q (id=%s) with %d cameras", dev.Name, id, len(cameras)))
	c.JSON(http.StatusOK, gin.H{"message": "device deleted"})
}
```

- [ ] **Step 4: Register device routes**

In `internal/nvr/api/router.go`, add device handler creation in `RegisterRoutes` (near where `cameraHandler` is created) and add routes. After the camera routes block (around line 193), add:

```go
	// Devices.
	deviceHandler := &DeviceHandler{
		DB:         cfg.DB,
		YAMLWriter: cfg.YAMLWriter,
	}
	protected.GET("/devices", deviceHandler.List)
	protected.GET("/devices/:id", deviceHandler.Get)
	protected.DELETE("/devices/:id", deviceHandler.Delete)
```

Import `yamlwriter` if not already imported (it likely is since `CameraHandler` uses it).

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd internal/nvr/api && go test -run "TestDevice" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/api/devices.go internal/nvr/api/devices_test.go internal/nvr/api/router.go
git commit -m "feat(api): add device list, get, and cascading delete endpoints (KAI-26)"
```

---

### Task 6: Multi-Channel Camera Creation API

**Files:**
- Modify: `internal/nvr/api/cameras.go` (Create handler)
- Test: `internal/nvr/api/cameras_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/nvr/api/cameras_test.go`:

```go
func TestCreateMultiChannelCamera(t *testing.T) {
	handler, cleanup := setupCameraTest(t)
	defer cleanup()

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.POST("/cameras", handler.Create)

	body := `{
		"device": {
			"name": "Front Multi-Sensor",
			"onvif_endpoint": "http://192.168.1.50",
			"onvif_username": "admin",
			"onvif_password": "pass"
		},
		"channels": [
			{
				"name": "Front Left",
				"rtsp_url": "rtsp://192.168.1.50/ch1",
				"profiles": [{"name": "Main", "rtsp_url": "rtsp://192.168.1.50/ch1", "video_codec": "H264", "width": 2560, "height": 1440}],
				"channel_index": 0
			},
			{
				"name": "Front Right",
				"rtsp_url": "rtsp://192.168.1.50/ch2",
				"profiles": [{"name": "Main", "rtsp_url": "rtsp://192.168.1.50/ch2", "video_codec": "H264", "width": 2560, "height": 1440}],
				"channel_index": 1
			}
		]
	}`

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("POST", "/cameras", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code)

	var result map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))

	// Should have a device_id in the response.
	deviceID, ok := result["device_id"].(string)
	require.True(t, ok, "response should include device_id")
	require.NotEmpty(t, deviceID)

	// Should have created cameras array.
	cameras, ok := result["cameras"].([]interface{})
	require.True(t, ok, "response should include cameras array")
	require.Len(t, cameras, 2)

	// Verify device was created in DB.
	dev, err := handler.DB.GetDevice(deviceID)
	require.NoError(t, err)
	require.Equal(t, "Front Multi-Sensor", dev.Name)
	require.Equal(t, 2, dev.ChannelCount)
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/api && go test -run TestCreateMultiChannelCamera -v`
Expected: FAIL — `device` and `channels` fields not recognized in cameraRequest

- [ ] **Step 3: Add multi-channel request types**

In `internal/nvr/api/cameras.go`, add a new request type after `cameraRequest` (after line 216):

```go
// multiChannelRequest is the JSON body for creating a multi-channel camera device.
type multiChannelRequest struct {
	Device *deviceInfo `json:"device"`
	Channels []channelInfo `json:"channels"`
}

type deviceInfo struct {
	Name          string `json:"name"`
	ONVIFEndpoint string `json:"onvif_endpoint"`
	ONVIFUsername  string `json:"onvif_username"`
	ONVIFPassword string `json:"onvif_password"`
}

type channelInfo struct {
	Name         string `json:"name"`
	RTSPURL      string `json:"rtsp_url"`
	ChannelIndex int    `json:"channel_index"`
	Profiles     []struct {
		Name         string `json:"name"`
		RTSPURL      string `json:"rtsp_url"`
		ProfileToken string `json:"profile_token"`
		VideoCodec   string `json:"video_codec"`
		AudioCodec   string `json:"audio_codec"`
		Width        int    `json:"width"`
		Height       int    `json:"height"`
		Roles        string `json:"roles"`
	} `json:"profiles"`
}
```

- [ ] **Step 4: Add CreateMultiChannel handler**

In `internal/nvr/api/cameras.go`, add a new handler method:

```go
// CreateMultiChannel creates a device with multiple camera channels.
func (h *CameraHandler) CreateMultiChannel(c *gin.Context) {
	var req multiChannelRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}
	if req.Device == nil || len(req.Channels) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device and channels are required"})
		return
	}

	// Create the device record.
	dev := &db.Device{
		Name:          req.Device.Name,
		ONVIFEndpoint: req.Device.ONVIFEndpoint,
		ONVIFUsername:  req.Device.ONVIFUsername,
		ONVIFPassword: h.encryptPassword(req.Device.ONVIFPassword),
		ChannelCount:  len(req.Channels),
	}
	if err := h.DB.CreateDevice(dev); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create device", err)
		return
	}

	storagePath := "./recordings"

	var cameras []*db.Camera
	for _, ch := range req.Channels {
		if ch.RTSPURL == "" || !strings.HasPrefix(ch.RTSPURL, "rtsp://") {
			// Clean up: delete device and any cameras created so far.
			for _, cam := range cameras {
				_ = h.DB.DeleteCamera(cam.ID)
				_ = h.YAMLWriter.RemovePath(cam.MediaMTXPath)
			}
			_ = h.DB.DeleteDevice(dev.ID)
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("channel %d: rtsp_url must start with rtsp://", ch.ChannelIndex)})
			return
		}

		camID := uuid.New().String()
		pathName := "nvr/" + camID + "/main"
		recordPath := storagePath + "/%path/%Y-%m/%d/%H-%M-%S-%f"
		chIdx := ch.ChannelIndex

		cam := &db.Camera{
			ID:           camID,
			Name:         ch.Name,
			ONVIFEndpoint: dev.ONVIFEndpoint,
			ONVIFUsername: dev.ONVIFUsername,
			ONVIFPassword: dev.ONVIFPassword,
			RTSPURL:      ch.RTSPURL,
			MediaMTXPath: pathName,
			DeviceID:     dev.ID,
			ChannelIndex: &chIdx,
		}
		if err := h.DB.CreateCamera(cam); err != nil {
			apiError(c, http.StatusInternalServerError, "failed to create camera for channel", err)
			return
		}

		// Auto-create streams from profiles.
		for i, p := range ch.Profiles {
			roles := p.Roles
			if roles == "" {
				switch {
				case len(ch.Profiles) == 1:
					roles = "live_view,recording,ai_detection,mobile"
				case i == 0:
					roles = "live_view"
				case i == len(ch.Profiles)-1:
					roles = "recording,ai_detection,mobile"
				}
			}
			stream := &db.CameraStream{
				CameraID:     cam.ID,
				Name:         p.Name,
				RTSPURL:      p.RTSPURL,
				ProfileToken: p.ProfileToken,
				VideoCodec:   p.VideoCodec,
				AudioCodec:   p.AudioCodec,
				Width:        p.Width,
				Height:       p.Height,
				Roles:        roles,
			}
			if err := h.DB.CreateCameraStream(stream); err != nil {
				nvrLogWarn("cameras", fmt.Sprintf("failed to create stream for channel camera %s: %v", cam.ID, err))
			}
		}

		// Write YAML path for main stream.
		yamlConfig := map[string]interface{}{
			"source":     cam.RTSPURL,
			"record":     false,
			"recordPath": recordPath,
		}
		if err := h.YAMLWriter.AddPath(pathName, yamlConfig); err != nil {
			nvrLogWarn("cameras", fmt.Sprintf("failed to write config for channel camera %s: %v", cam.ID, err))
		}

		// Write YAML paths for sub-streams.
		streams, _ := h.DB.ListCameraStreams(cam.ID)
		for i, s := range streams {
			if i == 0 {
				continue
			}
			subPath := cameraStreamPath(cam, s.ID)
			subConfig := map[string]interface{}{
				"source":     s.RTSPURL,
				"record":     false,
				"recordPath": recordPath,
			}
			if err := h.YAMLWriter.AddPath(subPath, subConfig); err != nil {
				nvrLogWarn("cameras", fmt.Sprintf("failed to write sub-stream path %s: %v", subPath, err))
			}
		}

		cameras = append(cameras, cam)
	}

	nvrLogInfo("cameras", fmt.Sprintf("Created device %q (id=%s) with %d channels", dev.Name, dev.ID, len(cameras)))

	// Return device with cameras.
	c.JSON(http.StatusCreated, gin.H{
		"device_id": dev.ID,
		"device":    dev,
		"cameras":   cameras,
	})
}
```

Add `"github.com/google/uuid"` to the imports if not already present (it likely is).

- [ ] **Step 5: Wire up the route**

In `internal/nvr/api/router.go`, add after the existing `POST /cameras` route:

```go
	protected.POST("/cameras/multi-channel", cameraHandler.CreateMultiChannel)
```

Also update the test to use this endpoint. In the test, change the URL from `"/cameras"` to `"/cameras/multi-channel"`.

- [ ] **Step 6: Run test to verify it passes**

Run: `cd internal/nvr/api && go test -run TestCreateMultiChannelCamera -v`
Expected: PASS

- [ ] **Step 7: Run all existing API tests for regression**

Run: `cd internal/nvr/api && go test -v`
Expected: All PASS

- [ ] **Step 8: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/cameras_test.go internal/nvr/api/router.go
git commit -m "feat(api): add multi-channel camera creation endpoint (KAI-26)"
```

---

### Task 7: Camera List with Device Grouping

**Files:**
- Modify: `internal/nvr/api/cameras.go` (List handler)
- Test: `internal/nvr/api/cameras_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/nvr/api/cameras_test.go`:

```go
func TestCameraListGroupByDevice(t *testing.T) {
	handler, cleanup := setupCameraTest(t)
	defer cleanup()

	// Create a device with 2 cameras.
	dev := &db.Device{Name: "Multi", ONVIFEndpoint: "http://x", ChannelCount: 2}
	require.NoError(t, handler.DB.CreateDevice(dev))

	cam1 := &db.Camera{Name: "Ch1", RTSPURL: "rtsp://x/ch1", MediaMTXPath: "nvr/c1/main", DeviceID: dev.ID, ChannelIndex: intPtr(0)}
	cam2 := &db.Camera{Name: "Ch2", RTSPURL: "rtsp://x/ch2", MediaMTXPath: "nvr/c2/main", DeviceID: dev.ID, ChannelIndex: intPtr(1)}
	require.NoError(t, handler.DB.CreateCamera(cam1))
	require.NoError(t, handler.DB.CreateCamera(cam2))

	// Create a standalone camera.
	cam3 := &db.Camera{Name: "Standalone", RTSPURL: "rtsp://y", MediaMTXPath: "nvr/c3/main"}
	require.NoError(t, handler.DB.CreateCamera(cam3))

	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET("/cameras", handler.List)

	// Test group_by=device.
	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/cameras?group_by=device", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var result struct {
		Devices    []map[string]interface{} `json:"devices"`
		Standalone []map[string]interface{} `json:"standalone"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &result))
	require.Len(t, result.Devices, 1)
	require.Len(t, result.Standalone, 1)

	deviceEntry := result.Devices[0]
	deviceCameras := deviceEntry["cameras"].([]interface{})
	assert.Len(t, deviceCameras, 2)
	assert.Equal(t, dev.ID, deviceEntry["id"])
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `cd internal/nvr/api && go test -run TestCameraListGroupByDevice -v`
Expected: FAIL — response is a flat array, not a grouped object

- [ ] **Step 3: Update List handler to support group_by=device**

In `internal/nvr/api/cameras.go`, modify the `List` method (line 237). Replace the entire method body:

```go
func (h *CameraHandler) List(c *gin.Context) {
	cameras, err := h.DB.ListCameras()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list cameras", err)
		return
	}
	if cameras == nil {
		cameras = []*db.Camera{}
	}

	// Batch-fetch all path statuses in a single HTTP call.
	statuses := h.getPathStatuses()
	for _, cam := range cameras {
		if statuses == nil {
			cam.Status = "unknown"
		} else if cam.MediaMTXPath != "" {
			if s, ok := statuses[cam.MediaMTXPath]; ok {
				cam.Status = s
			} else {
				cam.Status = "disconnected"
			}
		}
	}

	if c.Query("group_by") == "device" {
		h.listGroupedByDevice(c, cameras)
		return
	}

	responses := make([]cameraWithStreams, 0, len(cameras))
	for _, cam := range cameras {
		responses = append(responses, h.buildCameraWithStreams(cam))
	}
	c.JSON(http.StatusOK, responses)
}

// groupedDeviceEntry wraps a device with its enriched camera responses.
type groupedDeviceEntry struct {
	db.Device
	Cameras []cameraWithStreams `json:"cameras"`
}

// groupedResponse is the response format when group_by=device.
type groupedResponse struct {
	Devices    []groupedDeviceEntry `json:"devices"`
	Standalone []cameraWithStreams   `json:"standalone"`
}

func (h *CameraHandler) listGroupedByDevice(c *gin.Context, cameras []*db.Camera) {
	deviceCameras := make(map[string][]*db.Camera)
	var standalone []*db.Camera

	for _, cam := range cameras {
		if cam.DeviceID != "" {
			deviceCameras[cam.DeviceID] = append(deviceCameras[cam.DeviceID], cam)
		} else {
			standalone = append(standalone, cam)
		}
	}

	var deviceEntries []groupedDeviceEntry
	for deviceID, cams := range deviceCameras {
		dev, err := h.DB.GetDevice(deviceID)
		if err != nil {
			continue
		}
		entry := groupedDeviceEntry{Device: *dev}
		for _, cam := range cams {
			entry.Cameras = append(entry.Cameras, h.buildCameraWithStreams(cam))
		}
		deviceEntries = append(deviceEntries, entry)
	}

	standaloneResponses := make([]cameraWithStreams, 0, len(standalone))
	for _, cam := range standalone {
		standaloneResponses = append(standaloneResponses, h.buildCameraWithStreams(cam))
	}

	c.JSON(http.StatusOK, groupedResponse{
		Devices:    deviceEntries,
		Standalone: standaloneResponses,
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `cd internal/nvr/api && go test -run TestCameraListGroupByDevice -v`
Expected: PASS

- [ ] **Step 5: Run all API tests to verify no regression**

Run: `cd internal/nvr/api && go test -v`
Expected: All PASS

- [ ] **Step 6: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/cameras_test.go
git commit -m "feat(api): add group_by=device support to camera list endpoint (KAI-26)"
```

---

### Task 8: Credential Resolution from Device

**Files:**
- Modify: `internal/nvr/api/cameras.go` (background probe, RefreshCapabilities)
- Test: `internal/nvr/api/cameras_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/nvr/api/cameras_test.go`:

```go
func TestResolveDeviceCredentials(t *testing.T) {
	handler, cleanup := setupCameraTest(t)
	defer cleanup()
	handler.EncryptionKey = []byte("0123456789abcdef0123456789abcdef") // 32-byte key

	dev := &db.Device{
		Name:          "Multi",
		ONVIFEndpoint: "http://192.168.1.50",
		ONVIFUsername: "admin",
		ONVIFPassword: handler.encryptPassword("secret"),
		ChannelCount:  2,
	}
	require.NoError(t, handler.DB.CreateDevice(dev))

	cam := &db.Camera{
		Name:         "Ch1",
		RTSPURL:      "rtsp://x",
		MediaMTXPath: "nvr/c1/main",
		DeviceID:     dev.ID,
		ChannelIndex: intPtr(0),
	}
	require.NoError(t, handler.DB.CreateCamera(cam))

	// Resolve credentials for a device-linked camera.
	endpoint, username, password := handler.resolveONVIFCredentials(cam)
	assert.Equal(t, "http://192.168.1.50", endpoint)
	assert.Equal(t, "admin", username)
	assert.Equal(t, "secret", password)
}

func TestResolveStandaloneCredentials(t *testing.T) {
	handler, cleanup := setupCameraTest(t)
	defer cleanup()
	handler.EncryptionKey = []byte("0123456789abcdef0123456789abcdef")

	cam := &db.Camera{
		Name:          "Standalone",
		RTSPURL:       "rtsp://y",
		MediaMTXPath:  "nvr/s1/main",
		ONVIFEndpoint: "http://192.168.1.100",
		ONVIFUsername:  "user",
		ONVIFPassword: handler.encryptPassword("pwd"),
	}
	require.NoError(t, handler.DB.CreateCamera(cam))

	endpoint, username, password := handler.resolveONVIFCredentials(cam)
	assert.Equal(t, "http://192.168.1.100", endpoint)
	assert.Equal(t, "user", username)
	assert.Equal(t, "pwd", password)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd internal/nvr/api && go test -run "TestResolve.*Credentials" -v`
Expected: FAIL — `resolveONVIFCredentials` undefined

- [ ] **Step 3: Implement resolveONVIFCredentials**

In `internal/nvr/api/cameras.go`, add a helper method:

```go
// resolveONVIFCredentials returns the ONVIF endpoint, username, and decrypted
// password for a camera. If the camera belongs to a device, credentials are
// read from the device record.
func (h *CameraHandler) resolveONVIFCredentials(cam *db.Camera) (endpoint, username, password string) {
	if cam.DeviceID != "" {
		dev, err := h.DB.GetDevice(cam.DeviceID)
		if err == nil {
			return dev.ONVIFEndpoint, dev.ONVIFUsername, h.decryptPassword(dev.ONVIFPassword)
		}
	}
	return cam.ONVIFEndpoint, cam.ONVIFUsername, h.decryptPassword(cam.ONVIFPassword)
}
```

- [ ] **Step 4: Update background probe to use resolveONVIFCredentials**

In the `Create` handler's background probe goroutine (around line 477), replace:

```go
result, probeErr = onvif.ProbeDeviceFull(camCopy.ONVIFEndpoint, camCopy.ONVIFUsername, h.decryptPassword(camCopy.ONVIFPassword))
```

with:

```go
endpoint, user, pass := h.resolveONVIFCredentials(&camCopy)
result, probeErr = onvif.ProbeDeviceFull(endpoint, user, pass)
```

Also update `RefreshCapabilities` (search for the method) to use the same helper.

- [ ] **Step 5: Run tests to verify they pass**

Run: `cd internal/nvr/api && go test -run "TestResolve.*Credentials" -v`
Expected: PASS

- [ ] **Step 6: Run all tests**

Run: `cd internal/nvr/api && go test -v`
Expected: All PASS

- [ ] **Step 7: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/cameras_test.go
git commit -m "feat(api): resolve ONVIF credentials from device for multi-channel cameras (KAI-26)"
```

---

### Task 9: Discovery Results Include Channel Grouping

**Files:**
- Modify: `internal/nvr/api/cameras.go` (DiscoverResults handler)
- Test: `internal/nvr/onvif/discovery_test.go`

- [ ] **Step 1: Verify channel grouping is already exposed**

The `DiscoverResults` handler returns `[]DiscoveredDevice` directly from `Discovery.GetResults()`. Since we added `Channels []DiscoveredChannel` to `DiscoveredDevice` and populate it in `enrichDevice`, the discovery results API already includes channel data when multiple video sources are detected.

Verify the existing discovery test still passes:

Run: `cd internal/nvr/onvif && go test -v`
Expected: All PASS

- [ ] **Step 2: Write integration test for channel grouping in discovery results**

Append to `internal/nvr/onvif/discovery_test.go`:

```go
func TestDiscoveredDeviceChannelsJSON(t *testing.T) {
	// Verify that a DiscoveredDevice with channels serializes correctly.
	dev := DiscoveredDevice{
		XAddr:        "http://192.168.1.50",
		Manufacturer: "Hanwha",
		Model:        "PNM-9322VQP",
		Profiles: []MediaProfile{
			{Token: "P1", Name: "Main Ch1", VideoSourceToken: "VS_1", Width: 2560, Height: 1440},
			{Token: "P2", Name: "Main Ch2", VideoSourceToken: "VS_2", Width: 2560, Height: 1440},
		},
		Channels: []DiscoveredChannel{
			{
				VideoSourceToken: "VS_1",
				Name:             "Channel 1",
				Profiles:         []MediaProfile{{Token: "P1", Name: "Main Ch1", VideoSourceToken: "VS_1", Width: 2560, Height: 1440}},
			},
			{
				VideoSourceToken: "VS_2",
				Name:             "Channel 2",
				Profiles:         []MediaProfile{{Token: "P2", Name: "Main Ch2", VideoSourceToken: "VS_2", Width: 2560, Height: 1440}},
			},
		},
	}

	data, err := json.Marshal(dev)
	require.NoError(t, err)

	var decoded map[string]interface{}
	require.NoError(t, json.Unmarshal(data, &decoded))

	channels, ok := decoded["channels"].([]interface{})
	require.True(t, ok, "channels should be present in JSON")
	assert.Len(t, channels, 2)
}
```

Add `"encoding/json"` to the imports in `discovery_test.go`.

- [ ] **Step 3: Run test to verify it passes**

Run: `cd internal/nvr/onvif && go test -run TestDiscoveredDeviceChannelsJSON -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/onvif/discovery_test.go
git commit -m "test(onvif): add channel grouping serialization test (KAI-26)"
```

---

### Task 10: Final Integration — Build Verification and Cleanup

**Files:**
- All modified files

- [ ] **Step 1: Run full test suite**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -v`
Expected: All PASS

- [ ] **Step 2: Build verification**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: Build succeeds with no errors

- [ ] **Step 3: Run go vet**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go vet ./internal/nvr/...`
Expected: No issues

- [ ] **Step 4: Commit any fixups if needed**

If any fixes were needed, commit them:

```bash
git add -A
git commit -m "fix: address build/vet issues from multi-channel camera support (KAI-26)"
```

- [ ] **Step 5: Final commit message for the feature**

If all tests pass and the build succeeds, no additional commit needed. The feature is complete.
