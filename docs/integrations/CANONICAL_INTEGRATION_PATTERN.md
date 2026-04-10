# Canonical Integration Pattern

**Status:** Normative template for all first-party integrations (KAI-403 through KAI-411).
**Owner:** lead-devex
**Last updated:** 2026-04-08

Every first-party integration — access control (Brivo, OpenPath/Avigilon Alta, ProdataKey, Genetec, Lenel), alarm panels (Bosch, DMP, Honeywell), notifications (Slack, Teams, PagerDuty, Opsgenie), automation (Zapier, Make, n8n, Workato) — MUST follow this pattern. Deviations require lead-devex sign-off in the PR description.

This doc exists so that 8+ parallel sub-agents can ship integration scaffolds in lockstep without drifting on structure, security posture, tenancy discipline, or proto contracts.

## 1. Package layout

Every integration lives under `internal/integrations/<vendor>/` in the Go cloud tree. On-prem integrations that need a Directory-side component additionally live under `internal/directory/integrations/<vendor>/` — but the cloud package is always primary.

```
internal/integrations/
  <vendor>/
    integration.go          # implements the Integration interface
    client.go               # thin HTTP/SOAP/SDK wrapper
    client_test.go          # unit tests against httptest.Server
    auth.go                 # OAuth/API-key/basic-auth handling
    auth_test.go
    webhook.go              # inbound webhook handler (if vendor pushes events)
    webhook_test.go
    events.go               # vendor event → Kaivue event normalizer
    events_test.go
    actions.go              # Kaivue action → vendor call dispatcher
    actions_test.go
    config.go               # per-tenant config struct + validation
    fixtures/               # recorded request/response fixtures (vendor API samples)
    README.md               # vendor overview, auth model, known limits, support matrix
```

Shared plumbing (credential sealing, retry middleware, rate limiting, signature verifier, HMAC rotation, OAuth helpers) lives in `internal/integrations/common/`. **Never** copy-paste those primitives into a vendor package.

## 2. The `Integration` interface

```go
// internal/integrations/integration.go
package integrations

type Integration interface {
    // Metadata for the UI catalog + registry.
    Descriptor() Descriptor

    // Connect validates credentials and runs a live handshake.
    // MUST be idempotent and MUST NOT write to the vendor system.
    Connect(ctx context.Context, tenantID string, cfg Config) (Connection, error)

    // Actions returned here become Casbin-routable actions on this
    // integration's synthetic principal. Keys are stable action IDs
    // (e.g. "brivo.door.unlock") and MUST match proto enum entries.
    Actions() map[string]ActionHandler

    // EventTypes is the set of Kaivue-normalized event types this
    // integration can emit upstream. MUST match proto enum entries.
    EventTypes() []string

    // Health runs a non-mutating probe used by the fleet health dashboard.
    Health(ctx context.Context, conn Connection) HealthStatus
}
```

All vendor packages implement this interface and register via `init()` into `internal/integrations/registry.go`. The registry is the only place UI, API, and webhook dispatchers look up integrations — there is no vendor-specific branching outside the package.

## 3. Proto-first contracts (seam #2)

Every integration MUST have its message shapes defined in proto before any Go code is written:

```
proto/kaivue/integrations/v1/
  common.proto                # Credential, Connection, HealthStatus, ActionResult
  brivo.proto                 # per-vendor config + action enum
  openpath.proto
  ...
```

The action and event enums in proto are load-bearing: the Integration interface references their string names, the Casbin policy matrix references their action IDs, the public API exposes them via `actions.run`, and the webhook dispatcher routes by event type. If you add an action without adding it to proto, the PR does not ship.

Generate with `buf generate` — never hand-write the Go structs. CI gate: `buf lint && buf breaking --against '.git#branch=main'`.

## 4. Multi-tenant discipline (seam #10)

**Every** table an integration creates carries `tenant_id UUID NOT NULL`. No exceptions.

```sql
CREATE TABLE integration_brivo_connections (
    id            UUID PRIMARY KEY,
    tenant_id     UUID NOT NULL REFERENCES tenants(id),
    display_name  TEXT NOT NULL,
    client_id     TEXT NOT NULL,
    client_secret BYTEA NOT NULL,   -- sealed via cryptostore
    created_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at    TIMESTAMPTZ NOT NULL,
    UNIQUE (tenant_id, display_name)
);
CREATE INDEX ON integration_brivo_connections (tenant_id);
```

Every query in the package MUST filter by `tenant_id`. Use the tenant-scoped DB handle from `internal/shared/db/tenantdb`; never use the raw pool. Cross-tenant queries are a CI-failing lint violation (`tenantfilter` linter).

## 5. Credential storage (cryptostore / KAI-251)

Credentials (client secrets, API keys, OAuth refresh tokens, HMAC signing keys) MUST be sealed via the cryptostore envelope before insertion. Never call the raw DB with plaintext.

```go
sealed, err := cryptostore.Seal(ctx, tenantID, "integration:brivo:client_secret", []byte(cfg.ClientSecret))
if err != nil { return err }
// insert `sealed` into client_secret column
```

Rotation of vendor credentials is a first-class operation: every integration surfaces a `RotateCredentials(ctx, connID) error` entry point that wraps the seal/unseal dance. The rotation procedure is documented in the vendor package's README.md.

## 6. Actions: Casbin + synthetic principal

Each inbound action runs under a synthetic Casbin principal of the form `integration:<vendor>:<connection_id>` scoped to the tenant. The action dispatcher:

1. Resolves the connection by `(tenant_id, connection_id)`.
2. Constructs the synthetic principal.
3. Runs the Casbin enforcement check for `action:run` on the specific action ID (`brivo.door.unlock`).
4. On allow, dispatches to the integration package's `ActionHandler`.
5. On deny, returns a generic 403 (no oracle leaks about which step failed).

Every action MUST:
- Return `(ActionResult, error)` where `ActionResult` is the proto-defined common shape.
- Be idempotent where the vendor API permits. If not, echo the caller's idempotency key into an action run log and reject duplicates.
- Emit an audit-log entry (`internal/audit`) including `tenant_id`, `principal`, `action_id`, `connection_id`, `result`, `latency_ms`, `request_id`.

## 7. Inbound webhooks from vendors (if applicable)

Vendors that push events (Brivo, OpenPath, DMP, Bosch, PagerDuty incident updates, Slack interactivity) expose their webhook at:

```
POST /v1/integrations/<vendor>/webhook/{connection_id}
```

This is a sibling of the generic KAI-398 inbound webhook endpoint, but vendor-specific because the signature verification scheme is set by the vendor, not us:

- Brivo/OpenPath: vendor HMAC-SHA256 with shared secret.
- PagerDuty: vendor Ed25519 signature.
- Slack: `v0:timestamp:body` HMAC (Slack signing secret).
- Bosch/DMP: mTLS or IP allowlist + basic auth.

Each vendor's verifier lives in `<vendor>/webhook.go` and MUST:
1. Verify signature first, before any parsing.
2. Enforce the vendor's replay window (or ours, 60s, whichever is stricter).
3. Deduplicate by the vendor's event ID where available.
4. Normalize to a Kaivue event (proto-defined) and hand off to the outbound KAI-397 dispatcher so downstream subscribers see a uniform shape.
5. Emit metrics via `internal/integrations/common/metrics` (received, rejected, normalized, dispatched).

Outcome: a customer subscribing to `event.door.unlocked` sees the same shape regardless of whether the door is on a Brivo panel, an OpenPath reader, or a ProdataKey controller.

## 8. Auto-loop lead-security

Any PR touching the following MUST be tagged for `lead-security` review before merge:

- OAuth client registration, refresh token handling, PKCE verifier.
- HMAC signing/verification code, key rotation logic.
- Webhook signature verifiers (inbound or outbound).
- Credential storage (cryptostore touchpoints).
- Audit log emitters (integrations MUST emit, and lead-security MUST confirm the fields).
- Any code path that might process biometric or face-recognition output from a vendor (EU AI Act scope).

PR template already has a `[ ] lead-security review requested` checkbox — tick it and DM her in-thread.

## 9. i18n discipline (seam #7)

Every user-visible string the integration exposes (catalog name, description, error messages surfaced to UI, action display names, connection field labels) MUST be routed through the i18n catalog at `internal/shared/i18n/`. Never hardcode English.

Vendor-specific error codes from the upstream API are translated to Kaivue error codes in `actions.go`/`webhook.go`, and the Kaivue codes are what get i18n'd. Raw vendor strings are logged for operators but not shown to end users.

## 10. Testing requirements

Minimum bar for a PR to land:

1. **Unit tests** for every exported function. `client_test.go` uses `httptest.NewServer` with fixtures from `fixtures/`. No live network calls.
2. **Auth tests** covering happy path + expired token + rotation + invalid credential.
3. **Webhook tests** covering valid signature, invalid signature, replay (stale timestamp), duplicate event ID, malformed payload.
4. **Action tests** covering happy path, vendor 4xx, vendor 5xx, timeout, tenant mismatch (must reject).
5. **Tenant isolation test**: call every action/query with a wrong `tenant_id` and assert rejection.
6. Coverage ≥ 80% for the vendor package.
7. `go test ./internal/integrations/<vendor>/... -race` must be green.
8. `buf lint` and `buf breaking` green.

## 11. Untracked-files inspection (rescue hygiene)

Before starting work on any integration ticket, the sub-agent MUST:

```bash
cd .worktrees/<ticket-id>
git status
```

Inspect every untracked file in the worktree. If it looks like real scaffolding from a prior cancelled session (non-trivial, consistent with the ticket's goal), READ it and reuse it. Commit with a dedicated commit message:

```
chore(<vendor>): rescue untracked scaffold from prior session — KAI-NNN
```

Then continue forward. Only delete untracked files if they are clearly garbage (editor backups, build artifacts, `.DS_Store`). This rule comes from lead-flutter's rescue hygiene pattern — prior session work has been recovered from exactly this check on multiple worktrees.

## 12. Draft PR structure (during merge freeze)

During the current merge freeze (main HEAD pinned at c19ba24cb), all integration PRs ship as **draft** on their feature branch. The PR description MUST include:

```markdown
## Summary
- KAI-NNN: <vendor> integration scaffold
- Follows docs/integrations/CANONICAL_INTEGRATION_PATTERN.md

## What's in this PR
- Package scaffold at internal/integrations/<vendor>/
- Proto contract at proto/kaivue/integrations/v1/<vendor>.proto
- Unit tests (client, auth, actions, webhook)
- README.md with vendor overview

## What's NOT in this PR (deferred)
- Live vendor account testing (requires sandbox credentials — blocked on devops)
- UI catalog entry (pending web team)
- Zitadel scope wiring (pending lead-cloud)

## Seams checklist
- [x] Proto-first contract (seam #2)
- [x] tenant_id on every table (seam #10)
- [x] IdentityProvider firewall respected (seam #3)
- [x] i18n for all user-visible strings (seam #7)
- [x] Cryptostore for credentials (KAI-251)
- [ ] lead-security review requested

## Merge order
Stacked behind #152 (lint rollup). Will retarget to main after freeze lifts.
```

Mark **draft** until lead-security signs off and the freeze lifts.

## 13. Vendor README template

Every `internal/integrations/<vendor>/README.md` answers:

1. What is this vendor? (one paragraph)
2. Supported auth modes (OAuth2 / API key / HMAC / mTLS / basic)
3. Base URL(s) and region coverage
4. Rate limits observed in the vendor's docs
5. Actions we expose (with proto enum names)
6. Events we normalize from (with proto enum names)
7. Known limitations / edge cases
8. Sandbox/test account setup (how to get one)
9. Rotation procedures for each credential type
10. Links to vendor API docs

## 14. Reference implementation

KAI-403 (Brivo) is the reference. Sub-agents on KAI-404 through KAI-411 should read the Brivo package first and mirror its structure exactly. Divergence is allowed only where the vendor's auth/webhook/event model genuinely differs — and the divergence must be documented in the vendor README.

## 15. Checklist for sub-agents

Before marking the task complete:

- [ ] Worktree created at `.worktrees/<ticket-id>` on `feat/<ticket-id>-<desc>`
- [ ] `git status` inspected, any untracked scaffold rescued
- [ ] Package scaffolded under `internal/integrations/<vendor>/`
- [ ] Proto contract at `proto/kaivue/integrations/v1/<vendor>.proto`
- [ ] `buf generate` run and generated files committed
- [ ] `Integration` interface implemented
- [ ] Registered in `internal/integrations/registry.go`
- [ ] `tenant_id` on every table + every query
- [ ] Cryptostore used for every credential write
- [ ] i18n used for every user-visible string
- [ ] Unit tests green, coverage ≥ 80%
- [ ] `go test ./... -race` green for the package
- [ ] `buf lint` + `buf breaking` green
- [ ] README.md filled in using section 13 template
- [ ] Draft PR opened with section 12 template
- [ ] lead-security looped in if section 8 applies
- [ ] Task marked in_progress → completed on the kaivue-engineering list
