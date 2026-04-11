package diagnostics

import (
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
)

// Handler provides the HTTP API for diagnostics bundle generation.
type Handler struct {
	Collector *Collector

	mu      sync.Mutex
	bundles map[string]*Bundle // in-memory index, keyed by bundle ID
}

// NewHandler creates a Handler.
func NewHandler(c *Collector) *Handler {
	return &Handler{
		Collector: c,
		bundles:   make(map[string]*Bundle),
	}
}

// GenerateBundleRequest is the JSON body for POST /api/nvr/diagnostics/bundle.
type GenerateBundleRequest struct {
	HoursBack int      `json:"hours_back"`
	Sections  []string `json:"sections"`
}

// GenerateBundle handles POST /api/nvr/diagnostics/bundle.
// It generates a support bundle, encrypts it, and optionally uploads it.
func (h *Handler) GenerateBundle(c *gin.Context) {
	var req GenerateBundleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		// Allow empty body (defaults).
		req = GenerateBundleRequest{}
	}

	bundle, _, err := h.Collector.Generate(c.Request.Context(), GenerateRequest{
		HoursBack: req.HoursBack,
		Sections:  req.Sections,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "failed to generate support bundle",
			"detail": err.Error(),
		})
		return
	}

	h.mu.Lock()
	h.bundles[bundle.BundleID] = bundle
	h.mu.Unlock()

	c.JSON(http.StatusOK, bundle)
}

// GetBundle handles GET /api/nvr/diagnostics/bundle/:id.
func (h *Handler) GetBundle(c *gin.Context) {
	id := c.Param("id")

	h.mu.Lock()
	bundle, ok := h.bundles[id]
	h.mu.Unlock()

	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "bundle not found"})
		return
	}

	c.JSON(http.StatusOK, bundle)
}

// ListBundles handles GET /api/nvr/diagnostics/bundles.
func (h *Handler) ListBundles(c *gin.Context) {
	h.mu.Lock()
	out := make([]*Bundle, 0, len(h.bundles))
	for _, b := range h.bundles {
		out = append(out, b)
	}
	h.mu.Unlock()

	c.JSON(http.StatusOK, out)
}

// CleanExpired handles POST /api/nvr/diagnostics/cleanup.
func (h *Handler) CleanExpired(c *gin.Context) {
	h.mu.Lock()
	var all []Bundle
	for _, b := range h.bundles {
		all = append(all, *b)
	}
	h.mu.Unlock()

	deleted, err := h.Collector.CleanExpired(c.Request.Context(), all)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":  "cleanup failed",
			"detail": err.Error(),
		})
		return
	}

	// Remove expired bundles from in-memory index.
	h.mu.Lock()
	for _, b := range all {
		if _, ok := h.bundles[b.BundleID]; ok {
			if b.Status == StatusExpired || (b.StorageKey != "" && deleted > 0) {
				delete(h.bundles, b.BundleID)
			}
		}
	}
	h.mu.Unlock()

	c.JSON(http.StatusOK, gin.H{"deleted": deleted})
}
