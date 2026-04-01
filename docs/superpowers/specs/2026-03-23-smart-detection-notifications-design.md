# Smart Detection Notifications Design

## Goal

Replace the current frame-level class-counting notification system with a production-grade detection pipeline that tracks individual objects across frames, evaluates zone boundaries, manages per-zone per-class cooldowns, and fires notifications only on meaningful state transitions (entered, loitering, left).

## Architecture

Four tightly coupled subsystems compose the pipeline, executed in order each frame:

```
YOLO Detect → ByteTrack → Zone Assignment → State Machine → Cooldown Gate → Notify
```

All processing remains in-process within the Go binary. No external services or additional neural networks required. The existing `AIPipeline.ProcessFrame()` orchestrates the flow; each subsystem is a separate, testable unit.

**Concurrency model:** Each camera has its own `AIPipeline` instance running in a dedicated goroutine. The `ByteTracker`, `CooldownManager`, and zone state are per-pipeline and never accessed concurrently. The only cross-goroutine communication is zone cache invalidation (see Section 6).

## Tech Stack

- **Go** (pure, no CGO beyond existing ONNX Runtime)
- **ByteTrack** algorithm (pure Go, no model needed)
- **Kalman filter** (pure Go, simple 2D constant-velocity model)
- **SQLite** (new migration v15 for zones and alert rules tables)
- **React + TypeScript + Tailwind** (zone editor UI, updated notifications)

---

## 1. Object Tracking (ByteTrack)

### File: `internal/nvr/ai/tracker.go`

### Types

```go
type Track struct {
    ID         int
    Class      string
    Confidence float32
    BBox       [4]float32          // Kalman-filtered position: x, y, w, h (normalized 0-1)
    Kalman     KalmanState         // position + velocity state
    Lost       int                 // frames since last matched
    ZoneStates map[int64]ZoneState // zone ID -> state machine
}

// KalmanState uses the standard ByteTrack formulation: [cx, cy, area, aspect, dx, dy, da].
// Conversion: cx = x + w/2, cy = y + h/2, area = w*h, aspect = w/h.
// Track.BBox is derived from Kalman state after each predict/update step.
type KalmanState struct {
    X  [7]float64    // state vector: cx, cy, area, aspect, dx, dy, da
    P  [7][7]float64 // covariance matrix
}

type TrackedDetection struct {
    YOLODetection          // raw YOLO measurement (X, Y, W, H fields)
    TrackID       int      // assigned by ByteTracker
}

type ByteTracker struct {
    tracks     []*Track
    nextID     int
    maxLost    int     // default 30 (15s at 2 FPS)
    highThresh float32 // default 0.5
    lowThresh  float32 // default 0.3
    iouThresh  float32 // default 0.3
}
```

### YOLO Confidence Threshold Change

The existing `AIPipeline.confThreshold` must be lowered from **0.5 to 0.3** so that YOLO returns both high and low-confidence detections. ByteTrack then splits them:

- High confidence (>0.5): first-pass matching
- Low confidence (0.3-0.5): second-pass recovery of occluded objects

Without this change, ByteTrack's second pass would receive zero detections.

### Algorithm

Uses **greedy matching** (not Hungarian) for simplicity. Each frame:

1. Kalman predict step runs for all active tracks.
2. Split YOLO detections into high-confidence (>0.5) and low-confidence (0.3-0.5).
3. **First pass:** Compute IoU between predicted track positions and high-confidence detections. Greedily match pairs above IoU threshold 0.3 (highest IoU first). Matched pairs update their tracks via Kalman update.
4. **Second pass:** Unmatched tracks from pass 1 are matched against low-confidence detections using the same greedy IoU method. This recovers partially occluded objects.
5. Remaining unmatched detections become new tracks with fresh IDs.
6. Remaining unmatched tracks increment `Lost`. Tracks with `Lost > maxLost` are deleted.

### Interface

```go
func NewByteTracker() *ByteTracker
func (bt *ByteTracker) Update(detections []YOLODetection) []TrackedDetection
func (bt *ByteTracker) ActiveTracks() []*Track
```

### Integration

`AIPipeline.ProcessFrame()` calls `tracker.Update(detections)` immediately after YOLO inference. All downstream logic (zone assignment, state transitions, DB storage, notifications) operates on `TrackedDetection` values which carry a `TrackID`.

---

## 2. Detection Zones

### Files

- `internal/nvr/ai/zone.go` — zone model, point-in-polygon
- `internal/nvr/db/zones.go` — DB CRUD
- `internal/nvr/db/migrations.go` — v15 migration
- `internal/nvr/api/zones.go` — REST endpoints
- `ui/src/components/ZoneEditor.tsx` — polygon drawing UI

### Data Model

```sql
-- Migration v15
CREATE TABLE detection_zones (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL,
    polygon TEXT NOT NULL,  -- JSON: [[x1,y1],[x2,y2],...]
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);
CREATE INDEX idx_zones_camera ON detection_zones(camera_id);

CREATE TABLE zone_alert_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    zone_id INTEGER NOT NULL,
    class_name TEXT NOT NULL,        -- "person", "car", or "*"
    enabled INTEGER NOT NULL DEFAULT 1,
    cooldown_seconds INTEGER NOT NULL DEFAULT 30,
    loiter_seconds INTEGER NOT NULL DEFAULT 0,  -- 0 = disabled
    notify_on_enter INTEGER NOT NULL DEFAULT 1,
    notify_on_leave INTEGER NOT NULL DEFAULT 0,
    notify_on_loiter INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (zone_id) REFERENCES detection_zones(id) ON DELETE CASCADE,
    UNIQUE(zone_id, class_name)
);
```

### Default Zone Behavior

When a camera has no zones configured, the pipeline synthesizes an implicit full-frame zone at runtime:

```go
func implicitFullFrameZone(cameraID string) Zone {
    return Zone{
        ID:       -1, // sentinel
        CameraID: cameraID,
        Name:     "Full Frame",
        Polygon:  [][2]float64{{0,0},{1,0},{1,1},{0,1}},
        Enabled:  true,
    }
}
```

The implicit zone uses default alert rules:

- person: cooldown 30s, notify_on_enter=true, notify_on_leave=false, notify_on_loiter=false
- vehicle: cooldown 60s, notify_on_enter=true, notify_on_leave=false, notify_on_loiter=false
- animal: cooldown 60s, notify_on_enter=true, notify_on_leave=false, notify_on_loiter=false

When the user creates their first explicit zone, the implicit zone is no longer used.

### Point-in-Polygon

Ray-casting algorithm. Test is against the center point of the detection bounding box:

```go
func PointInPolygon(px, py float64, polygon [][2]float64) bool
```

### Zone API

All zone endpoints are registered under the authenticated route group.

| Method | Path                    | Description                                  |
| ------ | ----------------------- | -------------------------------------------- |
| GET    | `/cameras/:id/zones`    | List zones + rules for camera                |
| POST   | `/cameras/:id/zones`    | Create zone with polygon and optional rules  |
| PUT    | `/zones/:id`            | Update zone polygon, name, or rules          |
| DELETE | `/zones/:id`            | Delete zone and its rules                    |
| GET    | `/cameras/:id/snapshot` | Proxy camera's snapshot URI with digest auth |

### Snapshot Endpoint

`GET /cameras/:id/snapshot` proxies the camera's ONVIF snapshot URI (stored as `snapshot_uri` in the cameras table). The server fetches the JPEG using the camera's stored credentials with digest auth (same mechanism as `AIPipeline.captureAndDecode()`), then returns the image to the browser. This avoids exposing camera credentials to the frontend.

### Zone Editor UI

- Accessed from Camera Management page, per-camera "Zones" button
- Fetches snapshot from `/cameras/:id/snapshot`, renders as background
- Canvas overlay for polygon drawing (click to add points, double-click to close)
- Existing zones shown as colored semi-transparent overlays with labels
- Sidebar: zone list, per-zone config panel with class toggles, cooldown sliders (0-300s), loiter threshold slider (0-300s), enter/leave/loiter notification toggles
- Default rules auto-created on zone creation: person (30s cooldown, enter only), vehicle (60s cooldown, enter only), animal (60s cooldown, enter only)

---

## 3. State Transitions

### File: `internal/nvr/ai/state.go`

### States

```go
type ObjectState int
const (
    StateOutside   ObjectState = iota // not inside this zone
    StateInside                       // inside zone, monitoring for loiter
    StateLoitering                    // inside zone past loiter threshold
)
```

Three states only. "Entered" and "left" are transition events (actions), not persistent states.

### Per-Track, Per-Zone State

Each `Track` carries `ZoneStates map[int64]ZoneState`:

```go
type ZoneState struct {
    State          ObjectState
    EnteredAt      time.Time
    LoiterNotified bool // whether loiter notification already sent for this visit
}
```

### Transition Logic

Runs once per frame after zone assignment:

```
For each active track:
  For each zone (explicit or implicit full-frame):
    inZone = PointInPolygon(track.centerX, track.centerY, zone.polygon)
    state  = track.ZoneStates[zone.ID]

    CASE state == Outside && inZone:
        → state = Inside, record enteredAt = now
        → emit "entered" event for this track/zone/class

    CASE state == Inside && inZone:
        → if zone.loiterThreshold > 0 && now - enteredAt > threshold && !loiterNotified:
            → state = Loitering, loiterNotified = true
            → emit "loitering" event

    CASE (state == Inside || state == Loitering) && !inZone:
        → emit "left" event
        → state = Outside, clear enteredAt, loiterNotified = false

    CASE state == Outside && !inZone:
        → no-op
```

### Emitted Events

State transitions produce internal notification requests (not yet filtered by cooldown):

```go
type NotificationRequest struct {
    TrackID    int
    ZoneID     int64
    ZoneName   string
    Class      string
    Action     string  // "entered", "loitering", "left"
    Confidence float32
    Timestamp  time.Time
}
```

These are passed to the CooldownManager before reaching the EventBroadcaster.

---

## 4. Cooldown Timers

### File: `internal/nvr/ai/cooldown.go`

Each `AIPipeline` instance owns its own `CooldownManager`. There is no shared/global cooldown state. Since each pipeline runs in a single goroutine, no mutex is needed.

### Key Design

```go
type cooldownKey struct {
    ZoneID    int64
    ClassName string
    Action    string // "entered", "left"
}

type CooldownManager struct {
    lastNotified map[cooldownKey]time.Time
}
```

### Logic

```go
func (cm *CooldownManager) ShouldNotify(req NotificationRequest, rule ZoneAlertRule) bool {
    // Check if this action type is enabled for this zone
    if req.Action == "entered" && !rule.NotifyOnEnter { return false }
    if req.Action == "left" && !rule.NotifyOnLeave { return false }
    if req.Action == "loitering" && !rule.NotifyOnLoiter { return false }

    // Loitering is already time-gated by its threshold; always allow
    if req.Action == "loitering" { return true }

    key := cooldownKey{req.ZoneID, req.Class, req.Action}
    last, exists := cm.lastNotified[key]
    if exists && time.Since(last) < rule.CooldownDuration() {
        return false
    }
    cm.lastNotified[key] = req.Timestamp
    return true
}
```

### Defaults

| Class Category                                       | Cooldown | Loiter Threshold |
| ---------------------------------------------------- | -------- | ---------------- |
| person                                               | 30s      | 0 (disabled)     |
| vehicle (car, truck, bus, motorcycle, bicycle, boat) | 60s      | 0 (disabled)     |
| animal (cat, dog, horse, etc.)                       | 60s      | 0 (disabled)     |

### Garbage Collection

Every 60 seconds, remove entries from `lastNotified` older than 10 minutes. The cleanup runs inside the pipeline's `Run()` loop (ticker alongside the frame capture ticker) and stops when `stopCh` is closed.

---

## 5. Updated Event Model & Frontend

### Backend Event Changes

```go
type Event struct {
    Type       string  `json:"type"`
    Camera     string  `json:"camera"`
    Message    string  `json:"message"`
    Time       string  `json:"time"`
    Zone       string  `json:"zone,omitempty"`
    Class      string  `json:"class,omitempty"`
    Action     string  `json:"action,omitempty"`
    TrackID    int     `json:"track_id,omitempty"`
    Confidence float32 `json:"confidence,omitempty"`
}
```

### Updated EventPublisher Interface

The `EventPublisher` interface used by `AIPipeline` changes to:

```go
type EventPublisher interface {
    PublishTrackedDetection(camera, zone, class, action string, trackID int, confidence float32)
}
```

`EventBroadcaster` implements this:

```go
func (b *EventBroadcaster) PublishTrackedDetection(camera, zone, class, action string, trackID int, confidence float32) {
    label := strings.Title(class)
    var msg string
    switch action {
    case "entered":
        msg = fmt.Sprintf("%s entered %s (%0.f%%)", label, zone, confidence*100)
    case "loitering":
        msg = fmt.Sprintf("%s loitering in %s", label, zone)
    case "left":
        msg = fmt.Sprintf("%s left %s", label, zone)
    }
    b.Publish(Event{
        Type: "ai_detection", Camera: camera, Message: msg,
        Zone: zone, Class: class, Action: action,
        TrackID: trackID, Confidence: confidence,
    })
}
```

### Frontend Notification Updates (`useNotifications.ts`)

Updated `Notification` TypeScript interface:

```typescript
export interface Notification {
  id: string;
  type:
    | "motion"
    | "ai_detection"
    | "camera_offline"
    | "camera_online"
    | "recording_started"
    | "recording_stopped";
  camera: string;
  message: string;
  time: Date;
  read: boolean;
  // Structured AI fields (optional, present for ai_detection events)
  zone?: string;
  className?: string;
  action?: string; // "entered", "loitering", "left"
  trackId?: number;
  confidence?: number;
}
```

WebSocket `onmessage` handler extracts the new fields from the JSON payload:

```typescript
const notif: Notification = {
  id: crypto.randomUUID(),
  type: data.type,
  camera: data.camera,
  message: data.message,
  time: new Date(data.time),
  read: false,
  zone: data.zone,
  className: data.class,
  action: data.action,
  trackId: data.track_id,
  confidence: data.confidence,
};
```

- Title derived from action: "Person Entered", "Person Loitering", "Car Left"
- Subtitle shows zone name and camera: "Driveway - AD410"
- Toast severity: entered=warning (amber), loitering=error (red), left=info (blue)

### AnalyticsOverlay Updates

- Bounding boxes labeled with track ID: "Person #4 87%"
- Zone polygons rendered as semi-transparent colored overlays on live view (fetched from `/cameras/:id/zones`)
- Box color changes when track is loitering (amber -> red)

### Zone Editor Component (`ZoneEditor.tsx`)

- Snapshot-based polygon drawing with click-to-add-point, double-click-to-close
- Existing zones shown as colored overlays with labels
- Per-zone sidebar: name, class toggles (person/vehicle/animal), cooldown slider (0-300s), loiter threshold slider (0-300s), enter/leave/loiter notification toggles
- Default rules auto-populated on zone creation

---

## 6. Pipeline Integration

### Updated `ProcessFrame()` Flow

```
1. YOLO Detect(img, 0.3) → []YOLODetection  (lowered from 0.5)
2. ByteTracker.Update(detections) → []TrackedDetection
3. Load zones for camera (cached)
4. For each tracked detection:
   a. For each zone: PointInPolygon test
   b. Run state transition logic on track.ZoneStates[zone.ID]
   c. Collect NotificationRequests for transitions
5. For each NotificationRequest:
   a. Look up ZoneAlertRule for zone + class
   b. CooldownManager.ShouldNotify() gate
   c. If passes: PublishTrackedDetection()
6. Store detections in DB (with track_id)
7. Store/update motion event
```

### Zone Cache Invalidation

Each `AIPipeline` caches its camera's zones in memory. Zones are reloaded from the DB every **5 seconds** via a ticker in the `Run()` loop. This avoids cross-goroutine channels while keeping the staleness window small (a user creates a zone, it takes at most 5 seconds for the pipeline to pick it up). The reload is a lightweight single-row query per camera.

### What Gets Removed

- `prevClassCounts` field and count-comparison logic
- Current `ensureMotionEvent()` notification dispatch (replaced by state machine)
- `confThreshold` lowered from 0.5 to 0.3
- Motion event creation/closure still happens but is decoupled from notifications

### What Gets Added

- `tracker *ByteTracker` field on AIPipeline
- `cooldowns *CooldownManager` field on AIPipeline
- `zones []Zone` cached field, reloaded every 5 seconds
- `detection_zones` and `zone_alert_rules` tables (migration v15)
- `track_id` column on `detections` table (migration v15)

---

## 7. DB Migration v15

```sql
-- New tables
CREATE TABLE detection_zones (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    camera_id TEXT NOT NULL,
    name TEXT NOT NULL,
    polygon TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    FOREIGN KEY (camera_id) REFERENCES cameras(id) ON DELETE CASCADE
);
CREATE INDEX idx_zones_camera ON detection_zones(camera_id);

CREATE TABLE zone_alert_rules (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    zone_id INTEGER NOT NULL,
    class_name TEXT NOT NULL,
    enabled INTEGER NOT NULL DEFAULT 1,
    cooldown_seconds INTEGER NOT NULL DEFAULT 30,
    loiter_seconds INTEGER NOT NULL DEFAULT 0,
    notify_on_enter INTEGER NOT NULL DEFAULT 1,
    notify_on_leave INTEGER NOT NULL DEFAULT 0,
    notify_on_loiter INTEGER NOT NULL DEFAULT 0,
    FOREIGN KEY (zone_id) REFERENCES detection_zones(id) ON DELETE CASCADE,
    UNIQUE(zone_id, class_name)
);

-- Extend detections table
ALTER TABLE detections ADD COLUMN track_id INTEGER DEFAULT 0;
CREATE INDEX idx_detections_track ON detections(track_id);
```

---

## 8. File Map

| File                                     | Action | Responsibility                                                         |
| ---------------------------------------- | ------ | ---------------------------------------------------------------------- |
| `internal/nvr/ai/tracker.go`             | Create | ByteTrack + Kalman filter                                              |
| `internal/nvr/ai/zone.go`                | Create | Zone model, PointInPolygon                                             |
| `internal/nvr/ai/state.go`               | Create | State machine, transition logic                                        |
| `internal/nvr/ai/cooldown.go`            | Create | Cooldown manager                                                       |
| `internal/nvr/ai/pipeline.go`            | Modify | Integrate tracker, zones, state, cooldowns; lower confThreshold to 0.3 |
| `internal/nvr/db/zones.go`               | Create | Zone + rule CRUD                                                       |
| `internal/nvr/db/migrations.go`          | Modify | Add v15 migration                                                      |
| `internal/nvr/api/events.go`             | Modify | Add structured fields to Event, PublishTrackedDetection method         |
| `internal/nvr/api/zones.go`              | Create | Zone REST endpoints + snapshot proxy                                   |
| `internal/nvr/api/router.go`             | Modify | Register zone routes                                                   |
| `internal/nvr/nvr.go`                    | Modify | Wire zone loading, pass to pipelines                                   |
| `ui/src/components/ZoneEditor.tsx`       | Create | Polygon drawing + zone config UI                                       |
| `ui/src/components/AnalyticsOverlay.tsx` | Modify | Render track IDs, zone overlays, loiter color                          |
| `ui/src/hooks/useNotifications.ts`       | Modify | Updated Notification interface, action-based titles                    |
| `ui/src/pages/CameraManagement.tsx`      | Modify | Add "Zones" button per camera                                          |
| `ui/src/components/Toast.tsx`            | Modify | Action-based severity styling                                          |

---

## 9. Testing Strategy

- **tracker_test.go**: Unit tests for ByteTrack greedy matching, track creation/deletion, Kalman prediction accuracy, two-pass low-confidence recovery
- **zone_test.go**: Point-in-polygon with edge cases (point on edge, concave polygons, degenerate polygons)
- **state_test.go**: State machine transitions — enter, loiter, leave, re-enter sequences; verify loiterNotified reset on leave
- **cooldown_test.go**: Cooldown suppression, expiry, per-class independence, garbage collection
- **pipeline integration test**: Full frame sequence simulating objects entering/leaving zones, verifying correct notifications fire and cooldowns are respected
