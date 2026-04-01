package api

import (
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/integrity"
)

// IntegrityHandler implements HTTP endpoints for recording integrity operations.
type IntegrityHandler struct {
	DB             *db.DB
	Events         *EventBroadcaster
	RecordingsBase string
	QuarantineBase string
}

// VerifyRequest is the JSON body for the Verify endpoint.
type VerifyRequest struct {
	CameraID string `json:"camera_id"`
	Start    string `json:"start"`
	End      string `json:"end"`
}

// VerifyResponse is the response for the Verify endpoint.
type VerifyResponse struct {
	Total     int                 `json:"total"`
	OK        int                 `json:"ok"`
	Corrupted int                 `json:"corrupted"`
	Results   []VerifyResultEntry `json:"results"`
}

// VerifyResultEntry is a single result in the verify response.
type VerifyResultEntry struct {
	RecordingID int64  `json:"recording_id"`
	CameraID    string `json:"camera_id"`
	FilePath    string `json:"file_path"`
	Status      string `json:"status"`
	Detail      string `json:"detail,omitempty"`
}

// Verify triggers integrity verification for recordings matching the given filters.
// POST /api/nvr/recordings/verify
func (h *IntegrityHandler) Verify(c *gin.Context) {
	var req VerifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		req = VerifyRequest{}
	}

	if req.CameraID != "" {
		if !hasCameraPermission(c, req.CameraID) {
			c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
			return
		}
	}

	var startTime, endTime *time.Time
	if req.Start != "" {
		t, err := time.Parse(time.RFC3339, req.Start)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid start time"})
			return
		}
		startTime = &t
	}
	if req.End != "" {
		t, err := time.Parse(time.RFC3339, req.End)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid end time"})
			return
		}
		endTime = &t
	}

	recordings, err := h.DB.GetRecordingsByFilter(req.CameraID, startTime, endTime)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query recordings", err)
		return
	}

	resp := VerifyResponse{
		Total:   len(recordings),
		Results: make([]VerifyResultEntry, 0, len(recordings)),
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")

	for _, rec := range recordings {
		fragCount := 0
		if frags, err := h.DB.GetFragments(rec.ID); err == nil {
			fragCount = len(frags)
		}

		info := integrity.RecordingInfo{
			FilePath:      rec.FilePath,
			FileSize:      rec.FileSize,
			InitSize:      rec.InitSize,
			FragmentCount: fragCount,
			DurationMs:    rec.DurationMs,
		}

		result := integrity.VerifySegment(info)

		var detail *string
		if result.Detail != "" {
			detail = &result.Detail
		}
		h.DB.UpdateRecordingStatus(rec.ID, result.Status, detail, now)

		entry := VerifyResultEntry{
			RecordingID: rec.ID,
			CameraID:    rec.CameraID,
			FilePath:    rec.FilePath,
			Status:      result.Status,
			Detail:      result.Detail,
		}
		resp.Results = append(resp.Results, entry)

		if result.Status == integrity.StatusOK {
			resp.OK++
		} else {
			resp.Corrupted++
			if h.Events != nil {
				h.Events.PublishSegmentCorrupted(rec.CameraID, rec.ID, rec.FilePath, result.Detail)
			}
		}
	}

	c.JSON(http.StatusOK, resp)
}

// Quarantine moves a recording file to the quarantine directory.
// POST /api/nvr/recordings/:id/quarantine
func (h *IntegrityHandler) Quarantine(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid recording id"})
		return
	}

	rec, err := h.DB.GetRecording(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get recording", err)
		return
	}

	if !hasCameraPermission(c, rec.CameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	if rec.Status == integrity.StatusQuarantined {
		c.JSON(http.StatusConflict, gin.H{"error": "recording is already quarantined"})
		return
	}

	newPath, err := integrity.QuarantineFile(rec.FilePath, h.RecordingsBase, h.QuarantineBase)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to quarantine file", err)
		return
	}

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	detail := fmt.Sprintf("quarantined from %s", rec.FilePath)
	h.DB.UpdateRecordingStatus(id, integrity.StatusQuarantined, &detail, now)
	h.DB.UpdateRecordingFilePath(id, newPath)

	if h.Events != nil {
		h.Events.PublishSegmentQuarantined(rec.CameraID, id, newPath)
	}

	c.JSON(http.StatusOK, gin.H{
		"status":          "quarantined",
		"quarantine_path": newPath,
	})
}

// Unquarantine restores a quarantined recording file and re-verifies it.
// POST /api/nvr/recordings/:id/unquarantine
func (h *IntegrityHandler) Unquarantine(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid recording id"})
		return
	}

	rec, err := h.DB.GetRecording(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "recording not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get recording", err)
		return
	}

	if !hasCameraPermission(c, rec.CameraID) {
		c.JSON(http.StatusForbidden, gin.H{"error": "no permission for this camera"})
		return
	}

	if rec.Status != integrity.StatusQuarantined {
		c.JSON(http.StatusConflict, gin.H{"error": "recording is not quarantined"})
		return
	}

	restoredPath, err := integrity.UnquarantineFile(rec.FilePath, h.QuarantineBase, h.RecordingsBase)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to restore file", err)
		return
	}

	h.DB.UpdateRecordingFilePath(id, restoredPath)

	fragCount := 0
	if frags, dbErr := h.DB.GetFragments(id); dbErr == nil {
		fragCount = len(frags)
	}
	info := integrity.RecordingInfo{
		FilePath:      restoredPath,
		FileSize:      rec.FileSize,
		InitSize:      rec.InitSize,
		FragmentCount: fragCount,
		DurationMs:    rec.DurationMs,
	}
	result := integrity.VerifySegment(info)

	now := time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
	var resultDetail *string
	if result.Detail != "" {
		resultDetail = &result.Detail
	}
	h.DB.UpdateRecordingStatus(id, result.Status, resultDetail, now)

	c.JSON(http.StatusOK, gin.H{
		"status":    result.Status,
		"file_path": restoredPath,
	})
}
