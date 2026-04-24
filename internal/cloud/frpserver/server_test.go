package frpserver

import (
	"net"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func freePort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port
}

func TestNew(t *testing.T) {
	bindPort := freePort(t)
	vhostPort := freePort(t)

	srv, err := New(Config{
		BindAddr:      "127.0.0.1",
		BindPort:      bindPort,
		VhostHTTPPort: vhostPort,
		SubDomainHost: "test.local",
		Token:         "test-token",
	})
	require.NoError(t, err)
	require.NotNil(t, srv)
	// Clean up the listeners created by NewService.
	_ = srv.Close()
}

func TestRunAndConnect(t *testing.T) {
	bindPort := freePort(t)
	vhostPort := freePort(t)

	srv, err := New(Config{
		BindAddr:      "127.0.0.1",
		BindPort:      bindPort,
		VhostHTTPPort: vhostPort,
		SubDomainHost: "test.local",
		Token:         "test-token",
	})
	require.NoError(t, err)

	srv.Run()

	// Wait for the control port to accept connections.
	require.Eventually(t, func() bool {
		conn, dialErr := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(bindPort), 200*time.Millisecond)
		if dialErr != nil {
			return false
		}
		conn.Close()
		return true
	}, 5*time.Second, 100*time.Millisecond, "frp server did not start listening")

	require.NoError(t, srv.Close())
}
