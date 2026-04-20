// Package mediamtxsupervisor manages an embedded Raikada instance on
// behalf of the Recorder.
//
// Responsibilities (KAI-259):
//
//   - Own the lifecycle of a single Raikada sidecar process via
//     internal/shared/sidecar.
//   - Render a Raikada path-config block from the Recorder's
//     assigned_cameras cache (internal/recorder/state).
//   - On cache changes, regenerate the path config and apply it via the
//     Raikada HTTP control API as a *hot reload* — no process restart
//     unless the controller reports it cannot apply paths in-place.
//
// # Design
//
// The supervisor is split into three pieces so the moving parts can be
// tested without spawning a real Raikada binary:
//
//  1. CameraSource — anything that can return the current set of cameras
//     and tell us when they change. The production implementation wraps
//     *state.Store; tests use a fake in-memory source.
//
//  2. Controller — the Raikada-facing surface: ApplyPaths(ctx, cfg) for
//     hot reload, and Healthy(ctx) for the supervisor's health probe.
//     The production implementation talks HTTP to Raikada's
//     `/v3/config/paths/replace` endpoint; tests use a fake controller
//     that records calls.
//
//  3. MediaMTXSupervisor — the orchestrator. Calls the Controller on
//     every cache change, and (in production) registers a Sidecar with
//     internal/shared/sidecar so the Raikada process is restarted on
//     crash with exponential backoff.
//
// # Fail-open recording (per CLAUDE.md)
//
// The supervisor never blocks recording on Directory reachability. If
// the camera source returns an error, the supervisor logs it and keeps
// the previously-applied path config in place — recording continues
// against the last known camera set. The same applies when the
// Raikada HTTP API rejects a hot reload: we log, surface the error
// via Stats(), and try again on the next change.
package mediamtxsupervisor
