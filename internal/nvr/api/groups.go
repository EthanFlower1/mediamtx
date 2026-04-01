package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// GroupHandler implements HTTP endpoints for camera group management.
type GroupHandler struct {
	DB    *db.DB
	Audit *AuditLogger
}

// createGroupRequest is the JSON body for creating a camera group.
type createGroupRequest struct {
	Name      string   `json:"name" binding:"required"`
	CameraIDs []string `json:"camera_ids"`
}

// updateGroupRequest is the JSON body for updating a camera group.
type updateGroupRequest struct {
	Name      string   `json:"name" binding:"required"`
	CameraIDs []string `json:"camera_ids"`
}

// List returns all camera groups as a JSON array.
//
//	GET /api/nvr/camera-groups
func (h *GroupHandler) List(c *gin.Context) {
	groups, err := h.DB.ListGroups()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list camera groups", err)
		return
	}
	if groups == nil {
		groups = []db.CameraGroup{}
	}
	c.JSON(http.StatusOK, groups)
}

// Create inserts a new camera group.
//
//	POST /api/nvr/camera-groups
func (h *GroupHandler) Create(c *gin.Context) {
	var req createGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if req.CameraIDs == nil {
		req.CameraIDs = []string{}
	}

	group, err := h.DB.CreateGroup(req.Name, req.CameraIDs)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create camera group", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "create", "camera_group", group.ID, "Created group "+req.Name)
	}

	c.JSON(http.StatusCreated, group)
}

// Get returns a single camera group by ID.
//
//	GET /api/nvr/camera-groups/:id
func (h *GroupHandler) Get(c *gin.Context) {
	id := c.Param("id")

	group, err := h.DB.GetGroup(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera group not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera group", err)
		return
	}

	c.JSON(http.StatusOK, group)
}

// Update replaces the name and camera list of an existing group.
//
//	PUT /api/nvr/camera-groups/:id
func (h *GroupHandler) Update(c *gin.Context) {
	id := c.Param("id")

	_, err := h.DB.GetGroup(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera group not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera group for update", err)
		return
	}

	var req updateGroupRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if req.CameraIDs == nil {
		req.CameraIDs = []string{}
	}

	if err := h.DB.UpdateGroup(id, req.Name, req.CameraIDs); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update camera group", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "update", "camera_group", id, "Updated group "+req.Name)
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// Delete removes a camera group by ID.
//
//	DELETE /api/nvr/camera-groups/:id
func (h *GroupHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	_, err := h.DB.GetGroup(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "camera group not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve camera group for deletion", err)
		return
	}

	if err := h.DB.DeleteGroup(id); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete camera group", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "delete", "camera_group", id, "Deleted group")
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
