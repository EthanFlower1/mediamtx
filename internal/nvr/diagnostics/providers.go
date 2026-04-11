package diagnostics

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bluenviron/mediamtx/internal/nvr/metrics"
	"github.com/bluenviron/mediamtx/internal/nvr/syscheck"
)

// ----- File-based Log Provider -----

// FileLogProvider reads structured JSON logs from the NVR log directory.
type FileLogProvider struct {
	LogDir string
}

// ReadLogs scans log files and returns entries within the specified time window.
func (p *FileLogProvider) ReadLogs(hours int) ([]LogEntry, error) {
	cutoff := time.Now().UTC().Add(-time.Duration(hours) * time.Hour)

	var entries []LogEntry

	// Read current log file.
	logPath := filepath.Join(p.LogDir, "nvr.log")
	fileEntries, err := readLogFile(logPath, cutoff)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read %s: %w", logPath, err)
	}
	entries = append(entries, fileEntries...)

	// Read rotated log files (they match nvr-*.log).
	dirEntries, err := os.ReadDir(p.LogDir)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read dir %s: %w", p.LogDir, err)
	}
	for _, e := range dirEntries {
		if e.IsDir() || !strings.HasPrefix(e.Name(), "nvr-") || !strings.HasSuffix(e.Name(), ".log") {
			continue
		}
		path := filepath.Join(p.LogDir, e.Name())
		rotatedEntries, err := readLogFile(path, cutoff)
		if err != nil {
			continue // best-effort for rotated files
		}
		entries = append(entries, rotatedEntries...)
	}

	return entries, nil
}

func readLogFile(path string, cutoff time.Time) ([]LogEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []LogEntry
	scanner := bufio.NewScanner(f)
	// Allow for large log lines.
	scanner.Buffer(make([]byte, 0, 1024*1024), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry LogEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue // skip non-JSON lines
		}
		// Filter by time.
		if entry.Timestamp != "" {
			t, err := time.Parse(time.RFC3339Nano, entry.Timestamp)
			if err == nil && t.Before(cutoff) {
				continue
			}
		}
		entries = append(entries, entry)
	}
	return entries, scanner.Err()
}

// ----- Metrics Provider wrapping the existing ring-buffer collector -----

// RingMetricsProvider adapts the nvr/metrics.Collector to the MetricsProvider
// interface.
type RingMetricsProvider struct {
	Collector *metrics.Collector
}

// CurrentMetrics returns the latest sample from the ring buffer.
func (p *RingMetricsProvider) CurrentMetrics() MetricPoint {
	s := p.Collector.Current()
	return MetricPoint{
		Timestamp:  s.Timestamp,
		CPUPercent: s.CPUPercent,
		MemPercent: s.MemPercent,
		MemAllocMB: s.MemAllocMB,
		MemSysMB:   s.MemSysMB,
		Goroutines: s.Goroutines,
	}
}

// HistoryMetrics returns all samples from the ring buffer.
func (p *RingMetricsProvider) HistoryMetrics() []MetricPoint {
	history := p.Collector.History()
	out := make([]MetricPoint, len(history))
	for i, s := range history {
		out[i] = MetricPoint{
			Timestamp:  s.Timestamp,
			CPUPercent: s.CPUPercent,
			MemPercent: s.MemPercent,
			MemAllocMB: s.MemAllocMB,
			MemSysMB:   s.MemSysMB,
			Goroutines: s.Goroutines,
		}
	}
	return out
}

// ----- Hardware Provider wrapping syscheck -----

// SysCheckHardwareProvider adapts syscheck.GenerateReport to HardwareProvider.
type SysCheckHardwareProvider struct {
	RecordingsPath string
}

// GetHardwareHealth runs syscheck and converts the result.
func (p *SysCheckHardwareProvider) GetHardwareHealth() (*HardwareHealth, error) {
	report, err := syscheck.GenerateReport(p.RecordingsPath)
	if err != nil {
		return nil, err
	}
	const gb = 1024 * 1024 * 1024
	return &HardwareHealth{
		CPUCores:    report.CPUCores,
		CPUArch:     report.CPUArch,
		GOOS:        report.GOOS,
		TotalRAMGB:  float64(report.TotalRAM) / gb,
		FreeDiskGB:  float64(report.FreeDisk) / gb,
		GPUDetected: report.GPUDetected,
		NetworkIFs:  report.NetworkIFs,
		Tier:        string(report.Tier),
	}, nil
}

// ----- Default Sidecar Provider -----

// DefaultSidecarProvider probes known sidecar services by attempting TCP
// connections to their expected ports.
type DefaultSidecarProvider struct {
	// Sidecars maps service name to its expected address (e.g. "localhost:8080").
	Sidecars map[string]string
}

// ListSidecarStatus probes each configured sidecar.
func (p *DefaultSidecarProvider) ListSidecarStatus(_ context.Context) ([]SidecarStatus, error) {
	var out []SidecarStatus
	for name, addr := range p.Sidecars {
		status := "unknown"
		conn, err := net.DialTimeout("tcp", addr, 2*time.Second)
		if err == nil {
			conn.Close()
			status = "running"
		} else {
			status = "stopped"
		}
		out = append(out, SidecarStatus{
			Name:   name,
			Status: status,
		})
	}

	// If using HTTP health endpoints, try those too.
	for i, s := range out {
		if s.Status == "running" {
			addr := p.Sidecars[s.Name]
			url := fmt.Sprintf("http://%s/health", addr)
			client := &http.Client{Timeout: 2 * time.Second}
			resp, err := client.Get(url)
			if err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					out[i].Status = "running"
				}
			}
		}
	}

	return out, nil
}
