# MediaMTX NVR Flutter Client — UI/UX Redesign Spec

## Overview

Full UI/UX redesign of the existing Flutter NVR client. Screen-by-screen rebuild using a new "Tactical HUD" design system while preserving the existing backend integration layer (Riverpod providers, Freezed models, Dio client, GoRouter, services).

**Approach:** Screen-by-screen rebuild. Each screen is purpose-built from scratch against the new design system. Existing `providers/`, `services/`, `models/`, and `router/` layers remain unchanged. Only `screens/`, `widgets/`, and `theme/` are rebuilt.

**Color system migration:** This is a clean cut-over from the existing Slate/Blue palette to the new Tactical HUD palette. The `NvrColors` class is redefined with all new values. Every widget referencing `NvrColors` will change appearance on update. There is no phased migration — all screens are rebuilt against the new theme simultaneously.

**Risk flag:** The fixed-center playhead timeline is a complete rewrite of the existing `ComposableTimeline`, `TimelineViewport`, and all `CustomPainter` layers. This is the single most complex UI component and should be prioritized for early prototyping.

## Design Decisions

| Decision | Choice | Rationale |
|---|---|---|
| Scope | Full UI/UX redesign + navigation overhaul | Current UI is functional but needs distinctive identity and improved UX flows |
| Target users | Simple by default, powerful when needed | Homeowner + security installer audiences with progressive disclosure |
| Devices | Truly adaptive (phone + tablet + desktop) | Phone for on-the-go alerts, tablet/desktop for monitoring stations |
| Inspiration | UniFi Protect (structure) + distinctive visual identity | UniFi's navigation patterns with a unique tactical aesthetic |
| Navigation | Icon rail + slide-out camera panel | Compact nav with expandable camera list, groups, drag-drop, and tours |
| Landing page | Straight to live view | Camera grid is the home screen, no dashboard intermediary |
| Visual style | Tactical HUD | Pure black, amber/orange accent, corner brackets, analog controls |
| Typography | Hybrid mono + sans | JetBrains Mono for data/labels/status, IBM Plex Sans for body/descriptions |
| Advanced features | Progressive disclosure | Hidden behind gear icons and toggles per screen, not a global mode switch |
| Mobile live view | Single camera focus with swipe | One camera fills screen, swipe left/right to switch, bottom sheet for camera list |
| Playback timeline | Fixed center playhead, scrolling timeline | Tape-deck model: playhead stays centered, timeline scrolls underneath |

## Design System

### Color Palette

| Token | Hex | Usage |
|---|---|---|
| `bgPrimary` | `#0A0A0A` | App background, primary surfaces |
| `bgSecondary` | `#111111` | Cards, panels, sidebar |
| `bgTertiary` | `#1A1A1A` | Controls, inputs, interactive surfaces |
| `border` | `#262626` | Dividers, card edges, control borders |
| `accent` | `#F97316` | Primary amber/orange accent |
| `accentHover` | `#EA580C` | Pressed/active accent state |
| `success` | `#22C55E` | Online, live, healthy status |
| `danger` | `#EF4444` | Errors, alerts, offline, critical |
| `warning` | `#EAB308` | Degraded, caution states |
| `textPrimary` | `#E5E5E5` | Headings, labels, primary text |
| `textSecondary` | `#737373` | Body text, descriptions |
| `textMuted` | `#404040` | Hints, disabled, placeholder text |

### Typography

**JetBrains Mono** (monospace) — used for:
- Status labels: uppercase, letter-spacing 1-2px (e.g., `LIVE`, `REC`, `ONLINE`)
- Camera IDs: `CAM-01`, `CAM-02`
- Timestamps: `14:32:07`
- Data values: `256.4 GB / 1 TB`
- Section headers: uppercase, letter-spacing 2px (e.g., `RECORDING RULES`)
- Control labels: uppercase, letter-spacing 1px (e.g., `SENSITIVITY`, `GRID`)

**IBM Plex Sans** (sans-serif) — used for:
- Page titles: 16px, weight 600
- Camera names: 13px, weight 500
- Descriptions and body text: 12px, line-height 1.5
- Button text: 12px, weight 600
- Alert messages and notifications

**Font Dependencies:** Bundle JetBrains Mono and IBM Plex Sans as asset files (not via `google_fonts` package). The NVR client may run on local networks without internet access, so fonts must be available offline.

### Accessibility

- `accent` (#F97316) on `bgPrimary` (#0A0A0A) yields ~5.8:1 contrast ratio — passes WCAG AA for normal text
- Status colors on their tinted backgrounds must maintain 4.5:1 minimum for text labels
- All interactive elements: minimum 44x44px touch targets
- Semantic labels on all icon-only buttons for screen readers
- `prefers-reduced-motion`: disable glow animations, timeline auto-scroll becomes instant jumps

### Animation Defaults

- Micro-interactions (toggles, button presses): 150ms, `Curves.easeOut`
- Panel slide in/out: 250ms, `Curves.easeInOut`
- Timeline seek animation: 300ms, `Curves.easeInOut`
- Tour camera transition: 400ms crossfade
- Overlay auto-hide: 3000ms delay, 200ms fade

### Custom Components

#### Camera Tile with Corner Brackets
Every camera feed (live or playback) has amber corner-bracket overlays at 40% opacity. The brackets are 2px solid lines forming L-shapes at each corner. Content within the brackets:
- Top-left: status indicator (green dot + `LIVE` label or playback time)
- Top-right: recording indicator (`REC` in red) or event badge (`MOTION` in amber)
- Bottom-left: camera name (IBM Plex Sans)
- Bottom-right: timestamp or stream metadata (JetBrains Mono)

#### Analog Slider
- Track: 4-6px height, `bgTertiary` background with `border` outline
- Fill: gradient from `accent` to `accent` at 40% opacity
- Thumb: 14-18px circle, `bgTertiary` background, 2px `accent` border, `accent` box-shadow glow
- Tick marks: 1px wide, 3-4px tall, `border` color, evenly spaced below the track
- Value readout: JetBrains Mono in `accent` color, positioned right-aligned above the track

#### Toggle Switch
- Track: 40-44px wide, 20-22px tall, `bgTertiary` background, rounded full
- ON state: 2px `accent` border, `accent` box-shadow glow, thumb at right position
- OFF state: 2px `border` border, no glow, thumb at left position
- Thumb: 12-14px circle. ON: `accent` fill with glow. OFF: `textMuted` fill
- Label below: JetBrains Mono, `ON`/`OFF` text in matching color

#### Rotary Knob
- Body: 28-40px circle, radial gradient (`bgTertiary` → `bgPrimary`), 2px `border` border
- Indicator line: 2px wide, `accent` color, extends from center toward the set position
- Notch marks: 1px lines around perimeter in `border` color at cardinal positions
- Value readout: JetBrains Mono in `accent` below the knob

#### Segmented Control
- Container: `bgPrimary` background, 1px `border` outline, 4px border-radius
- Segments: separated by 1px `border` dividers
- Active segment: `accent` at 13% opacity background, `accent` text (JetBrains Mono)
- Inactive segment: `textMuted` text

#### Status Badges
- Container: status color at 7% opacity background, status color at 27% opacity border, 4px border-radius
- Dot: 5-6px circle in status color with matching box-shadow glow
- Label: JetBrains Mono, uppercase, letter-spacing 0.5-1px, status color text

#### Buttons
- Primary: `accent` background, `bgPrimary` text, weight 600, 4px radius
- Secondary: `bgTertiary` background, `textPrimary` text, 1px `border` outline
- Danger: `danger` at 13% opacity background, `danger` text, `danger` at 27% opacity border
- Tactical: `bgTertiary` background, `accent` text, `accent` at 27% opacity border, JetBrains Mono

## Navigation Shell

### Desktop / Tablet Landscape

**Icon Rail** (60px wide):
- Fixed left sidebar, `bgSecondary` background, `border` right edge
- Logo at top: rotated diamond shape in `accent` color
- Navigation icons (top section): Live View, Playback, Search
- Separator line
- Navigation icons (bottom section): Devices
- Utility icons (bottom): Alerts (with unread badge), Settings
- Active state: icon has `accent` tinted background with `accent` border, plus a 3px `accent` indicator bar on the left edge
- Icons: Feather/Lucide icon set, 18px, stroke-width 2

**Camera Panel** (230px, slide-out):
- Triggered by tapping the active nav icon again or via camera icon
- Pushes content (doesn't overlay) on desktop
- `#0E0E0E` background, `border` right edge
- Header: `CAMERAS` label (JetBrains Mono), `+ GROUP` button, close X
- Search input: standard input styling
- Camera list: scrollable, each item shows status dot, thumbnail (44x26px), camera name (IBM Plex Sans), camera ID (JetBrains Mono), and drag handle icon
- Collapsible groups: header with collapse arrow, group name (JetBrains Mono uppercase), camera count, play-group button
- Tours section: pinned at bottom above `border` separator. Each tour shows cycle icon, name, config (camera count + interval), and active status badge

**Panel Behaviors:**
- Panel close: X button, clicking outside, or ESC
- Drag-drop: cameras have grab cursor and 6-dot drag handle. Drop targets in the grid show dashed `border` outline with `+ DROP HERE` label
- Groups: tapping a group name filters the live grid. Dragging a group onto the grid fills all slots
- Tours: activating a tour auto-cycles the live view. Floating pill appears with tour name + stop button

### Tablet Portrait
Camera panel overlays as a side sheet (doesn't push content) to preserve grid space.

### Mobile (Phone)
- Bottom navigation bar with 4 items: LIVE, PLAYBACK, SEARCH, SETTINGS
- Each item: icon (20px) + label (JetBrains Mono, 8px, letter-spacing 0.5px)
- Active: `accent` color. Inactive: `textSecondary`
- Camera panel: draggable bottom sheet triggered by camera icon in top bar
- Top bar: logo, page title, camera count badge, alerts bell
- Devices section accessible from Settings

## Navigation Shell State

The Camera Panel persists across screen transitions and needs its own provider:

- `cameraPanelProvider`: StateNotifierProvider managing panel open/closed state, search query, active group filter, and scroll position
- This provider lives in the navigation shell, not in any individual screen
- Drag feedback from Camera Panel to Live View Grid uses `LongPressDraggable` with an `Overlay` feedback widget to cross the shell/screen widget boundary

## Screens

### Login & Server Setup

These screens are restyled to match the Tactical HUD design system:

**Server Setup:**
- Center-aligned card on `bgPrimary` background
- Logo (rotated diamond) at top
- `SERVER URL` label (JetBrains Mono uppercase), standard input field
- "Connect" button (primary style)
- Error state: `danger` colored border on input + error message below

**Login:**
- Center-aligned card on `bgPrimary` background
- Logo + app name at top
- `USERNAME` and `PASSWORD` inputs (JetBrains Mono labels)
- "Sign In" button (primary style)
- Error state: shake animation + `danger` error message
- Server URL shown as `textMuted` at bottom with "Change" link

### Alerts / Notifications

Triggered by tapping the Alerts bell icon in the icon rail (desktop) or top bar (mobile):

**Desktop:** Slide-out panel from the right side (300px), overlays content. Same styling as the Camera Panel but right-aligned.

**Mobile:** Full-screen sheet sliding up from bottom.

**Content:**
- Header: `ALERTS` label (JetBrains Mono), `MARK ALL READ` button, close X
- Unread count badge
- Notification list (scrollable, most recent first, max 100):
  - Each item: status dot (color by type), camera name, message, timestamp (JetBrains Mono)
  - Types: motion (amber), camera_offline (red), camera_online (green), recording_started/stopped (neutral)
  - Unread items have `bgTertiary` background, read items have `bgSecondary`
  - Tap notification → navigates to relevant screen (e.g., playback at event time, device detail for offline alerts)
- Empty state: centered icon + "No alerts" message in `textMuted`
- Real-time: new notifications push to top with subtle slide-in animation

### Add Camera

Accessed from the "Add Camera" button on the Devices list:

**Two-tab layout** (segmented control at top):

**Discover tab:**
- "Scan Network" button triggers ONVIF discovery
- Progress indicator during scan (pulsing `accent` ring animation)
- Discovered cameras appear as cards: IP address, model name (if available), ONVIF capabilities detected
- Each card has "Add" button. Tapping pre-fills the manual form with discovered details.
- Empty state: "No cameras found" with troubleshooting tips in `textSecondary`

**Manual tab:**
- Form fields: Camera Name, RTSP URL, ONVIF Endpoint (optional), Username, Password
- "Test Connection" button (secondary) — shows success/failure badge inline
- "Add Camera" button (primary)

### Live View

**Desktop Grid:**
- Configurable NxN layout (1x1 through 4x4) via segmented control in top bar
- Top bar: page title, active group badge, grid size selector
- Each tile: camera feed with corner bracket overlay, status indicators, camera name, timestamp
- Empty slots: dashed `border` outline, plus icon, `DROP HERE` label
- Tap tile → fullscreen. Double-tap → quick-switch to playback at current time
- Right-click / long-press → context menu (detach, swap, camera settings)

**Fullscreen Single Camera:**
- Feed fills entire content area (behind the icon rail on desktop)
- Top gradient overlay: status dot + LIVE label, camera name, camera ID, REC indicator, timestamp, stream metadata (resolution, fps, bitrate)
- Bottom gradient overlay: Audio toggle, AI toggle with inline switch, Snapshot button, Grid return button, Exit fullscreen
- PTZ controls (right side, only for PTZ-capable cameras): directional d-pad (4 arrows + center home button), vertical zoom slider
- AI detection overlay: bounding boxes in `accent` color with class label + confidence badge above
- All overlays auto-hide after 3 seconds of inactivity, reveal on tap/mouse move
- Swipe left/right to cycle cameras in current group

**Mobile Live View:**
- Single camera fills the screen
- Swipe left/right to switch cameras
- Dot indicators below feed showing position in camera list
- Quick action buttons below: Audio, Fullscreen, PTZ
- Camera panel as bottom sheet

### Playback

**Fixed-Center Playhead Timeline Model:**
The playhead is a vertical line fixed at the horizontal center of the timeline. The timeline content (recordings, events, bookmarks, time labels) scrolls underneath it. This creates a tape-deck interaction where the "tape" moves, not the head.

**Playhead:**
- 2px vertical line in `accent` with box-shadow glow
- Grabbable circular handle (16px, `accent` fill, 3px `bgPrimary` border, glow shadow)
- Time readout badge above: `accent` background, `bgPrimary` text, JetBrains Mono, rounded

**Playhead States:**
1. **At rest / playing:** Timeline scrolls left automatically during playback. Playhead stays fixed at center.
2. **Grabbed:** Playhead handle glows brighter (larger shadow). Dragging left/right scrubs the timeline underneath. Playback pauses. Video updates in real-time. Drag right = backward, drag left = forward.
3. **Tap to seek:** Tapping anywhere on the timeline triggers a smooth animation sliding the timeline so the tapped point lands under the center playhead.
4. **Pinch/scroll zoom:** Zooms the time scale around the fixed playhead position. Levels: 1H, 30M, 10M, 5M. Also selectable via segmented control in transport bar.

**Mini Overview Bar:**
- Full 24-hour view at top of timeline area
- Recording availability shown as `accent` bars at 13% opacity
- Draggable viewport window: small rectangle with `accent` border showing current visible range
- Drag the viewport window to jump to different times
- Time labels: `00:00` and `24:00` at edges (JetBrains Mono, `textMuted`)

**Timeline Layers (scroll together under playhead):**
- Time labels: JetBrains Mono at major intervals (5-min or 10-min depending on zoom)
- Tick marks: major ticks (6px, `border`) at labeled intervals, minor ticks (3px, `bgTertiary`) between
- Recording layer (18px): `accent` at 20% opacity bars showing continuous/event recording periods. Gaps shown with diagonal hash pattern
- Event/motion layer (14px): `danger` at varying opacity showing motion intensity heat map
- Bookmark layer (12px): `accent` colored bookmark icons at bookmarked timestamps

**Transport Controls:**
- Skip to previous/next event, step back/forward (frame-by-frame), play/pause
- Play/pause: 36px button, `accent` fill, `bgPrimary` icon
- Other buttons: 28px, `bgSecondary` fill, `border` outline
- Speed knob: rotary control for 0.25x to 8x
- Jog slider: fine scrubbing control (horizontal analog slider centered at middle position)
- Current time: large JetBrains Mono readout in `accent`
- Timeline zoom: segmented control (1H / 30M / 10M / 5M)

**Multi-Camera Playback:**
- 2x2 grid with synchronized timelines
- All cameras share one playhead
- Event markers from all cameras merged on timeline
- Tap a tile to make it primary (larger)

**Desktop Layout:** Video area on top, transport controls bar, then timeline area at bottom.

**Mobile Layout:** Landscape-optimized. Video fills top portion. Compact transport + single-bar timeline at bottom. Fixed center playhead with arrow markers (top/bottom triangles). Swipeable timeline.

### Search

**Input:**
- Full-width search input with search icon and placeholder
- Search button: `accent` primary style
- Filter pills below: Camera (dropdown), Time range (preset options), Confidence threshold
- Active filters: `accent` tinted background + border

**Results:**
- Result count + sort indicator (JetBrains Mono)
- Responsive grid of result cards (min 200px column width)
- Each card: thumbnail with bounding box preview, confidence badge (top-right), camera name, detection class, timestamp
- Confidence badge: `accent` background on high confidence, reduced opacity on lower
- Tap result → clip player bottom sheet with 30s context centered on detection
- "Jump to playback" action from clip player

### Devices

**Device List:**
- Header: title, camera count, Discover button (secondary), Add Camera button (primary)
- Device cards: thumbnail with corner brackets, camera name, status badge, connection metadata (camera ID, resolution, protocol), capability badges (PTZ, AI, REC), chevron
- Status differentiation:
  - Online: green badge, normal opacity
  - Degraded: yellow badge, warning text showing issue
  - Offline: red border tint, reduced opacity, disconnection icon in thumbnail, "last seen" text
- Tap card → camera detail

**Camera Detail (Default View):**
- Back button, camera name, status badge, gear icon (advanced settings)
- Two-column layout (desktop), single column (mobile)
- Left: live preview with corner brackets, quick stat tiles (uptime, storage, events today, retention)
- Right: Recording mode (toggle + segmented: Continuous / Events / Schedule), AI Detection (toggle + confidence slider), Retention slider (7D-90D with tick labels), Connection info (protocol, resolution, bitrate)

**Camera Detail (Advanced — behind gear icon):**
- Collapsible sections, each with a header and expand/collapse arrow:
  - ONVIF Configuration: endpoint, username, password, profile token
  - Stream Settings: RTSP URL, sub-stream URL, snapshot URI
  - Imaging: brightness, contrast, saturation sliders
  - Detection Zones: polygon zone editor (rebuilt with new design system — existing logic preserved, new styling)
  - Recording Rules: schedule editor (rebuilt with new design system — existing logic preserved, new styling)
  - Analytics: module list, rule management
  - Relay Outputs: toggle controls for physical relays
  - Edge Recordings: import controls
  - Audio: transcode toggle, backchannel status

### Settings

**Layout:** Settings-specific left sidebar nav (180px) + content area. On mobile, sidebar becomes horizontal scrollable tabs.

**Sections:**

**System:**
- Stat tiles: version, uptime, camera count (with online/offline breakdown)
- System health indicators

**Storage:**
- Primary disk usage bar with gradient fill
- Breakdown legend: Recordings, Snapshots, System (colored dots + values)
- Health status badge
- Per-camera storage breakdown: camera name, mini usage bar, storage value
- Cleanup controls (advanced/admin)

**Users** (admin-only):
- User list with username, role badge, camera permissions count
- Add/edit/delete users
- Role assignment: admin / operator / viewer
- Camera permission picker

**Backups:**
- Export config button
- Import config with file picker
- Last backup timestamp

**Audit Log** (admin-only):
- Filterable table: timestamp, user, action, resource, IP address
- JetBrains Mono for timestamp and IP columns

## New Features (Not in Current App)

### Camera Groups
- User-created logical groupings of cameras
- Created via `+ GROUP` button in camera panel
- Groups appear as collapsible sections in camera panel
- Tapping a group filters the live/playback grid to show only that group
- Dragging a group onto the grid fills all available slots with group cameras
- A camera can belong to multiple groups
- Backend schema: `camera_groups` table (id, name, created_at) + `camera_group_members` join table (group_id, camera_id) for many-to-many relationship. The `CameraGroup` model's `cameraIds` field is populated by joining at query time.
- Deletion: confirmation dialog. Deleting a group does not delete cameras — only the grouping.

### Camera Tours
- Automated cycling through cameras on a user-defined schedule
- Created via `+ NEW` in the Tours section of the camera panel
- Configuration: tour name, list of cameras (ordered), dwell time per camera (seconds)
- Active tour: auto-advances the live view, floating pill shows tour name + stop button
- On mobile: auto-swipe animation between cameras (400ms crossfade)
- Only one tour can be active at a time. Starting a new tour stops the currently active one.
- Tour lifecycle: when the user navigates away from Live View, the tour pauses. Returning to Live View resumes the tour automatically. The floating pill persists across screens to indicate a paused tour.
- Backend: `tours` table (id, name, camera_ids JSON array preserving order, dwell_seconds, created_at). Active state is client-side only (not persisted to server).
- Deletion: confirmation dialog.

**Backend scope:** The Go backend endpoints and SQLite migrations for Camera Groups and Tours are prerequisites for these features. They are in scope for this redesign project but should be implemented before the Flutter screens that depend on them. The implementation plan should sequence backend work first.

### Drag-and-Drop Grid Assignment
- Desktop/tablet: drag cameras from panel onto grid slots
- Mobile: tap camera in bottom sheet to assign to current view, long-press for options
- Grid slots can be rearranged by dragging tiles between positions
- Empty slots show drop target styling (dashed border + label)

## Preserved Architecture

The following layers remain unchanged:

- **Models** (`models/`): Camera, RecordingSegment, MotionEvent, Bookmark, SavedClip, SearchResult, DetectionZone, AlertRule, DetectionFrame, User, RecordingRule. **Exception:** `NotificationEvent` gets an `isRead` bool field added (default false) to support read/unread styling in the Alerts panel. `NotificationState` in `notificationsProvider` is updated to track per-event read state instead of just a global `unreadCount` integer.
- **Providers** (`providers/`): authProvider, camerasProvider, recordingSegmentsProvider, motionEventsProvider, bookmarksProvider, searchProvider, notificationsProvider, detectionStreamProvider, timelineIntensityProvider, settingsProvider
- **Services** (`services/`): ApiClient, AuthService, PlaybackService, WebSocketService, WhepService, CameraPrefs
- **Router** (`router/`): GoRouter config with auth-based redirects. Route changes documented below.

### Router Changes

The existing router has 5 shell destinations: `/live`, `/playback`, `/search`, `/cameras`, `/settings`. The redesign modifies this:

- `/cameras` is renamed to `/devices` for consistency with the new "Devices" screen label
- `/devices/add` added for the Add Camera screen
- Desktop icon rail: 4 nav items (Live, Playback, Search, Devices) + 2 utility items (Alerts, Settings). Alerts is a slide-out panel, not a route.
- Mobile bottom nav: 4 items (Live, Playback, Search, Settings). Devices is accessed as a sub-section within Settings on mobile. The `_indexFromPath` mapping in `app_router.dart` must be updated for 4 destinations instead of 5.
- Breakpoints: width >= 1024px uses icon rail with push-panel. Width 600-1024px (tablet portrait) uses icon rail with overlay-panel. Width < 600px uses bottom nav with bottom-sheet panel.

## New Provider/Model Requirements

### For Camera Groups
- `CameraGroup` model (Freezed): id, name, cameraIds, createdAt
- `cameraGroupsProvider`: FutureProvider for CRUD operations
- Backend endpoints: `GET/POST/PUT/DELETE /camera-groups`

### For Camera Tours
- `Tour` model (Freezed): id, name, cameraIds (ordered), dwellSeconds, isActive, createdAt
- `toursProvider`: StateNotifierProvider managing active tour state
- `activeTourProvider`: StreamProvider emitting current camera on schedule
- Backend endpoints: `GET/POST/PUT/DELETE /tours` (CRUD only — active state is client-side, no start/stop endpoints needed)

### For Drag-and-Drop Grid
- `GridLayout` model: gridSize (NxN), slotAssignments (Map<int, String> mapping slot index to camera ID)
- `gridLayoutProvider`: StateNotifierProvider for current grid state
- Persisted via SharedPreferences, namespaced by user ID (key: `grid_layout_{userId}`) to support multi-user devices

### For Camera Panel State
- `cameraPanelProvider`: StateNotifierProvider managing isOpen, searchQuery, activeGroupFilter, scrollOffset
- Lives in the navigation shell, persists across route changes

## Empty States

All screens handle the "no data" case:

| Screen | Empty State |
|---|---|
| Live View grid | "No cameras assigned. Open the camera panel to drag cameras here." with arrow pointing to panel toggle |
| Camera Panel - groups | "No groups yet" + `+ GROUP` button inline |
| Camera Panel - tours | "No tours yet" + `+ NEW` button inline |
| Search results | "No results found. Try different search terms or adjust filters." |
| Devices list | "No cameras added. Discover cameras on your network or add one manually." + Discover/Add buttons |
| Alerts list | Centered muted icon + "No alerts" |
| Settings - audit log | "No activity recorded." |

## Error & Offline States

- **Backend unreachable:** Full-screen overlay with `danger` icon, "Cannot reach server" message, server URL in `textMuted`, and "Retry" button (primary). Auto-retry every 10 seconds with countdown.
- **Individual camera offline:** Tile shows disconnection icon, camera name, and "OFFLINE" badge in `danger`. Last known thumbnail shown at reduced opacity if available.
- **WebSocket disconnection:** Notification bell gets a `warning` dot. Reconnection is automatic with exponential backoff (existing behavior preserved).
- **API errors on actions:** Inline error message below the failed control in `danger` color, auto-dismissing after 5 seconds. No modal dialogs for non-critical errors.

## Performance Considerations

- **Live grid stream limits:** Maximum 9 simultaneous WebRTC connections (3x3). For 4x4 grids, non-focused tiles fall back to sub-stream or periodic snapshot refresh (every 2 seconds). Focused/fullscreen tile always uses the primary stream.
- **Sub-stream usage:** Camera tiles smaller than 480px width automatically request the sub-stream URL if configured, reducing bandwidth.
- **Timeline rendering:** Custom painter layers use `shouldRepaint` checks against data equality. Pinch-to-zoom debounces repaint to 16ms (60fps cap).
