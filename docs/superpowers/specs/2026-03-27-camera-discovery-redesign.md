# Camera Discovery Flow Redesign

**Date:** 2026-03-27
**Scope:** Backend enrichment changes + Flutter UI redesign for camera discovery

## Problem

The Flutter camera discovery flow is minimal — it shows name, IP, and model, then immediately adds the camera with no chance to enter credentials, view available streams, or see capabilities. Users can't make informed decisions about which stream to use or verify the camera works before adding it.

## Design Decisions

- **Detail sheet** pattern: tapping a discovered camera opens a bottom sheet with full info, auth, stream selection, and add button
- **Rich discovery cards** with manufacturer, model, IP, capability badges, auth status indicator, and "already added" badge
- **Stream selection** showing name, resolution, codec, RTSP URI, and profile token for each available stream
- **Capability badges** showing supported features (PTZ, Events, Audio, Analytics, etc.)
- **Auth status** indicator: the initial scan reports whether unauthenticated ONVIF access succeeded, so users know upfront if credentials are needed
- **Already-added detection**: cameras already in the system are shown dimmed with an "ALREADY ADDED" badge

## Architecture

No new API endpoints. Uses existing:
- `POST /cameras/discover` — start scan
- `GET /cameras/discover/status` — poll scan progress
- `GET /cameras/discover/results` — get discovered devices
- `POST /cameras/probe` — probe with credentials, returns profiles + capabilities
- `POST /cameras` — create camera

### Backend Changes

**`internal/nvr/onvif/discovery.go`:**
- Add `AuthRequired bool` field to `DiscoveredDevice` struct
- In `enrichDevice()`: if unauthenticated profile fetch fails, set `AuthRequired = true` instead of silently skipping. If it succeeds, set `AuthRequired = false` and include the profiles.
- Capture the number of profiles found during enrichment even without auth (some cameras expose profile count in device info)

**`internal/nvr/api/cameras.go`:**
- `DiscoverResults` handler: cross-reference discovered device IPs/XAddrs against cameras already in the DB. Add an `existing_camera_id` field to each result if matched, so the Flutter client can show "ALREADY ADDED."

### Flutter UI Changes

All new widgets use `NvrColors`, `NvrTypography`, `CornerBrackets`, and the existing pill/badge patterns from the HUD design system.

**`clients/flutter/lib/screens/cameras/add_camera_screen.dart`:**
Replace the `_DiscoverTab` widget internals:

#### Discovery Result Card
Replaces the current minimal ListTile. Shows:
- Camera name (manufacturer + model fallback)
- IP address
- Manufacturer and model on a second line
- Capability badges row (PTZ, EVENTS, AUDIO, ANALYTICS) — only shown if enrichment succeeded without auth
- Auth status badge: `AUTH REQUIRED` (red) or `OPEN` (green) or `ALREADY ADDED` (blue, dimmed)
- Stream count if available (e.g., "2 STREAMS")
- Chevron indicator for tap affordance
- Already-added cards have reduced opacity

#### Camera Detail Bottom Sheet
New widget: `_CameraDetailSheet` shown via `showModalBottomSheet`. Contains:

**Header section:**
- Camera name (large, bold)
- IP, manufacturer, firmware version
- Auth status badge

**Credentials section** (shown when auth is required or when user wants to re-probe):
- Username and Password text fields side by side
- "PROBE CAMERA" button — calls `POST /cameras/probe` with XAddr + credentials
- Loading state while probing
- Error display if probe fails

**Streams section** (shown after successful probe or if profiles were available without auth):
- Radio-selectable list of streams
- Each stream shows: name, resolution (W×H), codec, profile token
- RTSP URI in smaller muted text below
- Selected stream highlighted with accent border
- First/highest-resolution stream pre-selected

**Capabilities section:**
- Horizontal wrap of capability badges
- Supported capabilities in accent color, unsupported dimmed
- Icons: Media, Events, Analytics, PTZ, Audio, Imaging, Recording

**Camera name field:**
- Pre-populated with discovered name (manufacturer + model)
- Editable text field so user can customize before adding

**Add Camera button:**
- Full-width green button at bottom
- Disabled until a stream is selected
- Calls `POST /cameras` with: name, selected stream's RTSP URL (with injected credentials), ONVIF endpoint, ONVIF credentials, selected profile token
- Shows success feedback and pops the sheet + navigates back to camera list

## Data Flow

```
1. User taps SCAN NETWORK
2. POST /cameras/discover → starts async WS-Discovery
3. Poll GET /cameras/discover/status every 2s
4. When complete, GET /cameras/discover/results
   → Backend cross-references with existing cameras in DB
   → Each result has auth_required + existing_camera_id fields
5. Display rich cards in scrollable list
6. User taps a card → open _CameraDetailSheet
7. If auth required: user enters credentials → PROBE CAMERA
   POST /cameras/probe → returns profiles + capabilities
8. User selects a stream from the list
9. User optionally edits camera name
10. Tap ADD CAMERA → POST /cameras
11. Success → close sheet, refresh camera list
```

## Error Handling

- **Scan fails:** Show error message with retry button
- **Probe fails:** Show error inline in the detail sheet (e.g., "Connection refused" or "Invalid credentials") with the credential fields still visible for retry
- **Add fails:** Show error toast, keep sheet open so user can retry
- **No devices found:** Show empty state with troubleshooting hint ("Make sure cameras are on the same network")
- **Timeout on enrichment:** Show device with whatever info was gathered; mark capabilities as unknown

## Testing

- Unit test: `DiscoveredDevice` auth_required field logic
- Unit test: existing camera cross-reference matching
- Widget test: detail sheet renders streams, capabilities, auth fields correctly
- Widget test: stream selection updates selected state
- Integration: full discovery → probe → add flow against a real camera
