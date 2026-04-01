package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// ScheduleTemplateHandler implements HTTP endpoints for schedule template management.
type ScheduleTemplateHandler struct {
	DB *db.DB
}

// scheduleTemplateRequest is the JSON body for creating or updating a schedule template.
type scheduleTemplateRequest struct {
	Name             string `json:"name"`
	Mode             string `json:"mode"`
	Days             []int  `json:"days"`
	StartTime        string `json:"start_time"`
	EndTime          string `json:"end_time"`
	PostEventSeconds int    `json:"post_event_seconds"`
}

// List returns all schedule templates.
func (h *ScheduleTemplateHandler) List(c *gin.Context) {
	templates, err := h.DB.ListScheduleTemplates()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list schedule templates", err)
		return
	}
	if templates == nil {
		templates = []*db.ScheduleTemplate{}
	}
	c.JSON(http.StatusOK, templates)
}

// Create creates a new schedule template.
func (h *ScheduleTemplateHandler) Create(c *gin.Context) {
	var req scheduleTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.Mode != "always" && req.Mode != "events" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be 'always' or 'events'"})
		return
	}

	daysJSON, err := json.Marshal(req.Days)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid days"})
		return
	}

	tmpl := &db.ScheduleTemplate{
		Name:             req.Name,
		Mode:             req.Mode,
		Days:             string(daysJSON),
		StartTime:        req.StartTime,
		EndTime:          req.EndTime,
		PostEventSeconds: req.PostEventSeconds,
	}

	if err := h.DB.CreateScheduleTemplate(tmpl); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create schedule template", err)
		return
	}

	c.JSON(http.StatusCreated, tmpl)
}

// Update updates an existing schedule template.
func (h *ScheduleTemplateHandler) Update(c *gin.Context) {
	id := c.Param("id")

	existing, err := h.DB.GetScheduleTemplate(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "schedule template not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve schedule template", err)
		return
	}

	var req scheduleTemplateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.Mode != "always" && req.Mode != "events" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "mode must be 'always' or 'events'"})
		return
	}

	daysJSON, err := json.Marshal(req.Days)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid days"})
		return
	}

	existing.Name = req.Name
	existing.Mode = req.Mode
	existing.Days = string(daysJSON)
	existing.StartTime = req.StartTime
	existing.EndTime = req.EndTime
	existing.PostEventSeconds = req.PostEventSeconds

	if err := h.DB.UpdateScheduleTemplate(existing); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update schedule template", err)
		return
	}

	c.JSON(http.StatusOK, existing)
}

// Delete removes a schedule template.
func (h *ScheduleTemplateHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	tmpl, err := h.DB.GetScheduleTemplate(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "schedule template not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve schedule template", err)
		return
	}

	if tmpl.IsDefault {
		c.JSON(http.StatusBadRequest, gin.H{"error": "cannot delete a default schedule template"})
		return
	}

	count, err := h.DB.CountTemplateUsage(id)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to check template usage", err)
		return
	}
	if count > 0 {
		c.JSON(http.StatusConflict, gin.H{"error": fmt.Sprintf("template is in use by %d recording rule(s)", count)})
		return
	}

	if err := h.DB.DeleteScheduleTemplate(id); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete schedule template", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "schedule template deleted"})
}
