package api

// KAI-469: Ticket integration hooks API.
//
// Provides endpoints for:
//   - Creating support tickets from customer/recorder context
//   - Configuring per-integrator ticket integration (Zendesk, Freshdesk, internal)
//   - Listing tickets linked to screen-share sessions

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// TicketHandler implements HTTP endpoints for the ticket integration.
type TicketHandler struct {
	DB    *db.DB
	Audit *AuditLogger
}

// TicketHookConfig holds per-integrator ticket integration settings.
type TicketHookConfig struct {
	ConfigID     string `json:"config_id"`
	IntegratorID string `json:"integrator_id"`
	Provider     string `json:"provider"`     // "zendesk" | "freshdesk" | "internal"
	APIBaseURL   string `json:"api_base_url"`
	APIKey       string `json:"api_key,omitempty"` // Write-only; never returned in GET.
	AutoCreate   bool   `json:"auto_create"`
	TagTemplate  string `json:"tag_template"`
	Enabled      bool   `json:"enabled"`
}

// TicketRow is the JSON/DB shape for a created ticket.
type TicketRow struct {
	TicketID     string  `json:"ticket_id"`
	ExternalID   *string `json:"external_id,omitempty"`
	IntegratorID string  `json:"integrator_id"`
	CustomerID   string  `json:"customer_id"`
	CustomerName string  `json:"customer_name"`
	RecorderID   string  `json:"recorder_id"`
	SessionID    *string `json:"session_id,omitempty"`
	CameraPath   *string `json:"camera_path,omitempty"`
	Subject      string  `json:"subject"`
	Description  string  `json:"description"`
	Priority     string  `json:"priority"` // "low" | "normal" | "high" | "urgent"
	Provider     string  `json:"provider"`
	URL          *string `json:"url,omitempty"`
	CreatedAt    string  `json:"created_at_iso"`
}

type createTicketRequest struct {
	IntegratorID string `json:"integrator_id" binding:"required"`
	Subject      string `json:"subject"       binding:"required"`
	Priority     string `json:"priority"`
	Context      struct {
		CustomerID   string  `json:"customer_id"   binding:"required"`
		CustomerName string  `json:"customer_name"`
		RecorderID   string  `json:"recorder_id"   binding:"required"`
		SessionID    *string `json:"session_id"`
		CameraPath   *string `json:"camera_path"`
		Description  string  `json:"description"`
	} `json:"context" binding:"required"`
}

// Create creates a support ticket from customer context and optionally links
// it to a screen-share session.
//
//	POST /api/nvr/tickets
func (h *TicketHandler) Create(c *gin.Context) {
	var req createTicketRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	priority := req.Priority
	if priority == "" {
		priority = "normal"
	}
	validPriorities := map[string]bool{"low": true, "normal": true, "high": true, "urgent": true}
	if !validPriorities[priority] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "priority must be low, normal, high, or urgent"})
		return
	}

	// Look up the integrator's hook config.
	configVal, err := h.DB.GetConfig("ticket_hook_" + req.IntegratorID)
	if err != nil {
		c.JSON(http.StatusPreconditionFailed, gin.H{
			"error": "no ticket integration configured for this integrator",
		})
		return
	}

	var hookCfg TicketHookConfig
	if err := json.Unmarshal([]byte(configVal), &hookCfg); err != nil {
		apiError(c, http.StatusInternalServerError, "corrupt ticket hook config", err)
		return
	}
	if !hookCfg.Enabled {
		c.JSON(http.StatusPreconditionFailed, gin.H{"error": "ticket integration is disabled"})
		return
	}

	ticketID := uuid.New().String()
	now := time.Now().UTC().Format(time.RFC3339)

	var url *string
	switch hookCfg.Provider {
	case "zendesk":
		u := hookCfg.APIBaseURL + "/agent/tickets/" + ticketID
		url = &u
	case "freshdesk":
		u := hookCfg.APIBaseURL + "/a/tickets/" + ticketID
		url = &u
	case "internal":
		u := "/command/support/tickets/" + ticketID
		url = &u
	}

	row := TicketRow{
		TicketID:     ticketID,
		IntegratorID: req.IntegratorID,
		CustomerID:   req.Context.CustomerID,
		CustomerName: req.Context.CustomerName,
		RecorderID:   req.Context.RecorderID,
		SessionID:    req.Context.SessionID,
		CameraPath:   req.Context.CameraPath,
		Subject:      req.Subject,
		Description:  req.Context.Description,
		Priority:     priority,
		Provider:     hookCfg.Provider,
		URL:          url,
		CreatedAt:    now,
	}

	data, _ := json.Marshal(row)
	if err := h.DB.SetConfig("ticket_item_"+ticketID, string(data)); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create ticket", err)
		return
	}

	// Link ticket to screen-share session if provided.
	if req.Context.SessionID != nil && *req.Context.SessionID != "" {
		sessVal, err := h.DB.GetConfig("screenshare_session_" + *req.Context.SessionID)
		if err == nil {
			var sess ScreenShareSessionRow
			if err := json.Unmarshal([]byte(sessVal), &sess); err == nil {
				sess.LinkedTicketID = &ticketID
				sessData, _ := json.Marshal(sess)
				_ = h.DB.SetConfig("screenshare_session_"+*req.Context.SessionID, string(sessData))
			}
		}
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "ticket_create", "ticket", ticketID,
			"Created ticket for customer "+req.Context.CustomerID+" via "+hookCfg.Provider)
	}

	c.JSON(http.StatusCreated, row)
}

// List returns tickets for an integrator, optionally filtered by customer.
//
//	GET /api/nvr/tickets?integrator_id=...&customer_id=...
func (h *TicketHandler) List(c *gin.Context) {
	integratorID := c.Query("integrator_id")
	customerID := c.Query("customer_id")

	if integratorID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "integrator_id is required"})
		return
	}

	tickets := []TicketRow{}
	allConfigs, err := h.DB.ListConfigByPrefix("ticket_item_")
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"tickets": tickets, "total": 0})
		return
	}

	for _, val := range allConfigs {
		var row TicketRow
		if err := json.Unmarshal([]byte(val), &row); err != nil {
			continue
		}
		if row.IntegratorID != integratorID {
			continue
		}
		if customerID != "" && row.CustomerID != customerID {
			continue
		}
		tickets = append(tickets, row)
	}

	c.JSON(http.StatusOK, gin.H{
		"tickets": tickets,
		"total":   len(tickets),
	})
}

// GetHookConfig returns the ticket hook configuration for an integrator.
//
//	GET /api/nvr/tickets/config/:integratorId
func (h *TicketHandler) GetHookConfig(c *gin.Context) {
	integratorID := c.Param("integratorId")

	val, err := h.DB.GetConfig("ticket_hook_" + integratorID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "no ticket configuration found"})
		return
	}

	var cfg TicketHookConfig
	if err := json.Unmarshal([]byte(val), &cfg); err != nil {
		apiError(c, http.StatusInternalServerError, "corrupt ticket config", err)
		return
	}

	// Never return the API key.
	cfg.APIKey = ""

	c.JSON(http.StatusOK, cfg)
}

// UpsertHookConfig creates or updates the ticket hook configuration for an integrator.
//
//	PUT /api/nvr/tickets/config/:integratorId
func (h *TicketHandler) UpsertHookConfig(c *gin.Context) {
	integratorID := c.Param("integratorId")

	var cfg TicketHookConfig
	if err := c.ShouldBindJSON(&cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	cfg.IntegratorID = integratorID
	if cfg.ConfigID == "" {
		cfg.ConfigID = uuid.New().String()
	}

	validProviders := map[string]bool{"zendesk": true, "freshdesk": true, "internal": true}
	if !validProviders[cfg.Provider] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider must be zendesk, freshdesk, or internal"})
		return
	}

	data, _ := json.Marshal(cfg)
	if err := h.DB.SetConfig("ticket_hook_"+integratorID, string(data)); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to save ticket config", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "ticket_config_update", "ticket_hook_config", cfg.ConfigID,
			"Updated ticket hook config for integrator "+integratorID+" provider="+cfg.Provider)
	}

	// Don't return API key.
	cfg.APIKey = ""
	c.JSON(http.StatusOK, cfg)
}
