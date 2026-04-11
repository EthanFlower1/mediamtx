package federation_test

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"net"
	"net/http"
	"testing"
	"time"

	connect "connectrpc.com/connect"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/bluenviron/mediamtx/internal/directory/federation"
	kaivuev1 "github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1"
	"github.com/bluenviron/mediamtx/internal/shared/proto/gen/go/kaivue/v1/kaivuev1connect"
)

// selfSignedCA generates a self-signed CA and leaf certificate for testing.
func selfSignedCA(t *testing.T) (caCert *x509.Certificate, caPool *x509.CertPool, serverTLS *tls.Certificate, clientTLS *tls.Certificate) {
	t.Helper()

	// Generate CA key.
	caPub, caPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)

	caTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "Test Federation CA"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
	}
	caDER, err := x509.CreateCertificate(rand.Reader, caTemplate, caTemplate, caPub, caPriv)
	require.NoError(t, err)
	caCert, err = x509.ParseCertificate(caDER)
	require.NoError(t, err)

	caPool = x509.NewCertPool()
	caPool.AddCert(caCert)

	// Server leaf.
	serverPub, serverPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	serverTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(2),
		Subject:      pkix.Name{CommonName: "federation-server"},
		DNSNames:     []string{"localhost", "127.0.0.1"},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
	}
	serverDER, err := x509.CreateCertificate(rand.Reader, serverTemplate, caCert, serverPub, caPriv)
	require.NoError(t, err)
	serverTLS = &tls.Certificate{
		Certificate: [][]byte{serverDER, caDER},
		PrivateKey:  serverPriv,
	}
	parsed, err := x509.ParseCertificate(serverDER)
	require.NoError(t, err)
	serverTLS.Leaf = parsed

	// Client leaf.
	clientPub, clientPriv, err := ed25519.GenerateKey(rand.Reader)
	require.NoError(t, err)
	clientTemplate := &x509.Certificate{
		SerialNumber: big.NewInt(3),
		Subject:      pkix.Name{CommonName: "federation-client"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth},
	}
	clientDER, err := x509.CreateCertificate(rand.Reader, clientTemplate, caCert, clientPub, caPriv)
	require.NoError(t, err)
	clientTLS = &tls.Certificate{
		Certificate: [][]byte{clientDER, caDER},
		PrivateKey:  clientPriv,
	}

	return caCert, caPool, serverTLS, clientTLS
}

func TestServer_MTLSPingRoundTrip(t *testing.T) {
	_, caPool, serverCert, clientCert := selfSignedCA(t)

	handler, err := federation.NewRPCHandler(federation.RPCConfig{
		ServerVersion: "1.0.0-mTLS-test",
		JWKSProvider:  &staticJWKSProvider{json: testJWKS, maxAge: 300},
	})
	require.NoError(t, err)

	// Pick a free port.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	ln.Close()

	srv, err := federation.NewServer(federation.ServerConfig{
		ListenAddr: addr,
		TLSCert:    serverCert,
		ClientCAs:  caPool,
		Handler:    handler,
	})
	require.NoError(t, err)

	go func() { _ = srv.ListenAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	// Give the server a moment to start.
	time.Sleep(50 * time.Millisecond)

	// Build mTLS client.
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{*clientCert},
		RootCAs:      caPool,
		MinVersion:   tls.VersionTLS13,
	}
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: tlsCfg,
		},
	}

	client := kaivuev1connect.NewFederationPeerServiceClient(
		httpClient,
		"https://"+addr,
	)

	resp, err := client.Ping(context.Background(), connect.NewRequest(&kaivuev1.PingRequest{
		Nonce: "mtls-test",
	}))
	require.NoError(t, err)
	assert.Equal(t, "mtls-test", resp.Msg.GetNonce())
	assert.Equal(t, "1.0.0-mTLS-test", resp.Msg.GetServerVersion())
}

func TestServer_RejectsNoClientCert(t *testing.T) {
	_, caPool, serverCert, _ := selfSignedCA(t)

	handler, err := federation.NewRPCHandler(federation.RPCConfig{
		ServerVersion: "1.0.0-reject-test",
		JWKSProvider:  &staticJWKSProvider{json: testJWKS, maxAge: 300},
	})
	require.NoError(t, err)

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	addr := ln.Addr().String()
	ln.Close()

	srv, err := federation.NewServer(federation.ServerConfig{
		ListenAddr: addr,
		TLSCert:    serverCert,
		ClientCAs:  caPool,
		Handler:    handler,
	})
	require.NoError(t, err)

	go func() { _ = srv.ListenAndServe() }()
	t.Cleanup(func() { _ = srv.Shutdown(context.Background()) })

	time.Sleep(50 * time.Millisecond)

	// Client without a certificate should be rejected.
	httpClient := &http.Client{
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{
				RootCAs:    caPool,
				MinVersion: tls.VersionTLS13,
			},
		},
	}

	client := kaivuev1connect.NewFederationPeerServiceClient(
		httpClient,
		"https://"+addr,
	)

	_, err = client.Ping(context.Background(), connect.NewRequest(&kaivuev1.PingRequest{}))
	require.Error(t, err)
}

func TestNewServer_MissingConfig(t *testing.T) {
	_, err := federation.NewServer(federation.ServerConfig{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ListenAddr")
}
