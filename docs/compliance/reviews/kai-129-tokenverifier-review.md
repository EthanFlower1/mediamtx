# Security Review — KAI-129 TokenVerifier (PR #167)

**Reviewer:** lead-security
**Date:** 2026-04-08
**Branch:** `feat/kai-129-token-verifier`
**Worktree:** `.worktrees/kai-129`
**Scope:** `internal/shared/auth/tokenverify/` (3 files, +930 LOC)
**Criticality:** P0 — this primitive verifies every JWT in the product. A single bug is catastrophic.

---

## 1. File-by-file walkthrough

### `internal/shared/auth/tokenverify/doc.go` (78 lines)
Package-level documentation. The security contract is explicitly enumerated (items 1–8 at lines 32–46). Worth calling out:
- Line 44 states `SkewTolerance = 60 seconds` — within the 2-minute hard cap requirement. Good.
- Line 36 explicitly calls out `alg: none` and `alg: HS256` as unconditionally rejected, and cites spec-confusion as the motivator.
- Lines 58–64 describe the rotation behavior: kid-miss triggers one forced refresh, rate-limited.

No issues in documentation. The contract as documented is correct; my concern is whether the implementation matches it (see below).

### `internal/shared/auth/tokenverify/verifier.go` (302 lines)

**Lines 21–25 — Constants**
- `SkewTolerance = 60 * time.Second` — PASS, well under the 2-minute cap.
- `DefaultCacheTTL = 5 * time.Minute` — matches requirement.

**Lines 30–36 — Algorithm allowlist**
- Exactly `RS256, RS384, RS512, ES256, ES384`. No HS*, no `none`, no Ed25519 (EdDSA) — intentional; good for FIPS 140-3 roadmap (KAI-388).
- **Line 27 comment** incorrectly says "`HS256` are intentionally excluded" — but the allowlist also excludes HS384/HS512. Comment is accurate if slightly narrow; not a bug.

**Lines 121–166 — `NewTokenVerifier` (constructor)**
- Lines 122–130: validates `JWKSURL`, `Issuer`, `Audience` are non-empty. **Does NOT validate that `JWKSURL` uses `https://` scheme** — see Finding F-1 below.
- Lines 143–153: `keyfunc.Override` with `RefreshUnknownKID` rate-limited via `rate.NewLimiter(rate.Every(cfg.CacheTTL/5), 1)` — defends against unknown-kid flood → IdP DDoS. Good.
- Line 155: `keyfunc.NewDefaultOverrideCtx` — background goroutine started via context; cancelled on `Close()`.

**Lines 191–252 — `Verify` (hot path)**
- Line 192–194: empty-token fast path — fails closed.
- Lines 198–207: parser constructed with `jwt.WithValidMethods(allowedAlgs)`, `jwt.WithIssuer`, `jwt.WithAudience`, `jwt.WithLeeway(SkewTolerance)`, `jwt.WithExpirationRequired()`, `jwt.WithTimeFunc(...)`. This is the correct golang-jwt/v5 invocation to get alg allowlist + iss/aud/exp/nbf enforcement with injected clock.
- Lines 210–212: single `ParseWithClaims` call; any error returns `(nil, err)` — fail-closed.
- Line 214–216: defense-in-depth `token.Valid` check.
- Lines 222–225: **defense-in-depth alg re-check after parse** — excellent, guards against hypothetical library bugs (CVE-2022-29217-class issues).
- Lines 227–251: result extraction. Claims are copied out of `jwt.MapClaims`; extraction helpers `extractAudience` and `extractNumericDate` handle both `float64` and `json.Number` encodings — safe.

**Lines 267–302 — Helpers**
- `extractAudience` handles string / []string / []any.
- `extractNumericDate` handles float64, int64, json.Number — but **NOT string** encodings. RFC 7519 says NumericDate is a JSON number, so rejecting strings is correct.
- No panics on any type assertion — all use comma-ok idiom.

### `internal/shared/auth/tokenverify/verifier_test.go` (550 lines)

Tests cover: valid path, wrong issuer, wrong audience, expired, future nbf, bad signature, unknown kid, `alg: none`, `alg: HS256`, malformed (2 parts), non-base64 payload, tampered payload, empty token, skew tolerance (in + out of window), JWKS rotation with forced refresh, 32-worker concurrency, config validation, idempotent Close.

See section 4 for gaps.

---

## 2. Requirements checklist

| # | Requirement | Verdict | Notes |
|---|---|---|---|
| 1 | alg allowlist RS256/384/512 + ES256/384 only; reject `none` + all HS* | **PASS** | `verifier.go:30-36` + `jwt.WithValidMethods` + defense-in-depth re-check at 222-225. Negative tests for `none` and `HS256`. |
| 2 | kid required, JWKS fetch with 5min refresh, cache-poisoning protected | **PASS** | `keyfunc` library enforces kid; `RefreshUnknownKID` rate-limited; `CacheTTL` default 5min. Cache-poisoning protection comes from signature verification against the JWKS-sourced public key — an attacker cannot inject a key into the cache without compromising the IdP. |
| 3 | Standard claims: exp, nbf, iat, iss, aud all checked | **PASS-WITH-NOTE** | exp (required, enforced), nbf (enforced if present), iss (exact), aud (must contain). **iat is NOT validated** beyond being decoded — golang-jwt does not check iat by default and there's no `WithIssuedAtRequired`. iat is informational per RFC 7519 §4.1.6 so this is defensible, but flag it. |
| 4 | Clock skew ≤ 2 minutes max | **PASS** | Hardcoded 60s, non-configurable. Better than the 2-min cap. |
| 5 | Fail-closed on any parse/validation error | **PASS** | Every error path returns `(nil, err)`. `VerifiedToken` is only constructed after all checks succeed. |
| 6 | No panics on malformed input (fuzz-safe) | **PASS-WITH-NOTE** | All type assertions use comma-ok. No `panic()` calls. **No explicit fuzz test exists** — recommend adding `FuzzVerify` for belt-and-braces assurance (follow-up, not blocker). |
| 7 | Key rotation grace: old kid accepted during rotation window | **FAIL** | **See Finding F-2 below.** Current implementation refreshes on unknown-kid, which *replaces* the cached JWKS. If the IdP has removed the old key from its published set, tokens signed with the old key will fail verification immediately after any refresh — no grace window. The doc.go security contract does not promise this behavior, but the review requirements explicitly call for it. |

---

## 3. Attack scenarios — verified

| Attack | Result | Evidence |
|---|---|---|
| **alg:none bypass** | BLOCKED | `jwt.WithValidMethods` + defense-in-depth re-check at `verifier.go:222-225`. Test `TestVerify_Negative/alg: none` at `verifier_test.go:286-289` constructs a hand-built `alg:none` token (since golang-jwt refuses to sign one) and asserts rejection. |
| **alg confusion (RS→HS with pubkey as HMAC key)** | BLOCKED | `jwt.WithValidMethods([]string{"RS256",...})` rejects HS* at the parser level before key material is looked up. Test `TestVerify_Negative/alg: HS256` confirms rejection. |
| **kid injection (path traversal, SQLi in kid)** | BLOCKED | kid is used only as a map-lookup key inside `MicahParks/keyfunc`/`jwkset` — not as a filesystem path, not as a SQL parameter. No sanitization needed because no injection sink exists. **No negative test for exotic kids** (e.g. `../../`, `\x00`, very long strings) — recommend adding. |
| **Expired token accepted due to clock skew misuse** | BLOCKED | `SkewTolerance` is a constant (60s), not configurable. Test `TestVerify_SkewTolerance` verifies both sides: 30s-expired accepted, 120s-expired rejected. |
| **Missing aud check** | BLOCKED | `jwt.WithAudience` enforces. Test `TestVerify_Negative/wrong audience` confirms. Note: `WithAudience` passes if *any* audience in the token matches, per RFC 7519; multi-audience tokens with a matching entry are accepted by design. |
| **JWKS fetched over HTTP instead of HTTPS** | **NOT BLOCKED** — Finding F-1 | `NewTokenVerifier` does not validate the URL scheme. An operator misconfiguring `JWKSURL: http://...` would expose key material to MITM. |
| **JWKS endpoint SSRF** | PARTIALLY MITIGATED | The JWKS URL is set once at verifier construction from config, not derived from untrusted token fields. An attacker controlling the token cannot cause an SSRF. An attacker controlling the config file already has RCE. However, `HTTPClient` defaults to `http.DefaultClient` which follows redirects and has no custom dialer — a compromised IdP could redirect to internal hosts. Low risk; recommend documentation. |
| **Key rotation race (old key removed before all tokens expire)** | **NOT HANDLED** — Finding F-2 | See requirement 7 above. |
| **Empty signature accepted** | BLOCKED | Implicit via `golang-jwt` parser and the `none` test (which produces `header.payload.` with empty signature). **No standalone test** for a `header.payload.` token with an RS256 header claim — recommend adding. |

---

## 4. Test coverage assessment

**Present:**
- Positive: valid RS256, concurrent 32-worker stress.
- Negative: wrong iss, wrong aud, expired, future nbf, bad sig, unknown kid, `alg:none`, `alg:HS256`, malformed 2-part, non-base64, tampered, empty.
- Skew: in-window + out-of-window.
- Rotation: cache miss forces re-fetch and succeeds.
- Lifecycle: Close idempotent, config validation.

**Missing — should be added before or shortly after merge:**
1. **Positive tests for RS384, RS512, ES256, ES384** — only RS256 is exercised. A bug in the allowlist (e.g. a typo) would not be caught.
2. **Negative test for HS384 and HS512** — only HS256 is tested. The library allowlist makes this redundant in theory, but the defense-in-depth re-check at line 222-225 deserves explicit coverage for each rejected alg family.
3. **Negative test for EdDSA / Ed25519** — not in allowlist; needs explicit proof of rejection for FIPS conformance.
4. **JWKS URL scheme validation** — no test because no enforcement (Finding F-1).
5. **Key rotation grace window** — no test because no implementation (Finding F-2).
6. **kid with exotic bytes**: path-traversal strings, null bytes, 4KB payloads, non-UTF8 — to prove no injection sink exists downstream.
7. **Empty signature `header.payload.`** with a valid RS256 header — verify explicit rejection.
8. **Missing `exp` claim** — doc says `WithExpirationRequired()` enforces this; needs a test.
9. **Missing `iss` claim entirely** — should fail closed.
10. **Missing `kid` header entirely** — should fail closed (keyfunc behavior needs explicit verification).
11. **JWKS server returns HTTP 500 / empty body / malformed JSON on first fetch** — verifier should fail closed, not panic.
12. **JWKS server slow/hanging** — no explicit timeout on `HTTPClient` default; `http.DefaultClient` has no timeout. Tokens could block indefinitely. Recommend defaulting to a 10s timeout and adding a test.
13. **`go test -race`** not observed in CI snippet; concurrent test exists but race-detector run should be mandatory.
14. **FuzzVerify** — no fuzz corpus. Recommend for a fuzz-safe contract.

---

## 5. Dependencies

| Module | Version | Maintained? | Known CVEs (as of 2026-04-08) |
|---|---|---|---|
| `github.com/golang-jwt/jwt/v5` | v5.3.1 | Yes, active, upstream of the Go community JWT library. | Historical CVEs (CVE-2020-26160, CVE-2024-51744) resolved in earlier v5 releases. v5.3.1 has no known open CVEs at time of review. v5 is the recommended major; v4 is in maintenance. |
| `github.com/MicahParks/keyfunc/v3` | v3.8.0 | Yes, actively maintained. | No known CVEs. This is the de-facto JWKS client for golang-jwt. |
| `github.com/MicahParks/jwkset` | v0.11.0 | Yes, same maintainer as keyfunc. | No known CVEs. Pre-1.0 so API stability caveat applies. |
| `golang.org/x/time/rate` | (implicit) | Go project. | N/A. |

**Verdict:** dependency hygiene is good. All three JWT/JWKS libraries are maintained by responsible upstreams and pinned to current versions. Recommend wiring `dependabot`/`govulncheck` into CI for this package specifically.

---

## 6. FIPS 140-3 note (KAI-388)

**Ed25519 / EdDSA: NOT USED.** The allowlist at `verifier.go:30-36` is `RS256, RS384, RS512, ES256, ES384` only. This aligns with the FIPS 140-3 roadmap — NIST SP 800-186 approves P-256/P-384 curves and RSA-2048+, but Ed25519 acceptance in FIPS modules is still transitional.

No action required for KAI-388 from this PR. When FIPS mode lands we will need to add an additional minimum-key-size enforcement (reject RSA < 2048) — noted for KAI-388 follow-up, not a blocker here.

---

## 7. Blocking findings

### F-1 — JWKS URL scheme not enforced to HTTPS (BLOCKER)
**File:** `internal/shared/auth/tokenverify/verifier.go:121-130`
**Severity:** HIGH
**Details:** `NewTokenVerifier` accepts any URL for `JWKSURL`, including `http://`. If an operator misconfigures the IdP URL — or if config is sourced from a partially-trusted pipeline — the verifier would fetch signing keys over plaintext HTTP, allowing a network attacker to substitute attacker-controlled public keys and forge tokens accepted by every Kaivue service.
**Fix (for PR author, not for reviewer):** parse the URL in `NewTokenVerifier` and return an error unless `scheme == "https"` (allow `http://` only for explicit test builds behind a build tag or an explicit `VerifierConfig.AllowInsecureJWKS` flag that is never set in production). Add a negative test.

### F-2 — No key-rotation grace window (BLOCKER vs. stated requirement)
**File:** `internal/shared/auth/tokenverify/verifier.go:143-153`, doc.go:52-64
**Severity:** MEDIUM-HIGH
**Details:** The `keyfunc` library's default behavior on refresh is to **replace** the cached JWKS with the server response. Zitadel rotation practice publishes the new key before the old one is removed, but there is a window where old-signed tokens must still verify. The current implementation has no explicit grace: the moment the IdP stops publishing the old key, old-kid tokens fail verification, regardless of the token's `exp`.

Requirement #7 explicitly calls out "old kid accepted during rotation window." The doc.go security contract does not promise this — which is actually the concerning part: the contract and the requirements diverge.
**Fix options (for PR author):**
  (a) Accept that `keyfunc` replaces the cache and document explicitly that rotation grace relies on the IdP keeping the old key in its JWKS for ≥ max-token-lifetime. This is what Zitadel does, and may be acceptable — but needs a written operational requirement.
  (b) Maintain a local sidecar map of `kid → (publickey, lastSeenAt)` and retain previously-seen keys for `2 × max-token-lifetime` after they disappear from the upstream JWKS. More work; stronger guarantee.
**Recommendation:** option (a) + a test that documents and enforces the expected IdP behavior. Add a CLARIFY to the doc-block so downstream reviewers don't re-ask this question.

### F-3 — No HTTP client timeout (BLOCKER)
**File:** `internal/shared/auth/tokenverify/verifier.go:134-135`
**Severity:** MEDIUM
**Details:** `cfg.HTTPClient` defaults to `http.DefaultClient`, which has **no timeout**. A slow or hanging JWKS endpoint can stall the background refresh goroutine indefinitely, and a slow synchronous refresh (on unknown-kid) can stall the Verify call. This is a liveness risk and a DoS amplifier.
**Fix:** default to `&http.Client{Timeout: 10 * time.Second}` when nil.

---

## 8. Non-blocking follow-ups

- **FUP-1** Add fuzz test `FuzzVerify` seeded with the negative-test corpus; wire into CI fuzz budget.
- **FUP-2** Add positive tests for RS384, RS512, ES256, ES384; negative tests for HS384, HS512, EdDSA.
- **FUP-3** Add exotic-kid tests (path-traversal, null byte, 4KB, non-UTF8) to document the no-injection-sink guarantee.
- **FUP-4** Add tests for missing `exp`, missing `iss`, missing `kid`, missing `aud`.
- **FUP-5** Add test for JWKS server returning 500 / empty / malformed / slow — verifier must fail closed.
- **FUP-6** Wire `govulncheck` into CI for this package.
- **FUP-7** Clarify doc.go's rotation section to explicitly state the operational requirement on the IdP (see F-2 option a).
- **FUP-8** For KAI-388 FIPS roadmap: add RSA key-size minimum enforcement (reject < 2048). Not needed today.
- **FUP-9** Ensure `go test -race ./internal/shared/auth/tokenverify/...` runs in CI for every PR touching this package.

---

## 9. Verdict

**BLOCK — do not merge until F-1, F-2, F-3 are resolved.**

The cryptographic core is correct: the algorithm allowlist is tight, the defense-in-depth re-check is present, fail-closed semantics are honored, no panics on malformed input, clock skew is within bounds, and the dependency set is clean. The author clearly understood the JWT attack surface and wrote a disciplined implementation. The test suite is solid for the happy path and the headline negative cases.

However, three operational-security issues must be addressed before this primitive handles production tokens:
1. HTTPS is not enforced on the JWKS URL (F-1)
2. The key-rotation grace behavior diverges from the stated requirement and is undocumented (F-2)
3. The HTTP client has no timeout, allowing a hung IdP to stall verifies (F-3)

Each of these is a one-function, one-test fix. Once addressed, this PR should be approved with the non-blocking follow-ups tracked separately.

**Merge freeze:** active — this review is advisory only and no merge action is being taken regardless.
