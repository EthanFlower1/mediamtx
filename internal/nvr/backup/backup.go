// Package backup provides configuration backup and restore for the NVR subsystem.
// Backups are encrypted zip archives containing the SQLite database, mediamtx.yml,
// and any TLS certificates/keys found in the config directory.
package backup

import (
	"archive/zip"
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"golang.org/x/crypto/hkdf"
)

// deriveKeyFromPassword derives a 32-byte AES-256 key from a user password
// using HKDF-SHA256 with a fixed salt. The salt is not secret — HKDF security
// comes from the password entropy plus the info parameter.
func deriveKeyFromPassword(password string) []byte {
	salt := []byte("mediamtx-nvr-backup-v1")
	reader := hkdf.New(sha256.New, []byte(password), salt, []byte("backup-encryption"))
	key := make([]byte, 32)
	_, _ = reader.Read(key)
	return key
}

// encryptData encrypts plaintext with AES-256-GCM using a password-derived key.
// Returns nonce + ciphertext.
func encryptData(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// decryptData decrypts ciphertext produced by encryptData.
func decryptData(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt failed (wrong password?): %w", err)
	}
	return plaintext, nil
}

// BackupInfo describes a backup file on disk.
type BackupInfo struct {
	Filename  string    `json:"filename"`
	Size      int64     `json:"size"`
	CreatedAt time.Time `json:"created_at"`
	Auto      bool      `json:"auto"`
}

// Service manages backup creation, restoration, scheduling, and listing.
type Service struct {
	DatabasePath string // path to nvr.db
	ConfigPath   string // path to mediamtx.yml
	BackupDir    string // directory for storing backups

	mu       sync.Mutex
	stopCh   chan struct{}
	schedule struct {
		enabled  bool
		interval time.Duration
		password string
		maxKeep  int
	}
}

// New creates a new backup service.
func New(databasePath, configPath, backupDir string) *Service {
	return &Service{
		DatabasePath: databasePath,
		ConfigPath:   configPath,
		BackupDir:    backupDir,
	}
}

// Init creates the backup directory if it does not exist.
func (s *Service) Init() error {
	if s.BackupDir == "" {
		return fmt.Errorf("backup directory not configured")
	}
	return os.MkdirAll(s.BackupDir, 0o700)
}

// CreateBackup creates an encrypted zip backup and writes it to the backup directory.
// Returns the filename and any error.
func (s *Service) CreateBackup(password string, auto bool) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := s.Init(); err != nil {
		return "", fmt.Errorf("init backup dir: %w", err)
	}

	key := deriveKeyFromPassword(password)

	// Collect files to back up.
	files := make(map[string][]byte)

	// 1. SQLite database — use a file copy (safe with WAL since we hold the mutex).
	if s.DatabasePath != "" {
		data, err := os.ReadFile(s.DatabasePath)
		if err != nil {
			return "", fmt.Errorf("read database: %w", err)
		}
		files["nvr.db"] = data

		// Also include WAL and SHM if they exist — they may contain uncommitted data.
		for _, suffix := range []string{"-wal", "-shm"} {
			walPath := s.DatabasePath + suffix
			if data, err := os.ReadFile(walPath); err == nil {
				files["nvr.db"+suffix] = data
			}
		}
	}

	// 2. mediamtx.yml config file.
	if s.ConfigPath != "" {
		data, err := os.ReadFile(s.ConfigPath)
		if err != nil {
			log.Printf("[NVR] [WARN] [backup] could not read config: %v", err)
		} else {
			files["mediamtx.yml"] = data
		}
	}

	// 3. TLS certificates and keys from the config directory.
	if s.ConfigPath != "" {
		configDir := filepath.Dir(s.ConfigPath)
		certPatterns := []string{"*.pem", "*.crt", "*.key", "*.cert"}
		for _, pattern := range certPatterns {
			matches, _ := filepath.Glob(filepath.Join(configDir, pattern))
			for _, match := range matches {
				data, err := os.ReadFile(match)
				if err != nil {
					continue
				}
				relPath := filepath.Base(match)
				files["certs/"+relPath] = data
			}
		}
	}

	if len(files) == 0 {
		return "", fmt.Errorf("no files to back up")
	}

	// Build an in-memory zip.
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)
	for name, data := range files {
		fw, err := zw.Create(name)
		if err != nil {
			return "", fmt.Errorf("create zip entry %s: %w", name, err)
		}
		if _, err := fw.Write(data); err != nil {
			return "", fmt.Errorf("write zip entry %s: %w", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("close zip: %w", err)
	}

	// Encrypt the zip.
	encrypted, err := encryptData(key, zipBuf.Bytes())
	if err != nil {
		return "", fmt.Errorf("encrypt backup: %w", err)
	}

	// Write to disk with a timestamp filename including nanoseconds for uniqueness.
	prefix := "backup"
	if auto {
		prefix = "auto-backup"
	}
	now := time.Now().UTC()
	filename := fmt.Sprintf("%s-%s-%03d.enc", prefix, now.Format("20060102-150405"), now.Nanosecond()/1e6)
	outPath := filepath.Join(s.BackupDir, filename)

	if err := os.WriteFile(outPath, encrypted, 0o600); err != nil {
		return "", fmt.Errorf("write backup file: %w", err)
	}

	log.Printf("[NVR] [INFO] [backup] created %s (%d bytes)", filename, len(encrypted))
	return filename, nil
}

// ValidateBackup decrypts and validates a backup archive without applying it.
// Returns the list of files contained in the archive.
func (s *Service) ValidateBackup(data []byte, password string) ([]string, error) {
	key := deriveKeyFromPassword(password)

	plaintext, err := decryptData(key, data)
	if err != nil {
		return nil, fmt.Errorf("decrypt backup: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(plaintext), int64(len(plaintext)))
	if err != nil {
		return nil, fmt.Errorf("read zip: %w", err)
	}

	var fileNames []string
	for _, f := range reader.File {
		fileNames = append(fileNames, f.Name)
	}

	return fileNames, nil
}

// RestoreBackup decrypts, validates, and applies a backup archive.
// The database and config files are replaced on disk.
// The caller should restart the NVR after a successful restore.
func (s *Service) RestoreBackup(data []byte, password string) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := deriveKeyFromPassword(password)

	plaintext, err := decryptData(key, data)
	if err != nil {
		return nil, fmt.Errorf("decrypt backup: %w", err)
	}

	reader, err := zip.NewReader(bytes.NewReader(plaintext), int64(len(plaintext)))
	if err != nil {
		return nil, fmt.Errorf("read zip: %w", err)
	}

	// First pass: validate — make sure the archive looks reasonable.
	var hasDB, hasConfig bool
	for _, f := range reader.File {
		if f.Name == "nvr.db" {
			hasDB = true
		}
		if f.Name == "mediamtx.yml" {
			hasConfig = true
		}
	}
	if !hasDB && !hasConfig {
		return nil, fmt.Errorf("backup archive contains neither database nor config — invalid backup")
	}

	// Second pass: extract files.
	var restored []string
	for _, f := range reader.File {
		rc, err := f.Open()
		if err != nil {
			return restored, fmt.Errorf("open archive entry %s: %w", f.Name, err)
		}
		content, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			return restored, fmt.Errorf("read archive entry %s: %w", f.Name, err)
		}

		var destPath string
		switch {
		case f.Name == "nvr.db" || f.Name == "nvr.db-wal" || f.Name == "nvr.db-shm":
			if s.DatabasePath == "" {
				continue
			}
			if f.Name == "nvr.db" {
				destPath = s.DatabasePath
			} else {
				destPath = s.DatabasePath + strings.TrimPrefix(f.Name, "nvr.db")
			}
		case f.Name == "mediamtx.yml":
			if s.ConfigPath == "" {
				continue
			}
			destPath = s.ConfigPath
		case strings.HasPrefix(f.Name, "certs/"):
			if s.ConfigPath == "" {
				continue
			}
			certName := strings.TrimPrefix(f.Name, "certs/")
			destPath = filepath.Join(filepath.Dir(s.ConfigPath), certName)
		default:
			// Unknown file — skip.
			continue
		}

		// Create parent directory if needed.
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return restored, fmt.Errorf("create dir for %s: %w", f.Name, err)
		}

		if err := os.WriteFile(destPath, content, 0o600); err != nil {
			return restored, fmt.Errorf("write %s: %w", f.Name, err)
		}
		restored = append(restored, f.Name)
	}

	log.Printf("[NVR] [INFO] [backup] restored %d files from backup", len(restored))
	return restored, nil
}

// List returns metadata for all backups in the backup directory.
func (s *Service) List() ([]BackupInfo, error) {
	if s.BackupDir == "" {
		return nil, fmt.Errorf("backup directory not configured")
	}

	entries, err := os.ReadDir(s.BackupDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []BackupInfo{}, nil
		}
		return nil, fmt.Errorf("read backup dir: %w", err)
	}

	var backups []BackupInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if !strings.HasSuffix(entry.Name(), ".enc") {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			continue
		}
		backups = append(backups, BackupInfo{
			Filename:  entry.Name(),
			Size:      info.Size(),
			CreatedAt: info.ModTime(),
			Auto:      strings.HasPrefix(entry.Name(), "auto-backup-"),
		})
	}

	// Sort newest first.
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].CreatedAt.After(backups[j].CreatedAt)
	})

	if backups == nil {
		backups = []BackupInfo{}
	}
	return backups, nil
}

// GetBackupPath returns the full path to a backup file, validating it exists
// and is within the backup directory.
func (s *Service) GetBackupPath(filename string) (string, error) {
	// Prevent directory traversal.
	clean := filepath.Base(filename)
	if clean != filename {
		return "", fmt.Errorf("invalid filename")
	}
	if !strings.HasSuffix(clean, ".enc") {
		return "", fmt.Errorf("invalid backup file extension")
	}

	path := filepath.Join(s.BackupDir, clean)
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("backup not found: %w", err)
	}
	return path, nil
}

// DeleteBackup removes a backup file from disk.
func (s *Service) DeleteBackup(filename string) error {
	path, err := s.GetBackupPath(filename)
	if err != nil {
		return err
	}
	return os.Remove(path)
}

// StartSchedule starts an automatic backup schedule. It runs backups at the
// given interval with the specified password, keeping at most maxKeep backups.
func (s *Service) StartSchedule(interval time.Duration, password string, maxKeep int) {
	s.StopSchedule()

	if interval <= 0 || password == "" {
		return
	}
	if maxKeep <= 0 {
		maxKeep = 10
	}

	s.schedule.enabled = true
	s.schedule.interval = interval
	s.schedule.password = password
	s.schedule.maxKeep = maxKeep

	s.stopCh = make(chan struct{})
	go s.runSchedule()
}

// StopSchedule stops the automatic backup schedule if running.
func (s *Service) StopSchedule() {
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
	s.schedule.enabled = false
}

// runSchedule is the background goroutine for scheduled backups.
func (s *Service) runSchedule() {
	ticker := time.NewTicker(s.schedule.interval)
	defer ticker.Stop()

	log.Printf("[NVR] [INFO] [backup] scheduled backups every %s (keep %d)",
		s.schedule.interval, s.schedule.maxKeep)

	for {
		select {
		case <-s.stopCh:
			log.Printf("[NVR] [INFO] [backup] scheduled backups stopped")
			return
		case <-ticker.C:
			filename, err := s.CreateBackup(s.schedule.password, true)
			if err != nil {
				log.Printf("[NVR] [ERROR] [backup] scheduled backup failed: %v", err)
				continue
			}
			log.Printf("[NVR] [INFO] [backup] scheduled backup created: %s", filename)

			// Prune old auto-backups beyond maxKeep.
			s.pruneAutoBackups()
		}
	}
}

// pruneAutoBackups removes the oldest auto-backups when they exceed maxKeep.
func (s *Service) pruneAutoBackups() {
	backups, err := s.List()
	if err != nil {
		return
	}

	var autoBackups []BackupInfo
	for _, b := range backups {
		if b.Auto {
			autoBackups = append(autoBackups, b)
		}
	}

	if len(autoBackups) <= s.schedule.maxKeep {
		return
	}

	// autoBackups is already sorted newest-first, so remove from the end.
	for i := s.schedule.maxKeep; i < len(autoBackups); i++ {
		path := filepath.Join(s.BackupDir, autoBackups[i].Filename)
		if err := os.Remove(path); err != nil {
			log.Printf("[NVR] [WARN] [backup] failed to prune %s: %v", autoBackups[i].Filename, err)
		} else {
			log.Printf("[NVR] [INFO] [backup] pruned old auto-backup: %s", autoBackups[i].Filename)
		}
	}
}

// ScheduleStatus returns the current schedule configuration.
type ScheduleStatus struct {
	Enabled  bool          `json:"enabled"`
	Interval time.Duration `json:"interval_seconds"`
	MaxKeep  int           `json:"max_keep"`
}

// GetScheduleStatus returns the current schedule state.
func (s *Service) GetScheduleStatus() ScheduleStatus {
	return ScheduleStatus{
		Enabled:  s.schedule.enabled,
		Interval: s.schedule.interval / time.Second,
		MaxKeep:  s.schedule.maxKeep,
	}
}
