// Package updater provides system update checking, downloading, applying,
// and rollback functionality for the MediaMTX NVR.
package updater

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
)

// ReleaseInfo describes a version available for update.
type ReleaseInfo struct {
	Version     string `json:"version"`
	ReleaseDate string `json:"release_date"`
	DownloadURL string `json:"download_url"`
	SHA256      string `json:"sha256"`
	ReleaseNotes string `json:"release_notes"`
	Size        int64  `json:"size"`
}

// CheckResult is the result of checking for updates.
type CheckResult struct {
	UpdateAvailable bool         `json:"update_available"`
	CurrentVersion  string       `json:"current_version"`
	LatestVersion   string       `json:"latest_version"`
	Release         *ReleaseInfo `json:"release,omitempty"`
	CheckedAt       string       `json:"checked_at"`
}

// ApplyResult is the result of applying an update.
type ApplyResult struct {
	Success   bool   `json:"success"`
	Message   string `json:"message"`
	RecordID  int64  `json:"record_id,omitempty"`
	FromVersion string `json:"from_version,omitempty"`
	ToVersion   string `json:"to_version,omitempty"`
}

// githubRelease is a subset of the GitHub releases API response.
type githubRelease struct {
	TagName     string `json:"tag_name"`
	PublishedAt string `json:"published_at"`
	Body        string `json:"body"`
	Assets      []struct {
		Name               string `json:"name"`
		BrowserDownloadURL string `json:"browser_download_url"`
		Size               int64  `json:"size"`
	} `json:"assets"`
}

// Manager handles update checking, downloading, and applying.
type Manager struct {
	DB             *db.DB
	CurrentVersion string
	BinaryPath     string // path to the currently running binary
	BackupDir      string // directory for storing backup binaries

	// configurable for testing
	GitHubOwner string
	GitHubRepo  string
	HTTPClient  *http.Client

	mu          sync.Mutex
	lastCheck   *CheckResult
	applying    bool
}

// New creates a new update Manager.
func New(database *db.DB, currentVersion string) *Manager {
	execPath, _ := os.Executable()
	if execPath == "" {
		execPath = os.Args[0]
	}

	return &Manager{
		DB:             database,
		CurrentVersion: currentVersion,
		BinaryPath:     execPath,
		BackupDir:      filepath.Join(filepath.Dir(execPath), ".updates"),
		GitHubOwner:    "bluenviron",
		GitHubRepo:     "mediamtx",
		HTTPClient:     &http.Client{Timeout: 30 * time.Second},
	}
}

// Check queries the GitHub releases API for the latest version and compares
// it to the current running version.
func (m *Manager) Check() (*CheckResult, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest",
		m.GitHubOwner, m.GitHubRepo)

	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "MediaMTX-NVR/"+m.CurrentVersion)

	resp, err := m.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("fetch latest release: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return nil, fmt.Errorf("decode release: %w", err)
	}

	latestVersion := strings.TrimPrefix(release.TagName, "v")
	currentVersion := strings.TrimPrefix(m.CurrentVersion, "v")
	updateAvailable := latestVersion != currentVersion && latestVersion != ""

	result := &CheckResult{
		UpdateAvailable: updateAvailable,
		CurrentVersion:  m.CurrentVersion,
		LatestVersion:   release.TagName,
		CheckedAt:       time.Now().UTC().Format(time.RFC3339),
	}

	if updateAvailable {
		ri := &ReleaseInfo{
			Version:      release.TagName,
			ReleaseDate:  release.PublishedAt,
			ReleaseNotes: release.Body,
		}

		// Find the matching asset for the current platform.
		assetName := m.expectedAssetName(release.TagName)
		for _, asset := range release.Assets {
			if asset.Name == assetName {
				ri.DownloadURL = asset.BrowserDownloadURL
				ri.Size = asset.Size
				break
			}
		}

		// Look for the checksum file.
		for _, asset := range release.Assets {
			if strings.HasSuffix(asset.Name, "_checksums.txt") || asset.Name == "SHA256SUMS" {
				sha, err := m.fetchChecksum(asset.BrowserDownloadURL, assetName)
				if err == nil {
					ri.SHA256 = sha
				}
				break
			}
		}

		result.Release = ri
	}

	m.mu.Lock()
	m.lastCheck = result
	m.mu.Unlock()

	return result, nil
}

// LastCheck returns the cached result from the most recent check, or nil.
func (m *Manager) LastCheck() *CheckResult {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.lastCheck
}

// Apply downloads and installs the specified version. It:
// 1. Downloads the binary from the release URL
// 2. Verifies the SHA-256 checksum
// 3. Backs up the current binary
// 4. Replaces the current binary
// 5. Records the update in the database
//
// The caller is responsible for restarting the service after a successful apply.
func (m *Manager) Apply(release *ReleaseInfo, initiatedBy string) (*ApplyResult, error) {
	m.mu.Lock()
	if m.applying {
		m.mu.Unlock()
		return nil, fmt.Errorf("an update is already in progress")
	}
	m.applying = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.applying = false
		m.mu.Unlock()
	}()

	if release == nil || release.DownloadURL == "" {
		return &ApplyResult{Success: false, Message: "no download URL available for this release"}, nil
	}

	// Record the update attempt.
	rec := &db.UpdateRecord{
		FromVersion: m.CurrentVersion,
		ToVersion:   release.Version,
		Status:      "downloading",
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
		InitiatedBy: initiatedBy,
	}
	recordID, err := m.DB.InsertUpdateRecord(rec)
	if err != nil {
		return nil, fmt.Errorf("record update start: %w", err)
	}

	result := &ApplyResult{
		RecordID:    recordID,
		FromVersion: m.CurrentVersion,
		ToVersion:   release.Version,
	}

	// Create backup directory.
	if err := os.MkdirAll(m.BackupDir, 0o755); err != nil {
		m.failUpdate(recordID, "failed to create backup directory: "+err.Error())
		result.Message = "failed to create backup directory"
		return result, nil
	}

	// Download the new binary.
	log.Printf("[NVR] [INFO] [updater] downloading update %s from %s", release.Version, release.DownloadURL)
	tmpFile, err := m.downloadBinary(release.DownloadURL)
	if err != nil {
		m.failUpdate(recordID, "download failed: "+err.Error())
		result.Message = "download failed: " + err.Error()
		return result, nil
	}
	defer os.Remove(tmpFile)

	// Verify checksum if available.
	if release.SHA256 != "" {
		if err := m.DB.UpdateUpdateRecord(recordID, "verifying", "", false); err != nil {
			log.Printf("[NVR] [WARN] [updater] failed to update status: %v", err)
		}

		actualHash, err := fileSHA256(tmpFile)
		if err != nil {
			m.failUpdate(recordID, "checksum computation failed: "+err.Error())
			result.Message = "checksum computation failed"
			return result, nil
		}

		if !strings.EqualFold(actualHash, release.SHA256) {
			msg := fmt.Sprintf("checksum mismatch: expected %s, got %s", release.SHA256, actualHash)
			m.failUpdate(recordID, msg)
			result.Message = msg
			return result, nil
		}
		log.Printf("[NVR] [INFO] [updater] checksum verified: %s", actualHash)

		// Store verified checksum.
		rec.SHA256Checksum = actualHash
	}

	// Backup current binary.
	if err := m.DB.UpdateUpdateRecord(recordID, "applying", "", false); err != nil {
		log.Printf("[NVR] [WARN] [updater] failed to update status: %v", err)
	}

	backupPath := filepath.Join(m.BackupDir, fmt.Sprintf("mediamtx.%s.bak", strings.TrimPrefix(m.CurrentVersion, "v")))
	if err := copyFile(m.BinaryPath, backupPath); err != nil {
		m.failUpdate(recordID, "backup failed: "+err.Error())
		result.Message = "failed to backup current binary"
		return result, nil
	}
	log.Printf("[NVR] [INFO] [updater] backed up current binary to %s", backupPath)

	// Replace binary.
	if err := copyFile(tmpFile, m.BinaryPath); err != nil {
		// Try to restore backup.
		log.Printf("[NVR] [ERROR] [updater] failed to replace binary: %v, restoring backup", err)
		if restoreErr := copyFile(backupPath, m.BinaryPath); restoreErr != nil {
			msg := fmt.Sprintf("CRITICAL: binary replacement failed and restore failed: replace=%v restore=%v", err, restoreErr)
			m.failUpdate(recordID, msg)
			result.Message = msg
			return result, nil
		}
		m.failUpdate(recordID, "binary replacement failed, backup restored: "+err.Error())
		result.Message = "binary replacement failed, backup restored"
		return result, nil
	}

	// Make binary executable.
	if err := os.Chmod(m.BinaryPath, 0o755); err != nil {
		log.Printf("[NVR] [WARN] [updater] failed to chmod binary: %v", err)
	}

	// Mark as completed.
	if err := m.DB.UpdateUpdateRecord(recordID, "completed", "", true); err != nil {
		log.Printf("[NVR] [WARN] [updater] failed to record completion: %v", err)
	}

	result.Success = true
	result.Message = fmt.Sprintf("update to %s applied successfully; restart the service to use the new version", release.Version)
	log.Printf("[NVR] [INFO] [updater] %s", result.Message)

	return result, nil
}

// Rollback restores the previous binary from backup.
func (m *Manager) Rollback(initiatedBy string) (*ApplyResult, error) {
	m.mu.Lock()
	if m.applying {
		m.mu.Unlock()
		return nil, fmt.Errorf("an update is already in progress")
	}
	m.applying = true
	m.mu.Unlock()

	defer func() {
		m.mu.Lock()
		m.applying = false
		m.mu.Unlock()
	}()

	// Find the latest completed update with a rollback available.
	records, err := m.DB.ListUpdateHistory(10)
	if err != nil {
		return nil, fmt.Errorf("list update history: %w", err)
	}

	var targetRecord *db.UpdateRecord
	for _, r := range records {
		if r.RollbackAvailable && (r.Status == "completed" || r.Status == "failed") {
			targetRecord = r
			break
		}
	}

	if targetRecord == nil {
		return &ApplyResult{Success: false, Message: "no rollback available"}, nil
	}

	backupPath := filepath.Join(m.BackupDir, fmt.Sprintf("mediamtx.%s.bak", strings.TrimPrefix(targetRecord.FromVersion, "v")))
	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		return &ApplyResult{Success: false, Message: "backup binary not found at " + backupPath}, nil
	}

	// Record the rollback.
	rollbackRec := &db.UpdateRecord{
		FromVersion: m.CurrentVersion,
		ToVersion:   targetRecord.FromVersion,
		Status:      "applying",
		StartedAt:   time.Now().UTC().Format(time.RFC3339),
		InitiatedBy: initiatedBy,
	}
	recordID, err := m.DB.InsertUpdateRecord(rollbackRec)
	if err != nil {
		return nil, fmt.Errorf("record rollback: %w", err)
	}

	// Restore the backup.
	if err := copyFile(backupPath, m.BinaryPath); err != nil {
		m.failUpdate(recordID, "rollback failed: "+err.Error())
		return &ApplyResult{
			Success:  false,
			Message:  "rollback failed: " + err.Error(),
			RecordID: recordID,
		}, nil
	}

	if err := os.Chmod(m.BinaryPath, 0o755); err != nil {
		log.Printf("[NVR] [WARN] [updater] failed to chmod binary: %v", err)
	}

	// Mark the original update as rolled back.
	if err := m.DB.UpdateUpdateRecord(int64(targetRecord.ID), "rolled_back", "", false); err != nil {
		log.Printf("[NVR] [WARN] [updater] failed to update original record: %v", err)
	}

	// Mark rollback as completed.
	if err := m.DB.UpdateUpdateRecord(recordID, "completed", "", false); err != nil {
		log.Printf("[NVR] [WARN] [updater] failed to record rollback completion: %v", err)
	}

	log.Printf("[NVR] [INFO] [updater] rolled back from %s to %s", m.CurrentVersion, targetRecord.FromVersion)

	return &ApplyResult{
		Success:     true,
		Message:     fmt.Sprintf("rolled back to %s; restart the service to use the restored version", targetRecord.FromVersion),
		RecordID:    recordID,
		FromVersion: m.CurrentVersion,
		ToVersion:   targetRecord.FromVersion,
	}, nil
}

// failUpdate marks an update record as failed.
func (m *Manager) failUpdate(recordID int64, errMsg string) {
	log.Printf("[NVR] [ERROR] [updater] update failed: %s", errMsg)
	if err := m.DB.UpdateUpdateRecord(recordID, "failed", errMsg, false); err != nil {
		log.Printf("[NVR] [WARN] [updater] failed to record failure: %v", err)
	}
}

// downloadBinary downloads a file to a temp location and returns the path.
func (m *Manager) downloadBinary(url string) (string, error) {
	resp, err := m.HTTPClient.Get(url)
	if err != nil {
		return "", fmt.Errorf("download: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download returned status %d", resp.StatusCode)
	}

	tmpFile, err := os.CreateTemp("", "mediamtx-update-*")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}

	if _, err := io.Copy(tmpFile, resp.Body); err != nil {
		tmpFile.Close()
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("write temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("close temp file: %w", err)
	}

	return tmpFile.Name(), nil
}

// expectedAssetName returns the expected binary asset name for the current platform.
func (m *Manager) expectedAssetName(version string) string {
	v := strings.TrimPrefix(version, "v")
	goarch := runtime.GOARCH
	if goarch == "amd64" {
		goarch = "amd64"
	}
	return fmt.Sprintf("mediamtx_%s_%s_%s.tar.gz", v, runtime.GOOS, goarch)
}

// fetchChecksum downloads a checksum file and extracts the hash for a given filename.
func (m *Manager) fetchChecksum(checksumURL, filename string) (string, error) {
	resp, err := m.HTTPClient.Get(checksumURL)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}

	for _, line := range strings.Split(string(body), "\n") {
		parts := strings.Fields(line)
		if len(parts) == 2 && parts[1] == filename {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("checksum not found for %s", filename)
}

// fileSHA256 computes the SHA-256 hash of a file.
func fileSHA256(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyFile copies src to dst, preserving permissions.
func copyFile(src, dst string) error {
	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	srcInfo, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, srcInfo.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	_, err = io.Copy(dstFile, srcFile)
	return err
}
