// Package tokenverify implements the shared JWT verification primitive
// used identically by every Kaivue role that accepts an access token —
// Directory API middleware, Recorder MediaMTX auth webhook (KAI-260),
// and Gateway stream auth (KAI-261). It lives inside
// internal/shared/auth, the architectural firewall: no Zitadel SDK type
// ever leaks out of this package, and no caller imports Zitadel
// libraries to do JWT verification. Instead, callers construct a
// [TokenVerifier] from a JWKS URL (the one Zitadel exposes today;
// tomorrow it could be Keycloak or Authentik) and call [TokenVerifier.Verify]
// on raw JWT strings.
//
// # Firewall seam
//
// The parent package internal/shared/auth defines the IdentityProvider
// interface, the high-level identity firewall for the control plane.
// That interface operates on tenant-scoped [auth.Claims] with UserID,
// TenantRef, Groups, etc. The primitive in this subpackage operates one
// layer deeper: it takes a raw JWT + a JWKS URL, verifies signature and
// standard RFC 7519 claims, and returns the decoded [VerifiedToken] with
// the remaining custom claims as a map. Adapter code (the Zitadel
// adapter in KAI-132, the Directory API middleware in KAI-148, etc.) is
// responsible for mapping [VerifiedToken].Custom into tenant-scoped
// [auth.Claims]. Two layers, one firewall: the ONLY place JWT parsing
// happens is here.
//
// # Security contract
//
// [TokenVerifier.Verify] is fail-closed. Every one of the following
// checks must pass or Verify returns (nil, error); no partial result,
// no warning path:
//
//  1. Token is well-formed: three base64url parts with valid JSON.
//  2. Header alg is on the allowlist: RS256, RS384, RS512, ES256, ES384.
//     `alg: none` and `alg: HS256` are REJECTED unconditionally — the
//     JWT spec-confusion attacks that motivate this allowlist are real
//     and frequent.
//  3. Header kid is present and matches a key in the cached JWKS. On a
//     cache miss, a single synchronous refresh is triggered (handles
//     Zitadel key rotation mid-verify); if the kid is still missing
//     after the refresh, verification fails.
//  4. The signature verifies against that key.
//  5. iss equals the configured issuer exactly.
//  6. aud contains the configured audience.
//  7. exp > clock.Now() − SkewTolerance.
//  8. nbf ≤ clock.Now() + SkewTolerance (if nbf is present).
//
// Clock skew is bounded by [SkewTolerance] = 60 seconds on both exp and
// nbf. The clock is injectable via [VerifierConfig.Clock] so tests can
// pin time deterministically.
//
// # JWKS caching and rotation
//
// The verifier lazily fetches its JWKS on the first Verify call and
// then refreshes in the background every [VerifierConfig.CacheTTL]
// (default 5 minutes). This is the same behavior every Kaivue token
// acceptor needs — hoisted once, tested once, reviewed once.
//
// Zitadel rotates its signing keys periodically. The normal refresh
// loop handles that eventually, but the window between rotation and
// the next scheduled refresh can be long. To avoid rejecting valid
// tokens signed with the new key during that window, a kid miss
// triggers a single synchronous forced refresh and a retry of the
// signature check. This is rate-limited internally so a flood of
// unknown-kid tokens (forged or otherwise) cannot DDOS the IdP.
//
// # Shutdown
//
// [TokenVerifier.Close] stops the background refresh goroutine and
// must be called when the owner is shutting down — failing to call it
// leaks a goroutine per verifier instance. Close is idempotent.
//
// # Concurrency
//
// [TokenVerifier] is safe for concurrent use from multiple goroutines.
// Verify may be called concurrently with itself and with the background
// refresh; the underlying JWKS storage is protected by a read/write
// mutex provided by the keyfunc storage implementation.
package tokenverify
