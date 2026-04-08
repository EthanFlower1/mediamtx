// Package auth defines the IdentityProvider interface — the identity firewall
// for the Kaivue Recording Server cloud control plane and on-prem Directory.
//
// # Architectural seam #3: the identity firewall
//
// Every identity operation (authentication, SSO, user/group CRUD, provider
// configuration, token verification) in the cloud control plane and the
// on-prem Directory MUST flow through the IdentityProvider interface defined
// here. The current implementation in Wave 2 (KAI-223) is a Zitadel adapter,
// but no caller is permitted to import Zitadel types directly. The whole
// point of this seam is that swapping Zitadel for Keycloak, Authentik, or
// any other IdP is a contained ~3-week adapter rewrite, not a cross-cutting
// refactor.
//
// Hard rules for this package:
//
//   - No Zitadel imports. Ever. Not in provider.go, not in types.go, not in
//     fake/. The Zitadel adapter lives in its own package and depends on
//     this one — never the other way around.
//   - Every method is tenant-scoped via TenantRef. There is no "global" user
//     and no "global" group; identity is always tied to either an integrator
//     tenant or a customer tenant.
//   - Token verification returns a Claims struct that already encodes the
//     tenant, the site scope (which Recorders the principal may touch), and
//     the integrator-relationship narrowing chain. Callers must not re-derive
//     these from raw JWT claims.
//
// # Fail-closed-for-security policy
//
// Every method in IdentityProvider is fail-closed. Concretely:
//
//  1. Any error from the underlying provider (network failure, malformed
//     response, expired credentials, unknown tenant, missing config) MUST
//     result in a non-nil error return and a nil result. Implementations
//     MUST NOT return a partially-populated Session, Claims, User, or Group
//     on error. Callers may safely treat (nil, err) as "deny".
//  2. VerifyToken treats unknown signing keys, expired tokens, missing
//     `tid` (tenant id) claims, audience mismatches, and clock-skew
//     overflows as verification failures — never as warnings.
//  3. AuthenticateLocal MUST NOT distinguish "unknown user" from "wrong
//     password" in the returned error; both surface as ErrInvalidCredentials
//     to prevent user enumeration.
//  4. ConfigureProvider and TestProvider MUST refuse to persist a provider
//     configuration whose TestProvider probe failed. The "Sign-in Methods"
//     wizard depends on this — a misconfigured SAML IdP that silently
//     "saves" but does not work is a tenant-lockout incident.
//  5. RevokeSession is idempotent and MUST succeed (return nil) for sessions
//     that are already revoked or never existed. The semantic is "ensure
//     this session id cannot authenticate", not "delete a row".
//  6. Any cross-tenant access attempt — e.g. GetUser called with a UserID
//     that belongs to a different TenantRef than the caller's session —
//     MUST return ErrTenantMismatch. Implementations may not "helpfully"
//     resolve to the right tenant.
//
// Implementations of IdentityProvider are expected to be safe for concurrent
// use by multiple goroutines.
package auth
