//go:build tsnetstub

package tsnet

import (
	"context"
	"errors"
	"net"
	"net/netip"
)

// stubBackend is substituted for realBackend when the tsnetstub build
// tag is set. It always returns a "tsnet disabled" error. The stub
// exists so that environments that cannot or do not want to compile
// the full tailscale.com dependency tree (gvisor, wireguard-go) can
// still build the rest of the codebase; those builds must run the
// on-prem components in TestMode.
type stubBackend struct{}

var errStubDisabled = errors.New("tsnet: real backend disabled by tsnetstub build tag (use TestMode)")

func newRealBackend(_ NodeConfig) (nodeBackend, error) {
	return nil, errStubDisabled
}

func (stubBackend) Listen(_, _ string) (net.Listener, error) {
	return nil, errStubDisabled
}

func (stubBackend) Dial(_ context.Context, _, _ string) (net.Conn, error) {
	return nil, errStubDisabled
}

func (stubBackend) Addr() netip.Addr { return netip.Addr{} }

func (stubBackend) Shutdown(_ context.Context) error { return nil }
