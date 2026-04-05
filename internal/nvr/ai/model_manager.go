package ai

import (
	"crypto/sha256"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// ModelInfo describes an installed model file.
type ModelInfo struct {
	Name         string    `json:"name"`
	Path         string    `json:"path"`
	Size         int64     `json:"size"`
	SizeHuman    string    `json:"size_human"`
	LastModified time.Time `json:"last_modified"`
	SHA256       string    `json:"sha256,omitempty"`
	Active       bool      `json:"active"`
	Type         ModelType `json:"type"`
}

// ModelType classifies a model by its function.
type ModelType string

const (
	ModelTypeDetector ModelType = "detector"
	ModelTypeEmbedder ModelType = "embedder"
	ModelTypeUnknown  ModelType = "unknown"
)

// ModelManager manages YOLO detector models with hot-swap and fallback support.
// It scans a models directory for .onnx files, tracks the active model, and
// supports swapping models atomically while the detection pipeline continues.
type ModelManager struct {
	mu sync.RWMutex

	modelsDir string

	// Active detector and its path.
	activeDetector *Detector
	activeModel    string // file path of the active model

	// Previous detector kept for fallback.
	previousDetector *Detector
	previousModel    string
}

// NewModelManager creates a model manager for the given models directory.
// If an initial detector is already loaded, pass it along with its model path
// so the manager can track it. Pass nil if no detector is loaded yet.
func NewModelManager(modelsDir string, initialDetector *Detector, initialModelPath string) *ModelManager {
	return &ModelManager{
		modelsDir:      modelsDir,
		activeDetector: initialDetector,
		activeModel:    initialModelPath,
	}
}

// ListModels scans the models directory and returns information about all
// installed .onnx model files.
func (m *ModelManager) ListModels() ([]ModelInfo, error) {
	m.mu.RLock()
	activeModel := m.activeModel
	m.mu.RUnlock()

	entries, err := os.ReadDir(m.modelsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ModelInfo{}, nil
		}
		return nil, fmt.Errorf("reading models directory: %w", err)
	}

	var models []ModelInfo
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		ext := strings.ToLower(filepath.Ext(name))
		if ext != ".onnx" && ext != ".bin" && ext != ".json" {
			continue
		}

		fullPath := filepath.Join(m.modelsDir, name)
		info, err := entry.Info()
		if err != nil {
			continue
		}

		modelType := classifyModel(name)

		models = append(models, ModelInfo{
			Name:         name,
			Path:         fullPath,
			Size:         info.Size(),
			SizeHuman:    humanSize(info.Size()),
			LastModified: info.ModTime(),
			Active:       fullPath == activeModel,
			Type:         modelType,
		})
	}

	return models, nil
}

// ActiveModel returns the path of the currently active detector model.
func (m *ModelManager) ActiveModel() string {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeModel
}

// ActiveDetector returns the currently active detector. Callers must not
// call Close on the returned detector.
func (m *ModelManager) ActiveDetector() *Detector {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return m.activeDetector
}

// Activate loads a new model by path, swaps it in atomically, and keeps the
// previous model for fallback. If the new model fails to load, the active
// model remains unchanged and an error is returned.
func (m *ModelManager) Activate(modelPath string) error {
	// Resolve to absolute path if relative.
	if !filepath.IsAbs(modelPath) {
		modelPath = filepath.Join(m.modelsDir, modelPath)
	}

	// Verify the file exists.
	if _, err := os.Stat(modelPath); err != nil {
		return fmt.Errorf("model file not found: %s: %w", modelPath, err)
	}

	// Check if this is already the active model.
	m.mu.RLock()
	if m.activeModel == modelPath {
		m.mu.RUnlock()
		return nil
	}
	m.mu.RUnlock()

	// Load the new model in the background (outside the lock).
	log.Printf("[ai][model-manager] loading new model: %s", modelPath)
	newDetector, err := NewDetector(modelPath)
	if err != nil {
		return fmt.Errorf("failed to load model %s: %w", modelPath, err)
	}
	log.Printf("[ai][model-manager] new model loaded successfully: %s", modelPath)

	// Swap atomically.
	m.mu.Lock()
	oldPrevious := m.previousDetector
	m.previousDetector = m.activeDetector
	m.previousModel = m.activeModel
	m.activeDetector = newDetector
	m.activeModel = modelPath
	m.mu.Unlock()

	// Close the old previous detector (two generations back) outside the lock.
	if oldPrevious != nil {
		oldPrevious.Close()
		log.Printf("[ai][model-manager] closed old fallback model")
	}

	log.Printf("[ai][model-manager] activated model: %s (fallback: %s)", modelPath, m.previousModel)
	return nil
}

// Rollback switches back to the previous model. Returns an error if no
// previous model is available.
func (m *ModelManager) Rollback() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.previousDetector == nil {
		return fmt.Errorf("no previous model available for rollback")
	}

	// Swap: current becomes discarded, previous becomes active.
	old := m.activeDetector
	m.activeDetector = m.previousDetector
	m.activeModel = m.previousModel
	m.previousDetector = nil
	m.previousModel = ""

	// Close the old active detector.
	if old != nil {
		old.Close()
	}

	log.Printf("[ai][model-manager] rolled back to: %s", m.activeModel)
	return nil
}

// VerifyModel computes the SHA-256 checksum of a model file for integrity
// verification.
func (m *ModelManager) VerifyModel(modelPath string) (string, error) {
	if !filepath.IsAbs(modelPath) {
		modelPath = filepath.Join(m.modelsDir, modelPath)
	}

	f, err := os.Open(modelPath)
	if err != nil {
		return "", fmt.Errorf("opening model file: %w", err)
	}
	defer f.Close()

	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", fmt.Errorf("computing checksum: %w", err)
	}

	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// Close releases all detector resources held by the manager.
func (m *ModelManager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.activeDetector != nil {
		m.activeDetector.Close()
		m.activeDetector = nil
	}
	if m.previousDetector != nil {
		m.previousDetector.Close()
		m.previousDetector = nil
	}
}

// classifyModel guesses a model's type from its filename.
func classifyModel(name string) ModelType {
	lower := strings.ToLower(name)
	switch {
	case strings.Contains(lower, "yolo"):
		return ModelTypeDetector
	case strings.Contains(lower, "clip"):
		return ModelTypeEmbedder
	case strings.HasSuffix(lower, ".json") || strings.HasSuffix(lower, ".bin"):
		return ModelTypeEmbedder // vocab/projection files
	default:
		if strings.HasSuffix(lower, ".onnx") {
			return ModelTypeUnknown
		}
		return ModelTypeUnknown
	}
}

// humanSize formats a byte count as a human-readable string.
func humanSize(bytes int64) string {
	const unit = 1024
	if bytes < unit {
		return fmt.Sprintf("%d B", bytes)
	}
	div, exp := int64(unit), 0
	for n := bytes / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %ciB", float64(bytes)/float64(div), "KMGTPE"[exp])
}
