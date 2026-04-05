package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// RoleHandler implements HTTP endpoints for role management.
type RoleHandler struct {
	DB    *db.DB
	Audit *AuditLogger
}

// roleCreateRequest is the JSON body for creating a role.
type roleCreateRequest struct {
	Name        string   `json:"name" binding:"required"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions" binding:"required"`
}

// roleUpdateRequest is the JSON body for updating a role.
type roleUpdateRequest struct {
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Permissions []string `json:"permissions"`
}

// cameraPermissionRequest is the JSON body for setting per-camera permissions.
type cameraPermissionRequest struct {
	CameraID    string   `json:"camera_id" binding:"required"`
	Permissions []string `json:"permissions" binding:"required"`
}

// List returns all roles as a JSON array.
func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.DB.ListRoles()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list roles", err)
		return
	}
	if roles == nil {
		roles = []*db.Role{}
	}
	c.JSON(http.StatusOK, roles)
}

// Get returns a single role by ID.
func (h *RoleHandler) Get(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	id := c.Param("id")
	role, err := h.DB.GetRole(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve role", err)
		return
	}
	c.JSON(http.StatusOK, role)
}

// Create creates a new custom role.
func (h *RoleHandler) Create(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	var req roleCreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	// Validate permissions.
	if !validatePermissions(req.Permissions) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid permissions",
			"valid_permissions": db.AllPermissions,
		})
		return
	}

	role := &db.Role{
		Name:        req.Name,
		Description: req.Description,
		Permissions: req.Permissions,
	}

	if err := h.DB.CreateRole(role); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create role", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "create", "role", role.ID, "Created role "+role.Name)
	}

	c.JSON(http.StatusCreated, role)
}

// Update updates an existing role.
func (h *RoleHandler) Update(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	id := c.Param("id")

	existing, err := h.DB.GetRole(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve role", err)
		return
	}

	var req roleUpdateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.Name != "" {
		existing.Name = req.Name
	}
	if req.Description != "" {
		existing.Description = req.Description
	}
	if req.Permissions != nil {
		if !validatePermissions(req.Permissions) {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":             "invalid permissions",
				"valid_permissions": db.AllPermissions,
			})
			return
		}
		existing.Permissions = req.Permissions
	}

	if err := h.DB.UpdateRole(existing); err != nil {
		if err.Error() == "cannot modify system role" {
			c.JSON(http.StatusForbidden, gin.H{"error": "cannot modify system role"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to update role", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "update", "role", existing.ID, "Updated role "+existing.Name)
	}

	c.JSON(http.StatusOK, existing)
}

// Delete deletes a role by ID.
func (h *RoleHandler) Delete(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	id := c.Param("id")

	if err := h.DB.DeleteRole(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
			return
		}
		if err.Error() == "cannot delete system role" {
			c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete system role"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to delete role", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "delete", "role", id, "Deleted role")
	}

	c.JSON(http.StatusOK, gin.H{"message": "role deleted"})
}

// SetCameraPermissions sets per-camera permissions for a specific user.
func (h *RoleHandler) SetCameraPermissions(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	userID := c.Param("id")

	// Verify user exists.
	_, err := h.DB.GetUser(userID)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "user not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve user", err)
		return
	}

	var req cameraPermissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if !validatePermissions(req.Permissions) {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":             "invalid permissions",
			"valid_permissions": db.AllPermissions,
		})
		return
	}

	if err := h.DB.SetUserCameraPermissions(userID, req.CameraID, req.Permissions); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to set camera permissions", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "update", "user_camera_permission", userID,
			"Set camera permissions for camera "+req.CameraID)
	}

	c.JSON(http.StatusOK, gin.H{"message": "camera permissions updated"})
}

// GetCameraPermissions returns all per-camera permissions for a user.
func (h *RoleHandler) GetCameraPermissions(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	userID := c.Param("id")

	perms, err := h.DB.GetUserCameraPermissions(userID)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get camera permissions", err)
		return
	}
	if perms == nil {
		perms = []db.UserCameraPermission{}
	}
	c.JSON(http.StatusOK, perms)
}

// DeleteCameraPermissions removes all per-camera permission overrides for a user.
func (h *RoleHandler) DeleteCameraPermissions(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	userID := c.Param("id")

	if err := h.DB.DeleteUserCameraPermissions(userID); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete camera permissions", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "delete", "user_camera_permission", userID,
			"Removed all camera permission overrides")
	}

	c.JSON(http.StatusOK, gin.H{"message": "camera permissions removed"})
}

// validatePermissions checks that all provided permissions are valid.
func validatePermissions(perms []string) bool {
	valid := make(map[string]bool, len(db.AllPermissions))
	for _, p := range db.AllPermissions {
		valid[p] = true
	}
	for _, p := range perms {
		if !valid[p] {
			return false
		}
	}
	return true
}

// hasPermission checks whether the authenticated user has a specific permission.
// It first checks the user's role permissions, then falls back to legacy role
// names for backward compatibility.
// For admin users, all permissions are granted.
func hasPermission(c *gin.Context, permission string) bool {
	role, _ := c.Get("role")
	roleStr, _ := role.(string)
	if roleStr == "admin" {
		return true
	}

	permsRaw, exists := c.Get("role_permissions")
	if !exists {
		// Backward compatibility: if no role_permissions are set in the JWT,
		// fall back to legacy role-based logic.
		return legacyRoleHasPermission(roleStr, permission)
	}
	permsStr, ok := permsRaw.(string)
	if !ok {
		return legacyRoleHasPermission(roleStr, permission)
	}

	var perms []string
	if err := json.Unmarshal([]byte(permsStr), &perms); err != nil {
		return legacyRoleHasPermission(roleStr, permission)
	}
	for _, p := range perms {
		if p == permission {
			return true
		}
	}
	return false
}

// legacyRoleHasPermission provides backward-compatible permission checks for
// users whose JWTs don't yet contain role_permissions (e.g., before re-login
// after the RBAC migration).
func legacyRoleHasPermission(role, permission string) bool {
	switch role {
	case "admin":
		return true
	case "operator":
		switch permission {
		case db.PermViewLive, db.PermViewPlayback, db.PermExport, db.PermPTZControl:
			return true
		}
	case "viewer":
		switch permission {
		case db.PermViewLive, db.PermViewPlayback:
			return true
		}
	case "":
		// No role set at all — allow all non-admin permissions for backward
		// compatibility with test contexts and pre-RBAC JWTs.
		return permission != db.PermAdmin
	}
	return false
}

// hasCameraSpecificPermission checks whether the user has a specific permission
// for a specific camera. It checks per-camera overrides first, then falls back
// to the role's permissions.
func hasCameraSpecificPermission(c *gin.Context, cameraID, permission string) bool {
	role, _ := c.Get("role")
	if role == "admin" {
		return true
	}

	// First check if the user has camera access at all.
	if !hasCameraPermission(c, cameraID) {
		return false
	}

	// Check per-camera permission overrides.
	camPermsRaw, exists := c.Get("camera_specific_permissions")
	if exists {
		camPermsStr, ok := camPermsRaw.(string)
		if ok && camPermsStr != "" {
			var camPerms map[string][]string
			if err := json.Unmarshal([]byte(camPermsStr), &camPerms); err == nil {
				if perms, found := camPerms[cameraID]; found {
					for _, p := range perms {
						if p == permission {
							return true
						}
					}
					// Per-camera override exists but doesn't include this permission.
					return false
				}
			}
		}
	}

	// Fall back to role-level permissions.
	return hasPermission(c, permission)
}

// requirePermission checks that the authenticated user has a specific permission.
// Returns true if the request should proceed, false if it was aborted.
func requirePermission(c *gin.Context, permission string) bool {
	if !hasPermission(c, permission) {
		c.JSON(http.StatusForbidden, gin.H{"error": permission + " permission required"})
		return false
	}
	return true
}

// requireCameraPermission checks that the authenticated user has a specific
// permission for a specific camera. Returns true if the request should proceed.
func requireCameraPermission(c *gin.Context, cameraID, permission string) bool {
	if !hasCameraSpecificPermission(c, cameraID, permission) {
		c.JSON(http.StatusForbidden, gin.H{"error": permission + " permission required for this camera"})
		return false
	}
	return true
}
