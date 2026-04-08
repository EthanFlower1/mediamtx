package permissions

import (
	"fmt"
	"strings"

	"github.com/bluenviron/mediamtx/internal/shared/auth"
)

// RoleTemplate is a named bundle of (object-pattern, action-pattern) grants
// that is instantiated per tenant. The Enforcer turns a template into concrete
// PolicyRules via SeedRole.
type RoleTemplate struct {
	// Name is the template identifier (e.g. "admin", "viewer"). Role IDs in
	// Casbin are rendered as "role:<name>@<tenant>".
	Name string

	// Description is a short human-readable explanation for admin UIs.
	Description string

	// Grants is the list of (object-pattern, action-pattern) tuples. Object
	// patterns should omit the tenant prefix — SeedRole stamps the tenant in
	// automatically (e.g. "cameras/*" becomes "<tenant>/cameras/*").
	Grants []RoleGrant
}

// RoleGrant is one allow line inside a RoleTemplate.
type RoleGrant struct {
	// ObjectPattern is the tenant-relative object pattern (e.g. "cameras/*").
	// Use "*" for "everything in the tenant".
	ObjectPattern string
	// Action is an action constant or wildcard (e.g. "view.*", "*").
	Action string
}

// Canonical role template names.
const (
	RoleAdmin             = "admin"
	RoleOperator          = "operator"
	RoleViewer            = "viewer"
	RoleIntegratorAdmin   = "integrator_admin"
	RoleIntegratorSupport = "integrator_support"
)

// DefaultRoleTemplates is the seed set of roles every new tenant is
// provisioned with. These are intentionally coarse; KAI-315's permissions
// matrix will expose finer-grained custom roles on top of this base.
var DefaultRoleTemplates = map[string]RoleTemplate{
	RoleAdmin: {
		Name:        RoleAdmin,
		Description: "Full control over the tenant (every action on every resource).",
		Grants: []RoleGrant{
			{ObjectPattern: "*", Action: "*"},
		},
	},
	RoleOperator: {
		Name:        RoleOperator,
		Description: "Day-to-day operator: view, PTZ, add/edit cameras, audit read.",
		Grants: []RoleGrant{
			{ObjectPattern: "cameras/*", Action: "view.*"},
			{ObjectPattern: "cameras/*", Action: ActionPTZControl},
			{ObjectPattern: "cameras/*", Action: ActionAudioTalkback},
			{ObjectPattern: "cameras/*", Action: ActionCamerasAdd},
			{ObjectPattern: "cameras/*", Action: ActionCamerasEdit},
			{ObjectPattern: "cameras/*", Action: ActionCamerasMove},
			{ObjectPattern: "audit/*", Action: ActionAuditRead},
			{ObjectPattern: "system/*", Action: ActionSystemHealth},
		},
	},
	RoleViewer: {
		Name:        RoleViewer,
		Description: "Read-only: thumbnails, live, playback, snapshots. No control.",
		Grants: []RoleGrant{
			{ObjectPattern: "cameras/*", Action: "view.*"},
		},
	},
	RoleIntegratorAdmin: {
		Name:        RoleIntegratorAdmin,
		Description: "Integrator staff full access on a managed customer tenant.",
		Grants: []RoleGrant{
			{ObjectPattern: "*", Action: "*"},
		},
	},
	RoleIntegratorSupport: {
		Name:        RoleIntegratorSupport,
		Description: "Integrator support: viewing + PTZ only; no config changes.",
		Grants: []RoleGrant{
			{ObjectPattern: "cameras/*", Action: "view.*"},
			{ObjectPattern: "cameras/*", Action: ActionPTZControl},
			{ObjectPattern: "audit/*", Action: ActionAuditRead},
			{ObjectPattern: "system/*", Action: ActionSystemHealth},
		},
	},
}

// RoleID renders the Casbin-wire role identifier for a template within a
// tenant (e.g. "role:admin@tenant-A").
func RoleID(roleName string, tenant auth.TenantRef) string {
	return fmt.Sprintf("role:%s@%s", roleName, tenant.ID)
}

// SeedRole materializes a RoleTemplate into the enforcer for a specific
// tenant. It writes one PolicyRule per grant and returns the canonical
// role id so callers can assign it via BindSubjectToRole.
func SeedRole(e *Enforcer, tmpl RoleTemplate, tenant auth.TenantRef) (string, error) {
	if tenant.ID == "" {
		return "", fmt.Errorf("permissions: SeedRole requires tenant")
	}
	if strings.TrimSpace(tmpl.Name) == "" {
		return "", fmt.Errorf("permissions: SeedRole requires template name")
	}
	role := RoleID(tmpl.Name, tenant)
	for _, g := range tmpl.Grants {
		obj := fmt.Sprintf("%s/%s", tenant.ID, g.ObjectPattern)
		if g.ObjectPattern == "*" {
			obj = fmt.Sprintf("%s/*", tenant.ID)
		}
		if err := e.store.AddPolicy(PolicyRule{
			Sub: role,
			Obj: obj,
			Act: g.Action,
			Eft: "allow",
		}); err != nil {
			return "", err
		}
	}
	if err := e.ReloadPolicy(); err != nil {
		return "", err
	}
	return role, nil
}

// BindSubjectToRole links a SubjectRef to an already-seeded role. The role
// argument must be a canonical role id from RoleID / SeedRole.
func BindSubjectToRole(e *Enforcer, subject SubjectRef, role string) error {
	if err := subject.Validate(); err != nil {
		return err
	}
	return e.AddGrouping(GroupingRule{Subject: subject.String(), Role: role})
}

// UnbindSubjectFromRole is the inverse of BindSubjectToRole.
func UnbindSubjectFromRole(e *Enforcer, subject SubjectRef, role string) error {
	return e.RemoveGrouping(GroupingRule{Subject: subject.String(), Role: role})
}
