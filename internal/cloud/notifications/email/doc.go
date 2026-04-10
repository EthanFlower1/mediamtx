// Package email provides per-integrator sender domain provisioning for
// Kaivue's transactional email (alerts, invites, notifications).
//
// # Scope (KAI-357)
//
// Each integrator (tenant) brings their own sender domain — e.g.
// "alerts.acme-security.com" — so end-customer emails come from the
// integrator's brand rather than from "@kaivue.io". This package owns:
//
//   - Sender domain CRUD (tenant-scoped)
//   - DKIM keypair generation (RSA 2048, private key held by the
//     KAI-251 cryptostore; only the public key and a cryptostore key
//     id are persisted here)
//   - DNS record computation (SPF include, DKIM TXT at the selector
//     subdomain, DMARC policy record) — what the integrator must add
//     to their DNS zone for verification
//   - Verification poller contract (interface only in v1; a resolver
//     adapter will be wired in by KAI-232 CI/CD)
//   - SendGrid subuser provisioning contract (interface only in v1;
//     the HTTP adapter lives behind the SendGrid seam to keep this
//     package testable without a real API key)
//   - Tenant-scoped audit logging hook (looped into KAI-233 recorder)
//
// # Seam ownership
//
// Infra/Terraform (owned by lead-sre, KAI-357 infra follow-up):
//
//   - Parent SendGrid account + root API key in Secrets Manager
//   - Shared IP pool + reverse DNS
//   - Webhook signing secret
//
// This package reads the parent API key ARN via an injected
// [Provisioner] at runtime (IRSA-read). It never constructs an HTTP
// client to api.sendgrid.com directly.
//
// # Architectural seams enforced
//
//   - Seam #4 (multi-tenant isolation): every store method requires a
//     tenant_id and binds it as the first WHERE predicate. Cross-
//     tenant reads are impossible through the Go API.
//   - Seam #3 (IdentityProvider firewall): this package has no
//     Zitadel imports and does not call into internal/shared/auth.
//   - Seam #1 (package boundaries): no imports from internal/recorder,
//     internal/directory, or any on-prem package.
//
// # DKIM rotation strategy
//
// Two selectors are supported per domain: "s1" and "s2". Rotation
// writes the new keypair under the inactive selector, publishes the
// new public-key TXT record, and flips active_selector after a 48h
// grace window (matching industry practice). KAI-357 v1 implements
// the selector data model; the 48h rotator job ships as a follow-up.
package email
