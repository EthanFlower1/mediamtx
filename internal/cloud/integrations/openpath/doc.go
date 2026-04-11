// Package openpath implements the first-party integration with OpenPath /
// Avigilon Alta cloud access control (KAI-404).
//
// It provides:
//   - OAuth2 client-credentials flow against the Alta API
//   - Webhook receiver for door events (unlock, forced-open, held-open, denied)
//   - Door-event → camera correlation via configurable door-camera mappings
//   - Bidirectional event push (NVR motion → Alta lockdown trigger)
//   - Exponential-backoff retry for transient API failures
//
// Multi-tenant: every Config is scoped to a tenant_id. The Service enforces
// tenant isolation on all operations.
package openpath
