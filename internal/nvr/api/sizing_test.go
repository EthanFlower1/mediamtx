package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSizing_Defaults(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &SystemHandler{}
	r.GET("/system/sizing", h.Sizing)

	req := httptest.NewRequest("GET", "/system/sizing", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp sizingResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	// Defaults: 1 camera, 1080p, 15 fps, 30 days, no AI.
	assert.Equal(t, 1, resp.Input.Cameras)
	assert.Equal(t, "1080p", resp.Input.Resolution)
	assert.Equal(t, 15, resp.Input.FPS)
	assert.Equal(t, 30, resp.Input.RetentionDays)
	assert.False(t, resp.Input.AI)
	assert.Equal(t, "small", resp.Tier)
	assert.Greater(t, resp.CPU.Cores, 0)
	assert.Greater(t, resp.RAM.GB, 0)
	assert.Greater(t, resp.Storage.TotalGB, 0.0)
	assert.Greater(t, resp.Bandwidth.IngressMbps, 0.0)
}

func TestSizing_16Cameras1080p(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &SystemHandler{}
	r.GET("/system/sizing", h.Sizing)

	req := httptest.NewRequest("GET", "/system/sizing?cameras=16&resolution=1080p&fps=15&retention_days=30&ai=true", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp sizingResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, 16, resp.Input.Cameras)
	assert.Equal(t, "1080p", resp.Input.Resolution)
	assert.Equal(t, 15, resp.Input.FPS)
	assert.Equal(t, 30, resp.Input.RetentionDays)
	assert.True(t, resp.Input.AI)
	assert.Equal(t, "medium", resp.Tier)

	// 16 cameras at 1080p/15fps = 2 Mbps each = 32 Mbps total ingress.
	assert.InDelta(t, 32.0, resp.Bandwidth.IngressMbps, 0.1)

	// Storage: 2 Mbps = 0.25 MB/s per camera.
	// Per day: 0.25 * 86400 = 21600 MB = ~21.09 GB.
	// 30 days * 16 cameras = ~10125 GB = ~9.88 TB.
	assert.Greater(t, resp.Storage.TotalTB, 5.0)
	assert.Less(t, resp.Storage.TotalTB, 15.0)
}

func TestSizing_UnsupportedResolution(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &SystemHandler{}
	r.GET("/system/sizing", h.Sizing)

	req := httptest.NewRequest("GET", "/system/sizing?resolution=8k", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)

	var body map[string]interface{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
	assert.Contains(t, body["error"], "unsupported resolution")
}

func TestSizing_4KHighRetention(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &SystemHandler{}
	r.GET("/system/sizing", h.Sizing)

	req := httptest.NewRequest("GET", "/system/sizing?cameras=4&resolution=4k&fps=30&retention_days=90", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusOK, w.Code)

	var resp sizingResponse
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))

	assert.Equal(t, 4, resp.Input.Cameras)
	assert.Equal(t, "4K", resp.Input.Resolution)
	assert.Equal(t, 30, resp.Input.FPS)
	assert.Equal(t, 90, resp.Input.RetentionDays)

	// 4 cameras at 4K/30fps = 16 Mbps each = 64 Mbps.
	assert.InDelta(t, 64.0, resp.Bandwidth.IngressMbps, 0.1)

	// Storage should be significant for 4K 90 days.
	assert.Greater(t, resp.Storage.TotalTB, 10.0)
}

func TestSizing_TierClassification(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &SystemHandler{}
	r.GET("/system/sizing", h.Sizing)

	tests := []struct {
		cameras  string
		expected string
	}{
		{"1", "small"},
		{"8", "small"},
		{"9", "medium"},
		{"32", "medium"},
		{"33", "large"},
		{"64", "large"},
		{"65", "enterprise"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest("GET", "/system/sizing?cameras="+tt.cameras, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		require.Equal(t, http.StatusOK, w.Code)
		var resp sizingResponse
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
		assert.Equal(t, tt.expected, resp.Tier, "cameras=%s should be tier %s", tt.cameras, tt.expected)
	}
}

func TestSizing_AIIncreasesResources(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := &SystemHandler{}
	r.GET("/system/sizing", h.Sizing)

	// Without AI.
	req1 := httptest.NewRequest("GET", "/system/sizing?cameras=16&ai=false", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)

	// With AI.
	req2 := httptest.NewRequest("GET", "/system/sizing?cameras=16&ai=true", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)

	var noAI, withAI sizingResponse
	require.NoError(t, json.Unmarshal(w1.Body.Bytes(), &noAI))
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &withAI))

	assert.Greater(t, withAI.CPU.Cores, noAI.CPU.Cores)
	assert.Greater(t, withAI.RAM.GB, noAI.RAM.GB)
	// Storage and bandwidth should be the same.
	assert.Equal(t, noAI.Storage.TotalGB, withAI.Storage.TotalGB)
	assert.Equal(t, noAI.Bandwidth.IngressMbps, withAI.Bandwidth.IngressMbps)
}
