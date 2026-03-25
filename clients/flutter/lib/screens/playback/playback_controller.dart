import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';
import '../../models/bookmark.dart';
import '../../models/recording.dart';
import '../../services/playback_service.dart';

class PlaybackController extends ChangeNotifier {
  final PlaybackService playbackService;
  final Future<String?> Function() getAccessToken;

  bool _disposed = false;

  // State
  Duration _position = Duration.zero; // wall-clock time since midnight
  bool _isPlaying = false;
  bool _continuousMode = true;
  double _speed = 1.0;
  bool _isSeeking = false;
  DateTime _nextPositionUpdate = DateTime(2000);
  DateTime _selectedDate = DateTime.now();
  List<String> _selectedCameraIds = [];
  List<RecordingSegment> _segments = [];
  List<MotionEvent> _events = [];
  List<Bookmark> _bookmarks = [];
  String? _error;

  // Players — one per selected camera
  final Map<String, Player> _players = {};
  final Map<String, VideoController> _videoControllers = {};
  StreamSubscription<Duration>? _positionSub;
  StreamSubscription<bool>? _completedSub;

  // Camera ID → MediaMTX path
  final Map<String, String> _cameraPaths = {};

  // The wall-clock DateTime where the current stream starts.
  // Player position 0 = this time. For position display we add the
  // player position, adjusted for recording gaps using _gapMap.
  DateTime _streamStart = DateTime(2000);

  // Gap map for the current stream: list of (playerOffset, wallClockTime)
  // pairs marking where each recording section starts in the player
  // timeline. Built from _segments when a stream is opened.
  List<_GapEntry> _gapMap = [];

  static const _maxPosition = Duration(hours: 24);

  PlaybackController({
    required this.playbackService,
    required this.getAccessToken,
  });

  // ── Getters ─────────────────────────────────────────────────────────

  Duration get position => _position;
  bool get isPlaying => _isPlaying;
  bool get continuousMode => _continuousMode;
  double get speed => _speed;
  bool get isSeeking => _isSeeking;
  DateTime get selectedDate => _selectedDate;
  List<String> get selectedCameraIds => _selectedCameraIds;
  List<RecordingSegment> get segments => _segments;
  List<MotionEvent> get events => _events;
  List<Bookmark> get bookmarks => _bookmarks;
  Map<String, VideoController> get videoControllers => _videoControllers;
  String? get error => _error;

  // ── Data setters ────────────────────────────────────────────────────

  void setSegments(List<RecordingSegment> s) {
    final changed = !_segmentsEqual(_segments, s);
    _segments = s;
    if (changed && _position == Duration.zero && _segments.isNotEmpty) {
      _position = _segments.first.startTime.difference(_dayStart);
    }
  }

  void setEvents(List<MotionEvent> e) => _events = e;

  void setBookmarks(List<Bookmark> b) {
    _bookmarks = b;
    notifyListeners();
  }

  void setContinuousMode(bool enabled) {
    _continuousMode = enabled;
    notifyListeners();
  }

  void setCameraPaths(Map<String, String> paths) {
    _cameraPaths..clear()..addAll(paths);
  }

  void setSelectedDate(DateTime date) {
    if (_selectedDate.year == date.year &&
        _selectedDate.month == date.month &&
        _selectedDate.day == date.day) {
      return;
    }
    _selectedDate = date;
    _position = Duration.zero;
    _disposeAllPlayers();
    notifyListeners();
  }

  void setSelectedCameraIds(List<String> ids) {
    if (_listEquals(ids, _selectedCameraIds)) return;
    _selectedCameraIds = ids;
    _disposeAllPlayers();
    notifyListeners();
  }

  // ── Gap map: player position → wall-clock ─────────────────────────

  /// Build a gap map for a stream starting at [startWallClock].
  ///
  /// The MediaMTX /get endpoint concatenates recordings without gaps,
  /// so player position advances continuously. But wall-clock time jumps
  /// across recording gaps. The gap map tracks where each section starts
  /// in the player timeline so we can convert player position → wall-clock.
  List<_GapEntry> _buildGapMap(
      Duration startWallClock, String cameraId) {
    // Get this camera's segments sorted by start time.
    final camSegments = _segments
        .where((s) => s.cameraId == cameraId)
        .toList()
      ..sort((a, b) => a.startTime.compareTo(b.startTime));

    if (camSegments.isEmpty) return [];

    final dayStart = _dayStart;
    final startTime = dayStart.add(startWallClock);
    final entries = <_GapEntry>[];
    Duration cumulativePlayer = Duration.zero;

    for (final seg in camSegments) {
      // Skip segments that end before our start time.
      if (!seg.endTime.isAfter(startTime)) continue;

      // Determine the wall-clock start of content in this segment.
      DateTime sectionWallClock;
      Duration sectionMediaDuration;

      if (seg.startTime.isBefore(startTime)) {
        // Stream starts mid-segment.
        sectionWallClock = startTime;
        final usedMs = startTime.difference(seg.startTime).inMilliseconds;
        sectionMediaDuration =
            Duration(milliseconds: seg.durationMs - usedMs);
      } else {
        sectionWallClock = seg.startTime;
        sectionMediaDuration = Duration(milliseconds: seg.durationMs);
      }

      entries.add(_GapEntry(
        playerOffset: cumulativePlayer,
        wallClock: sectionWallClock,
      ));

      cumulativePlayer += sectionMediaDuration;
    }

    return entries;
  }

  /// Convert player position to wall-clock time using the gap map.
  Duration _playerToWallClock(Duration playerPos) {
    if (_gapMap.isEmpty) {
      // No gap data — fall back to simple offset from stream start.
      return _streamStart.difference(_dayStart) + playerPos;
    }

    // Find the last gap entry at or before playerPos.
    _GapEntry? active;
    for (final entry in _gapMap) {
      if (entry.playerOffset <= playerPos) {
        active = entry;
      } else {
        break;
      }
    }

    if (active == null) {
      return _streamStart.difference(_dayStart) + playerPos;
    }

    final offsetInSection = playerPos - active.playerOffset;
    final wallClock = active.wallClock.add(offsetInSection);
    return wallClock.difference(_dayStart);
  }

  // ── Playback controls ──────────────────────────────────────────────

  Future<void> play() async {
    if (_players.isEmpty) {
      await _openPlayersAt(_position);
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

  Future<void> seek(Duration wallClockTarget) async {
    final clamped = Duration(
      milliseconds:
          wallClockTarget.inMilliseconds.clamp(0, _maxPosition.inMilliseconds),
    );
    final snapped = _snapToSegment(clamped);

    _isSeeking = true;
    _position = snapped;
    notifyListeners();

    final wasPlaying = _isPlaying;

    try {
      await _openPlayersAt(snapped);

      if (wasPlaying) {
        _isPlaying = true;
        for (final p in _players.values) {
          p.play();
        }
      }
    } catch (e) {
      debugPrint('Playback seek error: $e');
      _error = e.toString();
    } finally {
      _isSeeking = false;
    }

    notifyListeners();
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
    final t = findNextEvent(_events, _dayStart, _position);
    if (t != null) seek(t);
  }

  void skipToPreviousEvent() {
    final t = findPreviousEvent(_events, _dayStart, _position);
    if (t != null) seek(t);
  }

  void skipToNextBookmark() {
    final t = _findNext(_bookmarks, _dayStart, _position, (b) => b.timestamp);
    if (t != null) seek(t);
  }

  void skipToPreviousBookmark() {
    final t = _findPrev(_bookmarks, _dayStart, _position, (b) => b.timestamp);
    if (t != null) seek(t);
  }

  void skipToNextGap() {
    final t = findNextGapEnd(_segments, _dayStart, _position);
    if (t != null) seek(t);
  }

  void skipToPreviousGap() {
    final t = findPreviousGapStart(_segments, _dayStart, _position);
    if (t != null) seek(t);
  }

  // ── Player management ──────────────────────────────────────────────

  DateTime get _dayStart => DateTime(
      _selectedDate.year, _selectedDate.month, _selectedDate.day);

  /// Open (or re-open) players using the MediaMTX /get endpoint starting
  /// at [wallClock] (Duration since midnight).
  ///
  /// The /get endpoint handles time-based seeking, multi-recording
  /// concatenation, and fMP4 muxing server-side. The client just opens
  /// the URL and plays from position 0 = target time.
  Future<void> _openPlayersAt(Duration wallClock) async {
    _disposeAllPlayers();

    final token = await getAccessToken();
    _error = null;

    final startTime = _dayStart.add(wallClock);
    _streamStart = startTime;

    // Duration: from the target time to end of day.
    final remainingSecs =
        const Duration(hours: 24).inSeconds - wallClock.inSeconds;
    final durationSecs = remainingSecs > 0 ? remainingSecs : 86400;

    // Build gap map from the primary camera's segments.
    final primaryCam =
        _selectedCameraIds.isNotEmpty ? _selectedCameraIds.first : null;
    if (primaryCam != null) {
      _gapMap = _buildGapMap(wallClock, primaryCam);
      debugPrint('Gap map: ${_gapMap.length} sections');
    }

    await Future.wait(_selectedCameraIds.map((camId) async {
      final cameraPath = _cameraPaths[camId];
      if (cameraPath == null) {
        debugPrint('No MediaMTX path for camera $camId');
        return;
      }

      final url = playbackService.clipUrl(
        cameraPath: cameraPath,
        start: startTime,
        durationSecs: durationSecs,
        token: token,
      );

      debugPrint('Opening player: camera=$camId, url=$url');

      try {
        final player = Player();
        player.setRate(_speed);
        _players[camId] = player;
        _videoControllers[camId] = VideoController(player);

        player.stream.error.listen((error) {
          if (_disposed) return;
          _error = error;
          debugPrint('Playback player error: $error');
          notifyListeners();
        });

        await player.open(Media(url), play: true);

        // Wait for the stream to load.
        if (player.state.duration == Duration.zero) {
          await player.stream.duration
              .firstWhere((d) => d > Duration.zero)
              .timeout(const Duration(seconds: 10),
                  onTimeout: () => Duration.zero);
        }

        // Pause — the caller decides whether to resume playing.
        player.pause();

        debugPrint(
            'Player ready: camera=$camId, duration=${player.state.duration}');
      } catch (e) {
        debugPrint('Failed to open player for camera $camId: $e');
        _players.remove(camId)?.dispose();
        _videoControllers.remove(camId);
      }
    }));

    if (_players.isEmpty) {
      _error = 'Failed to open any camera for playback';
      notifyListeners();
      return;
    }

    _positionSub?.cancel();
    _completedSub?.cancel();
    final primary = _players.values.first;

    _positionSub = primary.stream.position.listen((playerPos) {
      if (_disposed || _isSeeking) return;

      // Throttle to ~4 updates/sec.
      final now = DateTime.now();
      if (now.isBefore(_nextPositionUpdate)) return;
      _nextPositionUpdate = now.add(const Duration(milliseconds: 250));

      final wallClock = _playerToWallClock(playerPos);

      // Reject spurious backward jumps > 2s.
      if ((_position - wallClock).inSeconds > 2) return;

      _position = wallClock;
      notifyListeners();
    });

    _completedSub = primary.stream.completed.listen((completed) {
      if (_disposed || !completed || _isSeeking) return;
      if (_continuousMode) {
        final nextDay = DateTime(
            _selectedDate.year, _selectedDate.month, _selectedDate.day + 1);
        setSelectedDate(nextDay);
      } else {
        _isPlaying = false;
        notifyListeners();
      }
    });

    notifyListeners();
  }

  Duration _snapToSegment(Duration position) {
    return snapToSegment(_segments, _dayStart, position);
  }

  // ── Public static helpers (testable without a Player) ─────────────

  static RecordingSegment? findContainingSegment(
      List<RecordingSegment> segments, DateTime dayStart, Duration position) {
    final posTime = dayStart.add(position);
    for (final seg in segments) {
      if (!posTime.isBefore(seg.startTime) && !posTime.isAfter(seg.endTime)) {
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
    final posTime = dayStart.add(position);

    for (final seg in segments) {
      if (!posTime.isBefore(seg.startTime) && !posTime.isAfter(seg.endTime)) {
        return position;
      }
    }

    Duration? nearestBefore;
    Duration? nearestAfter;
    for (final seg in segments) {
      if (!seg.endTime.isAfter(posTime)) {
        nearestBefore = seg.endTime.difference(dayStart);
      }
      if (seg.startTime.isAfter(posTime) && nearestAfter == null) {
        nearestAfter = seg.startTime.difference(dayStart);
      }
    }
    if (nearestAfter != null) return nearestAfter;
    if (nearestBefore != null) return nearestBefore;
    return position;
  }

  static Duration? findNextEvent(
      List<MotionEvent> events, DateTime dayStart, Duration pos) {
    return _findNext(events, dayStart, pos, (e) => e.startTime);
  }

  static Duration? findPreviousEvent(
      List<MotionEvent> events, DateTime dayStart, Duration pos) {
    return _findPrev(events, dayStart, pos, (e) => e.startTime);
  }

  static Duration? findNextGapEnd(
      List<RecordingSegment> segments, DateTime dayStart, Duration pos) {
    final posTime = dayStart.add(pos);
    for (int i = 0; i < segments.length - 1; i++) {
      final gapEnd = segments[i + 1].startTime;
      if (gapEnd.isAfter(posTime) && segments[i].endTime != gapEnd) {
        return gapEnd.difference(dayStart);
      }
    }
    return null;
  }

  static Duration? findPreviousGapStart(
      List<RecordingSegment> segments, DateTime dayStart, Duration pos) {
    final posTime = dayStart.add(pos);
    Duration? result;
    for (int i = 0; i < segments.length - 1; i++) {
      final gapStart = segments[i].endTime;
      if (gapStart.isBefore(posTime) && gapStart != segments[i + 1].startTime) {
        result = gapStart.difference(dayStart);
      }
    }
    return result;
  }

  // ── Private helpers ───────────────────────────────────────────────

  static Duration? _findNext<T>(
      List<T> items, DateTime dayStart, Duration pos,
      DateTime Function(T) getTime) {
    final posTime = dayStart.add(pos);
    for (final item in items) {
      if (getTime(item).isAfter(posTime)) {
        return getTime(item).difference(dayStart);
      }
    }
    return null;
  }

  static Duration? _findPrev<T>(
      List<T> items, DateTime dayStart, Duration pos,
      DateTime Function(T) getTime) {
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

  static bool _segmentsEqual(
      List<RecordingSegment> a, List<RecordingSegment> b) {
    if (a.length != b.length) return false;
    for (int i = 0; i < a.length; i++) {
      if (a[i].id != b[i].id) return false;
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
    _disposeAllPlayers();
    super.dispose();
  }
}

/// Marks where a recording section starts in the player timeline.
class _GapEntry {
  final Duration playerOffset;
  final DateTime wallClock;

  const _GapEntry({required this.playerOffset, required this.wallClock});
}
