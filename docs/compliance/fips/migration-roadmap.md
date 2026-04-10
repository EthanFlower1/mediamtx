# FIPS 140-3 Migration Roadmap — Kaivue Recording Server

**Ticket:** KAI-388 — FIPS 140-3 migration plan
**Owner:** lead-security
**Status:** Draft for CTO review — 2026-04-08
**Companion doc:** `docs/compliance/fips/inventory.md` (crypto inventory, 166 import lines analyzed)
**Target:** FedRAMP Moderate / DoD IL4 eligibility for Q4-2026

---

## 0. TL;DR for the CTO

Kaivue is **~90% already FIPS-clean** thanks to the cryptostore rescue (KAI-251) and RS256/HMAC-SHA256 JWT adoption. Ten primitive sites block a FIPS-140-3 claim; the two that require engineering judgment (not just mechanical swaps) are:

1. **Ed25519 → ECDSA P-256 across the pairing and StepCA root CA.** This is a **breaking wire/cert format change** that rotates our directory↔recorder trust anchor. Every previously paired Recorder must re-enroll. Cost scales with installed fleet.
2. **ONVIF legacy auth (MD5 Digest + SHA-1 WS-UsernameToken).** Under a FIPS build these primitives must be unlinkable. That excludes essentially every pre-2018 camera in the market. This is a **product-scope decision**, not a technical one.

**Recommended path:** dual-binary (`kaivue-recorder` + `kaivue-recorder-fips`) built from one tree with a `fips` build tag; Go 1.24 `crypto/fips140` module as the validated provider; 7 engineer-weeks across four sub-tickets (KAI-388a–d). No net-new dependencies.

**Top blocking decision the CTO owns:** *Do we execute the Ed25519 → ECDSA P-256 cutover now (forcing every paired Recorder to re-enroll once) or stall until Go's FIPS module exposes Ed25519 (tracked in golang/go#71102, no ETA)?*  Everything else in this roadmap can be serialized around that call.

---

## 1. Executive summary

### 1.1 Why FIPS 140-3 matters

Kaivue's 2026 pipeline includes three revenue motions that cannot close without a FIPS-140-3 validated cryptographic module:

| Buyer segment | Requirement | Source of truth |
|---|---|---|
| **US Federal civilian (FedRAMP Moderate)** | All "cryptography used to protect Federal information" must come from a FIPS-140 validated module, or the CSP must submit a deviation request with a compensating control. | FedRAMP CSP Cryptographic Module Policy (2023 revision), SP 800-53 Rev. 5 SC-13. |
| **DoD (IL4 / IL5)** | FIPS 140-3 is mandatory for data-at-rest and data-in-transit; SP 800-171 §3.13.11 references FIPS 140. Without it we cannot list on DISA CC-SRG, which blocks anything touching CUI. | DoD CC-SRG v1r4, §5.10. |
| **Healthcare with federal affiliations (VA, IHS, DHA)** | HIPAA itself is algorithm-agnostic but federal healthcare affiliates inherit SC-13 from their parent agency's ATO. Several enterprise prospects (Ascension Federal, VA pilot) already asked for the FIPS boundary in our SOC 2 questionnaire. | Customer RFPs (Q1-2026). |

The commercial cost of *not* doing this: ~$4.5M in identified 2026 ARR gated on a FIPS statement. The cost of doing it: **7 engineer-weeks** and a modest hit to ONVIF camera compatibility for federal deployments.

### 1.2 Scope and non-scope

**In scope:** the Go monolith (`mediamtx` fork), every import of `crypto/*`, `golang.org/x/crypto/*`, `math/rand`, and third-party crypto under `internal/` and `pkg/`. Signing algorithms used by auth, streamclaims, crosstenant, stepca, nvr-api. TLS configuration. RNG for tokens, nonces, IDs.

**Out of scope:** Flutter client crypto (separate track, inherits OS CryptoKit / Conscrypt); React admin UI (browser WebCrypto — each federal tenant runs on a FIPS-capable browser); sidecars (Headscale/Tailscale, step-ca, Zitadel — supply-chain compliance items tracked under KAI-419/KAI-425); SQLite at-rest (already FIPS-track via cryptostore, KAI-251).

### 1.3 Cost & timeline headline

- **7 engineer-weeks** total implementation + CI work.
- **~5 calendar weeks** with two engineers working KAI-388a/b in parallel with KAI-388c.
- **Plus ~1.5 engineer-weeks** for the lead-security FedRAMP SSP crypto section and 3PAO prep (can run in parallel, not on the critical path).
- **Target internal cutover:** 2026-06-15.
- **Target first external FIPS binary drop:** 2026-07-01.
- **Target FedRAMP "In Process" listing:** 2026-09-01 (gated on 3PAO kickoff, not on this roadmap alone).

---

## 2. Target standard

### 2.1 FIPS 140-3 Level 1 (software)

We target **FIPS 140-3 Level 1**, the software-module level. Rationale:

- Level 1 is sufficient for FedRAMP Moderate, DoD IL4, and every healthcare federal affiliate we have in pipeline.
- Level 2 requires tamper-evident packaging and role-based authentication at the module boundary — appropriate for HSMs, not general-purpose servers.
- Level 3/4 require hardware tamper response, which makes no sense for a containerized Go binary.
- **Non-goal:** any claim above Level 1 in the 2026 timeframe. Customers who need HSM-backed keys will front Kaivue with an AWS CloudHSM or Azure Managed HSM; that integration is tracked separately (KAI-441, not blocking this roadmap).

### 2.2 Reliance on Go 1.24 `crypto/fips140`

Go 1.24 ships a `crypto/fips140` module — a carve-out of the standard library's crypto primitives packaged as a separately-versioned module submitted to the CMVP ("Cryptographic Module Validation Program"). When the binary is built and run with `GOFIPS140=v1.0.0` and executed with `GODEBUG=fips140=only`, every approved call routes through that module and every non-approved call panics.

As of 2026-04 it is listed in the CMVP "Implementation Under Test" (IUT) queue. We treat IUT as acceptable because:

- FedRAMP policy allows IUT modules with a PMO-approved waiver ("Modified 17020" guidance, 2023).
- Major vendors (Red Hat, Canonical, Docker) routinely ship on IUT modules.
- The only risk is that a bug found during validation forces a point-release; we mitigate by pinning the module version in our `go.mod` and CI, and by gating releases on the CI "fips build" lane staying green.

**If Go's FIPS validation slips** (see §8 Risk Register for the full play): our Plan B is `GOEXPERIMENT=boringcrypto`, which has been production-shipped since 2017 and backed by Google's own FedRAMP-authorized deployments. The migration cost from `crypto/fips140` to `boringcrypto` is roughly *one CI flag*, because both use the same stdlib API surface. Plan C is an external validated provider (OpenSSL 3.0 FIPS via CGO), which would require a 4–6 week rewrite of TLS bootstrap and is a last resort.

### 2.3 Approved primitives we already use

The inventory confirms the following are already on both the `crypto/fips140` and `boringcrypto` allowlists and need no migration:

- AES-256-GCM (cryptostore, nvr/crypto/encrypt, backup)
- HKDF-SHA256 (once we swap `golang.org/x/crypto/hkdf` → stdlib `crypto/hkdf`)
- HMAC-SHA256 (webhook dispatcher, HS256 JWTs)
- SHA-256, SHA-384, SHA-512
- RSA-2048/3072 (key generation, RS256 JWTs, TLS)
- ECDSA over P-256 / P-384 / P-521
- TLS 1.2 and TLS 1.3 (stdlib-provided suites only)
- `crypto/rand` (the Go stdlib RNG, which is DRBG-validated under both modules)

This list covers >90% of current crypto call sites. The remaining <10% is what Phases 1–4 retire.

### 2.4 Prohibited in FIPS mode

Every primitive below must be unreachable from the `fips` build or return `fips.ErrUnsupported`:

- Ed25519 / EdDSA (FIPS 186-5 approves it; Go's FIPS module does not yet expose it)
- X25519 (FIPS 186-5 does not approve it; P-256 ECDH is the substitute)
- ChaCha20 / ChaCha20-Poly1305 / Poly1305
- XSalsa20 / NaCl secretbox
- Argon2, bcrypt, scrypt (SP 800-132 requires PBKDF2)
- MD5 (any use)
- SHA-1 for digital signatures, password digests, or WS-Security constructions (permitted as a *legacy-use* MAC under SP 800-131A — we document this exception for ICE-STUN only)
- BLAKE2, RIPEMD
- RC4, DES, 3DES
- JWT algorithms `EdDSA`, `none`
- ECDSA over P-224 (not approved for new use)
- TLS < 1.2

---

## 3. Build strategy

### 3.1 `fips` build tag

We introduce a single Go build tag, `fips`, as the compile-time switch.

- Every file containing a non-FIPS primitive gets a `//go:build !fips` constraint.
- A paired `//go:build fips` stub file lives beside it and either (a) routes to an approved replacement or (b) returns a typed `fips.ErrUnsupported` from `internal/shared/fipsmode`.
- No runtime allocation cost: the decision is linker-resolved.
- The `fips` tag composes cleanly with our existing tags (`cgo`, `e2e`, platform tags).

### 3.2 Dual-binary release

We ship **two** artifacts per platform from the same source tree:

| Artifact | Build command | Target customers |
|---|---|---|
| `kaivue-recorder` | `go build ./cmd/kaivue-recorder` | All current customers. ChaCha20, Ed25519, WebRTC, legacy ONVIF all available. |
| `kaivue-recorder-fips` | `GOFIPS140=v1.0.0 go build -tags fips ./cmd/kaivue-recorder` | Federal, DoD, federal healthcare affiliates. Hardened attack surface; reduced ONVIF compat; no WebRTC playback. |

The two binaries are byte-different but functionally identical on the happy path. Container images are published as `ghcr.io/kaivue/recorder:<v>` and `ghcr.io/kaivue/recorder:<v>-fips`.

**Rejected alternative:** making `fips` the default for all customers. That would disable WebRTC (per §2.4) for non-federal users and halve our ONVIF compat matrix — unacceptable. Rejected.

**Rejected alternative:** a runtime flag that toggles the crypto provider. The Go stdlib does not support runtime toggling of `crypto/fips140`; `GOFIPS140` is a program startup decision. And a runtime flag would let a misconfigured operator *think* they were in FIPS mode when they were not. The build-tag approach is unambiguous.

### 3.3 CI matrix

The existing CI "build" lane already compiles on `linux/amd64`, `linux/arm64`, `darwin/arm64`, `windows/amd64`. We add a parallel **`build-fips`** lane that compiles every one of those platforms with `-tags fips GOFIPS140=v1.0.0` and runs the full test suite under `GODEBUG=fips140=only`.

- New: `build-fips` lane, per-platform, required before merge.
- New: `fipslint` analyzer lane (see §5), runs on every PR, fails fast if a disallowed import lands under the `fips` tag.
- New: `release-fips` lane, runs on tag push, produces the `kaivue-recorder-fips` artifact, SBOM, and cosign signatures (reuses the KAI-428 reproducible build pipeline).

Expected CI cost: ~30% more compute on the build step, ~0% on the test step (we reuse the same unit tests with the `fips` tag). Acceptable.

### 3.4 Runtime FIPS detection and logging

At process start, every binary logs its crypto posture exactly once:

```
INFO crypto posture: module=fips140 version=v1.0.0 mode=strict fips=true
INFO crypto posture: module=stdlib version=go1.24 mode=permissive fips=false
```

The determination uses `runtime/debug.ReadBuildInfo()` to detect the `fips` build tag and `GODEBUG` to detect strict mode. A single helper lives in `internal/shared/fipsmode/fipsmode.go` and exposes `fipsmode.Enabled() bool` for the rest of the codebase.

`fipsmode.Enabled()` is consumed by:

- The structured startup banner (above).
- `/healthz` JSON response (see §7).
- A new config validator that rejects config keys referencing non-approved primitives (e.g. `auth.jwtAlgorithm: EdDSA`) when FIPS is on.
- The fipslint analyzer's test harness.

---

## 4. Migration plan — four sub-tickets

Each sub-ticket is independently mergeable. KAI-388a and KAI-388b can run in parallel from day one; KAI-388c (Ed25519 migration) runs serially after KAI-388a because it touches the same files. KAI-388d (CI + linter + release) is the final cap.

### 4.1 KAI-388a — Ed25519 → ECDSA P-256 (2 weeks, critical path)

**Scope:** Retire every Ed25519 signing primitive across the Directory, StepCA, Pairing, and Recorder subsystems. Replace with ECDSA P-256 (ES256 for JWTs).

**Files touched:**

- `/Users/ethanflower/personal_projects/mediamtx/internal/directory/pairing/token.go`
- `/Users/ethanflower/personal_projects/mediamtx/internal/directory/pairing/service.go`
- `/Users/ethanflower/personal_projects/mediamtx/internal/directory/pairing/token_test.go`
- `/Users/ethanflower/personal_projects/mediamtx/internal/directory/pki/stepca/clusterca.go` (root CA!)
- `/Users/ethanflower/personal_projects/mediamtx/internal/directory/pki/stepca/clusterca_test.go`
- `/Users/ethanflower/personal_projects/mediamtx/internal/directory/pki/stepca/enroll_token.go` (uses `jwt.SigningMethodEdDSA`)
- `/Users/ethanflower/personal_projects/mediamtx/internal/recorder/pairing/keystore.go`
- `/Users/ethanflower/personal_projects/mediamtx/internal/recorder/pairing/join.go`
- `/Users/ethanflower/personal_projects/mediamtx/internal/recorder/pairing/join_test.go`

**Breaking change:** The StepCA root CA key type changes from Ed25519 to ECDSA P-256. Existing intermediates and issued recorder certs are signed by the Ed25519 root and will be untrusted by the new binary. Every previously paired Recorder must re-enroll.

**Version negotiation for pairing protocol:**

- Introduce `pairing_protocol_version` in the pairing handshake (currently implicit v1). v1 = Ed25519, v2 = ECDSA P-256.
- The Directory advertises both during the transition window; the Recorder picks v2 if it supports it.
- After the transition window (proposed 90 days), v1 support is removed from the Directory code path.
- The Recorder binary refuses to downgrade to v1 once it has v2 keys on disk.

**Cert rotation plan for the StepCA root:**

1. T+0: Release a Directory binary that supports *both* roots — old Ed25519 root remains trusted for inbound connections, new ECDSA P-256 root is the active issuer for new certs.
2. T+0: Every Recorder check-in triggers a silent re-enrollment under the new root. Operators see no UI.
3. T+30 days: Dashboard alert for any Recorder still on the legacy root.
4. T+60 days: Ops outreach for stragglers (typically <5% of a fleet).
5. T+90 days: Legacy root removed. Any Recorder still presenting an Ed25519 cert is rejected and must be factory-reset and re-paired.

**Dependencies:** Coordinate with lead-directory on the pairing check-in server-side verify (KAI-430), and with lead-backend on proto/Connect-Go signature formats (KAI-399). The ECDSA P-256 signature encoding is ASN.1 DER by default; we pin to DER for wire compat.

**Testing:** New contract tests that exercise both v1 and v2 pairing; existing Ed25519 test vectors preserved under a `//go:build !fips` test file so we can still regress-test the legacy path until it is removed.

**Effort:** 2 engineer-weeks including migration, rotation tooling, and tests.

### 4.2 KAI-388b — argon2 → PBKDF2-HMAC-SHA256 (3 days)

**Scope:** Retire `github.com/matthewhartstonge/argon2` from the credential hashing path. Replace with `crypto/pbkdf2.Key(password, salt, 600000, 32, sha256.New)` (SP 800-132 compliant, OWASP 2023 iteration guidance).

**Files touched:**

- `/Users/ethanflower/personal_projects/mediamtx/internal/conf/credential.go` (imports `matthewhartstonge/argon2` at line 11)
- `/Users/ethanflower/personal_projects/mediamtx/internal/nvr/api/auth.go`
- `/Users/ethanflower/personal_projects/mediamtx/internal/conf/credential_test.go` (new test vectors)

**DB migration — dual-format support during transition:**

Hash records acquire a `scheme` prefix: `argon2id$...` or `pbkdf2-sha256$600000$...`.

On password *verification*:

- If the stored hash begins with `argon2id$`, use the argon2 verifier (legacy path, `//go:build !fips` so it is absent from FIPS builds).
- If the stored hash begins with `pbkdf2-sha256$`, use PBKDF2.
- **FIPS builds** refuse to verify argon2id and return `fips.ErrUnsupported` — the operator must reset the password on first login. This is documented in the upgrade runbook.

On *successful login* with an argon2 hash (non-FIPS build):

- Re-hash the submitted plaintext with PBKDF2 and update the stored record.
- This gives us a silent, login-driven migration with zero ops involvement.

Over ~30 days the argon2 records naturally drain to zero for active users. Inactive users who have not logged in in 30 days are force-reset by the SRE runbook.

**Effort:** 3 engineer-days including dual-format decoder, migration, tests.

### 4.3 KAI-388c — ONVIF FIPS exclusion (1 week)

**Scope:** Under the `fips` build tag, disable MD5 Digest auth and SHA-1 WS-UsernameToken in the ONVIF client. Add a camera exclusion list. Ship a customer comms plan.

**Files touched:**

- `/Users/ethanflower/personal_projects/mediamtx/internal/nvr/onvif/snapshot.go` (line 233: `cnonce` uses `math/rand`, must use `crypto/rand`; MD5 Digest auth is retired under `fips`)
- `/Users/ethanflower/personal_projects/mediamtx/internal/nvr/onvif/events.go` (SHA-1 WS-UsernameToken PasswordDigest)
- `internal/nvr/onvif/snapshot_fips.go` (new, stub)
- `internal/nvr/onvif/events_fips.go` (new, stub)
- `internal/conf/conf.go` (new config key `onvif.allowLegacyAuth`)

**Under `fips` build:**

- MD5 Digest auth path is a stub returning `fips.ErrUnsupported` with a clear operator error: *"camera <name> requires MD5 Digest auth, which is disabled in FIPS mode"*.
- SHA-1 WS-UsernameToken path: same treatment.
- `onvif.allowLegacyAuth` defaults to `false` and is non-overridable under `fips` (config validator rejects `true`).

**Camera exclusion matrix:**

Maintained in `docs/compliance/fips/onvif-camera-matrix.md` (follow-up doc). First pass: any camera manufactured pre-2018 typically supports only Digest-MD5; anything pre-2020 may or may not support WS-Security UsernameToken with SHA-256. Known-compatible cameras: Axis P-series ≥ 2019, Hanwha Q-series ≥ 2020, Bosch Flexidome ≥ 2020, Hikvision DeepInView ≥ 2022. This matrix is a deliverable of KAI-388c, maintained by the camera-integrations team.

**Customer comms plan:**

- Federal customers receive a pre-purchase camera compatibility checklist as part of the deployment guide.
- The fipslint-enabled build refuses to start with a legacy camera in its config, logging the exact offending camera path.
- We do *not* ship a bridge sidecar that terminates MD5 Digest outside the FIPS boundary — that would defeat the purpose. Customers with legacy cameras either replace the camera or deploy the non-FIPS binary.

**Also in this ticket:** fix the `snapshot.go:233` cnonce to use `crypto/rand` regardless of build tag (this is a straightforward bug fix the inventory flagged; it will be flagged by `gosec` G404 once we enable it in CI).

**Effort:** 1 engineer-week including the comms plan and runbook update.

### 4.4 KAI-388d — nacl/secretbox → cryptostore AES-256-GCM (3 days)

**Scope:** Retire `golang.org/x/crypto/nacl/secretbox` from the encrypted config loader. Replace with the existing `internal/shared/cryptostore` AES-256-GCM primitive.

**Files touched:**

- `/Users/ethanflower/personal_projects/mediamtx/internal/conf/decrypt/decrypt.go` (imports `nacl/secretbox` at line 8)
- `/Users/ethanflower/personal_projects/mediamtx/internal/conf/conf_test.go` (test fixture)

**Migration:**

- New config envelopes written on or after this release use AES-256-GCM with a 96-bit random nonce, authenticated by the same HKDF-SHA256(master_secret, "conf-encrypt-v2") subkey pattern already used by cryptostore.
- The decrypt path tries the new envelope first; on parse failure it falls back to the legacy `secretbox` path for exactly **one release cycle** (v1.9 → v1.10).
- `v1.10` removes the fallback. Any operator still running an encrypted config from v1.8 or older must decrypt-then-reencrypt during the v1.9 upgrade. Upgrade runbook documents the command.
- Under the `fips` build, the secretbox fallback path is unreachable: the file is `//go:build !fips` and the fips stub returns `fips.ErrUnsupported` on fallback.

**Effort:** 3 engineer-days.

### 4.5 KAI-388e — CI, linter, release (1 week) [optional split]

If the team wants a cleaner sub-ticket boundary, the CI + linter + release pipeline work carves out cleanly as KAI-388e. For this roadmap we fold it into the overall 7-week estimate under the `build-fips` lane and §5 linter work.

---

## 5. Linter implementation sketch

**Location:** `tools/fipslint/` (new package, ~100 LOC).
**Form:** A single `golang.org/x/tools/go/analysis.Analyzer` named `fipslint.Analyzer`, invoked via `go vet -vettool=$(which fipslint)`. CI-gated.

**Behavior:** The analyzer runs on every package in the tree. When the package's build constraints include `fips` (or when the analyzer is invoked with `-tags fips`), it walks the AST and reports:

1. **Forbidden imports** (full path match):
   - `crypto/ed25519`, `golang.org/x/crypto/ed25519`
   - `golang.org/x/crypto/chacha20`, `chacha20poly1305`, `curve25519`, `poly1305`
   - `golang.org/x/crypto/nacl/box`, `nacl/secretbox`, `nacl/sign`, `nacl/auth`
   - `golang.org/x/crypto/bcrypt`, `scrypt`, `blake2b`, `blake2s`
   - `github.com/matthewhartstonge/argon2`
   - `crypto/md5`, `crypto/sha1` (allowlist: exactly one file — `internal/servers/webrtc/server.go` — gated by `//fipslint:allow rfc5389-hmac-sha1`)
   - `crypto/dsa`, `crypto/rc4`, `crypto/des`
   - `math/rand`, `math/rand/v2` (allowlist on each site via `//fipslint:allow jitter` inline comment)

2. **Forbidden string literals** inside `jwt.NewWithClaims(...)` call sites (AST match on the first argument): `"EdDSA"`, `"none"`. We walk `*ast.CallExpr` nodes whose `Fun` resolves to `github.com/golang-jwt/jwt/v5.NewWithClaims` and inspect the signing method selector.

3. **Forbidden elliptic curves**: `elliptic.P224()` call expressions.

4. **TLS downgrade**: any `tls.Config{MinVersion: ...}` composite literal whose `MinVersion` is a constant `< tls.VersionTLS12`. Detected via constant folding in the analyzer pass.

5. **Forbidden hash constants in signing contexts**: `crypto.MD5`, `crypto.SHA1` used as `crypto.Hash` arguments to `rsa.SignPKCS1v15`, `rsa.SignPSS`, `ecdsa.Sign`, `x509.CreateCertificate`, and the `hash.Hash` argument of `hmac.New` *except* within allowlisted files.

**Allowlist format:** an inline comment `//fipslint:allow <reason>` on the offending line is respected and surfaces in the PR diff for human review. We audit the allowlist quarterly.

**Implementation notes:**

- Use `golang.org/x/tools/go/ast/inspector.Inspector` over `*ast.ImportSpec` and `*ast.CallExpr` to stay fast.
- Detect the build tag by reading `//go:build` lines from `File.Doc` *or* by passing `-tags fips` at invocation time; we do both, and the analyzer warns if they disagree.
- Emit diagnostics via `pass.Reportf(importSpec.Pos(), "fipslint: %s is not FIPS 140-3 approved", path)`.
- Reference implementation: golangci-lint's `gosec` rules G401/G501 — we mirror the AST-walking pattern but restrict the rule set to the FIPS bubble.
- Unit tested with golden-file fixtures under `tools/fipslint/testdata/`.

**CI integration:** `fipslint` runs in its own GitHub Actions job named `fipslint` and is a required status check on every PR. Build time: <15 seconds.

---

## 6. Test strategy

### 6.1 Unit tests

Every file touched by KAI-388a–d gets unit tests covering both the FIPS and non-FIPS code paths. The non-FIPS paths are tested under the default build; the FIPS paths are tested under the `fips` build tag in the `build-fips` CI lane.

### 6.2 Integration tests

New package `internal/nvr/integration/fips_test.go` carrying the end-to-end FIPS mode matrix:

| Test | Asserts |
|---|---|
| `TestFipsStartupBanner` | Binary logs `fips=true` on startup with `-tags fips`. |
| `TestFipsHealthzFlag` | `/healthz` returns `fips_mode: true`. |
| `TestFipsPairingES256` | Pairing handshake completes successfully and selects ECDSA P-256 on both sides. |
| `TestFipsPairingRejectsEd25519` | A Recorder presenting an Ed25519 cert is rejected with a typed error. |
| `TestFipsPasswordPBKDF2` | A new user's password is hashed with PBKDF2-SHA256 N=600000. |
| `TestFipsPasswordRejectsArgon2Verify` | Verifying a password against an argon2id hash returns `fips.ErrUnsupported`. |
| `TestFipsOnvifMD5Rejected` | Configuring a camera with MD5 Digest returns `fips.ErrUnsupported` at config-load time. |
| `TestFipsOnvifSHA1UsernameTokenRejected` | Same for SHA-1 WS-UsernameToken. |
| `TestFipsConfigDecryptAES_GCM` | Encrypted config round-trip uses AES-256-GCM. |
| `TestFipsConfigDecryptRejectsSecretbox` | A legacy secretbox config fails to load under FIPS with a clear error. |
| `TestFipsWebRTCDisabled` | WebRTC routes return 501 Not Implemented. |
| `TestFipsJWTRejectsEdDSA` | A JWT signed with EdDSA is rejected at verification time, regardless of the public key type. |

### 6.3 Negative tests (critical)

A dedicated `TestFipsForbiddenPrimitivesPanic` test compiles a small helper program under `-tags fips` and asserts that direct calls to Ed25519, ChaCha20, and argon2 either fail to link or panic at runtime. This is a canary: if any of these start passing, the fips gate has a leak and the build must fail.

### 6.4 Compliance artifact regeneration

Every release of the fips binary regenerates three artifacts and uploads them as release assets:

- **SBOM** (CycloneDX 1.5, via `syft`) — already produced by KAI-428.
- **Crypto attestation**: a plain-text report naming every crypto primitive reachable from the fips binary, generated by running `fipslint -tags fips -report ./...`. This is the evidence the 3PAO will ask for.
- **`go version -m` output**: captures the `GOFIPS140=v1.0.0` build flag, proving the binary links the validated module.

---

## 7. Customer-facing surfaces

### 7.1 Config

New top-level config key:

```yaml
# mediamtx.yml (NVR)
compliance:
  fipsMode: auto   # auto | strict | off
```

- `auto` (default): mirrors the build tag. If the binary is `kaivue-recorder-fips`, behaves as `strict`; otherwise `off`.
- `strict`: fails startup if the binary is not a FIPS build. Prevents operators from thinking they are in FIPS mode when they are not.
- `off`: explicitly opts out. Only meaningful on a non-FIPS binary; a no-op on the fips binary.

Note: per CLAUDE.md we do not modify runtime `mediamtx.yml` defaults. The new key defaults to `auto` and is additive.

### 7.2 Startup banner

Structured log line on every boot (already described in §3.4). Emitted at `INFO` level so it survives `logLevel: debug` downgrades too.

### 7.3 `/healthz` extension

`/healthz` JSON response gains a `compliance` block:

```json
{
  "status": "ok",
  "version": "v1.9.0",
  "compliance": {
    "fips_mode": true,
    "fips_module": "crypto/fips140",
    "fips_module_version": "v1.0.0",
    "fips_module_status": "IUT"
  }
}
```

This is what federal customers scrape into their continuous monitoring dashboards. The field is present on both binaries so customers can alert on unexpected flips.

### 7.4 Compliance report endpoint

New authenticated endpoint `/api/v1/compliance/crypto-report` returns the same crypto attestation artifact produced at build time (§6.4). Federal customers use this for quarterly audit evidence. Gated by the `compliance:read` role.

### 7.5 Admin UI

The React admin console gains a **Compliance** card on the dashboard showing `FIPS Mode: Enabled / Disabled` with a link to the crypto report. Scope: UI-only, no backend changes beyond §7.3 — tracked as a follow-up UI ticket.

---

## 8. Risk register

| # | Risk | Likelihood | Impact | Mitigation | Owner |
|---|---|---|---|---|---|
| R1 | Go `crypto/fips140` CMVP validation slips past 2026-Q3. | Medium | High — blocks any "validated" claim. | Ship on IUT with FedRAMP waiver (well-established precedent). Keep CI green on `GOEXPERIMENT=boringcrypto` as Plan B — one flag swap. | lead-security |
| R2 | Ed25519 cutover breaks a customer's paired Recorder fleet mid-transition. | Medium | High | Dual-root grace window (90 days); dashboard alerts for stragglers; factory-reset runbook. | lead-directory |
| R3 | Federal customer has a pre-2018 ONVIF camera fleet and cannot use the FIPS binary. | High | Medium — customer friction, not a platform failure. | Ship camera compat matrix pre-sales; offer non-FIPS binary on same host for internal-only paths; pursue ONVIF vendor upgrades. | product, lead-security |
| R4 | Go `crypto/fips140` exposes Ed25519 *after* we cut over to ECDSA. Wasted work. | Medium | Low — ECDSA P-256 is a perfectly good algorithm; no real cost beyond the one-time migration. | Accept. Do not stall on a hypothetical future. | CTO |
| R5 | `pion/webrtc` ecosystem never becomes FIPS-capable. | High | Low — WebRTC playback is gated off in FIPS builds; HLS is the fallback. | Document HLS-over-TLS fallback (latency ~4s vs ~200ms). Revisit libwebrtc + BoringSSL in 2026-Q4 if a customer demands low-latency FIPS playback. | lead-media |
| R6 | `fipslint` analyzer has a false negative and a forbidden primitive ships in a release. | Low | High | Negative tests (§6.3) run in CI; `go vet -vettool=fipslint` is a required check; quarterly manual audit of the allowlist. | lead-security |
| R7 | `GOFIPS140` flag is forgotten on a release build, producing a binary that *looks* like a fips build but is not. | Medium | High | The `release-fips` CI lane hard-codes the flag; release tagging automation enforces the flag on tag push; `/healthz` surfaces the module version so customers can detect a drift. | devops |
| R8 | argon2 → PBKDF2 migration bricks users who cannot log in to trigger re-hash. | Low | Medium | SRE runbook for admin password reset; 30-day grace; pre-upgrade broadcast email. | lead-backend |
| R9 | External FIPS provider (OpenSSL 3.0 FIPS via CGO) becomes the only path. | Low | Very High — 4–6 week replatforming. | Monitor Go FIPS module validation status weekly. Trigger condition: if Go module falls off CMVP pipeline entirely. | lead-security, CTO |

---

## 9. Timeline

### 9.1 Effort by phase

| Sub-ticket | Scope | Engineer-weeks |
|---|---|---|
| KAI-388a | Ed25519 → ECDSA P-256 migration (pairing + StepCA + enroll_token) | **2.0** |
| KAI-388b | argon2 → PBKDF2-HMAC-SHA256 with dual-format support | **0.6** |
| KAI-388c | ONVIF FIPS exclusion + cnonce fix + camera matrix + comms plan | **1.0** |
| KAI-388d | nacl/secretbox → cryptostore AES-256-GCM | **0.6** |
| KAI-388e (CI/lint/release) | `build-fips` lane, fipslint analyzer, release pipeline, crypto attestation | **1.3** |
| KAI-388f (security/SSP) | FedRAMP SSP crypto section, 3PAO prep, runbook, upgrade docs | **1.5** |
| **Total** | | **~7.0 engineer-weeks** |

### 9.2 Serialized vs parallel lanes

**Critical path (serialized):** KAI-388a (Ed25519) → KAI-388e (CI/lint) → KAI-388f (SSP sign-off). 4.8 engineer-weeks on the critical path.

**Parallel lane A:** KAI-388b + KAI-388d (both are small, self-contained, and touch files untouched by 388a). Can run alongside KAI-388a from day one. 1.2 engineer-weeks.

**Parallel lane B:** KAI-388c (ONVIF). Also independent. 1.0 engineer-weeks.

**Calendar schedule with two engineers:**

- Week 1–2: Engineer 1 on KAI-388a. Engineer 2 on KAI-388b + KAI-388c + KAI-388d.
- Week 3: Engineer 1 finishes 388a tests and rotation tooling. Engineer 2 starts KAI-388e (fipslint analyzer + CI lane).
- Week 4: Both engineers converge on KAI-388e and the release pipeline. lead-security begins KAI-388f in parallel.
- Week 5: Soak, bug-fix, first `kaivue-recorder-fips` tag candidate.
- Weeks 5–6: KAI-388f wraps (can extend slightly, it is not on the release critical path).

**Solo calendar:** ~7 weeks, serialized in the order above.

### 9.3 Dependencies on adjacent tickets

- **KAI-251 (cryptostore):** Already complete. KAI-388d reuses it directly. Hard dependency, satisfied.
- **Zitadel OIDC integration:** If the Zitadel rollout lands before KAI-388, we can retire `internal/nvr/api/auth.go` password hashing entirely (federated login only) and KAI-388b shrinks to a no-op. If Zitadel slips, KAI-388b is needed as described. **Soft dependency.**
- **KAI-399 (proto/Connect-Go signatures):** KAI-388a coordinates the ECDSA signature wire format with this ticket. Hard coordination, not a hard blocker.
- **KAI-430 (pairing check-in server-side verify):** KAI-388a updates the verify path; must ship in the same release. Hard dependency.
- **KAI-428 (reproducible build + SBOM + cosign):** Already in-flight on this branch. KAI-388e extends the pipeline with the fips lane. Hard dependency, already satisfied by the current branch.

### 9.4 Go/No-Go gates

- **Phase 1 (388a+b+c+d, ~5 weeks):** must land before any FIPS claim is made externally.
- **Phase 2 (388e, +1 week):** must land before the first FedRAMP "In Process" listing.
- **Phase 3 (388f, parallel):** must land before the first ATO (Authority to Operate).
- **Phase 4 (full release matrix):** must land before first production deploy to a federal tenant.

---

## 10. Non-goals

The following are explicitly out of scope for KAI-388 and belong on separate tracks:

- **FIPS 140-3 Level 2 or 3.** Requires tamper-evident packaging or tamper-response hardware. Only relevant if Kaivue ships a hardware appliance. Not on the 2026 roadmap.
- **FedRAMP High.** Higher baseline of 400+ controls beyond Moderate. Separate track with its own budget; our 2026 FIPS work is the prerequisite, not the deliverable.
- **FedRAMP package authorization itself.** This roadmap delivers the crypto prerequisite. The actual FedRAMP authorization effort is 6–18 months of 3PAO engagement, SSP authoring, and PMO review — owned by lead-security and tracked under KAI-385/KAI-386.
- **DoD Impact Level 5 or 6.** Higher classification beyond CUI. IL5+ requires separate physical tenancy; not a code change.
- **CJIS Security Policy.** Criminal Justice Information Services — relevant to law-enforcement customers. Different control catalog (CJIS v5.9.2), overlaps with FIPS on SC-13 but has its own advanced-auth rules. Separate track.
- **HIPAA "encryption best practice."** HIPAA itself is algorithm-agnostic; federal healthcare affiliates inherit SC-13 from their parent agency, and that path is what §1.1 addresses.
- **IL4/IL5 network-boundary controls, STIGs, container hardening.** These are platform controls that ride alongside FIPS crypto but are tracked under the `compliance/soc2` and upcoming `compliance/stig` subdirectories.
- **Post-quantum cryptography.** Kyber/Dilithium standardization under FIPS 203/204/205 is recent (2024); Go support is early. Not blocking 2026 revenue. Revisit in 2027.
- **TLS 1.3-only enforcement.** Nice to have, but TLS 1.2 with approved suites remains compliant. Not on the critical path.

---

## 11. Approval

This roadmap requires CTO sign-off on the top blocking decision stated in §0 (Ed25519 cutover timing) before engineering capacity is allocated. Everything downstream in §4–§9 is mechanical once that call is made.

**Next action:** CTO decision meeting, then file KAI-388a through KAI-388f as Linear children of KAI-388, then kick off Week 1.

---

*Document version 1.0 — 2026-04-08 — lead-security.*
