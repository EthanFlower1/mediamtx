package api

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/backup"
)

// BackupHandler implements HTTP endpoints for system backup and restore.
type BackupHandler struct {
	Service *backup.Service
}

// Create creates a new encrypted backup archive.
//
//	POST /api/nvr/system/backups
//	Body: {"password": "...", "auto": false}
func (h *BackupHandler) Create(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	var req struct {
		Password string `json:"password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password is required"})
		return
	}

	if len(req.Password) < 8 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
		return
	}

	filename, err := h.Service.CreateBackup(req.Password, false)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to create backup", err)
		return
	}

	nvrLogInfo("backup", fmt.Sprintf("manual backup created: %s", filename))
	c.JSON(http.StatusOK, gin.H{
		"filename": filename,
		"message":  "backup created successfully",
	})
}

// List returns all available backups.
//
//	GET /api/nvr/system/backups
func (h *BackupHandler) List(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	backups, err := h.Service.List()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to list backups", err)
		return
	}

	c.JSON(http.StatusOK, gin.H{"backups": backups})
}

// Download serves a backup file for download.
//
//	GET /api/nvr/system/backups/:filename/download
func (h *BackupHandler) Download(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	filename := c.Param("filename")
	path, err := h.Service.GetBackupPath(filename)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}

	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	c.Header("Content-Type", "application/octet-stream")
	c.File(path)
}

// Delete removes a backup file.
//
//	DELETE /api/nvr/system/backups/:filename
func (h *BackupHandler) Delete(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	filename := c.Param("filename")
	if err := h.Service.DeleteBackup(filename); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
		return
	}

	nvrLogInfo("backup", fmt.Sprintf("backup deleted: %s", filename))
	c.JSON(http.StatusOK, gin.H{"message": "backup deleted"})
}

// Validate decrypts and validates an uploaded backup without applying it.
//
//	POST /api/nvr/system/backups/validate
//	Multipart form: file + password
func (h *BackupHandler) Validate(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	password := c.PostForm("password")
	if password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password is required"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "backup file is required"})
		return
	}

	f, err := file.Open()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to open uploaded file", err)
		return
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, 500<<20)) // 500 MB limit
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to read uploaded file", err)
		return
	}

	fileNames, err := h.Service.ValidateBackup(data, password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid backup: %v", err)})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"valid": true,
		"files": fileNames,
	})
}

// Restore decrypts, validates, and applies an uploaded backup.
// The server should be restarted after a successful restore.
//
//	POST /api/nvr/system/backups/restore
//	Multipart form: file + password
func (h *BackupHandler) Restore(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	password := c.PostForm("password")
	if password == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "password is required"})
		return
	}

	file, err := c.FormFile("file")
	if err != nil {
		// Try reading from the backup directory by filename.
		filename := c.PostForm("filename")
		if filename == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "backup file or filename is required"})
			return
		}
		path, pathErr := h.Service.GetBackupPath(filename)
		if pathErr != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": "backup not found"})
			return
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			apiError(c, http.StatusInternalServerError, "failed to read backup file", readErr)
			return
		}
		h.doRestore(c, data, password)
		return
	}

	f, err := file.Open()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to open uploaded file", err)
		return
	}
	defer f.Close()

	data, err := io.ReadAll(io.LimitReader(f, 500<<20)) // 500 MB limit
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to read uploaded file", err)
		return
	}

	h.doRestore(c, data, password)
}

// doRestore performs the actual restore operation.
func (h *BackupHandler) doRestore(c *gin.Context, data []byte, password string) {
	restored, err := h.Service.RestoreBackup(data, password)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("restore failed: %v", err)})
		return
	}

	nvrLogInfo("backup", fmt.Sprintf("backup restored with %d files, restart required", len(restored)))
	c.JSON(http.StatusOK, gin.H{
		"message":        "backup restored successfully — restart required",
		"restored_files": restored,
		"restart_required": true,
	})
}

// Schedule configures automatic scheduled backups.
//
//	PUT /api/nvr/system/backups/schedule
//	Body: {"enabled": true, "interval_hours": 24, "password": "...", "max_keep": 10}
func (h *BackupHandler) Schedule(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	var req struct {
		Enabled       bool   `json:"enabled"`
		IntervalHours int    `json:"interval_hours"`
		Password      string `json:"password"`
		MaxKeep       int    `json:"max_keep"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	if req.Enabled {
		if req.Password == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password is required for scheduled backups"})
			return
		}
		if len(req.Password) < 8 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "password must be at least 8 characters"})
			return
		}
		if req.IntervalHours < 1 {
			req.IntervalHours = 24
		}
		if req.MaxKeep < 1 {
			req.MaxKeep = 10
		}

		h.Service.StartSchedule(
			time.Duration(req.IntervalHours)*time.Hour,
			req.Password,
			req.MaxKeep,
		)
		nvrLogInfo("backup", fmt.Sprintf("scheduled backups enabled: every %dh, keep %d", req.IntervalHours, req.MaxKeep))
	} else {
		h.Service.StopSchedule()
		nvrLogInfo("backup", "scheduled backups disabled")
	}

	c.JSON(http.StatusOK, h.Service.GetScheduleStatus())
}

// GetSchedule returns the current backup schedule configuration.
//
//	GET /api/nvr/system/backups/schedule
func (h *BackupHandler) GetSchedule(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	c.JSON(http.StatusOK, h.Service.GetScheduleStatus())
}
