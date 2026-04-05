package db

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOpen(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	defer d.Close()

	// Verify we can ping the database.
	require.NoError(t, d.Ping())
}

func TestOpenRunsMigrations(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	defer d.Close()

	// All tables created by migrations should be queryable.
	tables := []string{
		"cameras",
		"recordings",
		"users",
		"refresh_tokens",
		"config",
		"schema_migrations",
		"recording_rules",
		"audit_log",
		"motion_events",
		"saved_clips",
		"detections",
		"pending_syncs",
		"schedule_templates",
		"screenshots",
		"connection_events",
		"queued_commands",
		"export_jobs",
		"evidence_exports",
		"update_history",
		"bulk_export_jobs",
		"bulk_export_items",
		"smtp_config",
		"alert_rules",
		"alerts",
	}

	for _, table := range tables {
		var n int
		err := d.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&n)
		require.NoError(t, err, "table %s should exist", table)
	}

	// Verify migration version was recorded.
	var version int
	err = d.QueryRow("SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&version)
	require.NoError(t, err)
	require.Equal(t, 43, version)
}

func TestOpenWALMode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	d, err := Open(dbPath)
	require.NoError(t, err)
	defer d.Close()

	var mode string
	err = d.QueryRow("PRAGMA journal_mode").Scan(&mode)
	require.NoError(t, err)
	require.Equal(t, "wal", mode)
}
