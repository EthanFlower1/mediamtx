package logmgr

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// rotatingWriter writes to a log file and rotates when size thresholds are exceeded.
// It also cleans up old rotated files based on max backups and max age.
type rotatingWriter struct {
	mu           sync.Mutex
	dir          string
	maxSizeBytes int64
	maxBackups   int
	maxAgeDays   int
	file         *os.File
	currentSize  int64
}

// newRotatingWriter creates a rotating writer that writes to nvr.log in the given directory.
func newRotatingWriter(dir string, maxSizeMB, maxBackups, maxAgeDays int) (*rotatingWriter, error) {
	w := &rotatingWriter{
		dir:          dir,
		maxSizeBytes: int64(maxSizeMB) * 1024 * 1024,
		maxBackups:   maxBackups,
		maxAgeDays:   maxAgeDays,
	}

	if err := w.openFile(); err != nil {
		return nil, err
	}
	return w, nil
}

func (w *rotatingWriter) logPath() string {
	return filepath.Join(w.dir, "nvr.log")
}

func (w *rotatingWriter) openFile() error {
	f, err := os.OpenFile(w.logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}

	w.file = f
	w.currentSize = info.Size()
	return nil
}

// Write writes data to the log file, rotating if needed.
func (w *rotatingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		if err := w.openFile(); err != nil {
			return 0, err
		}
	}

	// Check if rotation is needed.
	if w.maxSizeBytes > 0 && w.currentSize+int64(len(p)) > w.maxSizeBytes {
		if err := w.rotate(); err != nil {
			return 0, fmt.Errorf("rotate: %w", err)
		}
	}

	n, err := w.file.Write(p)
	w.currentSize += int64(n)
	return n, err
}

// rotate closes the current file, renames it with a timestamp, opens a new one,
// and cleans up old backups.
func (w *rotatingWriter) rotate() error {
	if w.file != nil {
		w.file.Close()
		w.file = nil
	}

	// Rename current log to timestamped backup.
	ts := time.Now().UTC().Format("20060102-150405")
	backupName := filepath.Join(w.dir, fmt.Sprintf("nvr-%s.log", ts))
	if err := os.Rename(w.logPath(), backupName); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rename log: %w", err)
	}

	// Open fresh log file.
	if err := w.openFile(); err != nil {
		return err
	}

	// Cleanup old backups in background-safe manner.
	w.cleanup()

	return nil
}

// cleanup removes old rotated log files beyond maxBackups and maxAgeDays.
func (w *rotatingWriter) cleanup() {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		return
	}

	var backups []os.DirEntry
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, "nvr-") && strings.HasSuffix(name, ".log") {
			backups = append(backups, e)
		}
	}

	// Sort by name descending (newest first since names contain timestamps).
	sort.Slice(backups, func(i, j int) bool {
		return backups[i].Name() > backups[j].Name()
	})

	cutoff := time.Now().AddDate(0, 0, -w.maxAgeDays)

	for i, b := range backups {
		path := filepath.Join(w.dir, b.Name())

		// Remove if beyond max backups count.
		if w.maxBackups > 0 && i >= w.maxBackups {
			os.Remove(path)
			continue
		}

		// Remove if beyond max age.
		if w.maxAgeDays > 0 {
			info, err := b.Info()
			if err == nil && info.ModTime().Before(cutoff) {
				os.Remove(path)
			}
		}
	}
}

// Close closes the underlying file.
func (w *rotatingWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file != nil {
		err := w.file.Close()
		w.file = nil
		return err
	}
	return nil
}
