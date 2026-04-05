package nvr

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

func setupMigrationTest(t *testing.T) (*MigrationManager, *db.DB, string) {
	t.Helper()
	tmpDir := t.TempDir()

	// Create a test database.
	dbPath := filepath.Join(tmpDir, "test.db")
	database, err := db.Open(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { database.Close() })

	// Create a test config file.
	configPath := filepath.Join(tmpDir, "mediamtx.yml")
	require.NoError(t, os.WriteFile(configPath, []byte("api: true\nnvr: true\n"), 0o600))

	backupDir := filepath.Join(tmpDir, "migration-backups")
	mgr := NewMigrationManager(database, configPath, dbPath, backupDir, "1.0.0")
	return mgr, database, tmpDir
}

func TestMigrationManager_Status(t *testing.T) {
	mgr, _, _ := setupMigrationTest(t)

	status, err := mgr.GetMigrationStatus()
	require.NoError(t, err)
	require.Equal(t, "1.0.0", status.AppVersion)
	require.Greater(t, status.SchemaVersion, 0)
	require.Empty(t, status.History)
	require.Nil(t, status.LastMigration)
	require.False(t, status.RollbackPossible)
}

func TestMigrationManager_PreUpgradeBackup(t *testing.T) {
	mgr, database, _ := setupMigrationTest(t)

	id, err := mgr.RunPreUpgradeBackup("1.0.0", "2.0.0")
	require.NoError(t, err)
	require.Greater(t, id, int64(0))

	// Verify the migration was recorded.
	migration, err := database.GetLatestUpgradeMigration()
	require.NoError(t, err)
	require.Equal(t, "1.0.0", migration.FromVersion)
	require.Equal(t, "2.0.0", migration.ToVersion)
	require.Equal(t, "pending", migration.Status)

	// Verify backup files exist.
	require.FileExists(t, migration.ConfigBackupPath)
	require.FileExists(t, migration.DBBackupPath)

	// Verify backup contents match original.
	origConfig, err := os.ReadFile(mgr.ConfigPath)
	require.NoError(t, err)
	backupConfig, err := os.ReadFile(migration.ConfigBackupPath)
	require.NoError(t, err)
	require.Equal(t, origConfig, backupConfig)
}

func TestMigrationManager_MarkCompleted(t *testing.T) {
	mgr, database, _ := setupMigrationTest(t)

	id, err := mgr.RunPreUpgradeBackup("1.0.0", "2.0.0")
	require.NoError(t, err)

	require.NoError(t, mgr.MarkCompleted(id))

	migration, err := database.GetLatestUpgradeMigration()
	require.NoError(t, err)
	require.Equal(t, "completed", migration.Status)
	require.NotEmpty(t, migration.CompletedAt)
}

func TestMigrationManager_Rollback(t *testing.T) {
	mgr, _, _ := setupMigrationTest(t)

	// Create pre-upgrade backup.
	id, err := mgr.RunPreUpgradeBackup("1.0.0", "2.0.0")
	require.NoError(t, err)

	// Simulate a config change during upgrade.
	require.NoError(t, os.WriteFile(mgr.ConfigPath, []byte("api: true\nnvr: true\nchanged: true\n"), 0o600))

	// Verify config was changed.
	data, err := os.ReadFile(mgr.ConfigPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "changed: true")

	// Rollback should restore original config.
	require.NoError(t, mgr.Rollback(id))

	data, err = os.ReadFile(mgr.ConfigPath)
	require.NoError(t, err)
	require.NotContains(t, string(data), "changed: true")
	require.Contains(t, string(data), "api: true")
}

func TestMigrationManager_MarkFailed_AutoRollback(t *testing.T) {
	mgr, _, _ := setupMigrationTest(t)

	id, err := mgr.RunPreUpgradeBackup("1.0.0", "2.0.0")
	require.NoError(t, err)

	// Change config to simulate failed upgrade.
	require.NoError(t, os.WriteFile(mgr.ConfigPath, []byte("broken config"), 0o600))

	// MarkFailed triggers automatic rollback.
	require.NoError(t, mgr.MarkFailed(id, "schema mismatch"))

	// Config should be restored.
	data, err := os.ReadFile(mgr.ConfigPath)
	require.NoError(t, err)
	require.Contains(t, string(data), "api: true")
}

func TestMigrationManager_Status_WithHistory(t *testing.T) {
	mgr, _, _ := setupMigrationTest(t)

	// Create and complete a migration.
	id, err := mgr.RunPreUpgradeBackup("1.0.0", "2.0.0")
	require.NoError(t, err)
	require.NoError(t, mgr.MarkCompleted(id))

	status, err := mgr.GetMigrationStatus()
	require.NoError(t, err)
	require.Len(t, status.History, 1)
	require.NotNil(t, status.LastMigration)
	require.Equal(t, "completed", status.LastMigration.Status)
	require.True(t, status.RollbackPossible)
}

func TestMigrationManager_Rollback_MissingBackup(t *testing.T) {
	mgr, _, _ := setupMigrationTest(t)

	id, err := mgr.RunPreUpgradeBackup("1.0.0", "2.0.0")
	require.NoError(t, err)

	// Delete backup files.
	migration, err := mgr.DB.GetLatestUpgradeMigration()
	require.NoError(t, err)
	os.Remove(migration.ConfigBackupPath)
	os.Remove(migration.DBBackupPath)

	// Rollback should fail.
	err = mgr.Rollback(id)
	require.Error(t, err)
	require.Contains(t, err.Error(), "backup files no longer exist")
}

func TestCopyFile(t *testing.T) {
	tmpDir := t.TempDir()
	src := filepath.Join(tmpDir, "src.txt")
	dst := filepath.Join(tmpDir, "sub", "dst.txt")

	require.NoError(t, os.WriteFile(src, []byte("hello world"), 0o600))
	require.NoError(t, copyFile(src, dst))

	data, err := os.ReadFile(dst)
	require.NoError(t, err)
	require.Equal(t, "hello world", string(data))
}
