# Playback Timeline & VCR Controls Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the broken playback timeline and VCR controls with a composable timeline showing real recording segments and events, wired to actual video player state via a shared PlaybackController.

**Architecture:** A `PlaybackController` (ChangeNotifier) wraps media_kit players and becomes the single source of truth for position, play state, and speed. The monolithic CustomPaint timeline is replaced by a stack of focused layers (grid, recordings, events, playhead, interaction) sharing a `TimelineViewport` transform. Transport controls, jog slider, and event detail popup are separate widgets.

**Tech Stack:** Flutter 3.2+, Riverpod, media_kit, CustomPaint layers

**Spec:** `docs/superpowers/specs/2026-03-24-playback-timeline-vcr-design.md`

---

## File Structure

All paths relative to `clients/flutter/`.

### New files:
| File | Responsibility |
|------|---------------|
| `lib/screens/playback/playback_controller.dart` | ChangeNotifier wrapping media_kit players; owns position, play state, speed, seek, skip, frame-step |
| `lib/screens/playback/timeline/timeline_viewport.dart` | Manages visible time window, zoom, pan, time-to-pixel conversions |
| `lib/screens/playback/timeline/grid_layer.dart` | CustomPainter for hour/minute grid lines and time labels |
| `lib/screens/playback/timeline/recording_layer.dart` | CustomPainter for recording segment bars |
| `lib/screens/playback/timeline/event_layer.dart` | CustomPainter for event duration bars/dots with color coding |
| `lib/screens/playback/timeline/playhead_layer.dart` | Draggable playhead indicator |
| `lib/screens/playback/timeline/interaction_layer.dart` | Transparent hit-test layer for tap, long-press, pinch, pan |
| `lib/screens/playback/timeline/mini_overview_bar.dart` | 32px overview bar showing full 24h with viewport indicator |
| `lib/screens/playback/timeline/composable_timeline.dart` | Assembles all layers into a Stack |
| `lib/screens/playback/controls/transport_controls.dart` | Row of transport buttons (play, pause, frame step, skip event/gap) |
| `lib/screens/playback/controls/jog_slider.dart` | Horizontal slider for variable-speed scrubbing -2x to +2x |
| `lib/screens/playback/event_detail_popup.dart` | Overlay popup showing event thumbnail, type, confidence, time |
| `test/playback/recording_model_test.dart` | Unit tests for RecordingSegment model |
| `test/playback/timeline_viewport_test.dart` | Unit tests for viewport time-to-pixel math |
| `test/playback/playback_controller_test.dart` | Unit tests for controller seek/skip logic |

### Modified files:
| File | Changes |
|------|---------|
| `lib/models/recording.dart` | Update RecordingSegment to parse full backend response (id, startTime, endTime, durationMs, etc.) |
| `lib/providers/recordings_provider.dart` | Fix RFC3339 timestamps, fix motion events to use `date` param |
| `lib/screens/playback/playback_screen.dart` | Rewire to use PlaybackController, replace layouts with horizontal timeline below video |
| `lib/screens/playback/camera_player.dart` | Receive PlaybackController instead of raw state props |

### Deleted files:
| File | Reason |
|------|--------|
| `lib/screens/playback/timeline_widget.dart` | Replaced by composable timeline |
| `lib/screens/playback/playback_controls.dart` | Replaced by transport controls + jog slider |

---

## Task 1: Fix RecordingSegment model and provider bugs

**Files:**
- Modify: `lib/models/recording.dart:1-11`
- Modify: `lib/providers/recordings_provider.dart:1-50`
- Create: `test/playback/recording_model_test.dart`

This is the foundation — everything else depends on correct data.

- [ ] **Step 1: Write tests for new RecordingSegment model**

Create `test/playback/recording_model_test.dart`:

```dart
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/recording.dart';

void main() {
  group('RecordingSegment', () {
    test('fromJson parses backend response correctly', () {
      final json = {
        'id': 42,
        'camera_id': 'cam-1',
        'start_time': '2026-03-24T10:00:00Z',
        'end_time': '2026-03-24T10:15:00Z',
        'duration_ms': 900000,
        'file_path': '/recordings/cam1/2026-03-24/10-00.fmp4',
        'file_size': 15000000,
        'format': 'fmp4',
      };

      final segment = RecordingSegment.fromJson(json);

      expect(segment.id, 42);
      expect(segment.cameraId, 'cam-1');
      expect(segment.startTime, DateTime.utc(2026, 3, 24, 10, 0, 0));
      expect(segment.endTime, DateTime.utc(2026, 3, 24, 10, 15, 0));
      expect(segment.durationMs, 900000);
      expect(segment.filePath, '/recordings/cam1/2026-03-24/10-00.fmp4');
      expect(segment.fileSize, 15000000);
      expect(segment.format, 'fmp4');
    });

    test('fromJson handles nullable fields', () {
      final json = {
        'id': 1,
        'camera_id': 'cam-1',
        'start_time': '2026-03-24T10:00:00Z',
        'end_time': '2026-03-24T10:15:00Z',
        'duration_ms': 900000,
      };

      final segment = RecordingSegment.fromJson(json);

      expect(segment.filePath, isNull);
      expect(segment.fileSize, isNull);
      expect(segment.format, isNull);
    });
  });

  group('MotionEvent', () {
    test('fromJson parses id as string from int', () {
      final json = {
        'id': 99,
        'camera_id': 'cam-1',
        'started_at': '2026-03-24T10:05:00Z',
        'ended_at': '2026-03-24T10:05:30Z',
        'thumbnail_path': '/thumbnails/event_99.jpg',
        'event_type': 'motion',
        'object_class': 'person',
        'confidence': 0.92,
      };

      final event = MotionEvent.fromJson(json);

      expect(event.cameraId, 'cam-1');
      expect(event.startTime, DateTime.utc(2026, 3, 24, 10, 5, 0));
      expect(event.endTime, DateTime.utc(2026, 3, 24, 10, 5, 30));
      expect(event.objectClass, 'person');
      expect(event.confidence, 0.92);
    });
  });
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd clients/flutter && flutter test test/playback/recording_model_test.dart`
Expected: FAIL — RecordingSegment constructor doesn't match.

- [ ] **Step 3: Update RecordingSegment model**

Replace `lib/models/recording.dart` lines 1-11 with:

```dart
class RecordingSegment {
  final int id;
  final String cameraId;
  final DateTime startTime;
  final DateTime endTime;
  final int durationMs;
  final String? filePath;
  final int? fileSize;
  final String? format;

  const RecordingSegment({
    required this.id,
    required this.cameraId,
    required this.startTime,
    required this.endTime,
    required this.durationMs,
    this.filePath,
    this.fileSize,
    this.format,
  });

  factory RecordingSegment.fromJson(Map<String, dynamic> json) {
    return RecordingSegment(
      id: json['id'] as int,
      cameraId: json['camera_id'] as String,
      startTime: DateTime.parse(json['start_time'] as String),
      endTime: DateTime.parse(json['end_time'] as String),
      durationMs: json['duration_ms'] as int,
      filePath: json['file_path'] as String?,
      fileSize: json['file_size'] as int?,
      format: json['format'] as String?,
    );
  }
}
```

Also fix MotionEvent.fromJson to handle `id` coming as int from backend (currently casts as String):

```dart
factory MotionEvent.fromJson(Map<String, dynamic> json) {
  return MotionEvent(
    id: json['id'].toString(),
    cameraId: json['camera_id'] as String,
    startedAt: json['started_at'] as String,
    endedAt: json['ended_at'] as String?,
    thumbnailPath: json['thumbnail_path'] as String?,
    eventType: json['event_type'] as String?,
    objectClass: json['object_class'] as String?,
    confidence: (json['confidence'] as num?)?.toDouble(),
  );
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd clients/flutter && flutter test test/playback/recording_model_test.dart`
Expected: ALL PASS

- [ ] **Step 5: Fix provider bugs**

In `lib/providers/recordings_provider.dart`:

For `recordingSegmentsProvider` (line 13-14), change timestamps to include timezone:
```dart
final start = '${key.date}T00:00:00Z';
final end = '${key.date}T23:59:59Z';
```

For `motionEventsProvider` (lines 30-49), replace the entire provider body:
```dart
final motionEventsProvider =
    FutureProvider.family<List<MotionEvent>, RecordingsKey>((ref, key) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];

  final res = await api.get<dynamic>(
    '/cameras/${key.cameraId}/motion-events',
    queryParameters: {'date': key.date},
  );

  final data = res.data;
  if (data == null) return [];
  final list = data is List ? data : (data['events'] as List? ?? []);
  return list
      .map((e) => MotionEvent.fromJson(e as Map<String, dynamic>))
      .toList();
});
```

- [ ] **Step 6: Commit**

```bash
git add lib/models/recording.dart lib/providers/recordings_provider.dart test/playback/recording_model_test.dart
git commit -m "fix: update RecordingSegment model and fix provider API bugs"
```

---

## Task 2: Create TimelineViewport

**Files:**
- Create: `lib/screens/playback/timeline/timeline_viewport.dart`
- Create: `test/playback/timeline_viewport_test.dart`

The viewport is the shared coordinate system for all timeline layers.

- [ ] **Step 1: Write tests for TimelineViewport**

Create `test/playback/timeline_viewport_test.dart`:

```dart
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/screens/playback/timeline/timeline_viewport.dart';

void main() {
  group('TimelineViewport', () {
    test('full 24h view maps midnight to 0 and end to width', () {
      final vp = TimelineViewport(
        visibleStart: Duration.zero,
        visibleEnd: const Duration(hours: 24),
        widthPx: 1000,
      );

      expect(vp.timeToPixel(Duration.zero), 0.0);
      expect(vp.timeToPixel(const Duration(hours: 24)), 1000.0);
      expect(vp.timeToPixel(const Duration(hours: 12)), 500.0);
    });

    test('pixelToTime is inverse of timeToPixel', () {
      final vp = TimelineViewport(
        visibleStart: Duration.zero,
        visibleEnd: const Duration(hours: 24),
        widthPx: 1000,
      );

      final time = const Duration(hours: 6, minutes: 30);
      final px = vp.timeToPixel(time);
      final roundTrip = vp.pixelToTime(px);

      expect(roundTrip.inSeconds, time.inSeconds);
    });

    test('zoomed view maps correctly', () {
      // Viewing hours 10-14 (4 hours) in 800px
      final vp = TimelineViewport(
        visibleStart: const Duration(hours: 10),
        visibleEnd: const Duration(hours: 14),
        widthPx: 800,
      );

      expect(vp.timeToPixel(const Duration(hours: 10)), 0.0);
      expect(vp.timeToPixel(const Duration(hours: 14)), 800.0);
      expect(vp.timeToPixel(const Duration(hours: 12)), 400.0);
      // Outside visible range returns negative or > width
      expect(vp.timeToPixel(const Duration(hours: 9)), lessThan(0));
    });

    test('zoomLevel computes correctly', () {
      final vp = TimelineViewport(
        visibleStart: const Duration(hours: 10),
        visibleEnd: const Duration(hours: 14),
        widthPx: 800,
      );

      // 24h / 4h = 6x zoom
      expect(vp.zoomLevel, 6.0);
    });

    test('visibleDuration returns correct span', () {
      final vp = TimelineViewport(
        visibleStart: const Duration(hours: 3),
        visibleEnd: const Duration(hours: 9),
        widthPx: 600,
      );

      expect(vp.visibleDuration, const Duration(hours: 6));
    });

    test('gridInterval adapts to zoom level', () {
      // At 1x zoom (24h), grid interval should be 3 hours
      final vp1x = TimelineViewport(
        visibleStart: Duration.zero,
        visibleEnd: const Duration(hours: 24),
        widthPx: 1000,
      );
      expect(vp1x.gridInterval, const Duration(hours: 3));

      // At 6x zoom (4h), grid interval should be 30 minutes
      final vp6x = TimelineViewport(
        visibleStart: const Duration(hours: 10),
        visibleEnd: const Duration(hours: 14),
        widthPx: 1000,
      );
      expect(vp6x.gridInterval, const Duration(minutes: 30));

      // At 24x zoom (1h), grid interval should be 5 minutes
      final vp24x = TimelineViewport(
        visibleStart: const Duration(hours: 10),
        visibleEnd: const Duration(hours: 11),
        widthPx: 1000,
      );
      expect(vp24x.gridInterval, const Duration(minutes: 5));
    });
  });
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd clients/flutter && flutter test test/playback/timeline_viewport_test.dart`
Expected: FAIL — file doesn't exist yet.

- [ ] **Step 3: Implement TimelineViewport**

Create `lib/screens/playback/timeline/timeline_viewport.dart`:

```dart
class TimelineViewport {
  final Duration visibleStart;
  final Duration visibleEnd;
  final double widthPx;

  static const Duration dayDuration = Duration(hours: 24);

  const TimelineViewport({
    required this.visibleStart,
    required this.visibleEnd,
    required this.widthPx,
  });

  Duration get visibleDuration => visibleEnd - visibleStart;

  double get zoomLevel =>
      dayDuration.inMilliseconds / visibleDuration.inMilliseconds;

  double timeToPixel(Duration time) {
    final offset = (time - visibleStart).inMilliseconds;
    final span = visibleDuration.inMilliseconds;
    if (span == 0) return 0;
    return (offset / span) * widthPx;
  }

  Duration pixelToTime(double px) {
    final frac = px / widthPx;
    final ms = visibleStart.inMilliseconds +
        (frac * visibleDuration.inMilliseconds).round();
    return Duration(milliseconds: ms.clamp(0, dayDuration.inMilliseconds));
  }

  /// Returns the appropriate grid interval based on zoom level.
  Duration get gridInterval {
    final z = zoomLevel;
    if (z < 2) return const Duration(hours: 3);
    if (z < 6) return const Duration(hours: 1);
    if (z < 12) return const Duration(minutes: 30);
    if (z < 24) return const Duration(minutes: 15);
    if (z < 48) return const Duration(minutes: 5);
    return const Duration(minutes: 1);
  }

  /// Returns a new viewport zoomed around [focalTime] by [factor].
  /// factor > 1 zooms in, factor < 1 zooms out.
  TimelineViewport zoom(double factor, Duration focalTime) {
    final newDurationMs =
        (visibleDuration.inMilliseconds / factor).round();
    final clampedDuration = newDurationMs.clamp(
      const Duration(minutes: 24).inMilliseconds, // max zoom ~60x
      dayDuration.inMilliseconds,
    );

    // Keep focal point at same relative position
    final focalFrac = (focalTime - visibleStart).inMilliseconds /
        visibleDuration.inMilliseconds;
    final newStartMs =
        focalTime.inMilliseconds - (focalFrac * clampedDuration).round();
    final newStart = Duration(
        milliseconds: newStartMs.clamp(0, dayDuration.inMilliseconds - clampedDuration));
    final newEnd = Duration(
        milliseconds: (newStart.inMilliseconds + clampedDuration)
            .clamp(0, dayDuration.inMilliseconds));

    return TimelineViewport(
      visibleStart: newStart,
      visibleEnd: newEnd,
      widthPx: widthPx,
    );
  }

  /// Returns a new viewport panned by [deltaPx] pixels.
  TimelineViewport pan(double deltaPx) {
    final deltaMs =
        (deltaPx / widthPx * visibleDuration.inMilliseconds).round();
    var newStartMs = visibleStart.inMilliseconds - deltaMs;
    newStartMs = newStartMs.clamp(
        0, dayDuration.inMilliseconds - visibleDuration.inMilliseconds);
    return TimelineViewport(
      visibleStart: Duration(milliseconds: newStartMs),
      visibleEnd:
          Duration(milliseconds: newStartMs + visibleDuration.inMilliseconds),
      widthPx: widthPx,
    );
  }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd clients/flutter && flutter test test/playback/timeline_viewport_test.dart`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add lib/screens/playback/timeline/timeline_viewport.dart test/playback/timeline_viewport_test.dart
git commit -m "feat: add TimelineViewport with zoom, pan, and grid interval logic"
```

---

## Task 3: Create PlaybackController

**Files:**
- Create: `lib/screens/playback/playback_controller.dart`
- Create: `test/playback/playback_controller_test.dart`

The controller is the single source of truth for playback state.

- [ ] **Step 1: Write tests for PlaybackController skip/gap logic**

Create `test/playback/playback_controller_test.dart`:

```dart
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/recording.dart';
import 'package:nvr_client/screens/playback/playback_controller.dart';

void main() {
  group('PlaybackController segment helpers', () {
    // Test the static helper methods without needing media_kit
    final segments = [
      RecordingSegment(
        id: 1, cameraId: 'c1',
        startTime: DateTime.utc(2026, 3, 24, 8, 0),
        endTime: DateTime.utc(2026, 3, 24, 9, 0),
        durationMs: 3600000,
      ),
      RecordingSegment(
        id: 2, cameraId: 'c1',
        startTime: DateTime.utc(2026, 3, 24, 10, 0),
        endTime: DateTime.utc(2026, 3, 24, 11, 30),
        durationMs: 5400000,
      ),
      RecordingSegment(
        id: 3, cameraId: 'c1',
        startTime: DateTime.utc(2026, 3, 24, 14, 0),
        endTime: DateTime.utc(2026, 3, 24, 16, 0),
        durationMs: 7200000,
      ),
    ];

    final dayStart = DateTime.utc(2026, 3, 24);

    test('findContainingSegment returns segment when position is inside', () {
      final pos = const Duration(hours: 8, minutes: 30);
      final result = PlaybackController.findContainingSegment(segments, dayStart, pos);
      expect(result?.id, 1);
    });

    test('findContainingSegment returns null when position is in gap', () {
      final pos = const Duration(hours: 9, minutes: 30); // gap between seg 1 and 2
      final result = PlaybackController.findContainingSegment(segments, dayStart, pos);
      expect(result, isNull);
    });

    test('findNextSegmentStart returns next segment after gap position', () {
      final pos = const Duration(hours: 9, minutes: 30);
      final result = PlaybackController.findNextSegmentStart(segments, dayStart, pos);
      expect(result, const Duration(hours: 10));
    });

    test('findNextSegmentStart returns null after last segment', () {
      final pos = const Duration(hours: 17);
      final result = PlaybackController.findNextSegmentStart(segments, dayStart, pos);
      expect(result, isNull);
    });

    test('snapToSegment seeks to next segment start when in gap', () {
      final pos = const Duration(hours: 9, minutes: 30);
      final snapped = PlaybackController.snapToSegment(segments, dayStart, pos);
      expect(snapped, const Duration(hours: 10));
    });

    test('snapToSegment returns same position when inside segment', () {
      final pos = const Duration(hours: 8, minutes: 30);
      final snapped = PlaybackController.snapToSegment(segments, dayStart, pos);
      expect(snapped, pos);
    });
  });

  group('PlaybackController event skip helpers', () {
    final events = [
      MotionEvent(id: '1', cameraId: 'c1', startedAt: '2026-03-24T08:05:00Z'),
      MotionEvent(id: '2', cameraId: 'c1', startedAt: '2026-03-24T10:30:00Z'),
      MotionEvent(id: '3', cameraId: 'c1', startedAt: '2026-03-24T14:15:00Z'),
    ];

    final dayStart = DateTime.utc(2026, 3, 24);

    test('findNextEvent returns first event after position', () {
      final pos = const Duration(hours: 9);
      final result = PlaybackController.findNextEvent(events, dayStart, pos);
      expect(result, const Duration(hours: 10, minutes: 30));
    });

    test('findPreviousEvent returns last event before position', () {
      final pos = const Duration(hours: 12);
      final result = PlaybackController.findPreviousEvent(events, dayStart, pos);
      expect(result, const Duration(hours: 10, minutes: 30));
    });

    test('findNextEvent returns null when no events after position', () {
      final pos = const Duration(hours: 15);
      final result = PlaybackController.findNextEvent(events, dayStart, pos);
      expect(result, isNull);
    });

    test('findPreviousEvent returns null when no events before position', () {
      final pos = const Duration(hours: 7);
      final result = PlaybackController.findPreviousEvent(events, dayStart, pos);
      expect(result, isNull);
    });
  });

  group('PlaybackController gap skip helpers', () {
    final segments = [
      RecordingSegment(
        id: 1, cameraId: 'c1',
        startTime: DateTime.utc(2026, 3, 24, 8, 0),
        endTime: DateTime.utc(2026, 3, 24, 9, 0),
        durationMs: 3600000,
      ),
      RecordingSegment(
        id: 2, cameraId: 'c1',
        startTime: DateTime.utc(2026, 3, 24, 10, 0),
        endTime: DateTime.utc(2026, 3, 24, 11, 30),
        durationMs: 5400000,
      ),
    ];

    final dayStart = DateTime.utc(2026, 3, 24);

    test('findNextGapEnd returns start of next segment after gap', () {
      // At 8:30 (inside segment 1), next gap ends at 10:00 (start of segment 2)
      final pos = const Duration(hours: 8, minutes: 30);
      final result = PlaybackController.findNextGapEnd(segments, dayStart, pos);
      expect(result, const Duration(hours: 10));
    });

    test('findPreviousGapStart returns end of previous segment before gap', () {
      // At 10:30 (inside segment 2), previous gap starts at 9:00 (end of segment 1)
      final pos = const Duration(hours: 10, minutes: 30);
      final result = PlaybackController.findPreviousGapStart(segments, dayStart, pos);
      expect(result, const Duration(hours: 9));
    });
  });
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `cd clients/flutter && flutter test test/playback/playback_controller_test.dart`
Expected: FAIL — PlaybackController doesn't exist yet.

- [ ] **Step 3: Implement PlaybackController**

Create `lib/screens/playback/playback_controller.dart`:

```dart
import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:media_kit/media_kit.dart';
import '../../models/recording.dart';
import '../../services/playback_service.dart';

class PlaybackController extends ChangeNotifier {
  final PlaybackService playbackService;

  // State
  Duration _position = Duration.zero;
  bool _isPlaying = false;
  double _speed = 1.0;
  bool _isSeeking = false;
  DateTime _selectedDate = DateTime.now();
  List<String> _selectedCameraIds = [];
  List<RecordingSegment> _segments = [];
  List<MotionEvent> _events = [];

  // Players keyed by camera ID
  final Map<String, Player> _players = {};
  final Map<String, VideoController> _videoControllers = {};
  StreamSubscription<Duration>? _positionSub;

  static const _maxPosition = Duration(hours: 24);
  static const _positionThrottle = Duration(milliseconds: 66); // ~15fps

  PlaybackController({required this.playbackService});

  // Getters
  Duration get position => _position;
  bool get isPlaying => _isPlaying;
  double get speed => _speed;
  bool get isSeeking => _isSeeking;
  DateTime get selectedDate => _selectedDate;
  List<String> get selectedCameraIds => _selectedCameraIds;
  List<RecordingSegment> get segments => _segments;
  List<MotionEvent> get events => _events;
  Map<String, VideoController> get videoControllers => _videoControllers;

  // ── Data setters ──────────────────────────────────────────────────────

  void setSegments(List<RecordingSegment> segments) {
    _segments = segments;
  }

  void setEvents(List<MotionEvent> events) {
    _events = events;
  }

  void setSelectedDate(DateTime date) {
    _selectedDate = date;
    _position = Duration.zero;
    _rebuildPlayers();
    notifyListeners();
  }

  void setSelectedCameraIds(List<String> ids) {
    // Avoid redundant rebuilds when called from build()
    if (_listEquals(ids, _selectedCameraIds)) return;
    _selectedCameraIds = ids;
    _rebuildPlayers();
    notifyListeners();
  }

  static bool _listEquals(List<String> a, List<String> b) {
    if (a.length != b.length) return false;
    for (int i = 0; i < a.length; i++) {
      if (a[i] != b[i]) return false;
    }
    return true;
  }

  // ── Player management ─────────────────────────────────────────────────

  void _rebuildPlayers() {
    // Dispose players no longer needed
    final toRemove = _players.keys
        .where((id) => !_selectedCameraIds.contains(id))
        .toList();
    for (final id in toRemove) {
      _players[id]?.dispose();
      _players.remove(id);
      _videoControllers.remove(id);
    }

    // Create players for new cameras
    for (final id in _selectedCameraIds) {
      if (!_players.containsKey(id)) {
        final player = Player();
        _players[id] = player;
        _videoControllers[id] = VideoController(player);
      }
    }

    // Subscribe to first player's position stream
    _positionSub?.cancel();
    if (_players.isNotEmpty) {
      DateTime? lastUpdate;
      _positionSub = _players.values.first.stream.position.listen((pos) {
        if (_isSeeking) return;
        final now = DateTime.now();
        if (lastUpdate != null &&
            now.difference(lastUpdate!) < _positionThrottle) {
          return;
        }
        lastUpdate = now;
        _position = pos;

        // Auto-pause at end of last recording segment
        if (_isPlaying && _segments.isNotEmpty) {
          final dayStart = DateTime(
              _selectedDate.year, _selectedDate.month, _selectedDate.day);
          final lastEnd = _segments.last.endTime.difference(dayStart);
          if (pos >= lastEnd) {
            pause();
          }
        }

        notifyListeners();
      });
    }

    _openAllPlayers();
  }

  void _openAllPlayers() {
    final dayStart = DateTime(
        _selectedDate.year, _selectedDate.month, _selectedDate.day);

    for (final id in _selectedCameraIds) {
      final player = _players[id];
      if (player == null) continue;

      // Find camera path — caller must have set this via setCameraPaths
      final path = _cameraPaths[id];
      if (path == null) continue;

      final url = playbackService.playbackUrl(path, dayStart);
      player.open(Media(url), play: _isPlaying);
      player.setRate(_speed);
    }
  }

  // Camera path mapping (mediamtxPath per camera ID)
  final Map<String, String> _cameraPaths = {};

  void setCameraPaths(Map<String, String> paths) {
    _cameraPaths.addAll(paths);
  }

  // ── Playback controls ─────────────────────────────────────────────────

  void play() {
    _isPlaying = true;
    for (final p in _players.values) {
      p.play();
    }
    notifyListeners();
  }

  void pause() {
    _isPlaying = false;
    for (final p in _players.values) {
      p.pause();
    }
    notifyListeners();
  }

  void togglePlayPause() {
    if (_isPlaying) {
      pause();
    } else {
      play();
    }
  }

  void setSpeed(double speed) {
    _speed = speed;
    for (final p in _players.values) {
      p.setRate(speed);
    }
    notifyListeners();
  }

  Future<void> seek(Duration target) async {
    final clamped = Duration(
      milliseconds: target.inMilliseconds.clamp(0, _maxPosition.inMilliseconds),
    );

    // Snap to segment if seeking into a gap
    final dayStart = DateTime(
        _selectedDate.year, _selectedDate.month, _selectedDate.day);
    final snapped = snapToSegment(_segments, dayStart, clamped);

    _isSeeking = true;
    _position = snapped;
    notifyListeners();

    final futures = _players.values.map((p) => p.seek(snapped));
    await Future.wait(futures);

    _isSeeking = false;
    notifyListeners();
  }

  void stepFrame(int direction) {
    if (_isPlaying) pause();

    if (direction > 0) {
      // Forward: use native frame step if available
      for (final p in _players.values) {
        // media_kit doesn't expose player.step() directly,
        // so we seek forward by ~33ms
        p.seek(_position + const Duration(milliseconds: 33));
      }
      _position += const Duration(milliseconds: 33);
    } else {
      // Backward: seek to prior keyframe (~3s back)
      final newPos = Duration(
        milliseconds:
            (_position.inMilliseconds - 3000).clamp(0, _maxPosition.inMilliseconds),
      );
      for (final p in _players.values) {
        p.seek(newPos);
      }
      _position = newPos;
    }
    notifyListeners();
  }

  void skipToNextEvent() {
    final dayStart = DateTime(
        _selectedDate.year, _selectedDate.month, _selectedDate.day);
    final target = findNextEvent(_events, dayStart, _position);
    if (target != null) seek(target);
  }

  void skipToPreviousEvent() {
    final dayStart = DateTime(
        _selectedDate.year, _selectedDate.month, _selectedDate.day);
    final target = findPreviousEvent(_events, dayStart, _position);
    if (target != null) seek(target);
  }

  void skipToNextGap() {
    final dayStart = DateTime(
        _selectedDate.year, _selectedDate.month, _selectedDate.day);
    final target = findNextGapEnd(_segments, dayStart, _position);
    if (target != null) seek(target);
  }

  void skipToPreviousGap() {
    final dayStart = DateTime(
        _selectedDate.year, _selectedDate.month, _selectedDate.day);
    final target = findPreviousGapStart(_segments, dayStart, _position);
    if (target != null) seek(target);
  }

  // ── Static helpers (testable without media_kit) ───────────────────────

  static RecordingSegment? findContainingSegment(
      List<RecordingSegment> segments, DateTime dayStart, Duration position) {
    final posTime = dayStart.add(position);
    for (final seg in segments) {
      if (!posTime.isBefore(seg.startTime) && posTime.isBefore(seg.endTime)) {
        return seg;
      }
    }
    return null;
  }

  static Duration? findNextSegmentStart(
      List<RecordingSegment> segments, DateTime dayStart, Duration position) {
    final posTime = dayStart.add(position);
    for (final seg in segments) {
      if (seg.startTime.isAfter(posTime)) {
        return seg.startTime.difference(dayStart);
      }
    }
    return null;
  }

  static Duration snapToSegment(
      List<RecordingSegment> segments, DateTime dayStart, Duration position) {
    if (segments.isEmpty) return position;
    final containing = findContainingSegment(segments, dayStart, position);
    if (containing != null) return position;
    return findNextSegmentStart(segments, dayStart, position) ?? position;
  }

  static Duration? findNextEvent(
      List<MotionEvent> events, DateTime dayStart, Duration position) {
    final posTime = dayStart.add(position);
    for (final e in events) {
      if (e.startTime.isAfter(posTime)) {
        return e.startTime.difference(dayStart);
      }
    }
    return null;
  }

  static Duration? findPreviousEvent(
      List<MotionEvent> events, DateTime dayStart, Duration position) {
    final posTime = dayStart.add(position);
    Duration? result;
    for (final e in events) {
      if (e.startTime.isBefore(posTime)) {
        result = e.startTime.difference(dayStart);
      }
    }
    return result;
  }

  static Duration? findNextGapEnd(
      List<RecordingSegment> segments, DateTime dayStart, Duration position) {
    final posTime = dayStart.add(position);
    for (int i = 0; i < segments.length - 1; i++) {
      final gapStart = segments[i].endTime;
      final gapEnd = segments[i + 1].startTime;
      if (gapEnd.isAfter(posTime) && gapStart != gapEnd) {
        return gapEnd.difference(dayStart);
      }
    }
    return null;
  }

  static Duration? findPreviousGapStart(
      List<RecordingSegment> segments, DateTime dayStart, Duration position) {
    final posTime = dayStart.add(position);
    Duration? result;
    for (int i = 0; i < segments.length - 1; i++) {
      final gapStart = segments[i].endTime;
      final gapEnd = segments[i + 1].startTime;
      if (gapStart.isBefore(posTime) && gapStart != gapEnd) {
        result = gapStart.difference(dayStart);
      }
    }
    return result;
  }

  @override
  void dispose() {
    _positionSub?.cancel();
    for (final p in _players.values) {
      p.dispose();
    }
    _players.clear();
    _videoControllers.clear();
    super.dispose();
  }
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `cd clients/flutter && flutter test test/playback/playback_controller_test.dart`
Expected: ALL PASS

- [ ] **Step 5: Commit**

```bash
git add lib/screens/playback/playback_controller.dart test/playback/playback_controller_test.dart
git commit -m "feat: add PlaybackController with seek, skip, and frame-step logic"
```

---

## Task 4: Create GridLayer

**Files:**
- Create: `lib/screens/playback/timeline/grid_layer.dart`

No unit test needed — this is pure painting. Verified visually when composable timeline is assembled.

- [ ] **Step 1: Implement GridLayer**

Create `lib/screens/playback/timeline/grid_layer.dart`:

```dart
import 'package:flutter/material.dart';
import '../../../theme/nvr_colors.dart';
import 'timeline_viewport.dart';

class GridLayer extends CustomPainter {
  final TimelineViewport viewport;

  GridLayer({required this.viewport});

  @override
  void paint(Canvas canvas, Size size) {
    final gridPaint = Paint()
      ..color = NvrColors.border.withOpacity(0.3)
      ..strokeWidth = 0.5;

    final interval = viewport.gridInterval;
    final labelStyle = TextStyle(
      color: NvrColors.textMuted,
      fontSize: 10,
    );

    // Walk through the visible range at grid intervals
    var t = Duration(
      milliseconds: (viewport.visibleStart.inMilliseconds ~/
              interval.inMilliseconds) *
          interval.inMilliseconds,
    );

    while (t <= viewport.visibleEnd) {
      if (t >= viewport.visibleStart) {
        final x = viewport.timeToPixel(t);

        // Grid line
        canvas.drawLine(
          Offset(x, 0),
          Offset(x, size.height),
          gridPaint,
        );

        // Time label
        final hours = t.inHours % 24;
        final minutes = t.inMinutes % 60;
        final label = minutes == 0
            ? '${hours.toString().padLeft(2, '0')}:00'
            : '${hours.toString().padLeft(2, '0')}:${minutes.toString().padLeft(2, '0')}';

        final tp = TextPainter(
          text: TextSpan(text: label, style: labelStyle),
          textDirection: TextDirection.ltr,
        )..layout();

        tp.paint(canvas, Offset(x + 4, size.height - tp.height - 4));
      }

      t += interval;
    }
  }

  @override
  bool shouldRepaint(GridLayer old) =>
      old.viewport.visibleStart != viewport.visibleStart ||
      old.viewport.visibleEnd != viewport.visibleEnd ||
      old.viewport.widthPx != viewport.widthPx;
}
```

- [ ] **Step 2: Commit**

```bash
git add lib/screens/playback/timeline/grid_layer.dart
git commit -m "feat: add GridLayer for timeline hour/minute grid lines"
```

---

## Task 5: Create RecordingLayer

**Files:**
- Create: `lib/screens/playback/timeline/recording_layer.dart`

- [ ] **Step 1: Implement RecordingLayer**

Create `lib/screens/playback/timeline/recording_layer.dart`:

```dart
import 'package:flutter/material.dart';
import '../../../models/recording.dart';
import '../../../theme/nvr_colors.dart';
import 'timeline_viewport.dart';

class RecordingLayer extends CustomPainter {
  final TimelineViewport viewport;
  final List<RecordingSegment> segments;
  final DateTime dayStart;

  RecordingLayer({
    required this.viewport,
    required this.segments,
    required this.dayStart,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()..color = NvrColors.accent.withOpacity(0.25);

    for (final seg in segments) {
      final startDur = seg.startTime.difference(dayStart);
      final endDur = seg.endTime.difference(dayStart);

      final x1 = viewport.timeToPixel(startDur);
      final x2 = viewport.timeToPixel(endDur);

      // Skip segments entirely outside visible range
      if (x2 < 0 || x1 > viewport.widthPx) continue;

      canvas.drawRect(
        Rect.fromLTRB(
          x1.clamp(0, viewport.widthPx),
          0,
          x2.clamp(0, viewport.widthPx),
          size.height,
        ),
        paint,
      );
    }
  }

  @override
  bool shouldRepaint(RecordingLayer old) =>
      old.segments != segments ||
      old.viewport.visibleStart != viewport.visibleStart ||
      old.viewport.visibleEnd != viewport.visibleEnd;
}
```

- [ ] **Step 2: Commit**

```bash
git add lib/screens/playback/timeline/recording_layer.dart
git commit -m "feat: add RecordingLayer for timeline recording segment bars"
```

---

## Task 6: Create EventLayer

**Files:**
- Create: `lib/screens/playback/timeline/event_layer.dart`

- [ ] **Step 1: Implement EventLayer**

Create `lib/screens/playback/timeline/event_layer.dart`:

```dart
import 'package:flutter/material.dart';
import '../../../models/recording.dart';
import 'timeline_viewport.dart';

class EventLayer extends CustomPainter {
  final TimelineViewport viewport;
  final List<MotionEvent> events;
  final DateTime dayStart;

  EventLayer({
    required this.viewport,
    required this.events,
    required this.dayStart,
  });

  static const _eventColors = <String, Color>{
    'person': Colors.blue,
    'vehicle': Colors.green,
    'car': Colors.green,
    'truck': Colors.green,
    'bus': Colors.green,
    'motorcycle': Colors.green,
    'motion': Colors.amber,
  };

  static Color colorForClass(String? objectClass) {
    if (objectClass == null) return Colors.red;
    return _eventColors[objectClass.toLowerCase()] ?? Colors.red;
  }

  @override
  void paint(Canvas canvas, Size size) {
    final showBars = viewport.zoomLevel > 4;

    for (final event in events) {
      final startDur = event.startTime.difference(dayStart);
      final x1 = viewport.timeToPixel(startDur);

      // Skip events outside visible range
      if (x1 > viewport.widthPx + 10) continue;

      final color = colorForClass(event.objectClass);
      final y = size.height * 0.6; // Events sit in lower portion

      if (showBars && event.endTime != null) {
        // Duration bar
        final endDur = event.endTime!.difference(dayStart);
        final x2 = viewport.timeToPixel(endDur);

        if (x2 < -10) continue;

        final barPaint = Paint()..color = color.withOpacity(0.6);
        canvas.drawRRect(
          RRect.fromRectAndRadius(
            Rect.fromLTRB(
              x1.clamp(0, viewport.widthPx),
              y - 4,
              x2.clamp(0, viewport.widthPx).clamp(x1 + 2, viewport.widthPx),
              y + 4,
            ),
            const Radius.circular(2),
          ),
          barPaint,
        );
      } else {
        // Dot
        if (x1 < -10) continue;
        canvas.drawCircle(
          Offset(x1.clamp(0, viewport.widthPx), y),
          4,
          Paint()..color = color,
        );
      }
    }
  }

  @override
  bool shouldRepaint(EventLayer old) =>
      old.events != events ||
      old.viewport.visibleStart != viewport.visibleStart ||
      old.viewport.visibleEnd != viewport.visibleEnd;
}
```

- [ ] **Step 2: Commit**

```bash
git add lib/screens/playback/timeline/event_layer.dart
git commit -m "feat: add EventLayer for timeline event bars and dots"
```

---

## Task 7: Create PlayheadLayer

**Files:**
- Create: `lib/screens/playback/timeline/playhead_layer.dart`

- [ ] **Step 1: Implement PlayheadLayer**

Create `lib/screens/playback/timeline/playhead_layer.dart`:

PlayheadLayer is purely visual — no gesture handling. All gestures (including playhead dragging) are handled by InteractionLayer to avoid gesture conflicts.

```dart
import 'package:flutter/material.dart';
import '../../../theme/nvr_colors.dart';
import 'timeline_viewport.dart';

/// Purely visual — paints the playhead line and handle.
/// All gesture handling lives in InteractionLayer.
class PlayheadLayer extends CustomPainter {
  final TimelineViewport viewport;
  final Duration position;
  final bool isDragging;
  final double? dragX; // override position when dragging

  PlayheadLayer({
    required this.viewport,
    required this.position,
    this.isDragging = false,
    this.dragX,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final x = dragX ?? viewport.timeToPixel(position);

    final paint = Paint()
      ..color = NvrColors.accent
      ..strokeWidth = 2;

    // Vertical line
    canvas.drawLine(Offset(x, 0), Offset(x, size.height), paint);

    // Handle circle
    final radius = isDragging ? 8.0 : 6.0;
    canvas.drawCircle(
      Offset(x, size.height / 2),
      radius,
      Paint()..color = NvrColors.accent,
    );
    canvas.drawCircle(
      Offset(x, size.height / 2),
      radius,
      Paint()
        ..color = Colors.white
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2,
    );
  }

  @override
  bool shouldRepaint(PlayheadLayer old) =>
      old.position != position ||
      old.isDragging != isDragging ||
      old.dragX != dragX;
}
```

- [ ] **Step 2: Commit**

```bash
git add lib/screens/playback/timeline/playhead_layer.dart
git commit -m "feat: add PlayheadLayer (visual-only) for playhead line and handle"
```

---

## Task 8: Create InteractionLayer

**Files:**
- Create: `lib/screens/playback/timeline/interaction_layer.dart`

- [ ] **Step 1: Implement InteractionLayer**

Create `lib/screens/playback/timeline/interaction_layer.dart`:

InteractionLayer is the ONLY gesture handler in the timeline stack. It handles tap-to-seek, playhead dragging, long-press for events, pinch-to-zoom, and pan. This avoids gesture conflicts between multiple GestureDetectors.

```dart
import 'package:flutter/material.dart';
import '../../../models/recording.dart';
import 'timeline_viewport.dart';

class InteractionLayer extends StatefulWidget {
  final TimelineViewport viewport;
  final Duration position;
  final List<MotionEvent> events;
  final DateTime dayStart;
  final ValueChanged<Duration> onSeek;
  final void Function(MotionEvent event, Offset position) onEventLongPress;
  final ValueChanged<double> onZoom; // scale factor
  final ValueChanged<double> onPan; // delta pixels
  // Playhead drag state callbacks
  final ValueChanged<bool> onDragStateChanged;
  final ValueChanged<double?> onDragXChanged;

  const InteractionLayer({
    super.key,
    required this.viewport,
    required this.position,
    required this.events,
    required this.dayStart,
    required this.onSeek,
    required this.onEventLongPress,
    required this.onZoom,
    required this.onPan,
    required this.onDragStateChanged,
    required this.onDragXChanged,
  });

  @override
  State<InteractionLayer> createState() => _InteractionLayerState();
}

class _InteractionLayerState extends State<InteractionLayer> {
  bool _draggingPlayhead = false;

  bool _isNearPlayhead(double px) {
    final playheadX = widget.viewport.timeToPixel(widget.position);
    return (px - playheadX).abs() < 20;
  }

  MotionEvent? _findNearestEvent(double px) {
    MotionEvent? nearest;
    double nearestPxDist = 20; // max 20px match distance

    for (final event in widget.events) {
      final eventDur = event.startTime.difference(widget.dayStart);
      final pxDist = (widget.viewport.timeToPixel(eventDur) - px).abs();
      if (pxDist < nearestPxDist) {
        nearest = event;
        nearestPxDist = pxDist;
      }
    }
    return nearest;
  }

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTapUp: (details) {
        final time = widget.viewport.pixelToTime(details.localPosition.dx);
        widget.onSeek(time);
      },
      onLongPressStart: (details) {
        final event = _findNearestEvent(details.localPosition.dx);
        if (event != null) {
          widget.onEventLongPress(event, details.globalPosition);
        }
      },
      onHorizontalDragStart: (details) {
        if (_isNearPlayhead(details.localPosition.dx)) {
          _draggingPlayhead = true;
          widget.onDragStateChanged(true);
          widget.onDragXChanged(details.localPosition.dx);
        }
      },
      onHorizontalDragUpdate: (details) {
        if (_draggingPlayhead) {
          final x = details.localPosition.dx.clamp(0, widget.viewport.widthPx);
          widget.onDragXChanged(x);
        } else {
          // Not dragging playhead — pan the timeline
          widget.onPan(details.delta.dx);
        }
      },
      onHorizontalDragEnd: (details) {
        if (_draggingPlayhead) {
          _draggingPlayhead = false;
          widget.onDragStateChanged(false);
          widget.onDragXChanged(null);
          // Final seek to drag position
          final x = details.localPosition.dx.clamp(0, widget.viewport.widthPx);
          final time = widget.viewport.pixelToTime(x);
          widget.onSeek(time);
        }
      },
      onScaleStart: (_) {},
      onScaleUpdate: (details) {
        if (details.pointerCount >= 2) {
          widget.onZoom(details.scale);
        }
      },
      child: const SizedBox.expand(),
    );
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add lib/screens/playback/timeline/interaction_layer.dart
git commit -m "feat: add InteractionLayer for tap, long-press, pinch, and pan"
```

---

## Task 9: Create MiniOverviewBar

**Files:**
- Create: `lib/screens/playback/timeline/mini_overview_bar.dart`

- [ ] **Step 1: Implement MiniOverviewBar**

Create `lib/screens/playback/timeline/mini_overview_bar.dart`:

```dart
import 'package:flutter/material.dart';
import '../../../models/recording.dart';
import '../../../theme/nvr_colors.dart';
import 'timeline_viewport.dart';

class MiniOverviewBar extends StatelessWidget {
  final TimelineViewport mainViewport;
  final List<RecordingSegment> segments;
  final List<MotionEvent> events;
  final DateTime dayStart;
  final Duration position;
  final ValueChanged<Duration> onViewportJump;

  const MiniOverviewBar({
    super.key,
    required this.mainViewport,
    required this.segments,
    required this.events,
    required this.dayStart,
    required this.position,
    required this.onViewportJump,
  });

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 32,
      child: LayoutBuilder(
        builder: (context, constraints) {
          final fullDayVp = TimelineViewport(
            visibleStart: Duration.zero,
            visibleEnd: const Duration(hours: 24),
            widthPx: constraints.maxWidth,
          );

          return GestureDetector(
            onTapUp: (details) {
              final time = fullDayVp.pixelToTime(details.localPosition.dx);
              onViewportJump(time);
            },
            onHorizontalDragUpdate: (details) {
              final time = fullDayVp.pixelToTime(details.localPosition.dx);
              onViewportJump(time);
            },
            child: CustomPaint(
              size: Size(constraints.maxWidth, 32),
              painter: _MiniOverviewPainter(
                viewport: fullDayVp,
                mainViewport: mainViewport,
                segments: segments,
                events: events,
                dayStart: dayStart,
                position: position,
              ),
            ),
          );
        },
      ),
    );
  }
}

class _MiniOverviewPainter extends CustomPainter {
  final TimelineViewport viewport;
  final TimelineViewport mainViewport;
  final List<RecordingSegment> segments;
  final List<MotionEvent> events;
  final DateTime dayStart;
  final Duration position;

  _MiniOverviewPainter({
    required this.viewport,
    required this.mainViewport,
    required this.segments,
    required this.events,
    required this.dayStart,
    required this.position,
  });

  @override
  void paint(Canvas canvas, Size size) {
    // Background
    canvas.drawRect(
      Rect.fromLTWH(0, 0, size.width, size.height),
      Paint()..color = NvrColors.bgTertiary.withOpacity(0.5),
    );

    // Recording segments as thin bars
    final segPaint = Paint()..color = NvrColors.accent.withOpacity(0.4);
    for (final seg in segments) {
      final x1 = viewport.timeToPixel(seg.startTime.difference(dayStart));
      final x2 = viewport.timeToPixel(seg.endTime.difference(dayStart));
      canvas.drawRect(
        Rect.fromLTRB(x1, 4, x2, size.height - 4),
        segPaint,
      );
    }

    // Event dots (tiny)
    for (final event in events) {
      final x = viewport.timeToPixel(event.startTime.difference(dayStart));
      canvas.drawCircle(
        Offset(x, size.height / 2),
        2,
        Paint()..color = Colors.amber.withOpacity(0.7),
      );
    }

    // Visible range highlight
    final rangeX1 = viewport.timeToPixel(mainViewport.visibleStart);
    final rangeX2 = viewport.timeToPixel(mainViewport.visibleEnd);
    canvas.drawRect(
      Rect.fromLTRB(rangeX1, 0, rangeX2, size.height),
      Paint()
        ..color = NvrColors.accent.withOpacity(0.15)
        ..style = PaintingStyle.fill,
    );
    canvas.drawRect(
      Rect.fromLTRB(rangeX1, 0, rangeX2, size.height),
      Paint()
        ..color = NvrColors.accent.withOpacity(0.6)
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1,
    );

    // Playhead position
    final px = viewport.timeToPixel(position);
    canvas.drawLine(
      Offset(px, 0),
      Offset(px, size.height),
      Paint()
        ..color = NvrColors.accent
        ..strokeWidth = 1.5,
    );
  }

  @override
  bool shouldRepaint(_MiniOverviewPainter old) => true; // small widget, always repaint
}
```

- [ ] **Step 2: Commit**

```bash
git add lib/screens/playback/timeline/mini_overview_bar.dart
git commit -m "feat: add MiniOverviewBar showing full 24h with viewport indicator"
```

---

## Task 10: Create ComposableTimeline

**Files:**
- Create: `lib/screens/playback/timeline/composable_timeline.dart`

Assembles all layers into a stack and manages viewport state.

- [ ] **Step 1: Implement ComposableTimeline**

Create `lib/screens/playback/timeline/composable_timeline.dart`:

```dart
import 'package:flutter/material.dart';
import '../../../models/recording.dart';
import '../../../theme/nvr_colors.dart';
import '../event_detail_popup.dart';
import 'event_layer.dart';
import 'grid_layer.dart';
import 'interaction_layer.dart';
import 'mini_overview_bar.dart';
import 'playhead_layer.dart';
import 'recording_layer.dart';
import 'timeline_viewport.dart';

class ComposableTimeline extends StatefulWidget {
  final List<RecordingSegment> segments;
  final List<MotionEvent> events;
  final DateTime selectedDate;
  final Duration position;
  final ValueChanged<Duration> onSeek;
  final bool isLoading;

  const ComposableTimeline({
    super.key,
    required this.segments,
    required this.events,
    required this.selectedDate,
    required this.position,
    required this.onSeek,
    this.isLoading = false,
  });

  @override
  State<ComposableTimeline> createState() => _ComposableTimelineState();
}

class _ComposableTimelineState extends State<ComposableTimeline> {
  Duration _visibleStart = Duration.zero;
  Duration _visibleEnd = const Duration(hours: 24);
  double _lastScale = 1.0;
  bool _draggingPlayhead = false;
  double? _dragX;

  DateTime get _dayStart => DateTime(
      widget.selectedDate.year, widget.selectedDate.month, widget.selectedDate.day);

  void _handleZoom(double scale) {
    setState(() {
      final center = _visibleStart +
          Duration(
              milliseconds:
                  ((_visibleEnd - _visibleStart).inMilliseconds / 2).round());
      final factor = scale / _lastScale;
      _lastScale = scale;

      final vp = TimelineViewport(
        visibleStart: _visibleStart,
        visibleEnd: _visibleEnd,
        widthPx: 1, // widthPx doesn't affect zoom calculation
      ).zoom(factor, center);

      _visibleStart = vp.visibleStart;
      _visibleEnd = vp.visibleEnd;
    });
  }

  void _handlePan(double deltaPx) {
    setState(() {
      final vp = TimelineViewport(
        visibleStart: _visibleStart,
        visibleEnd: _visibleEnd,
        widthPx: context.size?.width ?? 800,
      ).pan(deltaPx);

      _visibleStart = vp.visibleStart;
      _visibleEnd = vp.visibleEnd;
    });
  }

  void _handleViewportJump(Duration centerTime) {
    setState(() {
      final halfVisible =
          Duration(milliseconds: (_visibleEnd - _visibleStart).inMilliseconds ~/ 2);
      var newStart = centerTime - halfVisible;
      var newEnd = centerTime + halfVisible;

      if (newStart.isNegative) {
        newStart = Duration.zero;
        newEnd = _visibleEnd - _visibleStart;
      }
      if (newEnd > const Duration(hours: 24)) {
        newEnd = const Duration(hours: 24);
        newStart = newEnd - (_visibleEnd - _visibleStart);
      }

      _visibleStart = newStart;
      _visibleEnd = newEnd;
    });
  }

  void _handleEventLongPress(MotionEvent event, Offset globalPosition) {
    showEventDetailPopup(
      context: context,
      event: event,
      position: globalPosition,
      onPlayFromHere: () {
        final dur = event.startTime.difference(_dayStart);
        widget.onSeek(dur);
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        // Mini overview bar
        LayoutBuilder(
          builder: (context, constraints) {
            final mainVp = TimelineViewport(
              visibleStart: _visibleStart,
              visibleEnd: _visibleEnd,
              widthPx: constraints.maxWidth,
            );
            return MiniOverviewBar(
              mainViewport: mainVp,
              segments: widget.segments,
              events: widget.events,
              dayStart: _dayStart,
              position: widget.position,
              onViewportJump: _handleViewportJump,
            );
          },
        ),
        // Main timeline
        Expanded(
          child: LayoutBuilder(
            builder: (context, constraints) {
              final vp = TimelineViewport(
                visibleStart: _visibleStart,
                visibleEnd: _visibleEnd,
                widthPx: constraints.maxWidth,
              );

              return Container(
                color: NvrColors.bgSecondary,
                child: Stack(
                  children: [
                    // Grid
                    Positioned.fill(
                      child: CustomPaint(
                        painter: GridLayer(viewport: vp),
                      ),
                    ),
                    // Recording segments
                    if (!widget.isLoading)
                      Positioned.fill(
                        child: CustomPaint(
                          painter: RecordingLayer(
                            viewport: vp,
                            segments: widget.segments,
                            dayStart: _dayStart,
                          ),
                        ),
                      ),
                    // Events
                    if (!widget.isLoading)
                      Positioned.fill(
                        child: CustomPaint(
                          painter: EventLayer(
                            viewport: vp,
                            events: widget.events,
                            dayStart: _dayStart,
                          ),
                        ),
                      ),
                    // Loading shimmer
                    if (widget.isLoading)
                      Positioned.fill(
                        child: _ShimmerOverlay(),
                      ),
                    // Playhead (visual only — no gestures)
                    Positioned.fill(
                      child: CustomPaint(
                        painter: PlayheadLayer(
                          viewport: vp,
                          position: widget.position,
                          isDragging: _draggingPlayhead,
                          dragX: _dragX,
                        ),
                      ),
                    ),
                    // Interaction (ALL gestures: tap, drag, long-press, zoom, pan)
                    Positioned.fill(
                      child: InteractionLayer(
                        viewport: vp,
                        position: widget.position,
                        events: widget.events,
                        dayStart: _dayStart,
                        onSeek: widget.onSeek,
                        onEventLongPress: _handleEventLongPress,
                        onZoom: _handleZoom,
                        onPan: _handlePan,
                        onDragStateChanged: (dragging) =>
                            setState(() => _draggingPlayhead = dragging),
                        onDragXChanged: (x) =>
                            setState(() => _dragX = x),
                      ),
                    ),
                    // Time label at playhead
                    Positioned(
                      left: vp.timeToPixel(widget.position) + 8,
                      top: 4,
                      child: Container(
                        padding: const EdgeInsets.symmetric(
                            horizontal: 6, vertical: 2),
                        decoration: BoxDecoration(
                          color: NvrColors.bgTertiary,
                          borderRadius: BorderRadius.circular(4),
                        ),
                        child: Text(
                          _formatPosition(widget.position),
                          style: const TextStyle(
                            color: NvrColors.textPrimary,
                            fontSize: 11,
                            fontWeight: FontWeight.w500,
                          ),
                        ),
                      ),
                    ),
                  ],
                ),
              );
            },
          ),
        ),
      ],
    );
  }

  String _formatPosition(Duration d) {
    final h = d.inHours.toString().padLeft(2, '0');
    final m = (d.inMinutes % 60).toString().padLeft(2, '0');
    final s = (d.inSeconds % 60).toString().padLeft(2, '0');
    return '$h:$m:$s';
  }
}

class _ShimmerOverlay extends StatefulWidget {
  @override
  State<_ShimmerOverlay> createState() => _ShimmerOverlayState();
}

class _ShimmerOverlayState extends State<_ShimmerOverlay>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller;

  @override
  void initState() {
    super.initState();
    _controller = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 1500),
    )..repeat();
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: _controller,
      builder: (context, _) {
        return CustomPaint(
          painter: _ShimmerPainter(_controller.value),
        );
      },
    );
  }
}

class _ShimmerPainter extends CustomPainter {
  final double progress;

  _ShimmerPainter(this.progress);

  @override
  void paint(Canvas canvas, Size size) {
    final shimmerX = (progress * (size.width + 200)) - 100;
    final gradient = LinearGradient(
      colors: [
        Colors.transparent,
        NvrColors.accent.withOpacity(0.08),
        Colors.transparent,
      ],
      stops: const [0.0, 0.5, 1.0],
    );

    final rect = Rect.fromLTWH(shimmerX - 100, 0, 200, size.height);
    canvas.drawRect(
      rect,
      Paint()..shader = gradient.createShader(rect),
    );
  }

  @override
  bool shouldRepaint(_ShimmerPainter old) => old.progress != progress;
}
```

**Note:** The `AnimatedBuilder` on line should be `AnimatedBuilder` — fix to use `AnimatedBuilder` (it's the correct widget name in Flutter).

- [ ] **Step 2: Verify it compiles**

Run: `cd clients/flutter && flutter analyze lib/screens/playback/timeline/composable_timeline.dart`
Fix any import or compile errors.

- [ ] **Step 3: Commit**

```bash
git add lib/screens/playback/timeline/composable_timeline.dart
git commit -m "feat: add ComposableTimeline assembling all timeline layers"
```

---

## Task 11: Create TransportControls

**Files:**
- Create: `lib/screens/playback/controls/transport_controls.dart`

- [ ] **Step 1: Implement TransportControls**

Create `lib/screens/playback/controls/transport_controls.dart`:

```dart
import 'package:flutter/material.dart';
import '../../../theme/nvr_colors.dart';

class TransportControls extends StatelessWidget {
  final bool isPlaying;
  final VoidCallback onPlayPause;
  final VoidCallback onStepForward;
  final VoidCallback onStepBackward;
  final VoidCallback onNextEvent;
  final VoidCallback onPreviousEvent;
  final VoidCallback onNextGap;
  final VoidCallback onPreviousGap;

  const TransportControls({
    super.key,
    required this.isPlaying,
    required this.onPlayPause,
    required this.onStepForward,
    required this.onStepBackward,
    required this.onNextEvent,
    required this.onPreviousEvent,
    required this.onNextGap,
    required this.onPreviousGap,
  });

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        _TransportButton(
          icon: Icons.skip_previous,
          tooltip: 'Previous recording',
          onPressed: onPreviousGap,
        ),
        _TransportButton(
          icon: Icons.arrow_back,
          tooltip: 'Previous event',
          onPressed: onPreviousEvent,
        ),
        _TransportButton(
          icon: Icons.chevron_left,
          tooltip: 'Frame back',
          onPressed: onStepBackward,
        ),
        _TransportButton(
          icon: isPlaying ? Icons.pause : Icons.play_arrow,
          tooltip: isPlaying ? 'Pause' : 'Play',
          onPressed: onPlayPause,
          size: 40,
          iconSize: 28,
        ),
        _TransportButton(
          icon: Icons.chevron_right,
          tooltip: 'Frame forward',
          onPressed: onStepForward,
        ),
        _TransportButton(
          icon: Icons.arrow_forward,
          tooltip: 'Next event',
          onPressed: onNextEvent,
        ),
        _TransportButton(
          icon: Icons.skip_next,
          tooltip: 'Next recording',
          onPressed: onNextGap,
        ),
      ],
    );
  }
}

class _TransportButton extends StatelessWidget {
  final IconData icon;
  final String tooltip;
  final VoidCallback onPressed;
  final double size;
  final double iconSize;

  const _TransportButton({
    required this.icon,
    required this.tooltip,
    required this.onPressed,
    this.size = 36,
    this.iconSize = 20,
  });

  @override
  Widget build(BuildContext context) {
    return Tooltip(
      message: tooltip,
      child: SizedBox(
        width: size,
        height: size,
        child: IconButton(
          padding: EdgeInsets.zero,
          icon: Icon(icon, size: iconSize, color: NvrColors.textPrimary),
          onPressed: onPressed,
        ),
      ),
    );
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add lib/screens/playback/controls/transport_controls.dart
git commit -m "feat: add TransportControls with frame step, event skip, gap skip"
```

---

## Task 12: Create JogSlider

**Files:**
- Create: `lib/screens/playback/controls/jog_slider.dart`

- [ ] **Step 1: Implement JogSlider**

Create `lib/screens/playback/controls/jog_slider.dart`:

```dart
import 'dart:async';
import 'package:flutter/material.dart';
import '../../../theme/nvr_colors.dart';

class JogSlider extends StatefulWidget {
  final ValueChanged<double> onSpeedChange; // -2.0 to +2.0
  final VoidCallback onRelease; // snap back to center

  const JogSlider({
    super.key,
    required this.onSpeedChange,
    required this.onRelease,
  });

  @override
  State<JogSlider> createState() => _JogSliderState();
}

class _JogSliderState extends State<JogSlider>
    with SingleTickerProviderStateMixin {
  double _value = 0.0;
  double _valueAtRelease = 0.0;
  bool _isDragging = false;
  late final AnimationController _springController;
  Timer? _jogTimer;

  @override
  void initState() {
    super.initState();
    _springController = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 200),
    );
    _springController.addListener(() {
      setState(() {
        _value = _valueAtRelease * (1 - _springController.value);
      });
    });
  }

  @override
  void dispose() {
    _jogTimer?.cancel();
    _springController.dispose();
    super.dispose();
  }

  void _startJog() {
    _jogTimer?.cancel();
    _jogTimer = Timer.periodic(const Duration(milliseconds: 100), (_) {
      if (_value.abs() > 0.05) {
        widget.onSpeedChange(_value);
      }
    });
  }

  void _stopJog() {
    _jogTimer?.cancel();
    _jogTimer = null;
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        // Speed label
        Text(
          _value.abs() < 0.05
              ? '0x'
              : '${_value > 0 ? '+' : ''}${_value.toStringAsFixed(1)}x',
          style: const TextStyle(
            color: NvrColors.textSecondary,
            fontSize: 11,
          ),
        ),
        const SizedBox(height: 2),
        // Slider
        SizedBox(
          height: 32,
          child: SliderTheme(
            data: SliderThemeData(
              trackHeight: 4,
              thumbShape: const RoundSliderThumbShape(enabledThumbRadius: 8),
              activeTrackColor: NvrColors.accent,
              inactiveTrackColor: NvrColors.bgTertiary,
              thumbColor: NvrColors.accent,
              overlayColor: NvrColors.accent.withOpacity(0.2),
            ),
            child: Slider(
              value: _value,
              min: -2.0,
              max: 2.0,
              onChangeStart: (_) {
                _isDragging = true;
                _springController.reset();
                _startJog();
              },
              onChanged: (v) {
                setState(() => _value = v);
              },
              onChangeEnd: (_) {
                _isDragging = false;
                _valueAtRelease = _value;
                _stopJog();
                widget.onRelease();
                _springController.forward(from: 0);
              },
            ),
          ),
        ),
      ],
    );
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add lib/screens/playback/controls/jog_slider.dart
git commit -m "feat: add JogSlider for variable-speed scrubbing with spring-back"
```

---

## Task 13: Create EventDetailPopup

**Files:**
- Create: `lib/screens/playback/event_detail_popup.dart`

- [ ] **Step 1: Implement EventDetailPopup**

Create `lib/screens/playback/event_detail_popup.dart`:

```dart
import 'package:flutter/material.dart';
import '../../models/recording.dart';
import '../../theme/nvr_colors.dart';
import 'timeline/event_layer.dart';

void showEventDetailPopup({
  required BuildContext context,
  required MotionEvent event,
  required Offset position,
  required VoidCallback onPlayFromHere,
}) {
  final overlay = Overlay.of(context);
  late OverlayEntry entry;

  entry = OverlayEntry(
    builder: (context) => _EventDetailOverlay(
      event: event,
      position: position,
      onPlayFromHere: () {
        entry.remove();
        onPlayFromHere();
      },
      onDismiss: () => entry.remove(),
    ),
  );

  overlay.insert(entry);
}

class _EventDetailOverlay extends StatelessWidget {
  final MotionEvent event;
  final Offset position;
  final VoidCallback onPlayFromHere;
  final VoidCallback onDismiss;

  const _EventDetailOverlay({
    required this.event,
    required this.position,
    required this.onPlayFromHere,
    required this.onDismiss,
  });

  String _formatTime(DateTime? t) {
    if (t == null) return '--:--:--';
    return '${t.hour.toString().padLeft(2, '0')}:'
        '${t.minute.toString().padLeft(2, '0')}:'
        '${t.second.toString().padLeft(2, '0')}';
  }

  String _formatDuration(DateTime start, DateTime? end) {
    if (end == null) return 'ongoing';
    final d = end.difference(start);
    if (d.inMinutes > 0) return '${d.inMinutes}m ${d.inSeconds % 60}s';
    return '${d.inSeconds}s';
  }

  String _classLabel(String? objectClass, String? eventType) {
    if (objectClass != null && objectClass.isNotEmpty) {
      return '${objectClass[0].toUpperCase()}${objectClass.substring(1)} detected';
    }
    if (eventType != null) return eventType;
    return 'Event';
  }

  @override
  Widget build(BuildContext context) {
    final color = EventLayer.colorForClass(event.objectClass);

    return Stack(
      children: [
        // Dismiss on tap outside
        Positioned.fill(
          child: GestureDetector(
            onTap: onDismiss,
            child: const ColoredBox(color: Colors.transparent),
          ),
        ),
        // Popup card
        Positioned(
          left: (position.dx - 140).clamp(8, MediaQuery.of(context).size.width - 288),
          top: (position.dy - 200).clamp(8, MediaQuery.of(context).size.height - 220),
          child: Material(
            color: Colors.transparent,
            child: Container(
              width: 280,
              padding: const EdgeInsets.all(12),
              decoration: BoxDecoration(
                color: NvrColors.bgSecondary,
                borderRadius: BorderRadius.circular(8),
                border: Border.all(color: NvrColors.border),
                boxShadow: [
                  BoxShadow(
                    color: Colors.black.withOpacity(0.4),
                    blurRadius: 12,
                    offset: const Offset(0, 4),
                  ),
                ],
              ),
              child: Column(
                mainAxisSize: MainAxisSize.min,
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  // Thumbnail
                  if (event.thumbnailPath != null)
                    ClipRRect(
                      borderRadius: BorderRadius.circular(4),
                      child: Image.network(
                        event.thumbnailPath!,
                        width: 256,
                        height: 80,
                        fit: BoxFit.cover,
                        errorBuilder: (_, __, ___) => Container(
                          width: 256,
                          height: 80,
                          color: NvrColors.bgTertiary,
                          child: const Icon(Icons.image_not_supported,
                              color: NvrColors.textMuted),
                        ),
                      ),
                    )
                  else
                    Container(
                      width: 256,
                      height: 80,
                      decoration: BoxDecoration(
                        color: NvrColors.bgTertiary,
                        borderRadius: BorderRadius.circular(4),
                      ),
                      child: const Icon(Icons.videocam,
                          color: NvrColors.textMuted, size: 32),
                    ),
                  const SizedBox(height: 8),
                  // Event type with color indicator
                  Row(
                    children: [
                      Container(
                        width: 8,
                        height: 8,
                        decoration: BoxDecoration(
                          color: color,
                          shape: BoxShape.circle,
                        ),
                      ),
                      const SizedBox(width: 8),
                      Text(
                        _classLabel(event.objectClass, event.eventType),
                        style: const TextStyle(
                          color: NvrColors.textPrimary,
                          fontSize: 14,
                          fontWeight: FontWeight.w600,
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 4),
                  // Confidence
                  if (event.confidence != null)
                    Text(
                      '${event.objectClass ?? 'unknown'} (${(event.confidence! * 100).toStringAsFixed(0)}%)',
                      style: const TextStyle(
                          color: NvrColors.textSecondary, fontSize: 12),
                    ),
                  const SizedBox(height: 4),
                  // Time range
                  Text(
                    '${_formatTime(event.startTime)} – ${_formatTime(event.endTime)}  (${_formatDuration(event.startTime, event.endTime)})',
                    style: const TextStyle(
                        color: NvrColors.textSecondary, fontSize: 12),
                  ),
                  const SizedBox(height: 8),
                  // Play from here button
                  SizedBox(
                    width: double.infinity,
                    child: ElevatedButton.icon(
                      onPressed: onPlayFromHere,
                      icon: const Icon(Icons.play_arrow, size: 16),
                      label: const Text('Play from here'),
                      style: ElevatedButton.styleFrom(
                        backgroundColor: NvrColors.accent,
                        foregroundColor: Colors.white,
                        padding: const EdgeInsets.symmetric(vertical: 8),
                        textStyle: const TextStyle(fontSize: 12),
                      ),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ],
    );
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add lib/screens/playback/event_detail_popup.dart
git commit -m "feat: add EventDetailPopup with thumbnail, event info, and play-from-here"
```

---

## Task 14: Rewire PlaybackScreen and CameraPlayer

**Files:**
- Modify: `lib/screens/playback/playback_screen.dart` (full rewrite)
- Modify: `lib/screens/playback/camera_player.dart`
- Delete: `lib/screens/playback/timeline_widget.dart`
- Delete: `lib/screens/playback/playback_controls.dart`

This is the integration task — connects everything together.

- [ ] **Step 1: Rewrite CameraPlayer to use PlaybackController**

Replace `lib/screens/playback/camera_player.dart` with:

```dart
import 'package:flutter/material.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';

import '../../theme/nvr_colors.dart';

class CameraPlayer extends StatelessWidget {
  final VideoController videoController;
  final String cameraName;

  const CameraPlayer({
    super.key,
    required this.videoController,
    required this.cameraName,
  });

  @override
  Widget build(BuildContext context) {
    return Stack(
      fit: StackFit.expand,
      children: [
        ColoredBox(
          color: Colors.black,
          child: Video(controller: videoController),
        ),
        // Camera name label
        Positioned(
          top: 8,
          left: 8,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
            decoration: BoxDecoration(
              color: Colors.black54,
              borderRadius: BorderRadius.circular(4),
            ),
            child: Text(
              cameraName,
              style: const TextStyle(
                color: NvrColors.textPrimary,
                fontSize: 12,
                fontWeight: FontWeight.w500,
              ),
            ),
          ),
        ),
      ],
    );
  }
}
```

- [ ] **Step 2: Rewrite PlaybackScreen**

Replace `lib/screens/playback/playback_screen.dart` with the new version that uses PlaybackController, ComposableTimeline, TransportControls, and JogSlider. Key changes:

- Create `PlaybackController` in `initState`, dispose in `dispose`
- Pass controller to all child widgets
- Replace `_WideLayout` and `_NarrowLayout` with a single layout: video grid on top, then timeline + controls below
- Remove all local state (`_playing`, `_speed`, `_position`) — controller owns this now
- Listen to `recordingSegmentsProvider` and `motionEventsProvider` and pass data to controller
- Video grid uses `controller.videoControllers` map instead of creating players per-widget
- Remove `ValueKey` with position (was causing player recreation)

```dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/camera.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/auth_provider.dart';
import '../../providers/recordings_provider.dart';
import '../../services/playback_service.dart';
import '../../theme/nvr_colors.dart';
import 'camera_player.dart';
import 'controls/jog_slider.dart';
import 'controls/transport_controls.dart';
import 'playback_controller.dart';
import 'timeline/composable_timeline.dart';

class PlaybackScreen extends ConsumerStatefulWidget {
  const PlaybackScreen({super.key});

  @override
  ConsumerState<PlaybackScreen> createState() => _PlaybackScreenState();
}

class _PlaybackScreenState extends ConsumerState<PlaybackScreen> {
  late PlaybackController _controller;
  DateTime _selectedDate = DateTime.now();
  final Set<String> _selectedCameraIds = {};

  static const _speeds = [0.5, 1.0, 1.5, 2.0, 4.0, 8.0];
  double _playbackSpeed = 1.0;

  String get _dateKey =>
      '${_selectedDate.year}-${_selectedDate.month.toString().padLeft(2, '0')}-${_selectedDate.day.toString().padLeft(2, '0')}';

  @override
  void initState() {
    super.initState();
    _controller = PlaybackController(
      playbackService: PlaybackService(serverUrl: ''),
    );
    _controller.addListener(_onControllerChanged);
  }

  void _onControllerChanged() {
    if (mounted) setState(() {});
  }

  @override
  void dispose() {
    _controller.removeListener(_onControllerChanged);
    _controller.dispose();
    super.dispose();
  }

  void _updateControllerServerUrl(String serverUrl) {
    if (_controller.playbackService.serverUrl != serverUrl) {
      _controller.removeListener(_onControllerChanged);
      _controller.dispose();
      _controller = PlaybackController(
        playbackService: PlaybackService(serverUrl: serverUrl),
      );
      _controller.addListener(_onControllerChanged);
    }
  }

  @override
  Widget build(BuildContext context) {
    final camerasAsync = ref.watch(camerasProvider);
    final auth = ref.watch(authProvider);
    final serverUrl = auth.serverUrl ?? '';

    // Update server URL when auth changes
    _updateControllerServerUrl(serverUrl);

    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      appBar: AppBar(
        backgroundColor: NvrColors.bgSecondary,
        title: const Text('Playback',
            style: TextStyle(color: NvrColors.textPrimary)),
        actions: [
          _DatePickerButton(
            date: _selectedDate,
            onChanged: (d) {
              setState(() => _selectedDate = d);
              _controller.setSelectedDate(d);
            },
          ),
        ],
      ),
      body: camerasAsync.when(
        loading: () => const Center(
          child: CircularProgressIndicator(color: NvrColors.accent),
        ),
        error: (e, _) => Center(
          child: Text('Error: $e',
              style: const TextStyle(color: NvrColors.danger)),
        ),
        data: (cameras) => _buildBody(cameras),
      ),
    );
  }

  Widget _buildBody(List<Camera> cameras) {
    if (cameras.isEmpty) {
      return const Center(
        child: Text('No cameras configured.',
            style: TextStyle(color: NvrColors.textMuted)),
      );
    }

    // Default select first camera
    if (_selectedCameraIds.isEmpty && cameras.isNotEmpty) {
      _selectedCameraIds.add(cameras.first.id);
    }

    // Update controller with camera paths and selection
    final pathMap = {for (final c in cameras) c.id: c.mediamtxPath};
    _controller.setCameraPaths(pathMap);
    _controller.setSelectedCameraIds(_selectedCameraIds.toList());

    final selected =
        cameras.where((c) => _selectedCameraIds.contains(c.id)).toList();

    // Fetch and merge recordings/events for all selected cameras
    final allSegments = <RecordingSegment>[];
    final allEvents = <MotionEvent>[];
    bool isLoading = false;

    for (final cam in selected) {
      final key = (cameraId: cam.id, date: _dateKey);
      final segAsync = ref.watch(recordingSegmentsProvider(key));
      final evtAsync = ref.watch(motionEventsProvider(key));

      if (segAsync.isLoading || evtAsync.isLoading) isLoading = true;
      allSegments.addAll(segAsync.valueOrNull ?? []);
      allEvents.addAll(evtAsync.valueOrNull ?? []);
    }

    // Sort merged lists by time
    allSegments.sort((a, b) => a.startTime.compareTo(b.startTime));
    allEvents.sort((a, b) => a.startTime.compareTo(b.startTime));

    // Pass data to controller
    _controller.setSegments(allSegments);
    _controller.setEvents(allEvents);

    final segments = allSegments;
    final events = allEvents;

    return Column(
      children: [
        // Camera selector chips
        _CameraChips(
          cameras: cameras,
          selectedIds: _selectedCameraIds,
          onToggle: (id) => setState(() {
            if (_selectedCameraIds.contains(id)) {
              if (_selectedCameraIds.length > 1) {
                _selectedCameraIds.remove(id);
              }
            } else {
              _selectedCameraIds.add(id);
            }
          }),
        ),
        // Video grid
        Expanded(
          flex: 3,
          child: _VideoGrid(
            cameras: selected,
            controller: _controller,
          ),
        ),
        // Timeline
        Expanded(
          flex: 2,
          child: ComposableTimeline(
            segments: segments,
            events: events,
            selectedDate: _selectedDate,
            position: _controller.position,
            onSeek: (d) => _controller.seek(d),
            isLoading: isLoading,
          ),
        ),
        // Controls
        Container(
          color: NvrColors.bgSecondary,
          padding: const EdgeInsets.symmetric(vertical: 8, horizontal: 12),
          child: Column(
            children: [
              TransportControls(
                isPlaying: _controller.isPlaying,
                onPlayPause: _controller.togglePlayPause,
                onStepForward: () => _controller.stepFrame(1),
                onStepBackward: () => _controller.stepFrame(-1),
                onNextEvent: _controller.skipToNextEvent,
                onPreviousEvent: _controller.skipToPreviousEvent,
                onNextGap: _controller.skipToNextGap,
                onPreviousGap: _controller.skipToPreviousGap,
              ),
              const SizedBox(height: 4),
              Row(
                children: [
                  Expanded(
                    child: JogSlider(
                      onSpeedChange: (speed) {
                        // Jog mode: seek by speed * 200ms every 100ms tick
                        final seekDelta = Duration(
                            milliseconds: (speed * 200).round());
                        _controller.seek(
                            _controller.position + seekDelta);
                      },
                      onRelease: () {
                        // Snap back — no action needed, slider handles animation
                      },
                    ),
                  ),
                  const SizedBox(width: 12),
                  // Speed dropdown
                  DropdownButton<double>(
                    value: _playbackSpeed,
                    dropdownColor: NvrColors.bgSecondary,
                    style: const TextStyle(
                        color: NvrColors.textPrimary, fontSize: 13),
                    underline: const SizedBox.shrink(),
                    items: _speeds
                        .map((s) => DropdownMenuItem(
                              value: s,
                              child: Text('${s}x'),
                            ))
                        .toList(),
                    onChanged: (s) {
                      if (s != null) {
                        setState(() => _playbackSpeed = s);
                        _controller.setSpeed(s);
                      }
                    },
                  ),
                ],
              ),
            ],
          ),
        ),
      ],
    );
  }
}

// ── Camera Chips ──────────────────────────────────────────────────────────────

class _CameraChips extends StatelessWidget {
  final List<Camera> cameras;
  final Set<String> selectedIds;
  final ValueChanged<String> onToggle;

  const _CameraChips({
    required this.cameras,
    required this.selectedIds,
    required this.onToggle,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      color: NvrColors.bgSecondary,
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      child: SizedBox(
        height: 36,
        child: ListView.separated(
          scrollDirection: Axis.horizontal,
          itemCount: cameras.length,
          separatorBuilder: (_, __) => const SizedBox(width: 8),
          itemBuilder: (_, i) {
            final c = cameras[i];
            final selected = selectedIds.contains(c.id);
            return FilterChip(
              label: Text(c.name,
                  style: TextStyle(
                    color: selected ? Colors.white : NvrColors.textSecondary,
                    fontSize: 12,
                  )),
              selected: selected,
              onSelected: (_) => onToggle(c.id),
              backgroundColor: NvrColors.bgTertiary,
              selectedColor: NvrColors.accent,
              checkmarkColor: Colors.white,
              side: BorderSide(
                  color: selected ? NvrColors.accent : NvrColors.border),
              padding: const EdgeInsets.symmetric(horizontal: 4),
            );
          },
        ),
      ),
    );
  }
}

// ── Date Picker Button ────────────────────────────────────────────────────────

class _DatePickerButton extends StatelessWidget {
  final DateTime date;
  final ValueChanged<DateTime> onChanged;

  const _DatePickerButton({required this.date, required this.onChanged});

  @override
  Widget build(BuildContext context) {
    final label =
        '${date.year}-${date.month.toString().padLeft(2, '0')}-${date.day.toString().padLeft(2, '0')}';
    return TextButton.icon(
      onPressed: () async {
        final picked = await showDatePicker(
          context: context,
          initialDate: date,
          firstDate: DateTime(2020),
          lastDate: DateTime.now(),
          builder: (context, child) => Theme(
            data: Theme.of(context).copyWith(
              colorScheme: const ColorScheme.dark(
                primary: NvrColors.accent,
                surface: NvrColors.bgSecondary,
              ),
            ),
            child: child!,
          ),
        );
        if (picked != null) onChanged(picked);
      },
      icon: const Icon(Icons.calendar_today,
          size: 16, color: NvrColors.textSecondary),
      label: Text(label,
          style: const TextStyle(color: NvrColors.textSecondary, fontSize: 13)),
    );
  }
}

// ── Video Grid ────────────────────────────────────────────────────────────────

class _VideoGrid extends StatelessWidget {
  final List<Camera> cameras;
  final PlaybackController controller;

  const _VideoGrid({
    required this.cameras,
    required this.controller,
  });

  @override
  Widget build(BuildContext context) {
    final cols = cameras.length > 1 ? 2 : 1;

    return GridView.builder(
      physics: const NeverScrollableScrollPhysics(),
      gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
        crossAxisCount: cols,
        childAspectRatio: 16 / 9,
        crossAxisSpacing: 4,
        mainAxisSpacing: 4,
      ),
      itemCount: cameras.length,
      itemBuilder: (_, i) {
        final cam = cameras[i];
        final vc = controller.videoControllers[cam.id];
        if (vc == null) {
          return const ColoredBox(
            color: Colors.black,
            child: Center(
              child: CircularProgressIndicator(color: NvrColors.accent),
            ),
          );
        }
        return CameraPlayer(
          key: ValueKey('player-${cam.id}'),
          videoController: vc,
          cameraName: cam.name,
        );
      },
    );
  }
}
```

- [ ] **Step 3: Delete old files**

```bash
rm clients/flutter/lib/screens/playback/timeline_widget.dart
rm clients/flutter/lib/screens/playback/playback_controls.dart
```

- [ ] **Step 4: Verify compilation**

Run: `cd clients/flutter && flutter analyze`
Fix any import errors, type mismatches, or missing references.

- [ ] **Step 5: Commit**

```bash
git add -A lib/screens/playback/
git commit -m "feat: rewire PlaybackScreen with PlaybackController and composable timeline"
```

---

## Task 15: Smoke test and polish

**Files:**
- Various — fix any issues found during testing

- [ ] **Step 1: Run full analysis**

Run: `cd clients/flutter && flutter analyze`
Fix any warnings or errors.

- [ ] **Step 2: Run all tests**

Run: `cd clients/flutter && flutter test`
Ensure all tests pass.

- [ ] **Step 3: Fix any issues found**

Address compile errors, import issues, type mismatches. Common things to watch for:
- Verify `AnimatedBuilder` class name compiles (it's the correct Flutter widget name).
- Verify all imports resolve correctly — especially the 3-level relative imports from `timeline/` subdirectory.
- Check that `RecordingSegment` model import in `recordings_provider.dart` still resolves after changes.
- Verify `import '../models/recording.dart'` is added to `playback_screen.dart` for the `RecordingSegment` type used in sorting.

- [ ] **Step 4: Final commit**

```bash
git add -A
git commit -m "fix: resolve analysis issues and polish playback timeline integration"
```
