# ONVIF Profile M (Metadata & Analytics) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add ONVIF Profile M consumer support: receive and display analytics metadata (bounding boxes, object classification), filter events by object type, and configure analytics rules from the UI.

**Architecture:** The metadata parsing types were already defined in Profile T (`metadata.go`). This plan connects them to the live video overlay (WebSocket stream of detection results), stores object classification in the database, and adds UI for filtering and rule configuration. Analytics rule management uses the API built in Profile T.

**Tech Stack:** Go (use-go/onvif v0.0.9, Gin, gorilla/websocket), React + TypeScript + Tailwind CSS, HTML5 Canvas

**Spec:** `docs/superpowers/specs/2026-03-21-onvif-full-profiles-design.md` (Sub-Project 4)

---

## File Structure

### New files
| File | Responsibility |
|------|---------------|
| `ui/src/components/AnalyticsOverlay.tsx` | Canvas overlay rendering bounding boxes on live video |
| `ui/src/components/AnalyticsConfig.tsx` | Analytics module management UI (enable/disable modules, create rules) |

### Modified files
| File | Change |
|------|--------|
| `internal/nvr/db/migrations.go` | Migration v12: object_class + confidence on motion_events (if not already from Profile T) |
| `internal/nvr/db/motion_events.go` | Add object_class and confidence fields, filter query |
| `internal/nvr/scheduler/scheduler.go` | Store object class from metadata when available |
| `internal/nvr/api/recordings.go` | Add object type filter to motion events query |
| `ui/src/pages/Recordings.tsx` | Add object type filter chips (All, Person, Vehicle, Animal) |
| `ui/src/pages/LiveView.tsx` | Add analytics overlay toggle to camera modal |
| `ui/src/components/Timeline.tsx` | Different emoji per object class |
| `ui/src/pages/CameraManagement.tsx` | Add AnalyticsConfig section |

---

### Task 1: Object classification in database

**Files:**
- Modify: `internal/nvr/db/migrations.go`
- Modify: `internal/nvr/db/motion_events.go`
- Modify: `internal/nvr/api/recordings.go`

- [ ] **Step 1: Add object_class and confidence columns**

Check if migration v10 already added `event_type`. Add a new migration for `object_class` and `confidence` if not present:

```sql
ALTER TABLE motion_events ADD COLUMN object_class TEXT DEFAULT '';
ALTER TABLE motion_events ADD COLUMN confidence REAL DEFAULT 0;
CREATE INDEX idx_motion_events_object_class ON motion_events(camera_id, object_class);
```

- [ ] **Step 2: Update MotionEvent struct**

Add `ObjectClass string` and `Confidence float64` to MotionEvent. Update INSERT and SELECT queries.

- [ ] **Step 3: Add filtered query**

```go
func (d *DB) QueryMotionEventsByClass(cameraID, objectClass string, start, end time.Time) ([]*MotionEvent, error)
```

- [ ] **Step 4: Add filter parameter to motion events API**

In `recordings.go` `MotionEvents` handler, accept optional `object_class` query param:

```
GET /cameras/:id/motion-events?date=2026-03-21&object_class=person
```

- [ ] **Step 5: Build, test, commit**

---

### Task 2: Object type filter UI on Recordings page

**Files:**
- Modify: `ui/src/pages/Recordings.tsx`
- Modify: `ui/src/components/Timeline.tsx`

- [ ] **Step 1: Add filter chips to Recordings page**

Above the timeline, add clickable filter pills:
- All | 🏃 Motion | 👤 Person | 🚗 Vehicle | 🐾 Animal | 🛡️ Tampering
- Selected filter highlighted with accent color
- Clicking a filter refetches motion events with `object_class` param
- "All" shows all events

- [ ] **Step 2: Update Timeline event markers with class-specific emojis**

In Timeline.tsx, update the emoji based on `object_class`:
```
"person" → 👤
"vehicle" → 🚗
"animal" → 🐾
"tampering" → 🛡️
"motion" or "" → 🏃
```

- [ ] **Step 3: Build and commit**

---

### Task 3: Analytics overlay component

**Files:**
- Create: `ui/src/components/AnalyticsOverlay.tsx`
- Modify: `ui/src/pages/LiveView.tsx`

- [ ] **Step 1: Create AnalyticsOverlay.tsx**

A Canvas-based overlay that renders bounding boxes on top of live video:

```typescript
interface Detection {
  objectId: string
  className: string   // "Person", "Vehicle", "Animal"
  confidence: number  // 0-100
  box: { left: number; top: number; right: number; bottom: number } // 0-1 normalized
}

interface Props {
  cameraId: string
  videoRef: React.RefObject<HTMLVideoElement>
  enabled: boolean
}
```

Features:
- Positioned absolutely over the video element
- Canvas matches video dimensions
- Renders color-coded bounding boxes:
  - Blue: Person
  - Green: Vehicle
  - Amber: Animal
  - Red: Unknown
- Label above each box: "Person 94%"
- Boxes fade after 2 seconds if not refreshed
- For v1: poll the analytics metadata from a REST endpoint every 1 second (real-time WebSocket streaming deferred)
- Toggle button to show/hide overlay

Note: Most consumer cameras (including the user's AD410) don't stream ONVIF metadata. This component is ready for cameras that do support it. For cameras without metadata streaming, the overlay simply shows nothing.

- [ ] **Step 2: Add toggle to LiveView camera modal**

In LiveView.tsx CameraModal, add an "Analytics" toggle button next to the screenshot button. When enabled, render AnalyticsOverlay on top of the video.

- [ ] **Step 3: Build and commit**

---

### Task 4: Analytics configuration UI

**Files:**
- Create: `ui/src/components/AnalyticsConfig.tsx`
- Modify: `ui/src/pages/CameraManagement.tsx`

- [ ] **Step 1: Create AnalyticsConfig.tsx**

A management component for analytics modules and rules:

Features:
- Fetch modules from `GET /cameras/:id/analytics/modules`
- Fetch rules from `GET /cameras/:id/analytics/rules`
- Show modules as cards with name, type, and description
- Show rules as cards with name, type, parameters, and delete button
- "Add Rule" opens the DetectionZoneEditor (already built in Profile T)
- "Analytics not available" when camera doesn't support it
- Styled with Tailwind nvr- colors

- [ ] **Step 2: Integrate into CameraManagement**

Add an "Analytics" tab/section in the camera detail panel, below the existing Motion Zones section. Shows AnalyticsConfig component when `camera.supports_analytics` is true.

- [ ] **Step 3: Build and commit**

---

### Task 5: Integration test

- [ ] **Step 1: Run all tests**

```bash
cd /Users/ethanflower/personal_projects/mediamtx
go test ./internal/nvr/... -count=1
```

- [ ] **Step 2: Build UI**

```bash
export NVM_DIR="$HOME/.nvm" && source "$NVM_DIR/nvm.sh" && nvm use 20
cd ui && npm run build
```

- [ ] **Step 3: Final commit**

```bash
git add -A
git commit -m "test(nvr): verify ONVIF Profile M analytics completion"
```
