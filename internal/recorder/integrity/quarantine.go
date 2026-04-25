package integrity

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// QuarantineFile moves a recording file to the quarantine directory,
// preserving its relative path structure. Returns the new file path.
func QuarantineFile(filePath, recordingsBase, quarantineBase string) (string, error) {
	if _, err := os.Stat(filePath); err != nil {
		return "", fmt.Errorf("source file not found: %w", err)
	}

	relPath, err := filepath.Rel(recordingsBase, filePath)
	if err != nil {
		relPath = filepath.Base(filePath)
	}
	if strings.HasPrefix(relPath, "..") {
		relPath = filepath.Base(filePath)
	}

	destPath := filepath.Join(quarantineBase, relPath)
	destDir := filepath.Dir(destPath)

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create quarantine directory: %w", err)
	}

	if err := os.Rename(filePath, destPath); err != nil {
		return "", fmt.Errorf("move file to quarantine: %w", err)
	}

	return destPath, nil
}

// UnquarantineFile moves a quarantined file back to the recordings directory.
// Returns the restored file path.
func UnquarantineFile(quarantinePath, quarantineBase, recordingsBase string) (string, error) {
	if _, err := os.Stat(quarantinePath); err != nil {
		return "", fmt.Errorf("quarantined file not found: %w", err)
	}

	relPath, err := filepath.Rel(quarantineBase, quarantinePath)
	if err != nil {
		relPath = filepath.Base(quarantinePath)
	}
	if strings.HasPrefix(relPath, "..") {
		relPath = filepath.Base(quarantinePath)
	}

	destPath := filepath.Join(recordingsBase, relPath)
	destDir := filepath.Dir(destPath)

	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return "", fmt.Errorf("create recordings directory: %w", err)
	}

	if err := os.Rename(quarantinePath, destPath); err != nil {
		return "", fmt.Errorf("move file from quarantine: %w", err)
	}

	return destPath, nil
}
