package api

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// WebhookHandler handles CRUD operations for webhook configurations.
type WebhookHandler struct {
	DB *db.DB
}

// CreateWebhookRequest is the JSON body for creating a webhook config.
type CreateWebhookRequest struct {
	Name           string  `json:"name" binding:"required"`
	URL            string  `json:"url" binding:"required"`
	Secret         string  `json:"secret"`
	CameraID       string  `json:"camera_id"`
	EventTypes     string  `json:"event_types"`
	ObjectClasses  string  `json:"object_classes"`
	MinConfidence  float64 `json:"min_confidence"`
	Enabled        *bool   `json:"enabled"`
	MaxRetries     int     `json:"max_retries"`
	TimeoutSeconds int     `json:"timeout_seconds"`
}

// UpdateWebhookRequest is the JSON body for updating a webhook config.
type UpdateWebhookRequest struct {
	Name           string   `json:"name"`
	URL            string   `json:"url"`
	Secret         *string  `json:"secret"`
	CameraID       *string  `json:"camera_id"`
	EventTypes     *string  `json:"event_types"`
	ObjectClasses  *string  `json:"object_classes"`
	MinConfidence  *float64 `json:"min_confidence"`
	Enabled        *bool    `json:"enabled"`
	MaxRetries     *int     `json:"max_retries"`
	TimeoutSeconds *int     `json:"timeout_seconds"`
}

// Create creates a new webhook configuration.
func (h *WebhookHandler) Create(c *gin.Context) {
	var req CreateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name and url are required"})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	if req.MaxRetries < 0 {
		req.MaxRetries = 0
	}
	if req.MaxRetries > 10 {
		req.MaxRetries = 10
	}
	if req.MaxRetries == 0 {
		req.MaxRetries = 3
	}

	if req.TimeoutSeconds <= 0 {
		req.TimeoutSeconds = 10
	}
	if req.TimeoutSeconds > 60 {
		req.TimeoutSeconds = 60
	}

	if req.EventTypes == "" {
		req.EventTypes = "detection"
	}

	wh := &db.WebhookConfig{
		ID:             uuid.New().String(),
		Name:           req.Name,
		URL:            req.URL,
		Secret:         req.Secret,
		CameraID:       req.CameraID,
		EventTypes:     req.EventTypes,
		ObjectClasses:  req.ObjectClasses,
		MinConfidence:  req.MinConfidence,
		Enabled:        enabled,
		MaxRetries:     req.MaxRetries,
		TimeoutSeconds: req.TimeoutSeconds,
	}

	if err := h.DB.InsertWebhookConfig(wh); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create webhook", err)
		return
	}

	c.JSON(http.StatusCreated, wh)
}

// List returns all webhook configurations.
func (h *WebhookHandler) List(c *gin.Context) {
	webhooks, err := h.DB.ListWebhookConfigs()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list webhooks", err)
		return
	}

	if webhooks == nil {
		webhooks = []*db.WebhookConfig{}
	}

	c.JSON(http.StatusOK, webhooks)
}

// Get returns a single webhook config by ID.
func (h *WebhookHandler) Get(c *gin.Context) {
	id := c.Param("id")

	wh, err := h.DB.GetWebhookConfig(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to get webhook", err)
		return
	}

	c.JSON(http.StatusOK, wh)
}

// Update modifies an existing webhook config.
func (h *WebhookHandler) Update(c *gin.Context) {
	id := c.Param("id")

	existing, err := h.DB.GetWebhookConfig(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to get webhook", err)
		return
	}

	var req UpdateWebhookRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.URL != "" {
		existing.URL = req.URL
	}
	if req.Secret != nil {
		existing.Secret = *req.Secret
	}
	if req.CameraID != nil {
		existing.CameraID = *req.CameraID
	}
	if req.EventTypes != nil {
		existing.EventTypes = *req.EventTypes
	}
	if req.ObjectClasses != nil {
		existing.ObjectClasses = *req.ObjectClasses
	}
	if req.MinConfidence != nil {
		existing.MinConfidence = *req.MinConfidence
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}
	if req.MaxRetries != nil {
		mr := *req.MaxRetries
		if mr < 0 {
			mr = 0
		}
		if mr > 10 {
			mr = 10
		}
		existing.MaxRetries = mr
	}
	if req.TimeoutSeconds != nil {
		ts := *req.TimeoutSeconds
		if ts <= 0 {
			ts = 10
		}
		if ts > 60 {
			ts = 60
		}
		existing.TimeoutSeconds = ts
	}

	if err := h.DB.UpdateWebhookConfig(existing); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update webhook", err)
		return
	}

	c.JSON(http.StatusOK, existing)
}

// Delete removes a webhook config by ID.
func (h *WebhookHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	if err := h.DB.DeleteWebhookConfig(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to delete webhook", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// Deliveries returns the delivery log for a specific webhook.
func (h *WebhookHandler) Deliveries(c *gin.Context) {
	id := c.Param("id")

	// Verify webhook exists.
	if _, err := h.DB.GetWebhookConfig(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "webhook not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to get webhook", err)
		return
	}

	limit := 100
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	deliveries, err := h.DB.ListWebhookDeliveries(id, limit)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list deliveries", err)
		return
	}

	if deliveries == nil {
		deliveries = []*db.WebhookDelivery{}
	}

	c.JSON(http.StatusOK, deliveries)
}
