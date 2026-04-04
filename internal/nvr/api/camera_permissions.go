package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// CameraPermissionHandler implements HTTP endpoints for per-camera permission management.
type CameraPermissionHandler struct {
	DB    *db.DB
	Audit *AuditLogger
}

// cameraPermissionRequest is the JSON body for setting camera permissions.
type cameraPermissionRequest struct {
	CameraID    string   `json:"camera_id" binding:"required"`
	Permissions []string `json:"permissions" binding:"required"`
}

// bulkCameraPermissionRequest sets all camera permissions for a user at once.
type bulkCameraPermissionRequest struct {
	Permissions []cameraPermissionRequest `json:"permissions" binding:"required"`
}

// List returns all camera permissions for a user.
func (h *CameraPermissionHandler) List(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	userID := c.Param("id")
	perms, err := h.DB.ListCameraPermissions(userID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list camera permissions", err)
		return
	}
	if perms == nil {
		perms = []*db.CameraPermission{}
	}
	c.JSON(http.StatusOK, perms)
}

// Set creates or updates camera permissions for a specific user+camera pair.
func (h *CameraPermissionHandler) Set(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	userID := c.Param("id")

	// Verify user exists.
	if _, err := h.DB.GetUser(userID); errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	var req cameraPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: camera_id and permissions required"})
		return
	}

	// Validate permissions (camera-level permissions, no "admin").
	cameraPerms := map[string]bool{
		"view_live":     true,
		"view_playback": true,
		"export":        true,
		"ptz_control":   true,
	}
	for _, p := range req.Permissions {
		if !cameraPerms[p] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid camera permission: " + p})
			return
		}
	}

	permsJSON, _ := json.Marshal(req.Permissions)
	cp := &db.CameraPermission{
		UserID:      userID,
		CameraID:    req.CameraID,
		Permissions: string(permsJSON),
	}

	if err := h.DB.SetCameraPermission(cp); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to set camera permission", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "update", "camera_permission", userID,
			"Set camera permissions for camera "+req.CameraID)
	}

	c.JSON(http.StatusOK, cp)
}

// SetBulk replaces all camera permissions for a user.
func (h *CameraPermissionHandler) SetBulk(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	userID := c.Param("id")

	// Verify user exists.
	if _, err := h.DB.GetUser(userID); errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}

	var req bulkCameraPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	cameraPerms := map[string]bool{
		"view_live":     true,
		"view_playback": true,
		"export":        true,
		"ptz_control":   true,
	}

	var dbPerms []*db.CameraPermission
	for _, p := range req.Permissions {
		for _, perm := range p.Permissions {
			if !cameraPerms[perm] {
				c.JSON(http.StatusBadRequest, gin.H{"error": "invalid camera permission: " + perm})
				return
			}
		}
		permsJSON, _ := json.Marshal(p.Permissions)
		dbPerms = append(dbPerms, &db.CameraPermission{
			CameraID:    p.CameraID,
			Permissions: string(permsJSON),
		})
	}

	if err := h.DB.SetBulkCameraPermissions(userID, dbPerms); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to set camera permissions", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "update", "camera_permission", userID,
			"Bulk updated camera permissions")
	}

	c.JSON(http.StatusOK, gin.H{"message": "camera permissions updated"})
}

// Delete removes camera permissions for a specific user+camera pair.
func (h *CameraPermissionHandler) Delete(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	userID := c.Param("id")
	cameraID := c.Param("cameraId")

	if err := h.DB.DeleteCameraPermission(userID, cameraID); errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera permission not found"})
		return
	} else if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete camera permission", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "delete", "camera_permission", userID,
			"Removed camera permissions for camera "+cameraID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "camera permission deleted"})
}
