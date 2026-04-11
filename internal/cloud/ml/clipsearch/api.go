package clipsearch

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

// Handler serves the cloud CLIP search API endpoints. It is registered
// on the Gin router by the cloud API server.
type Handler struct {
	Service *Service
}

// NewHandler constructs an API handler for the search service.
func NewHandler(svc *Service) *Handler {
	return &Handler{Service: svc}
}

// Search handles GET /api/cloud/search
//
// Query parameters:
//   - q:              search query text (required)
//   - tenant_id:      tenant scope (required; in production extracted from JWT)
//   - camera_ids:     comma-separated camera IDs (optional)
//   - start:          RFC3339 start time (optional)
//   - end:            RFC3339 end time (optional)
//   - limit:          max results, default 20, max 100 (optional)
//   - dedup_window:   dedup window in seconds, default 30 (optional)
//   - min_similarity: minimum cosine similarity 0.0-1.0 (optional)
func (h *Handler) Search(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' is required"})
		return
	}

	tenantID := c.Query("tenant_id")
	if tenantID == "" {
		// In production the tenant_id comes from the JWT middleware.
		// For the API layer, require it explicitly.
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'tenant_id' is required"})
		return
	}

	req := SearchRequest{
		TenantID: tenantID,
		Query:    query,
	}

	// Parse optional camera_ids.
	if cameraIDs := c.Query("camera_ids"); cameraIDs != "" {
		req.CameraIDs = strings.Split(cameraIDs, ",")
	}

	// Parse optional time range.
	if s := c.Query("start"); s != "" {
		parsed, err := time.Parse(time.RFC3339, s)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'start' time format, use RFC3339"})
			return
		}
		req.Start = parsed
	}
	if e := c.Query("end"); e != "" {
		parsed, err := time.Parse(time.RFC3339, e)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'end' time format, use RFC3339"})
			return
		}
		req.End = parsed
	}

	// Parse optional limit.
	if l := c.Query("limit"); l != "" {
		parsed, err := strconv.Atoi(l)
		if err != nil || parsed < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'limit' parameter"})
			return
		}
		req.Limit = parsed
	}

	// Parse optional dedup window.
	if d := c.Query("dedup_window"); d != "" {
		parsed, err := strconv.Atoi(d)
		if err != nil || parsed < 0 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'dedup_window' parameter"})
			return
		}
		req.DedupWindowSec = parsed
	}

	// Parse optional min similarity.
	if ms := c.Query("min_similarity"); ms != "" {
		parsed, err := strconv.ParseFloat(ms, 64)
		if err != nil || parsed < 0 || parsed > 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid 'min_similarity' parameter (0.0-1.0)"})
			return
		}
		req.MinSimilarity = parsed
	}

	resp, err := h.Service.Search(c.Request.Context(), req)
	if err != nil {
		status := http.StatusInternalServerError
		switch {
		case err == ErrMissingTenantID || err == ErrEmptyQuery:
			status = http.StatusBadRequest
		}
		c.JSON(status, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, resp)
}

// RegisterRoutes registers the search endpoint on a Gin router group.
// Typical usage:
//
//	cloudAPI := router.Group("/api/cloud")
//	handler.RegisterRoutes(cloudAPI)
func (h *Handler) RegisterRoutes(rg *gin.RouterGroup) {
	rg.GET("/search", h.Search)
}
