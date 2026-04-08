# `internal/cloud/identity/crosstenant` — Cross-tenant access service (KAI-224)

This package is the cloud control-plane service that issues short-lived
**scoped tokens** when integrator-staff users access one of their managed
customer tenants. It is the single choke point for cross-tenant impersonation
and — together with the Casbin enforcer (KAI-225) and the audit log (KAI-233)
— implements architectural seam #4 ("tenant isolation firewall").

## Mission

> Integrator staff must never hold a long-lived credential for a customer
> tenant. Every cross-tenant action is authorized by a freshly minted,
> short-lived, revocable, audit-logged scoped token.

## Surface

```go
type Service struct { /* ... */ }

func NewService(cfg Config,
    identity auth.IdentityProvider,
    relationshipStore RelationshipStore,
    permissionStore permissions.IntegratorRelationshipStore,
    sessionStore ScopedSessionStore,
    auditRecorder audit.Recorder,
) (*Service, error)

func (s *Service) MintScopedToken(ctx, integratorUserID, customerTenantID) (*ScopedToken, error)
func (s *Service) VerifyScopedToken(ctx, token string) (*ScopedClaims, error)
func (s *Service) RevokeScopedSession(ctx, sessionID string) error
```

The `ScopedToken` returned from `MintScopedToken`:

| Field              | Meaning                                                                 |
| ------------------ | ----------------------------------------------------------------------- |
| `Token`            | Signed JWT (HS256) the client presents back on every cross-tenant call. |
| `SessionID`        | Opaque id used by `RevokeScopedSession` to terminate the session.       |
| `ExpiresAt`        | Absolute UTC expiry (default TTL = 15 minutes).                         |
| `CustomerTenantID` | The tenant whose data the token authorizes.                             |
| `PermissionScope`  | Intersected allow-list of Casbin actions (deterministic order).         |

The `ScopedClaims` returned from `VerifyScopedToken` contains the same info
plus the canonical Casbin subject:

```
integrator:<integrator_user_id>@<customer_tenant_id>
```

This is the exact prefix format used by KAI-225
`permissions.NewIntegratorSubject`.

## Flow — `MintScopedToken`

```
┌──────────────────────────────────────────────────────────────────────────┐
│ 1. identity.GetUser(integratorTenant, integratorUserID)                  │
│     └─ not found → ErrUnknownIntegrator (+ audit DENY)                   │
│                                                                          │
│ 2. relationshipStore.Lookup(uid, customerTenantID)                       │
│     ├─ missing       → ErrNoRelationship       (+ audit DENY)            │
│     └─ revoked=true  → ErrRelationshipRevoked  (+ audit DENY)            │
│                                                                          │
│ 3. permissions.ResolveIntegratorScope(uid, customerTenant)               │
│     - walks parent_integrator chain bounded by 32                        │
│     - intersects ScopedActions at every hop                              │
│     ├─ depth > 32 → ErrHierarchyTooDeep        (+ audit DENY)            │
│     └─ empty      → ErrEmptyScope              (+ audit DENY)            │
│                                                                          │
│ 4. Build JWT claims (sub, iat, nbf, exp, sid, integrator_user_id,        │
│    integrator_tenant_id, customer_tenant_id, scope)                      │
│                                                                          │
│ 5. sessionStore.Put(ScopedSessionRecord{…})                              │
│                                                                          │
│ 6. audit.Record(action="permissions.cross_tenant_grant", result=ALLOW,   │
│                 tenant_id=customer, actor_agent="integrator", …)         │
└──────────────────────────────────────────────────────────────────────────┘
```

## Flow — `VerifyScopedToken`

```
1. jwt.Parse with HS256, issuer=mediamtx-cloud/crosstenant, clock=cfg.Now
     ├─ expired → ErrScopedTokenExpired (+ audit DENY)
     ├─ bad sig / wrong alg / malformed → ErrScopedTokenInvalid (+ audit DENY)
2. sessionStore.Get(sid)
     ├─ missing or Revoked=true → ErrSessionRevoked (+ audit DENY)
3. audit.Record(action="permissions.cross_tenant_verify", result=ALLOW, …)
4. Return *ScopedClaims
```

## Fail-closed contract

| Situation                                 | Error                    |
| ----------------------------------------- | ------------------------ |
| Integrator user unknown                   | `ErrUnknownIntegrator`   |
| No customer_integrator_relationships row  | `ErrNoRelationship`      |
| Relationship marked revoked               | `ErrRelationshipRevoked` |
| Intersection of parent chain is empty     | `ErrEmptyScope`          |
| Parent chain exceeds depth 32             | `ErrHierarchyTooDeep`    |
| JWT expired / wrong signing key / bad sig | `ErrScopedTokenInvalid`, `ErrScopedTokenExpired` |
| Session revoked                           | `ErrSessionRevoked`      |

All of these paths still emit an `audit.Entry` with `result=deny` and a
populated `error_code`. **Every cross-tenant action, including denied ones,
emits exactly one audit entry.** This is the contract other packages rely on.

## Audit contract

Every `MintScopedToken` call emits one entry with
`action = "permissions.cross_tenant_grant"`. Every `VerifyScopedToken` call
emits one entry with `action = "permissions.cross_tenant_verify"`.

Both entries are written against the **customer tenant** (`TenantID`) with
`ImpersonatedTenantID` also set to the customer tenant and
`ImpersonatingUserID` set to the integrator staff uid, so KAI-233's
`IncludeImpersonatedTenant` query surfaces the entries to both the customer
and the integrator.

`ActorAgent` is always `audit.AgentIntegrator`.

## Sub-reseller intersection

The package delegates scope resolution to KAI-225
`permissions.ResolveIntegratorScope`, which walks `ParentIntegrator` links and
intersects `ScopedActions` at every level. The narrowest link in the chain
wins: a leaf can never broaden a parent. The chaos test
`TestChaos_ThreeLevelSubResellerHierarchyIntersection` verifies this end-to-
end with a root → mid → leaf hierarchy.

## Signing keys

In tests we use `sha256("test-jwt-key-REPLACE_ME")`. In production the signing
key comes from the cloud's KMS wiring; this package never loads the key
itself, it just accepts it via `Config.SigningKey`. **No real signing keys
live in this package or in any of its fixtures.**

## Non-goals / seams owned by sibling agents

- **KAI-223** (Zitadel adapter) provides the `auth.IdentityProvider` — this
  package accepts whatever implementation is injected and never imports the
  adapter directly.
- **KAI-227** (tenant provisioning) owns the RDS-backed
  `customer_integrator_relationships` table. KAI-224 ships an in-memory
  `RelationshipStore` stub; the real one will replace it without touching
  `Service`.
- **KAI-226** (cloud API server) exposes the
  `POST /api/v1/integrators/{id}/impersonate` handler that calls
  `MintScopedToken` and returns the JWT to the browser. That handler is **not**
  in this package.

## Testing

```bash
go test ./internal/cloud/identity/crosstenant/...
```

12 tests cover: mint happy path, unknown integrator, missing relationship,
revoked relationship, sub-reseller narrowing, audit emission, verify happy
path, expired token, revoked session, wrong-signing-key, 3-level chaos
intersection, empty-scope fail-closed.
