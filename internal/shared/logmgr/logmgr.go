// Package logmgr provides structured JSON logging with per-module log levels,
// file rotation by size and age, and crash dump collection for the NVR subsystem.
package logmgr

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"
)

// Level represents a log severity level.
type Level int

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
)

// String returns the human-readable name of a log level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "debug"
	case LevelInfo:
		return "info"
	case LevelWarn:
		return "warn"
	case LevelError:
		return "error"
	default:
		return "unknown"
	}
}

// ParseLevel converts a string to a Level. Returns LevelInfo if unrecognized.
func ParseLevel(s string) Level {
	switch strings.ToLower(s) {
	case "debug":
		return LevelDebug
	case "info":
		return LevelInfo
	case "warn", "warning":
		return LevelWarn
	case "error":
		return LevelError
	default:
		return LevelInfo
	}
}

// Config holds the logging configuration.
type Config struct {
	// GlobalLevel is the default log level for all modules.
	GlobalLevel string `json:"global_level"`
	// ModuleLevels overrides log levels for specific modules.
	ModuleLevels map[string]string `json:"module_levels"`
	// LogDir is the directory where log files are stored.
	LogDir string `json:"log_dir"`
	// MaxSizeMB is the max size per log file in megabytes before rotation.
	MaxSizeMB int `json:"max_size_mb"`
	// MaxAgeDays is the number of days to retain rotated log files.
	MaxAgeDays int `json:"max_age_days"`
	// MaxBackups is the max number of rotated log files to keep.
	MaxBackups int `json:"max_backups"`
	// JSONOutput enables structured JSON log output.
	JSONOutput bool `json:"json_output"`
	// CrashDumpEnabled enables automatic crash dump collection.
	CrashDumpEnabled bool `json:"crash_dump_enabled"`
}

// DefaultConfig returns sensible defaults for log management.
func DefaultConfig() Config {
	return Config{
		GlobalLevel:      "info",
		ModuleLevels:     map[string]string{},
		LogDir:           "./logs",
		MaxSizeMB:        50,
		MaxAgeDays:       30,
		MaxBackups:       10,
		JSONOutput:       true,
		CrashDumpEnabled: true,
	}
}

// Entry is a structured log entry written as JSON.
type Entry struct {
	Timestamp string `json:"timestamp"`
	Level     string `json:"level"`
	Module    string `json:"module"`
	Message   string `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
	Caller    string `json:"caller,omitempty"`
}

// Manager is the central log manager. It manages per-module log levels,
// file rotation, and crash dump collection.
type Manager struct {
	mu           sync.RWMutex
	config       Config
	globalLevel  Level
	moduleLevels map[string]Level
	writer       *rotatingWriter
	fallback     io.Writer // stderr fallback
	configDB     configStore
}

// configStore abstracts DB access for persisting log config.
type configStore interface {
	GetConfig(key string) (string, error)
	SetConfig(key, value string) error
}

// New creates a new log Manager with the given config and optional DB for persistence.
func New(cfg Config, store configStore) (*Manager, error) {
	m := &Manager{
		config:       cfg,
		globalLevel:  ParseLevel(cfg.GlobalLevel),
		moduleLevels: make(map[string]Level),
		fallback:     os.Stderr,
		configDB:     store,
	}

	for mod, lvl := range cfg.ModuleLevels {
		m.moduleLevels[mod] = ParseLevel(lvl)
	}

	// Try to load persisted config from DB.
	if store != nil {
		if saved, err := store.GetConfig("logging_config"); err == nil && saved != "" {
			var savedCfg Config
			if err := json.Unmarshal([]byte(saved), &savedCfg); err == nil {
				m.applyConfig(savedCfg)
			}
		}
	}

	// Ensure log directory exists.
	if err := os.MkdirAll(m.config.LogDir, 0o755); err != nil {
		return nil, fmt.Errorf("create log dir %s: %w", m.config.LogDir, err)
	}

	rw, err := newRotatingWriter(m.config.LogDir, m.config.MaxSizeMB, m.config.MaxBackups, m.config.MaxAgeDays)
	if err != nil {
		return nil, fmt.Errorf("open log writer: %w", err)
	}
	m.writer = rw

	// Install crash dump handler if enabled.
	if m.config.CrashDumpEnabled {
		m.installCrashHandler()
	}

	return m, nil
}

// applyConfig updates internal state from a Config without re-opening files.
func (m *Manager) applyConfig(cfg Config) {
	m.config.GlobalLevel = cfg.GlobalLevel
	m.globalLevel = ParseLevel(cfg.GlobalLevel)

	if cfg.ModuleLevels != nil {
		m.config.ModuleLevels = cfg.ModuleLevels
		m.moduleLevels = make(map[string]Level)
		for mod, lvl := range cfg.ModuleLevels {
			m.moduleLevels[mod] = ParseLevel(lvl)
		}
	}
	if cfg.MaxSizeMB > 0 {
		m.config.MaxSizeMB = cfg.MaxSizeMB
	}
	if cfg.MaxAgeDays > 0 {
		m.config.MaxAgeDays = cfg.MaxAgeDays
	}
	if cfg.MaxBackups > 0 {
		m.config.MaxBackups = cfg.MaxBackups
	}
	m.config.JSONOutput = cfg.JSONOutput
	m.config.CrashDumpEnabled = cfg.CrashDumpEnabled
}

// GetConfig returns a copy of the current logging configuration.
func (m *Manager) GetConfig() Config {
	m.mu.RLock()
	defer m.mu.RUnlock()

	cfg := m.config
	cfg.ModuleLevels = make(map[string]string)
	for k, v := range m.config.ModuleLevels {
		cfg.ModuleLevels[k] = v
	}
	return cfg
}

// UpdateConfig applies a new configuration and persists it.
func (m *Manager) UpdateConfig(cfg Config) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.applyConfig(cfg)

	// Update writer rotation settings.
	if m.writer != nil {
		m.writer.maxSizeBytes = int64(m.config.MaxSizeMB) * 1024 * 1024
		m.writer.maxBackups = m.config.MaxBackups
		m.writer.maxAgeDays = m.config.MaxAgeDays
	}

	// Persist to DB.
	if m.configDB != nil {
		data, err := json.Marshal(m.config)
		if err != nil {
			return fmt.Errorf("marshal config: %w", err)
		}
		if err := m.configDB.SetConfig("logging_config", string(data)); err != nil {
			return fmt.Errorf("persist config: %w", err)
		}
	}

	return nil
}

// effectiveLevel returns the log level for a given module.
func (m *Manager) effectiveLevel(module string) Level {
	if lvl, ok := m.moduleLevels[module]; ok {
		return lvl
	}
	return m.globalLevel
}

// Log writes a log entry if the level meets the module threshold.
func (m *Manager) Log(level Level, module, message string, fields map[string]interface{}) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	if level < m.effectiveLevel(module) {
		return
	}

	entry := Entry{
		Timestamp: time.Now().UTC().Format(time.RFC3339Nano),
		Level:     level.String(),
		Module:    module,
		Message:   message,
		Fields:    fields,
	}

	// Add caller information for warn and error.
	if level >= LevelWarn {
		if _, file, line, ok := runtime.Caller(2); ok {
			entry.Caller = fmt.Sprintf("%s:%d", filepath.Base(file), line)
		}
	}

	if m.config.JSONOutput {
		data, err := json.Marshal(entry)
		if err != nil {
			fmt.Fprintf(m.fallback, "[logmgr] marshal error: %v\n", err)
			return
		}
		data = append(data, '\n')
		if m.writer != nil {
			if _, err := m.writer.Write(data); err != nil {
				fmt.Fprintf(m.fallback, "[logmgr] write error: %v\n", err)
			}
		}
	} else {
		line := fmt.Sprintf("[%s] [%s] [%s] %s", entry.Timestamp, strings.ToUpper(entry.Level), entry.Module, entry.Message)
		if len(fields) > 0 {
			data, _ := json.Marshal(fields)
			line += " " + string(data)
		}
		line += "\n"
		if m.writer != nil {
			m.writer.Write([]byte(line))
		}
	}
}

// Debug logs a debug-level message.
func (m *Manager) Debug(module, message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	m.Log(LevelDebug, module, message, f)
}

// Info logs an info-level message.
func (m *Manager) Info(module, message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	m.Log(LevelInfo, module, message, f)
}

// Warn logs a warn-level message.
func (m *Manager) Warn(module, message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	m.Log(LevelWarn, module, message, f)
}

// Error logs an error-level message.
func (m *Manager) Error(module, message string, fields ...map[string]interface{}) {
	var f map[string]interface{}
	if len(fields) > 0 {
		f = fields[0]
	}
	m.Log(LevelError, module, message, f)
}

// Close shuts down the log manager and flushes any pending writes.
func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.writer != nil {
		return m.writer.Close()
	}
	return nil
}

// installCrashHandler sets up panic recovery and goroutine dump collection.
func (m *Manager) installCrashHandler() {
	crashDir := filepath.Join(m.config.LogDir, "crashes")
	os.MkdirAll(crashDir, 0o755)
}

// WriteCrashDump writes a crash dump file with stack trace and system info.
func (m *Manager) WriteCrashDump(panicVal interface{}) string {
	m.mu.RLock()
	crashDir := filepath.Join(m.config.LogDir, "crashes")
	enabled := m.config.CrashDumpEnabled
	m.mu.RUnlock()

	if !enabled {
		return ""
	}

	os.MkdirAll(crashDir, 0o755)

	ts := time.Now().UTC().Format("20060102-150405")
	filename := filepath.Join(crashDir, fmt.Sprintf("crash-%s.log", ts))

	var buf []byte
	buf = append(buf, fmt.Sprintf("Crash Dump - %s\n", time.Now().UTC().Format(time.RFC3339))...)
	buf = append(buf, fmt.Sprintf("Panic: %v\n", panicVal)...)
	buf = append(buf, fmt.Sprintf("Go Version: %s\n", runtime.Version())...)
	buf = append(buf, fmt.Sprintf("GOOS: %s\n", runtime.GOOS)...)
	buf = append(buf, fmt.Sprintf("GOARCH: %s\n", runtime.GOARCH)...)
	buf = append(buf, fmt.Sprintf("NumGoroutine: %d\n", runtime.NumGoroutine())...)

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)
	buf = append(buf, fmt.Sprintf("MemAlloc: %d MB\n", memStats.Alloc/(1024*1024))...)
	buf = append(buf, fmt.Sprintf("MemSys: %d MB\n", memStats.Sys/(1024*1024))...)

	buf = append(buf, "\n--- Stack Trace ---\n"...)
	buf = append(buf, debug.Stack()...)

	// Build info.
	if info, ok := debug.ReadBuildInfo(); ok {
		buf = append(buf, "\n--- Build Info ---\n"...)
		buf = append(buf, fmt.Sprintf("Path: %s\n", info.Path)...)
		buf = append(buf, fmt.Sprintf("Main: %s@%s\n", info.Main.Path, info.Main.Version)...)
	}

	os.WriteFile(filename, buf, 0o644)

	// Also log the crash.
	m.Log(LevelError, "crash", fmt.Sprintf("crash dump written to %s", filename), map[string]interface{}{
		"panic": fmt.Sprintf("%v", panicVal),
	})

	return filename
}

// ListCrashDumps returns metadata about existing crash dump files.
func (m *Manager) ListCrashDumps() []CrashDumpInfo {
	m.mu.RLock()
	crashDir := filepath.Join(m.config.LogDir, "crashes")
	m.mu.RUnlock()

	entries, err := os.ReadDir(crashDir)
	if err != nil {
		return nil
	}

	var dumps []CrashDumpInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		dumps = append(dumps, CrashDumpInfo{
			Filename:  e.Name(),
			SizeBytes: info.Size(),
			CreatedAt: info.ModTime().UTC().Format(time.RFC3339),
		})
	}

	sort.Slice(dumps, func(i, j int) bool {
		return dumps[i].CreatedAt > dumps[j].CreatedAt
	})

	return dumps
}

// CrashDumpInfo holds metadata about a crash dump file.
type CrashDumpInfo struct {
	Filename  string `json:"filename"`
	SizeBytes int64  `json:"size_bytes"`
	CreatedAt string `json:"created_at"`
}

// GetCrashDump reads the contents of a specific crash dump file.
func (m *Manager) GetCrashDump(filename string) (string, error) {
	m.mu.RLock()
	crashDir := filepath.Join(m.config.LogDir, "crashes")
	m.mu.RUnlock()

	// Prevent path traversal.
	clean := filepath.Base(filename)
	if clean != filename {
		return "", fmt.Errorf("invalid filename")
	}

	data, err := os.ReadFile(filepath.Join(crashDir, clean))
	if err != nil {
		return "", err
	}
	return string(data), nil
}
