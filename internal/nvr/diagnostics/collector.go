package diagnostics

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
)

// DefaultTTL is the lifetime of an uploaded support bundle.
const DefaultTTL = 7 * 24 * time.Hour

// LogProvider returns structured log entries for the given time window.
type LogProvider interface {
	// ReadLogs returns log entries from the last `hours` hours.
	ReadLogs(hours int) ([]LogEntry, error)
}

// MetricsProvider returns current and historical metric samples.
type MetricsProvider interface {
	CurrentMetrics() MetricPoint
	HistoryMetrics() []MetricPoint
}

// CameraProvider returns the state of all configured cameras.
type CameraProvider interface {
	ListCameraStates(ctx context.Context) ([]CameraState, error)
}

// HardwareProvider returns system hardware health.
type HardwareProvider interface {
	GetHardwareHealth() (*HardwareHealth, error)
}

// SidecarProvider returns the status of companion services.
type SidecarProvider interface {
	ListSidecarStatus(ctx context.Context) ([]SidecarStatus, error)
}

// Uploader persists an encrypted bundle and returns a download URL.
type Uploader interface {
	Upload(ctx context.Context, key string, data io.Reader, size int64, ttl time.Duration) (downloadURL string, err error)
	Delete(ctx context.Context, key string) error
}

// IDGenerator produces unique bundle IDs.
type IDGenerator func() string

func defaultIDGen() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// CollectorConfig holds all dependencies for the Collector.
type CollectorConfig struct {
	Logs     LogProvider
	Metrics  MetricsProvider
	Cameras  CameraProvider
	Hardware HardwareProvider
	Sidecars SidecarProvider
	Uploader Uploader

	// EncryptionKey must be 32 bytes for AES-256-GCM. If nil, bundles are not
	// encrypted (useful for development but not recommended for production).
	EncryptionKey []byte

	// Version is the NVR software version string included in the manifest.
	Version string

	// IDGen overrides the default random ID generator (useful for tests).
	IDGen IDGenerator

	// ConfigPath is the path to mediamtx.yml for including a sanitized config
	// snapshot (secrets are redacted).
	ConfigPath string
}

// Collector orchestrates support bundle generation.
type Collector struct {
	cfg CollectorConfig
}

// NewCollector creates a Collector with the given config.
func NewCollector(cfg CollectorConfig) *Collector {
	if cfg.IDGen == nil {
		cfg.IDGen = defaultIDGen
	}
	return &Collector{cfg: cfg}
}

// Generate creates a support bundle synchronously. The returned Bundle contains
// metadata; the actual archive is uploaded to temp storage if an Uploader is
// configured. When no Uploader is present the encrypted archive bytes are
// returned in the second return value for the caller to handle.
func (c *Collector) Generate(ctx context.Context, req GenerateRequest) (*Bundle, []byte, error) {
	if req.HoursBack <= 0 {
		req.HoursBack = 24
	}

	sections := req.Sections
	if len(sections) == 0 {
		sections = AllSections
	}

	bundleID := c.cfg.IDGen()
	now := time.Now().UTC()
	bundle := &Bundle{
		BundleID:  bundleID,
		Status:    StatusPending,
		Encrypted: len(c.cfg.EncryptionKey) > 0,
		ExpiresAt: now.Add(DefaultTTL),
		CreatedAt: now,
		Sections:  sections,
	}

	// Assemble archive contents.
	archiveBuf, err := c.assembleArchive(ctx, bundleID, req.HoursBack, sections)
	if err != nil {
		bundle.Status = StatusFailed
		bundle.Error = err.Error()
		return bundle, nil, fmt.Errorf("assemble archive: %w", err)
	}

	// Encrypt if a key is provided.
	var payload []byte
	if len(c.cfg.EncryptionKey) > 0 {
		payload, err = encrypt(c.cfg.EncryptionKey, archiveBuf)
		if err != nil {
			bundle.Status = StatusFailed
			bundle.Error = err.Error()
			return bundle, nil, fmt.Errorf("encrypt bundle: %w", err)
		}
	} else {
		payload = archiveBuf
	}

	bundle.SizeBytes = int64(len(payload))

	// Upload if an uploader is configured.
	if c.cfg.Uploader != nil {
		bundle.Status = StatusUploading
		storageKey := fmt.Sprintf("support-bundles/%s/%s.tar.gz.enc", bundleID, bundleID)
		if !bundle.Encrypted {
			storageKey = fmt.Sprintf("support-bundles/%s/%s.tar.gz", bundleID, bundleID)
		}
		bundle.StorageKey = storageKey

		url, uploadErr := c.cfg.Uploader.Upload(ctx, storageKey, bytes.NewReader(payload), int64(len(payload)), DefaultTTL)
		if uploadErr != nil {
			bundle.Status = StatusFailed
			bundle.Error = uploadErr.Error()
			return bundle, payload, fmt.Errorf("upload bundle: %w", uploadErr)
		}
		bundle.DownloadURL = url
		bundle.Status = StatusReady
		return bundle, nil, nil
	}

	bundle.Status = StatusReady
	return bundle, payload, nil
}

// assembleArchive builds a tar.gz archive with the requested sections.
func (c *Collector) assembleArchive(ctx context.Context, bundleID string, hoursBack int, sections []string) ([]byte, error) {
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	sectionSet := make(map[string]bool, len(sections))
	for _, s := range sections {
		sectionSet[s] = true
	}

	// Manifest.
	manifest := BundleManifest{
		BundleID:    bundleID,
		GeneratedAt: time.Now().UTC().Format(time.RFC3339),
		HoursBack:   hoursBack,
		Sections:    sections,
		Version:     c.cfg.Version,
	}
	if err := c.addJSON(tw, "manifest.json", manifest); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	// Logs.
	if sectionSet["logs"] && c.cfg.Logs != nil {
		entries, err := c.cfg.Logs.ReadLogs(hoursBack)
		if err != nil {
			// Non-fatal: include error marker.
			c.addJSON(tw, "logs/error.json", map[string]string{"error": err.Error()})
		} else {
			if err := c.addJSON(tw, "logs/structured.json", entries); err != nil {
				return nil, err
			}
		}
	}

	// Metrics.
	if sectionSet["metrics"] && c.cfg.Metrics != nil {
		snap := MetricsSnapshot{
			CollectedAt: time.Now().UTC().Format(time.RFC3339),
			Samples:     c.cfg.Metrics.HistoryMetrics(),
		}
		if err := c.addJSON(tw, "metrics/snapshot.json", snap); err != nil {
			return nil, err
		}
		current := c.cfg.Metrics.CurrentMetrics()
		if err := c.addJSON(tw, "metrics/current.json", current); err != nil {
			return nil, err
		}
	}

	// Camera states.
	if sectionSet["cameras"] && c.cfg.Cameras != nil {
		states, err := c.cfg.Cameras.ListCameraStates(ctx)
		if err != nil {
			c.addJSON(tw, "cameras/error.json", map[string]string{"error": err.Error()})
		} else {
			if err := c.addJSON(tw, "cameras/states.json", states); err != nil {
				return nil, err
			}
		}
	}

	// Hardware health.
	if sectionSet["hardware"] && c.cfg.Hardware != nil {
		hw, err := c.cfg.Hardware.GetHardwareHealth()
		if err != nil {
			c.addJSON(tw, "hardware/error.json", map[string]string{"error": err.Error()})
		} else {
			if err := c.addJSON(tw, "hardware/report.json", hw); err != nil {
				return nil, err
			}
		}
	}

	// Sidecars.
	if sectionSet["sidecars"] && c.cfg.Sidecars != nil {
		statuses, err := c.cfg.Sidecars.ListSidecarStatus(ctx)
		if err != nil {
			c.addJSON(tw, "sidecars/error.json", map[string]string{"error": err.Error()})
		} else {
			if err := c.addJSON(tw, "sidecars/status.json", statuses); err != nil {
				return nil, err
			}
		}
	}

	// Sanitized config.
	if sectionSet["config"] && c.cfg.ConfigPath != "" {
		configData, err := readSanitizedConfig(c.cfg.ConfigPath)
		if err != nil {
			c.addJSON(tw, "config/error.json", map[string]string{"error": err.Error()})
		} else {
			if err := addFile(tw, "config/mediamtx-sanitized.yml", configData); err != nil {
				return nil, err
			}
		}
	}

	if err := tw.Close(); err != nil {
		return nil, fmt.Errorf("close tar: %w", err)
	}
	if err := gw.Close(); err != nil {
		return nil, fmt.Errorf("close gzip: %w", err)
	}

	return buf.Bytes(), nil
}

// addJSON marshals v and writes it as a tar entry.
func (c *Collector) addJSON(tw *tar.Writer, name string, v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", name, err)
	}
	return addFile(tw, name, data)
}

// addFile writes raw bytes as a tar entry.
func addFile(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{
		Name:    name,
		Size:    int64(len(data)),
		Mode:    0o644,
		ModTime: time.Now().UTC(),
	}
	if err := tw.WriteHeader(hdr); err != nil {
		return fmt.Errorf("write header %s: %w", name, err)
	}
	if _, err := tw.Write(data); err != nil {
		return fmt.Errorf("write body %s: %w", name, err)
	}
	return nil
}

// encrypt performs AES-256-GCM encryption. The output format is:
// [12-byte nonce][ciphertext+tag]
func encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("generate nonce: %w", err)
	}
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return ciphertext, nil
}

// Decrypt performs AES-256-GCM decryption (inverse of encrypt).
func Decrypt(key, ciphertext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("create cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("create GCM: %w", err)
	}
	nonceSize := gcm.NonceSize()
	if len(ciphertext) < nonceSize {
		return nil, fmt.Errorf("ciphertext too short")
	}
	nonce, ct := ciphertext[:nonceSize], ciphertext[nonceSize:]
	plaintext, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return plaintext, nil
}

// readSanitizedConfig reads the YAML config and redacts sensitive fields.
func readSanitizedConfig(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	// Redact known sensitive keys line-by-line.
	sensitiveKeys := []string{
		"nvrJWTSecret",
		"password",
		"secret",
		"apiKey",
		"api_key",
		"token",
		"credential",
	}

	lines := strings.Split(string(data), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		for _, key := range sensitiveKeys {
			if strings.HasPrefix(trimmed, key+":") || strings.HasPrefix(trimmed, key+" :") {
				// Keep the key but redact the value.
				parts := strings.SplitN(line, ":", 2)
				if len(parts) == 2 {
					lines[i] = parts[0] + ": [REDACTED]"
				}
				break
			}
		}
	}

	return []byte(strings.Join(lines, "\n")), nil
}

// ReadBundleArchive extracts file names from a tar.gz archive (for verification).
func ReadBundleArchive(data []byte) ([]string, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	var names []string
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		names = append(names, hdr.Name)
	}
	return names, nil
}

// ReadArchiveFile extracts a single file from a tar.gz archive by name.
func ReadArchiveFile(data []byte, name string) ([]byte, error) {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Name == name {
			return io.ReadAll(tr)
		}
	}
	return nil, fmt.Errorf("file %q not found in archive", name)
}

// CleanExpired is a helper suitable for periodic invocation (e.g. via cron or
// background goroutine). It deletes bundles from storage whose ExpiresAt has
// passed. The caller provides the list of bundles and this function calls
// Uploader.Delete for each expired one.
func (c *Collector) CleanExpired(ctx context.Context, bundles []Bundle) (deleted int, _ error) {
	if c.cfg.Uploader == nil {
		return 0, nil
	}
	now := time.Now().UTC()
	for _, b := range bundles {
		if b.StorageKey != "" && now.After(b.ExpiresAt) {
			if err := c.cfg.Uploader.Delete(ctx, b.StorageKey); err != nil {
				return deleted, fmt.Errorf("delete %s: %w", b.BundleID, err)
			}
			deleted++
		}
	}
	return deleted, nil
}

