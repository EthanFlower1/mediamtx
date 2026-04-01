package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

const defaultGapThresholdMs = 2000

// StatsHandler implements HTTP endpoints for recording statistics.
type StatsHandler struct {
	DB *db.DB
}

// cameraStatsResponse is the per-camera entry in the stats response.
type cameraStatsResponse struct {
	CameraID        string  `json:"camera_id"`
	CameraName      string  `json:"camera_name"`
	TotalBytes      int64   `json:"total_bytes"`
	SegmentCount    int64   `json:"segment_count"`
	TotalRecordedMs int64   `json:"total_recorded_ms"`
	CurrentUptimeMs int64   `json:"current_uptime_ms"`
	LastGapEnd      *string `json:"last_gap_end"`
	OldestRecording string  `json:"oldest_recording"`
	NewestRecording string  `json:"newest_recording"`
	GapCount        int     `json:"gap_count"`
}

// GetStats returns aggregate recording statistics per camera.
// Optional query param: camera_id to filter to a single camera.
func (h *StatsHandler) GetStats(c *gin.Context) {
	cameraID := c.Query("camera_id")

	stats, err := h.DB.GetRecordingStats(cameraID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query recording stats", err)
		return
	}

	cameras := make([]cameraStatsResponse, 0, len(stats))
	for _, s := range stats {
		gaps, err := h.DB.GetRecordingGaps(s.CameraID, defaultGapThresholdMs)
		if err != nil {
			apiError(c, http.StatusInternalServerError, "failed to query recording gaps", err)
			return
		}

		entry := cameraStatsResponse{
			CameraID:        s.CameraID,
			CameraName:      s.CameraName,
			TotalBytes:      s.TotalBytes,
			SegmentCount:    s.SegmentCount,
			TotalRecordedMs: s.TotalRecordedMs,
			OldestRecording: s.OldestRecording,
			NewestRecording: s.NewestRecording,
			GapCount:        len(gaps),
		}

		if len(gaps) > 0 {
			lastGapEnd := gaps[len(gaps)-1].End
			entry.LastGapEnd = &lastGapEnd
			// Current uptime = newest recording end - last gap end.
			newest, err1 := time.Parse("2006-01-02T15:04:05.000Z", s.NewestRecording)
			lastEnd, err2 := time.Parse("2006-01-02T15:04:05.000Z", lastGapEnd)
			if err1 == nil && err2 == nil {
				entry.CurrentUptimeMs = newest.Sub(lastEnd).Milliseconds()
			}
		} else if s.OldestRecording != "" {
			// No gaps: uptime = total span from oldest to newest.
			oldest, err1 := time.Parse("2006-01-02T15:04:05.000Z", s.OldestRecording)
			newest, err2 := time.Parse("2006-01-02T15:04:05.000Z", s.NewestRecording)
			if err1 == nil && err2 == nil {
				entry.CurrentUptimeMs = newest.Sub(oldest).Milliseconds()
			}
		}

		cameras = append(cameras, entry)
	}

	c.JSON(http.StatusOK, gin.H{"cameras": cameras})
}

// GetGaps returns the full gap history for a single camera.
// Path param: camera_id.
func (h *StatsHandler) GetGaps(c *gin.Context) {
	cameraID := c.Param("camera_id")

	if !hasCameraPermission(c, cameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	gaps, err := h.DB.GetRecordingGaps(cameraID, defaultGapThresholdMs)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query recording gaps", err)
		return
	}

	if gaps == nil {
		gaps = []db.Gap{}
	}

	c.JSON(http.StatusOK, gin.H{
		"camera_id": cameraID,
		"gaps":      gaps,
	})
}
