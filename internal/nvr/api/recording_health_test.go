package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/recorder/scheduler"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"
)

type mockHealthProvider struct {
	health map[string]*scheduler.RecordingHealth
}

func (m *mockHealthProvider) GetAllRecordingHealth() map[string]*scheduler.RecordingHealth {
	return m.health
}

func (m *mockHealthProvider) GetRecordingHealth(cameraID string) *scheduler.RecordingHealth {
	return m.health[cameraID]
}

func setupTestDBForHealth(t *testing.T) *db.DB {
	t.Helper()
	tmpDir := t.TempDir()
	d, err := db.Open(tmpDir + "/test.db")
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })
	return d
}

func TestRecordingHealthHandler_List(t *testing.T) {
	gin.SetMode(gin.TestMode)

	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	mock := &mockHealthProvider{
		health: map[string]*scheduler.RecordingHealth{
			"cam-1": {
				Status:          scheduler.HealthHealthy,
				LastSegmentTime: now,
			},
			"cam-2": {
				Status:          scheduler.HealthStalled,
				LastSegmentTime: now.Add(-40 * time.Second),
				StallDetectedAt: now.Add(-10 * time.Second),
				RestartAttempts: 1,
				LastError:       "no segment received for 30s",
			},
		},
	}

	d := setupTestDBForHealth(t)

	require.NoError(t, d.CreateCamera(&db.Camera{ID: "cam-1", Name: "Front Door", RTSPURL: "rtsp://test1", MediaMTXPath: "cam1"}))
	require.NoError(t, d.CreateCamera(&db.Camera{ID: "cam-2", Name: "Garage", RTSPURL: "rtsp://test2", MediaMTXPath: "cam2"}))

	h := &RecordingHealthHandler{
		DB:             d,
		HealthProvider: mock,
	}

	router := gin.New()
	router.GET("/recordings/health", h.List)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/recordings/health", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Cameras []recordingHealthEntry `json:"cameras"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Len(t, resp.Cameras, 2)
}

func TestRecordingHealthHandler_ListFilterByCamera(t *testing.T) {
	gin.SetMode(gin.TestMode)

	mock := &mockHealthProvider{
		health: map[string]*scheduler.RecordingHealth{
			"cam-1": {Status: scheduler.HealthHealthy, LastSegmentTime: time.Now()},
			"cam-2": {Status: scheduler.HealthStalled, LastSegmentTime: time.Now()},
		},
	}

	d := setupTestDBForHealth(t)
	require.NoError(t, d.CreateCamera(&db.Camera{ID: "cam-1", Name: "Front Door", RTSPURL: "rtsp://test1", MediaMTXPath: "cam1"}))
	require.NoError(t, d.CreateCamera(&db.Camera{ID: "cam-2", Name: "Garage", RTSPURL: "rtsp://test2", MediaMTXPath: "cam2"}))

	h := &RecordingHealthHandler{DB: d, HealthProvider: mock}

	router := gin.New()
	router.GET("/recordings/health", h.List)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/recordings/health?camera_id=cam-1", nil)
	router.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Cameras []recordingHealthEntry `json:"cameras"`
	}
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Len(t, resp.Cameras, 1)
	require.Equal(t, "cam-1", resp.Cameras[0].CameraID)
}
