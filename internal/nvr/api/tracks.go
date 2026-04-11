package api

import (
	"database/sql"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// TrackHandler implements cross-camera person tracking API endpoints (KAI-482).
// This is a beta feature that relies on detection embeddings to perform
// cross-camera re-identification.
type TrackHandler struct {
	DB *db.DB
}

// GetTrack returns a single track with all sightings.
// GET /api/nvr/tracks/:id
func (h *TrackHandler) GetTrack(c *gin.Context) {
	idStr := c.Param("id")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid track ID"})
		return
	}

	track, err := h.DB.GetTrackWithSightings(id)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "track not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to get track", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"beta":  true,
		"track": track,
	})
}

// ListTracks returns recent cross-camera tracks.
// GET /api/nvr/tracks?limit=50
func (h *TrackHandler) ListTracks(c *gin.Context) {
	limit := 50
	if l := c.Query("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	tracks, err := h.DB.ListTracks(limit)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list tracks", err)
		return
	}

	if tracks == nil {
		tracks = []*db.TrackWithSightings{}
	}

	c.JSON(http.StatusOK, gin.H{
		"beta":   true,
		"count":  len(tracks),
		"tracks": tracks,
	})
}

// StartTracking initiates a cross-camera track from a detection event.
// It finds the detection's embedding, then searches all cameras for similar
// detections within a time window to build the initial sighting list.
//
// POST /api/nvr/detections/:id/track
func (h *TrackHandler) StartTracking(c *gin.Context) {
	idStr := c.Param("id")
	detectionID, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid detection ID"})
		return
	}

	// Check if a track already exists for this detection.
	existing, err := h.DB.FindTrackByDetection(detectionID)
	if err == nil && existing != nil {
		track, err := h.DB.GetTrackWithSightings(existing.ID)
		if err != nil {
			apiError(c, http.StatusInternalServerError, "failed to get existing track", err)
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"beta":    true,
			"message": "track already exists for this detection",
			"track":   track,
		})
		return
	}

	// Get source detection to extract embedding + camera info.
	srcDet, srcCameraID, err := h.getDetectionWithCamera(detectionID)
	if err != nil {
		if err == sql.ErrNoRows {
			c.JSON(http.StatusNotFound, gin.H{"error": "detection not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to get detection", err)
		return
	}

	if len(srcDet.Embedding) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "detection has no embedding; re-identification requires CLIP embeddings",
		})
		return
	}

	// Parse source detection time.
	srcTime, err := time.Parse("2006-01-02T15:04:05.000Z", srcDet.FrameTime)
	if err != nil {
		srcTime, err = time.Parse(time.RFC3339, srcDet.FrameTime)
		if err != nil {
			apiError(c, http.StatusInternalServerError, "failed to parse detection time", err)
			return
		}
	}

	// Create the track.
	track := &db.Track{
		Label:       fmt.Sprintf("Person track from %s", srcTime.Format("15:04:05")),
		Status:      "active",
		DetectionID: detectionID,
	}
	if err := h.DB.InsertTrack(track); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create track", err)
		return
	}

	// Add source detection as the first sighting.
	srcSighting := &db.Sighting{
		TrackID:    track.ID,
		CameraID:   srcCameraID,
		Timestamp:  srcDet.FrameTime,
		Confidence: srcDet.Confidence,
	}
	if err := h.DB.InsertSighting(srcSighting); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to insert source sighting", err)
		return
	}

	// Search for similar detections across all cameras within a +/- 30 minute window.
	windowStart := srcTime.Add(-30 * time.Minute)
	windowEnd := srcTime.Add(30 * time.Minute)
	candidates, err := h.DB.ListDetectionsWithEmbeddings("", windowStart, windowEnd)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to search detections", err)
		return
	}

	srcEmb := bytesToFloat32(srcDet.Embedding)
	matchCount := 0

	for _, cand := range candidates {
		if cand.ID == detectionID {
			continue // skip source
		}
		if len(cand.Embedding) == 0 {
			continue
		}

		candEmb := bytesToFloat32(cand.Embedding)
		sim := cosineSimilarity(srcEmb, candEmb)

		// Threshold: 0.75 cosine similarity for re-id match.
		if sim >= 0.75 {
			// Get camera ID for this detection via its motion event.
			_, candCameraID, err := h.getDetectionWithCamera(cand.ID)
			if err != nil {
				continue
			}

			sighting := &db.Sighting{
				TrackID:    track.ID,
				CameraID:   candCameraID,
				Timestamp:  cand.FrameTime,
				Confidence: cand.Confidence * float64(sim),
			}
			if err := h.DB.InsertSighting(sighting); err != nil {
				continue
			}
			matchCount++
		}
	}

	// Fetch the complete track with all sightings.
	result, err := h.DB.GetTrackWithSightings(track.ID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get created track", err)
		return
	}

	c.JSON(http.StatusCreated, gin.H{
		"beta":    true,
		"message": fmt.Sprintf("track created with %d sightings across %d cameras", len(result.Sightings), result.CameraCount),
		"track":   result,
	})
}

// getDetectionWithCamera returns a detection and its camera ID by joining
// through the motion_events table.
func (h *TrackHandler) getDetectionWithCamera(detectionID int64) (*db.Detection, string, error) {
	row := h.DB.QueryRow(`
		SELECT d.id, d.motion_event_id, d.frame_time, d.class, d.confidence,
			d.box_x, d.box_y, d.box_w, d.box_h, d.embedding, COALESCE(d.attributes, ''),
			me.camera_id
		FROM detections d
		JOIN motion_events me ON d.motion_event_id = me.id
		WHERE d.id = ?`, detectionID)

	det := &db.Detection{}
	var cameraID string
	err := row.Scan(
		&det.ID, &det.MotionEventID, &det.FrameTime, &det.Class,
		&det.Confidence, &det.BoxX, &det.BoxY, &det.BoxW, &det.BoxH,
		&det.Embedding, &det.Attributes,
		&cameraID,
	)
	if err != nil {
		return nil, "", err
	}
	return det, cameraID, nil
}

// bytesToFloat32 converts a byte slice to a float32 slice (little-endian).
func bytesToFloat32(b []byte) []float32 {
	if len(b) == 0 {
		return nil
	}
	n := len(b) / 4
	result := make([]float32, n)
	for i := 0; i < n; i++ {
		bits := uint32(b[i*4]) | uint32(b[i*4+1])<<8 | uint32(b[i*4+2])<<16 | uint32(b[i*4+3])<<24
		result[i] = math.Float32frombits(bits)
	}
	return result
}

// cosineSimilarity computes cosine similarity between two float32 vectors.
func cosineSimilarity(a, b []float32) float64 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}
	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}
	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}
	return dot / denom
}
