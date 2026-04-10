// Package bootstrap implements the one-time Zitadel bootstrap procedure
// (KAI-221) that runs after Zitadel is deployed (KAI-220) and before any
// tenant can be provisioned.
//
// The bootstrap sequence:
//
//  1. Create the platform org (or recover its ID if it already exists)
//  2. Create a platform-level service account with org-manager + user-manager
//     roles on the platform org
//  3. Export the service account's JWT key (saved to a file by the caller)
//  4. Create OIDC applications for each Kaivue service that needs to
//     authenticate users via Zitadel:
//     - "kaivue-directory" — the Directory service (server-side, confidential)
//     - "kaivue-recorder"  — the Recorder service (server-side, confidential)
//     - "kaivue-gateway"   — the Gateway service (server-side, confidential)
//     - "kaivue-flutter"   — the Flutter client app (native, PKCE)
//     - "kaivue-web"       — the React admin console (SPA, PKCE)
//
// Every operation is idempotent: re-running bootstrap on an already-configured
// Zitadel instance returns the existing resource IDs without error.
//
// # Seam #3 compliance
//
// This package does NOT import Zitadel SDK types. It consumes the adapter
// through the auth.IdentityProvider interface + the bootstrap helpers on
// *zitadel.Adapter. The caller (cmd/kaivue-bootstrap or a startup hook)
// constructs the adapter and passes it in.
package bootstrap
