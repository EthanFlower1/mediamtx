# ONVIF Full Profile Implementation Design

## Overview

Implement comprehensive ONVIF consumer support across Profiles S, T, G, and M for the MediaMTX NVR. The NVR will use ONVIF to discover, configure, stream from, and receive analytics from IP cameras. All features are consumer-side only (NVR talks to cameras, not vice versa for ONVIF compliance).

## Sub-Projects

Four sub-projects in dependency order. Each produces independently testable, shippable functionality.

### Sub-Project 1: Complete Profile S (Streaming)

**Currently implemented:** WS-Discovery, GetDeviceInformation, GetProfiles, GetStreamURI, PTZ (ContinuousMove, Stop, GotoPreset, GotoHome, GetPresets), Events (WS-BaseNotification push + PullPoint fallback), GetImagingSettings/SetImagingSettings, GetVideoSources.

**New features:**

| Feature | ONVIF Service | Description |
|---------|--------------|-------------|
| GetSnapshotURI | Media | Query the camera's actual snapshot endpoint instead of guessing common URLs. Use for event thumbnails and live preview stills. |
| Audio backchannel | Media/Device | Send audio TO the camera for two-way communication (doorbells, intercoms). Requires discovering the backchannel audio profile and streaming audio via RTSP backchannel or HTTP POST. |
| Relay outputs | Device | Trigger physical relay outputs on the camera (sirens, lights, door locks). GetRelayOutputs to discover available outputs, SetRelayOutputState to trigger. |
| PTZ configuration | PTZ | GetNodes to discover PTZ capabilities (pan/tilt/zoom ranges, speed limits, supported spaces). GetConfigurations to get current PTZ config. Use to build smarter UI controls with proper speed scaling and range limits. |

**Architecture:**
- `media.go`: Add `GetSnapshotURI(profileToken)` returning the snapshot URL.
- `audio.go`: New file. `GetAudioOutputs()`, `GetAudioBackchannelURI()`, audio streaming via RTSP backchannel.
- `relay.go`: New file. `GetRelayOutputs()`, `SetRelayOutputState(token, state)`.
- `ptz.go`: Extend with `GetNodes()`, `GetConfigurations()`. Return capability structs.

**UI changes:**
- Thumbnail capture: use `GetSnapshotURI` instead of guessing common URLs in `snapshot.go`.
- Audio intercom: push-to-talk button in live view camera modal (uses `getUserMedia` for microphone, streams to camera).
- Relay panel: list of available relay outputs with toggle buttons in camera settings.
- PTZ controls: use range data from `GetNodes` to scale joystick sensitivity correctly.

### Sub-Project 2: Profile T (Advanced Streaming)

**Depends on:** Sub-Project 1 (Profile S complete).

**New features:**

| Feature | ONVIF Service | Description |
|---------|--------------|-------------|
| Media2 service | Media2 | Modern replacement for Media1. `GetProfiles`, `GetStreamUri`, `GetSnapshotUri` via the Media2 WSDL. Better H.265 negotiation and more detailed stream configuration. Auto-detect whether camera supports Media2 and prefer it over Media1. |
| Motion region configuration | Analytics/Media2 | Query and modify motion detection regions on the camera. `GetSupportedAnalyticsModules`, `GetAnalyticsModuleOptions`, `CreateAnalyticsModule` / `ModifyAnalyticsModule`. Users draw detection zones on a video still in the UI. |
| Tampering detection events | Events | Subscribe to tampering-related event topics: `tns1:VideoSource/GlobalSceneChange/ImagingService` (camera covered/defocused), `tns1:Device/Trigger/DigitalInput` (physical tampering). Display as distinct event type with specific icon. |
| Metadata streaming | Media2 | Subscribe to the camera's metadata stream (track type "Metadata") which carries structured analytics data (object bounding boxes, classification). Parse the ONVIF metadata XML schema into usable structs. This is the foundation for Profile M analytics display. |

**Architecture:**
- `media2.go`: New file. Full Media2 service client: `GetProfiles2`, `GetStreamUri2`, `GetSnapshotUri2`, `GetVideoEncoderConfigurations`. Auto-detection: try Media2 first, fall back to Media1.
- `analytics.go`: New file (partial — analytics configuration). `GetSupportedAnalyticsModules`, `GetAnalyticsModuleOptions`, `ModifyAnalyticsModule` for motion region config.
- `events.go`: Extend topic matching to detect tampering events and classify them separately from motion.
- `types.go`: Add metadata stream XML parsing types (ONVIF metadata schema for objects, frames, shapes).

**UI changes:**
- `DetectionZoneEditor.tsx`: New component. Canvas overlay on a camera snapshot/still. Users draw rectangles or polygons defining detection zones. Saves zone coordinates via ONVIF analytics module configuration.
- Camera settings: "Motion Zones" tab showing current zones, add/edit/delete with visual editor.
- Timeline: tampering events shown with a distinct icon (shield with exclamation).
- Stream selection: when camera supports Media2, show H.265/H.264 profile options.

**Media2 auto-detection logic:**
```
On device connection:
  1. Check services map for "media2" endpoint
  2. If present: use Media2 for GetProfiles, GetStreamUri, GetSnapshotUri
  3. If absent: fall back to Media1 (current behavior)
  4. Store the preference per camera in the database
```

### Sub-Project 3: Profile G (Edge Recording)

**Can run in parallel with Sub-Project 2.** No dependency on Profile T.

**New features:**

| Feature | ONVIF Service | Description |
|---------|--------------|-------------|
| Recording search | Recording Search | `FindRecordings` to discover recording sources on the camera. `GetRecordingSearchResults` to list recordings by time range. `FindEvents` to search for events stored on the camera. |
| Playback control | Replay | `GetReplayUri` to get an RTSP URI for playing back recorded footage from the camera. Standard RTSP PLAY with Range header for seeking. PAUSE, speed control via Scale header. |
| Recording list | Recording Search | `GetRecordingSummary` for overview (total recordings, time span). `GetRecordingInformation` for details on each recording (tracks, source, time range). |
| Track management | Recording Search | `GetTrackList` for discovering what tracks exist (video, audio, metadata) and their configurations. |
| Import to NVR | Custom (not ONVIF) | Download recordings from camera storage to NVR disk. Uses RTSP replay to stream the recording, re-muxes to fMP4, and saves as a local recording with proper metadata in the NVR database. |

**Architecture:**
- `recording.go`: New file. Recording Search service client: `FindRecordings`, `GetRecordingSearchResults`, `GetRecordingSummary`, `GetRecordingInformation`, `GetMediaAttributes`.
- `replay.go`: New file. Replay service client: `GetReplayUri`, `SetReplayConfiguration`. RTSP connection management for playback.
- API: New endpoints:
  - `GET /api/nvr/cameras/:id/edge-recordings?start=&end=` — search camera SD card
  - `GET /api/nvr/cameras/:id/edge-playback?recording_token=&start=` — get RTSP replay URI
  - `POST /api/nvr/cameras/:id/edge-import` — import recording from camera to NVR

**UI changes:**
- `CameraStorageBrowser.tsx`: New component in the Recordings page. Toggle between "NVR Recordings" and "Camera Storage". Shows a timeline of what's on the SD card.
- Playback: clicking a camera-side recording plays it via the RTSP replay URI through WebRTC (MediaMTX can proxy the RTSP replay stream).
- Import: "Save to NVR" button on camera-side recordings. Shows progress during download/import.

**Playback flow:**
```
User selects camera-side recording
  → NVR queries GetReplayUri(recordingToken)
  → NVR creates a temporary MediaMTX path with source = replay RTSP URI
  → Browser connects via WHEP to that temporary path
  → User controls playback via RTSP Range/Scale headers
  → On close, NVR removes the temporary path
```

### Sub-Project 4: Profile M (Metadata & Analytics)

**Depends on:** Sub-Project 2 (Profile T metadata streaming).

**New features:**

| Feature | ONVIF Service | Description |
|---------|--------------|-------------|
| Receive analytics metadata | Media2/Metadata | Parse the metadata stream established in Profile T. Extract object detection results: bounding boxes, object class (human, vehicle, animal, face), confidence score, tracking ID. |
| Display bounding boxes | UI only | Canvas overlay on live video rendering detection boxes with labels and confidence. Updates in real-time from the metadata stream. |
| Filter events by object type | Events + DB | Store object classification alongside motion events in the database. UI filter chips: All, Person, Vehicle, Animal. Timeline markers change icon based on what was detected. |
| Configure analytics rules | Analytics | Full analytics module configuration: `GetSupportedRules`, `GetRuleOptions`, `CreateRules`, `ModifyRules`, `DeleteRules`. Rule types: CellMotionDetection, LineDetection (line crossing), FieldDetection (intrusion zone), TamperDetection, LoiteringDetection. |
| Manage analytics modules | Analytics | `GetAnalyticsModules`, `CreateAnalyticsModule`, `DeleteAnalyticsModule`. Enable/disable specific analytics features on the camera. |
| Scene description | Metadata | Parse scene description metadata: total object count per class, movement direction, dwell time. Display as a real-time overlay or dashboard widget. |

**Architecture:**
- `analytics.go`: Extend with full rule management: `GetSupportedRules`, `GetRuleOptions`, `CreateRules`, `ModifyRules`, `DeleteRules`, `GetRules`.
- `types.go`: Extend with ONVIF analytics metadata XML schema types: `Frame`, `Object`, `Shape`, `Appearance`, `Class`, `BoundingBox`.
- DB: Add `object_class` and `confidence` columns to `motion_events` table. New migration.
- API: New endpoints:
  - `GET /api/nvr/cameras/:id/analytics/rules` — list configured rules
  - `POST /api/nvr/cameras/:id/analytics/rules` — create rule
  - `PUT /api/nvr/cameras/:id/analytics/rules/:ruleId` — modify rule
  - `DELETE /api/nvr/cameras/:id/analytics/rules/:ruleId` — delete rule
  - `GET /api/nvr/cameras/:id/analytics/modules` — list analytics modules
  - `WS /api/nvr/cameras/:id/analytics/stream` — WebSocket stream of real-time detection results

**UI changes:**
- `AnalyticsOverlay.tsx`: New component. HTML5 Canvas overlay positioned on top of the live video. Receives detection data via WebSocket. Renders:
  - Bounding boxes with class labels and confidence (e.g., "Person 94%")
  - Color-coded by class: blue=person, green=vehicle, amber=animal, red=unknown
  - Tracking lines showing movement direction
  - Toggle button to show/hide overlay
- `DetectionZoneEditor.tsx`: Extended from Profile T. Now supports all rule types:
  - **Cell motion**: grid of cells, toggle which cells detect motion
  - **Line crossing**: draw lines with direction arrows
  - **Intrusion zone**: draw polygons for restricted areas
  - **Loitering**: draw zone + set dwell time threshold
- `AnalyticsConfig.tsx`: New page/section in Camera Settings. Lists all analytics modules and rules with enable/disable toggles. "Add Rule" opens the zone editor.
- Recordings page: filter chips above timeline (All | Person | Vehicle | Animal). Events show detected object class and thumbnail. Timeline markers use class-specific icons.
- Live view: small badge showing active detection count ("3 people, 1 vehicle").

**Metadata streaming flow:**
```
Camera → RTSP metadata track → MediaMTX → NVR metadata parser
  → Parsed detections sent to:
    1. WebSocket → Browser (real-time bounding boxes)
    2. Database (event logging with object class)
    3. EventBroadcaster (notifications with "Person detected on Front Door")
```

## ONVIF Package Restructure

The current `internal/nvr/onvif/` package will be reorganized:

```
internal/nvr/onvif/
├── client.go          # Device connection, service discovery, WS-Security
├── discovery.go       # WS-Discovery (existing, unchanged)
├── device.go          # Device service: info, relay outputs, network config
├── media.go           # Media1: profiles, stream URIs, snapshots
├── media2.go          # Media2: profiles, stream URIs, snapshots, metadata
├── events.go          # Event service: push + pull subscriptions
├── imaging.go         # Imaging service (existing, unchanged)
├── ptz.go             # PTZ service: movement, presets, node config
├── analytics.go       # Analytics service: rules, modules, zone config
├── recording.go       # Recording Search: find, list, summary
├── replay.go          # Replay service: playback URIs
├── audio.go           # Audio backchannel
├── relay.go           # Relay output control
├── snapshot.go        # Snapshot capture (refactored to use GetSnapshotURI)
├── metadata.go        # Metadata stream parser (XML → Go structs)
└── types.go           # Shared ONVIF types
```

**Shared `client.go`:**
Extract the common device connection and SOAP helper code currently duplicated across files into a single `Client` struct:
```go
type Client struct {
    dev      *onviflib.Device
    services map[string]string
    username string
    password string
}

func NewClient(xaddr, username, password string) (*Client, error)
func (c *Client) HasService(name string) bool
func (c *Client) CallMethod(request interface{}) (*http.Response, error)
func (c *Client) SendSOAP(endpoint, body string) ([]byte, error)
```

All service files use `Client` instead of creating their own device connections.

## Database Changes

**New migration (v9):**
```sql
ALTER TABLE motion_events ADD COLUMN object_class TEXT DEFAULT '';
ALTER TABLE motion_events ADD COLUMN confidence REAL DEFAULT 0;
CREATE INDEX idx_motion_events_object_class ON motion_events(camera_id, object_class);

ALTER TABLE cameras ADD COLUMN supports_media2 INTEGER DEFAULT 0;
ALTER TABLE cameras ADD COLUMN supports_analytics INTEGER DEFAULT 0;
ALTER TABLE cameras ADD COLUMN supports_edge_recording INTEGER DEFAULT 0;
ALTER TABLE cameras ADD COLUMN supports_audio_backchannel INTEGER DEFAULT 0;
ALTER TABLE cameras ADD COLUMN supports_relay_outputs INTEGER DEFAULT 0;
```

These capability flags are populated during device probing and stored so the UI can show/hide features per camera without re-probing.

## Implementation Order

1. **Sub-Project 1: Profile S** — GetSnapshotURI, Audio, Relay, PTZ config
2. **Sub-Project 2: Profile T** — Media2, Motion zones, Tampering, Metadata streaming
3. **Sub-Project 3: Profile G** — Edge recording search, playback, import (can overlap with T)
4. **Sub-Project 4: Profile M** — Analytics display, filtering, rule configuration
