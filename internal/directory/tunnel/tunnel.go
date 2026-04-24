// Package tunnel wraps frp's client library to create reverse tunnels
// from the local Directory services to the cloud relay.
package tunnel

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
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
type Tunnel struct {
	cfg    Config
	svc    *client.Service
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

	// Build HTTP proxy rules.
	proxies := buildProxies(cfg)

	// Wire up the config source.
	cfgSource := source.NewConfigSource()
	if err := cfgSource.ReplaceAll(proxies, nil); err != nil {
		return nil, fmt.Errorf("tunnel: replacing config source: %w", err)
	}
	aggregator := source.NewAggregator(cfgSource)

	svc, err := client.NewService(client.ServiceOptions{
		Common:                 commonCfg,
		ConfigSourceAggregator: aggregator,
	})
	if err != nil {
		return nil, fmt.Errorf("tunnel: creating frp service: %w", err)
	}

	return &Tunnel{
		cfg: cfg,
		svc: svc,
		log: log,
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

// Stop gracefully shuts down the tunnel.
func (t *Tunnel) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.cancel != nil {
		t.cancel()
	}
	t.svc.Close()
}

// proxySpec describes a single HTTP proxy rule.
type proxySpec struct {
	name      string
	localPort int
	locations []string
}

// buildProxies constructs frp HTTP proxy configurers from the tunnel config.
func buildProxies(cfg Config) []v1.ProxyConfigurer {
	specs := []proxySpec{
		{
			name:      cfg.SubDomain + "-api",
			localPort: cfg.LocalPorts.API,
			locations: []string{"/api/", "/healthz", "/admin"},
		},
		{
			name:      cfg.SubDomain + "-hls",
			localPort: cfg.LocalPorts.HLS,
			locations: []string{"/nvr/"},
		},
		{
			name:      cfg.SubDomain + "-webrtc",
			localPort: cfg.LocalPorts.WebRTC,
			locations: []string{"/whep", "/whip"},
		},
		{
			name:      cfg.SubDomain + "-playback",
			localPort: cfg.LocalPorts.Playback,
			locations: []string{"/list", "/get", "/seek"},
		},
	}

	var proxies []v1.ProxyConfigurer
	for _, s := range specs {
		if s.localPort <= 0 {
			continue
		}
		p := &v1.HTTPProxyConfig{
			ProxyBaseConfig: v1.ProxyBaseConfig{
				Name: s.name,
				Type: string(v1.ProxyTypeHTTP),
				ProxyBackend: v1.ProxyBackend{
					LocalIP:   "127.0.0.1",
					LocalPort: s.localPort,
				},
			},
			DomainConfig: v1.DomainConfig{
				SubDomain: cfg.SubDomain,
			},
			Locations: s.locations,
		}
		proxies = append(proxies, p)
	}
	return proxies
}
