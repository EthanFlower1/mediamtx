package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func setupStatsTest(t *testing.T) (*db.DB, *gin.Engine) {
	t.Helper()
	gin.SetMode(gin.TestMode)
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })

	engine := gin.New()
	handler := &StatsHandler{DB: d}

	// Simulate authenticated user with wildcard camera permissions.
	engine.Use(func(c *gin.Context) {
		c.Set("camera_permissions", "*")
		c.Next()
	})
	engine.GET("/api/nvr/recordings/stats", handler.GetStats)
	engine.GET("/api/nvr/recordings/stats/:camera_id/gaps", handler.GetGaps)
	return d, engine
}

func TestGetStats_Empty(t *testing.T) {
	_, engine := setupStatsTest(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/nvr/recordings/stats", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Cameras []json.RawMessage `json:"cameras"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Empty(t, resp.Cameras)
}

func TestGetStats_WithRecordings(t *testing.T) {
	d, engine := setupStatsTest(t)

	cam := &db.Camera{Name: "TestCam", RTSPURL: "rtsp://test/stream"}
	require.NoError(t, d.CreateCamera(cam))

	require.NoError(t, d.InsertRecording(&db.Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T10:00:00.000Z",
		EndTime: "2025-01-15T11:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/t1.mp4", FileSize: 500000,
	}))
	require.NoError(t, d.InsertRecording(&db.Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T11:05:00.000Z",
		EndTime: "2025-01-15T12:00:00.000Z", DurationMs: 3300000,
		FilePath: "/recordings/t2.mp4", FileSize: 400000,
	}))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/nvr/recordings/stats", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Cameras []struct {
			CameraID        string `json:"camera_id"`
			CameraName      string `json:"camera_name"`
			TotalBytes      int64  `json:"total_bytes"`
			SegmentCount    int64  `json:"segment_count"`
			TotalRecordedMs int64  `json:"total_recorded_ms"`
			CurrentUptimeMs int64  `json:"current_uptime_ms"`
			GapCount        int    `json:"gap_count"`
		} `json:"cameras"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Len(t, resp.Cameras, 1)

	s := resp.Cameras[0]
	require.Equal(t, cam.ID, s.CameraID)
	require.Equal(t, int64(900000), s.TotalBytes)
	require.Equal(t, int64(2), s.SegmentCount)
	require.Equal(t, int64(6900000), s.TotalRecordedMs)
	require.Equal(t, 1, s.GapCount)
	// Current uptime = newest recording end - last gap end = 12:00 - 11:05 = 55 min = 3300000ms
	require.Equal(t, int64(3300000), s.CurrentUptimeMs)
}

func TestGetGaps_WithGap(t *testing.T) {
	d, engine := setupStatsTest(t)

	cam := &db.Camera{Name: "GapCam", RTSPURL: "rtsp://test/stream"}
	require.NoError(t, d.CreateCamera(cam))

	require.NoError(t, d.InsertRecording(&db.Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T10:00:00.000Z",
		EndTime: "2025-01-15T11:00:00.000Z", DurationMs: 3600000,
		FilePath: "/recordings/gap1.mp4", FileSize: 100,
	}))
	require.NoError(t, d.InsertRecording(&db.Recording{
		CameraID: cam.ID, StartTime: "2025-01-15T11:05:00.000Z",
		EndTime: "2025-01-15T12:00:00.000Z", DurationMs: 3300000,
		FilePath: "/recordings/gap2.mp4", FileSize: 100,
	}))

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/nvr/recordings/stats/"+cam.ID+"/gaps", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		CameraID string   `json:"camera_id"`
		Gaps     []db.Gap `json:"gaps"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Equal(t, cam.ID, resp.CameraID)
	require.Len(t, resp.Gaps, 1)
	require.Equal(t, "2025-01-15T11:00:00.000Z", resp.Gaps[0].Start)
	require.Equal(t, "2025-01-15T11:05:00.000Z", resp.Gaps[0].End)
	require.Equal(t, int64(300000), resp.Gaps[0].DurationMs)
}

func TestGetGaps_NotFound(t *testing.T) {
	_, engine := setupStatsTest(t)

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/api/nvr/recordings/stats/nonexistent/gaps", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp struct {
		Gaps []db.Gap `json:"gaps"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	require.Empty(t, resp.Gaps)
}
