package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/diagnostics"
)

// DiagnosticsHandler exposes read-only remote diagnostics endpoints for the
// integrator portal: recorder status, log viewer, network probes, and support
// bundle generation with expiring downloads.
type DiagnosticsHandler struct {
	Service *diagnostics.Service
}

// GetRecorderStatuses returns the recording status of every configured camera.
//
//	GET /api/nvr/diagnostics/recorders (admin only)
func (h *DiagnosticsHandler) GetRecorderStatuses(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	statuses := h.Service.GetRecorderStatuses()
	c.JSON(http.StatusOK, gin.H{
		"recorders": statuses,
	})
}

// QueryLogs returns filtered log entries for the log viewer.
//
//	GET /api/nvr/diagnostics/logs (admin only)
//
// Query parameters: search, level, module, after, before, limit, offset
func (h *DiagnosticsHandler) QueryLogs(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "100"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	q := diagnostics.LogQuery{
		Search: c.Query("search"),
		Level:  c.Query("level"),
		Module: c.Query("module"),
		After:  c.Query("after"),
		Before: c.Query("before"),
		Limit:  limit,
		Offset: offset,
	}

	entries, total, err := h.Service.QueryLogs(q)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query logs", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"entries": entries,
		"total":   total,
		"limit":   q.Limit,
		"offset":  q.Offset,
	})
}

// RunNetworkProbes runs connectivity checks against default NVR endpoints and
// returns the results.
//
//	POST /api/nvr/diagnostics/network-probe (admin only)
func (h *DiagnosticsHandler) RunNetworkProbes(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	// Accept optional custom targets in request body; otherwise run defaults.
	var req struct {
		Targets []string `json:"targets"`
	}
	_ = c.ShouldBindJSON(&req)

	var results []diagnostics.ProbeResult
	if len(req.Targets) > 0 {
		results = h.Service.RunNetworkProbe(req.Targets)
	} else {
		results = h.Service.RunDefaultProbes()
	}

	c.JSON(http.StatusOK, gin.H{
		"results": results,
	})
}

// GenerateBundle triggers asynchronous support bundle generation and returns
// the bundle ID for status polling.
//
//	POST /api/nvr/diagnostics/bundles (admin only)
func (h *DiagnosticsHandler) GenerateBundle(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	id := h.Service.GenerateBundle()
	nvrLogInfo("diagnostics", fmt.Sprintf("support bundle generation started: %s", id))

	c.JSON(http.StatusAccepted, gin.H{
		"id":      id,
		"status":  "pending",
		"message": "Support bundle generation started.",
	})
}

// GetBundle returns the metadata for a specific bundle.
//
//	GET /api/nvr/diagnostics/bundles/:id (admin only)
func (h *DiagnosticsHandler) GetBundle(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	id := c.Param("id")
	bundle, ok := h.Service.GetBundle(id)
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": "bundle not found"})
		return
	}

	c.JSON(http.StatusOK, bundle)
}

// ListBundles returns all known bundles.
//
//	GET /api/nvr/diagnostics/bundles (admin only)
func (h *DiagnosticsHandler) ListBundles(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	bundles := h.Service.ListBundles()
	c.JSON(http.StatusOK, gin.H{
		"bundles": bundles,
	})
}

// DownloadBundle streams the bundle ZIP file if it is ready and not expired.
//
//	GET /api/nvr/diagnostics/bundles/:id/download (admin only)
func (h *DiagnosticsHandler) DownloadBundle(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	id := c.Param("id")
	path := h.Service.BundlePath(id)
	if path == "" {
		bundle, ok := h.Service.GetBundle(id)
		if !ok {
			c.JSON(http.StatusNotFound, gin.H{"error": "bundle not found"})
			return
		}
		if bundle.Status == "expired" {
			c.JSON(http.StatusGone, gin.H{"error": "bundle has expired"})
			return
		}
		c.JSON(http.StatusConflict, gin.H{
			"error":  "bundle not ready for download",
			"status": bundle.Status,
		})
		return
	}

	filename := fmt.Sprintf("support-bundle-%s.zip", id)
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.File(path)
}
