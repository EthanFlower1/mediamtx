import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:video_player/video_player.dart';
import '../../models/bookmark.dart';
import '../../models/recording.dart';
import '../../services/playback_service.dart';

class PlaybackController extends ChangeNotifier {
  final PlaybackService playbackService;
  final Future<String?> Function() getAccessToken;

  bool _disposed = false;

  // State
  Duration _position = Duration.zero;
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
  final Map<String, VideoPlayerController> _players = {};

  // Camera ID → MediaMTX path
  final Map<String, String> _cameraPaths = {};

  // The recording segment currently loaded in the player.
  RecordingSegment? _currentSegment;

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
  Map<String, VideoPlayerController> get videoControllers => _players;
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
    _currentSegment = null;
    notifyListeners();
  }

  void setSelectedCameraIds(List<String> ids) {
    if (_listEquals(ids, _selectedCameraIds)) return;
    _selectedCameraIds = ids;
    _disposeAllPlayers();
    _currentSegment = null;
    notifyListeners();
  }

  // ── Position mapping ──────────────────────────────────────────────

  /// Convert player position to wall-clock. Simple: the file starts
  /// at _currentSegment.startTime, player position is offset from there.
  Duration _playerToWallClock(Duration playerPos) {
    if (_currentSegment == null) return _position;
    final wc = _currentSegment!.startTime.add(playerPos);
    return wc.difference(_dayStart);
  }

  // ── Playback controls ──────────────────────────────────────────────

  Future<void> play() async {
    if (_players.isEmpty) {
      await _openSegmentAt(_position);
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
      final targetTime = _dayStart.add(snapped);

      // Find the recording segment containing the target time.
      final targetSeg = _findSegmentForTime(targetTime);

      if (targetSeg != null &&
          _currentSegment != null &&
          targetSeg.id == _currentSegment!.id &&
          _players.isNotEmpty) {
        // Same segment — seek within the local file (instant).
        final offset = targetTime.difference(targetSeg.startTime);
        debugPrint('SEEK: in-place, offset=$offset in segment ${targetSeg.id}');
        for (final p in _players.values) {
          await p.seekTo(offset);
        }
      } else if (targetSeg != null) {
        // Different segment — open the new file.
        await _openSegmentAt(snapped);
        if (wasPlaying) {
          _isPlaying = true;
          for (final p in _players.values) {
            p.play();
          }
        }
      } else {
        _error = 'No recording at this time';
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
      p.setPlaybackSpeed(s);
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

  /// Find the recording segment (for the primary camera) that contains [time].
  RecordingSegment? _findSegmentForTime(DateTime time) {
    final primaryCam =
        _selectedCameraIds.isNotEmpty ? _selectedCameraIds.first : null;
    if (primaryCam == null) return null;

    for (final seg in _segments) {
      if (seg.cameraId == primaryCam &&
          !time.isBefore(seg.startTime) &&
          !time.isAfter(seg.endTime)) {
        return seg;
      }
    }
    return null;
  }

  /// Find the next segment at or after [time].
  RecordingSegment? _findNextSegmentForTime(DateTime time) {
    final primaryCam =
        _selectedCameraIds.isNotEmpty ? _selectedCameraIds.first : null;
    if (primaryCam == null) return null;

    for (final seg in _segments) {
      if (seg.cameraId == primaryCam &&
          (seg.contains(time) || seg.startTime.isAfter(time))) {
        return seg;
      }
    }
    return null;
  }

  /// Build the URL for serving a recording file directly.
  /// The server's /vod/segments endpoint serves the raw file with
  /// HTTP Range support, making it fully seekable.
  String _segmentFileUrl(RecordingSegment seg, String? token) {
    final serverUrl = playbackService.serverUrl;
    final uri = Uri.parse(serverUrl);

    // Strip the recordings prefix from the file path.
    var rel = seg.filePath ?? '';
    if (rel.startsWith('./recordings/')) {
      rel = rel.substring('./recordings/'.length);
    } else if (rel.startsWith('recordings/')) {
      rel = rel.substring('recordings/'.length);
    }

    final params = <String, String>{};
    if (token != null && token.isNotEmpty) {
      params['jwt'] = token;
    }

    return Uri(
      scheme: uri.scheme,
      host: uri.host,
      port: uri.port,
      path: '/api/nvr/vod/segments/$rel',
      queryParameters: params.isNotEmpty ? params : null,
    ).toString();
  }

  void _onPositionUpdate() {
    if (_disposed || _isSeeking) return;
    final primary = _players.values.firstOrNull;
    if (primary == null) return;

    final now = DateTime.now();
    if (now.isBefore(_nextPositionUpdate)) return;
    _nextPositionUpdate = now.add(const Duration(milliseconds: 250));

    final playerPos = primary.value.position;
    final wc = _playerToWallClock(playerPos);

    if ((_position - wc).inSeconds > 2) return;

    _position = wc;
    notifyListeners();

    // Auto-advance to next segment when current one ends.
    if (_isPlaying &&
        _currentSegment != null &&
        primary.value.duration > Duration.zero &&
        playerPos >= primary.value.duration - const Duration(milliseconds: 500)) {
      _advanceToNextSegment();
    }
  }

  Future<void> _advanceToNextSegment() async {
    if (_currentSegment == null) return;
    final nextSeg = _findNextSegmentForTime(
        _currentSegment!.endTime.add(const Duration(milliseconds: 1)));

    if (nextSeg != null) {
      final wallClock = nextSeg.startTime.difference(_dayStart);
      _isPlaying = true;
      await _openSegmentAt(wallClock);
      for (final p in _players.values) {
        p.play();
      }
      notifyListeners();
    } else if (_continuousMode) {
      final nextDay = DateTime(
          _selectedDate.year, _selectedDate.month, _selectedDate.day + 1);
      setSelectedDate(nextDay);
    } else {
      _isPlaying = false;
      notifyListeners();
    }
  }

  /// Open the recording segment that contains [wallClock].
  /// Serves the raw fMP4 file via HTTP with Range support — fully seekable.
  Future<void> _openSegmentAt(Duration wallClock) async {
    _disposeAllPlayers();

    final token = await getAccessToken();
    _error = null;

    final targetTime = _dayStart.add(wallClock);

    // Find the segment containing the target, or the next one.
    var seg = _findSegmentForTime(targetTime);
    DateTime seekTarget = targetTime;

    if (seg == null) {
      seg = _findNextSegmentForTime(targetTime);
      if (seg == null) {
        _error = 'No recordings at this time';
        notifyListeners();
        return;
      }
      seekTarget = seg.startTime;
      _position = seekTarget.difference(_dayStart);
    }

    _currentSegment = seg;

    if (seg.filePath == null || seg.filePath!.isEmpty) {
      _error = 'Recording has no file path';
      notifyListeners();
      return;
    }

    final url = _segmentFileUrl(seg, token);
    final offset = seekTarget.difference(seg.startTime);

    debugPrint(
        'Opening file: segment ${seg.id}, '
        'start=${seg.startTime}, url=$url, seekOffset=$offset');

    // Open for each selected camera. For now, all cameras use the
    // primary camera's segment (multi-camera would need per-camera segments).
    for (final camId in _selectedCameraIds) {
      try {
        final controller = VideoPlayerController.networkUrl(
          Uri.parse(url),
        );

        await controller.initialize();
        controller.setPlaybackSpeed(_speed);
        controller.addListener(_onPositionUpdate);

        // Seek to the target offset within the file.
        if (offset > Duration.zero) {
          await controller.seekTo(offset);
        }

        _players[camId] = controller;

        debugPrint(
            'Player ready: camera=$camId, '
            'duration=${controller.value.duration}, '
            'seeked to $offset');
      } catch (e) {
        debugPrint('Failed to open player for camera $camId: $e');
        _players.remove(camId)?.dispose();
      }
    }

    if (_players.isEmpty) {
      _error = 'Failed to open any camera for playback';
      notifyListeners();
      return;
    }

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
    for (final p in _players.values) {
      p.removeListener(_onPositionUpdate);
      p.dispose();
    }
    _players.clear();
  }

  @override
  void dispose() {
    _disposed = true;
    _disposeAllPlayers();
    super.dispose();
  }
}

extension on RecordingSegment {
  bool contains(DateTime time) =>
      !time.isBefore(startTime) && !time.isAfter(endTime);
}
