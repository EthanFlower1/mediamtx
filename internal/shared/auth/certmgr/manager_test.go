package certmgr_test

import (
	"bytes"
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bluenviron/mediamtx/internal/shared/auth/certmgr"
	"github.com/bluenviron/mediamtx/internal/shared/auth/certmgr/fake"
)

// ---- helpers ---------------------------------------------------------------

func newCA(t *testing.T) *fake.CAClient {
	t.Helper()
	ca, err := fake.NewCAClient()
	if err != nil {
		t.Fatalf("fake.NewCAClient: %v", err)
	}
	return ca
}

func newDeviceKey(t *testing.T) *ecdsa.PrivateKey {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate device key: %v", err)
	}
	return key
}

// newManagerWithConfig is the lower-level constructor for tests that need to
// tweak Config fields individually.
func newManagerWithConfig(t *testing.T, cfg certmgr.Config) *certmgr.Manager {
	t.Helper()
	mgr, err := certmgr.New(cfg)
	if err != nil {
		t.Fatalf("certmgr.New: %v", err)
	}
	return mgr
}

// seedKeyStore issues a cert via the CA and saves it to ks, returning the
// cert's NotAfter timestamp.
func seedKeyStore(t *testing.T, ca *fake.CAClient, ks *fake.MemKeyStore, sans []string) time.Time {
	t.Helper()
	cert, err := ca.ReEnroll(context.Background(), nil, sans)
	if err != nil {
		t.Fatalf("seed ReEnroll: %v", err)
	}
	if err := ks.Save(context.Background(), cert); err != nil {
		t.Fatalf("seed Save: %v", err)
	}
	return cert.Leaf.NotAfter
}

// waitFor polls cond every 10ms until it returns true or 2 seconds elapse.
func waitFor(cond func() bool) bool {
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return true
		}
		time.Sleep(10 * time.Millisecond)
	}
	return false
}

// ---- unit tests ------------------------------------------------------------

// TestRenewalBeforeExpiry verifies that the Manager renews when remaining
// lifetime drops below RenewThreshold.
func TestRenewalBeforeExpiry(t *testing.T) {
	ca := newCA(t)
	ks := &fake.MemKeyStore{}
	notAfter := seedKeyStore(t, ca, ks, []string{"recorder.kaivue.local"})

	// Clock is certExpiry - 6h → 6h remaining, below 8h threshold.
	fixedClock := func() time.Time { return notAfter.Add(-6 * time.Hour) }

	mgr := newManagerWithConfig(t, certmgr.Config{
		CA:             ca,
		KeyStore:       ks,
		DeviceKey:      newDeviceKey(t),
		SANs:           []string{"recorder.kaivue.local"},
		RenewThreshold: 8 * time.Hour,
		TickInterval:   time.Hour,
		Clock:          fixedClock,
		Logger:         slog.Default(),
		Metrics:        certmgr.NewMetrics(),
	})
	mgr.Start(context.Background())
	defer mgr.Shutdown(context.Background())

	before := ca.RenewCalls.Load()
	mgr.ForceRenew()

	if !waitFor(func() bool { return ca.RenewCalls.Load() > before }) {
		t.Fatal("expected Renew to be called; it was not within 2s")
	}

	if mgr.ActiveCert() == nil {
		t.Fatal("expected non-nil active cert after renewal")
	}
}

// TestNoRenewalWhenFresh verifies that the Manager skips renewal when the cert
// has more than RenewThreshold remaining.
func TestNoRenewalWhenFresh(t *testing.T) {
	ca := newCA(t)
	ks := &fake.MemKeyStore{}
	notAfter := seedKeyStore(t, ca, ks, []string{"recorder.kaivue.local"})

	// Clock is notAfter - 20h → 20h remaining, well above 8h threshold.
	fixedClock := func() time.Time { return notAfter.Add(-20 * time.Hour) }

	mgr := newManagerWithConfig(t, certmgr.Config{
		CA:             ca,
		KeyStore:       ks,
		DeviceKey:      newDeviceKey(t),
		SANs:           []string{"recorder.kaivue.local"},
		RenewThreshold: 8 * time.Hour,
		TickInterval:   time.Hour,
		Clock:          fixedClock,
		Logger:         slog.Default(),
		Metrics:        certmgr.NewMetrics(),
	})
	mgr.Start(context.Background())
	defer mgr.Shutdown(context.Background())

	calls := ca.RenewCalls.Load()
	mgr.ForceRenew()
	time.Sleep(200 * time.Millisecond)

	if ca.RenewCalls.Load() != calls {
		t.Fatal("Renew was called when cert was fresh (above threshold)")
	}
}

// TestReEnrollmentWhenExpired verifies that the Manager falls back to ReEnroll
// when the current cert is already past NotAfter.
func TestReEnrollmentWhenExpired(t *testing.T) {
	ca := newCA(t)
	ks := &fake.MemKeyStore{}
	notAfter := seedKeyStore(t, ca, ks, []string{"recorder.kaivue.local"})

	// Clock is 1 minute past expiry.
	fixedClock := func() time.Time { return notAfter.Add(time.Minute) }

	mgr := newManagerWithConfig(t, certmgr.Config{
		CA:             ca,
		KeyStore:       ks,
		DeviceKey:      newDeviceKey(t),
		SANs:           []string{"recorder.kaivue.local"},
		RenewThreshold: 8 * time.Hour,
		TickInterval:   time.Hour,
		Clock:          fixedClock,
		Logger:         slog.Default(),
		Metrics:        certmgr.NewMetrics(),
	})
	mgr.Start(context.Background())
	defer mgr.Shutdown(context.Background())

	before := ca.ReEnrollCalls.Load()
	mgr.ForceRenew()

	if !waitFor(func() bool { return ca.ReEnrollCalls.Load() > before }) {
		t.Fatal("expected ReEnroll to be called for expired cert; it was not")
	}
	if mgr.ActiveCert() == nil {
		t.Fatal("expected non-nil active cert after re-enrollment")
	}
}

// TestReEnrollFallbackOnExpiredRenewalError verifies that when Renew returns
// an "expired" error, the Manager falls back to ReEnroll.
func TestReEnrollFallbackOnExpiredRenewalError(t *testing.T) {
	ca := newCA(t)
	ks := &fake.MemKeyStore{}
	notAfter := seedKeyStore(t, ca, ks, []string{"recorder.kaivue.local"})

	// Renew will fail with an expired-cert error.
	ca.RenewErr = errors.New("tls: bad certificate: x509: certificate has expired or is not yet valid")
	fixedClock := func() time.Time { return notAfter.Add(-6 * time.Hour) }

	mgr := newManagerWithConfig(t, certmgr.Config{
		CA:             ca,
		KeyStore:       ks,
		DeviceKey:      newDeviceKey(t),
		SANs:           []string{"recorder.kaivue.local"},
		RenewThreshold: 8 * time.Hour,
		TickInterval:   time.Hour,
		Clock:          fixedClock,
		Logger:         slog.Default(),
		Metrics:        certmgr.NewMetrics(),
	})
	mgr.Start(context.Background())
	defer mgr.Shutdown(context.Background())

	before := ca.ReEnrollCalls.Load()
	mgr.ForceRenew()

	if !waitFor(func() bool { return ca.ReEnrollCalls.Load() > before }) {
		t.Fatal("expected ReEnroll fallback after expiry Renew error; not called")
	}
}

// TestRenewalFailureDoesNotCrash verifies that a non-expiry renewal error is
// alarmed but does not kill the process or nil the active cert.
func TestRenewalFailureDoesNotCrash(t *testing.T) {
	ca := newCA(t)
	ks := &fake.MemKeyStore{}
	notAfter := seedKeyStore(t, ca, ks, []string{"recorder.kaivue.local"})

	// Non-expiry error.
	ca.RenewErr = errors.New("step-ca: internal server error: 500")
	fixedClock := func() time.Time { return notAfter.Add(-6 * time.Hour) }

	mgr := newManagerWithConfig(t, certmgr.Config{
		CA:             ca,
		KeyStore:       ks,
		DeviceKey:      newDeviceKey(t),
		SANs:           []string{"recorder.kaivue.local"},
		RenewThreshold: 8 * time.Hour,
		TickInterval:   time.Hour,
		Clock:          fixedClock,
		Logger:         slog.Default(),
		Metrics:        certmgr.NewMetrics(),
	})
	mgr.Start(context.Background())
	defer mgr.Shutdown(context.Background())

	before := mgr.ActiveCert()
	mgr.ForceRenew()
	time.Sleep(300 * time.Millisecond)

	after := mgr.ActiveCert()
	if after == nil {
		t.Fatal("active cert became nil after failed renewal")
	}
	if before != after {
		t.Fatal("cert was swapped despite non-expiry renewal failure")
	}
}

// TestVerificationFailureKeepsOldCert verifies that a renewed cert that fails
// x509 chain verification is rejected (fail-closed).
func TestVerificationFailureKeepsOldCert(t *testing.T) {
	ca1 := newCA(t)
	ca2 := newCA(t)
	ks := &fake.MemKeyStore{}
	notAfter := seedKeyStore(t, ca1, ks, []string{"recorder.kaivue.local"})

	// Manager uses ca2's root pool → ca1-issued certs won't verify.
	conflicting := &rootOverrideCA{inner: ca1, pool: ca2.RootPool()}
	fixedClock := func() time.Time { return notAfter.Add(-6 * time.Hour) }

	mgr := newManagerWithConfig(t, certmgr.Config{
		CA:             conflicting,
		KeyStore:       ks,
		DeviceKey:      newDeviceKey(t),
		SANs:           []string{"recorder.kaivue.local"},
		RenewThreshold: 8 * time.Hour,
		TickInterval:   time.Hour,
		Clock:          fixedClock,
		Logger:         slog.Default(),
		Metrics:        certmgr.NewMetrics(),
	})
	mgr.Start(context.Background())
	defer mgr.Shutdown(context.Background())

	before := mgr.ActiveCert()
	mgr.ForceRenew()
	time.Sleep(300 * time.Millisecond)

	if mgr.ActiveCert() != before {
		t.Fatal("cert was swapped despite verification failure (fail-closed violated)")
	}
}

// rootOverrideCA wraps a fake.CAClient but substitutes a different RootPool,
// used to simulate cert chain verification failure.
type rootOverrideCA struct {
	inner *fake.CAClient
	pool  *x509.CertPool
}

func (r *rootOverrideCA) Renew(ctx context.Context, current *tls.Certificate) (*tls.Certificate, error) {
	return r.inner.Renew(ctx, current)
}
func (r *rootOverrideCA) ReEnroll(ctx context.Context, dk crypto.PrivateKey, sans []string) (*tls.Certificate, error) {
	return r.inner.ReEnroll(ctx, dk, sans)
}
func (r *rootOverrideCA) RootPool() *x509.CertPool { return r.pool }

// TestMetricsOnSuccessfulRenewal verifies that certmgr_renewals_total{result="ok"}
// increments after a successful renewal.
func TestMetricsOnSuccessfulRenewal(t *testing.T) {
	ca := newCA(t)
	ks := &fake.MemKeyStore{}
	notAfter := seedKeyStore(t, ca, ks, []string{"recorder.kaivue.local"})
	fixedClock := func() time.Time { return notAfter.Add(-6 * time.Hour) }

	metrics := certmgr.NewMetrics()
	mgr := newManagerWithConfig(t, certmgr.Config{
		CA:             ca,
		KeyStore:       ks,
		DeviceKey:      newDeviceKey(t),
		SANs:           []string{"recorder.kaivue.local"},
		RenewThreshold: 8 * time.Hour,
		TickInterval:   time.Hour,
		Clock:          fixedClock,
		Logger:         slog.Default(),
		Metrics:        metrics,
	})
	mgr.Start(context.Background())
	defer mgr.Shutdown(context.Background())

	before := ca.RenewCalls.Load()
	mgr.ForceRenew()
	if !waitFor(func() bool { return ca.RenewCalls.Load() > before }) {
		t.Fatal("renewal did not happen within 2s")
	}
	time.Sleep(50 * time.Millisecond) // let installCert record the metric

	var buf bytes.Buffer
	metrics.WritePrometheus(&buf)
	output := buf.String()
	if !strings.Contains(output, `certmgr_renewals_total{result="ok"} 1`) {
		t.Fatalf("expected renewals_total ok=1 in metrics output; got:\n%s", output)
	}
	if !strings.Contains(output, "certmgr_cert_expires_at") {
		t.Fatalf("expected certmgr_cert_expires_at in metrics output; got:\n%s", output)
	}
}

// ---- integration test: hot reload ------------------------------------------

// TestGetCertificateHotReload spins up a real tls.Listener, serves one
// request, triggers a hot cert swap via ForceRenew, then serves another
// request — verifying the listener never drops.
func TestGetCertificateHotReload(t *testing.T) {
	ca := newCA(t)
	ks := &fake.MemKeyStore{}
	notAfter := seedKeyStore(t, ca, ks, []string{"127.0.0.1", "localhost"})
	fixedClock := func() time.Time { return notAfter.Add(-6 * time.Hour) }

	mgr := newManagerWithConfig(t, certmgr.Config{
		CA:             ca,
		KeyStore:       ks,
		DeviceKey:      newDeviceKey(t),
		SANs:           []string{"127.0.0.1", "localhost"},
		RenewThreshold: 8 * time.Hour,
		TickInterval:   time.Hour,
		Clock:          fixedClock,
		Logger:         slog.Default(),
		Metrics:        certmgr.NewMetrics(),
	})
	mgr.Start(context.Background())
	defer mgr.Shutdown(context.Background())

	serverTLS := &tls.Config{
		GetCertificate: mgr.GetCertificate,
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", serverTLS)
	if err != nil {
		t.Fatalf("tls.Listen: %v", err)
	}
	defer ln.Close()

	addr := ln.Addr().String()

	var serverErr atomic.Value
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func(c net.Conn) {
				defer c.Close()
				buf := make([]byte, 1)
				if _, err := c.Read(buf); err != nil {
					serverErr.Store(fmt.Sprintf("server read: %v", err))
					return
				}
				if _, err := c.Write(buf); err != nil {
					serverErr.Store(fmt.Sprintf("server write: %v", err))
				}
			}(conn)
		}
	}()

	clientTLS := &tls.Config{
		RootCAs:    ca.RootPool(),
		ServerName: "127.0.0.1",
	}

	echoOnce := func(t *testing.T) {
		t.Helper()
		conn, err := tls.Dial("tcp", addr, clientTLS)
		if err != nil {
			t.Fatalf("tls.Dial: %v", err)
		}
		defer conn.Close()
		if _, err := conn.Write([]byte{0x42}); err != nil {
			t.Fatalf("write: %v", err)
		}
		buf := make([]byte, 1)
		if _, err := conn.Read(buf); err != nil {
			t.Fatalf("read: %v", err)
		}
		if buf[0] != 0x42 {
			t.Fatalf("echo mismatch: want 0x42 got 0x%02x", buf[0])
		}
	}

	// Pre-renewal: listener must serve.
	echoOnce(t)

	// Trigger hot cert swap.
	prevRenew := ca.RenewCalls.Load()
	mgr.ForceRenew()
	if !waitFor(func() bool { return ca.RenewCalls.Load() > prevRenew }) {
		t.Fatal("renewal did not complete within 2s")
	}

	// Post-renewal: listener must still serve without restart.
	echoOnce(t)

	if v := serverErr.Load(); v != nil {
		t.Fatalf("server error during test: %v", v)
	}
}
