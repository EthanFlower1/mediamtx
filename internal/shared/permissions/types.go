package permissions

import (
	"fmt"
	"strings"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// SubjectKind distinguishes the three subject families the engine understands.
type SubjectKind string

const (
	// SubjectKindUser is a direct in-tenant identity ("user:<id>@<tenant>").
	SubjectKindUser SubjectKind = "user"

	// SubjectKindIntegrator is reseller staff acting on a customer tenant
	// ("integrator:<id>@<customer_tenant>"). The tenant portion is the
	// CUSTOMER tenant, not the integrator's own tenant — this is the whole
	// point of the cross-tenant prefix.
	SubjectKindIntegrator SubjectKind = "integrator"

	// SubjectKindFederation is a federated peer directory
	// ("federation:<peer_directory_id>"). No tenant suffix — the peer
	// directory record itself carries the allowed tenant scope.
	SubjectKindFederation SubjectKind = "federation"
)

// SubjectRef identifies an actor for an Enforce call. Construct via the
// helpers below (NewUserSubject, NewIntegratorSubject, NewFederationSubject)
// or SubjectFromClaims — never by hand, to avoid formatting drift.
type SubjectRef struct {
	Kind   SubjectKind
	ID     string        // user id OR peer directory id
	Tenant auth.TenantRef // customer tenant being acted upon (empty for federation)
}

// NewUserSubject builds a direct-user subject reference.
func NewUserSubject(userID auth.UserID, tenant auth.TenantRef) SubjectRef {
	return SubjectRef{Kind: SubjectKindUser, ID: string(userID), Tenant: tenant}
}

// NewIntegratorSubject builds a cross-tenant integrator subject. The tenant
// argument is the CUSTOMER tenant the integrator is acting on.
func NewIntegratorSubject(userID auth.UserID, customerTenant auth.TenantRef) SubjectRef {
	return SubjectRef{Kind: SubjectKindIntegrator, ID: string(userID), Tenant: customerTenant}
}

// NewFederationSubject builds a federated peer subject.
func NewFederationSubject(peerDirectoryID string) SubjectRef {
	return SubjectRef{Kind: SubjectKindFederation, ID: peerDirectoryID}
}

// String renders the subject in Casbin wire format (see model.conf).
func (s SubjectRef) String() string {
	switch s.Kind {
	case SubjectKindUser:
		return fmt.Sprintf("user:%s@%s", s.ID, s.Tenant.ID)
	case SubjectKindIntegrator:
		return fmt.Sprintf("integrator:%s@%s", s.ID, s.Tenant.ID)
	case SubjectKindFederation:
		return fmt.Sprintf("federation:%s", s.ID)
	default:
		return ""
	}
}

// Validate reports whether the subject is structurally well-formed.
func (s SubjectRef) Validate() error {
	if s.ID == "" {
		return fmt.Errorf("permissions: subject id is empty")
	}
	switch s.Kind {
	case SubjectKindUser, SubjectKindIntegrator:
		if s.Tenant.ID == "" {
			return fmt.Errorf("permissions: %s subject requires tenant", s.Kind)
		}
	case SubjectKindFederation:
		// tenant not used
	default:
		return fmt.Errorf("permissions: unknown subject kind %q", s.Kind)
	}
	return nil
}

// ObjectRef is a tenant-scoped resource reference. Tenant is MANDATORY — the
// String form always carries the tenant prefix so policy wildcards cannot
// accidentally match across tenants.
type ObjectRef struct {
	Tenant       auth.TenantRef
	ResourceType string // e.g. "cameras", "users", "recordings"
	ResourceID   string // "*" means any instance of the type in this tenant
}

// NewObject builds an ObjectRef for a specific resource instance.
func NewObject(tenant auth.TenantRef, resourceType, resourceID string) ObjectRef {
	return ObjectRef{Tenant: tenant, ResourceType: resourceType, ResourceID: resourceID}
}

// NewObjectAll builds an ObjectRef that matches any instance of the given
// resource type within a tenant. Use "*" for resource type too if you mean
// "everything in this tenant".
func NewObjectAll(tenant auth.TenantRef, resourceType string) ObjectRef {
	return ObjectRef{Tenant: tenant, ResourceType: resourceType, ResourceID: "*"}
}

// String renders the object in Casbin wire format.
func (o ObjectRef) String() string {
	if o.ResourceID == "" {
		return fmt.Sprintf("%s/%s/*", o.Tenant.ID, o.ResourceType)
	}
	return fmt.Sprintf("%s/%s/%s", o.Tenant.ID, o.ResourceType, o.ResourceID)
}

// Validate reports whether the object is structurally well-formed.
func (o ObjectRef) Validate() error {
	if o.Tenant.ID == "" {
		return fmt.Errorf("permissions: object tenant is mandatory")
	}
	if o.ResourceType == "" {
		return fmt.Errorf("permissions: object resource type is empty")
	}
	if strings.Contains(o.Tenant.ID, "/") || strings.Contains(o.ResourceType, "/") {
		return fmt.Errorf("permissions: object components may not contain '/'")
	}
	return nil
}

// SubjectFromClaims derives a direct-user SubjectRef from a verified auth
// session's claims. Cross-tenant (integrator) subjects are built by the
// handler that resolved the IntegratorRelationshipRef — not automatically.
func SubjectFromClaims(c auth.Claims) SubjectRef {
	return NewUserSubject(c.UserID, c.TenantRef)
}
