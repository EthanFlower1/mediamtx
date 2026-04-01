package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// QuotaHandler implements HTTP endpoints for storage quota management.
type QuotaHandler struct {
	DB *db.DB
}

// ListQuotas returns all storage quotas (global + any per-path quotas).
//
//	GET /api/nvr/quotas
func (h *QuotaHandler) ListQuotas(c *gin.Context) {
	quotas, err := h.DB.ListStorageQuotas()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list quotas", err)
		return
	}
	if quotas == nil {
		quotas = []*db.StorageQuota{}
	}
	c.JSON(http.StatusOK, quotas)
}

// SetGlobalQuota creates or updates the global storage quota.
//
//	PUT /api/nvr/quotas/global
func (h *QuotaHandler) SetGlobalQuota(c *gin.Context) {
	var req struct {
		QuotaBytes      int64 `json:"quota_bytes" binding:"required"`
		WarningPercent  int   `json:"warning_percent"`
		CriticalPercent int   `json:"critical_percent"`
		Enabled         *bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	if req.QuotaBytes < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quota_bytes must be non-negative"})
		return
	}

	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	if req.WarningPercent <= 0 {
		req.WarningPercent = 80
	}
	if req.CriticalPercent <= 0 {
		req.CriticalPercent = 90
	}

	q := &db.StorageQuota{
		ID:              "global",
		Name:            "Global Storage Quota",
		QuotaBytes:      req.QuotaBytes,
		WarningPercent:  req.WarningPercent,
		CriticalPercent: req.CriticalPercent,
		Enabled:         enabled,
	}
	if err := h.DB.UpsertStorageQuota(q); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to set global quota", err)
		return
	}

	c.JSON(http.StatusOK, q)
}

// QuotaStatus returns current quota usage status for all cameras and global scope.
//
//	GET /api/nvr/quotas/status
func (h *QuotaHandler) QuotaStatus(c *gin.Context) {
	// Global quota status.
	var globalStatus *db.QuotaStatus
	globalQuota, err := h.DB.GetStorageQuota("global")
	if err == nil && globalQuota.Enabled && globalQuota.QuotaBytes > 0 {
		totalUsed, _ := h.DB.GetTotalStorageUsage()
		usedPercent := float64(totalUsed) / float64(globalQuota.QuotaBytes) * 100
		status := "ok"
		if usedPercent >= 100 {
			status = "exceeded"
		} else if int(usedPercent) >= globalQuota.CriticalPercent {
			status = "critical"
		} else if int(usedPercent) >= globalQuota.WarningPercent {
			status = "warning"
		}
		globalStatus = &db.QuotaStatus{
			QuotaBytes:      globalQuota.QuotaBytes,
			UsedBytes:       totalUsed,
			UsedPercent:     usedPercent,
			Status:          status,
			WarningPercent:  globalQuota.WarningPercent,
			CriticalPercent: globalQuota.CriticalPercent,
		}
	}

	// Per-camera quota status.
	cameras, err := h.DB.ListCameras()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list cameras", err)
		return
	}

	storageMap := make(map[string]int64)
	perCamera, err := h.DB.GetStoragePerCamera()
	if err == nil {
		for _, cs := range perCamera {
			storageMap[cs.CameraID] = cs.TotalBytes
		}
	}

	var cameraStatuses []db.QuotaStatus
	for _, cam := range cameras {
		if cam.QuotaBytes <= 0 {
			continue
		}
		used := storageMap[cam.ID]
		usedPercent := float64(used) / float64(cam.QuotaBytes) * 100
		status := "ok"
		if usedPercent >= 100 {
			status = "exceeded"
		} else if int(usedPercent) >= cam.QuotaCriticalPercent {
			status = "critical"
		} else if int(usedPercent) >= cam.QuotaWarningPercent {
			status = "warning"
		}
		cameraStatuses = append(cameraStatuses, db.QuotaStatus{
			CameraID:        cam.ID,
			CameraName:      cam.Name,
			QuotaBytes:      cam.QuotaBytes,
			UsedBytes:       used,
			UsedPercent:     usedPercent,
			Status:          status,
			WarningPercent:  cam.QuotaWarningPercent,
			CriticalPercent: cam.QuotaCriticalPercent,
		})
	}

	if cameraStatuses == nil {
		cameraStatuses = []db.QuotaStatus{}
	}

	c.JSON(http.StatusOK, gin.H{
		"global":  globalStatus,
		"cameras": cameraStatuses,
	})
}

// SetCameraQuota updates the quota settings for a specific camera.
//
//	PUT /api/nvr/cameras/:id/quota
func (h *QuotaHandler) SetCameraQuota(c *gin.Context) {
	id := c.Param("id")

	var req struct {
		QuotaBytes      int64 `json:"quota_bytes"`
		WarningPercent  int   `json:"warning_percent"`
		CriticalPercent int   `json:"critical_percent"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request: " + err.Error()})
		return
	}

	if req.QuotaBytes < 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "quota_bytes must be non-negative"})
		return
	}
	if req.WarningPercent <= 0 {
		req.WarningPercent = 80
	}
	if req.CriticalPercent <= 0 {
		req.CriticalPercent = 90
	}

	if err := h.DB.UpdateCameraQuota(id, req.QuotaBytes, req.WarningPercent, req.CriticalPercent); err != nil {
		if err == db.ErrNotFound {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		apiError(c, http.StatusInternalServerError, "failed to update camera quota", err)
		return
	}

	cam, err := h.DB.GetCamera(id)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve updated camera", err)
		return
	}

	c.JSON(http.StatusOK, cam)
}
