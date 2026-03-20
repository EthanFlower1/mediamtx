package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func newTestDB(t *testing.T) *DB {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func TestCameraCreate(t *testing.T) {
	d := newTestDB(t)

	cam := &Camera{Name: "Front Door", RTSPURL: "rtsp://192.168.1.10/stream"}
	err := d.CreateCamera(cam)
	require.NoError(t, err)
	require.NotEmpty(t, cam.ID)
	require.Equal(t, "disconnected", cam.Status)
	require.NotEmpty(t, cam.CreatedAt)
}

func TestCameraGet(t *testing.T) {
	d := newTestDB(t)

	cam := &Camera{Name: "Garage", RTSPURL: "rtsp://192.168.1.11/stream"}
	require.NoError(t, d.CreateCamera(cam))

	got, err := d.GetCamera(cam.ID)
	require.NoError(t, err)
	require.Equal(t, cam.ID, got.ID)
	require.Equal(t, "Garage", got.Name)
	require.Equal(t, cam.RTSPURL, got.RTSPURL)
}

func TestCameraGetNotFound(t *testing.T) {
	d := newTestDB(t)

	_, err := d.GetCamera("nonexistent-id")
	require.ErrorIs(t, err, ErrNotFound)
}

func TestCameraList(t *testing.T) {
	d := newTestDB(t)

	require.NoError(t, d.CreateCamera(&Camera{Name: "Backyard", MediaMTXPath: "cameras/backyard"}))
	require.NoError(t, d.CreateCamera(&Camera{Name: "Attic", MediaMTXPath: "cameras/attic"}))
	require.NoError(t, d.CreateCamera(&Camera{Name: "Cellar", MediaMTXPath: "cameras/cellar"}))

	cameras, err := d.ListCameras()
	require.NoError(t, err)
	require.Len(t, cameras, 3)
	// Should be ordered by name.
	require.Equal(t, "Attic", cameras[0].Name)
	require.Equal(t, "Backyard", cameras[1].Name)
	require.Equal(t, "Cellar", cameras[2].Name)
}

func TestCameraUpdate(t *testing.T) {
	d := newTestDB(t)

	cam := &Camera{Name: "Lobby", Status: "connected"}
	require.NoError(t, d.CreateCamera(cam))

	cam.Name = "Main Lobby"
	cam.PTZCapable = true
	require.NoError(t, d.UpdateCamera(cam))

	got, err := d.GetCamera(cam.ID)
	require.NoError(t, err)
	require.Equal(t, "Main Lobby", got.Name)
	require.True(t, got.PTZCapable)
}

func TestCameraDelete(t *testing.T) {
	d := newTestDB(t)

	cam := &Camera{Name: "Driveway"}
	require.NoError(t, d.CreateCamera(cam))

	require.NoError(t, d.DeleteCamera(cam.ID))

	_, err := d.GetCamera(cam.ID)
	require.ErrorIs(t, err, ErrNotFound)

	// Deleting again should return ErrNotFound.
	require.ErrorIs(t, d.DeleteCamera(cam.ID), ErrNotFound)
}

func TestCameraGetByPath(t *testing.T) {
	d := newTestDB(t)

	cam := &Camera{Name: "Pool", MediaMTXPath: "cameras/pool"}
	require.NoError(t, d.CreateCamera(cam))

	got, err := d.GetCameraByPath("cameras/pool")
	require.NoError(t, err)
	require.Equal(t, cam.ID, got.ID)
	require.Equal(t, "Pool", got.Name)

	_, err = d.GetCameraByPath("cameras/nonexistent")
	require.ErrorIs(t, err, ErrNotFound)
}
