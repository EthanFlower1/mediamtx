package api

import (
	"fmt"
	"image"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/ai"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// CLIPIndexHandler handles CLIP search index management endpoints.
type CLIPIndexHandler struct {
	DB       *db.DB
	Embedder *ai.Embedder // may be nil if CLIP is not available
}

// Status returns the current state of the CLIP embedding index.
//
// GET /api/nvr/ai/clip/status
func (h *CLIPIndexHandler) Status(c *gin.Context) {
	stats, err := h.DB.GetEmbeddingStats()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get embedding stats: " + err.Error()})
		return
	}

	totalEmbeddings := stats.DetectionWithEmbedding + stats.EventWithEmbedding
	totalSearchable := stats.DetectionTotal + stats.EventTotal

	var coveragePct float64
	if totalSearchable > 0 {
		coveragePct = float64(totalEmbeddings) / float64(totalSearchable) * 100
	}

	c.JSON(http.StatusOK, gin.H{
		"embedder_available": h.Embedder != nil,
		"index": gin.H{
			"total_embeddings": totalEmbeddings,
			"total_searchable": totalSearchable,
			"coverage_percent": coveragePct,
			"oldest_embedding": stats.OldestEmbedding,
			"newest_embedding": stats.NewestEmbedding,
		},
		"detections": gin.H{
			"total":          stats.DetectionTotal,
			"with_embedding": stats.DetectionWithEmbedding,
		},
		"events": gin.H{
			"total":          stats.EventTotal,
			"with_embedding": stats.EventWithEmbedding,
		},
	})
}

// Reindex clears all existing embeddings and triggers a full backfill.
//
// POST /api/nvr/ai/clip/reindex
func (h *CLIPIndexHandler) Reindex(c *gin.Context) {
	if h.Embedder == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "CLIP embedder not available"})
		return
	}

	// Clear all existing embeddings.
	cleared, err := h.DB.ClearAllEmbeddings()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to clear embeddings: " + err.Error()})
		return
	}

	log.Printf("[clip-index] cleared %d embeddings, starting full reindex", cleared)

	// Find all detections needing embeddings (effectively all of them now).
	start := time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC)
	end := time.Now().UTC()

	dets, err := h.DB.ListDetectionsNeedingEmbedding(start, end)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list detections: " + err.Error()})
		return
	}

	if len(dets) == 0 {
		c.JSON(http.StatusOK, gin.H{
			"message": "no detections to reindex",
			"cleared": cleared,
			"queued":  0,
		})
		return
	}

	// Run backfill in background.
	go h.runReindex(dets)

	c.JSON(http.StatusAccepted, gin.H{
		"message": fmt.Sprintf("reindex started: cleared %d embeddings, backfilling %d detections", cleared, len(dets)),
		"cleared": cleared,
		"queued":  len(dets),
	})
}

func (h *CLIPIndexHandler) runReindex(dets []*db.DetectionForBackfill) {
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
			log.Printf("[clip-index] reindex progress: %d/%d (ok=%d, fail=%d)", i+1, len(dets), succeeded, failed)
		}
	}

	log.Printf("[clip-index] reindex complete: %d succeeded, %d failed out of %d total", succeeded, failed, len(dets))
}
