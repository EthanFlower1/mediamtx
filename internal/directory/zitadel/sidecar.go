// Package zitadel implements the Sidecar interface for managing a Zitadel
// identity provider as a supervised subprocess on the Directory host.
//
// Zitadel is deployed as a single-binary sidecar alongside the Directory.
// It listens on localhost only (127.0.0.1:8080 gRPC, :8081 HTTP console)
// and uses a local CockroachDB-compatible SQLite backend via the built-in
// "start-from-init" command.
//
// The Directory supervisor (internal/shared/sidecar) manages the lifecycle:
// start, health-check, crash restart with exponential backoff, and graceful
// shutdown. This package provides:
//
//   - ZitadelSidecar: implements sidecar.Sidecar
//   - ZitadelConfig: encrypted-at-rest configuration (master key, ports, TLS)
//   - Health check via Zitadel's /debug/healthz endpoint
package zitadel

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

// Config holds the Zitadel sidecar configuration. Sensitive fields (MasterKey,
// TLSKeyPEM) are encrypted at rest by the cryptostore (KAI-251).
type Config struct {
	// BinaryPath is the path to the Zitadel binary. Typically
	// /opt/kaivue/bin/zitadel on production installs.
	BinaryPath string

	// DataDir is the directory where Zitadel stores its database and
	// runtime state. Must be persistent across restarts.
	DataDir string

	// MasterKey is the 32-byte Zitadel master encryption key. Used to
	// encrypt Zitadel's internal secrets (OIDC client secrets, etc.).
	// Stored encrypted via cryptostore at rest.
	MasterKey string

	// GRPCPort is the local gRPC port. Default: 8080.
	GRPCPort int

	// HTTPPort is the local HTTP port for the console and OIDC endpoints.
	// Default: 8081.
	HTTPPort int

	// ExternalDomain is the domain clients use to reach Zitadel.
	// On-prem this is typically the Directory's LAN hostname.
	ExternalDomain string

	// ExternalPort is the port clients use. Typically matches HTTPPort
	// for direct access, or 443 if behind a reverse proxy.
	ExternalPort int

	// ExternalSecure indicates whether clients reach Zitadel over TLS.
	ExternalSecure bool

	// TLSCertPEM and TLSKeyPEM are optional TLS credentials for Zitadel's
	// built-in TLS termination. If empty, Zitadel runs plain HTTP (suitable
	// when behind the Directory's TLS reverse proxy).
	TLSCertPEM string
	TLSKeyPEM  string

	// Logger is the base logger. Nil uses slog.Default().
	Logger *slog.Logger
}

func (c *Config) grpcPort() int {
	if c.GRPCPort > 0 {
		return c.GRPCPort
	}
	return 8080
}

func (c *Config) httpPort() int {
	if c.HTTPPort > 0 {
		return c.HTTPPort
	}
	return 8081
}

func (c *Config) externalPort() int {
	if c.ExternalPort > 0 {
		return c.ExternalPort
	}
	return c.httpPort()
}

func (c *Config) logger() *slog.Logger {
	if c.Logger != nil {
		return c.Logger
	}
	return slog.Default()
}

// ZitadelSidecar implements sidecar.Sidecar for the Zitadel identity provider.
type ZitadelSidecar struct {
	cfg Config
	log *slog.Logger

	mu      sync.Mutex
	onReady func() // optional callback
}

// New creates a ZitadelSidecar with the given configuration.
func New(cfg Config) (*ZitadelSidecar, error) {
	if cfg.BinaryPath == "" {
		return nil, fmt.Errorf("zitadel: BinaryPath is required")
	}
	if cfg.DataDir == "" {
		return nil, fmt.Errorf("zitadel: DataDir is required")
	}
	if cfg.MasterKey == "" {
		return nil, fmt.Errorf("zitadel: MasterKey is required")
	}
	if cfg.ExternalDomain == "" {
		return nil, fmt.Errorf("zitadel: ExternalDomain is required")
	}

	return &ZitadelSidecar{
		cfg: cfg,
		log: cfg.logger().With("component", "zitadel-sidecar"),
	}, nil
}

// SetOnReady registers a callback invoked when the sidecar becomes healthy.
func (z *ZitadelSidecar) SetOnReady(fn func()) {
	z.mu.Lock()
	z.onReady = fn
	z.mu.Unlock()
}

// --- sidecar.Sidecar interface ---

// Name returns "zitadel".
func (z *ZitadelSidecar) Name() string { return "zitadel" }

// Command returns a fresh exec.Cmd for "zitadel start-from-init".
// This Zitadel command initializes the database on first run and starts
// the server. On subsequent runs it detects the existing DB and just starts.
func (z *ZitadelSidecar) Command(ctx context.Context) *exec.Cmd {
	args := []string{
		"start-from-init",
		"--masterkeyFromEnv",
		"--tlsMode", z.tlsMode(),
		"--port", fmt.Sprintf("%d", z.cfg.grpcPort()),
		"--externalDomain", z.cfg.ExternalDomain,
		"--externalPort", fmt.Sprintf("%d", z.cfg.externalPort()),
	}
	if z.cfg.ExternalSecure {
		args = append(args, "--externalSecure")
	}

	cmd := exec.CommandContext(ctx, z.cfg.BinaryPath, args...)
	return cmd
}

// HealthCheck probes Zitadel's /debug/healthz endpoint.
func (z *ZitadelSidecar) HealthCheck(ctx context.Context) error {
	url := fmt.Sprintf("http://127.0.0.1:%d/debug/healthz", z.cfg.httpPort())

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("zitadel health: build request: %w", err)
	}

	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("zitadel health: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("zitadel health: status %d", resp.StatusCode)
	}
	return nil
}

// OnReady is called by the supervisor when HealthCheck first passes.
func (z *ZitadelSidecar) OnReady() {
	z.log.Info("zitadel sidecar is ready",
		slog.Int("grpc_port", z.cfg.grpcPort()),
		slog.Int("http_port", z.cfg.httpPort()))

	z.mu.Lock()
	fn := z.onReady
	z.mu.Unlock()
	if fn != nil {
		fn()
	}
}

// Env returns environment variables for the Zitadel subprocess.
func (z *ZitadelSidecar) Env() []string {
	env := []string{
		"ZITADEL_MASTERKEY=" + z.cfg.MasterKey,
		fmt.Sprintf("ZITADEL_PORT=%d", z.cfg.grpcPort()),
		// Use the data dir for Zitadel's embedded CockroachDB.
		"ZITADEL_DATABASE_SQLITE_PATH=" + filepath.Join(z.cfg.DataDir, "zitadel.db"),
	}
	if z.cfg.TLSCertPEM != "" {
		env = append(env,
			"ZITADEL_TLS_CERTPATH="+filepath.Join(z.cfg.DataDir, "tls.crt"),
			"ZITADEL_TLS_KEYPATH="+filepath.Join(z.cfg.DataDir, "tls.key"),
		)
	}
	return env
}

// WorkDir returns the Zitadel data directory.
func (z *ZitadelSidecar) WorkDir() string { return z.cfg.DataDir }

func (z *ZitadelSidecar) tlsMode() string {
	if z.cfg.TLSCertPEM != "" && z.cfg.TLSKeyPEM != "" {
		return "enabled"
	}
	return "disabled"
}

// HealthEndpoint returns the URL used for health checks. Useful for diagnostics.
func (z *ZitadelSidecar) HealthEndpoint() string {
	return fmt.Sprintf("http://127.0.0.1:%d/debug/healthz", z.cfg.httpPort())
}

// GRPCEndpoint returns the local gRPC endpoint. Used by the Zitadel adapter
// (KAI-132) to connect to the sidecar.
func (z *ZitadelSidecar) GRPCEndpoint() string {
	return fmt.Sprintf("127.0.0.1:%d", z.cfg.grpcPort())
}

// HTTPEndpoint returns the local HTTP endpoint. Used for OIDC discovery
// and the admin console.
func (z *ZitadelSidecar) HTTPEndpoint() string {
	return fmt.Sprintf("http://127.0.0.1:%d", z.cfg.httpPort())
}
