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

func TestHealthCheck_Healthy(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })

	handler := &HealthHandler{
		DB:             d,
		RecordingsPath: t.TempDir(), // writable temp dir with free space
	}

	engine := gin.New()
	engine.GET("/health", handler.Check)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp HealthResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, "healthy", resp.Status)
	require.Contains(t, resp.Components, "db")
	require.Contains(t, resp.Components, "recording")
	require.Contains(t, resp.Components, "storage")
	require.Contains(t, resp.Components, "onvif")
	require.Equal(t, "healthy", resp.Components["db"].Status)
	require.Equal(t, "healthy", resp.Components["storage"].Status)
	require.Less(t, resp.DurationMs, 100.0, "health check must complete under 100ms")
}

func TestHealthCheck_NoDB(t *testing.T) {
	gin.SetMode(gin.TestMode)

	handler := &HealthHandler{
		RecordingsPath: t.TempDir(),
	}

	engine := gin.New()
	engine.GET("/health", handler.Check)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp HealthResponse
	err := json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, "unhealthy", resp.Status)
	require.Equal(t, "unhealthy", resp.Components["db"].Status)
}

func TestHealthCheck_BadStoragePath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })

	handler := &HealthHandler{
		DB:             d,
		RecordingsPath: "/nonexistent/path/that/should/not/exist",
	}

	engine := gin.New()
	engine.GET("/health", handler.Check)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusServiceUnavailable, w.Code)

	var resp HealthResponse
	err = json.Unmarshal(w.Body.Bytes(), &resp)
	require.NoError(t, err)
	require.Equal(t, "unhealthy", resp.Status)
	require.Equal(t, "unhealthy", resp.Components["storage"].Status)
}

func TestHealthCheck_ResponseFormat(t *testing.T) {
	gin.SetMode(gin.TestMode)
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { d.Close() })

	handler := &HealthHandler{
		DB:             d,
		RecordingsPath: t.TempDir(),
	}

	engine := gin.New()
	engine.GET("/health", handler.Check)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	engine.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)
	require.Contains(t, w.Header().Get("Content-Type"), "application/json")

	var raw map[string]interface{}
	err = json.Unmarshal(w.Body.Bytes(), &raw)
	require.NoError(t, err)
	require.Contains(t, raw, "status")
	require.Contains(t, raw, "timestamp")
	require.Contains(t, raw, "duration_ms")
	require.Contains(t, raw, "components")
}
