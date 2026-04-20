package api

import (
	"encoding/base64"
	"errors"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	nvrCrypto "github.com/bluenviron/mediamtx/internal/shared/auth"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/recorder/onvif"
)

// ScreenshotHandler implements HTTP endpoints for on-demand camera screenshots.
type ScreenshotHandler struct {
	DB            *db.DB
	EncryptionKey []byte
}

// decryptPassword decrypts a stored password. If the value does not have
// the "enc:" prefix it is returned unchanged (plaintext / legacy value).
func (h *ScreenshotHandler) decryptPassword(stored string) string {
	if len(h.EncryptionKey) == 0 || stored == "" {
		return stored
	}
	if !strings.HasPrefix(stored, "enc:") {
		return stored
	}
	ct, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(stored, "enc:"))
	if err != nil {
		nvrLogWarn("screenshots", "failed to decode encrypted ONVIF password")
		return ""
	}
	pt, err := nvrCrypto.Decrypt(h.EncryptionKey, ct)
	if err != nil {
		nvrLogWarn("screenshots", "failed to decrypt ONVIF password")
		return ""
	}
	return string(pt)
}

// Capture handles POST /cameras/:id/screenshot.
// It fetches a live JPEG from the camera, saves it to disk, records it in the
// database, and returns the screenshot record.
func (h *ScreenshotHandler) Capture(c *gin.Context) {
	cameraID := c.Param("id")

	cam, err := h.DB.GetCamera(cameraID)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "camera not found"})
			return
		}
		log.Printf("[screenshots] get camera %s: %v", cameraID, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get camera"})
		return
	}

	if cam.SnapshotURI == "" && cam.ONVIFEndpoint == "" {
		c.JSON(http.StatusUnprocessableEntity, gin.H{"error": "camera has no snapshot URI or ONVIF endpoint"})
		return
	}

	password := h.decryptPassword(cam.ONVIFPassword)

	outputDir := filepath.Join(".", "screenshots", cam.ID)

	// CaptureSnapshot saves the file to outputDir and returns a web path like
	// "/thumbnails/<filename>". We need to derive the actual disk path and
	// build our own web path under /screenshots/{cameraID}/.
	webPath, err := onvif.CaptureSnapshot(cam.RTSPURL, cam.ONVIFUsername, password, outputDir, cam.ID, cam.SnapshotURI)
	if err != nil {
		log.Printf("[screenshots] capture snapshot for camera %s: %v", cameraID, err)
		c.JSON(http.StatusBadGateway, gin.H{"error": "failed to capture snapshot: " + err.Error()})
		return
	}

	// CaptureSnapshot returns "/thumbnails/<filename>" because saveSnapshot
	// hardcodes that prefix. Extract the filename and build the correct paths.
	filename := filepath.Base(webPath)
	diskPath := filepath.Join(outputDir, filename)
	screenshotWebPath := "/screenshots/" + cam.ID + "/" + filename

	// Stat the file to get its size.
	info, err := os.Stat(diskPath)
	if err != nil {
		log.Printf("[screenshots] stat captured file %s: %v", diskPath, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to stat screenshot file"})
		return
	}

	s := &db.Screenshot{
		CameraID:  cam.ID,
		FilePath:  screenshotWebPath,
		FileSize:  info.Size(),
		CreatedAt: time.Now().UTC().Format("2006-01-02T15:04:05Z"),
	}
	if err := h.DB.InsertScreenshot(s); err != nil {
		log.Printf("[screenshots] insert screenshot record: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to save screenshot record"})
		return
	}

	c.JSON(http.StatusCreated, s)
}

// List handles GET /screenshots.
// Query params: camera_id, sort (asc|desc, default desc), page (default 1),
// per_page (default 20).
func (h *ScreenshotHandler) List(c *gin.Context) {
	cameraID := c.Query("camera_id")
	sort := c.DefaultQuery("sort", "newest")
	if sort == "newest" {
		sort = "desc"
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	perPage, _ := strconv.Atoi(c.DefaultQuery("per_page", "20"))

	screenshots, total, err := h.DB.ListScreenshots(cameraID, sort, page, perPage)
	if err != nil {
		log.Printf("[screenshots] list: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to list screenshots"})
		return
	}

	if screenshots == nil {
		screenshots = []*db.Screenshot{}
	}

	c.JSON(http.StatusOK, gin.H{
		"screenshots": screenshots,
		"total":       total,
		"page":        page,
		"per_page":    perPage,
	})
}

// Download handles GET /screenshots/:id/download.
// It serves the screenshot file as an attachment.
func (h *ScreenshotHandler) Download(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid screenshot id"})
		return
	}

	s, err := h.DB.GetScreenshot(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "screenshot not found"})
			return
		}
		log.Printf("[screenshots] get screenshot %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get screenshot"})
		return
	}

	// s.FilePath is a web path like "/screenshots/{cameraID}/{filename}".
	// Prepend "." to get the disk path.
	diskPath := "." + s.FilePath
	filename := filepath.Base(diskPath)

	c.FileAttachment(diskPath, filename)
}

// Delete handles DELETE /screenshots/:id.
// It removes the screenshot file from disk and the record from the database.
func (h *ScreenshotHandler) Delete(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid screenshot id"})
		return
	}

	s, err := h.DB.GetScreenshot(id)
	if err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "screenshot not found"})
			return
		}
		log.Printf("[screenshots] get screenshot %d for delete: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to get screenshot"})
		return
	}

	// Delete file from disk; ignore errors (file may already be gone).
	diskPath := "." + s.FilePath
	_ = os.Remove(diskPath)

	if err := h.DB.DeleteScreenshot(id); err != nil {
		if errors.Is(err, db.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "screenshot not found"})
			return
		}
		log.Printf("[screenshots] delete screenshot %d: %v", id, err)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to delete screenshot"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "screenshot deleted"})
}
