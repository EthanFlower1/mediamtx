# Per-Camera Storage Paths Design

## Overview

Add per-camera storage path configuration so cameras can record to different storage locations (e.g., external NAS mounts). Includes automatic failover to local storage when remote paths are unreachable, and deferred sync-back when they recover.

## Requirements

- Each camera can specify a custom storage root (e.g., `/mnt/nas1/recordings/`)
- Cameras without a custom path use the global default (`./recordings/`)
- New directory structure: `<storage_path>/<camera_id>/<stream_type>/<YYYY-MM>/<DD>/<HH-MM-SS-ffffff>.mp4`
- When a storage path becomes unreachable, recording fails over to local storage transparently
- When storage recovers, fallback recordings are synced to the correct NAS location and local copies are deleted
- Storage health is visible in the UI

## Architecture: NVR-Layer Storage Manager

All storage intelligence lives in the NVR layer. MediaMTX's recorder writes wherever its `recordPath` config points — the NVR dynamically rewrites that config based on storage health. This keeps MediaMTX core untouched and centralizes storage logic.

## Data Model

### Camera table — new column

```sql
ALTER TABLE cameras ADD COLUMN storage_path TEXT NOT NULL DEFAULT '';
```

- Empty string = use global default (`./recordings/`)
- Any other value = absolute path to the camera's storage root

### New table: `pending_syncs`

```sql
CREATE TABLE pending_syncs (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    recording_id INTEGER NOT NULL,
    camera_id TEXT NOT NULL,
    local_path TEXT NOT NULL,
    target_path TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    attempts INTEGER NOT NULL DEFAULT 0,
    error_message TEXT,
    created_at TEXT NOT NULL,
    last_attempt_at TEXT,
    FOREIGN KEY (recording_id) REFERENCES recordings(id) ON DELETE CASCADE,
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);

CREATE INDEX idx_pending_syncs_status ON pending_syncs(status);
CREATE INDEX idx_pending_syncs_camera ON pending_syncs(camera_id);
```

Status values: `pending`, `syncing`, `failed`.

When a `pending_syncs` row is cascade-deleted (because the parent recording was deleted by retention), the sync worker's periodic sweep must also clean up the orphaned local file. The sweep checks for files in local fallback directories that have no corresponding `pending_syncs` entry.

### Record path format

MediaMTX requires `recordPath` to contain `%path`, `%Y %m %d %H %M %S`, and `%f`. The NVR achieves the desired directory structure by combining the `recordPath` with the MediaMTX path name.

The NVR sets each camera's `recordPath` in YAML as:

```
<effective_storage_path>/%path/%Y-%m/%d/%H-%M-%S-%f
```

Where `effective_storage_path` is `camera.storage_path` if set, otherwise `./recordings/`.

The `%path` token is replaced by MediaMTX at runtime with the camera's MediaMTX path name. The NVR structures the MediaMTX path name itself to encode camera ID and stream type:

```
nvr/<camera_id>/main    (main stream)
nvr/<camera_id>/sub     (sub stream)
```

This produces the final on-disk structure:

```
<storage_path>/nvr/<camera_id>/main/2026-03/25/14-30-45-123456.mp4
<storage_path>/nvr/<camera_id>/sub/2026-03/25/14-30-45-123456.mp4
```

This satisfies MediaMTX's `%path` requirement while achieving the desired `<camera_id>/<stream_type>/<month>/<day>/` hierarchy.

**Note:** This changes the MediaMTX path naming convention from `nvr/<sanitized-name>` to `nvr/<camera_id>/main` (and `/sub`). Existing cameras will need their `mediamtx_path` migrated. The migration section below covers this.

## StorageManager Service

New service at `internal/nvr/storage/manager.go`. Runs as a long-lived goroutine with three responsibilities. The in-memory health status map is protected by a `sync.RWMutex` since it is read by the API handler goroutines and written by the health monitor goroutine.

### Health Monitor

- Runs on a configurable interval (default 30 seconds)
- For each camera with a custom `storage_path`, checks reachability:
  - Primary: write and remove a small temp file to verify write access
  - Secondary: `os.Stat` on the mount point to detect stale NFS mounts
- Maintains in-memory map: storage path -> health status (protected by `sync.RWMutex`)
- On state change (healthy -> unhealthy or vice versa), triggers failover or recovery
- On startup, runs the first health check synchronously during `StorageManager` initialization (before entering the goroutine loop) so the API never returns `unknown` to clients

### Failover Handler

**When storage becomes unreachable:**

1. Rewrites the camera's YAML `recordPath` to local fallback via `SetPathValue(pathName, "recordPath", fallbackPath)`
2. Triggers MediaMTX config reload via existing API
3. Sets camera `storage_status` to `degraded` (see Storage Status section below)

**When storage becomes reachable again:**

1. Rewrites YAML back to the NAS path via `SetPathValue(pathName, "recordPath", primaryPath)`
2. Triggers config reload
3. Kicks off sync queue processing for that storage path
4. Clears degraded status

### Sync Worker

- Sets row status to `syncing` before beginning transfer (prevents concurrent workers or manual triggers from double-processing)
- Copies file to `target_path`, verifies integrity (size match), then deletes `local_path`
- Updates the `recordings` table `file_path` to the new NAS location
- On failure: increments `attempts`, sets `last_attempt_at` and `error_message`, retries on next cycle
- Max retries configurable (default 10); after exhaustion, status set to `failed` and alert surfaced
- Periodic orphan sweep: checks local fallback directories for files with no corresponding `pending_syncs` or `recordings` entry, and removes them

## YAML Writer Integration

### Path Construction

When a camera is created or updated, the NVR builds the `recordPath` and writes it via the existing YAML writer:

```
effective_storage_path = camera.storage_path || "./recordings/"
recordPath = <effective_storage_path>/%path/%Y-%m/%d/%H-%M-%S-%f
```

Written to YAML using `SetPathValue(pathName, "recordPath", recordPath)`.

The `stream_type` (e.g., `main`, `sub`) is encoded in the MediaMTX path name itself (e.g., `nvr/<camera_id>/main`), so it becomes part of the `%path` expansion automatically.

### Dynamic Rewriting

The StorageManager calls `SetPathValue()` when failing over or recovering. No new YAML writer API needed.

### Config Reload

After YAML changes, the NVR hits MediaMTX's config API to trigger a hot reload. In-progress recordings finish their current segment on the old path; the next segment uses the new path.

## Storage Status

`storage_status` is a **computed, in-memory field** added to the camera API response. It is NOT stored in the database.

Values:
- `default` — camera uses the global default storage path (no custom `storage_path` set)
- `healthy` — custom storage path is reachable
- `degraded` — custom storage path is unreachable, recording to local fallback

The StorageManager maintains the health map in memory. On NVR restart, the health monitor runs its first check synchronously during initialization, so the API always has a valid health state.

This is separate from the existing camera `status` field (which tracks stream connectivity like `online`/`disconnected`).

## API Changes

### Camera create/update — new field

```json
{
  "storage_path": "/mnt/nas1/recordings/"
}
```

**Validation:**
- Must be an absolute path or empty string (use default)
- Path must exist and be writable at time of create/update (fail fast)
- Trailing slash normalized

### New endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/storage/status` | Health status of all configured storage paths, including disk usage per path |
| GET | `/api/storage/pending` | Pending sync queue (count, per-camera breakdown) |
| POST | `/api/storage/sync/:camera_id` | Manually trigger sync for a camera's pending files |

The `GET /api/storage/status` endpoint reports disk usage (total/used/free) for each unique storage path using `syscall.Statfs`, extending the existing system stats approach to cover multiple mount points.

### Camera response — new fields

- `storage_path` — the configured path (empty = default)
- `storage_status` — `healthy`, `degraded`, `default`, or `unknown` (computed, not stored)

## Playback / HLS Serving Changes

The current `ServeSegment` handler validates that requested file paths fall under a single `RecordingsPath` prefix. With per-camera storage, files may live on different mounts.

### Approach: Serve by recording ID

Replace the path-prefix security model with a DB-backed lookup:

- `ServeSegment` accepts a recording ID (or segment reference) rather than a raw file path
- Looks up the actual `file_path` from the `recordings` table
- Serves the file directly — no prefix validation needed since the path comes from the DB, not the request
- The `segmentURLFromFilePath` helper is updated to generate URLs using recording IDs instead of relative paths
- The endpoint continues to rely on the existing JWT middleware on the NVR router for authentication

This eliminates the directory traversal concern entirely since the client never specifies a filesystem path.

### New URL format

Segment URLs change from `/api/nvr/vod/segments/RELATIVE_PATH?jwt=TOKEN` to `/api/nvr/vod/segments/<recording_id>?jwt=TOKEN`. The endpoint serves the entire file and supports HTTP Range requests for byte-range access (used by the HLS handler for fragment-level seeking).

### HLS playlist generation

When generating HLS playlists, segment URLs reference recording IDs rather than relative file paths. The HLS handler resolves each segment's actual location from the DB at playlist generation time.

## OnSegmentComplete Changes

### Camera discovery

With the new MediaMTX path naming convention (`nvr/<camera_id>/main`), camera discovery becomes deterministic:

- Parse the MediaMTX path name from the segment callback (already provided as a parameter)
- Extract `camera_id` from the path name structure: split on `/`, take the second segment
- Look up the camera by ID directly (O(1) DB lookup instead of iterating all cameras)
- Fallback: existing `MediaMTXPath` substring match for recordings created before migration

### Pending sync detection

After inserting the recording into the DB, compare the file's actual location against the camera's configured `storage_path`. If the file is in the local fallback location (i.e., `storage_path` is set but the file is under `./recordings/`), insert a `pending_syncs` row automatically.

## Record Cleaner Interaction

The MediaMTX-native record cleaner uses `RecordPath` from the currently active path config to walk directories. After a failover/recovery cycle, it only sees the current config path.

The NVR's own retention system (the scheduler) uses DB queries on `recordings.start_time` and deletes files by their stored `file_path`. Since `file_path` in the DB always reflects the file's actual current location (updated after sync), the NVR retention system correctly handles files regardless of which storage path they're on.

For the edge case of orphaned local fallback files (sync failed, then recording deleted by retention), the StorageManager's orphan sweep handles cleanup (see Sync Worker section).

## Changing a Camera's Storage Path

Changing a camera's `storage_path` after recordings exist applies only to **new recordings**. Existing recordings remain at their stored `file_path` in the DB. The playback system and retention system both use the DB `file_path`, so they continue to work for old recordings regardless of the config change.

The API documents this behavior: "Changing storage path affects new recordings only. Existing recordings are not moved."

## Flutter UI Changes

### Camera Detail — Storage tab

New tab alongside General, Recording, AI, Zones, Advanced:

- **Storage Path** — text field with folder picker. Placeholder shows global default. Helper text: "Leave empty to use default local storage"
- **Storage Status** — indicator chip: `Healthy` (green), `Degraded` (amber), `Default` (gray)
- **Pending Syncs** — count of files awaiting sync, with "Sync Now" button when > 0

### Settings/Dashboard — Storage Overview

- Table of all unique storage paths, their health status, camera count per path, and disk usage (total/used/free)
- Global pending sync count
- Alert banner when any storage path is degraded

## Migration Strategy

### MediaMTX path name migration

Existing cameras have `mediamtx_path` like `nvr/backyard`. The new convention is `nvr/<camera_id>/main`. A DB migration:

1. For each camera, update `mediamtx_path` from `nvr/<name>` to `nvr/<camera_id>/main`
2. Update the YAML config to rename the path entry accordingly
3. Existing recording files on disk are NOT moved — their `file_path` in the DB still points to the correct location

**Recovery from partial migration:** If the process crashes between the DB update (step 1) and YAML rename (step 2), the DB and YAML will be out of sync. On startup, the NVR should verify that every camera's `mediamtx_path` exists in the YAML and recreate missing entries.

### New recordings

New directory structure applies only to recordings created after migration. The playback system serves files by DB `file_path`, so old and new recordings coexist without issues.

### No retroactive file moves

Existing recordings remain in their original locations. The NVR retention system will eventually age them out.

## Key Files

| Component | File |
|-----------|------|
| Camera schema | `internal/nvr/db/cameras.go` |
| DB migrations | `internal/nvr/db/migrations.go` |
| Camera API | `internal/nvr/api/cameras.go` |
| StorageManager | `internal/nvr/storage/manager.go` (new) |
| YAML writer | `internal/nvr/yamlwriter/writer.go` |
| OnSegmentComplete | `internal/nvr/nvr.go` |
| Record path config | `internal/conf/path.go` |
| HLS handler | `internal/nvr/api/hls.go` |
| Flutter camera model | `clients/flutter/lib/models/camera.dart` |
| Flutter camera detail | `clients/flutter/lib/screens/cameras/camera_detail_screen.dart` |
