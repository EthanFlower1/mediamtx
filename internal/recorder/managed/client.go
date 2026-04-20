package managed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"runtime"
	"syscall"
	"time"
)

const (
	registerPath  = "/api/v1/recorders/register"
	heartbeatPath = "/api/v1/recorders/heartbeat"

	heartbeatInterval  = 30 * time.Second
	registrationRetry  = 10 * time.Second
	httpTimeout        = 10 * time.Second
)

// HealthProvider supplies runtime health data for heartbeats.
type HealthProvider interface {
	CameraCount() int
	GetRecordingsPath() string
}

// RegistrationPayload is sent once when the recorder first connects.
type RegistrationPayload struct {
	RecorderID string `json:"recorder_id"`
	Hostname   string `json:"hostname"`
	ListenAddr string `json:"listen_addr"`
	Version    string `json:"version"`
	OS         string `json:"os"`
	Arch       string `json:"arch"`
}

// HeartbeatPayload is sent periodically.
type HeartbeatPayload struct {
	RecorderID   string  `json:"recorder_id"`
	Timestamp    string  `json:"timestamp"`
	CameraCount  int     `json:"camera_count"`
	DiskTotalGB  float64 `json:"disk_total_gb"`
	DiskFreeGB   float64 `json:"disk_free_gb"`
	DiskUsedPct  float64 `json:"disk_used_pct"`
	UptimeSec    int64   `json:"uptime_sec"`
	GoRoutines   int     `json:"goroutines"`
}

// Client manages the recorder's relationship with the Directory.
type Client struct {
	cfg      Config
	health   HealthProvider
	version  string
	http     *http.Client
	startedAt time.Time
}

// NewClient creates a managed-mode Directory client.
func NewClient(cfg Config, health HealthProvider, version string) *Client {
	return &Client{
		cfg:       cfg,
		health:    health,
		version:   version,
		http:      &http.Client{Timeout: httpTimeout},
		startedAt: time.Now(),
	}
}

// Run starts the registration + heartbeat loop. It blocks until ctx is cancelled.
func (c *Client) Run(ctx context.Context) {
	c.registerLoop(ctx)

	ticker := time.NewTicker(heartbeatInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := c.sendHeartbeat(ctx); err != nil {
				log.Printf("[managed] heartbeat failed: %v", err)
			}
		}
	}
}

func (c *Client) registerLoop(ctx context.Context) {
	hostname := c.cfg.Hostname
	if hostname == "" {
		hostname, _ = os.Hostname()
	}

	payload := RegistrationPayload{
		RecorderID: c.cfg.RecorderID,
		Hostname:   hostname,
		ListenAddr: c.cfg.ListenAddr(),
		Version:    c.version,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
	}

	for {
		err := c.post(ctx, registerPath, payload)
		if err == nil {
			log.Printf("[managed] registered with Directory at %s (recorder_id=%s)", c.cfg.DirectoryURL, c.cfg.RecorderID)
			return
		}
		log.Printf("[managed] registration failed (retrying in %s): %v", registrationRetry, err)

		select {
		case <-ctx.Done():
			return
		case <-time.After(registrationRetry):
		}
	}
}

func (c *Client) sendHeartbeat(ctx context.Context) error {
	total, free := diskStats(c.health.GetRecordingsPath())
	usedPct := 0.0
	if total > 0 {
		usedPct = float64(total-free) / float64(total) * 100
	}

	payload := HeartbeatPayload{
		RecorderID:  c.cfg.RecorderID,
		Timestamp:   time.Now().UTC().Format(time.RFC3339),
		CameraCount: c.health.CameraCount(),
		DiskTotalGB: float64(total) / (1 << 30),
		DiskFreeGB:  float64(free) / (1 << 30),
		DiskUsedPct: usedPct,
		UptimeSec:   int64(time.Since(c.startedAt).Seconds()),
		GoRoutines:  runtime.NumGoroutine(),
	}

	return c.post(ctx, heartbeatPath, payload)
}

func (c *Client) post(ctx context.Context, path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}

	url := c.cfg.DirectoryURL + path
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.cfg.ServiceToken)

	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("request to %s: %w", path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d from %s", resp.StatusCode, path)
	}
	return nil
}

// diskStats returns total and free bytes for the filesystem containing path.
func diskStats(path string) (total, free uint64) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return 0, 0
	}
	total = stat.Blocks * uint64(stat.Bsize)
	free = stat.Bavail * uint64(stat.Bsize)
	return total, free
}
