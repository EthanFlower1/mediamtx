package jobs

// Payload structs for the eight seeded job kinds. Each implements
// TenantScoped so the runner can verify tenant isolation (Seam #4).
//
// These are intentionally thin: Wave 3 workers log + return success.
// The real fields are fleshed out in the follow-up tickets listed on
// each type.

// TenantWelcomeEmailPayload — first admin of a freshly provisioned
// tenant gets a welcome email. Real work lands in KAI-371 (email
// sender via SendGrid).
type TenantWelcomeEmailPayload struct {
	Tenant    string
	AdminUser string
	AdminMail string
}

// TenantID implements TenantScoped.
func (p TenantWelcomeEmailPayload) TenantID() string { return p.Tenant }

// TenantBootstrapStripePayload — creates a Stripe Connect account for
// the new tenant. Real work lands in KAI-361.
type TenantBootstrapStripePayload struct {
	Tenant      string
	LegalName   string
	Country     string
	BillingMode string // "direct" | "via_integrator"
}

// TenantID implements TenantScoped.
func (p TenantBootstrapStripePayload) TenantID() string { return p.Tenant }

// TenantBootstrapZitadelPayload — creates a Zitadel org for the new
// tenant. Real work lands in KAI-223 (Zitadel adapter).
type TenantBootstrapZitadelPayload struct {
	Tenant     string
	OrgName    string
	AdminEmail string
}

// TenantID implements TenantScoped.
func (p TenantBootstrapZitadelPayload) TenantID() string { return p.Tenant }

// BulkPushConfigPayload — fans out a config update to a set of
// customers belonging to one tenant (integrator pushing to its
// customers). Real work lands in KAI-343 (fleet config push).
type BulkPushConfigPayload struct {
	Tenant      string
	CustomerIDs []string
	ConfigHash  string
}

// TenantID implements TenantScoped.
func (p BulkPushConfigPayload) TenantID() string { return p.Tenant }

// CloudArchiveUploadTriggerPayload — kicks off a segment upload from
// an on-prem Recorder to the cloud archive bucket. Real work lands in
// KAI-258.
type CloudArchiveUploadTriggerPayload struct {
	Tenant    string
	SegmentID string
	CameraID  string
}

// TenantID implements TenantScoped.
func (p CloudArchiveUploadTriggerPayload) TenantID() string { return p.Tenant }

// BillingMonthlyRollupPayload — nightly batch that totals usage for a
// tenant. Real work lands in KAI-363.
type BillingMonthlyRollupPayload struct {
	Tenant string
	Period string // YYYY-MM
}

// TenantID implements TenantScoped.
func (p BillingMonthlyRollupPayload) TenantID() string { return p.Tenant }

// AuditPartitionCreateNextPayload — pg_partman helper that creates the
// next monthly partition of audit_log. The "tenant" here is the
// control-plane pseudo-tenant "__system__" since this job operates on
// shared infrastructure; it's still tenant-scoped so the verifier can
// gate it.
type AuditPartitionCreateNextPayload struct {
	Tenant    string // "__system__"
	TargetYYM string // e.g. 2026-05
}

// TenantID implements TenantScoped.
func (p AuditPartitionCreateNextPayload) TenantID() string { return p.Tenant }

// AuditDropExpiredPayload — retention cleanup, also system-scoped.
type AuditDropExpiredPayload struct {
	Tenant      string // "__system__"
	OlderThanYM string
}

// TenantID implements TenantScoped.
func (p AuditDropExpiredPayload) TenantID() string { return p.Tenant }

// SystemTenant is the pseudo-tenant used by infrastructure jobs that
// operate on shared resources (audit partitioning, etc.). The
// TenantVerifier must have this registered for such jobs to run.
const SystemTenant = "__system__"
