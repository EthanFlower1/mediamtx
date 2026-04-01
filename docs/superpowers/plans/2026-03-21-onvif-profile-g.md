# ONVIF Profile G (Edge Recording) — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add ONVIF Profile G consumer support: search, browse, and play back recordings stored on the camera's SD card, plus import camera-side recordings into the NVR.

**Architecture:** Recording search uses the ONVIF Recording Search service via raw SOAP calls (no SDK wrappers). Playback uses the Replay service to get an RTSP URI with Range header support, proxied through MediaMTX as a temporary path. Import downloads from the camera's replay stream and saves as a local recording. The UI adds a "Camera Storage" toggle to the Recordings page.

**Tech Stack:** Go (use-go/onvif v0.0.9, Gin), React + TypeScript + Tailwind CSS

**Spec:** `docs/superpowers/specs/2026-03-21-onvif-full-profiles-design.md` (Sub-Project 3)

---

## File Structure

### New files

| File                                         | Responsibility                                                                |
| -------------------------------------------- | ----------------------------------------------------------------------------- |
| `internal/nvr/onvif/recording.go`            | Recording Search service: FindRecordings, GetRecordingSummary, search results |
| `internal/nvr/onvif/replay.go`               | Replay service: GetReplayUri for playback from camera storage                 |
| `ui/src/components/CameraStorageBrowser.tsx` | Browse and play camera-side recordings                                        |

### Modified files

| File                            | Change                                                      |
| ------------------------------- | ----------------------------------------------------------- |
| `internal/nvr/onvif/client.go`  | Verify Recording and Replay in Capabilities (already there) |
| `internal/nvr/api/cameras.go`   | Add edge-recordings, edge-playback, edge-import endpoints   |
| `internal/nvr/api/router.go`    | Register new routes                                         |
| `internal/nvr/db/migrations.go` | Migration v11: supports_edge_recording flag                 |
| `internal/nvr/db/cameras.go`    | Add SupportsEdgeRecording field                             |
| `internal/nvr/onvif/device.go`  | ProbeDeviceFull populates edge recording capability         |
| `ui/src/pages/Recordings.tsx`   | Add NVR/Camera Storage toggle                               |
| `ui/src/hooks/useCameras.ts`    | Add supports_edge_recording field                           |

---

### Task 1: Recording Search service client

**Files:**

- Create: `internal/nvr/onvif/recording.go`

- [ ] **Step 1: Create recording.go with search functions**

The ONVIF Recording Search service uses a session-based search pattern: start a search, then poll for results. The library has no SDK wrappers, so use raw SOAP via the shared Client.

```go
package onvif

import (
    "encoding/xml"
    "fmt"
    "time"
)

type EdgeRecording struct {
    RecordingToken string `json:"recording_token"`
    SourceName     string `json:"source_name"`
    EarliestTime   string `json:"earliest_time"`
    LatestTime     string `json:"latest_time"`
    TrackCount     int    `json:"track_count"`
}

type EdgeRecordingSummary struct {
    TotalRecordings int    `json:"total_recordings"`
    EarliestTime    string `json:"earliest_time"`
    LatestTime      string `json:"latest_time"`
}

// GetRecordingSummary returns a summary of recordings on the camera's storage.
func GetRecordingSummary(xaddr, username, password string) (*EdgeRecordingSummary, error) {
    client, err := NewClient(xaddr, username, password)
    if err != nil { return nil, err }
    if !client.HasService("recording") {
        return nil, fmt.Errorf("camera does not support recording search service")
    }
    // SOAP call to GetRecordingSummary
    // Parse response for DataFrom, DataUntil, NumberRecordings
}

// FindRecordings searches for recordings on the camera storage.
// Uses the session-based search: FindRecordings → GetRecordingSearchResults.
func FindRecordings(xaddr, username, password string) ([]EdgeRecording, error) {
    client, err := NewClient(xaddr, username, password)
    if err != nil { return nil, err }
    // 1. Call FindRecordings to start search (returns SearchToken)
    // 2. Poll GetRecordingSearchResults with SearchToken until Complete
    // 3. Parse RecordingInformation results
}

// SearchRecordingEvents searches for events on camera storage within a time range.
func SearchRecordingEvents(xaddr, username, password string, start, end time.Time) ([]EdgeRecording, error) {
    // Similar session-based pattern with FindEvents
}
```

Use raw SOAP with the recording search namespace (`tse:` = `http://www.onvif.org/ver10/search/wsdl`).

- [ ] **Step 2: Build and commit**

```bash
go build ./internal/nvr/...
git commit -m "feat(nvr): add ONVIF Recording Search service client"
```

---

### Task 2: Replay service client

**Files:**

- Create: `internal/nvr/onvif/replay.go`

- [ ] **Step 1: Create replay.go**

```go
package onvif

import "fmt"

// GetReplayUri returns an RTSP URI for playing back a recording from the camera.
// The URI supports Range headers for seeking.
func GetReplayUri(xaddr, username, password, recordingToken string) (string, error) {
    client, err := NewClient(xaddr, username, password)
    if err != nil { return "", err }
    if !client.HasService("replay") {
        return "", fmt.Errorf("camera does not support replay service")
    }
    // SOAP call to GetReplayUri (trp: namespace)
    // Returns RTSP URI for the recording
}
```

Use replay namespace `trp:` = `http://www.onvif.org/ver10/replay/wsdl`.

- [ ] **Step 2: Build and commit**

```bash
go build ./internal/nvr/...
git commit -m "feat(nvr): add ONVIF Replay service client for camera playback"
```

---

### Task 3: Edge recording API endpoints

**Files:**

- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Add API endpoints**

```go
// GET /cameras/:id/edge-recordings
// Returns recordings available on the camera's SD card.
// Calls GetRecordingSummary + FindRecordings.

// GET /cameras/:id/edge-recordings/playback?recording_token=X&start=RFC3339
// Returns a temporary RTSP URI for playing back a camera-side recording.
// Calls GetReplayUri. The frontend uses this URI via the MediaMTX WebRTC proxy.

// POST /cameras/:id/edge-recordings/import
// Body: { recording_token, start, end }
// Downloads a time range from the camera's recording and saves it as an NVR recording.
// Uses the replay RTSP URI to stream and re-save.
// For v1: return the replay URI and let the user download directly via the playback server.
```

- [ ] **Step 2: Register routes**

In `router.go`:

```go
protected.GET("/cameras/:id/edge-recordings", cameraHandler.EdgeRecordings)
protected.GET("/cameras/:id/edge-recordings/playback", cameraHandler.EdgePlayback)
protected.POST("/cameras/:id/edge-recordings/import", cameraHandler.EdgeImport)
```

- [ ] **Step 3: Build and commit**

```bash
go build ./internal/nvr/...
git commit -m "feat(nvr): add edge recording API endpoints for camera storage access"
```

---

### Task 4: Database capability flag

**Files:**

- Modify: `internal/nvr/db/migrations.go`
- Modify: `internal/nvr/db/cameras.go`
- Modify: `internal/nvr/onvif/device.go`
- Modify: `internal/nvr/db/db_test.go`
- Modify: `ui/src/hooks/useCameras.ts`

- [ ] **Step 1: Add migration v11**

```sql
ALTER TABLE cameras ADD COLUMN supports_edge_recording INTEGER NOT NULL DEFAULT 0;
```

- [ ] **Step 2: Update Camera struct and queries**

Add `SupportsEdgeRecording bool` to Camera struct. Update INSERT, SELECT, UPDATE queries.

- [ ] **Step 3: Populate in ProbeDeviceFull**

In `device.go`, set `SupportsEdgeRecording` based on `client.HasService("recording") && client.HasService("replay")`.

- [ ] **Step 4: Update TypeScript and tests**

Add `supports_edge_recording?: boolean` to useCameras.ts Camera interface.
Update db_test.go migration version.

- [ ] **Step 5: Build, test, commit**

```bash
go test ./internal/nvr/db/ -v -run TestOpen -count=1
go build ./internal/nvr/...
git commit -m "feat(nvr): add edge recording capability flag to camera model"
```

---

### Task 5: Camera Storage Browser UI

**Files:**

- Create: `ui/src/components/CameraStorageBrowser.tsx`
- Modify: `ui/src/pages/Recordings.tsx`

- [ ] **Step 1: Create CameraStorageBrowser.tsx**

A component that browses and plays recordings from the camera's SD card:

```typescript
interface Props {
  cameraId: string;
  camera: Camera;
}
```

Features:

- Fetch edge recordings from `GET /api/nvr/cameras/{id}/edge-recordings`
- Show summary: total recordings, date range covered
- List recordings with recording token, source name, time range
- "Play" button: fetches playback URI from edge-playback endpoint, plays via the existing VideoPlayer component
- "Import to NVR" button: calls edge-import endpoint (v1: shows the replay URI for manual download)
- Loading/error states
- "Edge recording not supported" when camera doesn't have the service
- Styled with Tailwind nvr- colors

- [ ] **Step 2: Add NVR/Camera Storage toggle to Recordings page**

In `ui/src/pages/Recordings.tsx`, add a toggle/tab at the top:

- "NVR Recordings" (default) — shows existing timeline + playback
- "Camera Storage" — shows CameraStorageBrowser component

Only show the toggle when the selected camera has `supports_edge_recording: true`.

- [ ] **Step 3: Build and commit**

```bash
export NVM_DIR="$HOME/.nvm" && source "$NVM_DIR/nvm.sh" && nvm use 20
cd ui && npm run build
git commit -m "feat(nvr): add camera storage browser UI with NVR/Camera toggle"
```

---

### Task 6: Integration test

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
git commit -m "test(nvr): verify ONVIF Profile G edge recording completion"
```
