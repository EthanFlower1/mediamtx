package db

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDB(t *testing.T) *DB {
	t.Helper()
	d, err := Open(context.Background(), ":memory:")
	require.NoError(t, err)
	t.Cleanup(func() { _ = d.Close() })
	return d
}

func TestCameraCreateAndGet(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{
		Name:          "Front Door",
		ONVIFEndpoint: "http://192.0.2.1/onvif/device_service",
		RTSPURL:       "rtsp://192.0.2.1/stream1",
		MediaMTXPath:  "front_door",
	}
	require.NoError(t, d.CreateCamera(cam))
	assert.NotEmpty(t, cam.ID)
	assert.Equal(t, "disconnected", cam.Status)
	assert.Equal(t, 80, cam.QuotaWarningPercent)
	assert.Equal(t, 90, cam.QuotaCriticalPercent)

	got, err := d.GetCamera(cam.ID)
	require.NoError(t, err)
	assert.Equal(t, cam.ID, got.ID)
	assert.Equal(t, "Front Door", got.Name)
	assert.Equal(t, "rtsp://192.0.2.1/stream1", got.RTSPURL)
	assert.Equal(t, "front_door", got.MediaMTXPath)
}

func TestCameraGetByPath(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "Back Door", MediaMTXPath: "back_door"}
	require.NoError(t, d.CreateCamera(cam))

	got, err := d.GetCameraByPath("back_door")
	require.NoError(t, err)
	assert.Equal(t, cam.ID, got.ID)
	assert.Equal(t, "Back Door", got.Name)

	_, err = d.GetCameraByPath("nonexistent")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCameraList(t *testing.T) {
	d := openTestDB(t)

	require.NoError(t, d.CreateCamera(&Camera{Name: "Zebra Cam"}))
	require.NoError(t, d.CreateCamera(&Camera{Name: "Alpha Cam"}))
	require.NoError(t, d.CreateCamera(&Camera{Name: "Middle Cam"}))

	cameras, err := d.ListCameras()
	require.NoError(t, err)
	require.Len(t, cameras, 3)
	// Ordered by name.
	assert.Equal(t, "Alpha Cam", cameras[0].Name)
	assert.Equal(t, "Middle Cam", cameras[1].Name)
	assert.Equal(t, "Zebra Cam", cameras[2].Name)
}

func TestCameraUpdate(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "Old Name", RTSPURL: "rtsp://old"}
	require.NoError(t, d.CreateCamera(cam))

	cam.Name = "New Name"
	cam.RTSPURL = "rtsp://new"
	require.NoError(t, d.UpdateCamera(cam))

	got, err := d.GetCamera(cam.ID)
	require.NoError(t, err)
	assert.Equal(t, "New Name", got.Name)
	assert.Equal(t, "rtsp://new", got.RTSPURL)
}

func TestCameraDelete(t *testing.T) {
	d := openTestDB(t)

	cam := &Camera{Name: "To Delete"}
	require.NoError(t, d.CreateCamera(cam))

	require.NoError(t, d.DeleteCamera(cam.ID))

	_, err := d.GetCamera(cam.ID)
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCameraGetNotFound(t *testing.T) {
	d := openTestDB(t)

	_, err := d.GetCamera("does-not-exist")
	assert.ErrorIs(t, err, ErrNotFound)
}

func TestCameraDelete_NotFound(t *testing.T) {
	d := openTestDB(t)

	err := d.DeleteCamera("does-not-exist")
	assert.ErrorIs(t, err, ErrNotFound)
}
