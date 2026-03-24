# Playback Timeline & VCR Controls Redesign

## Problem

The playback page has two core issues:
1. The timeline doesn't accurately show where recordings exist or display events in a useful way
2. The VCR controls don't work — they update local state but aren't wired to the actual video player

## Approach

Replace the monolithic `CustomPaint` timeline with composable layers sharing a viewport transform. Replace the simple control bar with full transport controls, a jog slider, and a shared `PlaybackController` that wraps `media_kit` and becomes the single source of truth.

---

## 1. PlaybackController

A `ChangeNotifier` wrapping `media_kit`'s `Player`, provided via `ChangeNotifierProvider` scoped to `PlaybackScreen`.

**State:**
- `position` (Duration) — synced from player's position stream
- `isPlaying` (bool)
- `speed` (double)
- `selectedDate` (DateTime)
- `selectedCameraIds` (List<String>)

**Methods:**
- `seek(Duration position)` — seeks player, updates position
- `play()` / `pause()` / `togglePlayPause()`
- `setSpeed(double speed)` — sets playback rate on the player
- `stepFrame(int direction)` — for forward: use `media_kit`'s native `player.step()` (mpv frame-step). For backward: seek to nearest prior keyframe (~2-5s back depending on GOP). True single-frame reverse is not supported by mpv.
- `skipToNextEvent()` / `skipToPreviousEvent()` — binary search sorted events list, seek to nearest
- `skipToNextGap()` / `skipToPreviousGap()` — find nearest recording gap, seek to gap boundary

**Position sync:**
- Subscribes to `player.stream.position`, throttled to ~15fps (~66ms)
- Sets `_isSeeking = true` during seeks to prevent playhead jitter from stale stream updates
- Resumes listening after player confirms seek complete

**Multi-player management:**
- The controller owns a `Map<String, Player>` keyed by camera ID
- `position` on the controller is the "master" position — all players are kept in sync to this
- When `seek()` is called, all players seek to the same position
- When cameras are added/removed from selection, players are created/disposed accordingly
- Position stream is read from the first active player; others follow

**Edge cases:**
- Seeking into a gap: controller has the full recording segments list, so before seeking, check if target falls within any segment. If it falls in a gap, immediately seek to the next segment's start. This is deterministic and instant — no timeout heuristic.
- End of recordings: auto-pause at last segment's end time
- Position clamped to 0–86400s (one day), no date wrapping

---

## 2. Timeline Architecture — Composable Layers

### TimelineViewport

Manages the visible time window within 0–24h:
- `visibleStart` / `visibleEnd` (Duration)
- `zoomLevel` (1.0 = full 24h, max ~60x = ~24min visible)
- `panOffset` — scroll position within zoom
- `timeToPixel(Duration)` / `pixelToTime(double)` converters
- Handles pinch-to-zoom and scroll/pan gestures

### Layer Stack (bottom to top)

1. **GridLayer** — Hour/minute grid lines and time labels. Adapts density to zoom: 3h labels at 1x, 15min zoomed in, 5min/1min further.

2. **RecordingLayer** — Filled rectangular bars for each recording segment using actual `startTime` + `endTime` from backend. Semi-transparent accent color. Gaps are visually obvious as empty space.

3. **EventLayer** — Event duration bars (not just dots). Each event renders as a thin horizontal bar color-coded by class:
   - Person ("person") → Blue
   - Vehicle ("car", "truck", "bus", "motorcycle") → Green
   - Motion ("motion") → Amber
   - Other (any unrecognized `object_class`) → Red

   At low zoom, events collapse to dots. At high zoom, duration bars become visible.

4. **PlayheadLayer** — Current position indicator (line + circular handle). Draggable. Calls `controller.seek()` on drag. Handle animates on drag start/end.

5. **InteractionLayer** — Transparent hit-test layer handling:
   - Tap → seek to time
   - Long-press on event → show detail popup
   - Pinch → zoom
   - Pan → scroll through time

### MiniOverviewBar

A 32px-tall bar always visible above the main timeline:
- Shows full 24h with tiny recording bars and event markers
- Highlighted rectangle shows currently visible range
- Drag the highlighted rectangle to pan the main view proportionally
- Tap outside the rectangle to jump the viewport to that time region

### Orientation

Horizontal only. Eliminates the dual vertical/horizontal complexity of the current implementation.

---

## 3. Transport Controls & Jog Slider

### TransportControls

A row of icon buttons (left to right):

| Button | Icon | Action |
|--------|------|--------|
| Previous gap | `skip_previous` | `controller.skipToPreviousGap()` |
| Previous event | `arrow_back` | `controller.skipToPreviousEvent()` |
| Frame back | `chevron_left` | `controller.stepFrame(-1)` |
| Play/Pause | `play_arrow`/`pause` | `controller.togglePlayPause()` |
| Frame forward | `chevron_right` | `controller.stepFrame(1)` |
| Next event | `arrow_forward` | `controller.skipToNextEvent()` |
| Next gap | `skip_next` | `controller.skipToNextGap()` |

Frame step buttons auto-pause if playing. Tooltips on each button.

### JogSlider

Horizontal slider for variable-speed scrubbing:
- Range: -2.0x to +2.0x
- Center (0.0) = paused
- Right = forward at proportional speed, left = reverse. Try `player.setRate(-x)` for native reverse first; if unsupported, fall back to timer-based backward seeks (every 100ms, seek backward by `jog_position * 200ms` — so at full -2x, it seeks back 400ms every 100ms)
- On release: spring animation back to center (paused)
- Speed label displayed above thumb (e.g. "-1.2x", "+0.5x")
- Separate from the normal speed dropdown (1x, 2x, 4x, etc.) which remains for standard playback
- When jog slider is active, `_isSeeking` stays true to suppress position stream jitter from rapid seeks

### Layout

**Narrow screen (< 720px):**
```
[ Video (single camera)              ]
[ MiniOverviewBar                    ]
[ Main Timeline (expandable height)  ]
[ Transport Controls row             ]
[ Jog Slider                         ]
[ Speed: 1.0x dropdown               ]
```

**Wide screen (>= 720px):**
```
[ Video Grid (1-4 cameras)           ]
[ MiniOverviewBar                    ]
[ Main Timeline                      ]
[ Transport | Jog | Speed            ]
```

Timeline is always horizontal below the video in both layouts. The current vertical sidebar layout is removed.

---

## 4. Event Detail Popup

Triggered by long-press on an event marker in the timeline.

**Content:**
- Thumbnail (120x80px) from event's `thumbnailPath`, placeholder if none
- Event type label (e.g. "Person detected")
- Object class + confidence (e.g. "person (92%)")
- Time range: start–end in HH:MM:SS
- Duration
- "Play from here" button → `controller.seek()` to event start

**Implementation:** `OverlayEntry` positioned near the tapped marker. Dismisses on tap outside. Card style with rounded corners, slight elevation, dark background.

**Data source:** `EventLayer` has the full `MotionEvent` list. Identifies nearest event via `pixelToTime()`.

---

## 5. Recording Model Fix

**Problem:** Flutter `RecordingSegment` only parses `start`. Backend already returns `startTime`, `endTime`, `durationMs`.

**Fix:** Update model to match backend JSON keys:
```dart
class RecordingSegment {
  final int id;
  final String cameraId;    // json: "camera_id"
  final String startTime;   // json: "start_time" (ISO8601)
  final String endTime;     // json: "end_time" (ISO8601)
  final int durationMs;     // json: "duration_ms"
  final String? filePath;   // json: "file_path"

  factory RecordingSegment.fromJson(Map<String, dynamic> json) => RecordingSegment(
    id: json['id'],
    cameraId: json['camera_id'],
    startTime: json['start_time'],
    endTime: json['end_time'],
    durationMs: json['duration_ms'],
    filePath: json['file_path'],
  );
}
```

Eliminates the guesswork of estimating duration from the next segment or hardcoding +15min.

**Provider fixes (pre-existing bugs):**
- `recordingSegmentsProvider` — timestamps must include timezone for RFC3339 compliance. Change `'${key.date}T00:00:00'` to `'${key.date}T00:00:00Z'` (and same for end time). Parse full response fields.
- `motionEventsProvider` — backend expects a `date` query param (YYYY-MM-DD), not `start`/`end`. Change to send `queryParameters: {'date': key.date}` instead.
- Both passed into `PlaybackController` for skip-to-event and skip-to-gap logic

**Playback URL:** The backend serves a continuous stream for the full requested duration. `player.seek()` works within the stream — no need to reconstruct the URL on every seek. The current `ValueKey` that includes `position.inSeconds` (causing player recreation on every seek) must be changed to exclude position.

---

## 6. Playhead-Player Sync

**Seek flow:**
1. User interacts (drag playhead, tap timeline, jog slider, skip button)
2. Widget calls `controller.seek(duration)`
3. Controller sets `_isSeeking = true`, ignores position stream
4. Controller calls `player.seek(duration)`
5. Seek completes → `_isSeeking = false`, resume position stream

**Time display:** `Text` widget showing `HH:MM:SS` from controller position, plus selected date.

**Loading states:** While recording segments or motion events are loading (AsyncValue.loading), the RecordingLayer and EventLayer show a subtle shimmer/skeleton animation across the timeline area. The GridLayer and PlayheadLayer render immediately since they don't depend on data.

---

## Files Affected

### New files:
- `lib/screens/playback/playback_controller.dart` — PlaybackController
- `lib/screens/playback/timeline/timeline_viewport.dart` — viewport transform
- `lib/screens/playback/timeline/grid_layer.dart`
- `lib/screens/playback/timeline/recording_layer.dart`
- `lib/screens/playback/timeline/event_layer.dart`
- `lib/screens/playback/timeline/playhead_layer.dart`
- `lib/screens/playback/timeline/interaction_layer.dart`
- `lib/screens/playback/timeline/mini_overview_bar.dart`
- `lib/screens/playback/timeline/composable_timeline.dart` — assembles the stack
- `lib/screens/playback/controls/transport_controls.dart`
- `lib/screens/playback/controls/jog_slider.dart`
- `lib/screens/playback/event_detail_popup.dart`

### Modified files:
- `lib/models/recording.dart` — update RecordingSegment model
- `lib/screens/playback/playback_screen.dart` — rewire to use PlaybackController
- `lib/screens/playback/camera_player.dart` — receive controller instead of raw state
- `lib/providers/recordings_provider.dart` — parse full recording response

### Deleted files:
- `lib/screens/playback/timeline_widget.dart` — replaced by composable timeline
- `lib/screens/playback/playback_controls.dart` — replaced by transport controls + jog slider
