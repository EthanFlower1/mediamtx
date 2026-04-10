package customdomain

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func fixedClock(t time.Time) Clock {
	return func() time.Time { return t }
}

var testTime = time.Date(2026, 4, 10, 12, 0, 0, 0, time.UTC)

func newTestManager(t *testing.T, dns *MockDNSResolver, cert *MockCertProvider) *Manager {
	t.Helper()
	seq := 0
	mgr, err := NewManager(Config{
		Store: NewMemoryStore(),
		DNS:   dns,
		Cert:  cert,
		Clock: fixedClock(testTime),
		IDGen: func() string { seq++; return fmt.Sprintf("dom-%d", seq) },
	})
	require.NoError(t, err)
	return mgr
}

func TestNewManager_RequiredDeps(t *testing.T) {
	_, err := NewManager(Config{})
	require.ErrorContains(t, err, "store is required")

	_, err = NewManager(Config{Store: NewMemoryStore()})
	require.ErrorContains(t, err, "dns resolver is required")

	_, err = NewManager(Config{Store: NewMemoryStore(), DNS: &MockDNSResolver{}})
	require.ErrorContains(t, err, "cert provider is required")
}

func TestRegister(t *testing.T) {
	mgr := newTestManager(t, &MockDNSResolver{}, &MockCertProvider{})

	d, err := mgr.Register(context.Background(), RegisterRequest{
		TenantID: "tenant-1",
		Domain:   "cameras.acme.com",
	})
	require.NoError(t, err)
	require.Equal(t, "tenant-1", d.TenantID)
	require.Equal(t, "cameras.acme.com", d.Domain)
	require.Equal(t, StatusPending, d.Status)
	require.Equal(t, VerifyDomain, d.CNAMETarget)
}

func TestRegister_Validation(t *testing.T) {
	mgr := newTestManager(t, &MockDNSResolver{}, &MockCertProvider{})

	_, err := mgr.Register(context.Background(), RegisterRequest{})
	require.ErrorContains(t, err, "tenant_id is required")

	_, err = mgr.Register(context.Background(), RegisterRequest{TenantID: "t1"})
	require.ErrorContains(t, err, "domain is required")
}

func TestRegister_Duplicate(t *testing.T) {
	mgr := newTestManager(t, &MockDNSResolver{}, &MockCertProvider{})

	_, err := mgr.Register(context.Background(), RegisterRequest{TenantID: "t1", Domain: "a.com"})
	require.NoError(t, err)

	_, err = mgr.Register(context.Background(), RegisterRequest{TenantID: "t1", Domain: "a.com"})
	require.Error(t, err)
	require.Contains(t, err.Error(), "already exists")
}

func TestVerify_Success(t *testing.T) {
	dns := &MockDNSResolver{
		Results: map[string]string{
			"_acme-challenge.cameras.acme.com": "verify.kaivue.io.",
		},
	}
	mgr := newTestManager(t, dns, &MockCertProvider{})

	_, err := mgr.Register(context.Background(), RegisterRequest{TenantID: "t1", Domain: "cameras.acme.com"})
	require.NoError(t, err)

	d, err := mgr.Verify(context.Background(), "t1", "cameras.acme.com")
	require.NoError(t, err)
	require.Equal(t, StatusCNAMEVerified, d.Status)
	require.NotNil(t, d.VerifiedAt)
}

func TestVerify_WrongCNAME(t *testing.T) {
	dns := &MockDNSResolver{
		Results: map[string]string{
			"_acme-challenge.cameras.acme.com": "wrong.target.com.",
		},
	}
	mgr := newTestManager(t, dns, &MockCertProvider{})

	_, _ = mgr.Register(context.Background(), RegisterRequest{TenantID: "t1", Domain: "cameras.acme.com"})

	d, err := mgr.Verify(context.Background(), "t1", "cameras.acme.com")
	require.Error(t, err)
	require.Contains(t, err.Error(), "CNAME mismatch")
	require.Equal(t, StatusFailed, d.Status)
}

func TestVerify_DNSError(t *testing.T) {
	dns := &MockDNSResolver{Err: errors.New("NXDOMAIN")}
	mgr := newTestManager(t, dns, &MockCertProvider{})

	_, _ = mgr.Register(context.Background(), RegisterRequest{TenantID: "t1", Domain: "cameras.acme.com"})

	d, err := mgr.Verify(context.Background(), "t1", "cameras.acme.com")
	require.Error(t, err)
	require.Equal(t, StatusFailed, d.Status)
}

func TestVerify_NormalizesTrailingDot(t *testing.T) {
	dns := &MockDNSResolver{
		Results: map[string]string{
			"_acme-challenge.cameras.acme.com": "verify.kaivue.io", // no trailing dot
		},
	}
	mgr := newTestManager(t, dns, &MockCertProvider{})

	_, _ = mgr.Register(context.Background(), RegisterRequest{TenantID: "t1", Domain: "cameras.acme.com"})

	d, err := mgr.Verify(context.Background(), "t1", "cameras.acme.com")
	require.NoError(t, err)
	require.Equal(t, StatusCNAMEVerified, d.Status)
}

func TestFullLifecycle(t *testing.T) {
	dns := &MockDNSResolver{
		Results: map[string]string{
			"_acme-challenge.cameras.acme.com": "verify.kaivue.io.",
		},
	}
	cert := &MockCertProvider{}
	mgr := newTestManager(t, dns, cert)

	// 1. Register
	d, err := mgr.Register(context.Background(), RegisterRequest{TenantID: "t1", Domain: "cameras.acme.com"})
	require.NoError(t, err)
	require.Equal(t, StatusPending, d.Status)

	// 2. Verify CNAME
	d, err = mgr.Verify(context.Background(), "t1", "cameras.acme.com")
	require.NoError(t, err)
	require.Equal(t, StatusCNAMEVerified, d.Status)

	// 3. Provision cert
	d, err = mgr.ProvisionCert(context.Background(), "t1", "cameras.acme.com")
	require.NoError(t, err)
	require.Equal(t, StatusActive, d.Status)
	require.NotEmpty(t, d.CertificateARN)
	require.NotNil(t, d.ActivatedAt)
	require.Len(t, cert.Provisioned, 1)

	// 4. Revoke
	err = mgr.Revoke(context.Background(), "t1", "cameras.acme.com")
	require.NoError(t, err)
	require.Len(t, cert.Revoked, 1)

	// Verify status
	domains, err := mgr.ListDomains(context.Background(), "t1")
	require.NoError(t, err)
	require.Len(t, domains, 1)
	require.Equal(t, StatusRevoked, domains[0].Status)
}

func TestProvisionCert_RequiresVerified(t *testing.T) {
	mgr := newTestManager(t, &MockDNSResolver{}, &MockCertProvider{})

	_, _ = mgr.Register(context.Background(), RegisterRequest{TenantID: "t1", Domain: "a.com"})

	_, err := mgr.ProvisionCert(context.Background(), "t1", "a.com")
	require.ErrorContains(t, err, "must be cname_verified")
}

func TestProvisionCert_Failure(t *testing.T) {
	dns := &MockDNSResolver{
		Results: map[string]string{"_acme-challenge.a.com": "verify.kaivue.io."},
	}
	cert := &MockCertProvider{ProvisionErr: errors.New("rate limited")}
	mgr := newTestManager(t, dns, cert)

	_, _ = mgr.Register(context.Background(), RegisterRequest{TenantID: "t1", Domain: "a.com"})
	_, _ = mgr.Verify(context.Background(), "t1", "a.com")

	d, err := mgr.ProvisionCert(context.Background(), "t1", "a.com")
	require.Error(t, err)
	require.Equal(t, StatusFailed, d.Status)
}

func TestDelete_ActiveRevokesFirst(t *testing.T) {
	dns := &MockDNSResolver{
		Results: map[string]string{"_acme-challenge.a.com": "verify.kaivue.io."},
	}
	cert := &MockCertProvider{}
	mgr := newTestManager(t, dns, cert)

	_, _ = mgr.Register(context.Background(), RegisterRequest{TenantID: "t1", Domain: "a.com"})
	_, _ = mgr.Verify(context.Background(), "t1", "a.com")
	_, _ = mgr.ProvisionCert(context.Background(), "t1", "a.com")

	err := mgr.Delete(context.Background(), "t1", "a.com")
	require.NoError(t, err)
	require.Len(t, cert.Revoked, 1)

	domains, _ := mgr.ListDomains(context.Background(), "t1")
	require.Len(t, domains, 0)
}

func TestTenantIsolation(t *testing.T) {
	mgr := newTestManager(t, &MockDNSResolver{}, &MockCertProvider{})

	_, _ = mgr.Register(context.Background(), RegisterRequest{TenantID: "t1", Domain: "a.com"})
	_, _ = mgr.Register(context.Background(), RegisterRequest{TenantID: "t2", Domain: "b.com"})

	t1, _ := mgr.ListDomains(context.Background(), "t1")
	require.Len(t, t1, 1)
	require.Equal(t, "a.com", t1[0].Domain)

	t2, _ := mgr.ListDomains(context.Background(), "t2")
	require.Len(t, t2, 1)
	require.Equal(t, "b.com", t2[0].Domain)
}
