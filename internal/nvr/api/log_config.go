package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/shared/logmgr"
)

// LogConfigHandler handles GET/PUT /system/logging/config for reading and
// updating the structured logging configuration at runtime.
type LogConfigHandler struct {
	LogManager *logmgr.Manager
}

// GetLoggingConfig returns the current logging configuration.
//
//	GET /api/nvr/system/logging/config (admin only)
func (h *LogConfigHandler) GetLoggingConfig(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	if h.LogManager == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "log manager not available"})
		return
	}

	cfg := h.LogManager.GetConfig()

	c.JSON(http.StatusOK, gin.H{
		"global_level":       cfg.GlobalLevel,
		"module_levels":      cfg.ModuleLevels,
		"log_dir":            cfg.LogDir,
		"max_size_mb":        cfg.MaxSizeMB,
		"max_age_days":       cfg.MaxAgeDays,
		"max_backups":        cfg.MaxBackups,
		"json_output":        cfg.JSONOutput,
		"crash_dump_enabled": cfg.CrashDumpEnabled,
		"crash_dumps":        h.LogManager.ListCrashDumps(),
	})
}

// updateLoggingConfigRequest is the request body for PUT /system/logging/config.
type updateLoggingConfigRequest struct {
	GlobalLevel      *string            `json:"global_level"`
	ModuleLevels     map[string]string  `json:"module_levels"`
	MaxSizeMB        *int               `json:"max_size_mb"`
	MaxAgeDays       *int               `json:"max_age_days"`
	MaxBackups       *int               `json:"max_backups"`
	JSONOutput       *bool              `json:"json_output"`
	CrashDumpEnabled *bool              `json:"crash_dump_enabled"`
}

// UpdateLoggingConfig updates the logging configuration at runtime.
// Only provided fields are updated; omitted fields retain their current values.
//
//	PUT /api/nvr/system/logging/config (admin only)
func (h *LogConfigHandler) UpdateLoggingConfig(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	if h.LogManager == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "log manager not available"})
		return
	}

	var req updateLoggingConfigRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Validate level values.
	validLevels := map[string]bool{"debug": true, "info": true, "warn": true, "error": true}
	if req.GlobalLevel != nil {
		if !validLevels[*req.GlobalLevel] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid global_level; must be debug, info, warn, or error"})
			return
		}
	}
	for mod, lvl := range req.ModuleLevels {
		if !validLevels[lvl] {
			c.JSON(http.StatusBadRequest, gin.H{"error": "invalid level for module " + mod + "; must be debug, info, warn, or error"})
			return
		}
	}

	// Merge with current config.
	cfg := h.LogManager.GetConfig()

	if req.GlobalLevel != nil {
		cfg.GlobalLevel = *req.GlobalLevel
	}
	if req.ModuleLevels != nil {
		cfg.ModuleLevels = req.ModuleLevels
	}
	if req.MaxSizeMB != nil {
		if *req.MaxSizeMB < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "max_size_mb must be at least 1"})
			return
		}
		cfg.MaxSizeMB = *req.MaxSizeMB
	}
	if req.MaxAgeDays != nil {
		if *req.MaxAgeDays < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "max_age_days must be at least 1"})
			return
		}
		cfg.MaxAgeDays = *req.MaxAgeDays
	}
	if req.MaxBackups != nil {
		if *req.MaxBackups < 1 {
			c.JSON(http.StatusBadRequest, gin.H{"error": "max_backups must be at least 1"})
			return
		}
		cfg.MaxBackups = *req.MaxBackups
	}
	if req.JSONOutput != nil {
		cfg.JSONOutput = *req.JSONOutput
	}
	if req.CrashDumpEnabled != nil {
		cfg.CrashDumpEnabled = *req.CrashDumpEnabled
	}

	if err := h.LogManager.UpdateConfig(cfg); err != nil {
		apiError(c, http.StatusInternalServerError, "failed to update logging config", err)
		return
	}

	nvrLogInfo("logging", "logging configuration updated")

	c.JSON(http.StatusOK, gin.H{
		"global_level":       cfg.GlobalLevel,
		"module_levels":      cfg.ModuleLevels,
		"log_dir":            cfg.LogDir,
		"max_size_mb":        cfg.MaxSizeMB,
		"max_age_days":       cfg.MaxAgeDays,
		"max_backups":        cfg.MaxBackups,
		"json_output":        cfg.JSONOutput,
		"crash_dump_enabled": cfg.CrashDumpEnabled,
	})
}

// GetCrashDump returns the contents of a specific crash dump file.
//
//	GET /api/nvr/system/logging/crashes/:filename (admin only)
func (h *LogConfigHandler) GetCrashDump(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	if h.LogManager == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "log manager not available"})
		return
	}

	filename := c.Param("filename")
	content, err := h.LogManager.GetCrashDump(filename)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "crash dump not found"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"filename": filename,
		"content":  content,
	})
}
