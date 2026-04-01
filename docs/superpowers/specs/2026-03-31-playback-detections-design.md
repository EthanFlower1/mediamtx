# Playback Detection Overlay & Timeline Event Markers

**Date:** 2026-03-31
**Status:** Approved

## Problem

Two features are missing from the playback screen:

1. **Timeline event markers:** The `TimelinePainter` receives `List<MotionEvent>` but never renders them. Only aggregated intensity buckets are painted. Individual events with their `objectClass`, `eventType`, and time spans are invisible.

2. **Playback detection overlay:** The live view renders bounding boxes via `AnalyticsOverlay` + `detectionStreamProvider`, but the playback `CameraPlayer` has no overlay at all. Per-frame detection data (bounding boxes with `frame_time`, `class`, `confidence`, normalized coordinates) exists in the `detections` table but isn't exposed for playback consumption.

## Design Decisions

- **Approach:** Dedicated playback overlay widget. Don't touch live view code.
- **Timeline:** Keep intensity heat map, add individual event markers in a new lane below it. Color-coded by `objectClass`.
- **API:** New per-camera endpoint for fetching detections by time range.
- **Data strategy:** Batch prefetch all detections when a recording segment loads. Cache client-side, binary search by `frame_time` during playback.
- **Overlay toggle:** Only visible when detections exist for the current segment. Manual on/off per camera.
- **Frame matching:** Nearest frame within 500ms tolerance. No interpolation.

## Section 1: Backend — New API Endpoint

### New Route

`GET /api/nvr/cameras/:id/detections?start=RFC3339&end=RFC3339`

Returns all `Detection` rows for the given camera within the time range. Joins through `motion_events` to filter by `camera_id` (same pattern as `GetRecentDetections`). No embedding field in the response (already excluded via `json:"-"`).

### New DB Function

`QueryDetectionsByTimeRange(cameraID string, start, end time.Time) ([]*Detection, error)`

Similar to `GetRecentDetections` but with an explicit time range instead of "last 2 seconds". Returns `[]*Detection` ordered by `frame_time ASC` (ascending for playback consumption — the client indexes sequentially).

### Handler

Added to `CameraHandler` in `cameras.go`, registered in `router.go` under the existing `/cameras/:id/` group. Standard JWT auth, same permission check pattern as `LatestDetections`.

## Section 2: Flutter — Detection Data Flow for Playback

### Model

New `PlaybackDetection` class in `models/detection_frame.dart` (alongside existing `DetectionBox`):

- `frameTime` (`DateTime`) — timestamp for binary search lookup
- `className` (`String`)
- `confidence` (`double`)
- `x`, `y`, `w`, `h` (`double`) — normalized bounding box

Parsed from the API response fields (`frame_time`, `class`, `confidence`, `box_x/y/w/h`). A `toDetectionBox()` convenience method converts to the existing `DetectionBox` for rendering.

### Data Fetching

No Riverpod provider needed. The `PlaybackController` already holds a `PlaybackService` reference. A new `fetchDetections(cameraId, start, end)` method on `PlaybackService` calls the endpoint and returns `List<PlaybackDetection>`. The controller calls this directly from `_openSegmentAt`.

### Detection Cache on PlaybackController

When `_openSegmentAt` loads a new segment, it triggers a fetch of all detections for that segment's `startTime` to `endTime` range. The results are stored in a `Map<String, List<PlaybackDetection>>` on the controller, keyed by camera ID.

A helper method `getDetectionsAtTime(String cameraId, DateTime time, {Duration tolerance = 500ms})` scans the cached list and returns all detections with `frameTime` within the tolerance window. Since the list is sorted by `frame_time ASC`, this is a binary search.

### Lifecycle

Cache is cleared whenever `_disposeAllPlayers` is called (segment change, date change, camera change). Each new segment triggers a fresh fetch. During gap mode (no video playing), the overlay shows nothing.

## Section 3: Timeline — Event Markers Layer

### New Paint Method

`_paintMotionEvents` added to `TimelinePainter`, called in `paint()` after `_paintMotionIntensity`. Renders individual `MotionEvent` bars in a new lane below the intensity heat map.

### Layout

Existing layout constants:

- `_recordingTop = 0`, `_recordingHeight = 18` (recording segments)
- `_eventTop = 21`, `_eventHeight = 14` (intensity buckets)
- `_bookmarkY = 38`

New layout:

- `_motionEventTop = 37`, `_motionEventHeight = 10` (new event markers lane)
- `_bookmarkY` shifts to `49`
- `_timelineHeight` increases from `70` to `80` in `FixedPlayheadTimeline`

### Rendering

Each `MotionEvent` is drawn as a horizontal bar from `startTime` to `endTime`. Color is determined by `objectClass`:

- `person` — warm color (orange)
- `vehicle` — cool color (blue)
- `animal` — green
- Default/`motion` — muted gray

Standard visibility culling: skip events outside the visible time range. Bars get a slight alpha so overlapping events from multiple cameras are visible.

### Color Map

Defined as a static `Map<String, Color>` on `TimelinePainter` for easy extension.

## Section 4: Playback Detection Overlay Widget

### New Widget

`PlaybackDetectionOverlay` — a `StatelessWidget` that takes `List<DetectionBox>` and renders bounding boxes with labels via a `CustomPainter`. Visually identical to the live `_DetectionPainter` (2px accent stroke, class+confidence label pill above the box) but implemented independently. Located at `screens/playback/playback_detection_overlay.dart`.

### Integration into CameraPlayer

In `_buildVideoContent`, the overlay is stacked on top of the `VideoPlayer` widget inside the existing `Stack`. Only renders when detections are non-empty.

### Toggle Button

A new tile button (matching the existing mute button style) added to the bottom-right controls row in `CameraPlayer`. Icon: `visibility`/`visibility_off`. Only visible when the controller reports that detections exist for the current segment and camera. Toggle state tracked per-camera on `PlaybackController` via `Set<String> _overlayDisabledCameras` (similar to `_mutedCameras`).

### Detection Lookup Per Frame

`CameraPlayer` reads detections from the controller via `getDetectionsAtTime(cameraId, currentPlaybackTime, tolerance: 500ms)`. The `build` method already accesses `_ctrl.position` which triggers rebuilds. Returned detections are mapped to `DetectionBox` objects and passed to `PlaybackDetectionOverlay`.

### Performance

Tolerance window lookup uses binary search on the sorted, pre-fetched list. No API calls during playback — all data is in memory from the segment prefetch.

## Section 5: Data Flow and Error Handling

### End-to-End Flow

1. User opens playback, selects camera and date
2. Recording segments and motion events are fetched (existing behavior)
3. `TimelinePainter` renders intensity heat map (existing) + new event marker lane with color-coded bars
4. User hits play or seeks into a segment
5. `_openSegmentAt` fires — video player initializes, detection fetch kicks off for the segment's time range
6. Detections are cached on the controller, indexed by camera ID
7. As playback advances, `CameraPlayer` calls `getDetectionsAtTime()` with the current position
8. If detections exist and the toggle is on, `PlaybackDetectionOverlay` renders bounding boxes
9. When the segment changes (auto-advance or seek), cache is cleared and re-fetched for the new segment

### Error Handling

- **Detection fetch failure:** Log the error, don't show overlay, don't block video playback. The overlay is a nice-to-have layer, not critical path.
- **Empty detection response:** Toggle button stays hidden, overlay doesn't render. No user-facing error.
- **Seek during fetch:** If a new segment loads before the previous fetch completes, the in-flight request's result is discarded (segment mismatch check).

### Explicit Non-Goals

- No changes to the live view `AnalyticsOverlay` or `detectionStreamProvider`
- No interpolation of bounding boxes between frames
- No filtering of detections by object class in the overlay (all detections at the timestamp are shown)
- No detection data in the `MiniOverviewBar`

## Files Changed

### Backend (Go)

- `internal/nvr/db/detections.go` — new `QueryDetectionsByTimeRange` function
- `internal/nvr/api/cameras.go` — new `Detections` handler method
- `internal/nvr/api/router.go` — register new route

### Frontend (Flutter)

- `clients/flutter/lib/screens/playback/timeline/timeline_painter.dart` — new `_paintMotionEvents` method, layout adjustments
- `clients/flutter/lib/screens/playback/timeline/fixed_playhead_timeline.dart` — increase `_timelineHeight`
- `clients/flutter/lib/screens/playback/playback_controller.dart` — detection cache, `getDetectionsAtTime`, toggle state, fetch trigger in `_openSegmentAt`
- `clients/flutter/lib/screens/playback/camera_player.dart` — overlay integration, toggle button
- `clients/flutter/lib/screens/playback/playback_detection_overlay.dart` — new file
- `clients/flutter/lib/services/playback_service.dart` — new `fetchDetections` method
