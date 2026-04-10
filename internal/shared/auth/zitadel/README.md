# `internal/shared/auth/zitadel`

Zitadel-backed implementation of `auth.IdentityProvider` (KAI-223).

This package is **seam #3** of the multi-tenant architecture: it is the
only place in the codebase that imports Zitadel SDK / REST types. Every
other package consumes identity through `internal/shared/auth`.

## Org hierarchy

Kaivue's two-level tenant model maps onto Zitadel orgs like this:

```
Kaivue                          Zitadel
------                          -------
"platform" (the product)  ─────▶  Platform org  (Config.PlatformOrgID)
Integrator tenant         ─────▶  Org  (created with ParentOrg=Platform)
Customer  tenant          ─────▶  Org  (created with ParentOrg=Integrator)
User                      ─────▶  User inside tenant's org
Group                     ─────▶  Project role inside tenant's org
```

Every API call carries the `x-zitadel-orgid` header derived from the
caller's `TenantRef` — **never** from a client-supplied body. A zero-value
`TenantRef` is rejected with `auth.ErrTenantMismatch`; see `orgIDFor` in
`adapter.go`.

## Service-account bootstrap flow

1. **KAI-220** deploys Zitadel and provisions a platform-level
   service-account with "org manager" + "user manager" roles on the
   platform org.
2. The service-account's JWT profile key is saved to a file (e.g.
   `/etc/kaivue/zitadel-sa.json`). **Never commit it.** Tests use
   `testdata/REPLACE_ME.json` as a stand-in path.
3. `zitadel.New(ctx, Config{...})` loads the key once at startup and
   exchanges it for short-lived admin tokens on every call.
4. **KAI-227** (tenant provisioning service) calls:
   - `BootstrapIntegrator(ctx, Integrator{TenantID, DisplayName})` —
     creates an org under the platform and primes the adapter's cache.
   - `BootstrapCustomerTenant(ctx, CustomerTenant{..., ParentIntegratorOrgID})`
     — creates a customer org under a specific integrator.
   - `ProvisionUser(ctx, tenant, UserSpec{...})` — creates the first
     admin user inside a tenant's org.
5. **KAI-224** (cross-tenant access) will consume `*Adapter` for scoped
   token minting. The public API (`*Adapter`, `Config`, `New`, the
   bootstrap helpers) is the compatibility surface.

All three bootstrap helpers are idempotent on `409 Conflict` — they look
up the existing org by name and return its ID. Half-bootstrapped tenants
(failed org creation) are NEVER added to the cache.

## Fail-closed contract

- Any Zitadel transport / JSON / auth error → typed sentinel from
  `internal/shared/auth` (`ErrInvalidCredentials`, `ErrTokenInvalid`, …).
- `AuthenticateLocal` / `RefreshSession` / `VerifyToken` collapse every
  failure path to a single error — **no user enumeration**. The tests in
  `adapter_test.go` assert this for both "unknown user" (404) and "wrong
  password" (401).
- `CompleteSSOFlow` validates the `state` token against a server-side
  binding to the tenant; cross-tenant or replayed state → `ErrSSOStateInvalid`.
- `ConfigureProvider` refuses to persist a config that hasn't passed
  `TestProvider` — the round-trip probe is a first-class citizen.
- `ListUsers` / `GetUser` enforce a belt-and-suspenders `resourceOwner`
  check on every row; a misconfigured Zitadel instance that leaks
  cross-org users would still be filtered client-side.
- Nil `AuditRecorder` is permitted but `New` logs a loud warning; every
  successful auth operation emits an `audit.Entry` via the Recorder.

## Swapping back to the fake for tests

Downstream packages that want an in-memory identity provider for their
own unit tests should import `internal/shared/auth/fake` and construct
`fake.New()` — that returns a `*fake.Provider` which also satisfies
`auth.IdentityProvider`. Never import this package from a test.

## Build tags

```
go build ./internal/shared/auth/zitadel/...                # stub (default)
go build -tags zitadel_sdk ./internal/shared/auth/zitadel/... # real SDK (KAI-220)
```

- **Default (`!zitadel_sdk`):** `zitadel_sdk_stub.go` speaks Zitadel's
  public REST shapes directly over `Config.HTTPClient`. This is the
  build every CI job uses today, since the real SDK isn't vendored yet
  (see "KAI-220 handoff" below).
- **`-tags zitadel_sdk`:** `zitadel_sdk_real.go` is the slot where
  `github.com/zitadel/zitadel-go/v3` gets wired in. Building with the
  tag today returns `errRealSDKNotWired` from every call so a partially
  migrated codebase fails loudly instead of silently.

The public API (`*Adapter`, `Config`, `New`, bootstrap helpers) is
identical under both tags.

## KAI-220 handoff

When Zitadel is deployed and the real Go SDK is vendored:

1. `go get github.com/zitadel/zitadel-go/v3@<version>`
2. Replace the body of `newSDKClient` + `doJSON` in
   `zitadel_sdk_real.go` with SDK calls.
3. Port the typed request/response structs to their SDK equivalents (the
   JSON shapes in `zitadel_sdk_stub.go` were deliberately picked to
   match the SDK's generated types).
4. Flip CI to build with `-tags zitadel_sdk`.
5. `adapter.go` and the tests should require **zero** changes.

No real service-account keys live in this package. Tests use
`testdata/REPLACE_ME.json` as a fixture path; the stub doesn't actually
read the file because it stubs out the JWT exchange entirely.

## Audit log integration

Every successful auth operation calls `Adapter.auditEmit` which
constructs an `audit.Entry` and hands it to `Config.AuditRecorder`. The
tests assert allow entries are emitted for login, SSO completion, and
user CRUD. Nil recorder is a no-op (with a startup warning).
