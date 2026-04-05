package api

import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/updater"
)

// UpdateHandler implements HTTP endpoints for system update management.
type UpdateHandler struct {
	DB      *db.DB
	Manager *updater.Manager
}

// Check queries the upstream release endpoint and returns whether an
// update is available.
//
//	GET /api/nvr/system/updates/check
func (h *UpdateHandler) Check(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	result, err := h.Manager.Check()
	if err != nil {
		apiError(c, http.StatusBadGateway, "failed to check for updates", err)
		return
	}

	c.JSON(http.StatusOK, result)
}

// Apply downloads and installs the latest available update.
//
//	POST /api/nvr/system/updates/apply
func (h *UpdateHandler) Apply(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	lastCheck := h.Manager.LastCheck()
	if lastCheck == nil || !lastCheck.UpdateAvailable || lastCheck.Release == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "no update available; run check first",
			"code":  "no_update",
		})
		return
	}

	userID, _ := c.Get("user_id")
	initiatedBy := fmt.Sprintf("%v", userID)

	result, err := h.Manager.Apply(lastCheck.Release, initiatedBy)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "update failed", err)
		return
	}

	if !result.Success {
		c.JSON(http.StatusUnprocessableEntity, result)
		return
	}

	c.JSON(http.StatusOK, result)
}

// Rollback restores the previous binary from backup.
//
//	POST /api/nvr/system/updates/rollback
func (h *UpdateHandler) Rollback(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	userID, _ := c.Get("user_id")
	initiatedBy := fmt.Sprintf("%v", userID)

	result, err := h.Manager.Rollback(initiatedBy)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "rollback failed", err)
		return
	}

	if !result.Success {
		c.JSON(http.StatusUnprocessableEntity, result)
		return
	}

	c.JSON(http.StatusOK, result)
}

// History returns the update history log.
//
//	GET /api/nvr/system/updates/history
func (h *UpdateHandler) History(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	limit := 50
	if l := c.Query("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 200 {
			limit = parsed
		}
	}

	records, err := h.DB.ListUpdateHistory(limit)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list update history", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"updates": records,
		"total":   len(records),
	})
}
