package api

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// TourHandler implements HTTP endpoints for camera tour management.
type TourHandler struct {
	DB    *db.DB
	Audit *AuditLogger
}

// createTourRequest is the JSON body for creating a tour.
type createTourRequest struct {
	Name         string   `json:"name" binding:"required"`
	CameraIDs    []string `json:"camera_ids"`
	DwellSeconds int      `json:"dwell_seconds"`
}

// updateTourRequest is the JSON body for updating a tour.
type updateTourRequest struct {
	Name         string   `json:"name" binding:"required"`
	CameraIDs    []string `json:"camera_ids"`
	DwellSeconds int      `json:"dwell_seconds"`
}

// List returns all tours as a JSON array.
//
//	GET /api/nvr/tours
func (h *TourHandler) List(c *gin.Context) {
	tours, err := h.DB.ListTours()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list tours", err)
		return
	}
	if tours == nil {
		tours = []db.Tour{}
	}
	c.JSON(http.StatusOK, tours)
}

// Create inserts a new tour.
//
//	POST /api/nvr/tours
func (h *TourHandler) Create(c *gin.Context) {
	var req createTourRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if req.CameraIDs == nil {
		req.CameraIDs = []string{}
	}
	if req.DwellSeconds <= 0 {
		req.DwellSeconds = 10
	}

	tour, err := h.DB.CreateTour(req.Name, req.CameraIDs, req.DwellSeconds)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create tour", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "create", "tour", tour.ID, "Created tour "+req.Name)
	}

	c.JSON(http.StatusCreated, tour)
}

// Get returns a single tour by ID.
//
//	GET /api/nvr/tours/:id
func (h *TourHandler) Get(c *gin.Context) {
	id := c.Param("id")

	tour, err := h.DB.GetTour(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tour not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve tour", err)
		return
	}

	c.JSON(http.StatusOK, tour)
}

// Update replaces the name, camera list, and dwell time of an existing tour.
//
//	PUT /api/nvr/tours/:id
func (h *TourHandler) Update(c *gin.Context) {
	id := c.Param("id")

	_, err := h.DB.GetTour(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tour not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve tour for update", err)
		return
	}

	var req updateTourRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name is required"})
		return
	}

	if req.CameraIDs == nil {
		req.CameraIDs = []string{}
	}
	if req.DwellSeconds <= 0 {
		req.DwellSeconds = 10
	}

	if err := h.DB.UpdateTour(id, req.Name, req.CameraIDs, req.DwellSeconds); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update tour", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "update", "tour", id, "Updated tour "+req.Name)
	}

	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// Delete removes a tour by ID.
//
//	DELETE /api/nvr/tours/:id
func (h *TourHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	_, err := h.DB.GetTour(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "tour not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve tour for deletion", err)
		return
	}

	if err := h.DB.DeleteTour(id); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete tour", err)
		return
	}

	if h.Audit != nil {
		h.Audit.logAction(c, "delete", "tour", id, "Deleted tour")
	}

	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
