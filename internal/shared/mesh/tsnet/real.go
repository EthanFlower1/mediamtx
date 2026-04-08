//go:build !tsnetstub

package tsnet

import (
	"context"
	"fmt"
	"net"
	"net/netip"

	tstsnet "tailscale.com/tsnet"
)

// realBackend wraps a real tailscale.com/tsnet.Server. It is compiled
// by default; pass -tags tsnetstub to the Go toolchain to substitute
// the stub backend in stub.go instead (see the README).
type realBackend struct {
	srv *tstsnet.Server
}

func newRealBackend(cfg NodeConfig) (nodeBackend, error) {
	srv := &tstsnet.Server{
		Hostname:   cfg.Hostname,
		AuthKey:    cfg.AuthKey,
		Dir:        cfg.StateDir,
		ControlURL: cfg.ControlURL,
		Ephemeral:  false,
	}
	if cfg.Logf != nil {
		// Only wire the UserLogf channel (auth URL, status). Backend
		// log is too noisy for production.
		srv.UserLogf = func(format string, args ...any) { cfg.Logf(format, args...) }
	}
	if err := srv.Start(); err != nil {
		return nil, fmt.Errorf("tsnet: start: %w", err)
	}
	// Block until the node is actually up so callers that immediately
	// Dial don't race the coordinator handshake.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if _, err := srv.Up(ctx); err != nil {
		_ = srv.Close()
		return nil, fmt.Errorf("tsnet: up: %w", err)
	}
	return &realBackend{srv: srv}, nil
}

func (b *realBackend) Listen(network, addr string) (net.Listener, error) {
	return b.srv.Listen(network, addr)
}

func (b *realBackend) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	return b.srv.Dial(ctx, network, addr)
}

func (b *realBackend) Addr() netip.Addr {
	v4, _ := b.srv.TailscaleIPs()
	return v4
}

func (b *realBackend) Shutdown(_ context.Context) error {
	return b.srv.Close()
}
