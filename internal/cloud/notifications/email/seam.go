package email

import "context"

// Provisioner is the seam into the SendGrid parent account for
// subuser creation. The production adapter lives in a subpackage so
// this package can be unit-tested without an HTTP client or a real
// API key. The infra module owned by lead-sre (KAI-357 infra
// follow-up) writes the parent API key to Secrets Manager and
// IRSA-exposes its ARN; the adapter reads it at startup.
type Provisioner interface {
	// CreateSubuser provisions a fresh SendGrid subuser for the
	// given tenant + domain and returns the subuser identifier
	// that should be stored on [Domain.SendGridSubuser]. It must be
	// idempotent: calling twice for the same (tenant, domain) must
	// return the existing subuser rather than creating a second.
	CreateSubuser(ctx context.Context, tenantID, domain string) (string, error)
}

// Resolver is the seam into a DNS resolver used by the verification
// poller to check whether an integrator has published the required
// SPF / DKIM / DMARC records. A net.Resolver wrapper satisfies this
// in production; tests use a map-backed fake.
type Resolver interface {
	// LookupTXT returns the raw TXT record values at name. If the
	// name does not resolve it MUST return a nil slice and a nil
	// error so callers can distinguish "no record" from "lookup
	// failure".
	LookupTXT(ctx context.Context, name string) ([]string, error)
}

// CryptostoreWriter is the seam into KAI-251 cryptostore. Only the
// private key bytes go through this interface; public keys are
// handled directly by the Store.
type CryptostoreWriter interface {
	// StorePrivateKey persists a DKIM private key and returns its
	// opaque cryptostore id, which the caller stores on the
	// DKIMKey row via [Store.InsertDKIMKey]. The key material is
	// never read back out through this package — signing happens
	// inside the cryptostore sign endpoint.
	StorePrivateKey(ctx context.Context, tenantID, pemBytes string) (string, error)
}

// AuditSink is the seam into KAI-233 audit log. Every mutation in
// this package (domain create, DKIM rotation, verification state
// flip) emits one Record call.
type AuditSink interface {
	Record(ctx context.Context, evt AuditEvent) error
}

// AuditEvent is a tenant-scoped record of an email-infra action.
// Intentionally minimal — the audit package owns the canonical
// event shape, this is just the payload.
type AuditEvent struct {
	TenantID string
	Actor    string // user id or "system" for automated rotations
	Action   string // "domain.create" | "dkim.rotate" | "verification.update" | ...
	Target   string // domain id or "{domain_id}/{selector}" for dkim actions
	Reason   string // free-form context
}
