// Package diagnostics provides remote diagnostic capabilities for the NVR,
// including support bundle generation, log querying, and network probing.
package diagnostics

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// BundleStatus represents the lifecycle state of a support bundle.
type BundleStatus string

const (
	BundleStatusPending   BundleStatus = "pending"
	BundleStatusBuilding  BundleStatus = "building"
	BundleStatusReady     BundleStatus = "ready"
	BundleStatusFailed    BundleStatus = "failed"
	BundleStatusExpired   BundleStatus = "expired"
)

// Bundle holds metadata for a generated support bundle.
type Bundle struct {
	ID        string       `json:"id"`
	Status    BundleStatus `json:"status"`
	CreatedAt string       `json:"created_at"`
	ExpiresAt string       `json:"expires_at"`
	SizeBytes int64        `json:"size_bytes"`
	Error     string       `json:"error,omitempty"`
}

// LogEntry represents a single log line returned by the log viewer.
type LogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Module    string                 `json:"module"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// LogQuery defines filters for the log viewer.
type LogQuery struct {
	Search string `json:"search"`
	Level  string `json:"level"`  // "debug", "info", "warn", "error"
	Module string `json:"module"` // filter by module name
	After  string `json:"after"`  // RFC3339 timestamp lower bound
	Before string `json:"before"` // RFC3339 timestamp upper bound
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
}

// ProbeResult holds the outcome of a network connectivity probe.
type ProbeResult struct {
	Target    string  `json:"target"`
	Port      int     `json:"port"`
	Reachable bool    `json:"reachable"`
	LatencyMs float64 `json:"latency_ms"`
	Error     string  `json:"error,omitempty"`
}

// RecorderStatus summarises the health of a single recorder camera.
type RecorderStatus struct {
	CameraID       string `json:"camera_id"`
	CameraName     string `json:"camera_name"`
	Status         string `json:"status"` // "recording", "idle", "stalled", "failed"
	LastSegmentAt  string `json:"last_segment_at,omitempty"`
	ErrorMessage   string `json:"error_message,omitempty"`
	RestartCount   int    `json:"restart_count"`
	UptimeSeconds  int64  `json:"uptime_seconds"`
}

// HealthProvider abstracts the scheduler's recording health interface.
type HealthProvider interface {
	GetAllRecordingHealth() []RecordingHealth
}

// RecordingHealth mirrors the scheduler data shape.
type RecordingHealth struct {
	CameraID       string
	CameraName     string
	Status         string
	LastSegmentAt  string
	ErrorMessage   string
	RestartCount   int
	UptimeSeconds  int64
}

// Service is the core diagnostics engine. It manages support bundle
// lifecycle, log querying, and network probes.
type Service struct {
	mu             sync.RWMutex
	bundles        map[string]*Bundle
	bundleDir      string
	logDir         string
	bundleExpiry   time.Duration
	healthProvider HealthProvider
	version        string
}

// ServiceConfig configures the diagnostics service.
type ServiceConfig struct {
	BundleDir      string
	LogDir         string
	BundleExpiry   time.Duration
	HealthProvider HealthProvider
	Version        string
}

// NewService creates a new diagnostics service.
func NewService(cfg ServiceConfig) (*Service, error) {
	if cfg.BundleDir == "" {
		cfg.BundleDir = "./data/bundles"
	}
	if cfg.LogDir == "" {
		cfg.LogDir = "./logs"
	}
	if cfg.BundleExpiry == 0 {
		cfg.BundleExpiry = 24 * time.Hour
	}

	if err := os.MkdirAll(cfg.BundleDir, 0o755); err != nil {
		return nil, fmt.Errorf("create bundle dir: %w", err)
	}

	return &Service{
		bundles:        make(map[string]*Bundle),
		bundleDir:      cfg.BundleDir,
		logDir:         cfg.LogDir,
		bundleExpiry:   cfg.BundleExpiry,
		healthProvider: cfg.HealthProvider,
		version:        cfg.Version,
	}, nil
}

// GenerateBundle creates a new support bundle asynchronously.
// It returns the bundle ID immediately so the caller can poll status.
func (s *Service) GenerateBundle() string {
	id := uuid.New().String()[:12]
	now := time.Now().UTC()

	bundle := &Bundle{
		ID:        id,
		Status:    BundleStatusPending,
		CreatedAt: now.Format(time.RFC3339),
		ExpiresAt: now.Add(s.bundleExpiry).Format(time.RFC3339),
	}

	s.mu.Lock()
	s.bundles[id] = bundle
	s.mu.Unlock()

	go s.buildBundle(id)
	return id
}

// GetBundle returns the metadata for a bundle by ID.
func (s *Service) GetBundle(id string) (*Bundle, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.bundles[id]
	if !ok {
		return nil, false
	}
	// Check expiry.
	if b.Status == BundleStatusReady {
		expires, _ := time.Parse(time.RFC3339, b.ExpiresAt)
		if time.Now().UTC().After(expires) {
			b.Status = BundleStatusExpired
		}
	}
	return b, true
}

// ListBundles returns all known bundles, sorted newest first.
func (s *Service) ListBundles() []*Bundle {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]*Bundle, 0, len(s.bundles))
	now := time.Now().UTC()
	for _, b := range s.bundles {
		if b.Status == BundleStatusReady {
			expires, _ := time.Parse(time.RFC3339, b.ExpiresAt)
			if now.After(expires) {
				b.Status = BundleStatusExpired
			}
		}
		result = append(result, b)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt > result[j].CreatedAt
	})
	return result
}

// BundlePath returns the file path for a ready bundle, or empty string.
func (s *Service) BundlePath(id string) string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	b, ok := s.bundles[id]
	if !ok || b.Status != BundleStatusReady {
		return ""
	}
	expires, _ := time.Parse(time.RFC3339, b.ExpiresAt)
	if time.Now().UTC().After(expires) {
		b.Status = BundleStatusExpired
		return ""
	}
	return filepath.Join(s.bundleDir, id+".zip")
}

// buildBundle runs in a goroutine to assemble the support bundle ZIP.
func (s *Service) buildBundle(id string) {
	s.mu.Lock()
	b := s.bundles[id]
	b.Status = BundleStatusBuilding
	s.mu.Unlock()

	zipPath := filepath.Join(s.bundleDir, id+".zip")

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)

	// 1. System info.
	s.addSystemInfo(zw)

	// 2. Recent logs.
	s.addLogs(zw)

	// 3. Health snapshot.
	s.addHealthSnapshot(zw)

	// 4. Network probe results.
	s.addNetworkProbes(zw)

	if err := zw.Close(); err != nil {
		s.failBundle(id, err)
		return
	}

	if err := os.WriteFile(zipPath, buf.Bytes(), 0o600); err != nil {
		s.failBundle(id, err)
		return
	}

	s.mu.Lock()
	b.Status = BundleStatusReady
	b.SizeBytes = int64(buf.Len())
	s.mu.Unlock()
}

func (s *Service) failBundle(id string, err error) {
	s.mu.Lock()
	if b, ok := s.bundles[id]; ok {
		b.Status = BundleStatusFailed
		b.Error = err.Error()
	}
	s.mu.Unlock()
}

func (s *Service) addSystemInfo(zw *zip.Writer) {
	w, err := zw.Create("system-info.json")
	if err != nil {
		return
	}

	var memStats runtime.MemStats
	runtime.ReadMemStats(&memStats)

	info := map[string]interface{}{
		"version":       s.version,
		"go_version":    runtime.Version(),
		"os":            runtime.GOOS,
		"arch":          runtime.GOARCH,
		"num_cpu":       runtime.NumCPU(),
		"num_goroutine": runtime.NumGoroutine(),
		"mem_alloc_mb":  memStats.Alloc / (1024 * 1024),
		"mem_sys_mb":    memStats.Sys / (1024 * 1024),
		"timestamp":     time.Now().UTC().Format(time.RFC3339),
	}
	if bi, ok := debug.ReadBuildInfo(); ok {
		info["build_path"] = bi.Path
	}

	data, _ := json.MarshalIndent(info, "", "  ")
	w.Write(data)
}

func (s *Service) addLogs(zw *zip.Writer) {
	entries, err := os.ReadDir(s.logDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || (!strings.HasSuffix(e.Name(), ".log") && !strings.HasSuffix(e.Name(), ".json")) {
			continue
		}
		info, err := e.Info()
		if err != nil || info.Size() > 50*1024*1024 {
			continue // skip files over 50MB
		}
		data, err := os.ReadFile(filepath.Join(s.logDir, e.Name()))
		if err != nil {
			continue
		}
		w, err := zw.Create("logs/" + e.Name())
		if err != nil {
			continue
		}
		w.Write(data)
	}
}

func (s *Service) addHealthSnapshot(zw *zip.Writer) {
	w, err := zw.Create("health-snapshot.json")
	if err != nil {
		return
	}

	statuses := s.GetRecorderStatuses()
	data, _ := json.MarshalIndent(statuses, "", "  ")
	w.Write(data)
}

func (s *Service) addNetworkProbes(zw *zip.Writer) {
	w, err := zw.Create("network-probes.json")
	if err != nil {
		return
	}

	targets := []struct {
		host string
		port int
	}{
		{"8.8.8.8", 53},
		{"1.1.1.1", 53},
		{"127.0.0.1", 8554},
		{"127.0.0.1", 9997},
	}

	results := make([]ProbeResult, 0, len(targets))
	for _, t := range targets {
		results = append(results, probeTarget(t.host, t.port))
	}

	data, _ := json.MarshalIndent(results, "", "  ")
	w.Write(data)
}

// GetRecorderStatuses returns the current status of all recorders.
func (s *Service) GetRecorderStatuses() []RecorderStatus {
	if s.healthProvider == nil {
		return []RecorderStatus{}
	}

	health := s.healthProvider.GetAllRecordingHealth()
	statuses := make([]RecorderStatus, 0, len(health))
	for _, h := range health {
		statuses = append(statuses, RecorderStatus{
			CameraID:      h.CameraID,
			CameraName:    h.CameraName,
			Status:        h.Status,
			LastSegmentAt: h.LastSegmentAt,
			ErrorMessage:  h.ErrorMessage,
			RestartCount:  h.RestartCount,
			UptimeSeconds: h.UptimeSeconds,
		})
	}
	return statuses
}

// QueryLogs reads log files and returns filtered entries.
func (s *Service) QueryLogs(q LogQuery) ([]LogEntry, int, error) {
	if q.Limit <= 0 || q.Limit > 500 {
		q.Limit = 100
	}

	var afterT, beforeT time.Time
	if q.After != "" {
		t, err := time.Parse(time.RFC3339, q.After)
		if err == nil {
			afterT = t
		}
	}
	if q.Before != "" {
		t, err := time.Parse(time.RFC3339, q.Before)
		if err == nil {
			beforeT = t
		}
	}

	searchLower := strings.ToLower(q.Search)
	levelLower := strings.ToLower(q.Level)

	// Collect log entries from all log files.
	entries, err := os.ReadDir(s.logDir)
	if err != nil {
		return nil, 0, fmt.Errorf("read log dir: %w", err)
	}

	var allEntries []LogEntry

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".log") && !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		f, err := os.Open(filepath.Join(s.logDir, e.Name()))
		if err != nil {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(f, 20*1024*1024)) // 20MB max
		f.Close()
		if err != nil {
			continue
		}

		lines := strings.Split(string(data), "\n")
		for _, line := range lines {
			line = strings.TrimSpace(line)
			if line == "" {
				continue
			}

			var entry LogEntry
			if err := json.Unmarshal([]byte(line), &entry); err != nil {
				// Try to parse plain text log lines.
				entry = parsePlainLogLine(line)
				if entry.Timestamp == "" {
					continue
				}
			}

			// Apply filters.
			if levelLower != "" && strings.ToLower(entry.Level) != levelLower {
				continue
			}
			if q.Module != "" && entry.Module != q.Module {
				continue
			}
			if searchLower != "" && !strings.Contains(strings.ToLower(entry.Message), searchLower) {
				continue
			}
			if !afterT.IsZero() || !beforeT.IsZero() {
				entryTime, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
				if err == nil {
					if !afterT.IsZero() && entryTime.Before(afterT) {
						continue
					}
					if !beforeT.IsZero() && entryTime.After(beforeT) {
						continue
					}
				}
			}

			allEntries = append(allEntries, entry)
		}
	}

	// Sort by timestamp descending (newest first).
	sort.Slice(allEntries, func(i, j int) bool {
		return allEntries[i].Timestamp > allEntries[j].Timestamp
	})

	total := len(allEntries)

	// Apply pagination.
	if q.Offset >= len(allEntries) {
		return []LogEntry{}, total, nil
	}
	end := q.Offset + q.Limit
	if end > len(allEntries) {
		end = len(allEntries)
	}

	return allEntries[q.Offset:end], total, nil
}

// parsePlainLogLine attempts to parse a non-JSON log line.
func parsePlainLogLine(line string) LogEntry {
	// Format: [TIMESTAMP] [LEVEL] [MODULE] message
	entry := LogEntry{}

	// Look for timestamp between first pair of brackets.
	if !strings.HasPrefix(line, "[") {
		return entry
	}
	closeBracket := strings.Index(line, "]")
	if closeBracket < 2 {
		return entry
	}
	entry.Timestamp = line[1:closeBracket]
	rest := strings.TrimSpace(line[closeBracket+1:])

	// Level.
	if strings.HasPrefix(rest, "[") {
		idx := strings.Index(rest, "]")
		if idx > 1 {
			entry.Level = strings.ToLower(rest[1:idx])
			rest = strings.TrimSpace(rest[idx+1:])
		}
	}

	// Module.
	if strings.HasPrefix(rest, "[") {
		idx := strings.Index(rest, "]")
		if idx > 1 {
			entry.Module = rest[1:idx]
			rest = strings.TrimSpace(rest[idx+1:])
		}
	}

	entry.Message = rest
	return entry
}

// RunNetworkProbe checks connectivity to a set of targets.
func (s *Service) RunNetworkProbe(targets []string) []ProbeResult {
	results := make([]ProbeResult, 0, len(targets))
	for _, target := range targets {
		host, port := parseTarget(target)
		results = append(results, probeTarget(host, port))
	}
	return results
}

// probeTarget performs a TCP connect probe to host:port.
func probeTarget(host string, port int) ProbeResult {
	addr := fmt.Sprintf("%s:%d", host, port)
	start := time.Now()

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	var d net.Dialer
	conn, err := d.DialContext(ctx, "tcp", addr)
	elapsed := time.Since(start)

	result := ProbeResult{
		Target:    host,
		Port:      port,
		LatencyMs: float64(elapsed.Microseconds()) / 1000.0,
	}

	if err != nil {
		result.Reachable = false
		result.Error = err.Error()
	} else {
		result.Reachable = true
		conn.Close()
	}

	return result
}

func parseTarget(target string) (string, int) {
	host, portStr, err := net.SplitHostPort(target)
	if err != nil {
		return target, 80
	}
	port := 80
	fmt.Sscanf(portStr, "%d", &port)
	return host, port
}

// RunDefaultProbes runs probes against common NVR endpoints.
func (s *Service) RunDefaultProbes() []ProbeResult {
	targets := []struct {
		host string
		port int
	}{
		{"8.8.8.8", 53},         // DNS (internet)
		{"1.1.1.1", 53},         // DNS (internet)
		{"127.0.0.1", 8554},     // RTSP
		{"127.0.0.1", 9997},     // API
		{"127.0.0.1", 8888},     // HLS
		{"127.0.0.1", 8889},     // WebRTC
	}

	ch := make(chan ProbeResult, len(targets))
	for _, t := range targets {
		go func(h string, p int) {
			ch <- probeTarget(h, p)
		}(t.host, t.port)
	}

	results := make([]ProbeResult, 0, len(targets))
	for range targets {
		results = append(results, <-ch)
	}
	return results
}

// Cleanup removes expired bundle files from disk.
func (s *Service) Cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now().UTC()
	for id, b := range s.bundles {
		expires, _ := time.Parse(time.RFC3339, b.ExpiresAt)
		if now.After(expires) {
			b.Status = BundleStatusExpired
			os.Remove(filepath.Join(s.bundleDir, id+".zip"))
		}
	}
}

// Close stops the diagnostics service.
func (s *Service) Close() {
	s.Cleanup()
}
