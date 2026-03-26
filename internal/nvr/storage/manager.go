package storage

import (
	"fmt"
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

func (m *Manager) syncLoop() {
	defer m.wg.Done()
	// Implemented in Task 6
	<-m.stopCh
}
