package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/ai"
)

// ModelHandler serves the AI model management API endpoints.
type ModelHandler struct {
	Manager *ai.ModelManager
}

// ListModels handles GET /api/nvr/ai/models.
// It scans the models directory and returns information about each ONNX model.
func (h *ModelHandler) ListModels(c *gin.Context) {
	models, err := h.Manager.ListModels()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list models", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"models": models,
		"count":  len(models),
		"active": h.Manager.ActiveModelPath(),
	})
}

// activateRequest is the JSON body for POST /api/nvr/ai/models/activate.
type activateRequest struct {
	ModelPath string `json:"model_path" binding:"required"`
}

// ActivateModel handles POST /api/nvr/ai/models/activate.
// It hot-swaps the active detection model. On failure, the previous model is
// retained automatically.
func (h *ModelHandler) ActivateModel(c *gin.Context) {
	var req activateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model_path is required"})
		return
	}

	if err := h.Manager.ActivateModel(req.ModelPath); err != nil {
		apiError(c, http.StatusBadRequest, "failed to activate model", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "model activated successfully",
		"active":  h.Manager.ActiveModelPath(),
	})
}

// RollbackModel handles POST /api/nvr/ai/models/rollback.
// It reverts to the previously active detection model.
func (h *ModelHandler) RollbackModel(c *gin.Context) {
	if err := h.Manager.Rollback(); err != nil {
		apiError(c, http.StatusBadRequest, "rollback failed", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "rolled back to previous model",
		"active":  h.Manager.ActiveModelPath(),
	})
}
