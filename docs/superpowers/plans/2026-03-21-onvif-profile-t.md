# ONVIF Profile T (Advanced Streaming) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add ONVIF Profile T consumer support: Media2 service with auto-fallback, motion region configuration via analytics rules, tampering detection events, and metadata stream parsing types for Profile M foundation.

**Architecture:** Media2 uses raw SOAP calls via `dev.CallMethod()` since `use-go/onvif v0.0.9` lacks Media2 SDK wrappers. Analytics rule configuration also uses `dev.CallMethod()` with the library's request types. Tampering detection extends the existing event parser. Metadata stream types are defined for later consumption by Profile M.

**Tech Stack:** Go (use-go/onvif v0.0.9, Gin), React + TypeScript + Tailwind CSS

**Spec:** `docs/superpowers/specs/2026-03-21-onvif-full-profiles-design.md` (Sub-Project 2)

---

## File Structure

### New files

| File                                        | Responsibility                                                                        |
| ------------------------------------------- | ------------------------------------------------------------------------------------- |
| `internal/nvr/onvif/media2.go`              | Media2 service: GetProfiles2, GetStreamUri2, GetSnapshotUri2, auto-fallback to Media1 |
| `internal/nvr/onvif/analytics.go`           | Analytics service: rules CRUD, supported modules query                                |
| `internal/nvr/onvif/metadata.go`            | Metadata stream XML parsing types (Frame, Object, BoundingBox)                        |
| `ui/src/components/DetectionZoneEditor.tsx` | Draw motion detection zones on camera snapshot                                        |

### Modified files

| File                                  | Change                                                                 |
| ------------------------------------- | ---------------------------------------------------------------------- |
| `internal/nvr/onvif/client.go`        | Add Media2 and Analytics to Capabilities                               |
| `internal/nvr/onvif/device.go`        | ProbeDeviceFull uses Media2 when available                             |
| `internal/nvr/onvif/events.go`        | Extend parseMotionEvents to detect tampering events, return event type |
| `internal/nvr/db/migrations.go`       | Migration v10: supports_media2, supports_analytics columns             |
| `internal/nvr/db/cameras.go`          | Add new capability fields                                              |
| `internal/nvr/api/cameras.go`         | Add analytics rule endpoints                                           |
| `internal/nvr/api/router.go`          | Register analytics routes                                              |
| `internal/nvr/api/events.go`          | Add tampering event type to Event struct                               |
| `internal/nvr/scheduler/scheduler.go` | Publish tampering events alongside motion                              |
| `ui/src/pages/CameraManagement.tsx`   | Add "Motion Zones" section in camera detail                            |
| `ui/src/components/Timeline.tsx`      | Add tampering event icon                                               |
| `ui/src/hooks/useCameras.ts`          | Add new capability fields                                              |

---

### Task 1: Create Media2 service client

**Files:**

- Create: `internal/nvr/onvif/media2.go`
- Modify: `internal/nvr/onvif/client.go`
- Modify: `internal/nvr/onvif/device.go`

- [ ] **Step 1: Create media2.go with GetProfiles2 and GetStreamUri2**

Since `use-go/onvif v0.0.9` has no Media2 SDK wrappers, use `dev.CallMethod()` with custom request types and manual XML response parsing.

```go
package onvif

import (
    "context"
    "encoding/xml"
    "fmt"
    "net/url"

    onviflib "github.com/use-go/onvif"
    "github.com/use-go/onvif/sdk"
    onviftypes "github.com/use-go/onvif/xsd/onvif"
)

// Media2 request/response types (not in use-go/onvif v0.0.9).

type getProfiles2Request struct {
    XMLName string                    `xml:"tr2:GetProfiles"`
    Token   onviftypes.ReferenceToken `xml:"tr2:Token,omitempty"`
    Type    string                    `xml:"tr2:Type,omitempty"`
}

type getStreamUri2Request struct {
    XMLName  string                    `xml:"tr2:GetStreamUri"`
    Protocol string                    `xml:"tr2:Protocol"`
    Token    onviftypes.ReferenceToken `xml:"tr2:ProfileToken"`
}

type getSnapshotUri2Request struct {
    XMLName string                    `xml:"tr2:GetSnapshotUri"`
    Token   onviftypes.ReferenceToken `xml:"tr2:ProfileToken"`
}

// Media2Profile represents a profile from the Media2 service.
type Media2Profile struct {
    Token      string `json:"token"`
    Name       string `json:"name"`
    VideoCodec string `json:"video_codec,omitempty"`
    Width      int    `json:"width,omitempty"`
    Height     int    `json:"height,omitempty"`
}

// GetProfiles2 queries the Media2 service for media profiles.
func GetProfiles2(client *Client) ([]Media2Profile, error) {
    if !client.HasService("media2") {
        return nil, fmt.Errorf("camera does not support Media2 service")
    }
    // Use dev.CallMethod with custom request type
    // Parse XML response manually
    // Return list of Media2Profile
}

// GetStreamUri2 gets a stream URI via the Media2 service.
func GetStreamUri2(client *Client, profileToken string) (string, error) {
    // Similar pattern: CallMethod + parse
}

// GetSnapshotUri2 gets a snapshot URI via the Media2 service.
func GetSnapshotUri2(client *Client, profileToken string) (string, error) {
    // Similar pattern: CallMethod + parse
}

// GetProfilesAuto tries Media2 first, falls back to Media1.
// Returns MediaProfile slice (same type used by existing code).
func GetProfilesAuto(xaddr, username, password string) ([]MediaProfile, bool, error) {
    // Returns (profiles, usedMedia2, error)
    client, err := NewClient(xaddr, username, password)
    if err != nil { return nil, false, err }

    if client.HasService("media2") {
        profiles, err := GetProfiles2(client)
        if err == nil && len(profiles) > 0 {
            // Convert Media2Profile to MediaProfile
            // Get stream URIs via Media2
            return result, true, nil
        }
    }

    // Fall back to Media1 (existing ProbeDevice logic)
    // ...
}
```

- [ ] **Step 2: Update ProbeDeviceFull to prefer Media2**

In `device.go`, modify `ProbeDeviceFull` to call `GetProfilesAuto` instead of directly using Media1. Store `supports_media2` flag in result.

- [ ] **Step 3: Build and commit**

```bash
go build ./internal/nvr/...
git commit -m "feat(nvr): add ONVIF Media2 service client with auto-fallback to Media1"
```

---

### Task 2: Add tampering detection events

**Files:**

- Modify: `internal/nvr/onvif/events.go`
- Modify: `internal/nvr/api/events.go`
- Modify: `internal/nvr/scheduler/scheduler.go`
- Modify: `ui/src/components/Timeline.tsx`

- [ ] **Step 1: Extend event parsing to detect tampering**

In `events.go`, modify `parseMotionEvents` to return an event type alongside the detection state. Rename it to `parseEvents` for clarity:

```go
type DetectedEventType string

const (
    EventMotion    DetectedEventType = "motion"
    EventTampering DetectedEventType = "tampering"
)

type DetectedEvent struct {
    Type     DetectedEventType
    Active   bool
}

// parseEvents scans for motion AND tampering events.
func parseEvents(body []byte) ([]DetectedEvent, error) {
    // Existing motion detection logic +
    // New: check for tampering topics:
    //   "globalscenechange", "tamper", "tampering"
    // Return list of detected events
}
```

Update `HandleNotification` and `pullMessages` to use the new function.

- [ ] **Step 2: Add tampering event type to EventBroadcaster**

In `api/events.go`, add:

```go
func (b *EventBroadcaster) PublishTampering(cameraName string) {
    b.Publish(Event{
        Type:    "tampering",
        Camera:  cameraName,
        Message: fmt.Sprintf("Tampering detected on %s", cameraName),
    })
}
```

- [ ] **Step 3: Update scheduler to handle tampering events**

In `scheduler.go`, modify the motion callbacks to also handle tampering:

```go
// In both startEventPipelineLocked and startMotionAlertSubscription:
if event.Type == EventTampering && event.Active && s.eventPub != nil {
    s.eventPub.PublishTampering(cam.Name)
}
```

- [ ] **Step 4: Add tampering icon to Timeline**

In `ui/src/components/Timeline.tsx`, update the MotionEvent interface and rendering:

- If event type is "tampering", show a shield icon (🛡️) instead of runner (🏃)
- Add `event_type` field to MotionEvent interface

- [ ] **Step 5: Build and commit**

```bash
go build ./internal/nvr/...
cd ui && npm run build
git commit -m "feat(nvr): add tampering detection events with timeline markers"
```

---

### Task 3: Implement analytics rule configuration

**Files:**

- Create: `internal/nvr/onvif/analytics.go`
- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Create analytics.go**

Use `dev.CallMethod()` with the library's analytics request types (they exist but have no SDK wrappers):

```go
package onvif

import (
    "context"
    "fmt"
    onvifanalytics "github.com/use-go/onvif/analytics"
    "github.com/use-go/onvif/sdk"
    onviftypes "github.com/use-go/onvif/xsd/onvif"
)

type AnalyticsRule struct {
    Name       string            `json:"name"`
    Type       string            `json:"type"` // e.g. "tt:CellMotionDetector"
    Parameters map[string]string `json:"parameters"`
}

type AnalyticsModule struct {
    Name       string            `json:"name"`
    Type       string            `json:"type"`
    Parameters map[string]string `json:"parameters"`
}

func GetSupportedRules(xaddr, username, password, configToken string) ([]string, error) {
    client, err := NewClient(xaddr, username, password)
    // dev.CallMethod(onvifanalytics.GetSupportedRules{ConfigurationToken: token})
    // Parse response for rule type names
}

func GetRules(xaddr, username, password, configToken string) ([]AnalyticsRule, error) {
    // dev.CallMethod(onvifanalytics.GetRules{ConfigurationToken: token})
}

func CreateRule(xaddr, username, password, configToken string, rule AnalyticsRule) error {
    // dev.CallMethod(onvifanalytics.CreateRules{...})
}

func ModifyRule(xaddr, username, password, configToken string, rule AnalyticsRule) error {
    // dev.CallMethod(onvifanalytics.ModifyRules{...})
}

func DeleteRule(xaddr, username, password, configToken, ruleName string) error {
    // dev.CallMethod(onvifanalytics.DeleteRules{...})
}

func GetAnalyticsModules(xaddr, username, password, configToken string) ([]AnalyticsModule, error) {
    // dev.CallMethod(onvifanalytics.GetAnalyticsModules{...})
}
```

- [ ] **Step 2: Add API endpoints**

In `cameras.go`, add analytics rule handlers:

```go
// GET  /cameras/:id/analytics/rules          — list rules
// POST /cameras/:id/analytics/rules          — create rule
// PUT  /cameras/:id/analytics/rules/:name    — modify rule
// DELETE /cameras/:id/analytics/rules/:name  — delete rule
// GET  /cameras/:id/analytics/modules        — list modules
```

Register in `router.go`.

- [ ] **Step 3: Build and commit**

```bash
go build ./internal/nvr/...
git commit -m "feat(nvr): add ONVIF analytics rule configuration API"
```

---

### Task 4: Create motion detection zone editor UI

**Files:**

- Create: `ui/src/components/DetectionZoneEditor.tsx`
- Modify: `ui/src/pages/CameraManagement.tsx`

- [ ] **Step 1: Create DetectionZoneEditor.tsx**

A canvas overlay component for drawing motion detection zones:

```typescript
interface Zone {
  name: string;
  points: { x: number; y: number }[]; // Normalized 0-1 coordinates
}

interface Props {
  cameraId: string;
  snapshotUrl?: string; // Background image from camera snapshot
  existingZones: Zone[];
  onSave: (zones: Zone[]) => void;
}
```

Features:

- Display camera snapshot as background
- Draw rectangles by click-drag
- Show existing zones as colored overlays with labels
- Delete zone button per zone
- "Save" button that calls the analytics rules API
- "Reset" button to clear all zones
- Coordinates normalized to 0-1 range (ONVIF standard)
- Styled with nvr- colors, transparent zone fills

- [ ] **Step 2: Integrate into CameraManagement**

Add a "Motion Zones" tab/section in the camera detail panel:

- Visible when `camera.supports_analytics` is true
- Loads existing rules from GET `/analytics/rules`
- Shows DetectionZoneEditor with current zones
- Save calls PUT/POST to analytics rules API
- Shows "Analytics not supported" when camera doesn't have the service

- [ ] **Step 3: Build and commit**

```bash
cd ui && npm run build
git commit -m "feat(nvr): add motion detection zone editor with canvas overlay"
```

---

### Task 5: Define metadata stream parsing types

**Files:**

- Create: `internal/nvr/onvif/metadata.go`
- Modify: `internal/nvr/db/migrations.go`
- Modify: `internal/nvr/db/cameras.go`

- [ ] **Step 1: Create metadata.go with XML parsing types**

Define the ONVIF metadata stream XML schema for parsing analytics data sent by cameras. This is the foundation for Profile M but the types are defined here:

```go
package onvif

import "encoding/xml"

// MetadataStream represents a parsed ONVIF metadata stream frame.
type MetadataStream struct {
    XMLName xml.Name        `xml:"MetadataStream"`
    Frames  []MetadataFrame `xml:"VideoAnalytics>Frame"`
}

type MetadataFrame struct {
    UtcTime string           `xml:"UtcTime,attr"`
    Objects []MetadataObject `xml:"Object"`
}

type MetadataObject struct {
    ObjectId string              `xml:"ObjectId,attr"`
    Class    MetadataClass       `xml:"Appearance>Class>Type"`
    Score    float64             `xml:"Appearance>Class>Type>Likelihood,attr"`
    Box     MetadataBoundingBox `xml:"Appearance>Shape>BoundingBox"`
}

type MetadataClass struct {
    Value string `xml:",chardata"` // "Human", "Vehicle", "Animal", etc.
}

type MetadataBoundingBox struct {
    Left   float64 `xml:"left,attr"`
    Top    float64 `xml:"top,attr"`
    Right  float64 `xml:"right,attr"`
    Bottom float64 `xml:"bottom,attr"`
}

// ParseMetadataFrame parses a single metadata XML chunk from the stream.
func ParseMetadataFrame(data []byte) (*MetadataFrame, error) {
    var stream MetadataStream
    if err := xml.Unmarshal(data, &stream); err != nil {
        return nil, err
    }
    if len(stream.Frames) == 0 {
        return nil, nil
    }
    return &stream.Frames[0], nil
}
```

- [ ] **Step 2: Add supports_media2 and supports_analytics to DB**

Add migration v10:

```sql
ALTER TABLE cameras ADD COLUMN supports_media2 INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cameras ADD COLUMN supports_analytics INTEGER NOT NULL DEFAULT 0;
```

Update Camera struct and CRUD queries.

- [ ] **Step 3: Update useCameras.ts**

Add `supports_media2` and `supports_analytics` to the TypeScript Camera interface.

- [ ] **Step 4: Build, test, commit**

```bash
go test ./internal/nvr/db/ -v -run TestOpen -count=1
go build ./internal/nvr/...
cd ui && npm run build
git commit -m "feat(nvr): add metadata stream parsing types and analytics capability flags"
```

---

### Task 6: Integration test and cleanup

**Files:**

- Various

- [ ] **Step 1: Run all tests**

```bash
cd /Users/ethanflower/personal_projects/mediamtx
go test ./internal/nvr/... -count=1 -v
```

- [ ] **Step 2: Build full UI**

```bash
export NVM_DIR="$HOME/.nvm" && source "$NVM_DIR/nvm.sh" && nvm use 20
cd ui && npm run build
```

- [ ] **Step 3: Verify ProbeDeviceFull returns Media2 flag**

Test with a camera that supports Media2 (if available) or verify the fallback works correctly.

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "test(nvr): verify ONVIF Profile T completion"
```
