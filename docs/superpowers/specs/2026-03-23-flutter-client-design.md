# Flutter NVR Client Design

## Goal

Build a cross-platform Flutter client for MediaMTX NVR that provides full feature parity with the React web UI, targeting iOS, Android, macOS, Windows, Linux, and Web.

## Decisions

- **Platforms:** iOS, Android, macOS, Windows, Linux, Web
- **Location:** `clients/flutter/` inside the mediamtx repo
- **Feature scope:** Full parity with React web client
- **Live streaming:** flutter_webrtc with WHEP protocol
- **Recording playback:** media_kit with HLS
- **State management:** Riverpod
- **Navigation:** Bottom nav (mobile) + side rail (desktop) via go_router
- **Visual design:** Custom dark surveillance theme using Material 3, inspired by React UI palette
- **Auth on mobile:** JWT + refresh token in secure storage (not cookies)
- **Analytics overlay:** WebSocket detection_frame events (not REST polling)
- **Push notifications:** Local notifications via foreground service / background task (no Firebase)
- **Offline:** Cached camera list, "Server Unreachable" banner, auto-reconnect
- **Multi-server:** Single server for v1

---

## 1. Project Structure

```
clients/flutter/
  lib/
    main.dart
    app.dart                          # MaterialApp, theme, router
    theme/
      nvr_theme.dart                  # Dark theme with Material 3
      nvr_colors.dart                 # Color constants (bg: #0f172a, accent: #3b82f6, etc.)
    router/
      app_router.dart                 # go_router with auth redirect guards
    services/
      api_client.dart                 # dio + JWT interceptor + auto-refresh
      whep_service.dart               # WebRTC WHEP live streaming
      playback_service.dart           # HLS URL builder for media_kit
      websocket_service.dart          # Persistent WS to API port + 1
      auth_service.dart               # Login, refresh, logout, secure storage
      notification_service.dart       # Local push notifications (mobile/desktop)
    providers/
      auth_provider.dart              # Auth state, current user, token
      cameras_provider.dart           # Camera list, status, CRUD ops
      notifications_provider.dart     # WS event stream, in-app history (100 items)
      detection_stream_provider.dart  # detection_frame events for overlay
      recordings_provider.dart        # Segments, motion events, timeline
      settings_provider.dart          # System info, storage, users, config
    models/
      camera.dart
      notification_event.dart
      detection.dart
      detection_frame.dart            # Per-frame bbox data from WS
      motion_event.dart
      user.dart
      recording.dart
      zone.dart                        # Includes alert rules
      saved_clip.dart
    screens/
      login_screen.dart
      setup_screen.dart
      server_setup_screen.dart        # First-launch server URL entry
      live_view/
        live_view_screen.dart         # Adaptive camera grid
        camera_tile.dart              # WebRTC player + connection state
        fullscreen_view.dart          # Single camera fullscreen
        ptz_controls.dart             # D-pad/joystick overlay
        analytics_overlay.dart        # CustomPainter bbox rendering
      playback/
        playback_screen.dart          # Multi-camera synchronized playback
        timeline_widget.dart          # Vertical timeline with event markers
        playback_controls.dart        # Play/pause/speed/seek bar
      search/
        clip_search_screen.dart       # AI semantic search + results
        search_result_card.dart       # Thumbnail + actions
      cameras/
        camera_list_screen.dart       # Camera list with status badges
        camera_detail_screen.dart     # Edit camera settings
        add_camera_screen.dart        # ONVIF discovery + manual
        zone_editor_screen.dart       # Polygon zone drawing on snapshot
        recording_rules_screen.dart   # Per-camera recording rules
      settings/
        settings_screen.dart          # Tabbed: system, storage, users, audit
        user_management_screen.dart   # CRUD users (admin only)
        storage_panel.dart            # Disk usage, per-camera breakdown
        backup_panel.dart             # Create/download/list backups
    widgets/
      adaptive_layout.dart            # Bottom nav (mobile) / side rail (desktop)
      connection_banner.dart          # "Server Unreachable" persistent banner
      notification_toast.dart         # In-app snackbar alerts
      camera_status_badge.dart        # Online/offline indicator
  test/
  pubspec.yaml
```

---

## 2. Key Packages

| Package                                              | Purpose                    | Platforms                           |
| ---------------------------------------------------- | -------------------------- | ----------------------------------- |
| `flutter_webrtc`                                     | Live view via WHEP         | All 6                               |
| `media_kit` + `media_kit_video` + `media_kit_libs_*` | HLS recording playback     | All 6                               |
| `riverpod` + `flutter_riverpod`                      | State management           | All                                 |
| `go_router`                                          | Navigation + auth guards   | All                                 |
| `dio`                                                | HTTP client + interceptors | All                                 |
| `flutter_secure_storage`                             | JWT/token storage          | iOS, Android, macOS, Windows, Linux |
| `web_socket_channel`                                 | WebSocket notifications    | All                                 |
| `flutter_local_notifications`                        | Native push alerts         | iOS, Android, macOS, Windows, Linux |
| `freezed` + `json_serializable`                      | Immutable models + JSON    | All                                 |

---

## 3. Authentication

### Login Flow (Mobile/Desktop)

1. User enters server URL on first launch → stored in secure storage
2. POST `{server}/api/nvr/auth/login` with `{"username": "...", "password": "..."}`
3. Response: `{"access_token": "eyJ...", "refresh_token": "raw-token-string", "expires_in": 900, "user": {...}}`
4. Store both tokens in `flutter_secure_storage`
5. Navigate to live view

### Backend Change Required

The `/auth/refresh` endpoint currently reads the refresh token from an HttpOnly cookie only. For mobile clients, it must also accept a JSON body:

```json
{ "refresh_token": "raw-token-string" }
```

The endpoint checks cookie first (web compatibility), then JSON body (mobile). One change in `internal/nvr/api/auth.go`'s `Refresh` handler.

Similarly, `/auth/login` response must include the raw `refresh_token` value in the JSON body (currently only set as a cookie). Add `"refresh_token": rawToken` to the login response JSON.

### JWT Interceptor (dio)

```dart
class AuthInterceptor extends Interceptor {
  // Attaches Authorization: Bearer {token} to every request
  // On 401: call /auth/refresh with stored refresh token
  // On refresh success: retry original request
  // On refresh failure: navigate to login
}
```

### Token Refresh Timer

Background timer fires 60 seconds before JWT expiry (`expires_in - 60`). On app resume from background, check token freshness immediately and refresh if needed.

---

## 4. Live View (WebRTC WHEP)

### WHEP Handshake (per camera)

```dart
class WhepService {
  Future<RTCVideoRenderer> connect(String serverUrl, String mediamtxPath) async {
    final pc = await createPeerConnection({'iceServers': []});
    pc.addTransceiver(kind: RTCRtpMediaType.RTCRtpMediaTypeVideo, init: RTCRtpTransceiverInit(direction: TransceiverDirection.RecvOnly));
    pc.addTransceiver(kind: RTCRtpMediaType.RTCRtpMediaTypeAudio, init: RTCRtpTransceiverInit(direction: TransceiverDirection.RecvOnly));

    final offer = await pc.createOffer();
    await pc.setLocalDescription(offer);
    // Wait for ICE gathering complete
    final answer = await _postWhepOffer(serverUrl, mediamtxPath, offer.sdp!);
    await pc.setRemoteDescription(RTCSessionDescription(answer, 'answer'));

    final renderer = RTCVideoRenderer();
    await renderer.initialize();
    pc.onTrack = (event) => renderer.srcObject = event.streams[0];
    return renderer;
  }
}
```

### Camera Grid

Adaptive layout using `LayoutBuilder`:

- Width < 600px (phone): 1 column
- Width 600-900px (tablet): 2 columns
- Width 900-1200px (small desktop): 3 columns
- Width > 1200px (large desktop): 4 columns

Each tile shows: video feed, camera name, status badge (online/offline), connection state indicator.

### Connection Resilience

- States: `connecting`, `connected`, `failed`
- On failure: exponential backoff retry (3s, 6s, 12s, max 30s), up to 5 attempts
- UI: spinner while connecting, error + retry button on failure
- Offline cameras show red "OFFLINE" badge

### Fullscreen View

Tap a tile → fullscreen with:

- PTZ d-pad overlay (for `ptz_capable` cameras)
- Analytics overlay (bounding boxes from WebSocket)
- Screenshot button
- Audio toggle (for backchannel-capable cameras)
- Swipe left/right to switch cameras

---

## 5. Analytics Overlay (WebSocket-Based)

### Backend Change Required

In `pipeline.go`'s `ProcessFrame`, after storing detections, broadcast a new `detection_frame` event type via the existing WebSocket (port 9998):

```json
{
  "type": "detection_frame",
  "camera": "AD410",
  "detections": [
    {
      "class": "person",
      "confidence": 0.87,
      "track_id": 4,
      "x": 0.3,
      "y": 0.2,
      "w": 0.1,
      "h": 0.3
    },
    {
      "class": "car",
      "confidence": 0.72,
      "track_id": 2,
      "x": 0.6,
      "y": 0.5,
      "w": 0.15,
      "h": 0.1
    }
  ],
  "time": "2026-03-23T21:50:40Z"
}
```

This is broadcast at the pipeline's frame rate (2 FPS) for each camera with AI enabled.

### Flutter Overlay

`DetectionStreamProvider` filters WebSocket events by `type == "detection_frame"` and the currently viewed camera ID. `AnalyticsOverlay` widget uses `CustomPainter` to draw:

- Color-coded bounding boxes (blue=person, green=vehicle, amber=animal)
- Labels: "Person #4 87%"
- Box color changes to red when loitering

Only subscribes when overlay is visible on screen. The backend only includes detection data for AI-enabled cameras, so bandwidth is bounded.

---

## 6. Recording Playback (HLS)

### URL Construction

```dart
String playbackUrl(String server, String path, DateTime start, double durationSecs) {
  final startIso = _toLocalRfc3339(start);
  return '$server:9996/get?path=${Uri.encodeComponent(path)}&start=${Uri.encodeComponent(startIso)}&duration=$durationSecs';
}
```

### media_kit Player

```dart
final player = Player();
final controller = VideoController(player);
player.open(Media(playbackUrl));
// Speed: player.setRate(2.0)
// Seek: player.seek(Duration(...))
```

### Multi-Camera Sync

Same approach as the web fix:

- Track ready state per camera
- On seek: pause all, wait for all to buffer at new position, resume together
- Shared `PlaybackController` manages sync state

### Timeline Widget

Vertical timeline with:

- 24-hour scale, responsive height (`min(960, viewportHeight - 200)`)
- Recording segment bars (colored by camera)
- Motion event markers (icons for person/car/animal)
- Tap to seek, drag playhead
- Auto-scroll to current position

---

## 7. Clip Search (AI Semantic)

### Search Flow

1. User types query ("person with backpack")
2. GET `/search?q={query}&limit=20`
3. Results displayed as cards with: thumbnail, class badge, confidence, timestamp, camera name
4. Tap card → play clip via HLS (30-second window centered on detection time)
5. Long-press → save clip, download, open in playback

### Semantic Search Status

Check `/system/info` on mount. If `clip_search_available` is false, show: "Basic search only (class matching). Install CLIP models for natural language search."

---

## 8. Camera Management

### Camera List

List view with: name, status badge, thumbnail (from `/cameras/:id/snapshot`), recording indicator, AI indicator. Swipe to delete (with confirmation).

### Add Camera

Two tabs:

- **Discover:** POST `/cameras/discover`, poll status, show results. 30-second timeout with cancel button.
- **Manual:** Enter RTSP URL (validated: must start with `rtsp://`), name, credentials.

### Camera Detail

Tabbed view:

- **General:** Name, RTSP URL, ONVIF endpoint, credentials
- **Recording:** Recording rules list, add/edit/delete
- **AI:** Enable toggle, sub-stream URL, confidence threshold slider (20-90%)
- **Zones:** Zone editor (snapshot + polygon drawing via `CustomPainter` with `GestureDetector`)
- **Advanced:** Image settings, motion timeout, retention days

### Zone Editor

- Fetches snapshot from `/cameras/:id/snapshot`
- Canvas with `GestureDetector` for tap-to-add-point
- Double-tap to close polygon
- Existing zones shown as semi-transparent colored overlays
- Per-zone config sheet: class toggles, cooldown slider, loiter threshold, enter/leave/loiter toggles
- Save via POST/PUT zone API

---

## 9. Settings

### Tabbed Layout

- **System:** Version, platform, uptime, server URL, connection status
- **Storage:** Disk usage bar, per-camera breakdown, warning/critical thresholds, cleanup button
- **Users:** List users, create/edit/delete (admin only), change own password
- **Backups:** Create backup, list backups, download
- **Audit:** Event log table, export CSV

---

## 10. Notifications

### In-App

- `WebSocketService` maintains persistent connection to `ws://{server}:9998/ws`
- Events parsed and routed to `NotificationsProvider`
- In-app history: last 100 events
- Toast/snackbar for real-time alerts (amber=entered, red=loitering, blue=left, etc.)
- Notification bell in app bar with unread count badge

### Background (Mobile)

- Android: Foreground service keeps WebSocket alive, triggers `flutter_local_notifications`
- iOS: Background task with `workmanager` for periodic WebSocket check, local notifications
- Desktop: App runs in system tray, WebSocket stays connected, native notifications

### Event Types Handled

| Type                | Alert                      | Badge                  |
| ------------------- | -------------------------- | ---------------------- |
| `ai_detection`      | Snackbar + push            | Person/car/animal icon |
| `motion`            | Snackbar + push            | Motion icon            |
| `camera_offline`    | Persistent snackbar + push | Red offline icon       |
| `camera_online`     | Brief snackbar             | Green online icon      |
| `recording_started` | Brief snackbar             | Record icon            |
| `detection_frame`   | No alert (overlay only)    | —                      |

---

## 11. Navigation

### Adaptive Layout

```dart
class AdaptiveLayout extends StatelessWidget {
  // LayoutBuilder checks width:
  // < 600px  → Scaffold with NavigationBar (bottom)
  // >= 600px → Scaffold with NavigationRail (side)
  // Both use the same 5 destinations:
  //   Live View, Playback, Search, Cameras, Settings
}
```

### Auth Guards

`go_router` redirect: if no valid JWT in secure storage, redirect to `/login`. If no server URL configured, redirect to `/server-setup`.

---

## 12. Theme

Dark surveillance aesthetic using Material 3:

```dart
// NVR Color Palette (matching React UI)
static const bgPrimary = Color(0xFF0f172a);     // Deep navy
static const bgSecondary = Color(0xFF1e293b);    // Card backgrounds
static const bgTertiary = Color(0xFF334155);     // Input backgrounds
static const accent = Color(0xFF3b82f6);          // Blue accent
static const accentHover = Color(0xFF2563eb);
static const textPrimary = Color(0xFFf1f5f9);
static const textSecondary = Color(0xFF94a3b8);
static const textMuted = Color(0xFF64748b);
static const success = Color(0xFF22c55e);
static const warning = Color(0xFFf59e0b);
static const danger = Color(0xFFef4444);
static const border = Color(0xFF334155);
```

Material 3 `ThemeData.dark()` customized with `ColorScheme.fromSeed` using the accent blue, then overriding surface/background colors to match the NVR palette.

---

## 13. Offline & Server Connection

- **Server URL:** Entered on first launch, stored in `flutter_secure_storage`
- **Health check:** GET `/system/health` to validate server before login
- **Offline cache:** Camera list cached locally via `SharedPreferences` (JSON)
- **Unreachable state:** Persistent `MaterialBanner` at top: "Server unreachable — retrying..."
- **Auto-reconnect:** API client retries failed requests. WebSocket auto-reconnects with backoff.

---

## 14. Backend Changes Required

Changes to the Go backend to support the Flutter client:

1. **Auth refresh via JSON body** (`internal/nvr/api/auth.go`): Accept `{"refresh_token": "..."}` in POST `/auth/refresh` body, in addition to the existing cookie. Check cookie first, then body. **This is a prerequisite — the Flutter client cannot authenticate without it.**

2. **Auth login includes refresh token** (`internal/nvr/api/auth.go`): Add `"refresh_token": rawToken` to the login JSON response. Currently only set as a cookie, which mobile clients cannot read.

3. **detection_frame WebSocket event** (`internal/nvr/ai/pipeline.go`): After processing each frame, broadcast a `detection_frame` event with bbox data via the existing EventBroadcaster. The `Event` struct gains a `Detections` field (`json:"detections,omitempty"`) — a slice of `{Class, Confidence, TrackID, X, Y, W, H}`. This field is nil/omitted for all other event types, so serialization is backward-compatible.

4. **Snapshot proxy endpoint** (`internal/nvr/api/zones.go` or new file): `GET /cameras/:id/snapshot` — proxies the camera's ONVIF snapshot URI with digest auth (same as the existing AI pipeline's `captureAndDecode`). The Flutter client cannot fetch snapshots directly from cameras due to digest auth and potential subnet restrictions. **Note:** This endpoint may already exist from the detection zones work — verify and use if so.

5. **Zone CRUD endpoints** (already implemented in worktree): `GET /cameras/:id/zones`, `POST /cameras/:id/zones`, `PUT /zones/:id`, `DELETE /zones/:id`. These exist on the `worktree-smart-detection-notifications` branch. Ensure they are merged to main.

6. **Backup endpoints** (already implemented in worktree): `POST /system/backup`, `GET /system/backups`, `GET /system/backups/:filename`. From the production readiness work.

7. **System info: clip_search_available** (`internal/nvr/api/system.go`): Add `"clip_search_available": embedder != nil` to the `/system/info` response so the client can detect CLIP availability.

8. **System info: ports** (`internal/nvr/api/system.go`): Add `"ws_port"`, `"playback_port"`, `"webrtc_port"` to `/system/info` so the Flutter client can discover ports rather than hardcoding. The WebSocket port is `API port + 1`, playback is 9996, WebRTC is 8889 — but these should be discoverable.

---

## 15. Platform-Specific Notes

### iOS

- **App Transport Security:** Self-signed certs or local HTTP connections require an ATS exception in `Info.plist` (`NSAppTransportSecurity > NSAllowsLocalNetworking = true`).
- **Background notifications:** iOS limits background execution to ~30 seconds. Real-time push while backgrounded is not achievable without APNs. The app uses `BGAppRefreshTask` for periodic checks (system-discretionary, ~15 min intervals). Document this limitation for customers.

### Android

- **Foreground service:** For persistent WebSocket notifications when backgrounded. Requires `FOREGROUND_SERVICE` permission and a persistent notification.
- **Cleartext traffic:** If connecting over HTTP, set `android:usesCleartextTraffic="true"` in `AndroidManifest.xml` or use a network security config.

### WebSocket Authentication

The WebSocket server (port API+1) currently has **no JWT authentication** — any client on the LAN can connect. This is acceptable for local network NVR deployments. If the NVR is exposed to the internet, JWT-based WebSocket auth should be added as a future enhancement (pass token as query parameter: `ws://host:port/ws?token=...`).

---

## 16. API Response Schemas

### Search Result (`GET /search`)

```json
{
  "query": "person",
  "count": 3,
  "results": [
    {
      "detection_id": 11655,
      "event_id": 198,
      "camera_id": "uuid",
      "camera_name": "AD410",
      "class": "person",
      "confidence": 0.92,
      "similarity": 0.87,
      "frame_time": "2026-03-23T22:56:47.205Z",
      "thumbnail_path": "thumbnails/event_xxx.jpg"
    }
  ]
}
```

### detection_frame WebSocket Event

```json
{
  "type": "detection_frame",
  "camera": "AD410",
  "time": "2026-03-23T21:50:40Z",
  "detections": [
    {
      "class": "person",
      "confidence": 0.87,
      "track_id": 4,
      "x": 0.3,
      "y": 0.2,
      "w": 0.1,
      "h": 0.3
    }
  ]
}
```

### Camera Object (`GET /cameras`)

```json
{
  "id": "uuid",
  "name": "AD410",
  "rtsp_url": "rtsp://...",
  "onvif_endpoint": "http://...",
  "mediamtx_path": "nvr/ad410",
  "status": "online",
  "ptz_capable": true,
  "ai_enabled": true,
  "sub_stream_url": "rtsp://...",
  "retention_days": 30,
  "motion_timeout_seconds": 8,
  "snapshot_uri": "http://...",
  "supports_events": true,
  "supports_analytics": true,
  "created_at": "2026-03-23T...",
  "updated_at": "2026-03-23T..."
}
```

---

## 17. File Map

| File                          | Action | Purpose                                     |
| ----------------------------- | ------ | ------------------------------------------- |
| `clients/flutter/`            | Create | Entire Flutter project                      |
| `internal/nvr/api/auth.go`    | Modify | Refresh token in JSON body + login response |
| `internal/nvr/ai/pipeline.go` | Modify | Broadcast detection_frame events            |
| `internal/nvr/api/events.go`  | Modify | Add Detections field to Event struct        |
| `internal/nvr/api/system.go`  | Modify | Add clip_search_available + port discovery  |
