package api

import (
	"net/http"
	"syscall"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/storage"
)

// StorageHandler serves storage health and pending-sync endpoints.
type StorageHandler struct {
	DB      *db.DB
	Manager *storage.Manager
}

type storagePathStatus struct {
	Path        string `json:"path"`
	Healthy     bool   `json:"healthy"`
	CameraCount int    `json:"camera_count"`
	TotalBytes  uint64 `json:"total_bytes"`
	UsedBytes   uint64 `json:"used_bytes"`
	FreeBytes   uint64 `json:"free_bytes"`
}

// Status returns health and disk-usage information for every custom storage
// path that is referenced by at least one camera.
func (h *StorageHandler) Status(c *gin.Context) {
	cameras, err := h.DB.ListCameras()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list cameras", err)
		return
	}

	pathCounts := make(map[string]int)
	for _, cam := range cameras {
		if cam.StoragePath != "" {
			pathCounts[cam.StoragePath]++
		}
	}

	healthMap := h.Manager.GetAllHealth()
	var result []storagePathStatus

	for path, count := range pathCounts {
		status := storagePathStatus{
			Path:        path,
			Healthy:     healthMap[path],
			CameraCount: count,
		}

		var stat syscall.Statfs_t
		if err := syscall.Statfs(path, &stat); err == nil {
			status.TotalBytes = stat.Blocks * uint64(stat.Bsize)
			status.FreeBytes = stat.Bavail * uint64(stat.Bsize)
			status.UsedBytes = status.TotalBytes - status.FreeBytes
		}

		result = append(result, status)
	}

	c.JSON(http.StatusOK, gin.H{"paths": result})
}

// Pending returns the total number of recordings awaiting sync to their
// primary storage path, broken down by camera ID.
func (h *StorageHandler) Pending(c *gin.Context) {
	counts, err := h.DB.PendingSyncCountByCamera()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get pending counts", err)
		return
	}

	total := 0
	for _, count := range counts {
		total += count
	}

	c.JSON(http.StatusOK, gin.H{"total": total, "by_camera": counts})
}

// TriggerSync manually triggers a sync pass for all pending syncs belonging
// to the given camera.
func (h *StorageHandler) TriggerSync(c *gin.Context) {
	cameraID := c.Param("camera_id")
	if cameraID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "camera_id is required"})
		return
	}
	h.Manager.TriggerSyncForCamera(cameraID)
	c.JSON(http.StatusOK, gin.H{"message": "sync triggered"})
}
