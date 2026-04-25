// Package detectionapi implements the recorder-side detection and AI API.
// It exposes routes for detection zones, detection events, and detection
// scheduling. This is a role-scoped extract of the monolithic
// internal/nvr/api layer (detection_zones.go, detection_events.go,
// detection_schedule.go).
package detectionapi

import (
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
)

// Handler is the detection API handler for the Recorder service.
type Handler struct {
	db *recdb.DB
}

// NewHandler creates a Handler backed by the given recorder DB.
func NewHandler(db *recdb.DB) *Handler {
	return &Handler{db: db}
}

// Register wires detection routes onto r.
// All routes are expected to be in a JWT-protected router group.
func (h *Handler) Register(r gin.IRouter) {
	// Detection zones.
	r.GET("/cameras/:id/detection-zones", h.ListZones)
	r.POST("/cameras/:id/detection-zones", h.CreateZone)
	r.PUT("/detection-zones/:zoneId", h.UpdateZone)
	r.DELETE("/detection-zones/:zoneId", h.DeleteZone)

	// Detection events.
	r.GET("/cameras/:id/detection-events", h.ListDetectionEvents)
	r.GET("/cameras/:id/detections", h.ListDetections)
}

// --- helpers ---------------------------------------------------------------

func apiError(c *gin.Context, status int, userMsg string, err error) {
	reqID := uuid.New().String()[:8]
	log.Printf("[detectionapi] [ERROR] [%s] %s: %v", reqID, userMsg, err)
	code := "internal_error"
	switch status {
	case http.StatusBadRequest:
		code = "bad_request"
	case http.StatusNotFound:
		code = "not_found"
	}
	c.JSON(status, gin.H{"error": userMsg, "code": code, "request_id": reqID})
}

// --- Detection zones -------------------------------------------------------

// ListZones returns all detection zones for a camera.
//
//	GET /cameras/:id/detection-zones
func (h *Handler) ListZones(c *gin.Context) {
	cameraID := c.Param("id")
	zones, err := h.db.ListDetectionZones(cameraID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list detection zones", err)
		return
	}
	if zones == nil {
		zones = []*recdb.DetectionZone{}
	}
	c.JSON(http.StatusOK, zones)
}

// detectionZoneRequest is the JSON body for create/update.
type detectionZoneRequest struct {
	Name        string                    `json:"name"`
	Points      []recdb.DetectionZonePoint `json:"points"`
	ClassFilter []string                  `json:"class_filter"`
	Enabled     *bool                     `json:"enabled"`
}

// CreateZone adds a new detection zone for a camera.
//
//	POST /cameras/:id/detection-zones
func (h *Handler) CreateZone(c *gin.Context) {
	cameraID := c.Param("id")

	var req detectionZoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}

	z := &recdb.DetectionZone{
		CameraID:    cameraID,
		Name:        req.Name,
		Points:      req.Points,
		ClassFilter: req.ClassFilter,
		Enabled:     enabled,
	}

	if err := h.db.CreateDetectionZone(z); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create detection zone", err)
		return
	}
	c.JSON(http.StatusCreated, z)
}

// UpdateZone updates an existing detection zone.
//
//	PUT /detection-zones/:zoneId
func (h *Handler) UpdateZone(c *gin.Context) {
	zoneID := c.Param("zoneId")

	existing, err := h.db.GetDetectionZone(zoneID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "detection zone not found"})
		return
	}

	var req detectionZoneRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if len(req.Points) > 0 {
		existing.Points = req.Points
	}
	if req.ClassFilter != nil {
		existing.ClassFilter = req.ClassFilter
	}
	if req.Enabled != nil {
		existing.Enabled = *req.Enabled
	}

	if err := h.db.UpdateDetectionZone(existing); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update detection zone", err)
		return
	}
	c.JSON(http.StatusOK, existing)
}

// DeleteZone removes a detection zone.
//
//	DELETE /detection-zones/:zoneId
func (h *Handler) DeleteZone(c *gin.Context) {
	zoneID := c.Param("zoneId")
	if err := h.db.DeleteDetectionZone(zoneID); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete detection zone", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// --- Detection events ------------------------------------------------------

// ListDetectionEvents returns aggregated detection events for a camera.
//
//	GET /cameras/:id/detection-events?class=<class>&start=<RFC3339>&end=<RFC3339>
func (h *Handler) ListDetectionEvents(c *gin.Context) {
	cameraID := c.Param("id")
	class := c.Query("class")
	start, _ := time.Parse(time.RFC3339, c.Query("start"))
	end, _ := time.Parse(time.RFC3339, c.Query("end"))
	if end.IsZero() {
		end = time.Now().UTC()
	}

	events, err := h.db.QueryDetectionEvents(cameraID, class, start, end)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query detection events", err)
		return
	}
	if events == nil {
		events = []*recdb.DetectionEvent{}
	}
	c.JSON(http.StatusOK, gin.H{"items": events})
}

// ListDetections returns detection event records matching the same query
// parameters as ListDetectionEvents. The route is kept separate because
// the NVR legacy API exposes both /detections and /detection-events.
//
//	GET /cameras/:id/detections?class=<class>&start=<RFC3339>&end=<RFC3339>
func (h *Handler) ListDetections(c *gin.Context) {
	h.ListDetectionEvents(c)
}
