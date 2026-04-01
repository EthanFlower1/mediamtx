package integrity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestQuarantineFile(t *testing.T) {
	dir := t.TempDir()
	recDir := filepath.Join(dir, "recordings", "cam1", "2026", "04")
	os.MkdirAll(recDir, 0o755)

	filePath := filepath.Join(recDir, "segment.mp4")
	os.WriteFile(filePath, []byte("test data"), 0o644)

	quarantineBase := filepath.Join(dir, "quarantine")
	recordingsBase := filepath.Join(dir, "recordings")

	newPath, err := QuarantineFile(filePath, recordingsBase, quarantineBase)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filePath); !os.IsNotExist(err) {
		t.Error("original file should not exist after quarantine")
	}

	if _, err := os.Stat(newPath); err != nil {
		t.Errorf("quarantined file should exist at %s: %v", newPath, err)
	}

	expected := filepath.Join(quarantineBase, "cam1", "2026", "04", "segment.mp4")
	if newPath != expected {
		t.Errorf("expected quarantine path %s, got %s", expected, newPath)
	}

	data, _ := os.ReadFile(newPath)
	if string(data) != "test data" {
		t.Errorf("quarantined file content mismatch")
	}
}

func TestUnquarantineFile(t *testing.T) {
	dir := t.TempDir()
	quarantineDir := filepath.Join(dir, "quarantine", "cam1")
	os.MkdirAll(quarantineDir, 0o755)

	quarantinePath := filepath.Join(quarantineDir, "segment.mp4")
	os.WriteFile(quarantinePath, []byte("test data"), 0o644)

	recordingsBase := filepath.Join(dir, "recordings")
	quarantineBase := filepath.Join(dir, "quarantine")

	restoredPath, err := UnquarantineFile(quarantinePath, quarantineBase, recordingsBase)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(quarantinePath); !os.IsNotExist(err) {
		t.Error("quarantined file should not exist after restore")
	}

	if _, err := os.Stat(restoredPath); err != nil {
		t.Errorf("restored file should exist at %s: %v", restoredPath, err)
	}

	expected := filepath.Join(recordingsBase, "cam1", "segment.mp4")
	if restoredPath != expected {
		t.Errorf("expected restored path %s, got %s", expected, restoredPath)
	}
}

func TestQuarantineFile_MissingSource(t *testing.T) {
	_, err := QuarantineFile("/nonexistent/file.mp4", "/recordings", "/quarantine")
	if err == nil {
		t.Error("expected error for missing source file")
	}
}
