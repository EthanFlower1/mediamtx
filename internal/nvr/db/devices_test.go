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
