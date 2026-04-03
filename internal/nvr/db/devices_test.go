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

func TestDeviceCreate(t *testing.T) {
	database := newTestDB(t)
	defer database.Close()

	dev := &Device{
		Name:            "Front Multi-Sensor",
		Manufacturer:    "Hanwha",
		Model:           "PNM-9322VQP",
		FirmwareVersion: "1.0.0",
		ONVIFEndpoint:   "http://192.168.1.50:80/onvif/device_service",
		ONVIFUsername:   "admin",
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
	require.Equal(t, "Alpha", devices[0].Name)
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
