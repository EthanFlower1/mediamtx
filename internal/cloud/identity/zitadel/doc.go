// Package zitadel implements the auth.IdentityProvider interface against a
// Zitadel (https://zitadel.com) identity server, and is the only place in
// the codebase permitted to import Zitadel SDK types. Every other package
// consumes identity through auth.IdentityProvider — seam #3 of the
// multi-tenant architecture.
//
// # Org hierarchy
//
// The adapter maps Kaivue's two-level tenant model onto Zitadel orgs:
//
//	Kaivue                      Zitadel
//	------                      -------
//	Integrator tenant    ─────▶  Org (type=integrator)
//	Customer   tenant    ─────▶  Org (type=customer, ParentOrgID=integrator)
//	User                 ─────▶  User inside tenant's org
//	Group                ─────▶  Project Role / Grant inside tenant's org
//
// A single "platform" org (Config.PlatformOrgID) owns the adapter's own
// service-account credentials. All tenant-scoped calls must carry the
// `x-zitadel-orgid` header derived from the caller's TenantRef — never from
// a client-supplied value. This is enforced centrally in client.go.
//
// # Fail-closed contract
//
// Every exported method honors the auth.IdentityProvider fail-closed
// contract:
//
//   - Any Zitadel transport / JSON / auth error → typed sentinel from
//     the auth package (ErrInvalidCredentials, ErrTokenInvalid, etc.) with
//     no leak of which check failed (prevents user enumeration).
//   - Missing tenant, zero-value TenantRef, cross-org access attempts →
//     auth.ErrTenantMismatch.
//   - nil Config or nil HTTPClient → New returns an error; the adapter
//     refuses to construct in an unsafe state.
//
// # Build tags
//
// The package compiles two different backends behind build tags so that
// sandboxes without network access to the Zitadel Go SDK can still build
// and test the adapter:
//
//	//go:build !zitadel_sdk    — default. Uses the in-tree HTTP stub in
//	                            zitadel_sdk_stub.go, which speaks raw
//	                            Zitadel REST over Config.HTTPClient and is
//	                            what every unit test exercises.
//
//	//go:build zitadel_sdk     — opt-in. Wires the real
//	                            github.com/zitadel/zitadel-go/v3 SDK.
//	                            Enable with `go build -tags zitadel_sdk`
//	                            once the dependency is vendored (KAI-220
//	                            handoff).
//
// The public API (`*Adapter`, `Config`, `New`, bootstrap helpers) is
// identical under both tags, so upstream callers never need to care which
// backend is compiled in.
package zitadel
