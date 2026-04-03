package api

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

// DeviceHandler implements HTTP endpoints for physical device management.
type DeviceHandler struct {
	DB            *db.DB
	YAMLWriter    *yamlwriter.Writer
	EncryptionKey []byte
	Scheduler     interface{ RemoveCamera(string) }
}

type deviceWithCameras struct {
	db.Device
	Cameras []*db.Camera `json:"cameras"`
}

func (h *DeviceHandler) List(c *gin.Context) {
	devices, err := h.DB.ListDevices()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list devices", err)
		return
	}

	result := make([]deviceWithCameras, 0, len(devices))
	for _, dev := range devices {
		cameras, _ := h.DB.ListCamerasByDevice(dev.ID)
		if cameras == nil {
			cameras = []*db.Camera{}
		}
		result = append(result, deviceWithCameras{Device: *dev, Cameras: cameras})
	}
	c.JSON(http.StatusOK, result)
}

func (h *DeviceHandler) Get(c *gin.Context) {
	id := c.Param("id")

	dev, err := h.DB.GetDevice(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve device", err)
		return
	}

	cameras, _ := h.DB.ListCamerasByDevice(dev.ID)
	if cameras == nil {
		cameras = []*db.Camera{}
	}
	c.JSON(http.StatusOK, deviceWithCameras{Device: *dev, Cameras: cameras})
}

func (h *DeviceHandler) Delete(c *gin.Context) {
	id := c.Param("id")

	dev, err := h.DB.GetDevice(id)
	if errors.Is(err, db.ErrNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to retrieve device", err)
		return
	}

	cameras, _ := h.DB.ListCamerasByDevice(id)
	for _, cam := range cameras {
		if cam.MediaMTXPath != "" {
			_ = h.YAMLWriter.RemovePath(cam.MediaMTXPath)
		}
		streams, _ := h.DB.ListCameraStreams(cam.ID)
		for i, s := range streams {
			if i > 0 {
				subPath := cameraStreamPath(cam, s.ID)
				_ = h.YAMLWriter.RemovePath(subPath)
			}
		}
		_ = h.DB.DeleteStreamsByCamera(cam.ID)
		_ = h.DB.DeleteCamera(cam.ID)
		if h.Scheduler != nil {
			h.Scheduler.RemoveCamera(cam.ID)
		}
	}

	if err := h.DB.DeleteDevice(id); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to delete device", err)
		return
	}

	nvrLogInfo("devices", fmt.Sprintf("Deleted device %q (id=%s) with %d cameras", dev.Name, id, len(cameras)))
	c.JSON(http.StatusOK, gin.H{"message": "device deleted"})
}
