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

func setupTicketRouter(t *testing.T) (*gin.Engine, *db.DB) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	database := setupTestDB(t)
	handler := &TicketHandler{DB: database}

	r := gin.New()
	g := r.Group("/api/nvr")
	g.POST("/tickets", handler.Create)
	g.GET("/tickets", handler.List)
	g.GET("/tickets/config/:integratorId", handler.GetHookConfig)
	g.PUT("/tickets/config/:integratorId", handler.UpsertHookConfig)

	return r, database
}

func TestUpsertAndGetHookConfig(t *testing.T) {
	r, _ := setupTicketRouter(t)

	cfg := `{
		"provider": "zendesk",
		"api_base_url": "https://test.zendesk.com",
		"auto_create": true,
		"tag_template": "kaivue,{{customer_id}}",
		"enabled": true
	}`

	// Upsert.
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/nvr/tickets/config/int-001", bytes.NewBufferString(cfg))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var saved TicketHookConfig
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &saved))
	assert.Equal(t, "zendesk", saved.Provider)
	assert.Equal(t, "int-001", saved.IntegratorID)
	assert.NotEmpty(t, saved.ConfigID)

	// Get.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/nvr/tickets/config/int-001", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var got TicketHookConfig
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &got))
	assert.Equal(t, "zendesk", got.Provider)
	assert.Empty(t, got.APIKey, "API key should never be returned")
}

func TestUpsertHookConfigInvalidProvider(t *testing.T) {
	r, _ := setupTicketRouter(t)

	cfg := `{"provider": "jira", "enabled": true}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/api/nvr/tickets/config/int-001", bytes.NewBufferString(cfg))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCreateTicket(t *testing.T) {
	r, database := setupTicketRouter(t)

	// First set up a hook config.
	hookCfg := TicketHookConfig{
		ConfigID:     "hc-001",
		IntegratorID: "int-001",
		Provider:     "zendesk",
		APIBaseURL:   "https://test.zendesk.com",
		Enabled:      true,
		TagTemplate:  "kaivue",
	}
	data, _ := json.Marshal(hookCfg)
	require.NoError(t, database.SetConfig("ticket_hook_int-001", string(data)))

	body := `{
		"integrator_id": "int-001",
		"subject": "Camera offline",
		"priority": "high",
		"context": {
			"customer_id": "cust-001",
			"customer_name": "Acme Corp",
			"recorder_id": "rec-001",
			"description": "Camera 3 is showing offline since 10am."
		}
	}`

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusCreated, w.Code)

	var ticket TicketRow
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &ticket))
	assert.Equal(t, "int-001", ticket.IntegratorID)
	assert.Equal(t, "cust-001", ticket.CustomerID)
	assert.Equal(t, "high", ticket.Priority)
	assert.Equal(t, "zendesk", ticket.Provider)
	assert.NotNil(t, ticket.URL)
	assert.Contains(t, *ticket.URL, "zendesk.com")
}

func TestCreateTicketNoConfig(t *testing.T) {
	r, _ := setupTicketRouter(t)

	body := `{
		"integrator_id": "int-no-config",
		"subject": "Test",
		"context": {
			"customer_id": "cust-001",
			"customer_name": "Test",
			"recorder_id": "rec-001"
		}
	}`

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusPreconditionFailed, w.Code)
}

func TestCreateTicketDisabledConfig(t *testing.T) {
	r, database := setupTicketRouter(t)

	hookCfg := TicketHookConfig{
		ConfigID:     "hc-002",
		IntegratorID: "int-002",
		Provider:     "freshdesk",
		APIBaseURL:   "https://test.freshdesk.com",
		Enabled:      false,
	}
	data, _ := json.Marshal(hookCfg)
	require.NoError(t, database.SetConfig("ticket_hook_int-002", string(data)))

	body := `{
		"integrator_id": "int-002",
		"subject": "Test",
		"context": {
			"customer_id": "cust-001",
			"customer_name": "Test",
			"recorder_id": "rec-001"
		}
	}`

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusPreconditionFailed, w.Code)
}

func TestListTickets(t *testing.T) {
	r, database := setupTicketRouter(t)

	// Set up config and create a ticket.
	hookCfg := TicketHookConfig{
		ConfigID:     "hc-001",
		IntegratorID: "int-001",
		Provider:     "internal",
		Enabled:      true,
	}
	data, _ := json.Marshal(hookCfg)
	require.NoError(t, database.SetConfig("ticket_hook_int-001", string(data)))

	body := `{
		"integrator_id": "int-001",
		"subject": "Test issue",
		"context": {
			"customer_id": "cust-001",
			"customer_name": "Acme",
			"recorder_id": "rec-001"
		}
	}`
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/nvr/tickets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	r.ServeHTTP(w, req)
	require.Equal(t, http.StatusCreated, w.Code)

	// List.
	w = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/api/nvr/tickets?integrator_id=int-001", nil)
	r.ServeHTTP(w, req)
	assert.Equal(t, http.StatusOK, w.Code)

	var listResp struct {
		Tickets []TicketRow `json:"tickets"`
		Total   int         `json:"total"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &listResp))
	assert.Equal(t, 1, listResp.Total)
}
