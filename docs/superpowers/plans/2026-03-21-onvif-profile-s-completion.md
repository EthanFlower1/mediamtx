# ONVIF Profile S Completion — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Complete ONVIF Profile S consumer support by adding GetSnapshotURI, audio backchannel, relay outputs, and PTZ configuration queries.

**Architecture:** Each feature is a self-contained addition to the `internal/nvr/onvif/` package with corresponding API endpoints and UI components. All use the `use-go/onvif` SDK wrapper pattern established by existing code (connect device → call SDK function → parse response). A shared `client.go` is extracted to reduce duplication.

**Tech Stack:** Go (use-go/onvif v0.0.9, Gin), React + TypeScript + Tailwind CSS

**Spec:** `docs/superpowers/specs/2026-03-21-onvif-full-profiles-design.md` (Sub-Project 1)

---

## File Structure

### New files

| File                                  | Responsibility                                                       |
| ------------------------------------- | -------------------------------------------------------------------- |
| `internal/nvr/onvif/client.go`        | Shared ONVIF device client with service discovery, capability checks |
| `internal/nvr/onvif/client_test.go`   | Client unit tests                                                    |
| `internal/nvr/onvif/relay.go`         | Relay output discovery and control                                   |
| `internal/nvr/onvif/audio.go`         | Audio backchannel discovery and URI retrieval                        |
| `ui/src/components/RelayControls.tsx` | Relay output toggle UI                                               |
| `ui/src/components/AudioIntercom.tsx` | Push-to-talk audio backchannel UI                                    |

### Modified files

| File                                | Change                                                          |
| ----------------------------------- | --------------------------------------------------------------- |
| `internal/nvr/onvif/snapshot.go`    | Replace URL guessing with GetSnapshotURI, fall back to guessing |
| `internal/nvr/onvif/ptz.go`         | Add GetNodes, GetConfigurations for capability discovery        |
| `internal/nvr/onvif/discovery.go`   | Use shared client, populate capability flags                    |
| `internal/nvr/onvif/imaging.go`     | Use shared client instead of inline connectDevice               |
| `internal/nvr/onvif/events.go`      | Use shared client instead of inline connectDevice               |
| `internal/nvr/api/cameras.go`       | Add relay and audio endpoints                                   |
| `internal/nvr/api/router.go`        | Register new routes                                             |
| `internal/nvr/db/migrations.go`     | Add v9 migration for capability flags on cameras                |
| `internal/nvr/db/cameras.go`        | Add capability flag fields to Camera struct                     |
| `ui/src/pages/CameraManagement.tsx` | Add relay controls and audio intercom to camera detail          |
| `ui/src/pages/LiveView.tsx`         | Add audio intercom button to camera modal                       |
| `ui/src/hooks/useCameras.ts`        | Add capability fields to Camera interface                       |

---

### Task 1: Extract shared ONVIF client

**Files:**

- Create: `internal/nvr/onvif/client.go`
- Modify: `internal/nvr/onvif/imaging.go`
- Modify: `internal/nvr/onvif/device.go`

- [ ] **Step 1: Create client.go with shared Client struct**

```go
// internal/nvr/onvif/client.go
package onvif

import (
    "fmt"
    onviflib "github.com/use-go/onvif"
)

// Client wraps an ONVIF device connection with service discovery.
type Client struct {
    Dev      *onviflib.Device
    Services map[string]string
    Username string
    Password string
}

// NewClient creates a Client connected to an ONVIF device.
func NewClient(xaddr, username, password string) (*Client, error) {
    host := xaddrToHost(xaddr)
    if host == "" {
        host = xaddr
    }
    dev, err := onviflib.NewDevice(onviflib.DeviceParams{
        Xaddr:    host,
        Username: username,
        Password: password,
    })
    if err != nil {
        return nil, fmt.Errorf("connect to ONVIF device: %w", err)
    }
    return &Client{
        Dev:      dev,
        Services: dev.GetServices(),
        Username: username,
        Password: password,
    }, nil
}

// HasService checks if the device exposes a named service.
func (c *Client) HasService(name string) bool {
    _, ok := c.Services[name]
    return ok
}

// ServiceURL returns the endpoint URL for a named service.
func (c *Client) ServiceURL(name string) string {
    return c.Services[name]
}

// Capabilities returns a summary of which ONVIF services are available.
type Capabilities struct {
    Media             bool
    Media2            bool
    PTZ               bool
    Imaging           bool
    Events            bool
    Analytics         bool
    DeviceIO          bool
    Recording         bool
    Replay            bool
    AudioBackchannel  bool
}

// GetCapabilities checks which services the device supports.
func (c *Client) GetCapabilities() Capabilities {
    return Capabilities{
        Media:    c.HasService("media"),
        Media2:   c.HasService("media2"),
        PTZ:      c.HasService("ptz"),
        Imaging:  c.HasService("imaging"),
        Events:   c.HasService("events") || c.HasService("event"),
        Analytics: c.HasService("analytics"),
        DeviceIO: c.HasService("deviceio"),
        Recording: c.HasService("recording"),
        Replay:   c.HasService("replay"),
    }
}
```

- [ ] **Step 2: Refactor imaging.go to use NewClient instead of connectDevice**

Replace the `connectDevice` calls in `GetImagingSettings` and `SetImagingSettings` with `NewClient`. Keep `connectDevice` as a thin wrapper for backward compatibility with other callers.

- [ ] **Step 3: Build and verify**

Run: `go build ./internal/nvr/...`
Expected: Clean build.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/onvif/client.go internal/nvr/onvif/imaging.go
git commit -m "refactor(nvr): extract shared ONVIF client with service discovery"
```

---

### Task 2: Add capability flags to camera database

**Files:**

- Modify: `internal/nvr/db/migrations.go`
- Modify: `internal/nvr/db/cameras.go`
- Modify: `internal/nvr/db/db_test.go`
- Modify: `ui/src/hooks/useCameras.ts`

- [ ] **Step 1: Add migration v9**

In `internal/nvr/db/migrations.go`, add:

```go
{
    version: 9,
    sql: `
ALTER TABLE cameras ADD COLUMN supports_ptz INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cameras ADD COLUMN supports_imaging INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cameras ADD COLUMN supports_events INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cameras ADD COLUMN supports_relay INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cameras ADD COLUMN supports_audio_backchannel INTEGER NOT NULL DEFAULT 0;
ALTER TABLE cameras ADD COLUMN snapshot_uri TEXT DEFAULT '';
`,
},
```

- [ ] **Step 2: Update Camera struct**

Add fields to the Camera struct in `cameras.go`:

```go
SupportsPTZ              bool   `json:"supports_ptz"`
SupportsImaging          bool   `json:"supports_imaging"`
SupportsEvents           bool   `json:"supports_events"`
SupportsRelay            bool   `json:"supports_relay"`
SupportsAudioBackchannel bool   `json:"supports_audio_backchannel"`
SnapshotURI              string `json:"snapshot_uri,omitempty"`
```

Update all CRUD queries (INSERT, SELECT, UPDATE) to include the new columns.

- [ ] **Step 3: Update Camera TypeScript interface**

In `ui/src/hooks/useCameras.ts`, add:

```typescript
supports_ptz?: boolean
supports_imaging?: boolean
supports_events?: boolean
supports_relay?: boolean
supports_audio_backchannel?: boolean
snapshot_uri?: string
```

- [ ] **Step 4: Update db_test.go migration version**

Update `TestOpenRunsMigrations` to expect version 9.

- [ ] **Step 5: Run tests and commit**

Run: `go test ./internal/nvr/db/ -v -run TestOpen -count=1`
Commit: `git commit -m "feat(nvr): add ONVIF capability flags and snapshot URI to camera model"`

---

### Task 3: Implement GetSnapshotURI

**Files:**

- Modify: `internal/nvr/onvif/snapshot.go`
- Modify: `internal/nvr/onvif/device.go`

- [ ] **Step 1: Add GetSnapshotURI function**

In `snapshot.go`, add a new function that queries the camera via ONVIF for its snapshot URI:

```go
// GetSnapshotURI queries the camera's Media service for the snapshot URI
// for the given profile token.
func GetSnapshotURI(xaddr, username, password, profileToken string) (string, error) {
    client, err := NewClient(xaddr, username, password)
    if err != nil {
        return "", err
    }

    if !client.HasService("media") {
        return "", fmt.Errorf("camera does not support media service")
    }

    ctx := context.Background()
    req := onvifmedia.GetSnapshotUri{
        ProfileToken: onviftypes.ReferenceToken(profileToken),
    }
    resp, err := sdkmedia.Call_GetSnapshotUri(ctx, client.Dev, req)
    if err != nil {
        return "", fmt.Errorf("get snapshot URI: %w", err)
    }

    uri := string(resp.MediaUri.Uri)
    if uri == "" {
        return "", fmt.Errorf("camera returned empty snapshot URI")
    }

    return uri, nil
}
```

- [ ] **Step 2: Update CaptureSnapshot to try ONVIF first**

Refactor `CaptureSnapshot` to:

1. If `snapshotURI` is provided (from DB), use it directly
2. Otherwise, try `GetSnapshotURI` via ONVIF
3. Fall back to guessing common URLs (existing behavior)

```go
func CaptureSnapshot(rtspURL, username, password, outputDir, cameraID, snapshotURI string) (string, error) {
    // Try provided snapshot URI first (cached from ONVIF discovery)
    if snapshotURI != "" {
        path, err := downloadSnapshot(snapshotURI, username, password, outputDir, cameraID)
        if err == nil {
            return path, nil
        }
    }

    // Fall back to guessing common URLs
    // ... existing code ...
}
```

- [ ] **Step 3: Populate snapshot_uri during camera probing**

In `device.go` `ProbeDevice`, after fetching profiles, also call `GetSnapshotURI` for the first profile and return it as part of the probe result. Add `SnapshotURI string` to `DiscoveredDevice` or return it separately.

- [ ] **Step 4: Build and commit**

Run: `go build ./internal/nvr/...`
Commit: `git commit -m "feat(nvr): implement ONVIF GetSnapshotURI for reliable thumbnail capture"`

---

### Task 4: Implement relay output control

**Files:**

- Create: `internal/nvr/onvif/relay.go`
- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`
- Create: `ui/src/components/RelayControls.tsx`

- [ ] **Step 1: Create relay.go**

```go
package onvif

import (
    "context"
    "fmt"
    onvifdevice "github.com/use-go/onvif/device"
    sdkdevice "github.com/use-go/onvif/sdk/device"
    onviftypes "github.com/use-go/onvif/xsd/onvif"
)

type RelayOutput struct {
    Token     string `json:"token"`
    Mode      string `json:"mode"`       // "Pulse" or "Bistable"
    IdleState string `json:"idle_state"` // "open" or "closed"
}

func GetRelayOutputs(xaddr, username, password string) ([]RelayOutput, error) {
    client, err := NewClient(xaddr, username, password)
    if err != nil {
        return nil, err
    }

    ctx := context.Background()
    resp, err := sdkdevice.Call_GetRelayOutputs(ctx, client.Dev, onvifdevice.GetRelayOutputs{})
    if err != nil {
        return nil, fmt.Errorf("get relay outputs: %w", err)
    }

    // Parse response — library returns single RelayOutput, may need XML parsing for multiple
    // ... parse and return
}

func SetRelayOutputState(xaddr, username, password, token string, active bool) error {
    client, err := NewClient(xaddr, username, password)
    if err != nil {
        return err
    }

    state := onviftypes.RelayLogicalState("inactive")
    if active {
        state = "active"
    }

    ctx := context.Background()
    _, err = sdkdevice.Call_SetRelayOutputState(ctx, client.Dev, onvifdevice.SetRelayOutputState{
        RelayOutputToken: onviftypes.ReferenceToken(token),
        LogicalState:     state,
    })
    return err
}
```

- [ ] **Step 2: Add API endpoints**

In `cameras.go`, add `GetRelayOutputs` and `SetRelayOutputState` handlers:

```go
// GET /cameras/:id/relay-outputs
// POST /cameras/:id/relay-outputs/:token/state  body: {"active": true}
```

Register in `router.go`.

- [ ] **Step 3: Create RelayControls.tsx**

A component that:

- Fetches relay outputs from API
- Shows each output as a card with name, mode, and a toggle switch
- Toggle calls the state endpoint
- Shows "No relay outputs" if camera doesn't support them
- Styled with Tailwind nvr- colors

- [ ] **Step 4: Integrate into CameraManagement detail panel**

Add RelayControls to the expanded camera detail view, visible only when `camera.supports_relay` is true.

- [ ] **Step 5: Build, test, commit**

Run: `go build ./internal/nvr/...`
Build UI: `cd ui && npm run build`
Commit: `git commit -m "feat(nvr): add ONVIF relay output discovery and control"`

---

### Task 5: Implement PTZ configuration queries

**Files:**

- Modify: `internal/nvr/onvif/ptz.go`
- Modify: `ui/src/components/PTZControls.tsx`

- [ ] **Step 1: Add GetNodes and GetConfigurations**

In `ptz.go`, add:

```go
type PTZNode struct {
    Token           string  `json:"token"`
    Name            string  `json:"name"`
    MaxPanSpeed     float64 `json:"max_pan_speed"`
    MaxTiltSpeed    float64 `json:"max_tilt_speed"`
    MaxZoomSpeed    float64 `json:"max_zoom_speed"`
    HomePosSupport  bool    `json:"home_position_support"`
    MaxPresets      int     `json:"max_presets"`
}

func (p *PTZController) GetNodes() ([]PTZNode, error) {
    ctx := context.Background()
    resp, err := sdkptz.Call_GetNodes(ctx, p.dev, onvifptz.GetNodes{})
    // Parse nodes with capability info
}

func (p *PTZController) GetConfigurations() ([]PTZConfiguration, error) {
    ctx := context.Background()
    resp, err := sdkptz.Call_GetConfigurations(ctx, p.dev, onvifptz.GetConfigurations{})
    // Parse configurations
}
```

- [ ] **Step 2: Add API endpoint**

```go
// GET /cameras/:id/ptz/capabilities
```

Returns PTZ node info including speed ranges and supported features.

- [ ] **Step 3: Update PTZControls.tsx**

Fetch capabilities on mount. Use speed ranges to scale the joystick sensitivity:

- If maxPanSpeed is 0.5, scale the UI joystick value from [-1,1] to [-0.5, 0.5]
- Show/hide Home button based on `homePosSupport`
- Show max preset count

- [ ] **Step 4: Build, test, commit**

Commit: `git commit -m "feat(nvr): add PTZ capability queries and smart speed scaling"`

---

### Task 6: Implement audio backchannel discovery

**Files:**

- Create: `internal/nvr/onvif/audio.go`
- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`
- Create: `ui/src/components/AudioIntercom.tsx`
- Modify: `ui/src/pages/LiveView.tsx`

- [ ] **Step 1: Create audio.go**

```go
package onvif

import (
    "context"
    "fmt"
    onvifmedia "github.com/use-go/onvif/media"
    sdkmedia "github.com/use-go/onvif/sdk/media"
)

type AudioCapabilities struct {
    HasBackchannel bool     `json:"has_backchannel"`
    AudioSources   int      `json:"audio_sources"`
    Encodings      []string `json:"encodings"` // "AAC", "G711", etc.
}

func GetAudioCapabilities(xaddr, username, password string) (*AudioCapabilities, error) {
    client, err := NewClient(xaddr, username, password)
    if err != nil {
        return nil, err
    }

    ctx := context.Background()
    caps := &AudioCapabilities{}

    // Check audio outputs (backchannel)
    outputResp, err := sdkmedia.Call_GetAudioOutputs(ctx, client.Dev, onvifmedia.GetAudioOutputs{})
    if err == nil {
        // If camera has audio outputs, it supports backchannel
        caps.HasBackchannel = true // parse response for count
    }

    // Check audio sources
    sourceResp, err := sdkmedia.Call_GetAudioSources(ctx, client.Dev, onvifmedia.GetAudioSources{})
    if err == nil {
        caps.AudioSources = 1 // parse response
    }

    return caps, nil
}
```

- [ ] **Step 2: Add API endpoint**

```go
// GET /cameras/:id/audio/capabilities
```

- [ ] **Step 3: Create AudioIntercom.tsx**

Push-to-talk component:

- Microphone button (press-and-hold to talk)
- Uses `navigator.mediaDevices.getUserMedia({ audio: true })` to capture microphone
- Visual feedback: pulsing ring while transmitting
- Disabled state when camera doesn't support backchannel

Note: Actual audio streaming to the camera requires RTSP backchannel or proprietary HTTP POST, which varies by camera. For v1, discover the capability and show the UI. Full audio streaming implementation can follow in a separate task.

- [ ] **Step 4: Add to LiveView camera modal**

Show AudioIntercom button next to the screenshot button when `camera.supports_audio_backchannel` is true.

- [ ] **Step 5: Build, test, commit**

Commit: `git commit -m "feat(nvr): add audio backchannel discovery and intercom UI"`

---

### Task 7: Populate capabilities during camera probing

**Files:**

- Modify: `internal/nvr/onvif/device.go`
- Modify: `internal/nvr/api/cameras.go`

- [ ] **Step 1: Update ProbeDevice to return capabilities**

Add a new function or extend `ProbeDevice` to also return device capabilities:

```go
type ProbeResult struct {
    Profiles     []MediaProfile
    SnapshotURI  string
    Capabilities Capabilities
}

func ProbeDeviceFull(xaddr, username, password string) (*ProbeResult, error) {
    client, err := NewClient(xaddr, username, password)
    // ... get profiles, snapshot URI, and capabilities
}
```

- [ ] **Step 2: Store capabilities when creating/updating cameras**

In `cameras.go` Create handler, after a successful probe, store the capability flags and snapshot URI in the camera record. When a user adds a camera via ONVIF discovery, the probe populates these automatically.

- [ ] **Step 3: Build, test, commit**

Run all tests: `go test ./internal/nvr/... -count=1`
Build UI: `cd ui && npm run build`
Commit: `git commit -m "feat(nvr): populate ONVIF capabilities during camera discovery and probe"`

---

### Task 8: Integration test and cleanup

**Files:**

- Various test files

- [ ] **Step 1: Verify all tests pass**

Run: `go test ./internal/nvr/... -count=1 -v`
Expected: All pass.

- [ ] **Step 2: Build full UI**

```bash
export NVM_DIR="$HOME/.nvm" && source "$NVM_DIR/nvm.sh" && nvm use 20
cd ui && npm run build
```

- [ ] **Step 3: End-to-end verification**

Start the server and verify:

1. Camera probe returns capability flags
2. Snapshot capture uses ONVIF URI when available
3. Relay outputs show (if camera supports)
4. PTZ controls respect speed ranges
5. Audio capabilities detected

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "test(nvr): verify ONVIF Profile S completion with integration tests"
```
