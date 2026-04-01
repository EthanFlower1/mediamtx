# Stateless Playback Rewrite

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the buggy WebSocket session playback system with MediaMTX's built-in `/get` playback endpoint + native media_kit player controls — eliminating all race conditions, muxer bugs, and command timing issues.

**Architecture:** MediaMTX already has a production-quality playback server on port 9996 that reads fMP4 recording segments, concatenates them with proper DTS continuity, and streams the result over HTTP. The Flutter client opens this URL in media_kit and uses native player APIs for play/pause/seek/speed. No WebSocket, no session state, no custom server code.

**Tech Stack:** MediaMTX built-in playback server (Go, port 9996), Flutter/Dart (media_kit player, Riverpod).

---

## Key Discovery

MediaMTX's built-in endpoint at `http://host:9996/get?path=CAMERA_PATH&start=RFC3339&duration=SECONDS` already does everything our custom playback session was trying to do:

- Finds recording segments via `recordstore.FindSegments`
- Reads fMP4 headers and concatenates segments with DTS offset handling
- Checks track compatibility across segment boundaries
- Streams proper fMP4 (ftyp+moov+moof+mdat) over HTTP
- Authenticates via MediaMTX auth (our NVR JWT with `"action": "playback"` works)
- Supports both `fmp4` and `mp4` output formats

**Verified working:** `curl -H "Authorization: Bearer $NVR_JWT" "http://localhost:9996/get?path=nvr/ad410&start=...&duration=10"` returns 2.9MB of valid fMP4 data.

---

## Design

### How each control works:

| Control           | Implementation                                                                  |
| ----------------- | ------------------------------------------------------------------------------- |
| **Play**          | `player.open(Media(vodUrl), play: true)` — media_kit plays the HTTP fMP4 stream |
| **Pause**         | `player.pause()` — native, instant, no server call                              |
| **Resume**        | `player.play()` — native, instant                                               |
| **Seek**          | Close player, build new URL with new `start` time, reopen                       |
| **Speed**         | `player.setRate(2.0)` — native, instant, no server call                         |
| **Step**          | Seek ±33ms (1 frame at 30fps)                                                   |
| **Skip to event** | Calculate target time from event list, seek                                     |
| **Jog slider**    | Accumulate delta while dragging, seek on release                                |

### What gets deleted:

All custom NVR playback code (~1500 lines) — session state machine, WebSocket protocol handler, splice muxer, HTTP stream handler, session manager. Plus their tests.

---

## File Structure

### Server (Go) — Delete

| File                                         | Reason                                         |
| -------------------------------------------- | ---------------------------------------------- |
| `internal/nvr/playback/session.go`           | Replaced by MediaMTX built-in                  |
| `internal/nvr/playback/session_test.go`      | Tests for removed code                         |
| `internal/nvr/playback/ws.go`                | WebSocket protocol no longer needed            |
| `internal/nvr/playback/stream.go`            | Session stream handler no longer needed        |
| `internal/nvr/playback/manager.go`           | Session manager no longer needed               |
| `internal/nvr/playback/splice_muxer.go`      | Muxer no longer needed                         |
| `internal/nvr/playback/splice_muxer_test.go` | Tests for removed code                         |
| `internal/nvr/playback/protocol.go`          | WS protocol types no longer needed             |
| `internal/nvr/playback/protocol_test.go`     | Tests for removed code                         |
| `internal/nvr/playback/fmp4_reader.go`       | Replaced by MediaMTX core playback             |
| `internal/nvr/playback/fmp4_reader_test.go`  | Tests for removed code                         |
| `internal/nvr/playback/vod.go`               | VoD handler not needed (use MediaMTX built-in) |
| `internal/nvr/playback/vod_test.go`          | Tests for removed code                         |

### Server (Go) — Modify

| File                         | Action | Change                                                                        |
| ---------------------------- | ------ | ----------------------------------------------------------------------------- |
| `internal/nvr/api/router.go` | Modify | Remove PlaybackManager from RouterConfig, remove WS+stream routes             |
| `internal/nvr/nvr.go`        | Modify | Remove playbackManager field, remove SessionManager/VoDHandler initialization |

### Client (Flutter) — Rewrite

| File                                                   | Action      | Responsibility                                             |
| ------------------------------------------------------ | ----------- | ---------------------------------------------------------- |
| `lib/screens/playback/playback_controller.dart`        | **Rewrite** | Direct media_kit control via MediaMTX `/get` URL           |
| `lib/services/playback_service.dart`                   | **Rewrite** | Build `/get` URL with camera path, start, duration, JWT    |
| `lib/screens/playback/playback_screen.dart`            | **Modify**  | Remove postFrameCallback hacks, simplify controller wiring |
| `lib/screens/playback/controls/jog_slider.dart`        | **Modify**  | Seek-on-release instead of timer flood                     |
| `lib/screens/playback/timeline/interaction_layer.dart` | **Modify**  | Larger drag target                                         |

### Client (Flutter) — Keep Unchanged

| File                                                     | Why                              |
| -------------------------------------------------------- | -------------------------------- |
| `lib/screens/playback/camera_player.dart`                | Renders media_kit Video widget   |
| `lib/screens/playback/controls/transport_controls.dart`  | Button layout unchanged          |
| `lib/screens/playback/timeline/composable_timeline.dart` | Auto-zoom + viewport             |
| `lib/screens/playback/timeline/timeline_viewport.dart`   | Math correct                     |
| `lib/screens/playback/timeline/*.dart`                   | All visual layers unchanged      |
| `lib/models/recording.dart`                              | RecordingSegment model unchanged |
| `lib/providers/recordings_provider.dart`                 | Data fetching unchanged          |

---

## Tasks

### Task 1: Remove custom playback code from server

**Files:**

- Delete: All files in `internal/nvr/playback/`
- Modify: `internal/nvr/api/router.go`
- Modify: `internal/nvr/nvr.go`

- [ ] **Step 1: Remove PlaybackManager from router config**

In `internal/nvr/api/router.go`, remove the `PlaybackManager` field from `RouterConfig` and delete the two route registrations that reference it:

```go
// Remove from RouterConfig:
//   PlaybackManager *playback.SessionManager

// Remove routes:
//   nvr.GET("/playback/stream/:session/:camera", playback.HandleStream(cfg.PlaybackManager))
//   protected.GET("/playback/ws", playback.HandleWebSocket(cfg.PlaybackManager))
```

Also remove the `playback` import.

- [ ] **Step 2: Remove playbackManager from NVR struct and initialization**

In `internal/nvr/nvr.go`:

- Remove `playbackManager` field from the NVR struct
- Remove the `playback.NewSessionManager(...)` call in `Initialize()`
- Remove the `PlaybackManager: n.playbackManager` line in `RegisterRoutes()`
- Remove the `VoDHandler` setup if it exists from earlier work
- Remove the `playback` import
- Keep the `recordPathPattern` logic (used by other code? check first — if only playback used it, remove it too)

- [ ] **Step 3: Delete the entire `internal/nvr/playback/` directory**

```bash
rm -rf internal/nvr/playback/
```

- [ ] **Step 4: Build and verify**

```bash
go build ./...
go test ./internal/nvr/... -v
```

Expected: Clean build, tests pass (playback tests gone, other tests still work).

- [ ] **Step 5: Commit**

```bash
git add -A internal/nvr/
git commit -m "refactor: remove custom playback session system, use MediaMTX built-in playback"
```

---

### Task 2: Rewrite PlaybackService and PlaybackController (Client)

**Files:**

- Rewrite: `clients/flutter/lib/services/playback_service.dart`
- Rewrite: `clients/flutter/lib/screens/playback/playback_controller.dart`

- [ ] **Step 1: Rewrite PlaybackService**

The service now builds a URL for MediaMTX's built-in `/get` endpoint on port 9996:

```dart
// lib/services/playback_service.dart

class PlaybackService {
  final String serverUrl;
  PlaybackService({required this.serverUrl});

  /// Builds the VoD URL pointing to MediaMTX's built-in playback endpoint.
  ///
  /// MediaMTX serves recordings at port 9996:
  ///   GET /get?path=CAMERA_PATH&start=RFC3339&duration=SECONDS
  ///
  /// The NVR JWT token (which includes "action": "playback") authenticates.
  String vodUrl({
    required String cameraPath,
    required DateTime start,
    int durationSecs = 7200,
    String? token,
  }) {
    final uri = Uri.parse(serverUrl);
    final startStr = _toRfc3339(start);
    final base = '${uri.scheme}://${uri.host}:9996/get';
    final params = {
      'path': cameraPath,
      'start': startStr,
      'duration': durationSecs.toString(),
    };
    if (token != null && token.isNotEmpty) {
      params['jwt'] = token;
    }
    return Uri.parse(base).replace(queryParameters: params).toString();
  }

  static String _toRfc3339(DateTime d) {
    final offset = d.timeZoneOffset;
    final sign = offset.isNegative ? '-' : '+';
    final abs = offset.abs();
    final h = abs.inHours.toString().padLeft(2, '0');
    final m = (abs.inMinutes % 60).toString().padLeft(2, '0');
    return '${d.year}-${_p(d.month)}-${_p(d.day)}'
        'T${_p(d.hour)}:${_p(d.minute)}:${_p(d.second)}$sign$h:$m';
  }

  static String _p(int n) => n.toString().padLeft(2, '0');
}
```

- [ ] **Step 2: Rewrite PlaybackController**

Replace the 500-line WebSocket controller with a ~180-line direct media_kit controller:

```dart
// lib/screens/playback/playback_controller.dart

import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';
import '../../models/recording.dart';
import '../../services/playback_service.dart';

class PlaybackController extends ChangeNotifier {
  final PlaybackService playbackService;
  final Future<String?> Function() getAccessToken;

  bool _disposed = false;

  // State
  Duration _position = Duration.zero;
  bool _isPlaying = false;
  double _speed = 1.0;
  bool _isSeeking = false;
  DateTime _selectedDate = DateTime.now();
  List<String> _selectedCameraIds = [];
  List<RecordingSegment> _segments = [];
  List<MotionEvent> _events = [];
  String? _error;

  // Players — one per selected camera
  final Map<String, Player> _players = {};
  final Map<String, VideoController> _videoControllers = {};
  StreamSubscription<Duration>? _positionSub;
  StreamSubscription<bool>? _completedSub;

  // The absolute time-since-midnight that corresponds to player position 0.
  Duration _streamOrigin = Duration.zero;

  // Camera ID → MediaMTX path
  final Map<String, String> _cameraPaths = {};

  // Debounce timer for seek
  Timer? _seekDebounce;

  static const _maxPosition = Duration(hours: 24);

  PlaybackController({
    required this.playbackService,
    required this.getAccessToken,
  });

  // ── Getters ─────────────────────────────────────────────────────────

  Duration get position => _position;
  bool get isPlaying => _isPlaying;
  double get speed => _speed;
  bool get isSeeking => _isSeeking;
  DateTime get selectedDate => _selectedDate;
  List<String> get selectedCameraIds => _selectedCameraIds;
  List<RecordingSegment> get segments => _segments;
  List<MotionEvent> get events => _events;
  Map<String, VideoController> get videoControllers => _videoControllers;
  String? get error => _error;

  // ── Data setters ────────────────────────────────────────────────────

  void setSegments(List<RecordingSegment> s) => _segments = s;
  void setEvents(List<MotionEvent> e) => _events = e;

  void setCameraPaths(Map<String, String> paths) {
    _cameraPaths..clear()..addAll(paths);
  }

  void setSelectedDate(DateTime date) {
    if (_selectedDate.year == date.year &&
        _selectedDate.month == date.month &&
        _selectedDate.day == date.day) return;
    _selectedDate = date;
    _position = Duration.zero;
    _streamOrigin = Duration.zero;
    _disposeAllPlayers();
    notifyListeners();
  }

  void setSelectedCameraIds(List<String> ids) {
    if (_listEquals(ids, _selectedCameraIds)) return;
    _selectedCameraIds = ids;
    _disposeAllPlayers();
    notifyListeners();
  }

  // ── Playback controls ──────────────────────────────────────────────

  Future<void> play() async {
    if (_players.isEmpty) {
      // First play — open streams. If at midnight, jump to first recording.
      var startPos = _position;
      if (startPos == Duration.zero && _segments.isNotEmpty) {
        startPos = _segments.first.startTime.difference(_dayStart);
        _position = startPos;
        _streamOrigin = startPos;
      }
      await _openPlayers(startPos);
    }
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

  Future<void> seek(Duration target) async {
    final clamped = Duration(
      milliseconds: target.inMilliseconds.clamp(0, _maxPosition.inMilliseconds),
    );
    final snapped = _snapToSegment(clamped);

    _isSeeking = true;
    _position = snapped;
    _streamOrigin = snapped;
    notifyListeners();

    // Debounce: if another seek arrives within 150ms, cancel this one.
    _seekDebounce?.cancel();
    _seekDebounce = Timer(const Duration(milliseconds: 150), () async {
      final wasPlaying = _isPlaying;
      _disposeAllPlayers();
      await _openPlayers(snapped);
      if (wasPlaying) {
        for (final p in _players.values) {
          p.play();
        }
        _isPlaying = true;
      }
      _isSeeking = false;
      if (!_disposed) notifyListeners();
    });
  }

  void setSpeed(double s) {
    _speed = s;
    for (final p in _players.values) {
      p.setRate(s);
    }
    notifyListeners();
  }

  void stepFrame(int direction) {
    const frameDur = Duration(milliseconds: 33);
    final target = direction > 0 ? _position + frameDur : _position - frameDur;
    if (_isPlaying) pause();
    seek(target);
  }

  void skipToNextEvent() {
    final t = _findNext(_events, _dayStart, _position, (e) => e.startTime);
    if (t != null) seek(t);
  }

  void skipToPreviousEvent() {
    final t = _findPrev(_events, _dayStart, _position, (e) => e.startTime);
    if (t != null) seek(t);
  }

  void skipToNextGap() {
    final posTime = _dayStart.add(_position);
    for (int i = 0; i < _segments.length - 1; i++) {
      final gapEnd = _segments[i + 1].startTime;
      if (gapEnd.isAfter(posTime) && _segments[i].endTime != gapEnd) {
        seek(gapEnd.difference(_dayStart));
        return;
      }
    }
  }

  void skipToPreviousGap() {
    final posTime = _dayStart.add(_position);
    Duration? result;
    for (int i = 0; i < _segments.length - 1; i++) {
      final gapStart = _segments[i].endTime;
      if (gapStart.isBefore(posTime) && gapStart != _segments[i + 1].startTime) {
        result = gapStart.difference(_dayStart);
      }
    }
    if (result != null) seek(result);
  }

  // ── Player management ──────────────────────────────────────────────

  DateTime get _dayStart => DateTime(
      _selectedDate.year, _selectedDate.month, _selectedDate.day);

  Future<void> _openPlayers(Duration startPosition) async {
    final token = await getAccessToken();
    final startTime = _dayStart.add(startPosition);
    _streamOrigin = startPosition;
    _error = null;

    for (final camId in _selectedCameraIds) {
      final camPath = _cameraPaths[camId];
      if (camPath == null || camPath.isEmpty) continue;

      final url = playbackService.vodUrl(
        cameraPath: camPath,
        start: startTime,
        token: token,
      );

      final player = Player();
      player.setRate(_speed);
      _players[camId] = player;
      _videoControllers[camId] = VideoController(player);

      // Listen for errors
      player.stream.error.listen((error) {
        if (_disposed) return;
        _error = error;
        debugPrint('Playback player error: $error');
        notifyListeners();
      });

      await player.open(Media(url), play: false);
    }

    // Position tracking from first player
    _positionSub?.cancel();
    _completedSub?.cancel();
    if (_players.isNotEmpty) {
      final primary = _players.values.first;

      _positionSub = primary.stream.position.listen((pos) {
        if (_disposed || _isSeeking) return;
        _position = _streamOrigin + pos;
        notifyListeners();
      });

      _completedSub = primary.stream.completed.listen((completed) {
        if (_disposed || !completed) return;
        _isPlaying = false;
        notifyListeners();
      });
    }

    notifyListeners();
  }

  Duration _snapToSegment(Duration position) {
    if (_segments.isEmpty) return position;
    final posTime = _dayStart.add(position);
    for (final seg in _segments) {
      if (!posTime.isBefore(seg.startTime) && posTime.isBefore(seg.endTime)) {
        return position; // inside a segment
      }
    }
    for (final seg in _segments) {
      if (seg.startTime.isAfter(posTime)) {
        return seg.startTime.difference(_dayStart);
      }
    }
    return position;
  }

  // ── Helpers ─────────────────────────────────────────────────────────

  static Duration? _findNext<T>(
      List<T> items, DateTime dayStart, Duration pos, DateTime Function(T) getTime) {
    final posTime = dayStart.add(pos);
    for (final item in items) {
      if (getTime(item).isAfter(posTime)) {
        return getTime(item).difference(dayStart);
      }
    }
    return null;
  }

  static Duration? _findPrev<T>(
      List<T> items, DateTime dayStart, Duration pos, DateTime Function(T) getTime) {
    final posTime = dayStart.add(pos);
    Duration? result;
    for (final item in items) {
      if (getTime(item).isBefore(posTime)) {
        result = getTime(item).difference(dayStart);
      }
    }
    return result;
  }

  static bool _listEquals(List<String> a, List<String> b) {
    if (a.length != b.length) return false;
    for (int i = 0; i < a.length; i++) {
      if (a[i] != b[i]) return false;
    }
    return true;
  }

  // ── Cleanup ─────────────────────────────────────────────────────────

  void _disposeAllPlayers() {
    _positionSub?.cancel();
    _positionSub = null;
    _completedSub?.cancel();
    _completedSub = null;
    for (final p in _players.values) {
      p.dispose();
    }
    _players.clear();
    _videoControllers.clear();
  }

  @override
  void dispose() {
    _disposed = true;
    _seekDebounce?.cancel();
    _disposeAllPlayers();
    super.dispose();
  }
}
```

- [ ] **Step 3: Verify analysis passes**

```bash
cd clients/flutter && dart analyze lib/screens/playback/ lib/services/playback_service.dart
```

Expected: No issues found.

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/screens/playback/playback_controller.dart \
       clients/flutter/lib/services/playback_service.dart
git commit -m "feat(flutter): rewrite playback for stateless VoD via MediaMTX built-in endpoint"
```

---

### Task 3: Update PlaybackScreen to work with new controller (Client)

**Files:**

- Modify: `clients/flutter/lib/screens/playback/playback_screen.dart`

- [ ] **Step 1: Remove postFrameCallback hacks, simplify controller wiring**

The new controller's `setSelectedCameraIds` and `setCameraPaths` only notify when values actually change, so they can be called directly in build without triggering loops.

Replace the `_buildBody` method's camera initialization and controller setup:

```dart
// Replace the postFrameCallback blocks with direct calls:
if (_selectedCameraIds.isEmpty && cameras.isNotEmpty) {
    _selectedCameraIds.add(cameras.first.id);
}

final pathMap = {for (final c in cameras) c.id: c.mediamtxPath};
controller.setCameraPaths(pathMap);
controller.setSelectedCameraIds(_selectedCameraIds.toList());
```

- [ ] **Step 2: Verify analysis passes**

```bash
dart analyze lib/screens/playback/playback_screen.dart
```

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/screens/playback/playback_screen.dart
git commit -m "fix(flutter): simplify PlaybackScreen, remove postFrameCallback workarounds"
```

---

### Task 4: Fix JogSlider — seek on release instead of timer flood (Client)

**Files:**

- Modify: `clients/flutter/lib/screens/playback/controls/jog_slider.dart`
- Modify: `clients/flutter/lib/screens/playback/playback_screen.dart` (wiring)

- [ ] **Step 1: Read the current jog_slider.dart to understand its interface**

- [ ] **Step 2: Rewrite to seek-on-release**

Replace the timer-based implementation with a simple slider that accumulates a delta and seeks on release. The slider springs back to center after release.

The callback changes from `onSpeedChange(double)` to `onSeek(Duration)` taking the absolute target position.

- [ ] **Step 3: Update wiring in playback_screen.dart**

```dart
JogSlider(
  currentPosition: controller.position,
  onSeek: (target) => controller.seek(target),
),
```

- [ ] **Step 4: Verify analysis passes**

```bash
dart analyze lib/screens/playback/
```

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/screens/playback/controls/jog_slider.dart \
       clients/flutter/lib/screens/playback/playback_screen.dart
git commit -m "fix(flutter): rewrite JogSlider to seek-on-release"
```

---

### Task 5: Improve timeline interaction (Client)

**Files:**

- Modify: `clients/flutter/lib/screens/playback/timeline/interaction_layer.dart`

- [ ] **Step 1: Increase playhead drag hit radius from 20px to 40px**

```dart
bool _isNearPlayhead(double px) {
    final playheadX = widget.viewport.timeToPixel(widget.position);
    return (px - playheadX).abs() < 40;
}
```

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/screens/playback/timeline/interaction_layer.dart
git commit -m "fix(flutter): increase playhead drag target for easier scrubbing"
```

---

### Task 6: End-to-end smoke test

- [ ] **Step 1: Rebuild server**

```bash
go build -o ./mediamtx .
go test ./internal/nvr/... -v
```

- [ ] **Step 2: Run Flutter analysis**

```bash
cd clients/flutter && dart analyze lib/
```

- [ ] **Step 3: Start server and test**

```bash
./mediamtx &
```

Wait for recordings to accumulate (or use existing ones), then in the Flutter app verify:

1. [ ] Playback screen loads, timeline shows recordings
2. [ ] Timeline auto-zooms to first recording
3. [ ] Play button starts video from first recording (not midnight)
4. [ ] Pause stops immediately
5. [ ] Tap on timeline seeks to that position
6. [ ] Drag playhead scrubs
7. [ ] JogSlider seeks on release without flooding
8. [ ] Speed dropdown changes rate (1x, 2x, 4x work natively)
9. [ ] Step forward/backward moves one frame
10. [ ] Event skip buttons navigate between events
11. [ ] Seeking into a gap snaps to next recording
12. [ ] Date picker loads different day's recordings
13. [ ] Camera chip selection works

- [ ] **Step 4: Fix any issues found, commit**

---

## What This Eliminates

| Removed                       | Lines     | Bugs it had                                                   |
| ----------------------------- | --------- | ------------------------------------------------------------- |
| PlaybackSession state machine | ~400      | Race conditions, incorrect gap detection, hardcoded timescale |
| WebSocket protocol handler    | ~200      | No command ACK, session disposal race, event buffer overflow  |
| SpliceMuxer                   | ~200      | Send-on-closed-channel panic, DTS desync across tracks        |
| HTTP stream handler           | ~60       | No backpressure, silent write errors                          |
| SessionManager                | ~120      | Returns success with zero cameras                             |
| Protocol types                | ~50       | N/A                                                           |
| fMP4 reader (ours)            | ~400      | Replaced by MediaMTX's battle-tested implementation           |
| **Total removed**             | **~1430** | **12+ critical bugs**                                         |

## What's Added

| Added                    | Lines    | Purpose                                     |
| ------------------------ | -------- | ------------------------------------------- |
| PlaybackController (new) | ~180     | Direct media_kit control, no protocol       |
| PlaybackService (new)    | ~30      | URL builder for `/get` endpoint             |
| **Total added**          | **~210** | **Zero custom protocol, zero server state** |
