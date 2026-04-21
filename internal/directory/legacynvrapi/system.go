package legacynvrapi

// system.go — System management endpoints for the legacy NVR API.
//
// GET  /api/nvr/system/storage  — Storage info (DB + disk stats)
// GET  /api/nvr/system/metrics  — Runtime metrics (goroutines, heap, etc.)
// POST /api/nvr/system/backup   — Create a directory DB backup
// GET  /api/nvr/system/backups  — List existing backups

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

// -----------------------------------------------------------------------
// Storage
// -----------------------------------------------------------------------

func (h *Handlers) systemStorage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	resp := map[string]any{
		"total_bytes": int64(0),
		"used_bytes":  int64(0),
		"free_bytes":  int64(0),
		"cameras":     []any{},
	}

	// Per-camera usage from RecDB.
	if h.RecDB != nil {
		total, err := h.RecDB.GetTotalStorageUsage()
		if err == nil {
			resp["used_bytes"] = total
		}

		perCam, err := h.RecDB.GetStoragePerCamera()
		if err == nil && perCam != nil {
			resp["cameras"] = perCam
		}
	}

	// Disk stats — try to find the data directory by resolving the directory DB path.
	// Fall back to the current working directory on any error.
	diskDir := "."
	if h.DB != nil {
		// The DB path is not exposed directly, but we can inspect the file
		// via the underlying *sql.DB stats — use CWD as a best-effort fallback.
		_ = diskDir // already set to "."
	}
	var stat syscall.Statfs_t
	if err := syscall.Statfs(diskDir, &stat); err == nil {
		blockSize := uint64(stat.Bsize)
		resp["total_bytes"] = int64(stat.Blocks * blockSize)
		resp["free_bytes"] = int64(stat.Bavail * blockSize)
	}

	writeJSON(w, http.StatusOK, resp)
}

// -----------------------------------------------------------------------
// Metrics
// -----------------------------------------------------------------------

func (h *Handlers) systemMetrics(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	var m runtime.MemStats
	runtime.ReadMemStats(&m)

	writeJSON(w, http.StatusOK, map[string]any{
		"goroutines":    runtime.NumGoroutine(),
		"heap_alloc_mb": m.HeapAlloc / 1024 / 1024,
		"heap_sys_mb":   m.HeapSys / 1024 / 1024,
		"heap_idle_mb":  m.HeapIdle / 1024 / 1024,
		"heap_inuse_mb": m.HeapInuse / 1024 / 1024,
		"gc_cycles":     m.NumGC,
		"num_cpu":       runtime.NumCPU(),
		"go_version":    runtime.Version(),
	})
}

// -----------------------------------------------------------------------
// Backup
// -----------------------------------------------------------------------

// dataDir attempts to locate the directory where the SQLite DB is stored.
// Without a stored path we fall back to the current working directory.
func dataDir() string {
	cwd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return cwd
}

func (h *Handlers) systemBackup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	dir := dataDir()
	src := filepath.Join(dir, "directory.db")
	timestamp := time.Now().UTC().Format("20060102-150405")
	dst := filepath.Join(dir, fmt.Sprintf("directory.db.backup-%s", timestamp))

	if err := copyFile(src, dst); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "backup failed: " + err.Error()})
		return
	}

	info, err := os.Stat(dst)
	size := int64(0)
	if err == nil {
		size = info.Size()
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"path": dst,
		"size": size,
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer func() {
		_ = out.Close()
	}()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Sync()
}

// -----------------------------------------------------------------------
// List backups
// -----------------------------------------------------------------------

func (h *Handlers) systemBackups(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, `{"error":"method not allowed"}`, http.StatusMethodNotAllowed)
		return
	}

	dir := dataDir()
	entries, err := os.ReadDir(dir)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}

	type backupEntry struct {
		Name      string `json:"name"`
		Path      string `json:"path"`
		SizeBytes int64  `json:"size_bytes"`
		CreatedAt string `json:"created_at"`
	}

	var backups []backupEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.Contains(name, ".backup") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		backups = append(backups, backupEntry{
			Name:      name,
			Path:      filepath.Join(dir, name),
			SizeBytes: info.Size(),
			CreatedAt: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	if backups == nil {
		backups = []backupEntry{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": backups})
}

// -----------------------------------------------------------------------
// System sub-router (called from handlers.go Register)
// -----------------------------------------------------------------------

func (h *Handlers) systemSubrouter(w http.ResponseWriter, r *http.Request) {
	sub := strings.TrimPrefix(r.URL.Path, "/api/nvr/system/")
	sub = strings.TrimSuffix(sub, "/")

	switch sub {
	case "storage":
		h.systemStorage(w, r)
	case "metrics":
		h.systemMetrics(w, r)
	case "backup":
		h.systemBackup(w, r)
	case "backups":
		h.systemBackups(w, r)
	default:
		h.notImplemented(w, r)
	}
}
