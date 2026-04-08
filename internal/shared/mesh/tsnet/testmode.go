package tsnet

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"net/netip"
	"sync"
)

// testRegistry is the process-wide in-memory directory of test-mode
// nodes. It lets two Nodes created in the same process connect to each
// other by hostname without touching the real network.
var testRegistry = struct {
	mu    sync.RWMutex
	nodes map[string]*testBackend
}{nodes: make(map[string]*testBackend)}

// testBackend is the hermetic loopback implementation of nodeBackend.
// It accepts on a local loopback TCP listener and routes Dials to the
// listener of the target hostname through the shared registry.
type testBackend struct {
	hostname string
	addr     netip.Addr

	mu       sync.Mutex
	listener net.Listener // singleton: only one Listen per node
	closed   bool
}

func newTestBackend(cfg NodeConfig) (nodeBackend, error) {
	b := &testBackend{
		hostname: cfg.Hostname,
		addr:     syntheticAddr(cfg.Hostname),
	}
	testRegistry.mu.Lock()
	defer testRegistry.mu.Unlock()
	if _, exists := testRegistry.nodes[cfg.Hostname]; exists {
		return nil, fmt.Errorf("tsnet: test-mode hostname %q already registered", cfg.Hostname)
	}
	testRegistry.nodes[cfg.Hostname] = b
	return b, nil
}

// syntheticAddr derives a deterministic 127.x.y.z address from the
// hostname. The first octet is always 127 so the result is always
// loopback-routable on every OS the build supports.
func syntheticAddr(hostname string) netip.Addr {
	sum := sha256.Sum256([]byte(hostname))
	var b [4]byte
	b[0] = 127
	b[1] = sum[0]
	b[2] = sum[1]
	// Avoid .0 and .255 which are network/broadcast on some loopbacks.
	b[3] = sum[2]
	if b[3] == 0 {
		b[3] = 1
	} else if b[3] == 255 {
		b[3] = 254
	}
	_ = binary.BigEndian // keep import stable if we swap encodings
	return netip.AddrFrom4(b)
}

func (b *testBackend) Listen(network, addr string) (net.Listener, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil, ErrClosed
	}
	if b.listener != nil {
		return nil, errors.New("tsnet: test-mode node already has a listener")
	}
	// Always bind loopback regardless of the caller's requested
	// address so tests never fight over real ports.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	b.listener = ln
	return ln, nil
}

func (b *testBackend) Dial(ctx context.Context, network, addr string) (net.Conn, error) {
	b.mu.Lock()
	if b.closed {
		b.mu.Unlock()
		return nil, ErrClosed
	}
	b.mu.Unlock()

	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		// Allow bare hostnames as a convenience.
		host = addr
	}

	testRegistry.mu.RLock()
	target, ok := testRegistry.nodes[host]
	testRegistry.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %q", ErrUnknownHost, host)
	}

	target.mu.Lock()
	ln := target.listener
	closed := target.closed
	target.mu.Unlock()
	if closed {
		return nil, ErrClosed
	}
	if ln == nil {
		return nil, fmt.Errorf("%w: %q has no listener", ErrUnknownHost, host)
	}

	d := net.Dialer{}
	return d.DialContext(ctx, "tcp", ln.Addr().String())
}

func (b *testBackend) Addr() netip.Addr {
	return b.addr
}

func (b *testBackend) Shutdown(_ context.Context) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.closed {
		return nil
	}
	b.closed = true
	var err error
	if b.listener != nil {
		err = b.listener.Close()
		b.listener = nil
	}
	testRegistry.mu.Lock()
	delete(testRegistry.nodes, b.hostname)
	testRegistry.mu.Unlock()
	return err
}
