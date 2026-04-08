// Package recordercontrol implements the Recorder-side client of the
// Directory → Recorder push channel defined in
// internal/shared/proto/v1/recorder_control.proto (KAI-238).
//
// The server side (KAI-252) lives in internal/cloud/recordercontrol/.
// This package is the mirror: it dials the Directory's
// StreamAssignments endpoint, applies incoming events to the local
// SQLite camera cache (KAI-250), and reconciles running capture loops
// idempotently.
//
// # Package boundary
//
// Per the depguard rules (KAI-236) this package may import:
//   - internal/shared/...
//   - internal/recorder/...
//
// It MUST NOT import internal/directory/... or internal/cloud/....
//
// # Recording-never-stops invariant
//
// Reconcile is advisory, not authoritative: a failure to start a new
// capture is logged and metered but NEVER causes an in-progress capture
// to stop. The capture loop is the source of truth; this package is the
// soft-state sync channel.
package recordercontrol
