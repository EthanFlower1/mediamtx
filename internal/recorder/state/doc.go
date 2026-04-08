// Package state is the Recorder-side local SQLite cache that mirrors
// the set of cameras assigned to this Recorder from the Directory.
//
// This package is the *source of truth for runtime behavior*. The
// Recorder reads from this cache — not from the Directory — when
// deciding what to capture. The Directory only pushes deltas into the
// cache via a reconciliation loop. If the Directory becomes unreachable,
// the Recorder continues to run against the last-known-good snapshot
// stored here.
//
// Three tables live in the local cache:
//
//	assigned_cameras — the current assignment snapshot (per camera_id)
//	local_state      — free-form key/value state (last sync, etc)
//	segment_index    — a thin index over on-disk recorded segments
//
// RTSP credentials are stored encrypted (ciphertext column) via the
// cryptostore interface defined in this package. The real implementation
// is expected to come from internal/shared/cryptostore (KAI-251); this
// package only depends on the interface.
package state
