import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';
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

  // The backend streams fMP4 from a start time — it's not seekable via HTTP
  // range requests. So "seeking" means opening a new stream from the target
  // time. _streamOrigin tracks what day-offset the current stream starts at,
  // so we can translate the player's stream-relative position to day-absolute.
  Duration _streamOrigin = Duration.zero;

  // Players keyed by camera ID
  final Map<String, Player> _players = {};
  final Map<String, VideoController> _videoControllers = {};
  StreamSubscription<Duration>? _positionSub;

  static const _maxPosition = Duration(hours: 24);
  static const _positionThrottle = Duration(milliseconds: 66);

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
    final toRemove = _players.keys
        .where((id) => !_selectedCameraIds.contains(id))
        .toList();
    for (final id in toRemove) {
      _players[id]?.dispose();
      _players.remove(id);
      _videoControllers.remove(id);
    }

    for (final id in _selectedCameraIds) {
      if (!_players.containsKey(id)) {
        final player = Player();
        _players[id] = player;
        _videoControllers[id] = VideoController(player);
      }
    }

    _positionSub?.cancel();
    if (_players.isNotEmpty) {
      DateTime? lastUpdate;
      _positionSub = _players.values.first.stream.position.listen((streamPos) {
        if (_isSeeking) return;

        final now = DateTime.now();
        if (lastUpdate != null &&
            now.difference(lastUpdate!) < _positionThrottle) {
          return;
        }
        lastUpdate = now;

        // The player reports position relative to the start of the current
        // stream. Translate to day-absolute by adding _streamOrigin.
        final dayPos = _streamOrigin + streamPos;
        _position = dayPos;

        // Auto-pause at end of last recording segment
        if (_isPlaying && _segments.isNotEmpty) {
          final dayStart = DateTime(
              _selectedDate.year, _selectedDate.month, _selectedDate.day);
          final lastEnd = _segments.last.endTime.difference(dayStart);
          if (dayPos >= lastEnd) {
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
    final streamStart = dayStart.add(_streamOrigin);
    // Duration from stream start to end of day
    final remainingSecs =
        (_maxPosition - _streamOrigin).inSeconds.clamp(1, 86400);

    for (final id in _selectedCameraIds) {
      final player = _players[id];
      if (player == null) continue;

      final path = _cameraPaths[id];
      if (path == null) continue;

      final url = playbackService.playbackUrl(
          path, streamStart, durationSecs: remainingSecs.toDouble());
      player.open(Media(url), play: _isPlaying);
      player.setRate(_speed);
    }
  }

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

    final dayStart = DateTime(
        _selectedDate.year, _selectedDate.month, _selectedDate.day);
    final snapped = snapToSegment(_segments, dayStart, clamped);

    _isSeeking = true;
    _position = snapped;
    _streamOrigin = snapped;
    notifyListeners();

    // Open new streams from the seek target — the backend serves fMP4 from
    // a start time and does not support HTTP range seeking within a stream.
    final streamStart = dayStart.add(snapped);
    final remainingSecs =
        (_maxPosition - snapped).inSeconds.clamp(1, 86400);

    for (final id in _selectedCameraIds) {
      final player = _players[id];
      if (player == null) continue;

      final path = _cameraPaths[id];
      if (path == null) continue;

      final url = playbackService.playbackUrl(
          path, streamStart, durationSecs: remainingSecs.toDouble());
      await player.open(Media(url), play: _isPlaying);
      player.setRate(_speed);
    }

    _isSeeking = false;
    notifyListeners();
  }

  void stepFrame(int direction) {
    if (_isPlaying) pause();

    if (direction > 0) {
      _position += const Duration(milliseconds: 33);
    } else {
      _position = Duration(
        milliseconds:
            (_position.inMilliseconds - 3000).clamp(0, _maxPosition.inMilliseconds),
      );
    }
    // Re-open stream from new position
    seek(_position);
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
