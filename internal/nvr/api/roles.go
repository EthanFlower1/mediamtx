package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// validPermissions is the set of all recognized permission strings.
var validPermissions = map[string]bool{
	"view_live":     true,
	"view_playback": true,
	"export":        true,
	"ptz_control":   true,
	"admin":         true,
}

// RoleHandler implements HTTP endpoints for role management.
type RoleHandler struct {
	DB    *db.DB
	Audit *AuditLogger
}

// roleRequest is the JSON body for creating/updating a role.
type roleRequest struct {
	Name        string   `json:"name" binding:"required"`
	Permissions []string `json:"permissions" binding:"required"`
}

// List returns all roles.
func (h *RoleHandler) List(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

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

	var req roleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: name and permissions required"})
		return
	}

	if err := validatePermissions(req.Permissions); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	permsJSON, _ := json.Marshal(req.Permissions)
	role := &db.Role{
		Name:        req.Name,
		Permissions: string(permsJSON),
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

// Update updates an existing custom role. System roles cannot be modified.
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

	var req roleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: name and permissions required"})
		return
	}

	if err := validatePermissions(req.Permissions); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	permsJSON, _ := json.Marshal(req.Permissions)
	existing.Name = req.Name
	existing.Permissions = string(permsJSON)

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

// Delete deletes a custom role. System roles cannot be deleted.
func (h *RoleHandler) Delete(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	id := c.Param("id")
	if err := h.DB.DeleteRole(id); err != nil {
		if err.Error() == "cannot delete system role" {
			c.JSON(http.StatusForbidden, gin.H{"error": "cannot delete system role"})
			return
		}
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
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

// validatePermissions checks that all provided permissions are recognized.
func validatePermissions(perms []string) error {
	for _, p := range perms {
		if !validPermissions[p] {
			return errors.New("invalid permission: " + p)
		}
	}
	return nil
}
