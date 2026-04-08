// Package audit is the cloud control-plane audit log service (KAI-233).
//
// Every authenticated action in the cloud API must emit exactly one audit
// Entry before the handler returns. Entries are durable, tenant-scoped, and
// retained for 7 years (SOC 2 / HIPAA) by default.
//
// See README.md for the full contract, schema, and retention policy.
package audit

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"
)

// ActorAgent identifies which class of principal performed the action.
type ActorAgent string

const (
	// AgentCloud   — a normal end-user authenticated against the cloud IdP.
	AgentCloud ActorAgent = "cloud"
	// AgentOnPrem — an on-prem Recorder or Directory service acting with a
	// machine credential.
	AgentOnPrem ActorAgent = "on_prem"
	// AgentIntegrator — an integrator-staff scoped token acting across one
	// of its managed customer tenants.
	AgentIntegrator ActorAgent = "integrator"
	// AgentFederation — a federated partner tenant exercising cross-tenant
	// permissions via a federation grant.
	AgentFederation ActorAgent = "federation"
)

// Valid returns true if the agent is one of the well-known values. Unknown
// values are rejected by Record so bad callers don't silently pollute the
// log.
func (a ActorAgent) Valid() bool {
	switch a {
	case AgentCloud, AgentOnPrem, AgentIntegrator, AgentFederation:
		return true
	}
	return false
}

// Result is the outcome of the authorization check or handler.
type Result string

const (
	// ResultAllow — permission granted and action executed.
	ResultAllow Result = "allow"
	// ResultDeny  — permission denied by Casbin / tenant scope.
	ResultDeny Result = "deny"
	// ResultError — action attempted but failed with an internal or external
	// error. error_code should be populated.
	ResultError Result = "error"
)

// Valid returns true if the result is one of the well-known values.
func (r Result) Valid() bool {
	switch r {
	case ResultAllow, ResultDeny, ResultError:
		return true
	}
	return false
}

// Entry is a single audit log record. All fields except the *Nullable ones
// are required; Record will reject entries missing a mandatory field.
//
// Seam #4: TenantID is the tenant *whose data* was touched. When an
// integrator-staff token reads tenant X, TenantID is X (not the integrator's
// own tenant). ImpersonatedTenantID is the same on impersonation hops and
// nil otherwise.
type Entry struct {
	ID                   string
	TenantID             string
	ActorUserID          string
	ActorAgent           ActorAgent
	ImpersonatingUserID  *string
	ImpersonatedTenantID *string
	Action               string
	ResourceType         string
	ResourceID           string
	Result               Result
	ErrorCode            *string
	IPAddress            string
	UserAgent            string
	RequestID            string
	Timestamp            time.Time
}

// Validate returns an error describing the first required field that is
// missing or invalid. Recorders call this before persisting.
func (e Entry) Validate() error {
	if e.TenantID == "" {
		return errors.New("audit: tenant_id is required")
	}
	if e.ActorUserID == "" {
		return errors.New("audit: actor_user_id is required")
	}
	if !e.ActorAgent.Valid() {
		return fmt.Errorf("audit: actor_agent %q is not valid", e.ActorAgent)
	}
	if e.Action == "" {
		return errors.New("audit: action is required")
	}
	if e.ResourceType == "" {
		return errors.New("audit: resource_type is required")
	}
	if !e.Result.Valid() {
		return fmt.Errorf("audit: result %q is not valid", e.Result)
	}
	if e.Timestamp.IsZero() {
		return errors.New("audit: timestamp is required")
	}
	if (e.ImpersonatingUserID == nil) != (e.ImpersonatedTenantID == nil) {
		return errors.New("audit: impersonating_user_id and impersonated_tenant_id must be set together")
	}
	if e.Result == ResultError && (e.ErrorCode == nil || *e.ErrorCode == "") {
		return errors.New("audit: error_code is required when result is error")
	}
	return nil
}

// QueryFilter constrains a Query or Export. TenantID is **required**; every
// Recorder must refuse empty-tenant reads so that no API handler can
// accidentally escape tenant scoping (Seam #4).
type QueryFilter struct {
	TenantID      string
	ActorUserID   string
	ActionPattern string // glob-like; "*" matches any suffix
	ResourceType  string
	Result        Result
	Since         time.Time
	Until         time.Time
	// IncludeImpersonatedTenant, when true, also returns entries whose
	// ImpersonatedTenantID equals TenantID. This lets an integrator audit
	// their own staff's cross-tenant activity without leaking other tenants.
	IncludeImpersonatedTenant bool
	Limit                     int
	// Cursor is the ID of the last row returned by the previous page, or ""
	// for the first page. Entries are ordered by (Timestamp desc, ID desc).
	Cursor string
}

// Validate checks the filter. Only TenantID is mandatory.
func (f QueryFilter) Validate() error {
	if f.TenantID == "" {
		return errors.New("audit: QueryFilter.TenantID is required (Seam #4)")
	}
	if f.Result != "" && !f.Result.Valid() {
		return fmt.Errorf("audit: QueryFilter.Result %q is not valid", f.Result)
	}
	if !f.Since.IsZero() && !f.Until.IsZero() && f.Until.Before(f.Since) {
		return errors.New("audit: QueryFilter.Until must be after Since")
	}
	return nil
}

// Recorder is the seam other cloud packages depend on. Implementations must
// enforce tenant scoping on every Query/Export; callers never have to
// double-check.
//
// The Casbin enforcer (KAI-225) calls Record directly when permission checks
// complete; HTTP handlers use the middleware in ./middleware to auto-record
// 2xx (allow) and 403 (deny).
type Recorder interface {
	Record(ctx context.Context, entry Entry) error
	Query(ctx context.Context, filter QueryFilter) ([]Entry, error)
	Export(ctx context.Context, filter QueryFilter, format ExportFormat, w io.Writer) error
}

// ExportFormat selects the serialization used by Export.
type ExportFormat string

const (
	// ExportCSV writes RFC 4180 CSV with a header row.
	ExportCSV ExportFormat = "csv"
	// ExportJSON writes newline-delimited JSON, one Entry per line.
	ExportJSON ExportFormat = "json"
)

// DefaultRetention is the default audit-log retention window used by the
// partition janitor. Seven years satisfies SOC 2 CC4, HIPAA §164.316(b)(2),
// and PCI DSS 10.7.
const DefaultRetention = 7 * 365 * 24 * time.Hour

// ErrTenantMismatch is returned when a caller tries to read an entry whose
// tenant does not match the filter's tenant. Tests assert this is the exact
// error surface for Seam #4 violations.
var ErrTenantMismatch = errors.New("audit: tenant mismatch (Seam #4 violation)")

// ErrNotFound is returned by Query when a cursor points at an unknown entry.
var ErrNotFound = errors.New("audit: not found")
