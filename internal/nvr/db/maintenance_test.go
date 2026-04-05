package db

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCheckIntegrity(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	ok, msg, err := d.CheckIntegrity()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("expected integrity OK, got: %s", msg)
	}
	if msg != "ok" {
		t.Fatalf("expected 'ok', got: %s", msg)
	}
}

func TestQuickIntegrityCheck(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	ok, msg, err := d.QuickIntegrityCheck()
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatalf("expected quick_check OK, got: %s", msg)
	}
}

func TestWALCheckpoint(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// Insert some data to generate WAL entries.
	_, err = d.Exec("CREATE TABLE IF NOT EXISTS wal_test (id INTEGER PRIMARY KEY, val TEXT)")
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 100; i++ {
		_, err = d.Exec("INSERT INTO wal_test (val) VALUES (?)", "test")
		if err != nil {
			t.Fatal(err)
		}
	}

	walFrames, checkpointed, err := d.WALCheckpoint()
	if err != nil {
		t.Fatal(err)
	}
	// After checkpoint, values should be non-negative.
	if walFrames < 0 || checkpointed < 0 {
		t.Fatalf("unexpected checkpoint result: walFrames=%d checkpointed=%d", walFrames, checkpointed)
	}
}

func TestVacuum(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	d, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	// Create and fill a table, then delete data, then vacuum.
	_, _ = d.Exec("CREATE TABLE IF NOT EXISTS vacuum_test (id INTEGER PRIMARY KEY, data TEXT)")
	for i := 0; i < 500; i++ {
		_, _ = d.Exec("INSERT INTO vacuum_test (data) VALUES (?)", "padding data for vacuum test")
	}
	// Checkpoint to flush to main DB file.
	_, _, _ = d.WALCheckpoint()

	info1, _ := os.Stat(dbPath)
	sizeBefore := info1.Size()

	_, _ = d.Exec("DELETE FROM vacuum_test")
	_, _, _ = d.WALCheckpoint()

	if err := d.Vacuum(); err != nil {
		t.Fatal(err)
	}

	info2, _ := os.Stat(dbPath)
	sizeAfter := info2.Size()

	if sizeAfter >= sizeBefore {
		t.Logf("VACUUM did not reduce file size (before=%d, after=%d) - may be expected for small DBs", sizeBefore, sizeAfter)
	}
}

func TestGetDBHealth(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	health, err := d.GetDBHealth()
	if err != nil {
		t.Fatal(err)
	}
	if !health.IntegrityOK {
		t.Fatalf("expected integrity OK, got: %s", health.IntegrityMsg)
	}
	if health.PageCount <= 0 {
		t.Fatalf("expected positive page count, got %d", health.PageCount)
	}
	if health.PageSize <= 0 {
		t.Fatalf("expected positive page size, got %d", health.PageSize)
	}
	if health.FileSizeBytes <= 0 {
		t.Fatalf("expected positive file size, got %d", health.FileSizeBytes)
	}
}

func TestMaintenanceRunner(t *testing.T) {
	dir := t.TempDir()
	d, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()

	alerts := make(chan string, 10)
	cfg := MaintenanceConfig{
		WALCheckpointInterval: 100 * time.Millisecond,
		VacuumHour:            -1, // disable scheduled vacuum for test
		VacuumDay:             -1,
	}

	runner := d.StartMaintenance(cfg, func(alertType, message string) {
		alerts <- alertType + ": " + message
	})

	// Wait for at least one checkpoint cycle.
	time.Sleep(300 * time.Millisecond)

	runner.Stop()

	// Verify that a checkpoint timestamp was recorded.
	ts, err := d.GetConfig("maintenance_last_checkpoint")
	if err != nil {
		t.Fatalf("expected checkpoint timestamp in config, got error: %v", err)
	}
	if ts == "" {
		t.Fatal("expected non-empty checkpoint timestamp")
	}

	// No alerts should have been fired for a healthy DB.
	select {
	case alert := <-alerts:
		t.Fatalf("unexpected alert: %s", alert)
	default:
		// Good - no alerts.
	}
}

func TestDefaultMaintenanceConfig(t *testing.T) {
	cfg := DefaultMaintenanceConfig()
	if cfg.WALCheckpointInterval != 4*time.Hour {
		t.Fatalf("expected 4h WAL interval, got %v", cfg.WALCheckpointInterval)
	}
	if cfg.VacuumHour != 3 {
		t.Fatalf("expected vacuum hour 3, got %d", cfg.VacuumHour)
	}
	if cfg.VacuumDay != 0 {
		t.Fatalf("expected vacuum day 0, got %d", cfg.VacuumDay)
	}
}
