package storage

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/db"
	"github.com/bluenviron/mediamtx/internal/nvr/yamlwriter"
)

const (
	defaultCheckInterval = 30 * time.Second
	defaultMaxRetries    = 10
)

// Manager monitors storage path health, handles failover, and recovery.
type Manager struct {
	db             *db.DB
	yamlWriter     *yamlwriter.Writer
	recordingsPath string
	apiAddress     string
	checkInterval  time.Duration
	maxRetries     int

	mu     sync.RWMutex
	health map[string]bool

	stopCh chan struct{}
	wg     sync.WaitGroup
}

// New creates a new Manager with the given dependencies and configuration.
func New(database *db.DB, yw *yamlwriter.Writer, recordingsPath, apiAddress string) *Manager {
	return &Manager{
		db:             database,
		yamlWriter:     yw,
		recordingsPath: recordingsPath,
		apiAddress:     apiAddress,
		checkInterval:  defaultCheckInterval,
		maxRetries:     defaultMaxRetries,
		health:         make(map[string]bool),
		stopCh:         make(chan struct{}),
	}
}

// Start runs an initial health check then starts the health and sync loops.
func (m *Manager) Start() {
	m.runHealthCheck()
	m.wg.Add(2)
	go m.healthLoop()
	go m.syncLoop()
}

// Stop signals the background loops to stop and waits for them to exit.
func (m *Manager) Stop() {
	close(m.stopCh)
	m.wg.Wait()
}

// GetHealth returns the last known health status for a storage path.
func (m *Manager) GetHealth(path string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.health[path]
}

// GetAllHealth returns a snapshot of all known path health statuses.
func (m *Manager) GetAllHealth() map[string]bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	result := make(map[string]bool, len(m.health))
	for k, v := range m.health {
		result[k] = v
	}
	return result
}

// StorageStatus returns a human-readable status string for a camera's storage path.
func (m *Manager) StorageStatus(cam *db.Camera) string {
	if cam.StoragePath == "" {
		return "default"
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.health[cam.StoragePath] {
		return "healthy"
	}
	return "degraded"
}

func (m *Manager) healthLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(m.checkInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.runHealthCheck()
		case <-m.stopCh:
			return
		}
	}
}

func (m *Manager) runHealthCheck() {
	cameras, err := m.db.ListCameras()
	if err != nil {
		log.Printf("[NVR] [storage] failed to list cameras: %v", err)
		return
	}
	pathCameras := make(map[string][]string)
	for _, cam := range cameras {
		if cam.StoragePath != "" {
			pathCameras[cam.StoragePath] = append(pathCameras[cam.StoragePath], cam.ID)
		}
	}
	m.evaluateHealth(pathCameras)
}

func (m *Manager) evaluateHealth(pathCameras map[string][]string) {
	for path, cameraIDs := range pathCameras {
		healthy, err := checkPathHealth(path)
		if err != nil {
			log.Printf("[NVR] [storage] health check error for %s: %v", path, err)
			continue
		}
		m.mu.Lock()
		wasHealthy, existed := m.health[path]
		m.health[path] = healthy
		m.mu.Unlock()

		if !existed {
			continue
		}
		if wasHealthy && !healthy {
			log.Printf("[NVR] [storage] path %s became UNREACHABLE", path)
			m.handleFailover(path, cameraIDs)
		} else if !wasHealthy && healthy {
			log.Printf("[NVR] [storage] path %s is REACHABLE again", path)
			m.handleRecovery(path, cameraIDs)
		}
	}
}

// checkPathHealth verifies that path exists, is a directory, and is writable.
func checkPathHealth(path string) (bool, error) {
	info, err := os.Stat(path)
	if err != nil {
		return false, nil
	}
	if !info.IsDir() {
		return false, nil
	}
	testFile := filepath.Join(path, ".nvr_health_check")
	if err := os.WriteFile(testFile, []byte("ok"), 0o644); err != nil {
		return false, nil
	}
	os.Remove(testFile)
	return true, nil
}

func (m *Manager) handleFailover(storagePath string, cameraIDs []string) {
	for _, camID := range cameraIDs {
		cam, err := m.db.GetCamera(camID)
		if err != nil {
			continue
		}
		fallbackPath := m.recordingsPath + "/%path/%Y-%m/%d/%H-%M-%S-%f"
		if err := m.yamlWriter.SetPathValue(cam.MediaMTXPath, "recordPath", fallbackPath); err != nil {
			log.Printf("[NVR] [storage] failed to failover camera %s: %v", camID, err)
		}
	}
	m.triggerConfigReload()
}

func (m *Manager) handleRecovery(storagePath string, cameraIDs []string) {
	for _, camID := range cameraIDs {
		cam, err := m.db.GetCamera(camID)
		if err != nil {
			continue
		}
		primaryPath := cam.StoragePath + "/%path/%Y-%m/%d/%H-%M-%S-%f"
		if err := m.yamlWriter.SetPathValue(cam.MediaMTXPath, "recordPath", primaryPath); err != nil {
			log.Printf("[NVR] [storage] failed to recover camera %s: %v", camID, err)
		}
	}
	m.triggerConfigReload()
}

func (m *Manager) triggerConfigReload() {
	url := fmt.Sprintf("http://localhost%s/v3/config/globalconf/patch", m.apiAddress)
	req, err := http.NewRequest("PATCH", url, strings.NewReader("{}"))
	if err != nil {
		log.Printf("[NVR] [storage] failed to create reload request: %v", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		log.Printf("[NVR] [storage] config reload failed: %v", err)
		return
	}
	resp.Body.Close()
}

const syncInterval = 60 * time.Second

func (m *Manager) syncLoop() {
	defer m.wg.Done()
	ticker := time.NewTicker(syncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			m.runSyncPass()
		case <-m.stopCh:
			return
		}
	}
}

// runSyncPass lists all pending syncs, processes each one, then sweeps orphan files.
func (m *Manager) runSyncPass() {
	syncs, err := m.db.ListPendingSyncs("pending")
	if err != nil {
		log.Printf("[NVR] [storage] failed to list pending syncs: %v", err)
		return
	}
	for _, ps := range syncs {
		m.processSync(ps)
	}
	m.sweepOrphans()
}

// processSync attempts to copy a file from local fallback storage to its target
// path, then updates the DB and removes the local copy on success.
// On failure it records the error and resets status to "pending" so it will
// be retried on the next pass.
func (m *Manager) processSync(ps *db.PendingSync) {
	// Mark as in-progress (no attempt increment yet).
	if err := m.db.SetPendingSyncStatus(ps.ID, "syncing"); err != nil {
		log.Printf("[NVR] [storage] failed to set sync status for %d: %v", ps.ID, err)
		return
	}

	if err := copyFile(ps.LocalPath, ps.TargetPath); err != nil {
		errMsg := fmt.Sprintf("copy failed: %v", err)
		log.Printf("[NVR] [storage] sync %d failed: %s", ps.ID, errMsg)
		if dbErr := m.db.RecordPendingSyncFailure(ps.ID, "pending", errMsg); dbErr != nil {
			log.Printf("[NVR] [storage] failed to record sync failure for %d: %v", ps.ID, dbErr)
		}
		m.checkMaxRetries(ps)
		return
	}

	// Verify the copied file has the expected size.
	info, err := os.Stat(ps.TargetPath)
	if err != nil {
		errMsg := fmt.Sprintf("stat target failed after copy: %v", err)
		log.Printf("[NVR] [storage] sync %d failed: %s", ps.ID, errMsg)
		if dbErr := m.db.RecordPendingSyncFailure(ps.ID, "pending", errMsg); dbErr != nil {
			log.Printf("[NVR] [storage] failed to record sync failure for %d: %v", ps.ID, dbErr)
		}
		m.checkMaxRetries(ps)
		return
	}
	srcInfo, err := os.Stat(ps.LocalPath)
	if err == nil && info.Size() != srcInfo.Size() {
		errMsg := fmt.Sprintf("size mismatch after copy: got %d, want %d", info.Size(), srcInfo.Size())
		log.Printf("[NVR] [storage] sync %d failed: %s", ps.ID, errMsg)
		_ = os.Remove(ps.TargetPath)
		if dbErr := m.db.RecordPendingSyncFailure(ps.ID, "pending", errMsg); dbErr != nil {
			log.Printf("[NVR] [storage] failed to record sync failure for %d: %v", ps.ID, dbErr)
		}
		m.checkMaxRetries(ps)
		return
	}

	// Update the recording's file_path to the target location.
	if err := m.db.UpdateRecordingFilePath(ps.RecordingID, ps.TargetPath); err != nil {
		errMsg := fmt.Sprintf("update recording path failed: %v", err)
		log.Printf("[NVR] [storage] sync %d failed: %s", ps.ID, errMsg)
		_ = os.Remove(ps.TargetPath)
		if dbErr := m.db.RecordPendingSyncFailure(ps.ID, "pending", errMsg); dbErr != nil {
			log.Printf("[NVR] [storage] failed to record sync failure for %d: %v", ps.ID, dbErr)
		}
		m.checkMaxRetries(ps)
		return
	}

	// Remove the local fallback copy.
	if err := os.Remove(ps.LocalPath); err != nil && !os.IsNotExist(err) {
		log.Printf("[NVR] [storage] warning: could not remove local file %s: %v", ps.LocalPath, err)
	}

	// Delete the pending sync record — work is done.
	if err := m.db.DeletePendingSync(ps.ID); err != nil {
		log.Printf("[NVR] [storage] failed to delete pending sync %d: %v", ps.ID, err)
	}

	log.Printf("[NVR] [storage] synced recording %d to %s", ps.RecordingID, ps.TargetPath)
}

// checkMaxRetries re-reads the sync record and marks it as "failed" if the
// attempt count has reached the configured maximum.
func (m *Manager) checkMaxRetries(ps *db.PendingSync) {
	current, err := m.db.GetPendingSync(ps.ID)
	if err != nil {
		return
	}
	if current.Attempts >= m.maxRetries {
		log.Printf("[NVR] [storage] pending sync %d exceeded max retries (%d), marking failed", ps.ID, m.maxRetries)
		if dbErr := m.db.SetPendingSyncStatus(ps.ID, "failed"); dbErr != nil {
			log.Printf("[NVR] [storage] failed to mark sync %d as failed: %v", ps.ID, dbErr)
		}
	}
}

// sweepOrphans walks the local fallback recording directories for cameras that
// have a custom StoragePath configured, and removes any files on disk that are
// no longer referenced by any recording or pending sync record.
func (m *Manager) sweepOrphans() {
	cameras, err := m.db.ListCameras()
	if err != nil {
		log.Printf("[NVR] [storage] sweepOrphans: failed to list cameras: %v", err)
		return
	}
	for _, cam := range cameras {
		if cam.StoragePath == "" {
			continue
		}
		fallbackDir := filepath.Join(m.recordingsPath, cam.MediaMTXPath)
		_ = filepath.Walk(fallbackDir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info == nil || info.IsDir() {
				return nil
			}
			if !m.db.FileIsReferenced(path) {
				log.Printf("[NVR] [storage] sweepOrphans: removing unreferenced file %s", path)
				if removeErr := os.Remove(path); removeErr != nil && !os.IsNotExist(removeErr) {
					log.Printf("[NVR] [storage] sweepOrphans: failed to remove %s: %v", path, removeErr)
				}
			}
			return nil
		})
	}
}

// TriggerSyncForCamera manually triggers a sync pass for all pending syncs
// belonging to a specific camera.
func (m *Manager) TriggerSyncForCamera(cameraID string) {
	syncs, err := m.db.ListPendingSyncsByCamera(cameraID)
	if err != nil {
		log.Printf("[NVR] [storage] TriggerSyncForCamera %s: failed to list pending syncs: %v", cameraID, err)
		return
	}
	for _, ps := range syncs {
		m.processSync(ps)
	}
}

// copyFile copies the file at src to dst, creating dst's parent directories as
// needed. The destination file is written atomically via a temporary file so
// that a partial copy is never visible at the final path.
func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("create target directory: %w", err)
	}

	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open source: %w", err)
	}
	defer in.Close()

	// Write to a temp file in the same directory so the rename is atomic.
	tmpDst := dst + ".tmp"
	out, err := os.Create(tmpDst)
	if err != nil {
		return fmt.Errorf("create temp dest: %w", err)
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmpDst)
		return fmt.Errorf("copy data: %w", err)
	}
	if err := out.Close(); err != nil {
		os.Remove(tmpDst)
		return fmt.Errorf("close dest: %w", err)
	}

	if err := os.Rename(tmpDst, dst); err != nil {
		os.Remove(tmpDst)
		return fmt.Errorf("rename to final path: %w", err)
	}
	return nil
}
