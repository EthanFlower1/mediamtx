package nvr

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/api"
	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// MigrationManager handles upgrade migrations with pre-upgrade backup,
// config file migration, and rollback support.
type MigrationManager struct {
	DB           *db.DB
	ConfigPath   string // path to mediamtx.yml
	DatabasePath string // path to nvr.db
	BackupDir    string // directory for pre-upgrade backups
	AppVersion   string // current application version

	mu sync.Mutex
}

// NewMigrationManager creates a MigrationManager and ensures the backup directory exists.
func NewMigrationManager(database *db.DB, configPath, databasePath, backupDir, appVersion string) *MigrationManager {
	m := &MigrationManager{
		DB:           database,
		ConfigPath:   configPath,
		DatabasePath: databasePath,
		BackupDir:    backupDir,
		AppVersion:   appVersion,
	}
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		log.Printf("[NVR] [WARN] [migration] failed to create backup dir: %v", err)
	}
	return m
}

// GetMigrationStatus returns the current migration status including schema version,
// app version, and recent migration history. Implements api.MigrationStatusProvider.
func (m *MigrationManager) GetMigrationStatus() (*api.MigrationStatusResponse, error) {
	schemaVer, err := m.DB.GetCurrentSchemaVersion()
	if err != nil {
		return nil, fmt.Errorf("get schema version: %w", err)
	}

	history, err := m.DB.ListUpgradeMigrations(20)
	if err != nil {
		return nil, fmt.Errorf("list migrations: %w", err)
	}

	var latest *db.UpgradeMigration
	rollbackPossible := false
	if len(history) > 0 {
		latest = history[0]
		// Rollback is possible if the latest migration completed successfully
		// and backup files still exist.
		if latest.Status == "completed" {
			rollbackPossible = m.backupFilesExist(latest)
		}
	}

	return &api.MigrationStatusResponse{
		SchemaVersion:    schemaVer,
		AppVersion:       m.AppVersion,
		LastMigration:    latest,
		History:          history,
		RollbackPossible: rollbackPossible,
	}, nil
}

// RunPreUpgradeBackup creates timestamped backups of the config file and database
// before an upgrade. Returns the migration record ID for tracking.
func (m *MigrationManager) RunPreUpgradeBackup(fromVersion, toVersion string) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now().UTC()
	timestamp := now.Format("20060102-150405")

	// Back up config file.
	configBackupPath := ""
	if m.ConfigPath != "" {
		configBackupPath = filepath.Join(m.BackupDir, fmt.Sprintf("mediamtx-%s.yml", timestamp))
		if err := copyFile(m.ConfigPath, configBackupPath); err != nil {
			return 0, fmt.Errorf("backup config: %w", err)
		}
		log.Printf("[NVR] [INFO] [migration] config backed up to %s", configBackupPath)
	}

	// Back up database.
	dbBackupPath := ""
	if m.DatabasePath != "" {
		dbBackupPath = filepath.Join(m.BackupDir, fmt.Sprintf("nvr-%s.db", timestamp))
		if err := copyFile(m.DatabasePath, dbBackupPath); err != nil {
			// Clean up the config backup if DB backup fails.
			if configBackupPath != "" {
				os.Remove(configBackupPath)
			}
			return 0, fmt.Errorf("backup database: %w", err)
		}
		log.Printf("[NVR] [INFO] [migration] database backed up to %s", dbBackupPath)
	}

	// Record the migration.
	record := &db.UpgradeMigration{
		FromVersion:      fromVersion,
		ToVersion:        toVersion,
		Status:           "pending",
		ConfigBackupPath: configBackupPath,
		DBBackupPath:     dbBackupPath,
		StartedAt:        now.Format(time.RFC3339),
	}

	id, err := m.DB.InsertUpgradeMigration(record)
	if err != nil {
		return 0, fmt.Errorf("record migration: %w", err)
	}

	log.Printf("[NVR] [INFO] [migration] pre-upgrade backup complete (id=%d, %s -> %s)", id, fromVersion, toVersion)
	return id, nil
}

// MarkCompleted marks a migration as successfully completed.
func (m *MigrationManager) MarkCompleted(migrationID int64) error {
	return m.DB.UpdateUpgradeMigrationStatus(migrationID, "completed", "")
}

// MarkFailed marks a migration as failed with the given error message,
// then attempts to rollback automatically.
func (m *MigrationManager) MarkFailed(migrationID int64, errMsg string) error {
	if err := m.DB.UpdateUpgradeMigrationStatus(migrationID, "failed", errMsg); err != nil {
		return fmt.Errorf("mark failed: %w", err)
	}

	// Attempt automatic rollback.
	if err := m.Rollback(migrationID); err != nil {
		log.Printf("[NVR] [ERROR] [migration] auto-rollback failed for migration %d: %v", migrationID, err)
		return fmt.Errorf("auto-rollback failed: %w", err)
	}

	return nil
}

// Rollback restores the config file and database from the pre-upgrade backup.
func (m *MigrationManager) Rollback(migrationID int64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	migration, err := m.DB.GetLatestUpgradeMigration()
	if err != nil {
		return fmt.Errorf("get migration: %w", err)
	}
	if migration == nil || migration.ID != migrationID {
		return fmt.Errorf("migration %d not found or not the latest", migrationID)
	}

	if !m.backupFilesExist(migration) {
		return fmt.Errorf("backup files no longer exist for migration %d", migrationID)
	}

	// Restore config.
	if migration.ConfigBackupPath != "" && m.ConfigPath != "" {
		if err := copyFile(migration.ConfigBackupPath, m.ConfigPath); err != nil {
			return fmt.Errorf("restore config: %w", err)
		}
		log.Printf("[NVR] [INFO] [migration] config restored from %s", migration.ConfigBackupPath)
	}

	// Restore database.
	if migration.DBBackupPath != "" && m.DatabasePath != "" {
		if err := copyFile(migration.DBBackupPath, m.DatabasePath); err != nil {
			return fmt.Errorf("restore database: %w", err)
		}
		log.Printf("[NVR] [INFO] [migration] database restored from %s", migration.DBBackupPath)
	}

	// Mark as rolled back (best effort since DB may have been replaced).
	_ = m.DB.SetUpgradeMigrationRollback(migrationID)

	log.Printf("[NVR] [INFO] [migration] rollback complete for migration %d", migrationID)
	return nil
}

// backupFilesExist checks whether the backup files for a migration still exist on disk.
func (m *MigrationManager) backupFilesExist(migration *db.UpgradeMigration) bool {
	if migration.ConfigBackupPath != "" {
		if _, err := os.Stat(migration.ConfigBackupPath); err != nil {
			return false
		}
	}
	if migration.DBBackupPath != "" {
		if _, err := os.Stat(migration.DBBackupPath); err != nil {
			return false
		}
	}
	return migration.ConfigBackupPath != "" || migration.DBBackupPath != ""
}

// copyFile copies src to dst, creating dst with 0600 permissions.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source %s: %w", src, err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
		return fmt.Errorf("create parent dir: %w", err)
	}

	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return fmt.Errorf("create dest %s: %w", dst, err)
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy: %w", err)
	}
	return out.Sync()
}
