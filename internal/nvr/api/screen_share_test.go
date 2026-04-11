package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func setupScreenShareRouter(t *testing.T) (*gin.Engine, *db.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	database := setupTestDB(t)
	handler := &ScreenShareHandler{DB: database}

	r := gin.New()
	g := r.Group("/api/nvr")
	g.POST("/screen-share/sessions", handler.Initiate)
	g.GET("/screen-share/sessions", handler.List)
	g.GET("/screen-share/sessions/:id", handler.Get)
	g.POST("/screen-share/sessions/:id/end", handler.End)

	return r, database
}

func TestScreenShareInitiate(t *testing.T) {
	r, _ := setupScreenShareRouter(t)

	body := `{
		"integrator_id": "int-001",
		"customer_id": "cust-001",
		"recorder_id": "rec-001",
		"transport": "webrtc"
	}`

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/screen-share/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp ScreenShareSessionRow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "int-001", resp.IntegratorID)
	assert.Equal(t, "cust-001", resp.CustomerID)
	assert.Equal(t, "rec-001", resp.RecorderID)
	assert.Equal(t, "webrtc", resp.Transport)
	assert.Equal(t, "pending", resp.Status)
	assert.NotEmpty(t, resp.SessionID)
	assert.Contains(t, resp.SignallingURL, resp.SessionID)
}

func TestScreenShareInitiateDefaultTransport(t *testing.T) {
	r, _ := setupScreenShareRouter(t)

	body := `{
		"integrator_id": "int-001",
		"customer_id": "cust-001",
		"recorder_id": "rec-001"
	}`

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/screen-share/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusCreated, w.Code)

	var resp ScreenShareSessionRow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &resp))
	assert.Equal(t, "webrtc", resp.Transport)
}

func TestScreenShareInitiateInvalidTransport(t *testing.T) {
	r, _ := setupScreenShareRouter(t)

	body := `{
		"integrator_id": "int-001",
		"customer_id": "cust-001",
		"recorder_id": "rec-001",
		"transport": "invalid"
	}`

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/screen-share/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestScreenShareListAndEnd(t *testing.T) {
	r, _ := setupScreenShareRouter(t)

	// Create a session first.
	body := `{
		"integrator_id": "int-001",
		"customer_id": "cust-001",
		"recorder_id": "rec-001",
		"transport": "webrtc"
	}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/screen-share/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var created ScreenShareSessionRow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	// List sessions.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/nvr/screen-share/sessions?integrator_id=int-001", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResp struct {
		Sessions []ScreenShareSessionRow `json:"sessions"`
		Total    int                     `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	assert.Equal(t, 1, listResp.Total)
	assert.Equal(t, created.SessionID, listResp.Sessions[0].SessionID)

	// End the session.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/nvr/screen-share/sessions/"+created.SessionID+"/end", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var ended ScreenShareSessionRow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ended))
	assert.Equal(t, "completed", ended.Status)

	// End again should conflict.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPost, "/api/nvr/screen-share/sessions/"+created.SessionID+"/end", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestScreenShareGet(t *testing.T) {
	r, _ := setupScreenShareRouter(t)

	// Create a session.
	body := `{
		"integrator_id": "int-001",
		"customer_id": "cust-001",
		"recorder_id": "rec-001"
	}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/screen-share/sessions", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	var created ScreenShareSessionRow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &created))

	// Get by ID.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/nvr/screen-share/sessions/"+created.SessionID, nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	// Not found.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/nvr/screen-share/sessions/nonexistent", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusNotFound, w.Code)
}
