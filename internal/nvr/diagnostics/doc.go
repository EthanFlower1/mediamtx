// Package diagnostics provides a one-click support bundle generator that
// collects structured logs, metrics snapshots, camera states, hardware health,
// and sidecar status. The bundle is encrypted with a customer master key
// (AES-256-GCM), uploaded to temporary storage with a 7-day TTL, and assigned
// a shareable bundle ID.
package diagnostics
