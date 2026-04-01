# Flutter Client Bugfixes Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix 5 bugs found during macOS testing: AI overlay, clip search crash, camera profiles, zones crash, and playback timeline.

**Architecture:** Each fix is independent and can be committed separately. Fixes range from simple type cast corrections to a full timeline widget rewrite.

**Tech Stack:** Flutter, Dart, CustomPainter

**Issues:** `docs/flutter-client-issues.md`

---

## File Structure

| File                                                            | Task | Fix                                           |
| --------------------------------------------------------------- | ---- | --------------------------------------------- |
| `clients/flutter/lib/screens/live_view/analytics_overlay.dart`  | 1    | Fix overlay visibility + WebSocket connection |
| `clients/flutter/lib/screens/live_view/fullscreen_view.dart`    | 1    | Ensure overlay is sized correctly in Stack    |
| `clients/flutter/lib/screens/live_view/camera_tile.dart`        | 1    | Add overlay to grid tiles too                 |
| `clients/flutter/lib/models/search_result.dart`                 | 2    | Fix type casts                                |
| `clients/flutter/lib/providers/search_provider.dart`            | 2    | Safe JSON parsing                             |
| `clients/flutter/lib/screens/cameras/camera_detail_screen.dart` | 3    | Fetch and display ONVIF profiles              |
| `clients/flutter/lib/models/zone.dart`                          | 4    | Handle polygon as JSON string                 |
| `clients/flutter/lib/screens/playback/timeline_widget.dart`     | 5    | Full rewrite as scrubable timeline bar        |
| `clients/flutter/lib/screens/playback/playback_screen.dart`     | 5    | Increase timeline size, make it interactive   |

---

### Task 1: Fix AI Overlay in Live View

**Root cause:** The analytics overlay depends on `detection_frame` WebSocket events, but:

1. The backend `detection_frame` broadcast was only added in the worktree branch — it may not be in the running server binary
2. The overlay widget uses `StreamProvider` which shows nothing on loading/error (returns `SizedBox.shrink`)
3. The overlay in `fullscreen_view.dart` is correctly positioned but the grid `camera_tile.dart` doesn't include it at all

**Files:**

- Modify: `clients/flutter/lib/screens/live_view/analytics_overlay.dart`
- Modify: `clients/flutter/lib/screens/live_view/fullscreen_view.dart`
- Modify: `clients/flutter/lib/screens/live_view/camera_tile.dart`

- [ ] **Step 1: Make overlay fall back to REST polling when WebSocket has no data**

The overlay should work even without `detection_frame` WebSocket events by falling back to polling `/cameras/:id/detections/latest`. Update `analytics_overlay.dart`:

```dart
// Instead of only watching detectionStreamProvider, also poll the REST API
// as a fallback. This ensures the overlay works even if detection_frame
// events aren't being broadcast.

class AnalyticsOverlay extends ConsumerStatefulWidget {
  final String cameraName;
  final String cameraId;
  const AnalyticsOverlay({super.key, required this.cameraName, required this.cameraId});

  @override
  ConsumerState<AnalyticsOverlay> createState() => _AnalyticsOverlayState();
}

class _AnalyticsOverlayState extends ConsumerState<AnalyticsOverlay> {
  List<DetectionBox> _detections = [];
  Timer? _pollTimer;

  @override
  void initState() {
    super.initState();
    // Poll REST API every 1 second as fallback
    _pollTimer = Timer.periodic(const Duration(seconds: 1), (_) => _poll());
    _poll();
  }

  Future<void> _poll() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      final res = await api.get('/cameras/${widget.cameraId}/detections/latest');
      if (res.data is List && mounted) {
        setState(() {
          _detections = (res.data as List).map((d) {
            final m = d as Map<String, dynamic>;
            return DetectionBox(
              className: m['class'] as String? ?? '',
              confidence: (m['confidence'] as num?)?.toDouble() ?? 0,
              trackId: (m['track_id'] as num?)?.toInt() ?? 0,
              x: (m['box_x'] as num?)?.toDouble() ?? 0,
              y: (m['box_y'] as num?)?.toDouble() ?? 0,
              w: (m['box_w'] as num?)?.toDouble() ?? 0,
              h: (m['box_h'] as num?)?.toDouble() ?? 0,
            );
          }).toList();
        });
      }
    } catch (_) {}
  }

  // Also listen to WebSocket stream — prefer WS data when available
  // Override _detections when WS frame arrives

  @override
  void dispose() {
    _pollTimer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    // Also try WebSocket stream
    final wsFrame = ref.watch(detectionStreamProvider(widget.cameraName));
    final dets = wsFrame.whenOrNull(data: (frame) => frame.detections) ?? _detections;

    if (dets.isEmpty) return const SizedBox.shrink();

    return Positioned.fill(
      child: CustomPaint(painter: _DetectionPainter(dets)),
    );
  }
}
```

The key change: add `cameraId` parameter so it can call the REST API, and use `Timer.periodic` for polling as a fallback. WebSocket data takes priority when available.

- [ ] **Step 2: Update fullscreen_view.dart to pass cameraId**

Find where `AnalyticsOverlay` is used and add `cameraId: widget.camera.id`:

```dart
if (widget.camera.aiEnabled)
  AnalyticsOverlay(cameraName: widget.camera.name, cameraId: widget.camera.id),
```

- [ ] **Step 3: Add overlay to camera_tile.dart grid tiles**

In the `Stack` children of `camera_tile.dart`, add the overlay for AI-enabled cameras:

```dart
if (widget.camera.aiEnabled && _connState == WhepConnectionState.connected)
  AnalyticsOverlay(cameraName: widget.camera.name, cameraId: widget.camera.id),
```

- [ ] **Step 4: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/screens/live_view/
git commit -m "fix(flutter): AI overlay with REST polling fallback, add to grid tiles"
```

---

### Task 2: Fix Clip Search Type Cast Crash

**Root cause:** The search API returns `detection_id` and `event_id` as integers, but somewhere in the parsing chain they're being cast `as String`. The `SearchResult.fromJson` uses `as String?` for `cameraId` which could fail if the API returns an int.

**Files:**

- Modify: `clients/flutter/lib/models/search_result.dart`

- [ ] **Step 1: Make all JSON casts safe using toString()**

Replace all `as String?` casts with `?.toString()` and all `as int?` with `(as num?)?.toInt()`:

```dart
factory SearchResult.fromJson(Map<String, dynamic> json) {
  return SearchResult(
    detectionId: (json['detection_id'] as num?)?.toInt() ?? 0,
    eventId: (json['event_id'] as num?)?.toInt() ?? 0,
    cameraId: json['camera_id']?.toString() ?? '',
    cameraName: json['camera_name']?.toString() ?? '',
    className: json['class']?.toString() ?? '',
    confidence: (json['confidence'] as num?)?.toDouble() ?? 0,
    similarity: (json['similarity'] as num?)?.toDouble() ?? 0,
    frameTime: json['frame_time']?.toString() ?? '',
    thumbnailPath: json['thumbnail_path']?.toString(),
  );
}
```

The key fix: use `?.toString()` instead of `as String?` for any field that could come as a different type from the API.

- [ ] **Step 2: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/models/search_result.dart
git commit -m "fix(flutter): safe type casts in search result JSON parsing"
```

---

### Task 3: Camera Settings — Show All ONVIF Profiles

**Root cause:** The camera detail screen only shows editable text fields for the current RTSP URL. It doesn't fetch or display available ONVIF media profiles.

**Files:**

- Modify: `clients/flutter/lib/screens/cameras/camera_detail_screen.dart`

- [ ] **Step 1: Add profile fetching to the General tab**

When the camera has an ONVIF endpoint, fetch profiles via `POST /cameras/probe` with the camera's ONVIF endpoint and credentials. Display them as a list of selectable radio buttons or a dropdown:

```dart
// In the General tab section, after the RTSP URL field:
// 1. If camera.onvifEndpoint is set, show a "Fetch Profiles" button
// 2. On tap, POST /cameras/probe with {endpoint, username, password}
// 3. Response contains profiles with name, rtsp_url, resolution, fps
// 4. Display as a list of cards, each with a "Use This Profile" button
// 5. Tapping "Use" updates the RTSP URL field

// Add state:
List<Map<String, dynamic>> _profiles = [];
bool _loadingProfiles = false;

Future<void> _fetchProfiles() async {
  setState(() => _loadingProfiles = true);
  try {
    final res = await api.post('/cameras/probe', data: {
      'endpoint': _camera.onvifEndpoint,
      'username': _onvifUsername,
      'password': _onvifPassword,
    });
    if (res.data is Map && res.data['profiles'] is List) {
      setState(() => _profiles = (res.data['profiles'] as List).cast<Map<String, dynamic>>());
    }
  } catch (_) {}
  setState(() => _loadingProfiles = false);
}

// Render profiles as selectable cards showing:
// - Profile name (e.g., "MainStream", "SubStream")
// - Resolution (e.g., "1920x1080")
// - Codec + FPS
// - RTSP URL
// - "Select" button that sets the RTSP URL
```

- [ ] **Step 2: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/screens/cameras/camera_detail_screen.dart
git commit -m "fix(flutter): fetch and display ONVIF profiles in camera settings"
```

---

### Task 4: Fix Zones Tab Type Cast Crash

**Root cause:** The backend returns `polygon` as a JSON string (e.g., `"[[0.1,0.2],[0.3,0.4]]"`) but the Dart model tries to cast it directly as `List<dynamic>`. When the API returns a string instead of an array, the cast fails.

**Files:**

- Modify: `clients/flutter/lib/models/zone.dart`

- [ ] **Step 1: Handle polygon as either string or list**

Update `DetectionZone.fromJson` to handle both cases:

```dart
factory DetectionZone.fromJson(Map<String, dynamic> json) {
  // polygon can be a JSON string or a direct list
  List<List<double>> poly = [];
  final rawPoly = json['polygon'];
  if (rawPoly is String) {
    // Parse JSON string to list
    try {
      final parsed = jsonDecode(rawPoly) as List;
      poly = parsed.map((p) => (p as List).map((v) => (v as num).toDouble()).toList()).toList();
    } catch (_) {}
  } else if (rawPoly is List) {
    poly = rawPoly.map((p) => (p as List).map((v) => (v as num).toDouble()).toList()).toList();
  }

  return DetectionZone(
    id: (json['id'] as num?)?.toInt(),
    cameraId: json['camera_id']?.toString() ?? '',
    name: json['name']?.toString() ?? '',
    polygon: poly,
    enabled: json['enabled'] as bool? ?? true,
    rules: (json['rules'] as List?)?.map((r) => AlertRule.fromJson(r as Map<String, dynamic>)).toList() ?? [],
  );
}
```

Add `import 'dart:convert';` at the top of the file.

- [ ] **Step 2: Also fix AlertRule parsing to be safe**

Use `?.toString()` and `(as num?)?.toInt()` for all fields, same pattern as Task 2.

- [ ] **Step 3: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/models/zone.dart
git commit -m "fix(flutter): handle polygon as JSON string or list in zone parsing"
```

---

### Task 5: Playback Timeline — Full Rewrite

**Root cause:** The current timeline is only 64px (too small to be useful) and only renders basic hour lines. It needs to be a full interactive timeline like Milestone/Frigate with recording segments, motion events, and a draggable playhead.

**Files:**

- Rewrite: `clients/flutter/lib/screens/playback/timeline_widget.dart`
- Modify: `clients/flutter/lib/screens/playback/playback_screen.dart`

- [ ] **Step 1: Rewrite timeline_widget.dart as a full scrubable timeline**

The new timeline should:

1. **Be at least 200px tall** (vertical) or 120px tall (horizontal)
2. **Show recording segments** as colored bars (fetch from `/recordings?camera_id=...&start=...&end=...`)
3. **Show motion events** as colored markers/dots on the timeline (amber for motion, blue for person, green for vehicle)
4. **Have a draggable playhead** — a colored line with a handle that the user can drag to seek
5. **Support pinch-to-zoom** — start at 24-hour view, zoom in to 1-hour view
6. **Show time labels** every hour (24h view) or every 10 minutes (zoomed)
7. **Auto-scroll** to keep the playhead visible

Implementation approach:

- Use `GestureDetector` for tap-to-seek and pan-to-drag-playhead
- Use `CustomPainter` for all rendering
- Recording segments painted as filled rectangles with rounded ends
- Motion events painted as small icons or colored dots
- Playhead painted as a bright line spanning full width/height with a circular handle
- Current time shown as text near the playhead

```dart
class TimelineWidget extends ConsumerStatefulWidget {
  final List<String> cameraIds;
  final DateTime selectedDate;
  final Duration position;
  final ValueChanged<Duration> onSeek;
  final bool vertical;

  // ...
}

class _TimelineWidgetState extends ConsumerState<TimelineWidget> {
  double _zoomLevel = 1.0; // 1.0 = 24 hours, 24.0 = 1 hour
  double _scrollOffset = 0.0;
  bool _isDragging = false;

  // Gesture handling:
  // - Tap: seek to tapped position
  // - Horizontal/vertical drag on playhead: scrub
  // - Pinch: zoom in/out

  @override
  Widget build(BuildContext context) {
    final segmentsAsync = /* fetch recording segments */;
    final eventsAsync = /* fetch motion events */;

    return GestureDetector(
      onTapDown: _handleTap,
      onVerticalDragUpdate: widget.vertical ? _handleDrag : null,
      onHorizontalDragUpdate: !widget.vertical ? _handleDrag : null,
      child: CustomPaint(
        painter: _FullTimelinePainter(
          segments: segments,
          events: events,
          position: widget.position,
          zoomLevel: _zoomLevel,
          scrollOffset: _scrollOffset,
          vertical: widget.vertical,
        ),
        size: Size.infinite,
      ),
    );
  }
}
```

The painter draws in this order:

1. Background (dark)
2. Time grid lines + labels
3. Recording segment bars (colored by camera)
4. Motion event markers (small colored circles with class icons)
5. Playhead line + handle + current time text

- [ ] **Step 2: Update playback_screen.dart to give the timeline more space**

Change the timeline container sizes:

- Wide layout: timeline width from `64` to `220`
- Narrow layout: timeline height from `64` to `140`

In `_WideLayout`:

```dart
Container(
  width: 220,  // was 64
  color: NvrColors.bgTertiary,
  child: /* timeline */,
),
```

In `_NarrowLayout`:

```dart
Container(
  height: 140,  // was 64
  color: NvrColors.bgTertiary,
  child: /* timeline */,
),
```

- [ ] **Step 3: Fetch recording segments for timeline**

Add a recording segments fetch to the timeline or the playback screen:

```dart
// In playback_screen.dart or timeline_widget.dart:
final segmentsAsync = ref.watch(recordingSegmentsProvider(
  (cameraId: cameras.first.id, date: dateKey),
));
```

Pass the segments to the timeline painter so it can draw them as filled bars.

- [ ] **Step 4: Verify + commit**

```bash
cd clients/flutter && flutter analyze
git add clients/flutter/lib/screens/playback/
git commit -m "fix(flutter): rewrite playback timeline with recording segments, events, and draggable playhead"
```
