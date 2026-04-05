package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/scheduler"
)

// DetectionScheduleHandler implements HTTP endpoints for per-camera detection
// scheduling.
type DetectionScheduleHandler struct {
	DB        *db.DB
	Evaluator *scheduler.DetectionEvaluator
}

// detectionScheduleEntry is the JSON representation of a single schedule entry.
type detectionScheduleEntry struct {
	DayOfWeek int    `json:"day_of_week"`
	StartTime string `json:"start_time"`
	EndTime   string `json:"end_time"`
	Enabled   bool   `json:"enabled"`
}

// detectionScheduleRequest is the JSON body for PUT /cameras/:id/detection-schedule.
type detectionScheduleRequest struct {
	Entries []detectionScheduleEntry `json:"entries"`
}

// detectionScheduleResponse wraps the schedule with status info.
type detectionScheduleResponse struct {
	CameraID        string                   `json:"camera_id"`
	Entries         []*db.DetectionSchedule   `json:"entries"`
	DetectionActive bool                     `json:"detection_active"`
	ActiveScheduleID string                  `json:"active_schedule_id,omitempty"`
}

// Get returns the detection schedule for a camera.
// GET /cameras/:id/detection-schedule
func (h *DetectionScheduleHandler) Get(c *gin.Context) {
	cameraID := c.Param("id")

	// Verify camera exists.
	if _, err := h.DB.GetCamera(cameraID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to get camera", err)
		return
	}

	schedules, err := h.DB.ListDetectionSchedules(cameraID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list detection schedules", err)
		return
	}
	if schedules == nil {
		schedules = []*db.DetectionSchedule{}
	}

	resp := detectionScheduleResponse{
		CameraID: cameraID,
		Entries:  schedules,
	}

	if h.Evaluator != nil {
		status := h.Evaluator.CameraStatus(cameraID)
		resp.DetectionActive = status.DetectionActive
		resp.ActiveScheduleID = status.ActiveScheduleID
	}

	c.JSON(http.StatusOK, resp)
}

// Update replaces the detection schedule for a camera.
// PUT /cameras/:id/detection-schedule
func (h *DetectionScheduleHandler) Update(c *gin.Context) {
	cameraID := c.Param("id")

	// Verify camera exists.
	if _, err := h.DB.GetCamera(cameraID); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to get camera", err)
		return
	}

	var req detectionScheduleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Validate entries.
	for _, e := range req.Entries {
		if e.DayOfWeek < 0 || e.DayOfWeek > 6 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "day_of_week must be 0-6"})
			return
		}
		if !validHHMM(e.StartTime) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start_time format, expected HH:MM"})
			return
		}
		if !validHHMM(e.EndTime) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end_time format, expected HH:MM"})
			return
		}
	}

	// Convert to DB objects.
	dbSchedules := make([]*db.DetectionSchedule, len(req.Entries))
	for i, e := range req.Entries {
		dbSchedules[i] = &db.DetectionSchedule{
			DayOfWeek: e.DayOfWeek,
			StartTime: e.StartTime,
			EndTime:   e.EndTime,
			Enabled:   e.Enabled,
		}
	}

	if err := h.DB.ReplaceDetectionSchedules(cameraID, dbSchedules); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update detection schedules", err)
		return
	}

	// Trigger immediate re-evaluation so the pipeline starts/stops right away.
	if h.Evaluator != nil {
		h.Evaluator.Evaluate()
	}

	// Return updated schedule.
	schedules, err := h.DB.ListDetectionSchedules(cameraID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to read updated schedules", err)
		return
	}
	if schedules == nil {
		schedules = []*db.DetectionSchedule{}
	}

	resp := detectionScheduleResponse{
		CameraID: cameraID,
		Entries:  schedules,
	}

	if h.Evaluator != nil {
		status := h.Evaluator.CameraStatus(cameraID)
		resp.DetectionActive = status.DetectionActive
		resp.ActiveScheduleID = status.ActiveScheduleID
	}

	c.JSON(http.StatusOK, resp)
}

// Templates returns the built-in detection schedule templates.
// GET /detection-schedule/templates
func (h *DetectionScheduleHandler) Templates(c *gin.Context) {
	c.JSON(http.StatusOK, scheduler.DetectionScheduleTemplates())
}

// Status returns detection schedule status for all cameras.
// GET /detection-schedule/status
func (h *DetectionScheduleHandler) Status(c *gin.Context) {
	if h.Evaluator == nil {
		c.JSON(http.StatusOK, map[string]*scheduler.DetectionScheduleStatus{})
		return
	}
	c.JSON(http.StatusOK, h.Evaluator.Status())
}

// validHHMM checks that s is in "HH:MM" format with valid ranges.
func validHHMM(s string) bool {
	if len(s) != 5 || s[2] != ':' {
		return false
	}
	h := (int(s[0]-'0') * 10) + int(s[1]-'0')
	m := (int(s[3]-'0') * 10) + int(s[4]-'0')
	return h >= 0 && h <= 23 && m >= 0 && m <= 59
}
