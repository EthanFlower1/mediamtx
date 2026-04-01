# Playback Detection Overlay & Timeline Event Markers — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Display per-frame detection bounding boxes during playback and render color-coded motion event markers on the timeline.

**Architecture:** New backend endpoint serves historical detections by time range. The PlaybackController batch-fetches detections when a segment loads and caches them client-side. A new PlaybackDetectionOverlay widget renders bounding boxes during playback using nearest-frame matching. The TimelinePainter gets a new lane for individual MotionEvent bars color-coded by object class.

**Tech Stack:** Go (gin, SQLite), Flutter (video_player, Riverpod, CustomPainter)

**Spec:** `docs/superpowers/specs/2026-03-31-playback-detections-design.md`

---

## File Structure

### Backend (Go)

- **Modify:** `internal/nvr/db/detections.go` — add `QueryDetectionsByTimeRange`
- **Modify:** `internal/nvr/api/cameras.go` — add `Detections` handler
- **Modify:** `internal/nvr/api/router.go` — register new route
- **Modify:** `internal/nvr/api/cameras_test.go` — test the new endpoint
- **Modify:** `internal/nvr/db/db_test.go` — (if needed, but we test via API)

### Frontend (Flutter)

- **Modify:** `clients/flutter/lib/models/detection_frame.dart` — add `PlaybackDetection` model
- **Modify:** `clients/flutter/lib/services/playback_service.dart` — add `fetchDetections` method
- **Modify:** `clients/flutter/lib/screens/playback/playback_controller.dart` — detection cache, lookup, toggle, fetch trigger
- **Create:** `clients/flutter/lib/screens/playback/playback_detection_overlay.dart` — bounding box overlay widget
- **Modify:** `clients/flutter/lib/screens/playback/camera_player.dart` — integrate overlay + toggle button
- **Modify:** `clients/flutter/lib/screens/playback/timeline/timeline_painter.dart` — new `_paintMotionEvents` method, layout adjustments
- **Modify:** `clients/flutter/lib/screens/playback/timeline/fixed_playhead_timeline.dart` — increase `_timelineHeight`

### Tests

- **Modify:** `internal/nvr/api/cameras_test.go` — API endpoint test
- **Modify:** `clients/flutter/test/playback/playback_controller_test.dart` — detection lookup tests

---

## Task 1: Backend — New DB Function

**Files:**

- Modify: `internal/nvr/db/detections.go`

- [ ] **Step 1: Add `QueryDetectionsByTimeRange` to `detections.go`**

Add this function after `GetRecentDetections` (after line 258):

```go
// QueryDetectionsByTimeRange returns all detections for a camera within the
// given time range, ordered by frame_time ascending for playback consumption.
func (d *DB) QueryDetectionsByTimeRange(cameraID string, start, end time.Time) ([]*Detection, error) {
	rows, err := d.Query(`
		SELECT d.id, d.motion_event_id, d.frame_time, d.class, d.confidence,
			d.box_x, d.box_y, d.box_w, d.box_h, COALESCE(d.attributes, '')
		FROM detections d
		JOIN motion_events me ON d.motion_event_id = me.id
		WHERE me.camera_id = ? AND d.frame_time >= ? AND d.frame_time <= ?
		ORDER BY d.frame_time ASC`,
		cameraID, start.UTC().Format(timeFormat), end.UTC().Format(timeFormat),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var detections []*Detection
	for rows.Next() {
		det := &Detection{}
		if err := rows.Scan(
			&det.ID, &det.MotionEventID, &det.FrameTime, &det.Class,
			&det.Confidence, &det.BoxX, &det.BoxY, &det.BoxW, &det.BoxH,
			&det.Attributes,
		); err != nil {
			return nil, err
		}
		detections = append(detections, det)
	}
	return detections, rows.Err()
}
```

- [ ] **Step 2: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/db/...`
Expected: clean compile, no errors.

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/db/detections.go
git commit -m "feat(db): add QueryDetectionsByTimeRange for playback"
```

---

## Task 2: Backend — New API Endpoint

**Files:**

- Modify: `internal/nvr/api/cameras.go`
- Modify: `internal/nvr/api/router.go`

- [ ] **Step 1: Add `Detections` handler to `cameras.go`**

Add this method after the `LatestDetections` handler (after line 1711):

```go
// Detections returns all detections for a camera within a time range.
// Used by the playback overlay to batch-fetch detections for a recording segment.
//
// GET /api/nvr/cameras/:id/detections?start=RFC3339&end=RFC3339
func (h *CameraHandler) Detections(c *gin.Context) {
	id := c.Param("id")

	startStr := c.Query("start")
	endStr := c.Query("end")
	if startStr == "" || endStr == "" {
		apiError(c, http.StatusBadRequest, "start and end query params required", nil)
		return
	}

	start, err := time.Parse(time.RFC3339, startStr)
	if err != nil {
		apiError(c, http.StatusBadRequest, "invalid start time", err)
		return
	}
	end, err := time.Parse(time.RFC3339, endStr)
	if err != nil {
		apiError(c, http.StatusBadRequest, "invalid end time", err)
		return
	}

	detections, err := h.DB.QueryDetectionsByTimeRange(id, start, end)
	if err != nil {
		apiError(c, http.StatusInternalServerError, "failed to query detections", err)
		return
	}
	if detections == nil {
		detections = []*db.Detection{}
	}
	c.JSON(http.StatusOK, detections)
}
```

- [ ] **Step 2: Register the route in `router.go`**

In `router.go`, after line 202 (the `/detections/latest` route), add:

```go
	protected.GET("/cameras/:id/detections", cameraHandler.Detections)
```

Note: this must come AFTER the `/detections/latest` route so gin matches the more specific path first.

- [ ] **Step 3: Verify it compiles**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./internal/nvr/...`
Expected: clean compile, no errors.

- [ ] **Step 4: Commit**

```bash
git add internal/nvr/api/cameras.go internal/nvr/api/router.go
git commit -m "feat(api): add GET /cameras/:id/detections endpoint for playback"
```

---

## Task 3: Backend — API Endpoint Test

**Files:**

- Modify: `internal/nvr/api/cameras_test.go`

- [ ] **Step 1: Write the test**

Add at the end of `cameras_test.go`:

```go
func TestDetections(t *testing.T) {
	gin.SetMode(gin.TestMode)
	handler, cleanup := setupCameraTest(t)
	defer cleanup()

	// Create a camera and motion event with detections.
	cam := &db.Camera{
		ID:   "cam1",
		Name: "Test Cam",
	}
	require.NoError(t, handler.DB.InsertCamera(cam))

	evt := &db.MotionEvent{
		CameraID:  "cam1",
		StartedAt: "2026-03-24T10:00:00Z",
		EventType: "ai_detection",
		ObjectClass: "person",
		Confidence:  0.95,
	}
	require.NoError(t, handler.DB.InsertMotionEvent(evt))

	det := &db.Detection{
		MotionEventID: evt.ID,
		FrameTime:     "2026-03-24T10:00:01Z",
		Class:         "person",
		Confidence:    0.95,
		BoxX:          0.1,
		BoxY:          0.2,
		BoxW:          0.3,
		BoxH:          0.4,
	}
	require.NoError(t, handler.DB.InsertDetection(det))

	// Query with matching time range.
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Params = gin.Params{{Key: "id", Value: "cam1"}}
	c.Request = httptest.NewRequest(http.MethodGet,
		"/?start=2026-03-24T09:59:00Z&end=2026-03-24T10:01:00Z", nil)

	handler.Detections(c)

	assert.Equal(t, http.StatusOK, w.Code)

	var detections []db.Detection
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &detections))
	assert.Len(t, detections, 1)
	assert.Equal(t, "person", detections[0].Class)
	assert.InDelta(t, 0.1, detections[0].BoxX, 0.001)

	// Query with non-overlapping time range returns empty.
	w2 := httptest.NewRecorder()
	c2, _ := gin.CreateTestContext(w2)
	c2.Params = gin.Params{{Key: "id", Value: "cam1"}}
	c2.Request = httptest.NewRequest(http.MethodGet,
		"/?start=2026-03-24T11:00:00Z&end=2026-03-24T12:00:00Z", nil)

	handler.Detections(c2)

	assert.Equal(t, http.StatusOK, w2.Code)

	var empty []db.Detection
	require.NoError(t, json.Unmarshal(w2.Body.Bytes(), &empty))
	assert.Empty(t, empty)

	// Missing params returns 400.
	w3 := httptest.NewRecorder()
	c3, _ := gin.CreateTestContext(w3)
	c3.Params = gin.Params{{Key: "id", Value: "cam1"}}
	c3.Request = httptest.NewRequest(http.MethodGet, "/", nil)

	handler.Detections(c3)

	assert.Equal(t, http.StatusBadRequest, w3.Code)
}
```

- [ ] **Step 2: Run the test**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/api/ -run TestDetections -v`
Expected: PASS

- [ ] **Step 3: Commit**

```bash
git add internal/nvr/api/cameras_test.go
git commit -m "test: add API test for GET /cameras/:id/detections"
```

---

## Task 4: Flutter — PlaybackDetection Model

**Files:**

- Modify: `clients/flutter/lib/models/detection_frame.dart`

- [ ] **Step 1: Add `PlaybackDetection` class**

Add after the `DetectionFrame` class (after line 56):

```dart
/// A detection from a recorded segment, used during playback.
/// Includes [frameTime] for timestamp-based lookup that [DetectionBox] lacks.
class PlaybackDetection {
  final DateTime frameTime;
  final String className;
  final double confidence;
  final double x;
  final double y;
  final double w;
  final double h;

  const PlaybackDetection({
    required this.frameTime,
    required this.className,
    required this.confidence,
    required this.x,
    required this.y,
    required this.w,
    required this.h,
  });

  factory PlaybackDetection.fromJson(Map<String, dynamic> json) {
    return PlaybackDetection(
      frameTime: DateTime.parse(json['frame_time'] as String),
      className: json['class'] as String? ?? '',
      confidence: (json['confidence'] as num?)?.toDouble() ?? 0.0,
      x: (json['box_x'] as num?)?.toDouble() ?? 0.0,
      y: (json['box_y'] as num?)?.toDouble() ?? 0.0,
      w: (json['box_w'] as num?)?.toDouble() ?? 0.0,
      h: (json['box_h'] as num?)?.toDouble() ?? 0.0,
    );
  }

  DetectionBox toDetectionBox() => DetectionBox(
        className: className,
        confidence: confidence,
        x: x,
        y: y,
        w: w,
        h: h,
      );
}
```

- [ ] **Step 2: Verify no analysis errors**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && dart analyze lib/models/detection_frame.dart`
Expected: No issues found.

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/models/detection_frame.dart
git commit -m "feat(flutter): add PlaybackDetection model for historical detections"
```

---

## Task 5: Flutter — PlaybackService.fetchDetections

**Files:**

- Modify: `clients/flutter/lib/services/playback_service.dart`

- [ ] **Step 1: Add import for the model**

At the top of `playback_service.dart`, add after the existing imports (after line 2):

```dart
import '../models/detection_frame.dart';
```

- [ ] **Step 2: Add `fetchDetections` method**

Add this method to the `PlaybackService` class, after the `clipUrl` method (after line 127):

```dart
  /// Fetch historical detections for a camera within a time range.
  /// Used by the playback overlay to batch-load detections for a segment.
  Future<List<PlaybackDetection>> fetchDetections({
    required String cameraId,
    required DateTime start,
    required DateTime end,
    String? token,
  }) async {
    final uri = Uri.parse(serverUrl);
    final params = <String, String>{
      'start': start.toUtc().toIso8601String(),
      'end': end.toUtc().toIso8601String(),
    };

    final url = Uri(
      scheme: uri.scheme,
      host: uri.host,
      port: uri.port,
      path: '/api/nvr/cameras/$cameraId/detections',
      queryParameters: params,
    ).toString();

    final dio = Dio();
    try {
      final options = Options();
      if (token != null && token.isNotEmpty) {
        options.headers = {'Authorization': 'Bearer $token'};
      }
      final response = await dio.get<List<dynamic>>(url, options: options);
      if (response.statusCode == 200 && response.data != null) {
        return response.data!
            .map((e) =>
                PlaybackDetection.fromJson(e as Map<String, dynamic>))
            .toList();
      }
      return [];
    } catch (e) {
      debugPrint('[PlaybackService] fetchDetections failed: $e');
      return [];
    } finally {
      dio.close();
    }
  }
```

- [ ] **Step 3: Verify no analysis errors**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && dart analyze lib/services/playback_service.dart`
Expected: No issues found.

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/services/playback_service.dart
git commit -m "feat(flutter): add fetchDetections to PlaybackService"
```

---

## Task 6: Flutter — Detection Cache on PlaybackController

**Files:**

- Modify: `clients/flutter/lib/screens/playback/playback_controller.dart`

- [ ] **Step 1: Add import**

At the top of `playback_controller.dart`, add after the existing imports (after line 5, the recording.dart import):

```dart
import '../../models/detection_frame.dart';
```

- [ ] **Step 2: Add detection state fields**

Add these fields after the `_currentSegment` field (after line 42):

```dart
  // Detection cache — prefetched per segment for playback overlay.
  final Map<String, List<PlaybackDetection>> _detectionCache = {};
  final Set<String> _overlayDisabledCameras = {};
```

- [ ] **Step 3: Add getters**

Add these getters after the existing `error` getter (after line 66):

```dart
  Map<String, List<PlaybackDetection>> get detectionCache => _detectionCache;
  bool isOverlayDisabled(String cameraId) =>
      _overlayDisabledCameras.contains(cameraId);
```

- [ ] **Step 4: Add `hasDetectionsForCamera` check**

Add after the getters:

```dart
  bool hasDetectionsForCamera(String cameraId) =>
      _detectionCache.containsKey(cameraId) &&
      _detectionCache[cameraId]!.isNotEmpty;
```

- [ ] **Step 5: Add `toggleOverlay` method**

Add after the `isCameraMuted` method (after line 276):

```dart
  void toggleOverlay(String cameraId) {
    if (_overlayDisabledCameras.contains(cameraId)) {
      _overlayDisabledCameras.remove(cameraId);
    } else {
      _overlayDisabledCameras.add(cameraId);
    }
    notifyListeners();
  }
```

- [ ] **Step 6: Add `getDetectionsAtTime` method**

Add after `toggleOverlay`:

```dart
  /// Returns detections for [cameraId] within [tolerance] of [time].
  /// Uses binary search on the sorted cache for efficient lookup.
  List<DetectionBox> getDetectionsAtTime(
    String cameraId,
    DateTime time, {
    Duration tolerance = const Duration(milliseconds: 500),
  }) {
    final cache = _detectionCache[cameraId];
    if (cache == null || cache.isEmpty) return [];

    final results = <DetectionBox>[];
    // Binary search for the first detection within the tolerance window.
    final windowStart = time.subtract(tolerance);
    final windowEnd = time.add(tolerance);

    int lo = 0, hi = cache.length;
    while (lo < hi) {
      final mid = (lo + hi) ~/ 2;
      if (cache[mid].frameTime.isBefore(windowStart)) {
        lo = mid + 1;
      } else {
        hi = mid;
      }
    }

    // Collect all detections within the window starting from lo.
    for (int i = lo; i < cache.length; i++) {
      if (cache[i].frameTime.isAfter(windowEnd)) break;
      results.add(cache[i].toDetectionBox());
    }

    return results;
  }
```

- [ ] **Step 7: Add `_fetchDetectionsForSegment` helper**

Add after `getDetectionsAtTime`:

```dart
  /// Fetch detections for all selected cameras within the given segment's
  /// time range and populate the cache.
  Future<void> _fetchDetectionsForSegment(RecordingSegment seg) async {
    final token = await getAccessToken();
    for (final camId in _selectedCameraIds) {
      try {
        final detections = await playbackService.fetchDetections(
          cameraId: camId,
          start: seg.startTime,
          end: seg.endTime,
          token: token,
        );
        // Guard against stale results if segment changed during fetch.
        if (_currentSegment?.id != seg.id) return;
        _detectionCache[camId] = detections;
      } catch (e) {
        debugPrint('Failed to fetch detections for camera $camId: $e');
      }
    }
    notifyListeners();
  }
```

- [ ] **Step 8: Clear cache in `_disposeAllPlayers`**

In the `_disposeAllPlayers` method (around line 719), add `_detectionCache.clear();` after `_players.clear();`:

```dart
  void _disposeAllPlayers() {
    for (final p in _players.values) {
      p.removeListener(_onPositionUpdate);
      p.dispose();
    }
    _players.clear();
    _detectionCache.clear();
  }
```

- [ ] **Step 9: Trigger fetch in `_openSegmentAt`**

In `_openSegmentAt`, after the player setup loop completes and before the final `notifyListeners()` call (around line 580, after the `if (_players.isEmpty)` error check), add:

```dart
    // Fetch detections for the segment (non-blocking).
    _fetchDetectionsForSegment(seg);
```

This is intentionally not `await`ed — the video starts immediately and detections load in the background.

- [ ] **Step 10: Verify no analysis errors**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && dart analyze lib/screens/playback/playback_controller.dart`
Expected: No issues found.

- [ ] **Step 11: Commit**

```bash
git add clients/flutter/lib/screens/playback/playback_controller.dart
git commit -m "feat(flutter): add detection cache and lookup to PlaybackController"
```

---

## Task 7: Flutter — Detection Lookup Unit Tests

**Files:**

- Modify: `clients/flutter/test/playback/playback_controller_test.dart`

- [ ] **Step 1: Add import**

At the top of the test file, add:

```dart
import 'package:nvr_client/models/detection_frame.dart';
```

- [ ] **Step 2: Add detection lookup tests**

Add a new test group at the end of `main()`, after the existing segment helper tests:

```dart
  group('PlaybackController.getDetectionsAtTime (static test)', () {
    // We test the binary search logic by constructing a cache and calling
    // the method. Since getDetectionsAtTime is an instance method, we
    // create a minimal controller. The playbackService won't be called.
    late PlaybackController controller;

    setUp(() {
      controller = PlaybackController(
        playbackService: PlaybackService(serverUrl: 'http://localhost'),
        getAccessToken: () async => null,
      );
    });

    test('returns detections within tolerance window', () {
      // Manually populate the cache.
      controller.detectionCache['cam1'] = [
        PlaybackDetection(
          frameTime: DateTime.utc(2026, 3, 24, 10, 0, 0),
          className: 'person', confidence: 0.9,
          x: 0.1, y: 0.2, w: 0.3, h: 0.4,
        ),
        PlaybackDetection(
          frameTime: DateTime.utc(2026, 3, 24, 10, 0, 1),
          className: 'vehicle', confidence: 0.8,
          x: 0.5, y: 0.6, w: 0.1, h: 0.2,
        ),
        PlaybackDetection(
          frameTime: DateTime.utc(2026, 3, 24, 10, 0, 5),
          className: 'person', confidence: 0.7,
          x: 0.2, y: 0.3, w: 0.4, h: 0.5,
        ),
      ];

      // Query at 10:00:00.500 with 500ms tolerance → should match first two.
      final results = controller.getDetectionsAtTime(
        'cam1',
        DateTime.utc(2026, 3, 24, 10, 0, 0, 500),
      );
      expect(results.length, 2);
      expect(results[0].className, 'person');
      expect(results[1].className, 'vehicle');
    });

    test('returns empty when no detections in window', () {
      controller.detectionCache['cam1'] = [
        PlaybackDetection(
          frameTime: DateTime.utc(2026, 3, 24, 10, 0, 0),
          className: 'person', confidence: 0.9,
          x: 0.1, y: 0.2, w: 0.3, h: 0.4,
        ),
      ];

      final results = controller.getDetectionsAtTime(
        'cam1',
        DateTime.utc(2026, 3, 24, 10, 0, 5),
      );
      expect(results, isEmpty);
    });

    test('returns empty for unknown camera', () {
      final results = controller.getDetectionsAtTime(
        'unknown',
        DateTime.utc(2026, 3, 24, 10, 0, 0),
      );
      expect(results, isEmpty);
    });
  });
```

- [ ] **Step 3: Run the tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter test test/playback/playback_controller_test.dart -v`
Expected: All tests pass.

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/test/playback/playback_controller_test.dart
git commit -m "test(flutter): add detection lookup unit tests"
```

---

## Task 8: Flutter — PlaybackDetectionOverlay Widget

**Files:**

- Create: `clients/flutter/lib/screens/playback/playback_detection_overlay.dart`

- [ ] **Step 1: Create the overlay widget**

Create `clients/flutter/lib/screens/playback/playback_detection_overlay.dart`:

```dart
import 'package:flutter/material.dart';

import '../../models/detection_frame.dart';
import '../../theme/nvr_colors.dart';

/// Renders bounding boxes for historical detections during playback.
/// Takes a flat list of [DetectionBox] and paints them over the video.
class PlaybackDetectionOverlay extends StatelessWidget {
  final List<DetectionBox> detections;

  const PlaybackDetectionOverlay({
    super.key,
    required this.detections,
  });

  @override
  Widget build(BuildContext context) {
    if (detections.isEmpty) return const SizedBox.shrink();
    return SizedBox.expand(
      child: CustomPaint(
        painter: _PlaybackDetectionPainter(detections),
      ),
    );
  }
}

class _PlaybackDetectionPainter extends CustomPainter {
  final List<DetectionBox> detections;

  const _PlaybackDetectionPainter(this.detections);

  @override
  void paint(Canvas canvas, Size size) {
    const boxColor = NvrColors.accent;
    const labelTextColor = NvrColors.bgPrimary;
    const labelBgColor = NvrColors.accent;

    final boxPaint = Paint()
      ..color = boxColor
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2;

    for (final box in detections) {
      final left = box.x * size.width;
      final top = box.y * size.height;
      final right = left + box.w * size.width;
      final bottom = top + box.h * size.height;
      final rect = Rect.fromLTRB(left, top, right, bottom);

      canvas.drawRect(rect, boxPaint);

      // Label: "Class 95%"
      final pct = (box.confidence * 100).round();
      final cls =
          box.className.isEmpty ? '' : '${box.className[0].toUpperCase()}${box.className.substring(1)}';
      final label = '$cls $pct%';

      final tp = TextPainter(
        text: TextSpan(
          text: label,
          style: const TextStyle(
            fontFamily: 'JetBrainsMono',
            fontSize: 8,
            fontWeight: FontWeight.w700,
            color: labelTextColor,
          ),
        ),
        textDirection: TextDirection.ltr,
      );
      tp.layout();

      const hPad = 3.0;
      const vPad = 2.0;
      final labelW = tp.width + hPad * 2;
      final labelH = tp.height + vPad * 2;
      final labelTop = (top - labelH).clamp(0.0, size.height - labelH);
      final labelLeft = left.clamp(0.0, size.width - labelW);

      final bgRect = RRect.fromLTRBR(
        labelLeft,
        labelTop,
        labelLeft + labelW,
        labelTop + labelH,
        const Radius.circular(2),
      );
      canvas.drawRRect(bgRect, Paint()..color = labelBgColor);
      tp.paint(canvas, Offset(labelLeft + hPad, labelTop + vPad));
    }
  }

  @override
  bool shouldRepaint(_PlaybackDetectionPainter old) =>
      old.detections != detections;
}
```

- [ ] **Step 2: Verify no analysis errors**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && dart analyze lib/screens/playback/playback_detection_overlay.dart`
Expected: No issues found.

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/screens/playback/playback_detection_overlay.dart
git commit -m "feat(flutter): add PlaybackDetectionOverlay widget"
```

---

## Task 9: Flutter — Integrate Overlay into CameraPlayer

**Files:**

- Modify: `clients/flutter/lib/screens/playback/camera_player.dart`

- [ ] **Step 1: Add imports**

At the top of `camera_player.dart`, add after the existing imports (after line 7):

```dart
import '../../models/detection_frame.dart';
import 'playback_detection_overlay.dart';
```

`detection_frame.dart` is needed because `_buildVideoContent` uses `<DetectionBox>[]` explicitly.

- [ ] **Step 2: Add overlay to `_buildVideoContent`**

Replace the `_buildVideoContent` method (lines 201-216) with:

```dart
  Widget _buildVideoContent(VideoPlayerController? vc, bool isInGap) {
    if (isInGap) return const SizedBox.expand();
    if (vc == null || !vc.value.isInitialized) {
      return const Center(
        child: CircularProgressIndicator(color: NvrColors.accent),
      );
    }

    // Get detections for the current playback time.
    final dayStart = DateTime(
      _ctrl.selectedDate.year,
      _ctrl.selectedDate.month,
      _ctrl.selectedDate.day,
    );
    final currentTime = dayStart.add(_ctrl.position);
    final showOverlay = !_ctrl.isOverlayDisabled(_camId);
    final detections = showOverlay
        ? _ctrl.getDetectionsAtTime(_camId, currentTime)
        : <DetectionBox>[];

    return FittedBox(
      fit: BoxFit.contain,
      child: SizedBox(
        width: vc.value.size.width,
        height: vc.value.size.height,
        child: Stack(
          children: [
            VideoPlayer(vc),
            if (detections.isNotEmpty)
              PlaybackDetectionOverlay(detections: detections),
          ],
        ),
      ),
    );
  }
```

- [ ] **Step 3: Add toggle button to the controls row**

In the `build` method, find the bottom-right controls `Positioned` widget (around lines 128-146). Replace the `Row` children to include the overlay toggle before the mute button:

```dart
            // Bottom-right: per-camera controls
            if (!isInGap)
              Positioned(
                bottom: 10,
                right: 10,
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (_ctrl.hasDetectionsForCamera(_camId))
                      Padding(
                        padding: const EdgeInsets.only(right: 4),
                        child: _TileButton(
                          icon: _ctrl.isOverlayDisabled(_camId)
                              ? Icons.visibility_off
                              : Icons.visibility,
                          tooltip: _ctrl.isOverlayDisabled(_camId)
                              ? 'Show detections'
                              : 'Hide detections',
                          onPressed: () => _ctrl.toggleOverlay(_camId),
                        ),
                      ),
                    _TileButton(
                      icon: _ctrl.isCameraMuted(_camId)
                          ? Icons.volume_off
                          : Icons.volume_up,
                      tooltip: _ctrl.isCameraMuted(_camId)
                          ? 'Unmute'
                          : 'Mute',
                      onPressed: () => _ctrl.toggleMute(_camId),
                    ),
                  ],
                ),
              ),
```

- [ ] **Step 4: Verify no analysis errors**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && dart analyze lib/screens/playback/camera_player.dart`
Expected: No issues found.

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/screens/playback/camera_player.dart
git commit -m "feat(flutter): integrate detection overlay and toggle into CameraPlayer"
```

---

## Task 10: Flutter — Timeline Event Markers

**Files:**

- Modify: `clients/flutter/lib/screens/playback/timeline/timeline_painter.dart`
- Modify: `clients/flutter/lib/screens/playback/timeline/fixed_playhead_timeline.dart`

- [ ] **Step 1: Update layout constants in `timeline_painter.dart`**

Replace the layout constants block (lines 52-56) with:

```dart
  // Layout constants
  static const double _recordingTop = 0;
  static const double _recordingHeight = 18;
  static const double _eventTop = 21;
  static const double _eventHeight = 14;
  static const double _motionEventTop = 37;
  static const double _motionEventHeight = 10;
  static const double _bookmarkY = 49;
```

- [ ] **Step 2: Add object class color map**

Add after the layout constants (after the new `_bookmarkY` line):

```dart
  // Color map for object classes.
  static const Map<String, Color> _objectClassColors = {
    'person': Color(0xFFF59E0B),   // amber/orange
    'vehicle': Color(0xFF3B82F6),  // blue
    'car': Color(0xFF3B82F6),      // blue (alias)
    'truck': Color(0xFF3B82F6),    // blue (alias)
    'animal': Color(0xFF22C55E),   // green
    'dog': Color(0xFF22C55E),      // green (alias)
    'cat': Color(0xFF22C55E),      // green (alias)
  };
  static const Color _defaultEventColor = Color(0xFF525252); // muted gray
```

- [ ] **Step 3: Add `_paintMotionEvents` method**

Add after `_paintMotionIntensity` (after line 314):

```dart
  // ─── Layer 3b: Individual Motion Event Markers ──────────────────────

  void _paintMotionEvents(
    Canvas canvas,
    Size size,
    ({double startSeconds, double endSeconds}) range,
  ) {
    if (events.isEmpty) return;

    for (final evt in events) {
      final evtStartSeconds =
          evt.startTime.difference(dayStart).inMilliseconds / 1000.0;
      final evtEnd = evt.endTime ?? evt.startTime.add(const Duration(seconds: 5));
      final evtEndSeconds =
          evtEnd.difference(dayStart).inMilliseconds / 1000.0;

      // Cull events outside the visible range.
      if (evtEndSeconds < range.startSeconds ||
          evtStartSeconds > range.endSeconds) {
        continue;
      }

      final color = _objectClassColors[evt.objectClass?.toLowerCase() ?? '']
          ?? _defaultEventColor;

      final paint = Paint()..color = color.withValues(alpha: 0.7);

      final x1 = _timeToX(evtStartSeconds, size.width).clamp(0.0, size.width);
      final x2 = _timeToX(evtEndSeconds, size.width).clamp(0.0, size.width);

      // Ensure a minimum width of 2px so very short events are visible.
      final minX2 = (x1 + 2).clamp(0.0, size.width);
      final drawX2 = x2 < minX2 ? minX2 : x2;

      canvas.drawRRect(
        RRect.fromLTRBR(
          x1, _motionEventTop, drawX2, _motionEventTop + _motionEventHeight,
          const Radius.circular(1.5),
        ),
        paint,
      );
    }
  }
```

- [ ] **Step 4: Call `_paintMotionEvents` from `paint()`**

In the `paint` method (around line 75-82), add the call after `_paintMotionIntensity`:

```dart
  @override
  void paint(Canvas canvas, Size size) {
    final range = _visibleRange(size.width);

    _paintTimeGrid(canvas, size, range);
    _paintRecordingSegments(canvas, size, range);
    _paintMotionIntensity(canvas, size, range);
    _paintMotionEvents(canvas, size, range);
    _paintBookmarks(canvas, size, range);
  }
```

- [ ] **Step 5: Update `_timelineHeight` in `fixed_playhead_timeline.dart`**

In `fixed_playhead_timeline.dart`, change line 138:

```dart
  static const double _timelineHeight = 80;
```

- [ ] **Step 6: Verify no analysis errors**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && dart analyze lib/screens/playback/timeline/`
Expected: No issues found.

- [ ] **Step 7: Commit**

```bash
git add clients/flutter/lib/screens/playback/timeline/timeline_painter.dart clients/flutter/lib/screens/playback/timeline/fixed_playhead_timeline.dart
git commit -m "feat(flutter): add color-coded motion event markers to timeline"
```

---

## Task 11: Smoke Test & Final Verification

- [ ] **Step 1: Run all Go tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go test ./internal/nvr/... -v -count=1`
Expected: All tests pass.

- [ ] **Step 2: Run all Flutter tests**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && flutter test`
Expected: All tests pass.

- [ ] **Step 3: Run Flutter analysis**

Run: `cd /Users/ethanflower/personal_projects/mediamtx/clients/flutter && dart analyze lib/`
Expected: No issues found.

- [ ] **Step 4: Verify Go builds**

Run: `cd /Users/ethanflower/personal_projects/mediamtx && go build ./...`
Expected: Clean compile.
