package api

import (
	"net/http"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/recorder/scheduler"
	"github.com/gin-gonic/gin"
)

// HealthProvider abstracts the scheduler's recording health methods so the
// handler can be tested without a full scheduler.
type HealthProvider interface {
	GetAllRecordingHealth() map[string]*scheduler.RecordingHealth
	GetRecordingHealth(cameraID string) *scheduler.RecordingHealth
}

// RecordingHealthHandler serves recording health status.
type RecordingHealthHandler struct {
	DB             *db.DB
	HealthProvider HealthProvider
}

type recordingHealthEntry struct {
	CameraID        string  `json:"camera_id"`
	CameraName      string  `json:"camera_name"`
	Status          string  `json:"status"`
	LastSegmentTime *string `json:"last_segment_time"`
	StallDetectedAt *string `json:"stall_detected_at,omitempty"`
	RestartAttempts int     `json:"restart_attempts"`
	LastError       string  `json:"last_error,omitempty"`
}

// List returns recording health for all cameras (or a single camera if
// ?camera_id is provided).
func (h *RecordingHealthHandler) List(c *gin.Context) {
	filterID := c.Query("camera_id")
	allHealth := h.HealthProvider.GetAllRecordingHealth()

	cameras, err := h.DB.ListCameras()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list cameras", err)
		return
	}

	nameByID := make(map[string]string, len(cameras))
	for _, cam := range cameras {
		nameByID[cam.ID] = cam.Name
	}

	entries := make([]recordingHealthEntry, 0, len(allHealth))
	for camID, rh := range allHealth {
		if filterID != "" && camID != filterID {
			continue
		}
		entry := recordingHealthEntry{
			CameraID:        camID,
			CameraName:      nameByID[camID],
			Status:          rh.Status,
			RestartAttempts: rh.RestartAttempts,
			LastError:       rh.LastError,
		}
		if !rh.LastSegmentTime.IsZero() {
			t := rh.LastSegmentTime.UTC().Format("2006-01-02T15:04:05Z")
			entry.LastSegmentTime = &t
		}
		if !rh.StallDetectedAt.IsZero() {
			t := rh.StallDetectedAt.UTC().Format("2006-01-02T15:04:05Z")
			entry.StallDetectedAt = &t
		}
		entries = append(entries, entry)
	}

	c.JSON(http.StatusOK, gin.H{"cameras": entries})
}
