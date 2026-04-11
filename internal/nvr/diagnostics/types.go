package diagnostics

import "time"

// CollectorBundleStatus represents the lifecycle state of a diagnostics bundle.
type CollectorBundleStatus string

const (
	StatusPending   CollectorBundleStatus = "pending"
	StatusUploading CollectorBundleStatus = "uploading"
	StatusReady     CollectorBundleStatus = "ready"
	StatusExpired   CollectorBundleStatus = "expired"
	StatusFailed    CollectorBundleStatus = "failed"
)

// Bundle holds metadata about a generated support bundle.
type CollectorBundle struct {
	BundleID    string       `json:"bundle_id"`
	Status      CollectorBundleStatus `json:"status"`
	SizeBytes   int64        `json:"size_bytes"`
	Encrypted   bool         `json:"encrypted"`
	StorageKey  string       `json:"storage_key,omitempty"` // object key in temp storage
	ExpiresAt   time.Time    `json:"expires_at"`
	CreatedAt   time.Time    `json:"created_at"`
	Error       string       `json:"error,omitempty"`
	Sections    []string     `json:"sections"` // which sections were collected
	DownloadURL string       `json:"download_url,omitempty"`
}

// GenerateRequest is the input for generating a new support bundle.
type GenerateRequest struct {
	// HoursBack controls how many hours of logs to include (default 24).
	HoursBack int `json:"hours_back"`
	// Sections allows selecting specific data to include. If empty, all
	// sections are collected.
	Sections []string `json:"sections"`
}

// AllSections lists every data section the collector supports.
var AllSections = []string{
	"logs",
	"metrics",
	"cameras",
	"hardware",
	"sidecars",
	"config",
}

// LogEntry is a single structured log line included in the bundle.
type CollectorLogEntry struct {
	Timestamp string                 `json:"timestamp"`
	Level     string                 `json:"level"`
	Module    string                 `json:"module"`
	Message   string                 `json:"message"`
	Fields    map[string]interface{} `json:"fields,omitempty"`
}

// MetricsSnapshot holds a point-in-time capture of system metrics.
type MetricsSnapshot struct {
	CollectedAt string        `json:"collected_at"`
	Samples     []MetricPoint `json:"samples"`
}

// MetricPoint is a single metric data point.
type MetricPoint struct {
	Timestamp  int64   `json:"t"`
	CPUPercent float64 `json:"cpu"`
	MemPercent float64 `json:"mem"`
	MemAllocMB float64 `json:"alloc"`
	MemSysMB   float64 `json:"sys"`
	Goroutines int     `json:"gr"`
}

// CameraState captures the current state of a single camera.
type CameraState struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Status        string `json:"status"`
	RecordingMode string `json:"recording_mode,omitempty"`
	LastSeen      string `json:"last_seen,omitempty"`
}

// HardwareHealth mirrors syscheck.HardwareReport for bundle inclusion.
type HardwareHealth struct {
	CPUCores    int      `json:"cpu_cores"`
	CPUArch     string   `json:"cpu_arch"`
	GOOS        string   `json:"goos"`
	TotalRAMGB  float64  `json:"total_ram_gb"`
	FreeDiskGB  float64  `json:"free_disk_gb"`
	GPUDetected bool     `json:"gpu_detected"`
	NetworkIFs  []string `json:"network_interfaces"`
	Tier        string   `json:"tier"`
}

// SidecarStatus reports health of companion services (e.g. Zitadel, MediaMTX).
type SidecarStatus struct {
	Name    string `json:"name"`
	Status  string `json:"status"` // "running", "stopped", "unknown"
	Version string `json:"version,omitempty"`
	Uptime  string `json:"uptime,omitempty"`
}

// BundleManifest is written into the archive root as manifest.json.
type BundleManifest struct {
	BundleID    string   `json:"bundle_id"`
	GeneratedAt string   `json:"generated_at"`
	HoursBack   int      `json:"hours_back"`
	Sections    []string `json:"sections"`
	Version     string   `json:"version"`
}
