# Legacy NVR API Completion — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement all remaining stubbed endpoints in `internal/directory/legacynvrapi/handlers.go` so the Flutter client has full functionality after the NVR decomposition.

**Architecture:** The `legacynvrapi` handlers package needs access to both the Directory DB (cameras, users, groups, schedules) and the Recorder DB (recordings, bookmarks, detections, exports, screenshots). In all-in-one mode, both databases are local. The `Handlers` struct gains a second DB field for the recorder database. Each endpoint delegates to existing DB CRUD methods — no new business logic.

**Tech Stack:** Go, SQLite, `net/http`, `encoding/json`

---

## File Map

| File | Change | What |
|------|--------|------|
| `internal/directory/legacynvrapi/handlers.go` | Modify | Add RecDB field, implement all stubbed endpoints |
| `internal/directory/legacynvrapi/recordings.go` | Create | Recordings, timeline, stats, HLS/VOD handlers |
| `internal/directory/legacynvrapi/bookmarks.go` | Create | Bookmarks CRUD + search |
| `internal/directory/legacynvrapi/exports.go` | Create | Export jobs CRUD + download |
| `internal/directory/legacynvrapi/detections.go` | Create | Detection zones, events, search, tracks |
| `internal/directory/legacynvrapi/screenshots.go` | Create | Screenshots list + delete |
| `internal/directory/legacynvrapi/tours.go` | Create | Tours CRUD |
| `internal/directory/legacynvrapi/onvif.go` | Create | ONVIF discovery, device info, PTZ, media profiles |
| `internal/directory/legacynvrapi/system.go` | Create | Storage, metrics, backup handlers |
| `internal/directory/legacynvrapi/audit.go` | Create | Audit log handler |
| `internal/directory/legacynvrapi/recording_rules.go` | Create | Recording rules CRUD per camera |
| `internal/directory/legacynvrapi/auth_extra.go` | Create | Password change handler |
| `internal/directory/boot.go` | Modify | Pass recorder DB to legacynvrapi Handlers |
| `internal/directory/legacynvrapi/handlers_test.go` | Create | Integration tests for key endpoints |

---

## Task 1: Add Recorder DB to Handlers struct and wire in boot.go

**Files:**
- Modify: `internal/directory/legacynvrapi/handlers.go`
- Modify: `internal/directory/boot.go`

- [ ] **Step 1: Add RecDB field to Handlers**

In `handlers.go`, update the struct:

```go
import (
    dirdb "github.com/bluenviron/mediamtx/internal/directory/db"
    recdb "github.com/bluenviron/mediamtx/internal/recorder/db"
)

type Handlers struct {
    DB    *dirdb.DB  // Directory DB (cameras, users, groups, schedules)
    RecDB *recdb.DB  // Recorder DB (recordings, bookmarks, detections, exports)
}
```

- [ ] **Step 2: Wire RecDB in boot.go**

In `internal/directory/boot.go`, where `nvrCompat` is created (~line 500), open the recorder DB and pass it:

```go
// Open recorder DB for legacy API compatibility
recDBPath := filepath.Join(cfg.DataDir, "recorder.db")
rdb, recDBErr := recdb.Open(ctx, recDBPath)
if recDBErr != nil {
    log.Warn("directory: recorder db not available for legacy API", "error", recDBErr)
}

nvrCompat := &legacynvrapi.Handlers{DB: nDB, RecDB: rdb}
nvrCompat.Register(mux)
```

- [ ] **Step 3: Verify build**

```bash
go build ./internal/directory/...
```

- [ ] **Step 4: Commit**

```bash
git add internal/directory/legacynvrapi/handlers.go internal/directory/boot.go
git commit -m "feat: add RecDB to legacynvrapi Handlers for recorder-owned endpoints"
```

---

## Task 2: Implement recordings and timeline endpoints

**Files:**
- Create: `internal/directory/legacynvrapi/recordings.go`

Implement using `recdb.DB` methods:

| Endpoint | Method | DB Call |
|----------|--------|--------|
| `GET /api/nvr/recordings` | GET | `RecDB.QueryRecordings(filter)` |
| `GET /api/nvr/recordings/stats` | GET | `RecDB.GetStoragePerCamera()` |
| `GET /api/nvr/vod/{cameraId}/playlist.m3u8` | GET | Proxy to playback server on :9996 |
| `GET /api/nvr/timeline/multi` | GET | `RecDB.GetTimeline(cameraID, start, end)` per camera |
| `GET /api/nvr/timeline/intensity` | GET | `RecDB.QueryMotionEventsByClass(cameraID, start, end, bucketSize)` |
| `GET /api/nvr/cameras/{id}/detections` | GET | `RecDB.QueryDetectionsByTimeRange(cameraID, start, end)` |

- [ ] **Step 1: Create `recordings.go` with handlers**

```go
package legacynvrapi

func (h *Handlers) recordingsList(w http.ResponseWriter, r *http.Request) {
    if h.RecDB == nil {
        h.notImplemented(w, r)
        return
    }
    cameraID := r.URL.Query().Get("camera_id")
    recs, err := h.RecDB.QueryRecordings(cameraID, "", "", 100, 0)
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"items": recs})
}

func (h *Handlers) recordingsStats(w http.ResponseWriter, r *http.Request) {
    if h.RecDB == nil {
        h.notImplemented(w, r)
        return
    }
    stats, err := h.RecDB.GetStoragePerCamera()
    if err != nil {
        writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
        return
    }
    writeJSON(w, http.StatusOK, map[string]any{"items": stats})
}
```

Implement all 6 endpoints. Parse query parameters (`camera_id`, `start`, `end`, `limit`, `offset`) from the URL.

- [ ] **Step 2: Register routes in handlers.go**

Replace the `notImplemented` stubs for recordings:

```go
mux.HandleFunc("/api/nvr/recordings", h.recordingsList)
mux.HandleFunc("/api/nvr/recordings/", h.recordingsSubrouter)
mux.HandleFunc("/api/nvr/recordings/stats", h.recordingsStats)
```

- [ ] **Step 3: Build and commit**

```bash
go build ./internal/directory/...
git add internal/directory/legacynvrapi/
git commit -m "feat: implement recordings, timeline, and stats endpoints"
```

---

## Task 3: Implement bookmarks endpoints

**Files:**
- Create: `internal/directory/legacynvrapi/bookmarks.go`

| Endpoint | Method | DB Call |
|----------|--------|--------|
| `GET /api/nvr/bookmarks` | GET | `RecDB.GetBookmarks(cameraID)` or `SearchBookmarks(query)` |
| `POST /api/nvr/bookmarks` | POST | `RecDB.InsertBookmark(bookmark)` |
| `DELETE /api/nvr/bookmarks/{id}` | DELETE | `RecDB.DeleteBookmark(id)` |

- [ ] **Step 1: Create `bookmarks.go`**

Implement list (with `camera_id` and `date` query params), create (JSON body with camera_id, timestamp, label, notes), and delete.

- [ ] **Step 2: Register routes and commit**

```bash
git commit -m "feat: implement bookmarks CRUD endpoints"
```

---

## Task 4: Implement export endpoints

**Files:**
- Create: `internal/directory/legacynvrapi/exports.go`

| Endpoint | Method | DB Call |
|----------|--------|--------|
| `POST /api/nvr/exports` | POST | `RecDB.CreateExportJob(job)` |
| `GET /api/nvr/exports/{id}` | GET | `RecDB.GetExportJob(id)` |
| `DELETE /api/nvr/exports/{id}` | DELETE | `RecDB.DeleteExportJob(id)` |
| `GET /api/nvr/exports/{id}/download` | GET | Serve file from `job.FilePath` |

- [ ] **Step 1: Create `exports.go`**

For the download endpoint, check `job.Status == "completed"`, then serve the file with `http.ServeFile`.

- [ ] **Step 2: Register routes and commit**

```bash
git commit -m "feat: implement export job CRUD and download endpoints"
```

---

## Task 5: Implement detection and tracking endpoints

**Files:**
- Create: `internal/directory/legacynvrapi/detections.go`

| Endpoint | Method | DB Call |
|----------|--------|--------|
| `GET /api/nvr/search` | GET | `RecDB.QueryDetectionsByTimeRange(cameraID, start, end)` |
| `GET /api/nvr/cameras/{id}/zones` | GET | `RecDB.ListDetectionZones(cameraID)` |
| `POST /api/nvr/cameras/{id}/zones` | POST | `RecDB.CreateDetectionZone(zone)` |
| `PUT /api/nvr/zones/{id}` | PUT | `RecDB.UpdateDetectionZone(zone)` |
| `DELETE /api/nvr/zones/{id}` | DELETE | `RecDB.DeleteDetectionZone(id)` |
| `GET /api/nvr/cameras/{id}/recording-rules` | GET | `RecDB.ListRecordingRules(cameraID)` |
| `POST /api/nvr/cameras/{id}/recording-rules` | POST | `RecDB.CreateRecordingRule(rule)` |
| `PUT /api/nvr/recording-rules/{id}` | PUT | `RecDB.UpdateRecordingRule(rule)` |
| `DELETE /api/nvr/recording-rules/{id}` | DELETE | `RecDB.DeleteRecordingRule(id)` |
| `POST /api/nvr/detections/{id}/track` | POST | `RecDB.InsertTrack(track)` + `InsertSighting(sighting)` |
| `GET /api/nvr/tracks/{id}` | GET | `RecDB.GetTrackWithSightings(id)` |
| `GET /api/nvr/tracks` | GET | `RecDB.ListTracks(limit)` |

- [ ] **Step 1: Create `detections.go`**

Implement all detection, zone, recording-rule, and tracking endpoints. Parse path segments for camera IDs and resource IDs.

- [ ] **Step 2: Register routes in handlers.go**

Add route patterns for zones, recording-rules, tracks.

- [ ] **Step 3: Build and commit**

```bash
git commit -m "feat: implement detection zones, recording rules, and tracking endpoints"
```

---

## Task 6: Implement screenshots endpoints

**Files:**
- Create: `internal/directory/legacynvrapi/screenshots.go`

| Endpoint | Method | DB Call |
|----------|--------|--------|
| `GET /api/nvr/screenshots` | GET | `RecDB.ListScreenshots(cameraID, limit, offset)` |
| `DELETE /api/nvr/screenshots/{id}` | DELETE | `RecDB.DeleteScreenshot(id)` |
| `POST /api/nvr/cameras/{id}/screenshot` | POST | Capture via ONVIF snapshot or frame grab |

- [ ] **Step 1: Create `screenshots.go`**

For the capture endpoint, if ONVIF snapshot URI is available on the camera, fetch and store it. Otherwise return NOT_IMPLEMENTED.

- [ ] **Step 2: Register routes and commit**

```bash
git commit -m "feat: implement screenshots list, delete, and capture endpoints"
```

---

## Task 7: Implement tours endpoints

**Files:**
- Create: `internal/directory/legacynvrapi/tours.go`

| Endpoint | Method | DB Call |
|----------|--------|--------|
| `GET /api/nvr/tours` | GET | `RecDB.ListTours()` |
| `POST /api/nvr/tours` | POST | `RecDB.CreateTour(tour)` |
| `PUT /api/nvr/tours/{id}` | PUT | `RecDB.UpdateTour(tour)` |
| `DELETE /api/nvr/tours/{id}` | DELETE | `RecDB.DeleteTour(id)` |

- [ ] **Step 1: Create `tours.go` with all 4 handlers**
- [ ] **Step 2: Register routes and commit**

```bash
git commit -m "feat: implement tours CRUD endpoints"
```

---

## Task 8: Implement ONVIF device management endpoints

**Files:**
- Create: `internal/directory/legacynvrapi/onvif.go`

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `POST /api/nvr/cameras/discover` | POST | Start ONVIF WS-Discovery scan |
| `GET /api/nvr/cameras/discover/results` | GET | Poll discovery results |
| `POST /api/nvr/cameras/probe` | POST | Probe camera at given endpoint |
| `GET /api/nvr/cameras/{id}/device-info` | GET | Get ONVIF device info |
| `GET /api/nvr/cameras/{id}/settings` | GET | Get imaging settings |
| `PUT /api/nvr/cameras/{id}/settings` | PUT | Set imaging settings |
| `GET /api/nvr/cameras/{id}/relay-outputs` | GET | Get relay outputs |
| `GET /api/nvr/cameras/{id}/ptz/presets` | GET | List PTZ presets |
| `GET /api/nvr/cameras/{id}/ptz/status` | GET | Get PTZ status |
| `POST /api/nvr/cameras/{id}/ptz` | POST | Move/zoom camera |
| `GET /api/nvr/cameras/{id}/audio/capabilities` | GET | Get audio capabilities |
| `GET /api/nvr/cameras/{id}/media/profiles` | GET | List media profiles |
| `GET /api/nvr/cameras/{id}/media/video-sources` | GET | List video sources |
| `GET /api/nvr/cameras/{id}/device/datetime` | GET | Get device datetime |
| `GET /api/nvr/cameras/{id}/device/hostname` | GET | Get hostname |
| `GET /api/nvr/cameras/{id}/device/network/interfaces` | GET | List interfaces |
| `GET /api/nvr/cameras/{id}/device/network/protocols` | GET | List protocols |
| `GET /api/nvr/cameras/{id}/device/users` | GET | List device users |

These call functions in `internal/recorder/onvif/` package. The Handlers struct needs an ONVIF client or the camera's ONVIF endpoint from the DB.

- [ ] **Step 1: Create `onvif.go`**

For each endpoint:
1. Look up camera from DB to get ONVIF endpoint + credentials
2. Call the corresponding `onvif.` function
3. Return the result as JSON

Add an `ONVIFClient` interface or import `internal/recorder/onvif` directly.

For ONVIF discovery:
1. `POST /cameras/discover` — start async scan, store results in-memory (use a sync.Map or channel)
2. `GET /cameras/discover/results` — return accumulated results

- [ ] **Step 2: Add ONVIF discovery state to Handlers struct**

```go
type Handlers struct {
    DB    *dirdb.DB
    RecDB *recdb.DB
    discoveryResults sync.Map  // or a typed struct
}
```

- [ ] **Step 3: Register routes in camerasSubrouter**

Update the `camerasSubrouter` to handle the device sub-resources (`device-info`, `settings`, `ptz`, `audio`, `media`, `device/*`).

- [ ] **Step 4: Build and commit**

```bash
git commit -m "feat: implement ONVIF device management and discovery endpoints"
```

---

## Task 9: Implement system storage, metrics, and backup endpoints

**Files:**
- Create: `internal/directory/legacynvrapi/system.go`

| Endpoint | Method | Implementation |
|----------|--------|---------------|
| `GET /api/nvr/system/storage` | GET | `RecDB.GetTotalStorageUsage()` + `RecDB.GetStoragePerCamera()` + disk stats |
| `GET /api/nvr/system/metrics` | GET | Runtime metrics (goroutines, memory, CPU) |
| `POST /api/nvr/system/backup` | POST | Create SQLite backup (`.backup` command) |
| `GET /api/nvr/system/backups` | GET | List backup files in data directory |

- [ ] **Step 1: Create `system.go`**

For storage: use `RecDB.GetTotalStorageUsage()` and `os.Stat` on the data directory for disk space.
For metrics: use `runtime.NumGoroutine()`, `runtime.MemStats`, etc.
For backup: use SQLite backup API or file copy.
For backups list: scan the data directory for `.db.backup` files.

- [ ] **Step 2: Register routes and commit**

```bash
git commit -m "feat: implement system storage, metrics, and backup endpoints"
```

---

## Task 10: Implement audit log and auth extras

**Files:**
- Create: `internal/directory/legacynvrapi/audit.go`
- Create: `internal/directory/legacynvrapi/auth_extra.go`

| Endpoint | Method | DB Call |
|----------|--------|--------|
| `GET /api/nvr/audit` | GET | Query `audit_entries` table from directory DB |
| `PUT /api/nvr/auth/password` | PUT | Verify old password, hash new password, update DB |

- [ ] **Step 1: Create `audit.go`**

Check if `ListAuditEntries()` exists in directory/db. If not, add it — it's a simple `SELECT * FROM audit_entries ORDER BY created_at DESC LIMIT ?`.

- [ ] **Step 2: Create `auth_extra.go`**

Password change: verify current password (bcrypt/argon2), hash new password with bcrypt, update user record.

- [ ] **Step 3: Register routes and commit**

```bash
git commit -m "feat: implement audit log and password change endpoints"
```

---

## Task 11: Implement remaining camera group and schedule template sub-routes

**Files:**
- Modify: `internal/directory/legacynvrapi/handlers.go`

| Endpoint | Method | DB Call |
|----------|--------|--------|
| `PUT /api/nvr/camera-groups/{id}` | PUT | `DB.UpdateGroup(group)` |
| `DELETE /api/nvr/camera-groups/{id}` | DELETE | `DB.DeleteGroup(id)` |
| `POST /api/nvr/schedule-templates` | POST | `DB.CreateScheduleTemplate(template)` |
| `PUT /api/nvr/schedule-templates/{id}` | PUT | `DB.UpdateScheduleTemplate(template)` |
| `DELETE /api/nvr/schedule-templates/{id}` | DELETE | `DB.DeleteScheduleTemplate(id)` |
| `GET /api/nvr/saved-clips` | GET | `RecDB.ListSavedClips()` |
| `GET /api/nvr/cameras/{id}/refresh` | POST | ONVIF re-probe capabilities |
| `GET /api/nvr/cameras/{id}/storage-estimate` | GET | `RecDB.GetCameraStorageUsage(id)` |

- [ ] **Step 1: Implement remaining group/template sub-routes**

Update the existing `cameraGroups` handler and add a `cameraGroupsSubrouter`. Same for schedule templates.

- [ ] **Step 2: Implement saved-clips and camera sub-resource endpoints**
- [ ] **Step 3: Register routes and commit**

```bash
git commit -m "feat: implement camera groups/schedule templates sub-routes and saved clips"
```

---

## Task 12: Remove all notImplemented stubs and verify

**Files:**
- Modify: `internal/directory/legacynvrapi/handlers.go`

- [ ] **Step 1: Grep for remaining notImplemented calls**

```bash
grep -n 'notImplemented' internal/directory/legacynvrapi/*.go
```

Every endpoint should now have a real handler. Any remaining stubs should be documented with a comment explaining why (e.g., "requires external service not available in directory-only mode").

- [ ] **Step 2: Full build and test**

```bash
go build ./...
go test ./internal/directory/... -count=1
```

- [ ] **Step 3: Manual verification with curl**

Test the top 10 most-used endpoints:

```bash
# Cameras
curl -s http://localhost:9995/api/nvr/cameras | python3 -m json.tool

# Recordings
curl -s http://localhost:9995/api/nvr/recordings | python3 -m json.tool

# Bookmarks
curl -s http://localhost:9995/api/nvr/bookmarks | python3 -m json.tool

# System info
curl -s http://localhost:9995/api/nvr/system/info | python3 -m json.tool

# System storage
curl -s http://localhost:9995/api/nvr/system/storage | python3 -m json.tool

# Screenshots
curl -s http://localhost:9995/api/nvr/screenshots | python3 -m json.tool

# Tours
curl -s http://localhost:9995/api/nvr/tours | python3 -m json.tool

# Audit log
curl -s http://localhost:9995/api/nvr/audit | python3 -m json.tool

# Schedule templates
curl -s http://localhost:9995/api/nvr/schedule-templates | python3 -m json.tool

# Camera groups
curl -s http://localhost:9995/api/nvr/camera-groups | python3 -m json.tool
```

All should return 200 with proper JSON (empty arrays are fine for empty databases).

- [ ] **Step 4: Final commit**

```bash
git commit -m "chore: remove all notImplemented stubs — full Flutter API coverage"
```
