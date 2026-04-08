package tsnet

import (
	"context"
	"net"
	"net/netip"
)

// Node is a single participant on the mesh tailnet. It wraps a
// tsnet.Server in real mode and a process-local loopback stand-in in
// test mode. All methods are safe to call from multiple goroutines.
type Node struct {
	cfg     NodeConfig
	backend nodeBackend
}

// nodeBackend is the minimal interface that both the real tsnet
// implementation and the test-mode loopback implementation satisfy.
// Keeping it unexported means callers of the package always use *Node
// and never couple to a particular backend.
type nodeBackend interface {
	Listen(network, addr string) (net.Listener, error)
	Dial(ctx context.Context, network, addr string) (net.Conn, error)
	Addr() netip.Addr
	Shutdown(ctx context.Context) error
}

// New constructs a Node from the given config. It validates the
// config, then dispatches to either the real tsnet backend or the
// hermetic test-mode backend depending on cfg.TestMode.
//
// In real mode the returned Node has already started its tsnet.Server
// and is ready to Listen or Dial. The caller owns the Node and must
// call Shutdown to release resources.
func New(cfg NodeConfig) (*Node, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	var (
		backend nodeBackend
		err     error
	)
	if cfg.TestMode {
		backend, err = newTestBackend(cfg)
	} else {
		backend, err = newRealBackend(cfg)
	}
	if err != nil {
		return nil, err
	}

	return &Node{cfg: cfg, backend: backend}, nil
}

// Listen returns a listener bound to the Node's tailnet address.
// Typical networks are "tcp" and "tcp4"; typical addresses are
// ":8443" (bind to every tailnet interface on port 8443).
func (n *Node) Listen(network, addr string) (net.Listener, error) {
	return n.backend.Listen(network, addr)
}

// Dial connects to another mesh node by hostname. In real mode the
// hostname is resolved via MagicDNS on the tailnet; in test mode it
// is looked up in the in-memory registry and will return ErrUnknownHost
// if no Node with that hostname exists in the current process.
func (n *Node) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	return n.backend.Dial(ctx, network, addr)
}

// Addr reports the Node's stable tailnet address. In real mode this
// is a 100.64.0.0/10 CGNAT address assigned by the coordinator; in
// test mode it is a synthetic 127.x.y.z address derived from the
// hostname hash.
func (n *Node) Addr() netip.Addr {
	return n.backend.Addr()
}

// Shutdown stops the Node and releases any resources it holds.
// Subsequent calls to Listen or Dial return ErrClosed.
func (n *Node) Shutdown(ctx context.Context) error {
	return n.backend.Shutdown(ctx)
}

// Hostname reports the configured hostname, unchanged.
func (n *Node) Hostname() string {
	return n.cfg.Hostname
}
