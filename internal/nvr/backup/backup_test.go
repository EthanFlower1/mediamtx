package backup

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateAndRestoreBackup(t *testing.T) {
	// Set up temp directories.
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nvr.db")
	configPath := filepath.Join(tmpDir, "mediamtx.yml")
	backupDir := filepath.Join(tmpDir, "backups")
	restoreDir := filepath.Join(tmpDir, "restore")

	// Create fake source files.
	if err := os.WriteFile(dbPath, []byte("fake-database-content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte("fake-config-content"), 0o600); err != nil {
		t.Fatal(err)
	}

	// Create a cert file.
	if err := os.WriteFile(filepath.Join(tmpDir, "server.crt"), []byte("fake-cert"), 0o600); err != nil {
		t.Fatal(err)
	}

	svc := New(dbPath, configPath, backupDir)

	// Test creating a backup.
	filename, err := svc.CreateBackup("test-password-1234", false)
	if err != nil {
		t.Fatalf("CreateBackup: %v", err)
	}
	if filename == "" {
		t.Fatal("expected non-empty filename")
	}

	// Read the backup file.
	backupPath := filepath.Join(backupDir, filename)
	data, err := os.ReadFile(backupPath)
	if err != nil {
		t.Fatalf("read backup: %v", err)
	}

	// Test validation with wrong password.
	_, err = svc.ValidateBackup(data, "wrong-password")
	if err == nil {
		t.Fatal("expected error with wrong password")
	}

	// Test validation with correct password.
	files, err := svc.ValidateBackup(data, "test-password-1234")
	if err != nil {
		t.Fatalf("ValidateBackup: %v", err)
	}
	if len(files) < 2 {
		t.Fatalf("expected at least 2 files in backup, got %d: %v", len(files), files)
	}

	// Test listing backups.
	backups, err := svc.List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(backups) != 1 {
		t.Fatalf("expected 1 backup, got %d", len(backups))
	}
	if backups[0].Filename != filename {
		t.Fatalf("expected filename %q, got %q", filename, backups[0].Filename)
	}
	if backups[0].Auto {
		t.Fatal("expected manual backup, not auto")
	}

	// Test restore to a new location.
	restoreDBPath := filepath.Join(restoreDir, "nvr.db")
	restoreConfigPath := filepath.Join(restoreDir, "mediamtx.yml")
	if err := os.MkdirAll(restoreDir, 0o755); err != nil {
		t.Fatal(err)
	}

	restoreSvc := New(restoreDBPath, restoreConfigPath, backupDir)
	restored, err := restoreSvc.RestoreBackup(data, "test-password-1234")
	if err != nil {
		t.Fatalf("RestoreBackup: %v", err)
	}
	if len(restored) < 2 {
		t.Fatalf("expected at least 2 restored files, got %d: %v", len(restored), restored)
	}

	// Verify restored files exist and have correct content.
	dbContent, err := os.ReadFile(restoreDBPath)
	if err != nil {
		t.Fatalf("read restored db: %v", err)
	}
	if string(dbContent) != "fake-database-content" {
		t.Fatalf("unexpected db content: %q", dbContent)
	}

	configContent, err := os.ReadFile(restoreConfigPath)
	if err != nil {
		t.Fatalf("read restored config: %v", err)
	}
	if string(configContent) != "fake-config-content" {
		t.Fatalf("unexpected config content: %q", configContent)
	}

	// Test delete.
	if err := svc.DeleteBackup(filename); err != nil {
		t.Fatalf("DeleteBackup: %v", err)
	}
	backups, err = svc.List()
	if err != nil {
		t.Fatalf("List after delete: %v", err)
	}
	if len(backups) != 0 {
		t.Fatalf("expected 0 backups after delete, got %d", len(backups))
	}
}

func TestAutoBackupNaming(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nvr.db")
	configPath := filepath.Join(tmpDir, "mediamtx.yml")
	backupDir := filepath.Join(tmpDir, "backups")

	os.WriteFile(dbPath, []byte("db"), 0o600)
	os.WriteFile(configPath, []byte("cfg"), 0o600)

	svc := New(dbPath, configPath, backupDir)

	filename, err := svc.CreateBackup("password12345678", true)
	if err != nil {
		t.Fatalf("CreateBackup auto: %v", err)
	}
	if len(filename) < 15 {
		t.Fatalf("filename too short: %q", filename)
	}

	backups, err := svc.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(backups) != 1 || !backups[0].Auto {
		t.Fatalf("expected 1 auto backup, got %d", len(backups))
	}
}

func TestDirectoryTraversal(t *testing.T) {
	tmpDir := t.TempDir()
	svc := New("", "", filepath.Join(tmpDir, "backups"))

	_, err := svc.GetBackupPath("../../../etc/passwd")
	if err == nil {
		t.Fatal("expected error for directory traversal")
	}

	_, err = svc.GetBackupPath("backup.txt")
	if err == nil {
		t.Fatal("expected error for non-.enc extension")
	}
}

func TestPruneAutoBackups(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "nvr.db")
	configPath := filepath.Join(tmpDir, "mediamtx.yml")
	backupDir := filepath.Join(tmpDir, "backups")

	os.WriteFile(dbPath, []byte("db"), 0o600)
	os.WriteFile(configPath, []byte("cfg"), 0o600)

	svc := New(dbPath, configPath, backupDir)
	svc.schedule.maxKeep = 2

	// Create 4 auto backups.
	for i := 0; i < 4; i++ {
		_, err := svc.CreateBackup("password12345678", true)
		if err != nil {
			t.Fatal(err)
		}
		time.Sleep(10 * time.Millisecond) // ensure different timestamps
	}

	backups, _ := svc.List()
	autoCount := 0
	for _, b := range backups {
		if b.Auto {
			autoCount++
		}
	}
	if autoCount != 4 {
		t.Fatalf("expected 4 auto backups before prune, got %d", autoCount)
	}

	svc.pruneAutoBackups()

	backups, _ = svc.List()
	autoCount = 0
	for _, b := range backups {
		if b.Auto {
			autoCount++
		}
	}
	if autoCount != 2 {
		t.Fatalf("expected 2 auto backups after prune, got %d", autoCount)
	}
}
