package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/recorder/ai"
)

// ModelHandler handles AI model management API endpoints.
type ModelHandler struct {
	Manager     *ai.ModelManager
	AIRestarter AIPipelineRestarter // restart pipelines after model swap
}

// List returns all installed models with metadata.
// GET /api/nvr/ai/models
func (h *ModelHandler) List(c *gin.Context) {
	if h.Manager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "AI model manager not available",
		})
		return
	}

	models, err := h.Manager.ListModels()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to list models: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"models":       models,
		"active_model": h.Manager.ActiveModel(),
	})
}

// activateRequest is the request body for POST /ai/models/activate.
type activateRequest struct {
	// ModelPath is the filename or full path of the model to activate.
	ModelPath string `json:"model_path" binding:"required"`
}

// Activate hot-swaps the active detector model.
// POST /api/nvr/ai/models/activate
func (h *ModelHandler) Activate(c *gin.Context) {
	if h.Manager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "AI model manager not available",
		})
		return
	}

	var req activateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request: " + err.Error(),
		})
		return
	}

	if err := h.Manager.Activate(req.ModelPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to activate model: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "model activated successfully",
		"active_model": h.Manager.ActiveModel(),
	})
}

// Rollback reverts to the previous model.
// POST /api/nvr/ai/models/rollback
func (h *ModelHandler) Rollback(c *gin.Context) {
	if h.Manager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "AI model manager not available",
		})
		return
	}

	if err := h.Manager.Rollback(); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message":      "rolled back to previous model",
		"active_model": h.Manager.ActiveModel(),
	})
}

// verifyRequest is the request body for POST /ai/models/verify.
type verifyRequest struct {
	ModelPath string `json:"model_path" binding:"required"`
}

// Verify computes and returns the SHA-256 checksum of a model file.
// POST /api/nvr/ai/models/verify
func (h *ModelHandler) Verify(c *gin.Context) {
	if h.Manager == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{
			"error": "AI model manager not available",
		})
		return
	}

	var req verifyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request: " + err.Error(),
		})
		return
	}

	checksum, err := h.Manager.VerifyModel(req.ModelPath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "verification failed: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"model_path": req.ModelPath,
		"sha256":     checksum,
	})
}
