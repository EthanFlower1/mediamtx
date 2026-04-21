// Package cameraapi implements the Directory-side camera management API.
// It exposes CRUD endpoints for cameras and related configuration under
// /api/v1/cameras. This package is a role-scoped extract of the monolithic
// internal/nvr/api layer; recording-specific and detection-specific logic
// live in internal/recorder/recordingapi and internal/recorder/detectionapi.
package cameraapi

import (
	"errors"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	dirdb "github.com/bluenviron/mediamtx/internal/directory/db"
)

// Handler is the camera API handler for the Directory service.
type Handler struct {
	db *dirdb.DB
}

// NewHandler creates a Handler backed by the given directory DB.
func NewHandler(db *dirdb.DB) *Handler {
	return &Handler{db: db}
}

// Register wires the camera routes onto r.
// All routes are protected — callers must apply JWT middleware before calling
// Register, or use a pre-authenticated router group.
func (h *Handler) Register(r gin.IRouter) {
	r.GET("/api/v1/cameras", h.List)
	r.POST("/api/v1/cameras", h.Create)
	r.GET("/api/v1/cameras/:id", h.Get)
	r.PUT("/api/v1/cameras/:id", h.Update)
	r.DELETE("/api/v1/cameras/:id", h.Delete)
}

// cameraRequest is the JSON body for create/update.
type cameraRequest struct {
	Name          string `json:"name"`
	ONVIFEndpoint string `json:"onvif_endpoint"`
	ONVIFUsername string `json:"onvif_username"`
	ONVIFPassword string `json:"onvif_password"`
	RTSPURL       string `json:"rtsp_url"`
	MediaMTXPath  string `json:"mediamtx_path"`
	Tags          string `json:"tags"`
}

// apiError writes a structured error response consistent with nvr/api's style.
func apiError(c *gin.Context, status int, userMsg string, err error) {
	reqID := uuid.New().String()[:8]
	log.Printf("[cameraapi] [ERROR] [%s] %s: %v", reqID, userMsg, err)
	code := "internal_error"
	switch status {
	case http.StatusBadRequest:
		code = "bad_request"
	case http.StatusNotFound:
		code = "not_found"
	case http.StatusConflict:
		code = "conflict"
	}
	c.JSON(status, gin.H{"error": userMsg, "code": code, "request_id": reqID})
}

// List returns all cameras.
//
//	GET /api/v1/cameras
func (h *Handler) List(c *gin.Context) {
	cameras, err := h.db.ListCameras()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list cameras", err)
		return
	}
	if cameras == nil {
		cameras = []*dirdb.Camera{}
	}
	c.JSON(http.StatusOK, gin.H{"items": cameras})
}

// Create adds a new camera.
//
//	POST /api/v1/cameras
func (h *Handler) Create(c *gin.Context) {
	var req cameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}
	if req.Name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	cam := &dirdb.Camera{
		Name:          req.Name,
		ONVIFEndpoint: req.ONVIFEndpoint,
		ONVIFUsername: req.ONVIFUsername,
		ONVIFPassword: req.ONVIFPassword,
		RTSPURL:       req.RTSPURL,
		MediaMTXPath:  req.MediaMTXPath,
		Tags:          req.Tags,
	}

	if err := h.db.CreateCamera(cam); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create camera", err)
		return
	}

	c.JSON(http.StatusCreated, cam)
}

// Get returns a single camera by ID.
//
//	GET /api/v1/cameras/:id
func (h *Handler) Get(c *gin.Context) {
	id := c.Param("id")
	cam, err := h.db.GetCamera(id)
	if err != nil {
		if errors.Is(err, dirdb.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to get camera", err)
		return
	}
	c.JSON(http.StatusOK, cam)
}

// Update replaces a camera's editable fields.
//
//	PUT /api/v1/cameras/:id
func (h *Handler) Update(c *gin.Context) {
	id := c.Param("id")

	// Verify the camera exists first.
	cam, err := h.db.GetCamera(id)
	if err != nil {
		if errors.Is(err, dirdb.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to get camera", err)
		return
	}

	var req cameraRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid JSON body"})
		return
	}

	if req.Name != "" {
		cam.Name = req.Name
	}
	if req.ONVIFEndpoint != "" {
		cam.ONVIFEndpoint = req.ONVIFEndpoint
	}
	if req.ONVIFUsername != "" {
		cam.ONVIFUsername = req.ONVIFUsername
	}
	if req.ONVIFPassword != "" {
		cam.ONVIFPassword = req.ONVIFPassword
	}
	if req.RTSPURL != "" {
		cam.RTSPURL = req.RTSPURL
	}
	if req.MediaMTXPath != "" {
		cam.MediaMTXPath = req.MediaMTXPath
	}
	if req.Tags != "" {
		cam.Tags = req.Tags
	}

	if err := h.db.UpdateCamera(cam); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update camera", err)
		return
	}

	c.JSON(http.StatusOK, cam)
}

// Delete removes a camera.
//
//	DELETE /api/v1/cameras/:id
func (h *Handler) Delete(c *gin.Context) {
	id := c.Param("id")
	if err := h.db.DeleteCamera(id); err != nil {
		if errors.Is(err, dirdb.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to delete camera", err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
