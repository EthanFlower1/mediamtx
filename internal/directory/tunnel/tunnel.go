// Package tunnel wraps frp's client library to create reverse tunnels
// from the local Directory services to the cloud relay.
package tunnel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"

	"github.com/fatedier/frp/client"
	v1 "github.com/fatedier/frp/pkg/config/v1"
	"github.com/fatedier/frp/pkg/config/source"
)

// LocalPorts defines the local service ports to tunnel.
type LocalPorts struct {
	API      int // Directory API (default 9995)
	HLS      int // HLS streaming (default 8898)
	WebRTC   int // WebRTC (default 8889)
	RTSP     int // RTSP (default 8554)
	Playback int // Playback (default 9996)
}

// Config holds all parameters needed to establish a tunnel.
type Config struct {
	ServerAddr string     // frp server address
	ServerPort int        // frp server port (typically 7000)
	Token      string     // shared auth token
	SubDomain  string     // site alias used as subdomain
	LocalPorts LocalPorts // local service ports
	Logger     *slog.Logger
}

// Validate checks that all required fields are populated.
func (c *Config) Validate() error {
	if c.ServerAddr == "" {
		return errors.New("tunnel: ServerAddr is required")
	}
	if c.ServerPort <= 0 {
		return errors.New("tunnel: ServerPort must be positive")
	}
	if c.Token == "" {
		return errors.New("tunnel: Token is required")
	}
	if c.SubDomain == "" {
		return errors.New("tunnel: SubDomain is required")
	}
	if c.LocalPorts.API <= 0 {
		return errors.New("tunnel: LocalPorts.API must be positive")
	}
	return nil
}

// Tunnel manages an frp client that tunnels local services to a cloud relay.
// It starts a local reverse proxy (mux) that merges all service ports behind
// one port, then tunnels that single port via frp.
type Tunnel struct {
	cfg    Config
	svc    *client.Service
	muxLn  net.Listener // local mux listener
	cancel context.CancelFunc
	mu     sync.Mutex
	log    *slog.Logger
}

// New validates the config and creates a Tunnel ready to run.
func New(cfg Config) (*Tunnel, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	log := cfg.Logger
	if log == nil {
		log = slog.Default()
	}

	// Build frp common config.
	commonCfg := &v1.ClientCommonConfig{
		ServerAddr: cfg.ServerAddr,
		ServerPort: cfg.ServerPort,
		Auth: v1.AuthClientConfig{
			Method: v1.AuthMethodToken,
			Token:  cfg.Token,
		},
	}
	if err := commonCfg.Complete(); err != nil {
		return nil, fmt.Errorf("tunnel: completing common config: %w", err)
	}

	// Start local mux that merges all service ports behind one port.
	muxLn, err := startLocalMux(cfg.LocalPorts)
	if err != nil {
		return nil, fmt.Errorf("tunnel: starting local mux: %w", err)
	}
	muxPort := muxLn.Addr().(*net.TCPAddr).Port
	log.Info("tunnel: local mux started", "port", muxPort)

	// Single frp proxy pointing at the mux port — no path conflicts.
	proxy := &v1.HTTPProxyConfig{
		ProxyBaseConfig: v1.ProxyBaseConfig{
			Name: cfg.SubDomain,
			Type: string(v1.ProxyTypeHTTP),
			ProxyBackend: v1.ProxyBackend{
				LocalIP:   "127.0.0.1",
				LocalPort: muxPort,
			},
		},
		DomainConfig: v1.DomainConfig{
			SubDomain: cfg.SubDomain,
		},
	}

	cfgSource := source.NewConfigSource()
	if err := cfgSource.ReplaceAll([]v1.ProxyConfigurer{proxy}, nil); err != nil {
		muxLn.Close()
		return nil, fmt.Errorf("tunnel: replacing config source: %w", err)
	}
	aggregator := source.NewAggregator(cfgSource)

	svc, err := client.NewService(client.ServiceOptions{
		Common:                 commonCfg,
		ConfigSourceAggregator: aggregator,
	})
	if err != nil {
		muxLn.Close()
		return nil, fmt.Errorf("tunnel: creating frp service: %w", err)
	}

	return &Tunnel{
		cfg:   cfg,
		svc:   svc,
		muxLn: muxLn,
		log:   log,
	}, nil
}

// Run connects to the frp server and blocks until ctx is cancelled or an error
// occurs. It is safe to call Stop from another goroutine.
func (t *Tunnel) Run(ctx context.Context) error {
	t.mu.Lock()
	ctx, t.cancel = context.WithCancel(ctx)
	t.mu.Unlock()

	t.log.Info("tunnel: connecting to relay",
		"server", fmt.Sprintf("%s:%d", t.cfg.ServerAddr, t.cfg.ServerPort),
		"subdomain", t.cfg.SubDomain,
	)
	return t.svc.Run(ctx)
}

// Stop gracefully shuts down the tunnel and the local mux.
func (t *Tunnel) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancel != nil {
		t.cancel()
	}
	t.svc.Close()
	if t.muxLn != nil {
		t.muxLn.Close()
	}
}

