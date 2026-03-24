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
- `stepFrame(int direction)` — pause, then seek +/-33ms (~1 frame at 30fps)
- `skipToNextEvent()` / `skipToPreviousEvent()` — binary search sorted events list, seek to nearest
- `skipToNextGap()` / `skipToPreviousGap()` — find nearest recording gap, seek to gap boundary

**Position sync:**
- Subscribes to `player.stream.position`, throttled to ~15fps (~66ms)
- Sets `_isSeeking = true` during seeks to prevent playhead jitter from stale stream updates
- Resumes listening after player confirms seek complete

**Edge cases:**
- Seeking into a gap: detect no position progress for 2s, auto-advance to next recording segment
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
   - Person → Blue
   - Vehicle → Green
   - Motion → Amber
   - Other → Red

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
- Draggable to pan the main view

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
- Right = forward, left = reverse (reverse simulated via rapid backward seeks if `media_kit` doesn't support native reverse)
- On release: spring animation back to center (paused)
- Speed label displayed above thumb
- Separate from the normal speed dropdown (1x, 2x, 4x, etc.) which remains for standard playback

### Layout

**Narrow screen:**
```
[ MiniOverviewBar                    ]
[ Main Timeline (expandable height)  ]
[ Transport Controls row             ]
[ Jog Slider                         ]
[ Speed: 1.0x dropdown               ]
```

**Wide screen:**
```
[ MiniOverviewBar          ]  [ Video Grid    ]
[ Main Timeline (tall)     ]  [               ]
[ Transport | Jog | Speed  ]  [               ]
```

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

**Fix:** Update model to:
```dart
class RecordingSegment {
  final int id;
  final String cameraId;
  final String startTime;  // ISO8601
  final String endTime;    // ISO8601
  final int durationMs;
  final String? filePath;
}
```

Eliminates the guesswork of estimating duration from the next segment or hardcoding +15min.

**Provider changes:**
- `recordingSegmentsProvider` — same API, parse full response
- `motionEventsProvider` — no change
- Both passed into `PlaybackController` for skip-to-event and skip-to-gap logic

---

## 6. Playhead-Player Sync

**Seek flow:**
1. User interacts (drag playhead, tap timeline, jog slider, skip button)
2. Widget calls `controller.seek(duration)`
3. Controller sets `_isSeeking = true`, ignores position stream
4. Controller calls `player.seek(duration)`
5. Seek completes → `_isSeeking = false`, resume position stream

**Time display:** `Text` widget showing `HH:MM:SS` from controller position, plus selected date.

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
