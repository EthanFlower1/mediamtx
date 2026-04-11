// Package brivo implements a first-party integration with Brivo's cloud
// access-control platform (https://www.brivo.com).
//
// Capabilities:
//   - OAuth 2.0 authorization-code flow with PKCE for tenant onboarding
//   - Automatic token refresh with configurable retry/back-off
//   - Webhook ingestion of Brivo door-unlock / forced-entry events
//   - Correlation of door events to nearby cameras for video snapshots
//   - Config UI helpers (list sites, list doors, test connectivity)
//   - Bidirectional event flow: NVR motion events can trigger Brivo lockdowns
//
// Multi-tenant: every Brivo connection is scoped to a (tenant_id, site_id)
// pair. Tokens are stored encrypted via the credential vault. Cross-tenant
// access is structurally impossible.
//
// Wire-up: register the Handler with the apiserver router under
// /kaivue.v1.IntegrationsService/brivo/*.
package brivo
