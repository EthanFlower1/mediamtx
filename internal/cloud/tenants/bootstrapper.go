package tenants

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// TenantBootstrapper is the narrow seam (#3) that the tenant provisioning
// service uses to talk to the identity provider. KAI-223's Zitadel adapter
// will implement this; tests use MemoryBootstrapper.
//
// It intentionally does NOT embed auth.IdentityProvider: KAI-222 froze the
// IdentityProvider interface at the set of day-to-day operations, and
// proto-locking adds of "create org" to that interface would block KAI-223.
// A separate bootstrapping seam keeps both teams moving.
type TenantBootstrapper interface {
	// CreateOrg creates a new organization in the identity provider and
	// returns its opaque id. Failure is non-idempotent: if an error is
	// returned the caller MUST NOT assume an org was created.
	CreateOrg(ctx context.Context, spec OrgSpec) (OrgID, error)

	// DeleteOrg is the compensating action used by the provisioning
	// service when a step after CreateOrg fails. It MUST be idempotent —
	// deleting an unknown org returns nil.
	DeleteOrg(ctx context.Context, id OrgID) error

	// CreateInitialAdmin creates the first admin user inside an org and
	// returns its opaque user id plus an invite token the caller is
	// responsible for mailing out (via the JobEnqueuer seam).
	CreateInitialAdmin(ctx context.Context, orgID OrgID, email string) (AdminUser, error)
}

// OrgSpec is the input to TenantBootstrapper.CreateOrg.
type OrgSpec struct {
	// Tenant is the cloud-side TenantRef the org will map to. Adapters use
	// this to stamp the Zitadel org name and its custom metadata so
	// bidirectional lookups stay cheap.
	Tenant auth.TenantRef
	// DisplayName is the human-readable name shown in Zitadel admin UI.
	DisplayName string
	// ContactEmail is the admin contact captured on org creation; it is
	// optional (empty string is allowed).
	ContactEmail string
}

// OrgID is the identity-provider's opaque id for a freshly created org.
type OrgID string

// AdminUser is the result of TenantBootstrapper.CreateInitialAdmin.
type AdminUser struct {
	UserID      string
	Email       string
	InviteToken string
}

// --- in-memory fake ------------------------------------------------------

// MemoryBootstrapper is a test implementation of TenantBootstrapper. It
// supports hooks that fail on the N-th call so rollback paths can be
// exercised deterministically.
type MemoryBootstrapper struct {
	mu sync.Mutex

	// CreateOrgErr, when non-nil, is returned from the next CreateOrg
	// call and then cleared. Tests set this to inject failures.
	CreateOrgErr error
	// DeleteOrgErr, when non-nil, is returned from the next DeleteOrg
	// call and then cleared.
	DeleteOrgErr error
	// CreateInitialAdminErr, when non-nil, is returned from the next
	// CreateInitialAdmin call and then cleared.
	CreateInitialAdminErr error

	orgs       map[OrgID]OrgSpec
	admins     map[OrgID][]AdminUser
	nextOrgSeq int

	CreateOrgCalls    []OrgSpec
	DeleteOrgCalls    []OrgID
	InitialAdminCalls []struct {
		OrgID OrgID
		Email string
	}
}

// NewMemoryBootstrapper returns an empty in-memory fake.
func NewMemoryBootstrapper() *MemoryBootstrapper {
	return &MemoryBootstrapper{
		orgs:   make(map[OrgID]OrgSpec),
		admins: make(map[OrgID][]AdminUser),
	}
}

// CreateOrg implements TenantBootstrapper.
func (m *MemoryBootstrapper) CreateOrg(_ context.Context, spec OrgSpec) (OrgID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.CreateOrgCalls = append(m.CreateOrgCalls, spec)
	if err := m.CreateOrgErr; err != nil {
		m.CreateOrgErr = nil
		return "", err
	}
	m.nextOrgSeq++
	id := OrgID(fmt.Sprintf("org-%d-%s", m.nextOrgSeq, spec.Tenant.ID))
	m.orgs[id] = spec
	return id, nil
}

// DeleteOrg implements TenantBootstrapper.
func (m *MemoryBootstrapper) DeleteOrg(_ context.Context, id OrgID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.DeleteOrgCalls = append(m.DeleteOrgCalls, id)
	if err := m.DeleteOrgErr; err != nil {
		m.DeleteOrgErr = nil
		return err
	}
	delete(m.orgs, id)
	delete(m.admins, id)
	return nil
}

// CreateInitialAdmin implements TenantBootstrapper.
func (m *MemoryBootstrapper) CreateInitialAdmin(_ context.Context, orgID OrgID, email string) (AdminUser, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.InitialAdminCalls = append(m.InitialAdminCalls, struct {
		OrgID OrgID
		Email string
	}{orgID, email})
	if err := m.CreateInitialAdminErr; err != nil {
		m.CreateInitialAdminErr = nil
		return AdminUser{}, err
	}
	if _, ok := m.orgs[orgID]; !ok {
		return AdminUser{}, errors.New("memory bootstrapper: unknown org")
	}
	u := AdminUser{
		UserID:      fmt.Sprintf("zu-%s-%s", orgID, email),
		Email:       email,
		InviteToken: fmt.Sprintf("invite-%s-%s", orgID, email),
	}
	m.admins[orgID] = append(m.admins[orgID], u)
	return u, nil
}

// OrgCount reports how many orgs currently exist (useful for rollback
// assertions).
func (m *MemoryBootstrapper) OrgCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.orgs)
}

// HasOrg reports whether an org is still present.
func (m *MemoryBootstrapper) HasOrg(id OrgID) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	_, ok := m.orgs[id]
	return ok
}
