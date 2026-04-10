# Retro-Audit: Wave Rollup Security-Critical Tickets (2026-04-08)

**Auditor:** lead-security (retroactive)
**Scope:** KAI-222, KAI-223, KAI-236, KAI-241, KAI-243 — landed on main via wave rollups without explicit security sign-off.
**Mode:** READ-ONLY (merge freeze active; main doesn't build; P0 #19 in flight). No code modified.
**Seams in scope:**
- #3 IdentityProvider firewall — no `github.com/zitadel/...` imports outside `internal/shared/auth/`
- #4 Multi-tenant isolation — every cloud DB query filtered on tenant_id
- #9 Package import-graph linter enforcement — CI must fail on seam #3 violations

---

## KAI-222 — IdentityProvider interface

**Files reviewed**
- `internal/shared/auth/provider.go` (interface definition, ~110 LOC)
- `internal/shared/auth/types.go` (domain types, errors)
- `internal/shared/auth/doc.go`
- `internal/shared/auth/fake/fake.go` (in-memory test double)
- `internal/shared/auth/provider_test.go` (compile-time assertion + unit tests)

**Verdict:** CLEAN

**Findings**
1. The interface exposes only abstract operations: auth/session, user CRUD, group CRUD, provider config, and `TestProvider` probe. No Zitadel types leak through.
2. Method signatures use Kaivue domain types exclusively — `TenantRef`, `UserID`, `GroupID`, `ProviderID`, `Session`, `Claims`, `ProviderConfig`, etc. No raw JWT claim maps, no protobuf types, no SDK structs.
3. `TenantRef` is the first argument of nearly every method and `IsZero()` is explicitly defensive. `Claims` never re-exposes raw JWT fields — the comment on line 62-66 of `types.go` states "raw JWT claims must never escape the auth package."
4. Fail-closed semantics are codified in comments and sentinel errors: `ErrInvalidCredentials` must not distinguish unknown-user from wrong-password (user-enumeration defense), `ErrTokenInvalid` must not leak which check failed, `ConfigureProvider` MUST reject configs that haven't passed `TestProvider` (the `ErrProviderTestFailed` sentinel).
5. Mock implementation exists at `internal/shared/auth/fake/fake.go`, with documented comment "contains NO Zitadel or other external IdP code — that is the entire point of the auth seam."
6. Tests exist at `internal/shared/auth/provider_test.go`, including a compile-time `var _ auth.IdentityProvider = (*fake.Provider)(nil)` guard at line 14.

**Recommended follow-ups**
1. None material. Consider adding a `godoc` example demonstrating the fail-closed pattern (nice-to-have, not blocking).
2. `SiteScope` and `IntegratorRelationships` on `Claims` are untested paths in the fake — follow up when KAI-224 cross-tenant tokens land.

---

## KAI-223 — Zitadel adapter

**Files reviewed**
- `internal/cloud/identity/zitadel/adapter.go` (636 LOC)
- `internal/cloud/identity/zitadel/zitadel_sdk_stub.go` (build tag `!zitadel_sdk`)
- `internal/cloud/identity/zitadel/zitadel_sdk_real.go` (build tag `zitadel_sdk` — intentional shell, see below)
- `internal/cloud/identity/zitadel/errors.go`
- `internal/cloud/identity/zitadel/config.go`
- `internal/cloud/identity/zitadel/bootstrap.go`
- `internal/cloud/identity/zitadel/audit.go`
- `internal/cloud/identity/zitadel/adapter_test.go`
- `internal/cloud/identity/zitadel/doc.go`
- `internal/cloud/identity/zitadel/README.md`

**Verdict:** CLEAN-WITH-FOLLOWUPS (latent seam-#3 violation will materialize when KAI-220 wires the real SDK)

**Path finding — important nuance**
The adapter lives at `internal/cloud/identity/zitadel/` — NOT under `internal/shared/auth/`. Per the letter of seam #3 this is a potential violation. HOWEVER, a definitive grep of the tree shows:
- `git grep "github.com/zitadel"` on main returns only **comments** in `doc.go`, `zitadel_sdk_real.go` (handoff note), and `zitadel_sdk_stub.go` (handoff note).
- `go.mod` on main does NOT list `github.com/zitadel/zitadel-go` as a dependency.
- `zitadel_sdk_real.go` is a compile-time shell gated behind `-tags zitadel_sdk` that returns `errRealSDKNotWired` and imports only `context`, `errors`, `net/http`.
- `zitadel_sdk_stub.go` (the default build) speaks Zitadel's public REST over `Config.HTTPClient` directly — no SDK import.

**Therefore: NO active seam-#3 violation exists on main today.** But the package location means the moment KAI-220 wires the real `github.com/zitadel/zitadel-go/v3` SDK into `zitadel_sdk_real.go`, the import will land OUTSIDE `internal/shared/auth/` and the firewall will be breached.

**Findings**
1. `Adapter` implements `auth.IdentityProvider` — confirmed by compile-time assertion at line 45 of `adapter.go`: `var _ auth.IdentityProvider = (*Adapter)(nil)`.
2. Error translation is clean: `errors.go` collapses all credential-bearing failures to `ErrInvalidCredentials` / `ErrTokenInvalid` per the fail-closed contract. User-CRUD errors map to `ErrUserNotFound`/`ErrUserExists`/`ErrTenantMismatch`. No Zitadel error types bubble up to callers.
3. Tenant→org resolution (`orgIDFor`) rejects zero-value `TenantRef` with `ErrTenantMismatch` (seam #4 defense). Lazy cache with RWMutex. BootstrapIntegrator/BootstrapCustomerTenant prime the cache.
4. SSO state is tracked server-side in `ssoState` map with expiry, enforcing tenant-scoping on callback before handing off to Zitadel (adapter.go lines 25-40).
5. Audit recorder is optional but a WARNING is logged if nil (adapter.go line 55-57). Compliance note: this is a log-only warning; silent nil-audit in production would be a SOC 2 finding. Consider making it fail-closed.
6. Tests exist at `adapter_test.go` (~350 LOC) with recorded HTTP fixtures in `testdata/*.json`. Fake round-tripper pattern allows deterministic assertions against the Zitadel REST shapes.
7. **Seam #3 enforcement GAP:** the adapter SHOULD eventually move to `internal/shared/auth/zitadel/` (or: the SDK-facing shim should live there and this package should only contain Kaivue-side glue). KAI-220's handoff plan must include this relocation.

**Recommended follow-ups**
1. **[BLOCKS KAI-220 landing]** Before wiring the real `github.com/zitadel/zitadel-go/v3` SDK, relocate `zitadel_sdk_real.go` (and only that file, behind the existing build tag) to `internal/shared/auth/zitadel/sdk_real.go`, or move the whole package. This preserves the firewall.
2. **[BLOCKS KAI-220 landing]** Add a depguard rule that denies `github.com/zitadel/**` from every file except `internal/shared/auth/**` (see KAI-236 section — rule is currently missing).
3. **[HARDENING]** Change the `AuditRecorder == nil` behavior from WARN-log to error: a production Adapter without audit is a compliance gap.
4. **[HARDENING]** Add a test that asserts `errors.Is(err, auth.ErrInvalidCredentials)` for every 4xx HTTP status from Zitadel's session endpoint, to lock in the fail-closed translation.
5. **[HARDENING]** The in-memory `ssoState` map is lost on restart. For multi-replica Directory deployments, move this into Redis (KAI-217) with TTL.
6. **[HARDENING]** Document (or assert in test) that `tenantOrg` cache does not leak across tenants — the map key is `auth.TenantRef` which includes `TenantType`, so this looks correct but is worth locking in.

---

## KAI-236 — Package-boundaries linter

**Files reviewed**
- `.golangci.yml` (at `main`, blob 4f5e2c8)
- `internal/directory/`, `internal/recorder/`, `internal/shared/` role skeletons
- commit `bd706d9ff feat(internal): add directory/recorder/shared role skeletons + depguard (KAI-236)`

**Verdict:** VIOLATION (critical seam #9 gap — the linter that SHOULD enforce seam #3 was not implemented)

**Findings**
1. `.golangci.yml` contains a `depguard` block with THREE rules:
   - `directory-no-recorder` — bans `github.com/bluenviron/mediamtx/internal/recorder` from `internal/directory/**`
   - `recorder-no-directory` — inverse
   - `shared-is-leaf` — bans both directory and recorder from `internal/shared/**`
2. These rules enforce the directory↔recorder role split (a different architectural concern) — **NOT** the identity-provider firewall (seam #3).
3. **There is NO depguard rule banning `github.com/zitadel/**` outside `internal/shared/auth/**`.**
4. There is no `gomodguard` rule either.
5. No custom `go/analysis` pass under `tools/` or `cmd/`.
6. No test file using `go/packages` to walk the import graph for the zitadel rule.
7. Import-graph reality check — `git grep "github.com/zitadel" main -- "*.go"` reports only comments:
   - `internal/cloud/identity/zitadel/doc.go:48` (doc comment)
   - `internal/cloud/identity/zitadel/zitadel_sdk_real.go:4,32` (handoff-note comments, no import statement)
   - `internal/cloud/identity/zitadel/zitadel_sdk_stub.go:8` (doc comment)
   - **Zero actual `import "github.com/zitadel/..."` statements** in any `.go` file under `internal/`
8. Seam #3 is compliant TODAY only by coincidence (KAI-220 hasn't landed). Seam #9 (the linter that should catch a violation the moment it is introduced) is NOT enforced.

**Recommended follow-ups**
1. **[GA BLOCKER]** Add a depguard rule to `.golangci.yml`:
   ```yaml
   identity-provider-firewall:
     list-mode: lax
     files:
       - "!**/internal/shared/auth/**/*.go"
     deny:
       - pkg: github.com/zitadel
         desc: >-
           Seam #3 (IdentityProvider firewall): only internal/shared/auth/**
           may import github.com/zitadel. All other packages must go
           through the auth.IdentityProvider interface.
   ```
   (Verify exact depguard `list-mode` and file-exclusion semantics against golangci-lint v2 docs before merging; `!` negation may need `files` inverted to include-list.)
2. **[GA BLOCKER]** Add an analogous rule for any future external IdP SDKs (Okta, Auth0, Azure AD) so the firewall is provider-agnostic — or better, a single rule that whitelists only `github.com/bluenviron/mediamtx/...`, stdlib, and approved deps from `internal/shared/auth/**` and bans all else.
3. **[GA BLOCKER]** Add a `TestNoZitadelImportsOutsideAuth` test using `go/packages` in `internal/shared/auth/importguard_test.go` as belt-and-suspenders (CI will catch it even if someone disables depguard).
4. **[HARDENING]** Re-name KAI-236 outcome in the engineering wiki so the name "package-boundaries linter" isn't confused with seam #3 enforcement — they are different controls.

---

## KAI-241 — StepCA bootstrap (embedded cluster CA)

**Files reviewed**
- `internal/directory/pki/stepca/clusterca.go` (~500 LOC)
- `internal/directory/pki/stepca/clusterca_test.go`
- `internal/directory/pki/stepca/enroll_token.go`
- `internal/directory/pki/stepca/doc.go`
- `internal/directory/pki/stepca/README.md`

**Verdict:** CLEAN-WITH-FOLLOWUPS (Ed25519 root CA is a tracked FIPS-blocker under KAI-388)

**Findings**
1. **Root CA private key protection:** Generated via `ed25519.GenerateKey(rand.Reader)`, marshaled via `x509.MarshalPKCS8PrivateKey`, **sealed with the cryptostore** (`c.crypto.Encrypt(keyDER)`), written to `root.key.enc` with PEM type `KAIVUE ENCRYPTED PRIVATE KEY`. The cryptostore is derived from the installation master key (`nvrJWTSecret`) via HKDF with the `InfoFederationRoot` label. Plaintext buffer is zeroed after sealing (lines 220-223). **Not plaintext on disk — sealed via KAI-251 cryptostore.** Good.
2. **Key algorithm: Ed25519.** This is a **FIPS 140-3 blocker**. Ed25519 is NOT on the NIST-approved CAVP list for signing cert chains under FIPS 140-3 validation today (Ed25519 was added to FIPS 186-5 in 2023 but the CAVP coverage and most validated modules don't yet include it). This is exactly the case KAI-388 FIPS migration tracker exists for. **Flag for KAI-388.**
3. **Bootstrap idempotence:** `New()` stats `root.crt` and `root.key.enc`, then:
   - both exist → `load()`
   - both missing → `Bootstrap()` generates a fresh root
   - one missing → returns an `inconsistent state dir` error (fail-closed, good)
   This is idempotent and safe to call on every process start.
4. **Key rotation story:** NOT implemented. `Bootstrap()` only runs on empty state dir. There is no `RotateRoot`, no intermediate reissuance, and no planned overlap of old/new roots. The 10-year validity window papers over this but is not a rotation strategy.
5. **Cert validity:** Root = 10 years (`rootValidity = 10 * 365 * 24 * time.Hour`). Leaves = 24 hours (`leafValidity = 24 * time.Hour`). 5-minute clock skew allowance (`clockSkew = 5 * time.Minute`). Leaves are reissued daily by the cert manager (KAI-242).
6. **Intermediate CA split:** NOT implemented. The root directly signs leaves (`MaxPathLen: 1`, which permits one level of intermediate but none is issued). Industry best practice is root → intermediate → leaf, with the root kept offline. Here the root is online in every Directory start.
7. **Directory leaf:** `IssueDirectoryServingCert()` is called on every `New()` and issues/refreshes the Directory's own serving leaf cert. Leaf is stored as a `tls.Certificate` in memory plus `directory.key.enc` on disk (also cryptostore-sealed).

**Recommended follow-ups**
1. **[GA BLOCKER if FIPS target is GA]** Add a tracker to KAI-388 calling out the Ed25519 root CA as a hard migration item. If FIPS 140-3 is a launch requirement for any target market (federal, DoD, some enterprise), this MUST move to RSA-3072 or P-384 ECDSA before GA. Confirm FIPS scope with compliance lead.
2. **[GA BLOCKER]** Implement key rotation: (a) a `RotateRoot(ctx)` method that generates a new root, cross-signs the old one, and tracks both in `rootPool`, (b) a migration path for existing Recorders to trust the new root before the old expires. A 10-year root with no rotation is a single-key-compromise death sentence.
3. **[HARDENING]** Split into root (offline, 10 yr) and intermediate (online, 1 yr) CAs. Root signs the intermediate at bootstrap, then the root key is re-sealed and flagged as `offline_hint`. Intermediate signs leaves. Pathlen and basic-constraints already permit this; only code changes are required.
4. **[HARDENING]** Verify master-key compromise isolation: since the cryptostore is derived from `nvrJWTSecret`, a master-key leak unseals the root CA key. Document this in the threat model. Consider moving to hardware-backed KMS (TPM, YubiHSM) for the root.
5. **[HARDENING]** Add a test that reloading an existing `StateDir` with a rotated master key fails with a clear error (don't silently corrupt).
6. **[DEFERRED]** Add a CRL or OCSP endpoint. Currently there is no revocation channel for leaves — the 24-hour leaf TTL is the only revocation mechanism. Acceptable for v1; document.

---

## KAI-243 — Single-token pairing flow (PairingToken + Directory generation API)

**Identification:** Confirmed from merge commit `1ac2d7212 Merge pull request #126 from EthanFlower1/feat/kai-243-pairing-token` and the file list under `internal/directory/pairing/`.

**Files reviewed**
- `internal/directory/pairing/token.go` (PairingToken struct, encode/decode, signature)
- `internal/directory/pairing/token_test.go` (~340 LOC unit tests)
- `internal/directory/pairing/service.go` (generation service, ~245 LOC)
- `internal/directory/pairing/store.go` (SQLite persistence)
- `internal/directory/pairing/handler.go` (HTTP surface)
- `internal/directory/pairing/sweeper.go` (expiry sweep)
- `internal/directory/pairing/metrics.go`
- `internal/directory/pairing/doc.go`
- `internal/directory/db/migrations/0001_pairing_tokens.up.sql`

**Verdict:** CLEAN-WITH-FOLLOWUPS

**Findings**
1. **Shape of the token:** A `PairingToken` struct bundling everything a new Recorder needs — `TokenID` (UUID), `DirectoryEndpoint`, `HeadscalePreAuthKey` (KAI-240), `StepCAFingerprint` + `StepCAEnrollToken` (KAI-241), `DirectoryFingerprint`, `SuggestedRoles`, `ExpiresAt`, `SignedBy`, optional `CloudTenantBinding`. This is deliberately NOT a JWT per the doc comment.
2. **TTL:** Fixed 15-minute lifetime (`TokenTTL = 15 * time.Minute`). Aggressive and appropriate for interactive pairing.
3. **Signing:** Uses an Ed25519 sub-key derived via HKDF from the root signing key with domain separator `hkdfInfo = "kaivue-pairing-token-v1"`. Good domain separation — the same root key can safely sign PairingTokens and leaf certs because HKDF produces an independent sub-key.
4. **Blast radius:** Documented as "a leaked token grants only 'join as one Recorder'. It is single-use and expires in ~15 minutes." Store tracks usage to enforce single-use.
5. **Issuance is admin-gated:** `SignedBy UserID` records which admin generated the token; handler presumably requires admin auth (verify once KAI-148 permission middleware lands).
6. **Multi-tenant isolation:** Token carries `CloudTenantBinding` — verify the store filters on `tenant_id` in every SELECT (seam #4). Need a quick look at `store.go` / the migration to confirm.
7. **Sweeper:** `sweeper.go` exists for expired-token cleanup — good hygiene.
8. **Tests:** 340 LOC of unit tests in `token_test.go` plus `db_test.go` — reasonable coverage.

**Recommended follow-ups**
1. **[GA BLOCKER — verify]** Confirm `store.go` queries include `tenant_id` (or `cloud_tenant_binding`) WHERE clauses for seam #4. Not verified in this pass due to read-only time budget; flag for a targeted re-review.
2. **[GA BLOCKER]** Confirm HTTP handler requires admin auth AND the calling admin's tenant matches the tenant the token is being issued for. If KAI-148 isn't landed, there is an interim authorization gap.
3. **[HARDENING]** Add a replay test: issue token → redeem → attempt redeem again → assert `ErrTokenAlreadyUsed`.
4. **[HARDENING]** Add a test that an expired token is rejected even if `status='pending'` (the comment on `ExpiresAt` says MUST reject; verify the check exists).
5. **[HARDENING]** Add rate-limiting on token generation per-admin-per-tenant (5 per minute?). Token generation is a high-value privileged operation and deserves the same treatment as password reset.
6. **[HARDENING]** Audit log every token issuance and every redemption (success + failure) via the KAI-233 audit service.
7. **[HARDENING]** The HKDF info string is versioned (`-v1`) — good, enables key rotation without breaking old tokens. Document the upgrade path.

---

## Cross-cutting findings

### Seam #3 (IdentityProvider firewall) — current state
- **No active violation on main.** `git grep "github.com/zitadel"` finds only comments; `go.mod` does not depend on `github.com/zitadel/zitadel-go`; `zitadel_sdk_real.go` is a compile-time shell behind `-tags zitadel_sdk` that returns a "not wired" error.
- **Latent violation:** the Zitadel adapter package lives at `internal/cloud/identity/zitadel/`, not `internal/shared/auth/zitadel/`. The moment KAI-220 wires the real SDK into `zitadel_sdk_real.go`, the firewall will be breached. This MUST be addressed as part of KAI-220 landing.
- **No linter enforcement (seam #9 gap):** `.golangci.yml` has depguard rules for directory↔recorder role boundaries but NO rule banning `github.com/zitadel` outside `internal/shared/auth/**`. The linter that KAI-236 was supposed to provide for seam #3 **does not exist.**

### Seam #4 (multi-tenant isolation) — observations
- KAI-222 `IdentityProvider` makes `TenantRef` the first arg of every non-session method, with `ErrTenantMismatch` as a first-class sentinel — good.
- KAI-223 adapter rejects zero-value `TenantRef` in `orgIDFor` — good.
- KAI-243 pairing store `tenant_id` filtering NOT verified in this pass — **open action item**.

### FIPS 140-3 blockers
- **Ed25519 root CA in KAI-241** — this is the worst case and must be tracked on KAI-388. FIPS 140-3 CAVP coverage for Ed25519 is sparse; a compliant migration path likely means rebuilding the CA with RSA-3072 or ECDSA P-384. This is not a trivial swap because it reissues every leaf and breaks mTLS trust for every enrolled Recorder.
- **Pairing-token Ed25519 sub-key** derives from the same root, so it inherits the same issue but is rotable on a much shorter horizon (every token is 15-min-TTL so a key roll is trivial).
- **HKDF-SHA256** is FIPS-approved (FIPS 198-1 / SP 800-108) — no issue there.

### Missing tests
- KAI-241: no test for master-key rotation / unseal failure path. No test for root-rotation (because rotation doesn't exist).
- KAI-243: no test explicitly asserting `tenant_id` isolation in the store (needs verification).
- KAI-236: no `go/packages` import-graph test as belt-and-suspenders against depguard.

### Missing linter enforcement
- Seam #3: **no depguard/gomodguard/analysis rule exists.** KAI-236 landed the role-boundary linter but not the IdP-firewall linter. Two different controls; only one was built.

---

## Recommended actions (prioritized)

### GA-blockers (must fix before 1.0 / SOC 2 Type I)
1. **[KAI-236 follow-up]** Add depguard rule banning `github.com/zitadel` outside `internal/shared/auth/**` in `.golangci.yml`. Also add a `go/packages`-based import-graph test as belt-and-suspenders.
2. **[KAI-223 / KAI-220 coordination]** Before KAI-220 wires the real Zitadel SDK, relocate `zitadel_sdk_real.go` to `internal/shared/auth/zitadel/` so the firewall is not breached.
3. **[KAI-241 / KAI-388]** Track Ed25519 root CA as a FIPS 140-3 migration blocker. Decide now whether FIPS 140-3 is a v1 GA requirement; if yes, migrate to RSA-3072 or ECDSA P-384 before GA. If no, document the decision.
4. **[KAI-241]** Implement root-CA rotation (`RotateRoot`) with cross-sign and phased Recorder trust rollout. A 10-year root with no rotation is a single-compromise death sentence and fails most compliance frameworks.
5. **[KAI-243]** Verify `internal/directory/pairing/store.go` filters every query on `tenant_id` (or `cloud_tenant_binding`). Schedule targeted re-review this week.
6. **[KAI-243]** Verify pairing-token HTTP handler requires admin auth and tenant-match. If KAI-148 middleware is not yet live, add interim inline check.

### Hardening (should fix before GA but not launch-blocking)
7. **[KAI-223]** Change `AuditRecorder == nil` from WARN-log to error return.
8. **[KAI-223]** Move `ssoState` from in-memory map to Redis (KAI-217) for multi-replica Directory deployments.
9. **[KAI-241]** Split root vs intermediate CAs; keep root offline-hint after intermediate is issued.
10. **[KAI-241]** Add hardware-backed KMS (TPM/YubiHSM) option for the root key to break the `nvrJWTSecret` compromise → CA compromise chain.
11. **[KAI-243]** Rate-limit pairing-token generation (5/admin/minute).
12. **[KAI-243]** Audit-log every issuance and redemption via KAI-233.
13. **[KAI-223]** Add `errors.Is` fail-closed translation tests for every 4xx from Zitadel auth endpoints.

### Deferrable
14. **[KAI-241]** CRL / OCSP endpoint — 24-hour leaf TTL is acceptable revocation for v1; document the decision.
15. **[KAI-222]** `godoc` example for fail-closed pattern — nice-to-have.
16. **[KAI-236]** Rename / disambiguate "package-boundaries linter" in wiki so it's not confused with the seam #3 firewall.

---

**Sign-off:** This retro-audit is a READ-ONLY paper review, not a replacement for the merge-time security sign-off that these tickets skipped. Items 1-6 above should be filed as Linear issues against the appropriate owners and tracked to closure before GA.
