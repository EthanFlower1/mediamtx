package api

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/jpeg"
	"log"
	"net/http"
	"os/exec"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/ai"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// SearchHandler implements the semantic search API endpoint.
type SearchHandler struct {
	DB       *db.DB
	Embedder *ai.Embedder // may be nil if CLIP is not available
}

// Search handles GET /api/nvr/search?q=...&camera_id=...&start=...&end=...&limit=20
//
// Query parameters:
//   - q: search query text (required)
//   - camera_id: filter by camera ID (optional)
//   - start: start time in RFC3339 format (optional, defaults to 24h ago)
//   - end: end time in RFC3339 format (optional, defaults to now)
//   - limit: maximum number of results (optional, defaults to 20, max 100)
func (h *SearchHandler) Search(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
		return
	}

	cameraID := c.Query("camera_id")

	now := time.Now().UTC()
	// Default to all time when no start/end provided.
	start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	end := now

	if s := c.Query("start"); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'start' time format, use RFC3339"})
			return
		}
		start = parsed
	}

	if e := c.Query("end"); e != "" {
		parsed, err := time.Parse(time.RFC3339, e)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'end' time format, use RFC3339"})
			return
		}
		end = parsed
	}

	limit := 20
	if l := c.Query("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'limit' parameter"})
			return
		}
		if parsed > 100 {
			parsed = 100
		}
		limit = parsed
	}

	results, err := ai.Search(h.Embedder, h.DB, query, cameraID, start, end, limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "search failed: " + err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"query":   query,
		"count":   len(results),
		"results": results,
	})
}

// Backfill generates CLIP embeddings for detections that are missing them.
// It extracts frames from recorded video, crops to the detection bounding box,
// and runs the CLIP visual encoder.
//
// POST /api/nvr/search/backfill?start=RFC3339&end=RFC3339
func (h *SearchHandler) Backfill(c *gin.Context) {
	if h.Embedder == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CLIP embedder not available"})
		return
	}

	now := time.Now().UTC()
	start := now.Add(-24 * time.Hour)
	end := now

	if s := c.Query("start"); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'start' time"})
			return
		}
		start = parsed
	}
	if e := c.Query("end"); e != "" {
		parsed, err := time.Parse(time.RFC3339, e)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'end' time"})
			return
		}
		end = parsed
	}

	dets, err := h.DB.ListDetectionsNeedingEmbedding(start, end)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list detections", err)
		return
	}

	if len(dets) == 0 {
		c.JSON(http.StatusOK, gin.H{"message": "no detections need backfill", "processed": 0})
		return
	}

	log.Printf("[backfill] starting embedding backfill for %d detections", len(dets))

	// Run backfill in background so the request returns immediately.
	go h.runBackfill(dets)

	c.JSON(http.StatusAccepted, gin.H{
		"message": fmt.Sprintf("backfill started for %d detections", len(dets)),
		"count":   len(dets),
	})
}

func (h *SearchHandler) runBackfill(dets []*db.DetectionForBackfill) {
	succeeded, failed := 0, 0

	for i, det := range dets {
		frameTime, err := time.Parse("2006-01-02T15:04:05.000Z", det.FrameTime)
		if err != nil {
			failed++
			continue
		}

		// Find recording containing this detection.
		recs, err := h.DB.QueryRecordings(det.CameraID, frameTime.Add(-1*time.Second), frameTime.Add(1*time.Second))
		if err != nil || len(recs) == 0 {
			recs, _ = h.DB.QueryRecordings(det.CameraID, frameTime.Add(-60*time.Second), frameTime)
		}
		if len(recs) == 0 {
			failed++
			continue
		}

		rec := recs[len(recs)-1]
		recStart, _ := time.Parse("2006-01-02T15:04:05.000Z", rec.StartTime)
		offset := frameTime.Sub(recStart)

		// Extract frame via ffmpeg.
		img, err := extractFrame(rec.FilePath, offset)
		if err != nil {
			failed++
			continue
		}

		// Crop to bounding box.
		bounds := img.Bounds()
		x := int(det.BoxX * float64(bounds.Dx()))
		y := int(det.BoxY * float64(bounds.Dy()))
		w := int(det.BoxW * float64(bounds.Dx()))
		bh := int(det.BoxH * float64(bounds.Dy()))
		if w <= 0 || bh <= 0 {
			failed++
			continue
		}
		cropRect := image.Rect(x, y, x+w, y+bh).Intersect(bounds)
		if cropRect.Empty() {
			failed++
			continue
		}
		cropped := cropToNRGBA(img, cropRect)

		// Generate embedding.
		embedding, err := h.Embedder.EncodeImage(cropped)
		if err != nil {
			failed++
			continue
		}

		embBytes := ai.Float32SliceToBytes(embedding)
		if err := h.DB.UpdateDetectionEmbedding(det.ID, embBytes); err != nil {
			failed++
			continue
		}

		succeeded++
		if (i+1)%50 == 0 {
			log.Printf("[backfill] progress: %d/%d (ok=%d, fail=%d)", i+1, len(dets), succeeded, failed)
		}
	}

	log.Printf("[backfill] complete: %d succeeded, %d failed out of %d total", succeeded, failed, len(dets))
}

// extractFrame uses ffmpeg to extract a single frame from a recording file.
func extractFrame(filePath string, offset time.Duration) (image.Image, error) {
	cmd := exec.Command("ffmpeg",
		"-ss", fmt.Sprintf("%.3f", offset.Seconds()),
		"-i", filePath,
		"-frames:v", "1",
		"-f", "image2",
		"-c:v", "mjpeg",
		"-an",
		"pipe:1",
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("ffmpeg: %w: %s", err, stderr.String())
	}
	img, _, err := image.Decode(&stdout)
	if err != nil {
		return nil, fmt.Errorf("decode frame: %w", err)
	}
	return img, nil
}

// cropToNRGBA extracts a sub-rectangle from an image into a new NRGBA image.
func cropToNRGBA(src image.Image, rect image.Rectangle) *image.NRGBA {
	dst := image.NewNRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		for x := rect.Min.X; x < rect.Max.X; x++ {
			r, g, b, a := src.At(x, y).RGBA()
			dst.SetNRGBA(x-rect.Min.X, y-rect.Min.Y, color.NRGBA{
				R: uint8(r >> 8), G: uint8(g >> 8), B: uint8(b >> 8), A: uint8(a >> 8),
			})
		}
	}
	return dst
}
