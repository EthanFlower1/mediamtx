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

	_, err := database.GetDevice(dev.ID)
	assert.ErrorIs(t, err, db.ErrNotFound)

	_, err = database.GetCamera(cam.ID)
	assert.ErrorIs(t, err, db.ErrNotFound)
}

func intPtr(i int) *int {
	return &i
}
