package api

import (
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// MigrationStatusProvider abstracts the upgrade migration manager for the API layer.
type MigrationStatusProvider interface {
	GetMigrationStatus() (*MigrationStatusResponse, error)
}

// MigrationStatusResponse is the response payload for GET /system/migration-status.
type MigrationStatusResponse struct {
	SchemaVersion    int                    `json:"schema_version"`
	AppVersion       string                 `json:"app_version"`
	LastMigration    *db.UpgradeMigration   `json:"last_migration,omitempty"`
	History          []*db.UpgradeMigration `json:"history"`
	RollbackPossible bool                   `json:"rollback_possible"`
}

// MigrationHandler implements the GET /api/nvr/system/migration-status endpoint.
type MigrationHandler struct {
	Manager MigrationStatusProvider
}

// Status returns the current upgrade migration status.
//
//	GET /api/nvr/system/migration-status (admin only)
func (h *MigrationHandler) Status(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}

	status, err := h.Manager.GetMigrationStatus()
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to get migration status", err)
		return
	}

	c.JSON(http.StatusOK, status)
}
