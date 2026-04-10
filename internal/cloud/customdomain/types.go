// Package customdomain manages integrator custom domain provisioning with
// CNAME verification and automated TLS certificate issuance via ACME (Let's
// Encrypt). Every domain is scoped to a tenant_id for multi-tenant isolation.
package customdomain

import (
	"context"
	"errors"
	"time"
)

// DomainStatus tracks the lifecycle of a custom domain registration.
type DomainStatus string

const (
	StatusPending          DomainStatus = "pending"
	StatusCNAMEVerified    DomainStatus = "cname_verified"
	StatusCertProvisioning DomainStatus = "cert_provisioning"
	StatusActive           DomainStatus = "active"
	StatusFailed           DomainStatus = "failed"
	StatusRevoked          DomainStatus = "revoked"
)

// Domain represents a custom domain record in the database.
type Domain struct {
	ID             string
	TenantID       string
	Domain         string
	CNAMETarget    string
	Status         DomainStatus
	CertificateARN string
	VerifiedAt     *time.Time
	ActivatedAt    *time.Time
	FailureReason  string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// RegisterRequest is the input for registering a new custom domain.
type RegisterRequest struct {
	TenantID string
	Domain   string // e.g. "cameras.acme-security.com"
}

// Validate checks required fields.
func (r RegisterRequest) Validate() error {
	if r.TenantID == "" {
		return errors.New("customdomain: tenant_id is required")
	}
	if r.Domain == "" {
		return errors.New("customdomain: domain is required")
	}
	return nil
}

// DNSResolver abstracts DNS lookups for testing.
type DNSResolver interface {
	// LookupCNAME returns the canonical name for the given host.
	LookupCNAME(ctx context.Context, host string) (string, error)
}

// CertProvider abstracts TLS certificate provisioning (ACME).
type CertProvider interface {
	// Provision requests a TLS certificate for the domain and returns an
	// identifier (e.g. ACM ARN or local cert path).
	Provision(ctx context.Context, domain string) (certID string, err error)

	// Revoke revokes a previously issued certificate.
	Revoke(ctx context.Context, certID string) error
}

// DomainStore abstracts database operations for custom domain records.
type DomainStore interface {
	// Insert creates a new domain record.
	Insert(ctx context.Context, d *Domain) error

	// GetByTenantAndDomain retrieves a domain record.
	GetByTenantAndDomain(ctx context.Context, tenantID, domain string) (*Domain, error)

	// Update persists changes to a domain record.
	Update(ctx context.Context, d *Domain) error

	// Delete removes a domain record.
	Delete(ctx context.Context, tenantID, domain string) error

	// ListByTenant returns all domains for a tenant.
	ListByTenant(ctx context.Context, tenantID string) ([]*Domain, error)
}
