package api

import (
	"net/http"
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
	start := now.Add(-24 * time.Hour)
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
