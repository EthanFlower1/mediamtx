package recorder

// ----------------------------------------------------------------------------
// Kaivue Recorder role package
// ----------------------------------------------------------------------------
//
// NOTE: This file does not redeclare the package-level godoc comment for
// `package recorder` (that lives in recorder.go). It documents the role this
// package plays in the Kaivue Recording Server architecture introduced by
// KAI-236.
//
// The Recorder role owns everything related to pulling video from cameras
// and putting it on disk:
//
//   - ONVIF discovery and per-camera capture pipelines.
//   - Continuous and event-driven recording to the local segment store.
//   - Local serving of live and playback streams to authorized clients via
//     the embedded MediaMTX sidecar.
//   - Disk lifecycle management: retention, GC, capacity planning, and
//     surfacing health to the Directory.
//   - Reporting timeline and segment metadata upstream so the Directory can
//     assemble a unified timeline across multiple Recorders.
//
// Defining invariant: recording never stops while the Recorder has power and
// disk. The Recorder must continue to capture and persist video even when
// the Directory, the cloud, or the mesh is unreachable. Failure modes fail
// OPEN for recording and CLOSED for auth.
//
// Boundary rules (enforced by depguard, see .golangci.yml):
//
//   - Code under internal/recorder MUST NOT import code under
//     internal/directory. The two roles communicate exclusively via the
//     Connect-Go services defined in internal/shared/proto.
//   - Code under internal/recorder MAY import code under internal/shared
//     for shared types, protos, and primitives.
//
// During the incremental migration from internal/nvr, this package starts
// as a thin wrapper over the existing MediaMTX segment recorder; subsystems
// will be moved here one at a time. Existing code in internal/nvr continues
// to function unchanged until each migration step lands. See
// docs/architecture/package-layout.md for the full plan.
