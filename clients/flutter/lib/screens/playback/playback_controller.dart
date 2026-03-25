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
  Duration _position = Duration.zero; // wall-clock time since midnight
  bool _isPlaying = false;
  double _speed = 1.0;
  bool _isSeeking = false;
  Duration? _lastSeekTarget;
  DateTime _seekDebounceUntil = DateTime(2000);
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

  // Camera ID → MediaMTX path (not used for HLS but kept for API compat)
  final Map<String, String> _cameraPaths = {};

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

  void setSegments(List<RecordingSegment> s) {
    final changed = _segments.length != s.length;
    _segments = s;
    if (changed) {
      _rebuildSegmentIndex();
      // Auto-position cursor at first recording when segments first load
      if (_position == Duration.zero && _segments.isNotEmpty) {
        _position = _segments.first.startTime.difference(_dayStart);
      }
    }
  }

  void setEvents(List<MotionEvent> e) => _events = e;

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
    _segmentIndex.clear();
    notifyListeners();
  }

  void setSelectedCameraIds(List<String> ids) {
    if (_listEquals(ids, _selectedCameraIds)) return;
    _selectedCameraIds = ids;
    _disposeAllPlayers();
    notifyListeners();
  }

  // ── Segment index for wall-clock ↔ player position mapping ─────────

  // Each entry: (wallClockStart, wallClockEnd, cumulativePlayerOffset)
  // wallClockStart/End are Durations since midnight.
  // cumulativePlayerOffset is the player position at the start of this segment.
  final List<_SegmentMapEntry> _segmentIndex = [];

  void _rebuildSegmentIndex() {
    _segmentIndex.clear();
    Duration cumulative = Duration.zero;
    for (final seg in _segments) {
      final wallStart = seg.startTime.difference(_dayStart);
      final wallEnd = seg.endTime.difference(_dayStart);
      final segDuration = wallEnd - wallStart;
      _segmentIndex.add(_SegmentMapEntry(
        wallStart: wallStart,
        wallEnd: wallEnd,
        playerOffset: cumulative,
      ));
      cumulative += segDuration;
    }
  }

  /// Convert wall-clock duration (since midnight) to player position.
  Duration _wallClockToPlayer(Duration wallClock) {
    for (final entry in _segmentIndex) {
      if (wallClock >= entry.wallStart && wallClock <= entry.wallEnd) {
        return entry.playerOffset + (wallClock - entry.wallStart);
      }
    }
    // Past all segments or in a gap — find the nearest segment
    for (final entry in _segmentIndex) {
      if (entry.wallStart > wallClock) {
        return entry.playerOffset; // snap to start of next segment
      }
    }
    // Past everything — return end
    if (_segmentIndex.isNotEmpty) {
      final last = _segmentIndex.last;
      return last.playerOffset + (last.wallEnd - last.wallStart);
    }
    return Duration.zero;
  }

  /// Convert player position to wall-clock duration (since midnight).
  Duration _playerToWallClock(Duration playerPos) {
    for (final entry in _segmentIndex) {
      final segDuration = entry.wallEnd - entry.wallStart;
      if (playerPos < entry.playerOffset + segDuration) {
        final offsetInSeg = playerPos - entry.playerOffset;
        return entry.wallStart + offsetInSeg;
      }
    }
    // Past all segments
    if (_segmentIndex.isNotEmpty) {
      return _segmentIndex.last.wallEnd;
    }
    return Duration.zero;
  }

  // ── Playback controls ──────────────────────────────────────────────

  Future<void> play() async {
    if (_players.isEmpty) {
      await _openPlayers();
      // If position is at 0 and we have segments, seek to first recording
      if (_position == Duration.zero && _segments.isNotEmpty) {
        final firstStart = _segments.first.startTime.difference(_dayStart);
        _position = firstStart;
        notifyListeners();
      }
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
      milliseconds: wallClockTarget.inMilliseconds.clamp(0, _maxPosition.inMilliseconds),
    );
    final snapped = _snapToSegment(clamped);

    _isSeeking = true;
    _position = snapped;
    _lastSeekTarget = snapped;
    notifyListeners();

    try {
      if (_players.isEmpty) {
        await _openPlayers();
      }

      final playerPos = _wallClockToPlayer(snapped);
      for (final p in _players.values) {
        await p.seek(playerPos);
      }
    } catch (e) {
      debugPrint('Playback seek error: $e');
      _error = e.toString();
    } finally {
      _isSeeking = false;
      _seekDebounceUntil = DateTime.now().add(const Duration(milliseconds: 150));
    }
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

  String get _dateKey =>
      '${_selectedDate.year}-${_selectedDate.month.toString().padLeft(2, '0')}-${_selectedDate.day.toString().padLeft(2, '0')}';

  Future<void> _openPlayers() async {
    final token = await getAccessToken();
    _error = null;

    for (final camId in _selectedCameraIds) {
      final url = playbackService.playlistUrl(
        cameraId: camId,
        date: _dateKey,
        token: token,
      );

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

        await player.open(Media(url), play: false);
      } catch (e) {
        debugPrint('Failed to open player for camera $camId: $e');
        _players.remove(camId)?.dispose();
        _videoControllers.remove(camId);
      }
    }

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
      if (DateTime.now().isBefore(_seekDebounceUntil)) return;

      final wallClock = _playerToWallClock(playerPos);

      if (_lastSeekTarget == null &&
          _position - wallClock > const Duration(seconds: 2)) {
        return;
      }

      _lastSeekTarget = null;
      _position = wallClock;
      notifyListeners();
    });

    _completedSub = primary.stream.completed.listen((completed) {
      if (_disposed || !completed) return;
      _isPlaying = false;
      notifyListeners();
    });

    notifyListeners();
  }

  Duration _snapToSegment(Duration position) {
    if (_segments.isEmpty) return position;
    final posTime = _dayStart.add(position);

    // Check if inside any segment (inclusive end boundary).
    for (final seg in _segments) {
      if (!posTime.isBefore(seg.startTime) && !posTime.isAfter(seg.endTime)) {
        return position;
      }
    }

    // In a gap: snap to nearest segment boundary (prev end or next start).
    Duration? nearestBefore;
    Duration? nearestAfter;

    for (final seg in _segments) {
      if (!seg.endTime.isAfter(posTime)) {
        nearestBefore = seg.endTime.difference(_dayStart);
      }
      if (seg.startTime.isAfter(posTime) && nearestAfter == null) {
        nearestAfter = seg.startTime.difference(_dayStart);
      }
    }

    // Prefer snapping forward to next segment start.
    if (nearestAfter != null) return nearestAfter;
    // If past all segments, snap to last segment end.
    if (nearestBefore != null) return nearestBefore;
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
    _disposeAllPlayers();
    super.dispose();
  }
}

class _SegmentMapEntry {
  final Duration wallStart;
  final Duration wallEnd;
  final Duration playerOffset;

  const _SegmentMapEntry({
    required this.wallStart,
    required this.wallEnd,
    required this.playerOffset,
  });
}
