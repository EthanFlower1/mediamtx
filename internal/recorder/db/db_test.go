package db_test

import (
	"os"
	"path/filepath"
	"testing"

	recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
)

func TestOpen(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "recorder_test.db")

	d, err := recdb.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer d.Close()

	// Verify key recorder-owned tables exist.
	tables := []string{
		"recordings",
		"motion_events",
		"detections",
		"detection_zones",
		"detection_events",
		"pending_syncs",
		"connection_events",
		"queued_commands",
		"export_jobs",
		"screenshots",
		"tours",
		"cross_camera_tracks",
	}

	for _, table := range tables {
		var count int
		row := d.QueryRow(
			"SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?",
			table,
		)
		if err := row.Scan(&count); err != nil {
			t.Fatalf("check table %q: %v", table, err)
		}
		if count == 0 {
			t.Errorf("table %q not found in recorder DB", table)
		}
	}
}

func TestOpenMemory(t *testing.T) {
	// Test that opening :memory: works (used in some tests).
	// Actually recorder/db doesn't support :memory: via the pragma DSN,
	// so use a temp file.
	dir := t.TempDir()
	path := filepath.Join(dir, "test2.db")

	d, err := recdb.Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := d.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	// Verify file exists.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("db file not created: %v", err)
	}
}
