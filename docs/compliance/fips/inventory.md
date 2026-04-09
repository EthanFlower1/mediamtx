# FIPS 140-3 Cryptography Inventory & Compliance Plan

**Ticket:** KAI-388 — FIPS 140-3 crypto library selection
**Owner:** lead-security (draft by security sub-agent)
**Status:** Working draft — 2026-04-08
**Scope target:** FedRAMP Moderate / DoD IL4 readiness for the Kaivue Recording Server Go monolith (`mediamtx` fork).

---

## Scope

This document inventories all cryptographic primitives in use across `internal/` and `pkg/` of the Kaivue Recording Server (Go), classifies each as FIPS-approved or not, and proposes a migration plan toward a FIPS 140-3 validated build.

**Out of scope:**

- Flutter client crypto (separate effort; dart:io TLS inherits OS module).
- React admin UI (browser WebCrypto — handled per-customer).
- Third-party sidecars (Headscale/Tailscale, step-ca, Zitadel) — handled as separate supply-chain compliance items.
- Database-at-rest (SQLite) — encryption is layered via `cryptostore`, already FIPS-track.

**In scope:**

- All Go source under `internal/**` and `pkg/**` that imports `crypto/*`, `golang.org/x/crypto/*`, `math/rand`, or third-party crypto libraries.
- JWT signing algorithms used by auth, streamclaims, crosstenant, stepca enroll, nvr api.
- TLS configuration (cipher suite selection).
- Random number generation for tokens, nonces, IDs, and retry jitter.

---

## Methodology

- `grep -rn --include="*.go"` over `internal/` and `pkg/` for each target import path.
- Manual inspection of call sites for non-obvious files (`pairing/token.go`, `snapshot.go`, `enroll_token.go`, `auth.go`).
- Classification against:
  - FIPS 140-3 Implementation Guidance (IG) D.G (approved algorithms)
  - NIST SP 800-131A Rev. 2 (transitioning the use of cryptographic algorithms)
  - NIST SP 800-186 (elliptic curve domain parameters)
  - Go 1.24+ `crypto/fips140` module allow-list
  - Go `GOEXPERIMENT=boringcrypto` allow-list

Total crypto-related import lines found: **166** across `internal/` + `pkg/`.
Approximate distinct call sites (signatures, encryptions, hash constructions, RNG draws): **~210**.

---

## Current crypto inventory

### Stdlib `crypto/*` imports

| File:line                                                            | Symbol                                                          | Category                                                                     | FIPS status                                                                                                                                |
| -------------------------------------------------------------------- | --------------------------------------------------------------- | ---------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| internal/shared/cryptostore/cryptostore.go:4-8                       | aes, cipher, hkdf, rand, sha256                                 | AES-256-GCM + HKDF-SHA256                                                    | **Approved** (both boringcrypto & crypto/fips140)                                                                                          |
| internal/shared/cryptostore/cryptostore_test.go:6                    | sha256                                                          | test vectors                                                                 | Approved                                                                                                                                   |
| internal/nvr/crypto/encrypt.go:4-6                                   | aes, cipher, rand                                               | AES-GCM envelope for DB columns                                              | Approved                                                                                                                                   |
| internal/nvr/crypto/keys.go:6-9                                      | rand, rsa, sha256, x509                                         | RSA keygen + JWT kid                                                         | Approved (RSA 2048+)                                                                                                                       |
| internal/nvr/crypto/tls.go:4-9                                       | ecdsa, elliptic, rand, rsa, sha256, x509                        | self-signed TLS bootstrap                                                    | Approved **iff** curve ∈ {P-256, P-384, P-521} — verify                                                                                    |
| internal/nvr/backup/backup.go:9-12                                   | aes, cipher, rand, sha256                                       | encrypted backup                                                             | Approved                                                                                                                                   |
| internal/nvr/api/auth.go:4-6, 17                                     | rand, rsa, sha256 + jwt/v5                                      | RS256 access tokens                                                          | **Approved**                                                                                                                               |
| internal/nvr/api/middleware.go:5, 10                                 | rsa + jwt/v5                                                    | RS256 verify                                                                 | Approved                                                                                                                                   |
| internal/nvr/api/router.go:4                                         | rsa                                                             | pubkey carrier                                                               | Approved                                                                                                                                   |
| internal/nvr/api/evidence.go:5                                       | sha256                                                          | chain-of-custody hashing                                                     | Approved                                                                                                                                   |
| internal/nvr/api/bulk_export.go:5                                    | sha256                                                          | export manifest                                                              | Approved                                                                                                                                   |
| internal/nvr/api/evidence_test.go:5                                  | sha256                                                          | test                                                                         | Approved                                                                                                                                   |
| internal/nvr/webhook/dispatcher.go:7-8                               | hmac, sha256                                                    | HMAC-SHA256 webhook signatures                                               | Approved                                                                                                                                   |
| internal/nvr/nvr.go:6-7                                              | rand, rsa                                                       | bootstrap                                                                    | Approved                                                                                                                                   |
| internal/nvr/updater/updater.go:6                                    | sha256                                                          | update manifest verify                                                       | Approved                                                                                                                                   |
| internal/nvr/ai/model_manager.go:4                                   | sha256                                                          | model integrity                                                              | Approved                                                                                                                                   |
| internal/nvr/alerts/email.go:6                                       | tls                                                             | SMTP STARTTLS                                                                | Approved (via stdlib TLS)                                                                                                                  |
| internal/nvr/backchannel/rtp.go:4                                    | rand                                                            | RTP SSRC / seq seed                                                          | Approved                                                                                                                                   |
| **internal/nvr/onvif/snapshot.go:5, 8**                              | **md5** + **math/rand**                                         | RFC 2617 Digest auth (ONVIF cameras)                                         | **NOT APPROVED** — see §Non-FIPS                                                                                                           |
| **internal/nvr/onvif/events.go:5-6**                                 | rand, **sha1**                                                  | WS-UsernameToken PasswordDigest (ONVIF-required)                             | **NOT APPROVED** for SHA-1                                                                                                                 |
| internal/conf/credential.go:4                                        | sha256                                                          | config credential hashing                                                    | Approved                                                                                                                                   |
| **internal/conf/credential.go:11**                                   | **matthewhartstonge/argon2**                                    | argon2id password verification                                               | **NOT APPROVED** (SP 800-132 requires PBKDF2)                                                                                              |
| internal/conf/conf_test.go:4, 13                                     | rand + nacl/secretbox                                           | test fixtures                                                                | N/A (test)                                                                                                                                 |
| **internal/conf/decrypt/decrypt.go:8**                               | **nacl/secretbox**                                              | encrypted conf loader                                                        | **NOT APPROVED** (XSalsa20-Poly1305)                                                                                                       |
| internal/auth/manager.go:19 + jwt_claims.go:9                        | golang-jwt/jwt/v5                                               | JWT verify (multiple algs)                                                   | Depends on alg — verify HS256/RS256/ES256 only under fips                                                                                  |
| internal/auth/manager_test.go:5-7                                    | rand, rsa, tls                                                  | test                                                                         | Approved                                                                                                                                   |
| internal/stream/stream_format.go:4                                   | rand                                                            | random format padding                                                        | Approved                                                                                                                                   |
| internal/protocols/httpp/server.go:5                                 | tls                                                             | HTTPS server                                                                 | Approved                                                                                                                                   |
| internal/protocols/tls/make_config.go:5-6                            | sha256, tls                                                     | TLS config + fingerprint pinning                                             | Approved                                                                                                                                   |
| internal/protocols/webrtc/from_stream.go:4                           | rand                                                            | SRTP key material (via pion)                                                 | Approved for stdlib use; **DTLS-SRTP via pion uses non-FIPS curves** — see §Non-FIPS                                                       |
| internal/protocols/tls/make_config_test.go:4                         | tls                                                             | test                                                                         | Approved                                                                                                                                   |
| internal/certloader/certloader.go:5 + \_test.go:4                    | tls                                                             | cert rotation                                                                | Approved                                                                                                                                   |
| internal/servers/hls/hlsjsdownloader/main.go:7                       | sha256                                                          | asset integrity                                                              | Approved                                                                                                                                   |
| internal/servers/rtsp/server.go:6                                    | tls                                                             | RTSPS                                                                        | Approved                                                                                                                                   |
| internal/servers/rtmp/server.go:6 + \_test.go:5                      | tls                                                             | RTMPS                                                                        | Approved                                                                                                                                   |
| **internal/servers/webrtc/server.go:6-8**                            | tls, hmac, rand, **sha1**                                       | ICE-STUN short-term credential (HMAC-SHA1 per RFC 5389)                      | **SHA-1 is NOT FIPS-approved for new signatures**; HMAC-SHA1 is still permitted under SP 800-131A as a legacy-use MAC. **Needs doc note.** |
| internal/shared/mesh/tsnet/testmode.go:5                             | sha256                                                          | tailnet test harness                                                         | Approved (test only)                                                                                                                       |
| internal/shared/auth/certmgr/manager.go:6-7 + interfaces.go:6-7      | tls, x509                                                       | cert store                                                                   | Approved                                                                                                                                   |
| internal/shared/auth/certmgr/manager_test.go:7-11                    | ecdsa, elliptic, rand, tls, x509                                | test                                                                         | Approved                                                                                                                                   |
| internal/shared/auth/certmgr/fake/fake.go:9-13                       | ecdsa, elliptic, rand, tls, x509                                | fake                                                                         | Approved (test)                                                                                                                            |
| internal/shared/auth/fake/fake.go:11                                 | rand                                                            | fake                                                                         | Approved (test)                                                                                                                            |
| internal/shared/inference/fake/fake.go:14                            | sha256                                                          | fake                                                                         | Approved                                                                                                                                   |
| internal/shared/streamclaims/issuer.go:5-6, 13                       | rand, rsa, jwt/v5 (RS256)                                       | stream access JWTs                                                           | **Approved**                                                                                                                               |
| internal/shared/streamclaims/nonce.go:4                              | rand                                                            | nonce generation                                                             | Approved                                                                                                                                   |
| internal/shared/streamclaims/verifier.go:11                          | jwt/v5                                                          | RS256 verify                                                                 | Approved                                                                                                                                   |
| **internal/directory/pairing/token.go:5, 7, 15**                     | **ed25519**, sha256, **hkdf (x/crypto)**                        | Pairing token signing + key derivation                                       | **ed25519 NOT APPROVED (yet)**; x/crypto/hkdf is acceptable but should be replaced by stdlib `crypto/hkdf` (available 1.24+)               |
| **internal/directory/pairing/service.go:5**                          | **ed25519**, sha256, tls                                        | Pairing service                                                              | **ed25519 NOT APPROVED**                                                                                                                   |
| **internal/directory/pairing/token_test.go:5-7**                     | ed25519, rand, tls                                              | test                                                                         | Non-FIPS (test)                                                                                                                            |
| **internal/directory/pki/stepca/clusterca.go:6-10**                  | **ed25519**, rand, sha256, tls, x509                            | Directory/Recorder mTLS root CA                                              | **ed25519 root CA NOT APPROVED** — blocker                                                                                                 |
| **internal/directory/pki/stepca/clusterca_test.go**                  | ed25519, rand, x509                                             | test                                                                         | Non-FIPS (test)                                                                                                                            |
| **internal/directory/pki/stepca/enroll_token.go:7**                  | jwt/v5 **EdDSA**                                                | enrollment JWT                                                               | **EdDSA NOT APPROVED** — blocker                                                                                                           |
| internal/directory/mesh/headscale/coordinator.go:5                   | rand                                                            | session nonces                                                               | Approved                                                                                                                                   |
| **internal/recorder/pairing/keystore.go:4-9, 17**                    | aes, cipher, **ed25519**, rand, sha256, x509, **x/crypto/hkdf** | On-disk device keystore                                                      | **ed25519 NOT APPROVED**; migrate hkdf to stdlib                                                                                           |
| **internal/recorder/pairing/join.go:6-10**                           | **ed25519**, rand, sha256, tls, x509                            | Recorder pairing join                                                        | **ed25519 NOT APPROVED**                                                                                                                   |
| **internal/recorder/pairing/join_test.go**                           | ed25519, rand, sha256, tls, x509                                | test                                                                         | Non-FIPS (test)                                                                                                                            |
| internal/recorder/pairing/helpers.go:4-5                             | tls, x509                                                       | helpers                                                                      | Approved                                                                                                                                   |
| internal/recorder/directoryingest/client.go:33, 38                   | tls + **math/rand**                                             | **math/rand for retry jitter only** — acceptable with nolint; audit required | Approved-with-caveat                                                                                                                       |
| internal/recorder/directoryingest/client_test.go:5                   | tls                                                             | test                                                                         | Approved                                                                                                                                   |
| internal/recorder/recordercontrol/client.go:7, 12                    | tls + **math/rand**                                             | retry jitter only                                                            | Approved-with-caveat                                                                                                                       |
| internal/recorder/recordercontrol/client_test.go:5                   | tls                                                             | test                                                                         | Approved                                                                                                                                   |
| internal/recorder/state/store_test.go:5                              | sha256                                                          | test                                                                         | Approved                                                                                                                                   |
| internal/cloud/identity/crosstenant/service.go:5, 12                 | rand, jwt/v5 **HS256**                                          | cross-tenant scoped tokens                                                   | **Approved** (HMAC-SHA256)                                                                                                                 |
| internal/cloud/identity/crosstenant/service_test.go:5, 11            | sha256, jwt/v5                                                  | test                                                                         | Approved                                                                                                                                   |
| internal/cloud/streams/signing_key.go:11-12                          | rsa, x509                                                       | RSA key loader                                                               | Approved                                                                                                                                   |
| internal/cloud/tenants/service.go:5                                  | rand                                                            | tenant IDs                                                                   | Approved                                                                                                                                   |
| internal/cloud/audit/memory.go:5                                     | rand                                                            | audit IDs                                                                    | Approved                                                                                                                                   |
| internal/cloud/apiserver/middleware.go:6                             | rand                                                            | request IDs                                                                  | Approved                                                                                                                                   |
| internal/cloud/jobs/jobs.go:23                                       | rand                                                            | job IDs                                                                      | Approved                                                                                                                                   |
| internal/cloud/relationships/service.go:5                            | rand                                                            | relationship IDs                                                             | Approved                                                                                                                                   |
| **internal/cloud/tests/chaos/chaos_test.go:14**                      | **math/rand**                                                   | chaos test seed                                                              | Test-only, non-blocking                                                                                                                    |
| internal/core/api_test.go:8 + metrics_test.go:6 + path_test.go:6     | tls                                                             | test                                                                         | Approved                                                                                                                                   |
| internal/staticsources/rtsp/source_test.go:5 + rtmp/source_test.go:5 | tls                                                             | test                                                                         | Approved                                                                                                                                   |
| internal/staticsources/rpicamera/mtxrpicamdownloader/main.go:8       | sha256                                                          | firmware pin                                                                 | Approved                                                                                                                                   |
| **internal/shared/sidecar/supervisor.go:10**                         | **math/rand**                                                   | backoff jitter (Rand injected)                                               | Approved-with-caveat (not security-sensitive; audit)                                                                                       |
| **internal/nvr/ai/benchmark_test.go:16**                             | **math/rand**                                                   | synthetic bench data                                                         | Test-only                                                                                                                                  |

### Third-party crypto

| Package                                                                                                   | Sites                                                     | FIPS status                                                                                                                                                                              |
| --------------------------------------------------------------------------------------------------------- | --------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `github.com/golang-jwt/jwt/v5`                                                                            | 11 imports                                                | **Conditional** — only HS256/HS384/HS512/RS256/RS384/RS512/ES256/ES384/ES512/PS256 are approved. `EdDSA` (used in `stepca/enroll_token.go`) is **NOT**.                                  |
| `github.com/matthewhartstonge/argon2`                                                                     | `internal/conf/credential.go`, `internal/nvr/api/auth.go` | **NOT APPROVED** — must migrate to PBKDF2-HMAC-SHA256 or FIPS-approved KDF                                                                                                               |
| `golang.org/x/crypto/hkdf`                                                                                | 4 sites                                                   | **OK functionally** (HKDF-SHA256 is approved IG D.8) but not delivered by the validated module. **Switch to stdlib `crypto/hkdf` (Go 1.24+)** so the routine comes from the FIPS module. |
| `golang.org/x/crypto/nacl/secretbox`                                                                      | `internal/conf/decrypt/decrypt.go` + test fixture         | **NOT APPROVED** (XSalsa20+Poly1305).                                                                                                                                                    |
| `github.com/pion/webrtc/v4` + `pion/ice/v4` + `pion/dtls`                                                 | WebRTC server + outgoing paths                            | pion's DTLS 1.2 defaults negotiate X25519 + Ed25519 / ChaCha20. **NOT APPROVED as shipped.** Needs explicit cipher-suite restriction or a FIPS build tag for pion.                       |
| `github.com/ProtonMail/go-crypto`, `minio/sha256-simd`, `cloudflare/circl`, `tink-go`, `libp2p/go-libp2p` | **Not imported.**                                         | N/A                                                                                                                                                                                      |
| `github.com/lestrrat-go/jwx`                                                                              | Indirect only (via jwkset); direct imports: none found    | N/A                                                                                                                                                                                      |

---

## FIPS-approved usage (clean list)

These are already compatible with `GOEXPERIMENT=boringcrypto` and Go 1.24 `crypto/fips140`:

1. **`internal/shared/cryptostore/*`** — AES-256-GCM, HKDF-SHA256, SHA-256, `crypto/rand`. Gold standard. **No changes required.**
2. **`internal/nvr/crypto/encrypt.go`, `backup/backup.go`** — AES-GCM with `crypto/rand` nonces. OK.
3. **`internal/nvr/api/auth.go` + `middleware.go`** — RS256 JWTs via `golang-jwt/jwt/v5` + `crypto/rsa`. OK.
4. **`internal/shared/streamclaims/*`** — RS256 JWT issuer/verifier. OK.
5. **`internal/cloud/identity/crosstenant/service.go`** — HS256 JWTs. OK.
6. **`internal/nvr/webhook/dispatcher.go`** — HMAC-SHA256 webhook signatures. OK.
7. **`internal/nvr/crypto/tls.go`, certmgr, certloader, protocols/tls** — TLS 1.2+ stdlib. OK under boringcrypto (cipher suites auto-restricted); **audit explicit curve/cipher pinning**.
8. **All `crypto/rand` token/nonce/ID generation** across cloud apiserver, jobs, tenants, relationships, audit, streamclaims nonce, stream format, backchannel RTP, nvr nvr.go.

---

## NON-FIPS usage (needs migration) — ordered by severity

### P0 — Blockers for any federal deployment

1. **Ed25519 everywhere in pairing + stepca root CA.**
   - Files: `internal/directory/pairing/{token,service}.go`, `internal/directory/pki/stepca/clusterca.go`, `internal/directory/pki/stepca/enroll_token.go` (uses `jwt.SigningMethodEdDSA`), `internal/recorder/pairing/{keystore,join}.go`.
   - Impact: Root of trust for Directory↔Recorder mTLS, pairing tokens, and enrollment. Ed25519 is not yet FIPS-approved (FIPS 186-5 approved it in Feb 2023 but Go's `crypto/fips140` module in Go 1.24 does **not** expose Ed25519 as a FIPS-mode primitive; boringcrypto also excludes it).
   - Migration: Replace with **ECDSA P-256** (SigningMethodES256 for the JWT; `ecdsa.GenerateKey(elliptic.P256(), rand.Reader)` for the CA and pairing keys). This is a breaking change for any existing paired Recorders — requires a one-shot re-pair migration and a version bump.
   - Effort: ~2 sprint-weeks including schema migration, key rollover, tests.

2. **argon2 password hashing in `internal/conf/credential.go` and `internal/nvr/api/auth.go`.**
   - Non-FIPS. SP 800-132 only approves **PBKDF2**. Must keep legacy bcrypt/argon2 decoder for backward-compat read, but **new hashes must be PBKDF2-HMAC-SHA256** with ≥ 600k iterations (OWASP 2023) under the `fips` build tag.
   - Effort: ~3 days.

3. **ONVIF Digest Auth (`internal/nvr/onvif/snapshot.go`) uses MD5.**
   - MD5 is NOT FIPS-approved and is not permitted for any purpose in a validated module. Even though this is a legacy protocol requirement for RFC 2617, a FIPS-mode binary **cannot link `crypto/md5`** (boringcrypto allows it only as a non-approved function).
   - Options:
     a. Require Digest-SHA-256 (RFC 7616) for FIPS builds and refuse to snapshot MD5-only cameras with a clear error.
     b. Move the `md5` code into a `//go:build !fips` file and provide a stub for the fips build that returns `errors.New("fips: ONVIF Digest MD5 disabled")`.
   - Recommend (b). Document the reduced camera compatibility matrix in the FedRAMP SSP.
   - Effort: ~1 day.

4. **ONVIF WS-UsernameToken (`internal/nvr/onvif/events.go`) uses SHA-1.**
   - Similar to above. SHA-1 is deprecated by SP 800-131A Rev. 2 for digital signatures but still permitted for HMAC and as a KDF ingredient. WS-UsernameToken PasswordDigest uses SHA-1 as a construction digest (not an HMAC). **Not FIPS-approved.**
   - Recommend: Under `fips` build tag, require WS-Security UsernameToken with SHA-256 or fall back to HTTP Basic over TLS only.
   - Effort: ~1 day.

5. **`nacl/secretbox` in `internal/conf/decrypt/decrypt.go`.**
   - XSalsa20-Poly1305 is NOT FIPS-approved. This is the mediamtx **encrypted config file loader** (a minor feature). Options:
     a. Replace with AES-256-GCM envelope (reuse `cryptostore`).
     b. Disable encrypted config loading entirely under `fips` build tag.
   - Effort: ~1 day. Recommend (a).

### P1 — High priority

6. **Pion WebRTC/DTLS stack in `internal/protocols/webrtc/*` and `internal/servers/webrtc/server.go`.**
   - Pion's DTLS 1.2 default suites include ECDHE-ECDSA-WITH-AES-128-GCM-SHA256 (OK) but it also advertises X25519 and ChaCha20-Poly1305. Pion does not currently support boringcrypto; it has its own Go implementations of AES-GCM and uses `crypto/ecdh` for X25519.
   - For FIPS builds we must either:
     a. Fork/patch pion to restrict to P-256 + AES-GCM only and replace its internal X25519 with crypto/ecdh P-256. Non-trivial.
     b. Use native libwebrtc via CGO with BoringSSL in FIPS mode. This aligns with the Flutter client direction (KAI-334) and is the cleanest path.
     c. Disable WebRTC in `fips` builds and restrict playback to HLS-over-TLS and RTSPS.
   - Recommend (c) for the FedRAMP first-release scope; revisit libwebrtc+BoringSSL in 2026-Q4.
   - Effort: (c) = 2 days; (b) = 6+ weeks.

7. **HMAC-SHA1 in ICE-STUN short-term credential (`internal/servers/webrtc/server.go:482`).**
   - RFC 5389 mandates HMAC-SHA1. SP 800-131A Rev. 2 permits HMAC-SHA1 as a "legacy use" MAC until further notice. Safe for now but must be documented in the SSP as a legacy-use allowance. Tied to the pion decision above — if we disable WebRTC under fips, this goes away.

### P2 — Low priority / housekeeping

8. **`golang.org/x/crypto/hkdf` usage (4 sites).**
   - Functionally OK (HKDF-SHA256) but sourced outside the FIPS module. **Switch to `crypto/hkdf` (Go 1.24 stdlib)** so the code is delivered by the validated module.
   - Files: `internal/directory/pairing/token.go:15`, `internal/recorder/pairing/keystore.go:17`, `internal/nvr/crypto/keys.go:16`, `internal/nvr/backup/backup.go:23`.
   - Effort: ~2 hours, trivial API swap.

9. **`math/rand` audit.**
   - 6 import sites found. All currently appear non-security-sensitive:
     - `internal/shared/sidecar/supervisor.go:10` — backoff jitter, Rand injectable.
     - `internal/recorder/directoryingest/client.go:38` — retry jitter, `//nolint:gosec` annotated.
     - `internal/recorder/recordercontrol/client.go:12` — retry jitter, `//nolint:gosec` annotated.
     - `internal/cloud/tests/chaos/chaos_test.go` — test only.
     - `internal/nvr/ai/benchmark_test.go` — test only.
     - **`internal/nvr/onvif/snapshot.go:233`** — `cnonce := fmt.Sprintf("%08x", rand.Uint32())`. **This is a security-relevant Digest-auth cnonce and MUST use `crypto/rand`.** Low CVSS (ONVIF digest auth on an internal network) but FIPS + CodeQL will both flag it. **Fix before shipping.**
   - Effort: ~2 hours.

---

## Migration plan

### Recommended: **Option A — Go stdlib FIPS build with `fips` build tag**

Rationale:

- Kaivue is already **≥90% stdlib-only for approved primitives** (cryptostore, nvr/crypto, streamclaims, nvr/api/auth, webhook dispatcher, TLS, all cloud services). The ratio of FIPS-ready to FIPS-blocking imports is roughly **45:10 plus 6 `math/rand`**.
- Go 1.24's `crypto/fips140` module provides a NIST CMVP-submitted implementation (FIPS 140-3 Implementation Under Test as of 2025). Shipping under `GOFIPS140=v1.0.0` is a one-line build flag.
- Boringcrypto remains available as a fallback.
- No dependency rewrite required for the bulk of the codebase.

Rejected alternatives:

- **Option B (interface-swappable FIPS crypto impls)**: too invasive, duplicates what Go already does under the hood.
- **Option C (RustCrypto sidecar)**: nonsensical for a Go monolith; only makes sense if we were cross-language.

### Plan of record

1. **Create `fips` build tag convention.**
   - All non-FIPS primitives move to `//go:build !fips` files.
   - Paired stubs under `//go:build fips` either route to an approved replacement or return `fips.ErrUnsupported`.

2. **Phase 1 — "Cosmetic" cleanups (KAI-388a, ~1 week).**
   - Swap `golang.org/x/crypto/hkdf` → stdlib `crypto/hkdf` (4 files).
   - Fix `onvif/snapshot.go` cnonce to use `crypto/rand`.
   - Audit remaining `math/rand` sites; annotate or migrate.
   - Add CI job: `GOFIPS140=v1.0.0 go build ./...` (should pass today once hkdf swap is done for non-ed25519 packages).

3. **Phase 2 — "Disable non-approved surfaces under fips tag" (KAI-388b, ~1 week).**
   - `nacl/secretbox` config decrypt: add fips stub returning error OR swap to AES-GCM.
   - ONVIF MD5 Digest: fips stub returns error; gate camera compat matrix.
   - ONVIF SHA-1 WS-UsernameToken: fips stub returns error.
   - WebRTC servers + protocols: exclude under `//go:build !fips`; routes return 501 Not Implemented when fips build is running.
   - argon2 password hashing: PBKDF2 code path for fips.

4. **Phase 3 — "Ed25519 → ECDSA P-256 migration" (KAI-388c, ~2 weeks, coordinated with lead-directory).**
   - Replace all pairing/stepca signing with ECDSA P-256.
   - stepca enroll_token: `jwt.SigningMethodES256`.
   - Data migration for existing paired Recorders: generate a new ECDSA key on next check-in, deprecate Ed25519 keys after grace period.
   - Update `KAI-430` pairing check-in server-side verify accordingly.
   - Coordinate with lead-backend on proto/Connect-Go signature formats (KAI-399).

5. **Phase 4 — "CMVP package + CI enforcement" (KAI-388d, ~1 week).**
   - `fips` build tag CI job: build + full test suite under `GOFIPS140=v1.0.0 GODEBUG=fips140=only`.
   - Linter (below) gates the `fips` tag.
   - Release matrix adds a `kaivue-recorder-fips` artifact alongside the default build.
   - Ship **only** the fips binary to FedRAMP/DoD customers.

**Total estimated effort: ~5 weeks (1 engineer) or ~3 weeks (2 engineers in parallel).**

---

## Linter proposal (~100 LOC go/analysis pass)

**Location:** `tools/fipslint/` (new).
**Form:** A `go/analysis.Analyzer` named `fipslint.Analyzer` invoked via `go vet -vettool=fipslint`.
**Behavior:** When the `fips` build tag is active (detected via `pass.Pkg.Imports()` and `//go:build` constraints), flag:

1. Imports of:
   - `math/rand`, `math/rand/v2` (allow with explicit `//fipslint:allow jitter` comment)
   - `golang.org/x/crypto/chacha20`, `chacha20poly1305`
   - `golang.org/x/crypto/curve25519`
   - `golang.org/x/crypto/ed25519`, `crypto/ed25519`
   - `golang.org/x/crypto/bcrypt`, `scrypt`, `nacl/*`, `blake2b`, `blake2s`, `poly1305`
   - `github.com/matthewhartstonge/argon2`
   - `crypto/md5`, `crypto/sha1` (allow in legacy-use-only files via allowlist comment)
   - `crypto/dsa`, `crypto/rc4`, `crypto/des`
2. String literals for disallowed JWT algs inside `jwt.NewWithClaims` calls:
   - `EdDSA`, `none`
3. Calls to `elliptic.P224()` (not approved for new use per SP 800-131A).
4. TLS config with `MinVersion < tls.VersionTLS12`.
5. Use of `crypto.SHA1` or `crypto.MD5` as a `crypto.Hash` constant inside signing/verification contexts.

**Allowlist format:** Inline comment `//fipslint:allow <reason>` on the offending line, reviewable in PR.

**Implementation notes for KAI-388 implementer:**

- Use `inspector.Inspector` over `*ast.ImportSpec` and `*ast.CallExpr`.
- Detect build tag via `pass.ResultOf[buildtag.Analyzer]` or read `//go:build` from the `File.Doc`.
- Emit `pass.Reportf(importSpec.Pos(), "fipslint: %s is not FIPS 140-3 approved", path)`.
- Reference impl: golangci-lint's `gosec` rule G401/G501.

---

## Open questions for lead-security

1. **Dual-build vs fips-only.** Do we ship two binaries (`kaivue-recorder` and `kaivue-recorder-fips`) or make fips the default for all customers? Dual-build doubles our CI/release surface; fips-only reduces WebRTC/ONVIF compat.
2. **Ed25519 migration timing.** Wait for Go's `crypto/fips140` to expose Ed25519 (tracked in golang/go#71102, currently gated pending CMVP IG updates)? Or migrate to ECDSA P-256 now and accept the breaking change for existing pairings?
3. **Module validation level.** Level 1 (software-only) is sufficient for FedRAMP Moderate and most SaaS. Level 2 requires tamper-evident packaging (not applicable — we're not an HSM). **Proposed: Level 1.**
4. **CMVP status dependency.** Go 1.24 `crypto/fips140` module is "Implementation Under Test" at CMVP as of mid-2025. Do we ship on IUT status or wait for "Active" certificate? Under FedRAMP "Modified 17020", IUT is acceptable with a waiver. Most vendors ship on IUT.
5. **Camera compat matrix.** ONVIF cameras with MD5 Digest (most pre-2018 cameras) will be unusable under fips. Do we document this as a hardware-compat constraint or bridge through a non-fips sidecar?
6. **WebRTC playback in fips mode.** Can the Flutter client fall back to HLS-over-TLS cleanly? (Yes per KAI-334 HLS path, but latency goes from ~200ms to ~4s.)
7. **Scope of federal deployment.** Do we need IL4, IL5, or IL6? IL5+ requires additional controls beyond FIPS crypto (e.g. separate physical tenancy).

---

## Cost / timeline estimate

| Phase     | Scope                                                                                                | Engineer-weeks        | Ticket               |
| --------- | ---------------------------------------------------------------------------------------------------- | --------------------- | -------------------- |
| 1         | Cosmetic cleanups (hkdf, math/rand, cnonce fix)                                                      | 1.0                   | KAI-388a             |
| 2         | Disable non-approved surfaces under fips tag (ONVIF MD5/SHA1, secretbox, argon2→PBKDF2, WebRTC gate) | 1.5                   | KAI-388b             |
| 3         | Ed25519 → ECDSA P-256 migration (pairing + stepca + enroll token)                                    | 2.0                   | KAI-388c             |
| 4         | CI, fipslint analyzer, release matrix, CMVP package                                                  | 1.0                   | KAI-388d             |
| 5         | FedRAMP SSP crypto section + 3PAO review prep                                                        | 1.5                   | (security, parallel) |
| **Total** |                                                                                                      | **~7 engineer-weeks** |                      |

**Calendar:** ~5 calendar weeks with 2 engineers in parallel, or ~7 weeks solo.

**Go/No-Go gates:**

- Phase 1 must land before any FIPS claim is made externally.
- Phase 2 must land before the first FedRAMP In-Process listing.
- Phase 3 must land before the first ATO (Authority to Operate).
- Phase 4 must land before first prod deploy to a federal tenant.

---

## References

- **FIPS 140-3** — Security Requirements for Cryptographic Modules (NIST, March 2019).
- **FIPS 140-3 Implementation Guidance** — NIST CMVP, latest revision.
- **NIST SP 800-131A Rev. 2** — Transitioning the Use of Cryptographic Algorithms and Key Lengths (March 2019).
- **NIST SP 800-132** — Recommendation for Password-Based Key Derivation (PBKDF2).
- **NIST SP 800-186** — Recommendations for Discrete Logarithm-Based Cryptography (Feb 2023; includes approved curves).
- **NIST SP 800-52 Rev. 2** — TLS Implementations (approved TLS 1.2+ cipher suites).
- **FIPS 186-5** — Digital Signature Standard (Feb 2023; includes Ed25519 — approved, but not yet exposed by Go's FIPS module).
- **FIPS 180-4** — Secure Hash Standard (SHA-2 family approved; SHA-1 legacy-use only).
- **NIST SP 800-107 Rev. 1** — Recommendation for Applications Using Approved Hash Algorithms.
- **Go boringcrypto documentation** — https://pkg.go.dev/crypto/internal/boring
- **Go `crypto/fips140` module** — https://go.dev/doc/security/fips140 (Go 1.24+).
- **CMVP — Modules In Process** — https://csrc.nist.gov/projects/cryptographic-module-validation-program/modules-in-process
- **FedRAMP Cryptographic Modules Policy** — https://www.fedramp.gov/assets/resources/documents/CSP_Cryptographic_Module_Compliance.pdf
- **Internal:** `docs/compliance/soc2/` (KAI-385), `docs/compliance/eu-ai-act/` (KAI-294), KAI-251 cryptostore rescue notes.

---

_End of working draft. Routing to lead-security for review. Please answer the 7 open questions before Phase 1 kickoff._
