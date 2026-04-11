// Package publicapi implements the KAI-399 public REST + Connect-Go API.
//
// This package mounts versioned public API endpoints under /api/v1/ and
// registers Connect-Go service stubs for all seven CRUD resources:
// cameras, users, recordings, events, schedules, retention, integrations.
//
// Authentication: API key (X-API-Key header) or OAuth bearer token.
// Rate limiting: tiered per tenant (free/starter/pro/enterprise).
// Versioning: /api/v1/ prefix; breaking changes get /v2/.
//
// The package is designed as a contract layer. Downstream tickets (KAI-400
// through KAI-411) implement against the interfaces defined here.
package publicapi
