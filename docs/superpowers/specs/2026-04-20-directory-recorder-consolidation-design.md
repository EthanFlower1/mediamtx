# Directory/Recorder Architecture Consolidation

**Date:** 2026-04-20
**Status:** Approved
**Scope:** Full decomposition of `internal/nvr/` (63K LOC, 176 files) into the directory/recorder/shared architecture. Removes legacy mode. Establishes clean control-plane/data-plane separation.

---

## 1. Problem Statement

The codebase has two overlapping architectures:

1. **Legacy NVR** (`internal/nvr/`) — a monolithic 63K-line package with its own DB, API, camera management, recording, AI, and auth. Activated by `mode: ""` (default).
2. **Federated architecture** (`internal/directory/` + `internal/recorder/`) — a new control-plane/data-plane split with pairing, streaming APIs, and mode dispatch. Activated by `mode: directory|recorder|all-in-one`.

Both implement camera CRUD, user management, recording schedules, and API routes. The duplication creates maintenance burden, architectural confusion, and blocks the Flutter client from having a single connection model that works for both single-server and enterprise deployments.

## 2. Design Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Single-server experience | All-in-one (directory + recorder in-process) | Eliminates dual-maintenance; user experience unchanged |
| Legacy mode | Removed | Default empty `mode:` becomes all-in-one |
| Recorder autonomy | Split-brain with local autonomy | "Recording never stops" invariant; Recorder operates on cached state when disconnected |
| Client-to-Recorder | Directory for discovery, direct to Recorder for data | Industry-standard VMS pattern (Milestone/Genetec); scales better than proxy |
| NVR decomposition | Full, single branch | Clean cut; no transitional wrappers |
| DB schema | Split into `directory.db` + `state.db` with one-time migration | Clean ownership boundaries per role |

## 3. Runtime Modes

The `mode` field in `mediamtx.yml` accepts three values:

| Config | Behavior | Audience |
|--------|----------|----------|
| `mode: ""` (default/omitted) | Boots all-in-one: directory + recorder in-process, auto-paired | Single-server users, small installs |
| `mode: directory` | Boots directory only | Enterprise control plane |
| `mode: recorder` | Boots recorder only | Enterprise capture nodes |

### Mode Dispatch (core.go)

```
Core.New()
  mode == "" or "all-in-one" → wire both directoryBooter + recorderBooter
  mode == "directory"        → wire directoryBooter only
  mode == "recorder"         → wire recorderBooter only

dispatchRuntimeMode()
  "" / "all-in-one" → Boot Directory → Boot Recorder → AutoPair
  "directory"       → Boot Directory
  "recorder"        → Boot Recorder
```

No legacy path. No `internal/nvr` import from core.

### Migration Path for Existing NVR Users

- Existing `nvr: true` config continues to work
- On first boot after upgrade, detects `nvr.db` exists and `directory.db` does not
- Runs one-time migration: splits `nvr.db` → `directory.db` + `state.db`
- Renames `nvr.db` → `nvr.db.backup` (never deletes user data)
- Logs migration summary with row counts
- Camera paths, recording history, user accounts, and settings are preserved

## 4. Package Structure

```
internal/
├── directory/           # Control plane — source of truth
│   ├── boot.go          # Existing 7-stage boot (updated)
│   ├── db/              # Existing migrations + absorbed NVR tables
│   ├── adminapi/        # Existing + absorbed: users, roles, auth,
│   │                    #   federation, branding, groups, audit, apikeys
│   ├── cameraapi/       # NEW: camera management CRUD (from nvr/api/cameras.go)
│   ├── recorderapi/     # Existing (registration, heartbeat, camera CRUD)
│   ├── recordercontrol/ # Existing (StreamAssignments, EventBus)
│   ├── webhook/         # From nvr/webhook/
│   ├── pairing/         # Existing
│   └── fanout/          # Existing FanoutService (query aggregation)
│
├── recorder/            # Data plane — capture, store, serve
│   ├── boot.go          # Existing multi-phase boot (updated)
│   ├── state/           # Existing local SQLite cache
│   ├── recordercontrol/ # Existing (StreamAssignments client)
│   ├── directoryingest/ # Existing (camera state, segments, AI events)
│   ├── pairing/         # Existing (Joiner)
│   ├── onvif/           # From nvr/onvif/ (8,866 LOC)
│   ├── ai/              # From nvr/ai/ (5,881 LOC)
│   ├── scheduler/       # From nvr/scheduler/ (2,336 LOC)
│   ├── driver/          # From nvr/driver/ (2,546 LOC)
│   ├── storage/         # From nvr/storage/ (735 LOC)
│   ├── backchannel/     # From nvr/backchannel/ (674 LOC)
│   ├── recovery/        # From nvr/recovery/ (502 LOC)
│   ├── integrity/       # From nvr/integrity/ (332 LOC)
│   ├── thumbnail/       # From nvr/thumbnail/ (339 LOC)
│   ├── alerts/          # From nvr/alerts/ (478 LOC)
│   ├── connmgr/         # From nvr/connmgr/ (362 LOC)
│   ├── yamlwriter/      # From nvr/yamlwriter/ (440 LOC)
│   ├── managed/         # From nvr/managed/ (518 LOC)
│   ├── recordingapi/    # NEW: recordings, HLS, export, clips, bookmarks,
│   │                    #   tours, tracks, thumbnails, screenshots,
│   │                    #   recording_control, backchannel, bulk_export
│   ├── detectionapi/    # NEW: detection zones/schedules/events,
│   │                    #   AI metrics, forensic search, edge search
│   └── db/              # NEW: recorder-local tables
│
├── shared/              # Cross-cutting utilities
│   ├── runtime/         # Existing (mode dispatch, AutoPair)
│   ├── proto/           # Existing (Connect-Go protos)
│   ├── auth/            # Existing + consolidated crypto from nvr/crypto/
│   ├── migration/       # From nvr/migration.go — legacy DB migrator
│   ├── diagnostics/     # From nvr/diagnostics/ (1,596 LOC)
│   ├── syscheck/        # From nvr/syscheck/ (458 LOC)
│   ├── logmgr/          # From nvr/logmgr/ (614 LOC)
│   ├── metrics/         # From nvr/metrics/ (225 LOC)
│   ├── updater/         # From nvr/updater/ (509 LOC)
│   ├── hwaccel/         # From nvr/hwaccel/ (338 LOC)
│   ├── middleware/      # From nvr/api/middleware.go — auth, logging, errors
│   └── systemapi/       # NEW: health, system status, diagnostics,
│                        #   updates, logging, security, TLS, sizing
│
└── core/
    └── core.go          # Updated: no nvr imports
```

## 5. Recorder Local Autonomy

The Recorder is a self-sufficient recording node that survives Directory disconnection.

### Behavior Matrix

| Responsibility | Connected | Disconnected |
|---|---|---|
| Camera assignments | Syncs from Directory via StreamAssignments | Uses cached assignments, continues recording |
| Recording schedules | Receives schedule updates from Directory | Executes last-known schedule |
| Retention/cleanup | Receives policy from Directory | Executes last-known policy |
| Segment indexing | Streams indexes to Directory via PublishSegmentIndex | Queues locally in `pending_syncs`, syncs on reconnect |
| AI detections | Streams events to Directory via PublishAIEvents | Queues locally, syncs on reconnect |
| Health reporting | Streams via StreamCameraState | Continues recording, buffers health data |
| Local diagnostics API | Available (read-only) | Available (read-only) |

### Recorder Local API

The Recorder exposes its own HTTP API:

- `GET /api/v1/recordings` — query local segment index
- `GET /api/v1/recordings/:id/hls` — serve HLS playback directly
- `GET /api/v1/timeline` — local timeline for assigned cameras
- `GET /api/v1/health` — disk, CPU, camera status, connection state
- `GET /api/v1/export` — local clip export
- `GET /api/v1/detections` — query local detection events
- `GET /api/v1/diagnostics` — support bundle from this node

These endpoints are what the Flutter client hits directly after discovering Recorders via Directory, and what the Directory's FanoutService calls when aggregating queries.

### Offline Queue

When the Recorder loses contact with the Directory, ingest data (segment indexes, AI events, camera health) queues in a local `pending_syncs` table. On reconnect, DirectoryIngest clients drain the queue in order before resuming real-time streaming. Queue is bounded (configurable max rows, default 100K) with oldest-first eviction.

## 6. Flutter Client Connection Model

### Two-Phase Connection

**Phase 1 — Directory (control plane):**
- Login, auth, camera list, federation peers, schedule/retention config, user management
- All admin operations go through the Directory
- Directory returns camera assignments with Recorder endpoints attached

**Phase 2 — Recorder (data plane):**
- Live streams (WHEP/WebRTC), HLS playback, timeline queries, exports, detection events
- Client connects directly to the Recorder that owns each camera

### Camera Model Enrichment

Camera objects returned from Directory include Recorder routing info:

```dart
class Camera {
  final String id;
  final String name;
  final String recorderId;        // which Recorder owns this camera
  final String recorderEndpoint;  // direct URL to that Recorder's API
  final String directoryId;       // which Directory (for federation)
}
```

### Token Strategy

- Directory issues a JWT valid across all Recorders in its fleet
- Recorders validate using a shared signing key distributed during pairing
- No separate login per Recorder — one token, fleet-wide
- Token carries `recorder_ids: ["*"]` or scoped list based on user role
- Existing `ConnectionScopedKeys` in secure storage handles this naturally

### Single-Server Behavior

In all-in-one mode, `recorderEndpoint` points to the same host as the Directory. Client code is identical — no special-casing. The Directory and Recorder APIs are on the same port.

### Federation

- Each federated peer is a separate Directory with its own Recorders
- Client gets peer camera lists via home Directory's federation API
- For federated camera playback, client uses the peer's Recorder endpoint directly
- `sourceConnectionId` tracks which Directory a camera belongs to

## 7. Database Schema Split

### Directory DB (`directory.db`) — 22 Tables

| Category | Tables |
|---|---|
| Identity | `users`, `roles`, `api_keys`, `sessions`, `tokens`, `notification_prefs` |
| Cameras | `cameras`, `camera_streams`, `devices` |
| Scheduling | `recording_schedules`, `retention_policies` |
| Enterprise | `federation`, `groups`, `integrations` |
| Ingest (from Recorders) | `segment_index`, `camera_health`, `ai_events` |
| System | `webhooks`, `audit`, `alerts`, `config`, `upgrade_migrations`, `branding` |

These merge with existing `internal/directory/db/` tables. Where both NVR and Directory have the same table, the Directory version wins and the NVR table's extra columns are absorbed via migration.

### Recorder DB (`state.db`) — 19 Tables

| Category | Tables |
|---|---|
| Local state | `local_state`, `assigned_cameras` |
| Recordings | `recordings`, `clips`, `bookmarks`, `saved_clips`, `clip_index` |
| Playback | `tracks`, `tours` |
| Detection | `detections`, `detection_events`, `detection_zones`, `detection_schedules`, `motion_events` |
| Storage | `retention`, `quota`, `storage_estimate`, `maintenance` |
| Sync | `pending_syncs`, `connection_events` |

These merge with existing `internal/recorder/state/` tables.

### Migration Utility (`internal/shared/migration/`)

1. Detect `nvr.db` exists and `directory.db` does not
2. Create `directory.db`, copy Directory-owned tables with data
3. Create `state.db`, copy Recorder-owned tables with data
4. Rename `nvr.db` → `nvr.db.backup`
5. Log migration summary with row counts

Runs once during `Core.New()` before mode dispatch.

## 8. API Split

The monolithic `nvr/api/` (65 files, 21,532 LOC) splits into 5 API packages:

### `internal/directory/cameraapi/` (~5,200 LOC)
Camera management CRUD: create/update/delete, stream config, ONVIF discovery triggers. Admin operations only.

### `internal/directory/adminapi/` (existing + ~4,800 LOC absorbed)
Users, roles, API keys, sessions, auth (login/SSO/refresh). Federation, groups, branding, audit. Alerts config, notification preferences, webhooks config.

### `internal/recorder/recordingapi/` (~8,000 LOC)
Recordings query, HLS serving, clip export, bulk export, evidence. Bookmarks, tours, tracks, saved clips, screenshots, thumbnails. Recording control, backchannel audio. Recording health, stats, storage/quota.

### `internal/recorder/detectionapi/` (~2,500 LOC)
Detection zones, detection schedules, detection events. AI metrics, forensic search, edge search.

### `internal/shared/systemapi/` (~3,000 LOC)
Health check, system status, sizing. Diagnostics, support bundle. Updates, logging config, security/TLS config. Metrics endpoint.

### Router Pattern

Per-package route registration replaces the monolithic router:

```go
// Each package exposes a Register function
func (h *Handler) Register(r gin.IRouter) {
    // registers only its own routes
}

// Directory boot.go composes:
cameraapi.NewHandler(dirDB).Register(router)
adminapi.NewHandler(dirDB).Register(router)
systemapi.NewHandler().Register(router)

// Recorder boot.go composes:
recordingapi.NewHandler(recDB).Register(router)
detectionapi.NewHandler(recDB).Register(router)
systemapi.NewHandler().Register(router)
```

In all-in-one mode, both sets register on the same router — full API surface on one port.

Middleware (`nvr/api/middleware.go`) moves to `internal/shared/middleware/` since both roles need auth, request logging, and error handling.

## 9. File Disposition Matrix

### Delete (1,713 LOC)
- `nvr/nvr.go` — orchestrator replaced by directory/recorder boot sequences

### Move Unchanged to Recorder (14 packages, ~23,800 LOC)
`onvif/`, `ai/`, `scheduler/`, `driver/`, `storage/`, `backchannel/`, `recovery/`, `integrity/`, `thumbnail/`, `alerts/`, `connmgr/`, `managed/`, `yamlwriter/`, `hwaccel/`

Update import paths only. No logic changes.

### Move Unchanged to Shared (5 packages, ~3,400 LOC)
`diagnostics/`, `syscheck/`, `logmgr/`, `updater/`, `metrics/`

### Move to Directory (1 package, 358 LOC)
`webhook/`

### Move to Shared Migration (226 LOC)
`nvr/migration.go` → `internal/shared/migration/`

### Absorb into Recorder (157 LOC)
`nvr/recovery_adapter.go` → `internal/recorder/recovery/`
`nvr/fragment_backfill.go` → `internal/recorder/` boot sequence

### Split (32,500 LOC)
- `api/` (21,532 LOC) → `cameraapi/`, `adminapi/`, `recordingapi/`, `detectionapi/`, `systemapi/`
- `db/` (10,317 LOC) → `directory/db/` + `recorder/db/`
- `crypto/` (474 LOC) → consolidated into `internal/shared/auth/`

### Absorb Small Packages (205 LOC)
- `audit/` (199 LOC) → admin-side into `directory/adminapi/`, recording-side into `recorder/recordingapi/`
- `ui/` (6 LOC, `embed.go`) → Directory serves SPA; in all-in-one both roles share it

### Net Result
`internal/nvr/` directory deleted entirely. Zero files remain.

## 10. Architectural Invariants

These must hold after the decomposition:

1. **`internal/directory/` and `internal/recorder/` cannot import each other.** Only `internal/shared/` crosses the boundary. Enforced by depguard linter.
2. **Connect-Go `.proto` is the only inter-role contract.** Serialized by proto-lock.
3. **Recording never stops.** Fail closed for security, fail open for recording.
4. **Default mode boots all-in-one.** Single-server users need zero config changes.
5. **One-time migration is non-destructive.** Original `nvr.db` is backed up, never deleted.
6. **All-in-one exposes full API surface on one port.** No functional difference from old single-NVR for the client.
7. **Fleet-wide JWT.** One token from Directory works on all Recorders.
8. **Recorder offline queue is bounded.** Default 100K rows, oldest-first eviction.
