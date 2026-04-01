# Plan 2: Core Screens — Live View, Playback, Search, Devices & Settings

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Rebuild every app screen from scratch using the Tactical HUD design system established in Plan 1, producing a fully functional NVR client with the new visual identity.

**Architecture:** Each screen is rebuilt independently against the new theme/widget library from Plan 1. Existing providers, services, and models are preserved. The playback timeline is a complete rewrite using a fixed-center playhead model. All screens follow progressive disclosure for advanced features.

**Tech Stack:** Flutter 3.2+, Riverpod 2.4, GoRouter 14, Material 3, HUD widget library (Plan 1)

**Spec:** `docs/superpowers/specs/2026-03-26-flutter-nvr-ui-redesign.md`

**Depends on:** Plan 1 (Foundation) must be completed first.

---

## File Structure

### New Files

| File                                                                         | Responsibility                                                          |
| ---------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| `clients/flutter/lib/screens/live_view/live_view_screen.dart`                | Rebuilt — configurable NxN grid with drag-drop targets                  |
| `clients/flutter/lib/screens/live_view/camera_tile.dart`                     | Rebuilt — HUD camera tile with corner brackets and overlays             |
| `clients/flutter/lib/screens/live_view/fullscreen_view.dart`                 | Rebuilt — fullscreen with auto-hiding overlays, PTZ, AI detection       |
| `clients/flutter/lib/screens/live_view/ptz_controls.dart`                    | Rebuilt — d-pad + vertical zoom slider, HUD styling                     |
| `clients/flutter/lib/screens/live_view/analytics_overlay.dart`               | Rebuilt — accent-colored bounding boxes with confidence labels          |
| `clients/flutter/lib/screens/playback/playback_screen.dart`                  | Rebuilt — fixed-center playhead timeline with transport controls        |
| `clients/flutter/lib/screens/playback/camera_player.dart`                    | Rebuilt — HUD-styled video player with corner brackets                  |
| `clients/flutter/lib/screens/playback/timeline/fixed_playhead_timeline.dart` | New — complete rewrite of timeline with fixed-center playhead model     |
| `clients/flutter/lib/screens/playback/timeline/timeline_painter.dart`        | New — combined CustomPainter for recording, event, bookmark layers      |
| `clients/flutter/lib/screens/playback/timeline/mini_overview_bar.dart`       | Rebuilt — 24h overview with draggable viewport window                   |
| `clients/flutter/lib/screens/playback/controls/transport_bar.dart`           | New — play/pause, step, skip, speed knob, jog, zoom selector            |
| `clients/flutter/lib/screens/search/clip_search_screen.dart`                 | Rebuilt — HUD search input, filter pills, result grid                   |
| `clients/flutter/lib/screens/search/search_result_card.dart`                 | Rebuilt — thumbnail with bounding box preview, confidence badge         |
| `clients/flutter/lib/screens/search/clip_player_sheet.dart`                  | Rebuilt — bottom sheet player with HUD styling                          |
| `clients/flutter/lib/screens/cameras/camera_list_screen.dart`                | Rebuilt — device cards with thumbnails, status, capability badges       |
| `clients/flutter/lib/screens/cameras/add_camera_screen.dart`                 | Rebuilt — discover/manual tabs with HUD styling                         |
| `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`              | Rebuilt — progressive disclosure, quick settings + advanced behind gear |
| `clients/flutter/lib/screens/settings/settings_screen.dart`                  | Rebuilt — left sidebar nav, system/storage/users/backups/audit          |
| `clients/flutter/lib/screens/settings/storage_panel.dart`                    | Rebuilt — disk usage bar, per-camera breakdown with HUD styling         |
| `clients/flutter/lib/widgets/alerts_panel.dart`                              | New — slide-out alerts panel (desktop) / bottom sheet (mobile)          |

### Files to Delete (replaced by new versions)

| File                                                                     | Reason                                     |
| ------------------------------------------------------------------------ | ------------------------------------------ |
| `clients/flutter/lib/screens/playback/timeline/composable_timeline.dart` | Replaced by `fixed_playhead_timeline.dart` |
| `clients/flutter/lib/screens/playback/timeline/timeline_viewport.dart`   | Logic absorbed into new timeline           |
| `clients/flutter/lib/screens/playback/timeline/recording_layer.dart`     | Merged into `timeline_painter.dart`        |
| `clients/flutter/lib/screens/playback/timeline/event_layer.dart`         | Merged into `timeline_painter.dart`        |
| `clients/flutter/lib/screens/playback/timeline/bookmark_layer.dart`      | Merged into `timeline_painter.dart`        |
| `clients/flutter/lib/screens/playback/timeline/intensity_layer.dart`     | Merged into `timeline_painter.dart`        |
| `clients/flutter/lib/screens/playback/timeline/grid_layer.dart`          | Merged into `timeline_painter.dart`        |
| `clients/flutter/lib/screens/playback/timeline/playhead_layer.dart`      | Absorbed into fixed playhead               |
| `clients/flutter/lib/screens/playback/timeline/interaction_layer.dart`   | Absorbed into new timeline                 |
| `clients/flutter/lib/screens/playback/controls/transport_controls.dart`  | Replaced by `transport_bar.dart`           |
| `clients/flutter/lib/screens/playback/controls/jog_slider.dart`          | Replaced by analog slider in transport bar |
| `clients/flutter/lib/widgets/adaptive_layout.dart`                       | Replaced by `NavigationShell` in Plan 1    |
| `clients/flutter/lib/widgets/camera_status_badge.dart`                   | Replaced by `StatusBadge` from HUD library |

---

## Tasks

### Task 1: Rebuild Live View Screen

**Files:**

- Modify: `clients/flutter/lib/screens/live_view/live_view_screen.dart`

- [ ] **Step 1: Rebuild LiveViewScreen**

Rebuild preserving `camerasProvider` usage. New structure:

- Top bar: page title ("Live View" in `pageTitle` style), active group badge (accent pill), grid size `HudSegmentedControl` (1×1, 2×2, 3×3, 4×4)
- Grid: `GridView.builder` with configurable column count
- Each occupied slot: new `CameraTile` widget
- Each empty slot: dashed border with `border` color, plus icon, "DROP HERE" label (JetBrains Mono)
- Remove AppBar — top bar is a custom Container within the screen body
- Responsive column count: use `HudSegmentedControl` selection, not MediaQuery-based auto-calculation

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/screens/live_view/live_view_screen.dart
git commit -m "feat(ui): rebuild LiveViewScreen with HUD grid and segmented control"
```

---

### Task 2: Rebuild Camera Tile

**Files:**

- Modify: `clients/flutter/lib/screens/live_view/camera_tile.dart`

- [ ] **Step 1: Rebuild CameraTile with HUD styling**

Preserve WebRTC connection logic (`WhepConnection`, state management, retry). Replace UI:

- Wrap video in `CornerBrackets` widget
- Top-left: status dot + `LIVE` label (green, JetBrains Mono)
- Top-right: `REC` badge or `MOTION` badge when applicable
- Bottom-left: camera name (IBM Plex Sans, 10px)
- Bottom-right: timestamp (JetBrains Mono, 8px)
- Background: `bgSecondary` with `border` outline, 6px radius
- Offline state: disconnection icon centered, camera name, "OFFLINE" `StatusBadge`
- Tap → navigate to fullscreen. GestureDetector wrapping the tile.

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/screens/live_view/camera_tile.dart
git commit -m "feat(ui): rebuild CameraTile with corner brackets and HUD overlays"
```

---

### Task 3: Rebuild Fullscreen View

**Files:**

- Modify: `clients/flutter/lib/screens/live_view/fullscreen_view.dart`
- Modify: `clients/flutter/lib/screens/live_view/ptz_controls.dart`
- Modify: `clients/flutter/lib/screens/live_view/analytics_overlay.dart`

- [ ] **Step 1: Rebuild FullscreenView**

Preserve WebRTC connection, auto-hide timer logic, system chrome management. Replace UI:

- Full-bleed video with `CornerBrackets` (larger 24px brackets)
- Top gradient overlay (fading from black): status, camera name, ID, REC, timestamp, stream metadata
- Bottom gradient overlay: Audio toggle, AI toggle (`HudToggle`), Snapshot, Grid return, Exit buttons — all in semi-transparent `bgSecondary` pill containers with `border`
- Auto-hide overlays: 3s delay, 200ms fade (use `NvrAnimations` constants)
- Swipe left/right to cycle cameras (preserve or add `PageView` navigation)

- [ ] **Step 2: Rebuild PTZ Controls with HUD styling**

Replace button layout with:

- D-pad: 4 directional arrows + center home button. Semi-transparent backgrounds with `border` outline, rounded 6px. Centered amber dot for home.
- Vertical zoom slider: `AnalogSlider` rotated 90°, or custom vertical implementation with `ZOOM` label and value readout.
- All controls have `backdrop-filter` blur effect (use `ClipRRect` + `BackdropFilter`)

- [ ] **Step 3: Rebuild AnalyticsOverlay**

Preserve polling/WebSocket detection logic. Change bounding box rendering:

- Box color: `NvrColors.accent` (amber) instead of class-based colors
- Label: class name + confidence % on `accent` background pill above box
- Font: JetBrains Mono, 8px, bold

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/screens/live_view/
git commit -m "feat(ui): rebuild fullscreen view, PTZ controls, analytics overlay"
```

---

### Task 4: Build Fixed-Center Playhead Timeline (HIGH RISK)

This is the most complex component. Build and test it independently before integrating into the playback screen.

**Files:**

- Create: `clients/flutter/lib/screens/playback/timeline/fixed_playhead_timeline.dart`
- Create: `clients/flutter/lib/screens/playback/timeline/timeline_painter.dart`

- [ ] **Step 1: Create TimelinePainter (combined CustomPainter)**

Create `clients/flutter/lib/screens/playback/timeline/timeline_painter.dart`:

This painter renders all timeline layers in a single paint call:

1. Time grid (tick marks + labels)
2. Recording segments (amber at 20% opacity bars)
3. Motion/event intensity (red at varying opacity)
4. Bookmarks (amber bookmark icons)

Interface:

```dart
class TimelinePainter extends CustomPainter {
  TimelinePainter({
    required this.visibleStart,    // Duration from midnight
    required this.visibleEnd,      // Duration from midnight
    required this.segments,        // List<RecordingSegment>
    required this.events,          // List<MotionEvent>
    required this.bookmarks,       // List<Bookmark>
    required this.intensityBuckets, // List<int> bucket counts
    required this.bucketDuration,  // Duration per bucket
  });
  // ...
}
```

Key methods:

- `_drawTickMarks(Canvas, Size)` — major ticks every 5min with labels, minor ticks every 1min
- `_drawRecordings(Canvas, Size)` — amber bars for recording periods, hash pattern for gaps
- `_drawEvents(Canvas, Size)` — red intensity heat map
- `_drawBookmarks(Canvas, Size)` — amber bookmark triangles

Use `shouldRepaint` with data equality checks.

- [ ] **Step 2: Create FixedPlayheadTimeline widget**

Create `clients/flutter/lib/screens/playback/timeline/fixed_playhead_timeline.dart`:

Core state:

- `_scrollOffset` (double) — pixels of timeline scroll. The time at center = `_scrollOffset / pixelsPerSecond`
- `_pixelsPerSecond` (double) — zoom level. Zoom presets: 1H → small pps, 5M → large pps
- `_isDragging` (bool) — whether playhead is being dragged
- `_zoomLevel` enum — `oneHour`, `thirtyMin`, `tenMin`, `fiveMin`

Interaction model:

1. **At rest / playing:** `_scrollOffset` auto-increments via `AnimationController` at playback speed
2. **Grab playhead:** Stop auto-scroll, track `GestureDetector.onHorizontalDragUpdate` → adjust `_scrollOffset`
3. **Tap to seek:** Animate `_scrollOffset` to put tapped point at center (300ms, easeInOut)
4. **Pinch zoom / scroll wheel:** Scale `_pixelsPerSecond`, recalculate `_scrollOffset` to keep center time fixed

Widget tree:

```
Column(
  MiniOverviewBar(...)         // 24h overview with viewport window
  SizedBox(height: 8)
  Stack(
    ClipRect(
      CustomPaint(TimelinePainter(...))   // Scrolling content — positioned via _scrollOffset
    )
    Center(                               // Fixed playhead at exact center
      _PlayheadWidget(isDragging: ...)
    )
  )
)
```

The playhead renders:

- 2px vertical line in `accent` with glow shadow
- 16px circular handle (grows to 20px when dragging) with `accent` fill, `bgPrimary` border
- Time badge above: `accent` background, `bgPrimary` text, JetBrains Mono

Callbacks:

- `onPositionChanged(Duration position)` — emitted whenever the time under the playhead changes
- `onDragStart()` / `onDragEnd()` — for pausing/resuming playback
- `onZoomChanged(Duration visibleRange)` — for adjusting data fetching granularity

- [ ] **Step 3: Test timeline renders and scrolls correctly**

Build the timeline in isolation with mock data. Verify:

- Tick marks render at correct intervals for each zoom level
- Recording bars align with time positions
- Playhead stays centered during auto-scroll
- Drag gesture moves the timeline, not the playhead
- Tap-to-seek animates smoothly
- Pinch zoom changes scale while keeping center time

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/screens/playback/timeline/fixed_playhead_timeline.dart
git add clients/flutter/lib/screens/playback/timeline/timeline_painter.dart
git commit -m "feat(ui): add fixed-center playhead timeline component"
```

---

### Task 5: Rebuild Mini Overview Bar

**Files:**

- Modify: `clients/flutter/lib/screens/playback/timeline/mini_overview_bar.dart`

- [ ] **Step 1: Rebuild MiniOverviewBar**

Preserve the painting logic but update styling:

- Container: 10px height, `bgSecondary` background, 4px radius, `border` outline
- Recording bars: `accent` at 13% opacity
- Viewport window: rectangle with `accent` border (1.5px), `accent` fill at 13% opacity, grabbable cursor
- Time labels: `00:00` and `24:00` in JetBrains Mono, `textMuted`
- Drag viewport → callback updates `FixedPlayheadTimeline._scrollOffset`

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/screens/playback/timeline/mini_overview_bar.dart
git commit -m "feat(ui): rebuild MiniOverviewBar with HUD styling"
```

---

### Task 6: Build Transport Bar

**Files:**

- Create: `clients/flutter/lib/screens/playback/controls/transport_bar.dart`

- [ ] **Step 1: Create TransportBar**

```dart
class TransportBar extends StatelessWidget {
  const TransportBar({
    super.key,
    required this.isPlaying,
    required this.currentTime,
    required this.speed,
    required this.zoomLevel,
    required this.onPlayPause,
    required this.onStepBack,
    required this.onStepForward,
    required this.onSkipPrevious,
    required this.onSkipNext,
    required this.onSpeedChanged,
    required this.onZoomChanged,
  });
  // ...
}
```

Layout (left to right):

1. Skip back, Step back, Play/Pause (36px, `accent` fill), Step forward, Skip forward — all 28px `bgSecondary` buttons
2. Speed `RotaryKnob` (28px, values: 0.25, 0.5, 1, 2, 4, 8)
3. Divider (1px `border`)
4. Current time: large JetBrains Mono in `accent` (13px)
5. Spacer
6. Zoom `HudSegmentedControl` (1H / 30M / 10M / 5M)

Background: `bgPrimary` with top `border`.

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/screens/playback/controls/transport_bar.dart
git commit -m "feat(ui): add TransportBar with analog controls"
```

---

### Task 7: Rebuild Playback Screen

**Files:**

- Modify: `clients/flutter/lib/screens/playback/playback_screen.dart`
- Modify: `clients/flutter/lib/screens/playback/camera_player.dart`

- [ ] **Step 1: Rebuild PlaybackScreen**

Preserve `PlaybackController` usage, segment/event/bookmark provider connections. Replace UI:

- Top bar: "Playback" title, date picker (HUD styled — `accent` calendar icon + JetBrains Mono date), Export button, Bookmark button, grid selector
- Video area: `CameraPlayer` wrapped in `CornerBrackets`
- Transport bar: new `TransportBar` component
- Timeline: new `FixedPlayheadTimeline` with `MiniOverviewBar`
- Wire up callbacks: `TransportBar` ↔ `PlaybackController`, `FixedPlayheadTimeline.onPositionChanged` → `PlaybackController.seek`

- [ ] **Step 2: Rebuild CameraPlayer**

Replace styling while preserving `VideoPlayer` integration:

- `CornerBrackets` around video
- Camera name top-left, timestamp top-right (JetBrains Mono, `accent`)
- Detection event marker at bottom (amber pill with event details)

- [ ] **Step 3: Delete old timeline files**

```bash
rm clients/flutter/lib/screens/playback/timeline/composable_timeline.dart
rm clients/flutter/lib/screens/playback/timeline/timeline_viewport.dart
rm clients/flutter/lib/screens/playback/timeline/recording_layer.dart
rm clients/flutter/lib/screens/playback/timeline/event_layer.dart
rm clients/flutter/lib/screens/playback/timeline/bookmark_layer.dart
rm clients/flutter/lib/screens/playback/timeline/intensity_layer.dart
rm clients/flutter/lib/screens/playback/timeline/grid_layer.dart
rm clients/flutter/lib/screens/playback/timeline/playhead_layer.dart
rm clients/flutter/lib/screens/playback/timeline/interaction_layer.dart
rm clients/flutter/lib/screens/playback/controls/transport_controls.dart
rm clients/flutter/lib/screens/playback/controls/jog_slider.dart
```

- [ ] **Step 4: Commit**

```bash
git add -A clients/flutter/lib/screens/playback/
git commit -m "feat(ui): rebuild playback screen with fixed-center playhead timeline"
```

---

### Task 8: Rebuild Search Screen

**Files:**

- Modify: `clients/flutter/lib/screens/search/clip_search_screen.dart`
- Modify: `clients/flutter/lib/screens/search/search_result_card.dart`
- Modify: `clients/flutter/lib/screens/search/clip_player_sheet.dart`

- [ ] **Step 1: Rebuild ClipSearchScreen**

Preserve `searchProvider` usage. Replace UI:

- Header: "Search" title
- Input: full-width with search icon, HUD styling. Search button (primary)
- Filter pills row: Camera dropdown, Time range presets (amber active state), Confidence threshold
- Results area: count + sort label (JetBrains Mono), responsive grid of `SearchResultCard`s
- Empty states per spec (pre-search, no results, loading, error)

- [ ] **Step 2: Rebuild SearchResultCard**

- Card with `bgSecondary` background, `border` outline, 6px radius
- Thumbnail (16:9 aspect ratio) with bounding box outline in `accent`
- Confidence badge top-right: `accent` background, JetBrains Mono bold
- Below thumbnail: camera name, detection class (JetBrains Mono, `accent`), timestamp

- [ ] **Step 3: Rebuild ClipPlayerSheet**

- Bottom sheet with `bgSecondary` background
- Video player with `CornerBrackets`
- "Jump to playback" button (primary)

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/screens/search/
git commit -m "feat(ui): rebuild search screen with HUD filter pills and result grid"
```

---

### Task 9: Rebuild Devices Screen

**Files:**

- Modify: `clients/flutter/lib/screens/cameras/camera_list_screen.dart`

- [ ] **Step 1: Rebuild CameraListScreen as device list**

Preserve `camerasProvider` and delete logic. Replace UI:

- Header: "Devices" title, camera count (JetBrains Mono), Discover button (secondary), Add Camera button (primary)
- Device cards: `bgSecondary` background, `border` outline, 8px radius
  - Left: 80x48px thumbnail with mini corner brackets
  - Middle: camera name + status badge (`StatusBadge` factory), connection metadata (JetBrains Mono)
  - Right: capability badges (PTZ, AI, REC — small pills), chevron
- Status differentiation per spec (online/degraded/offline with opacity/border changes)
- Tap → navigate to `/devices/:id`

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/screens/cameras/camera_list_screen.dart
git commit -m "feat(ui): rebuild Devices list with HUD device cards"
```

---

### Task 10: Rebuild Camera Detail Screen

**Files:**

- Modify: `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`

- [ ] **Step 1: Rebuild CameraDetailScreen with progressive disclosure**

Preserve all API call logic (\_fetchCamera, \_save methods, ONVIF probe). Major UI restructure:

**Default view (no tabs):**

- Header: back button, camera name, status badge, gear icon (advanced toggle)
- Two columns (desktop) / single column (mobile):
  - Left: live preview with `CornerBrackets`, quick stat tiles (uptime, storage, events, retention — `bgSecondary` cards with JetBrains Mono large values)
  - Right: Recording mode (`HudToggle` + `HudSegmentedControl`: Continuous/Events/Schedule), AI Detection (`HudToggle` + `AnalogSlider` for confidence), Retention (`AnalogSlider` with tick labels 7D-90D), Connection info (key-value pairs)

**Advanced view (behind gear icon):**

- Collapsible `ExpansionTile`-style sections with JetBrains Mono section headers
- Each section: ONVIF Config, Stream Settings, Imaging (3 `AnalogSlider`s), Detection Zones, Recording Rules, Analytics, Relay Outputs, Edge Recordings, Audio

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/screens/cameras/camera_detail_screen.dart
git commit -m "feat(ui): rebuild Camera Detail with progressive disclosure"
```

---

### Task 11: Rebuild Add Camera Screen

**Files:**

- Modify: `clients/flutter/lib/screens/cameras/add_camera_screen.dart`

- [ ] **Step 1: Rebuild AddCameraScreen**

Preserve discovery polling and manual form submission logic. Replace UI:

- `HudSegmentedControl` for tab switching (Discover / Manual) instead of TabBar
- Discover tab: "Scan Network" button (primary), pulsing amber ring animation during scan, result cards with IP/model/capabilities, "Add" button per card
- Manual tab: form fields with JetBrains Mono labels, "Test Connection" button (secondary), "Add Camera" button (primary)

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/screens/cameras/add_camera_screen.dart
git commit -m "feat(ui): rebuild Add Camera screen with HUD styling"
```

---

### Task 12: Rebuild Settings Screen

**Files:**

- Modify: `clients/flutter/lib/screens/settings/settings_screen.dart`
- Modify: `clients/flutter/lib/screens/settings/storage_panel.dart`
- Modify: `clients/flutter/lib/screens/settings/user_management_screen.dart`
- Modify: `clients/flutter/lib/screens/settings/backup_panel.dart`
- Modify: `clients/flutter/lib/screens/settings/audit_panel.dart`

- [ ] **Step 1: Rebuild SettingsScreen layout**

Replace TabBar with left sidebar navigation (180px) on desktop, horizontal scrollable tabs on mobile:

- Sidebar: `bgSecondary` background with `border` right edge
- Items: System, Storage, Users, Backups, Audit Log
- Active: `accent` at 7% background + 2px `accent` right border
- Content area fills remaining space

- [ ] **Step 2: Rebuild System tab**

- Stat tiles (3-column grid): Version, Uptime, Cameras (with online/offline count)
- JetBrains Mono for values, `textMuted` for labels
- `bgSecondary` cards with `border` outline

- [ ] **Step 3: Rebuild Storage panel**

- Disk usage: `AnalogSlider`-style bar (read-only) with gradient fill
- Breakdown legend: colored dots (Recordings=accent, Snapshots=blue, System=gray) + values
- Health badge: `StatusBadge`
- Per-camera breakdown: name + mini bar + value in a list

- [ ] **Step 4: Rebuild other panels (Users, Backups, Audit)**

Apply HUD styling to each:

- Users: list with role badges, CRUD dialogs with HUD buttons
- Backups: export/import buttons (primary/secondary)
- Audit: table with JetBrains Mono timestamp/IP columns

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/screens/settings/
git commit -m "feat(ui): rebuild Settings with sidebar nav and HUD panels"
```

---

### Task 13: Build Alerts Panel

**Files:**

- Create: `clients/flutter/lib/widgets/alerts_panel.dart`
- Modify: `clients/flutter/lib/models/notification_event.dart`
- Modify: `clients/flutter/lib/providers/notifications_provider.dart`

- [ ] **Step 1: Add isRead field to NotificationEvent**

In `clients/flutter/lib/models/notification_event.dart` (this is a plain Dart class, NOT Freezed), add:

```dart
final bool isRead;
```

Add `this.isRead = false` to the constructor. Update `fromJson` to read `isRead` with a default of `false`. Also add a `copyWith` method since the class is immutable and we need to toggle read state:

```dart
NotificationEvent copyWith({bool? isRead}) {
  return NotificationEvent(
    type: type, camera: camera, message: message, time: time,
    zone: zone, className: className, action: action,
    trackId: trackId, confidence: confidence,
    isRead: isRead ?? this.isRead,
  );
}
```

- [ ] **Step 2: Update NotificationsNotifier for per-event read state**

In `clients/flutter/lib/providers/notifications_provider.dart`:

- Change `markAllRead()` to set `isRead = true` on all events in history
- Add `markRead(int index)` for individual event read
- Recompute `unreadCount` from the list

- [ ] **Step 3: Create AlertsPanel widget**

Create `clients/flutter/lib/widgets/alerts_panel.dart`:

Desktop: right-side slide-out panel (300px, overlay). Mobile: full-screen bottom sheet.

```dart
class AlertsPanel extends ConsumerWidget {
  // Header: ALERTS label, MARK ALL READ button, close X
  // Notification list: scrollable, most recent first
  // Each item: status dot, camera name, message, timestamp
  // Unread: bgTertiary background. Read: bgSecondary
  // Tap → navigate to relevant screen
  // Empty state: centered icon + "No alerts" text
}
```

- [ ] **Step 4: Wire AlertsPanel into NavigationShell**

Update `clients/flutter/lib/widgets/shell/navigation_shell.dart` to show the AlertsPanel when the alerts icon is tapped:

```dart
void _onAlertsTap(BuildContext context) {
  // Desktop: show overlay panel
  // Mobile: show modal bottom sheet
}
```

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/widgets/alerts_panel.dart
git add clients/flutter/lib/models/notification_event.dart
git add clients/flutter/lib/providers/notifications_provider.dart
git add clients/flutter/lib/widgets/shell/navigation_shell.dart
git commit -m "feat(ui): add Alerts panel with per-event read tracking"
```

---

### Task 14: Delete Old Files and Clean Up

- [ ] **Step 1: Remove replaced widget files**

```bash
rm clients/flutter/lib/widgets/adaptive_layout.dart
rm clients/flutter/lib/widgets/camera_status_badge.dart
rm clients/flutter/lib/widgets/notification_bell.dart
rm clients/flutter/lib/widgets/notification_toast.dart
```

- [ ] **Step 2: Remove old notification_bell references**

Search for and remove any imports of the deleted files:

```bash
cd clients/flutter && grep -rl "notification_bell\|camera_status_badge\|adaptive_layout\|notification_toast" lib/ | head -20
```

Replace with new imports where needed.

- [ ] **Step 3: Run Flutter analyze and fix issues**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 4: Verify build**

```bash
cd clients/flutter && flutter build apk --debug 2>&1 | tail -5
```

- [ ] **Step 5: Commit**

```bash
git add -A clients/flutter/
git commit -m "chore(ui): remove replaced widgets, fix imports, clean up"
```

---

### Task 15: Run code generation

- [ ] **Step 1: Run build_runner for any Freezed models touched**

Note: `NotificationEvent` is NOT a Freezed model (plain Dart class), so it does not need code generation. Only run this if other Freezed models were modified during this plan.

```bash
cd clients/flutter && dart run build_runner build --delete-conflicting-outputs
```

- [ ] **Step 2: Verify no generation errors**

Check output for errors. Fix any issues.

- [ ] **Step 3: Commit if any generated files changed**

```bash
git add clients/flutter/lib/models/*.freezed.dart clients/flutter/lib/models/*.g.dart
git commit -m "chore: regenerate Freezed model code"
```
