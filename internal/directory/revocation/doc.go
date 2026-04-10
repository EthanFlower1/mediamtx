// Package revocation implements token force-revocation for the on-prem
// Directory. It provides a local JWT blocklist backed by SQLite, an HTTP
// handler for admin-triggered revocation, and a notifier interface for
// pushing revocation events to connected Recorders.
//
// Architecture:
//   - Store: SQLite CRUD for the revoked_tokens table (migration 0003).
//   - Handler: POST /api/v1/admin/recorders/{recorder_id}/revoke-tokens
//   - Notifier: interface for the push path (wired when RecorderControl
//     server-side lands via KAI-142).
//   - Checker: IsRevoked(jti) for middleware integration.
//
// The package lives in internal/directory/ and must not import
// internal/cloud/ or internal/recorder/.
package revocation
