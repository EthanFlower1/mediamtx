package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/ai"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// DetectionZoneHandler provides CRUD endpoints for detection zones.
type DetectionZoneHandler struct {
	DB *db.DB
}

// detectionZoneRequest is the JSON body for create/update.
type detectionZoneRequest struct {
	Name        string           `json:"name"`
	Points      []ai.Point       `json:"points"`
	ClassFilter []string         `json:"class_filter"`
	Enabled     *bool            `json:"enabled"`
}

// List returns all detection zones for a camera.
//
//	GET /api/nvr/cameras/:id/detection-zones
func (h *DetectionZoneHandler) List(c *gin.Context) {
	cameraID := c.Param("id")
	zones, err := h.DB.ListDetectionZones(cameraID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list detection zones", err)
		return
	}
	if zones == nil {
		zones = []*db.DetectionZone{}
	}
	c.JSON(http.StatusOK, zones)
}

// Create adds a new detection zone for a camera.
//
//	POST /api/nvr/cameras/:id/detection-zones
func (h *DetectionZoneHandler) Create(c *gin.Context) {
	cameraID := c.Param("id")

	var req detectionZoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	// Build the AI zone for validation.
	aiZone := &ai.DetectionZone{
		CameraID:    cameraID,
		Name:        req.Name,
		Points:      req.Points,
		ClassFilter: req.ClassFilter,
		Enabled:     enabled,
	}
	if err := ai.ValidateZone(aiZone); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Convert to DB model.
	dbPoints := make([]db.DetectionZonePoint, len(req.Points))
	for i, p := range req.Points {
		dbPoints[i] = db.DetectionZonePoint{X: p.X, Y: p.Y}
	}
	zone := &db.DetectionZone{
		CameraID:    cameraID,
		Name:        req.Name,
		Points:      dbPoints,
		ClassFilter: req.ClassFilter,
		Enabled:     enabled,
	}
	if zone.ClassFilter == nil {
		zone.ClassFilter = []string{}
	}

	if err := h.DB.CreateDetectionZone(zone); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create detection zone", err)
		return
	}
	c.JSON(http.StatusCreated, zone)
}

// Update modifies an existing detection zone.
//
//	PUT /api/nvr/detection-zones/:zoneId
func (h *DetectionZoneHandler) Update(c *gin.Context) {
	zoneID := c.Param("zoneId")

	existing, err := h.DB.GetDetectionZone(zoneID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "detection zone not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get detection zone", err)
		return
	}

	var req detectionZoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}

	enabled := existing.Enabled
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	aiZone := &ai.DetectionZone{
		CameraID:    existing.CameraID,
		Name:        req.Name,
		Points:      req.Points,
		ClassFilter: req.ClassFilter,
		Enabled:     enabled,
	}
	if err := ai.ValidateZone(aiZone); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	dbPoints := make([]db.DetectionZonePoint, len(req.Points))
	for i, p := range req.Points {
		dbPoints[i] = db.DetectionZonePoint{X: p.X, Y: p.Y}
	}
	existing.Name = req.Name
	existing.Points = dbPoints
	existing.ClassFilter = req.ClassFilter
	existing.Enabled = enabled
	if existing.ClassFilter == nil {
		existing.ClassFilter = []string{}
	}

	if err := h.DB.UpdateDetectionZone(existing); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update detection zone", err)
		return
	}
	c.JSON(http.StatusOK, existing)
}

// Delete removes a detection zone.
//
//	DELETE /api/nvr/detection-zones/:zoneId
func (h *DetectionZoneHandler) Delete(c *gin.Context) {
	zoneID := c.Param("zoneId")
	err := h.DB.DeleteDetectionZone(zoneID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "detection zone not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete detection zone", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
