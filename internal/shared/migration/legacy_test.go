package migration_test

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/bluenviron/mediamtx/internal/shared/migration"
)

// createNVRDB creates a minimal nvr.db with a cameras table (directory-owned)
// and a recordings table (recorder-owned) and inserts one row into each.
func createNVRDB(t *testing.T, dir string) {
	t.Helper()

	db, err := sql.Open("sqlite", filepath.Join(dir, "nvr.db"))
	if err != nil {
		t.Fatalf("open nvr.db: %v", err)
	}
	defer db.Close()

	_, err = db.Exec(`CREATE TABLE cameras (
		id   INTEGER PRIMARY KEY,
		name TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create cameras: %v", err)
	}

	_, err = db.Exec(`CREATE TABLE recordings (
		id          INTEGER PRIMARY KEY,
		camera_id   INTEGER NOT NULL,
		started_at  TEXT NOT NULL
	)`)
	if err != nil {
		t.Fatalf("create recordings: %v", err)
	}

	if _, err = db.Exec(`INSERT INTO cameras (id, name) VALUES (1, 'front-door')`); err != nil {
		t.Fatalf("insert camera: %v", err)
	}
	if _, err = db.Exec(`INSERT INTO recordings (id, camera_id, started_at) VALUES (1, 1, '2026-01-01T00:00:00Z')`); err != nil {
		t.Fatalf("insert recording: %v", err)
	}
}

func TestMigrateLegacyDB(t *testing.T) {
	dir := t.TempDir()
	createNVRDB(t, dir)

	result, err := migration.MigrateLegacyDB(dir, nil)
	if err != nil {
		t.Fatalf("MigrateLegacyDB: %v", err)
	}

	if result.Skipped {
		t.Fatal("expected migration to run, but it was skipped")
	}

	// directory.db should have cameras with 1 row.
	if n, ok := result.DirectoryRows["cameras"]; !ok || n != 1 {
		t.Errorf("expected cameras row count=1, got %v (present=%v)", result.DirectoryRows["cameras"], ok)
	}

	// recorder.db should have recordings with 1 row.
	if n, ok := result.RecorderRows["recordings"]; !ok || n != 1 {
		t.Errorf("expected recordings row count=1, got %v (present=%v)", result.RecorderRows["recordings"], ok)
	}

	// nvr.db should be renamed to nvr.db.backup.
	if _, err := os.Stat(filepath.Join(dir, "nvr.db")); err == nil {
		t.Error("nvr.db should have been renamed, but it still exists")
	}
	if _, err := os.Stat(filepath.Join(dir, "nvr.db.backup")); os.IsNotExist(err) {
		t.Error("nvr.db.backup should exist after migration")
	}

	// directory.db should contain the cameras row.
	dirDB, err := sql.Open("sqlite", filepath.Join(dir, "directory.db"))
	if err != nil {
		t.Fatalf("open directory.db: %v", err)
	}
	defer dirDB.Close()

	var camName string
	if err := dirDB.QueryRow("SELECT name FROM cameras WHERE id=1").Scan(&camName); err != nil {
		t.Fatalf("query directory.db cameras: %v", err)
	}
	if camName != "front-door" {
		t.Errorf("expected camera name 'front-door', got %q", camName)
	}

	// recorder.db should contain the recordings row.
	recDB, err := sql.Open("sqlite", filepath.Join(dir, "recorder.db"))
	if err != nil {
		t.Fatalf("open recorder.db: %v", err)
	}
	defer recDB.Close()

	var cameraID int
	if err := recDB.QueryRow("SELECT camera_id FROM recordings WHERE id=1").Scan(&cameraID); err != nil {
		t.Fatalf("query recorder.db recordings: %v", err)
	}
	if cameraID != 1 {
		t.Errorf("expected camera_id=1, got %d", cameraID)
	}
}

func TestMigrateLegacyDB_IdempotentWhenDirectoryDBExists(t *testing.T) {
	dir := t.TempDir()
	createNVRDB(t, dir)

	// Pre-create directory.db to simulate already-migrated state.
	f, err := os.Create(filepath.Join(dir, "directory.db"))
	if err != nil {
		t.Fatal(err)
	}
	f.Close()

	result, err := migration.MigrateLegacyDB(dir, nil)
	if err != nil {
		t.Fatalf("MigrateLegacyDB: %v", err)
	}

	if !result.Skipped {
		t.Error("expected migration to be skipped when directory.db already exists")
	}

	// nvr.db should still exist (not renamed).
	if _, err := os.Stat(filepath.Join(dir, "nvr.db")); os.IsNotExist(err) {
		t.Error("nvr.db should not have been renamed when skipping")
	}
}

func TestMigrateLegacyDB_NoNVRDB(t *testing.T) {
	dir := t.TempDir()
	// Don't create nvr.db at all.

	result, err := migration.MigrateLegacyDB(dir, nil)
	if err != nil {
		t.Fatalf("MigrateLegacyDB: %v", err)
	}
	if !result.Skipped {
		t.Error("expected migration to be skipped when nvr.db is absent")
	}
}
