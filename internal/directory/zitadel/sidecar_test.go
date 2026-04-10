package zitadel

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validConfig() Config {
	return Config{
		BinaryPath:     "/opt/kaivue/bin/zitadel",
		DataDir:        "/tmp/zitadel-test",
		MasterKey:      "test-master-key-at-least-32-bytes-long!",
		ExternalDomain: "directory.local",
	}
}

func TestNew_Success(t *testing.T) {
	z, err := New(validConfig())
	require.NoError(t, err)
	assert.Equal(t, "zitadel", z.Name())
}

func TestNew_MissingBinaryPath(t *testing.T) {
	cfg := validConfig()
	cfg.BinaryPath = ""
	_, err := New(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "BinaryPath")
}

func TestNew_MissingDataDir(t *testing.T) {
	cfg := validConfig()
	cfg.DataDir = ""
	_, err := New(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "DataDir")
}

func TestNew_MissingMasterKey(t *testing.T) {
	cfg := validConfig()
	cfg.MasterKey = ""
	_, err := New(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "MasterKey")
}

func TestNew_MissingExternalDomain(t *testing.T) {
	cfg := validConfig()
	cfg.ExternalDomain = ""
	_, err := New(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ExternalDomain")
}

func TestCommand_DefaultPorts(t *testing.T) {
	z, err := New(validConfig())
	require.NoError(t, err)

	cmd := z.Command(context.Background())
	args := strings.Join(cmd.Args, " ")

	assert.Contains(t, args, "start-from-init")
	assert.Contains(t, args, "--masterkeyFromEnv")
	assert.Contains(t, args, "--port 8080")
	assert.Contains(t, args, "--externalDomain directory.local")
	assert.Contains(t, args, "--externalPort 8081")
	assert.Contains(t, args, "--tlsMode disabled")
	assert.NotContains(t, args, "--externalSecure")
}

func TestCommand_CustomPorts(t *testing.T) {
	cfg := validConfig()
	cfg.GRPCPort = 9090
	cfg.HTTPPort = 9091
	cfg.ExternalPort = 443
	cfg.ExternalSecure = true

	z, err := New(cfg)
	require.NoError(t, err)

	cmd := z.Command(context.Background())
	args := strings.Join(cmd.Args, " ")

	assert.Contains(t, args, "--port 9090")
	assert.Contains(t, args, "--externalPort 443")
	assert.Contains(t, args, "--externalSecure")
}

func TestCommand_TLSEnabled(t *testing.T) {
	cfg := validConfig()
	cfg.TLSCertPEM = "-----BEGIN CERTIFICATE-----\ntest\n-----END CERTIFICATE-----"
	cfg.TLSKeyPEM = "-----BEGIN PRIVATE KEY-----\ntest\n-----END PRIVATE KEY-----"

	z, err := New(cfg)
	require.NoError(t, err)

	cmd := z.Command(context.Background())
	args := strings.Join(cmd.Args, " ")
	assert.Contains(t, args, "--tlsMode enabled")
}

func TestEnv(t *testing.T) {
	z, err := New(validConfig())
	require.NoError(t, err)

	env := z.Env()
	found := map[string]bool{}
	for _, e := range env {
		if strings.HasPrefix(e, "ZITADEL_MASTERKEY=") {
			found["masterkey"] = true
		}
		if strings.HasPrefix(e, "ZITADEL_PORT=") {
			found["port"] = true
		}
		if strings.HasPrefix(e, "ZITADEL_DATABASE_SQLITE_PATH=") {
			found["dbpath"] = true
			assert.Contains(t, e, "/tmp/zitadel-test/zitadel.db")
		}
	}
	assert.True(t, found["masterkey"])
	assert.True(t, found["port"])
	assert.True(t, found["dbpath"])
}

func TestEnv_TLSPaths(t *testing.T) {
	cfg := validConfig()
	cfg.TLSCertPEM = "cert"
	cfg.TLSKeyPEM = "key"

	z, err := New(cfg)
	require.NoError(t, err)

	env := z.Env()
	foundCert, foundKey := false, false
	for _, e := range env {
		if strings.HasPrefix(e, "ZITADEL_TLS_CERTPATH=") {
			foundCert = true
		}
		if strings.HasPrefix(e, "ZITADEL_TLS_KEYPATH=") {
			foundKey = true
		}
	}
	assert.True(t, foundCert)
	assert.True(t, foundKey)
}

func TestWorkDir(t *testing.T) {
	z, err := New(validConfig())
	require.NoError(t, err)
	assert.Equal(t, "/tmp/zitadel-test", z.WorkDir())
}

func TestHealthCheck_Healthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Extract port from test server.
	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1:")
	cfg := validConfig()
	// We need to override the port — use a trick: set HTTPPort to match test server.
	fmt.Sscanf(port, "%d", &cfg.HTTPPort)

	z, err := New(cfg)
	require.NoError(t, err)

	err = z.HealthCheck(context.Background())
	assert.NoError(t, err)
}

func TestHealthCheck_Unhealthy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer srv.Close()

	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1:")
	cfg := validConfig()
	fmt.Sscanf(port, "%d", &cfg.HTTPPort)

	z, err := New(cfg)
	require.NoError(t, err)

	err = z.HealthCheck(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "status 503")
}

func TestHealthCheck_Unreachable(t *testing.T) {
	cfg := validConfig()
	cfg.HTTPPort = 19999 // nothing listening

	z, err := New(cfg)
	require.NoError(t, err)

	err = z.HealthCheck(context.Background())
	assert.Error(t, err)
}

func TestOnReady_Callback(t *testing.T) {
	z, err := New(validConfig())
	require.NoError(t, err)

	var called atomic.Bool
	z.SetOnReady(func() { called.Store(true) })

	z.OnReady()
	assert.True(t, called.Load())
}

func TestOnReady_NoCallback(t *testing.T) {
	z, err := New(validConfig())
	require.NoError(t, err)
	// Should not panic with nil callback.
	z.OnReady()
}

func TestEndpoints(t *testing.T) {
	z, err := New(validConfig())
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:8080", z.GRPCEndpoint())
	assert.Equal(t, "http://127.0.0.1:8081", z.HTTPEndpoint())
	assert.Equal(t, "http://127.0.0.1:8081/debug/healthz", z.HealthEndpoint())
}

func TestEndpoints_CustomPorts(t *testing.T) {
	cfg := validConfig()
	cfg.GRPCPort = 9090
	cfg.HTTPPort = 9091

	z, err := New(cfg)
	require.NoError(t, err)

	assert.Equal(t, "127.0.0.1:9090", z.GRPCEndpoint())
	assert.Equal(t, "http://127.0.0.1:9091", z.HTTPEndpoint())
}
