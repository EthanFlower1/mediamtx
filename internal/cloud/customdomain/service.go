package customdomain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// VerifyDomain is the CNAME target that integrators point their
// _acme-challenge subdomain to. Production value: "verify.kaivue.io."
const VerifyDomain = "verify.kaivue.io."

// Clock abstracts time for testing.
type Clock func() time.Time

// IDGen generates unique IDs.
type IDGen func() string

// Config bundles the dependencies of a Manager.
type Config struct {
	Store        DomainStore
	DNS          DNSResolver
	Cert         CertProvider
	Clock        Clock
	IDGen        IDGen
}

// Manager orchestrates the custom domain lifecycle.
type Manager struct {
	store DomainStore
	dns   DNSResolver
	cert  CertProvider
	clock Clock
	idGen IDGen
}

// NewManager constructs a Manager. Store, DNS, and Cert are required.
func NewManager(cfg Config) (*Manager, error) {
	if cfg.Store == nil {
		return nil, errors.New("customdomain: store is required")
	}
	if cfg.DNS == nil {
		return nil, errors.New("customdomain: dns resolver is required")
	}
	if cfg.Cert == nil {
		return nil, errors.New("customdomain: cert provider is required")
	}
	clock := cfg.Clock
	if clock == nil {
		clock = func() time.Time { return time.Now().UTC() }
	}
	idGen := cfg.IDGen
	if idGen == nil {
		idGen = func() string { return fmt.Sprintf("dom-%d", time.Now().UnixNano()) }
	}
	return &Manager{
		store: cfg.Store,
		dns:   cfg.DNS,
		cert:  cfg.Cert,
		clock: clock,
		idGen: idGen,
	}, nil
}

// Register creates a new custom domain in Pending status. The integrator
// must create a CNAME record before calling Verify.
func (m *Manager) Register(ctx context.Context, req RegisterRequest) (*Domain, error) {
	if err := req.Validate(); err != nil {
		return nil, err
	}

	now := m.clock()
	d := &Domain{
		ID:          m.idGen(),
		TenantID:    req.TenantID,
		Domain:      req.Domain,
		CNAMETarget: VerifyDomain,
		Status:      StatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	if err := m.store.Insert(ctx, d); err != nil {
		return nil, fmt.Errorf("customdomain: register: %w", err)
	}
	return d, nil
}

// Verify checks that the CNAME record exists and points to the expected
// target. On success, transitions the domain to CNAMEVerified.
func (m *Manager) Verify(ctx context.Context, tenantID, domain string) (*Domain, error) {
	d, err := m.store.GetByTenantAndDomain(ctx, tenantID, domain)
	if err != nil {
		return nil, fmt.Errorf("customdomain: verify lookup: %w", err)
	}

	if d.Status != StatusPending && d.Status != StatusFailed {
		return nil, fmt.Errorf("customdomain: domain %s is in status %s, expected pending or failed", domain, d.Status)
	}

	// Check CNAME for _acme-challenge.<domain>
	challengeHost := "_acme-challenge." + domain
	cname, err := m.dns.LookupCNAME(ctx, challengeHost)
	if err != nil {
		d.Status = StatusFailed
		d.FailureReason = fmt.Sprintf("DNS lookup failed: %v", err)
		d.UpdatedAt = m.clock()
		_ = m.store.Update(ctx, d)
		return d, fmt.Errorf("customdomain: CNAME lookup failed for %s: %w", challengeHost, err)
	}

	// Normalize: ensure trailing dot for comparison
	if !strings.HasSuffix(cname, ".") {
		cname += "."
	}
	if cname != VerifyDomain {
		d.Status = StatusFailed
		d.FailureReason = fmt.Sprintf("CNAME points to %s, expected %s", cname, VerifyDomain)
		d.UpdatedAt = m.clock()
		_ = m.store.Update(ctx, d)
		return d, fmt.Errorf("customdomain: CNAME mismatch: got %s, want %s", cname, VerifyDomain)
	}

	now := m.clock()
	d.Status = StatusCNAMEVerified
	d.VerifiedAt = &now
	d.FailureReason = ""
	d.UpdatedAt = now
	if err := m.store.Update(ctx, d); err != nil {
		return nil, fmt.Errorf("customdomain: verify update: %w", err)
	}
	return d, nil
}

// ProvisionCert requests a TLS certificate for a verified domain.
func (m *Manager) ProvisionCert(ctx context.Context, tenantID, domain string) (*Domain, error) {
	d, err := m.store.GetByTenantAndDomain(ctx, tenantID, domain)
	if err != nil {
		return nil, fmt.Errorf("customdomain: provision lookup: %w", err)
	}

	if d.Status != StatusCNAMEVerified {
		return nil, fmt.Errorf("customdomain: domain %s must be cname_verified before cert provisioning (current: %s)", domain, d.Status)
	}

	d.Status = StatusCertProvisioning
	d.UpdatedAt = m.clock()
	_ = m.store.Update(ctx, d)

	certID, err := m.cert.Provision(ctx, domain)
	if err != nil {
		d.Status = StatusFailed
		d.FailureReason = fmt.Sprintf("cert provisioning failed: %v", err)
		d.UpdatedAt = m.clock()
		_ = m.store.Update(ctx, d)
		return d, fmt.Errorf("customdomain: cert provision: %w", err)
	}

	now := m.clock()
	d.Status = StatusActive
	d.CertificateARN = certID
	d.ActivatedAt = &now
	d.FailureReason = ""
	d.UpdatedAt = now
	if err := m.store.Update(ctx, d); err != nil {
		return nil, fmt.Errorf("customdomain: provision update: %w", err)
	}
	return d, nil
}

// Revoke revokes the certificate and marks the domain as revoked.
func (m *Manager) Revoke(ctx context.Context, tenantID, domain string) error {
	d, err := m.store.GetByTenantAndDomain(ctx, tenantID, domain)
	if err != nil {
		return fmt.Errorf("customdomain: revoke lookup: %w", err)
	}

	if d.CertificateARN != "" {
		if err := m.cert.Revoke(ctx, d.CertificateARN); err != nil {
			return fmt.Errorf("customdomain: revoke cert: %w", err)
		}
	}

	d.Status = StatusRevoked
	d.UpdatedAt = m.clock()
	if err := m.store.Update(ctx, d); err != nil {
		return fmt.Errorf("customdomain: revoke update: %w", err)
	}
	return nil
}

// ListDomains returns all custom domains for a tenant.
func (m *Manager) ListDomains(ctx context.Context, tenantID string) ([]*Domain, error) {
	if tenantID == "" {
		return nil, errors.New("customdomain: tenant_id is required")
	}
	return m.store.ListByTenant(ctx, tenantID)
}

// Delete removes a domain record. If active, revokes the cert first.
func (m *Manager) Delete(ctx context.Context, tenantID, domain string) error {
	d, err := m.store.GetByTenantAndDomain(ctx, tenantID, domain)
	if err != nil {
		return fmt.Errorf("customdomain: delete lookup: %w", err)
	}

	if d.Status == StatusActive && d.CertificateARN != "" {
		if err := m.cert.Revoke(ctx, d.CertificateARN); err != nil {
			return fmt.Errorf("customdomain: delete revoke: %w", err)
		}
	}

	return m.store.Delete(ctx, tenantID, domain)
}
