import 'dart:async';
import 'dart:io';
import 'package:dio/dio.dart';
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

  // Timespans from the MediaMTX /list endpoint (primary camera).
  List<PlaybackTimespan> _timespans = [];

  // The index into _timespans for the currently playing timespan.
  int _currentTimespanIndex = -1;

  // Downloaded MP4 files for local seekable playback.
  // Key: camera ID, value: temp file path.
  final Map<String, String> _downloadedFiles = {};

  // The wall-clock start of the downloaded timespan (timespan.start).
  // player position 0 = this time. Seeking within the timespan is just
  // player.seek(targetTime - _timespanStart).
  DateTime _timespanStart = DateTime(2000);

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
    _timespans = [];
    _currentTimespanIndex = -1;
    notifyListeners();
  }

  void setSelectedCameraIds(List<String> ids) {
    if (_listEquals(ids, _selectedCameraIds)) return;
    _selectedCameraIds = ids;
    _disposeAllPlayers();
    _timespans = [];
    _currentTimespanIndex = -1;
    notifyListeners();
  }

  // ── Timespan helpers ──────────────────────────────────────────────

  /// Find the timespan index that contains [time], or -1 if in a gap.
  int _findTimespanIndex(DateTime time) {
    for (int i = 0; i < _timespans.length; i++) {
      if (_timespans[i].contains(time)) return i;
    }
    return -1;
  }

  /// Find the next timespan at or after [time], or -1 if none.
  int _findNextTimespanIndex(DateTime time) {
    for (int i = 0; i < _timespans.length; i++) {
      if (_timespans[i].contains(time) || _timespans[i].start.isAfter(time)) {
        return i;
      }
    }
    return -1;
  }

  /// Fetch timespans from the /list endpoint for the primary camera.
  Future<void> _loadTimespans() async {
    final primaryCam =
        _selectedCameraIds.isNotEmpty ? _selectedCameraIds.first : null;
    final cameraPath = primaryCam != null ? _cameraPaths[primaryCam] : null;
    if (cameraPath == null) return;

    final dayStart = _dayStart;
    final dayEnd = dayStart.add(const Duration(hours: 24));

    _timespans = await playbackService.listTimespans(
      cameraPath: cameraPath,
      start: dayStart,
      end: dayEnd,
    );

    debugPrint('Loaded ${_timespans.length} timespans from /list');
  }

  /// Convert player position to wall-clock time.
  /// Player position 0 = _timespanStart (we download the full timespan).
  Duration _playerToWallClock(Duration playerPos) {
    final wallClock = _timespanStart.add(playerPos);
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
      final targetTime = _dayStart.add(snapped);
      final tsIndex = _findTimespanIndex(targetTime);

      // If the target is in the same timespan we already downloaded,
      // seek within the local file — instant, no network.
      if (tsIndex >= 0 &&
          tsIndex == _currentTimespanIndex &&
          _players.isNotEmpty) {
        final offset = targetTime.difference(_timespanStart);
        debugPrint('SEEK: in-place local, offset=$offset');

        for (final p in _players.values) {
          await p.seek(offset);
        }
      } else {
        // Different timespan — download and open.
        await _openPlayersAt(snapped);

        if (wasPlaying) {
          _isPlaying = true;
          for (final p in _players.values) {
            p.play();
          }
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

  /// Open (or re-open) players starting at [wallClock] (since midnight).
  ///
  /// Downloads the full timespan as an MP4 to a local temp file, then
  /// opens it with the player. Local files are fully seekable — subsequent
  /// seeks within the same timespan are instant with player.seek().
  Future<void> _openPlayersAt(Duration wallClock) async {
    _disposeAllPlayers();

    final token = await getAccessToken();
    _error = null;

    // Load timespans from /list if we haven't yet.
    if (_timespans.isEmpty) {
      await _loadTimespans();
    }

    final targetTime = _dayStart.add(wallClock);

    // Find the timespan containing the target, or the next one after it.
    var tsIndex = _findTimespanIndex(targetTime);
    DateTime streamStart;

    if (tsIndex >= 0) {
      streamStart = targetTime;
    } else {
      tsIndex = _findNextTimespanIndex(targetTime);
      if (tsIndex < 0) {
        _error = 'No recordings at this time';
        notifyListeners();
        return;
      }
      streamStart = _timespans[tsIndex].start;
      _position = streamStart.difference(_dayStart);
    }

    _currentTimespanIndex = tsIndex;

    final ts = _timespans[tsIndex];
    _timespanStart = ts.start;

    // Download the FULL timespan (from its start, not the seek target)
    // so that seeking backward within the timespan also works.
    final durationSecs = ts.durationSecs.ceil();

    debugPrint(
        'Downloading: timespan $tsIndex/${_timespans.length}, '
        'tsStart=${ts.start}, duration=${durationSecs}s');

    final tempDir = Directory.systemTemp;

    await Future.wait(_selectedCameraIds.map((camId) async {
      // Use the pre-built URL from /list — guaranteed to match what
      // MediaMTX will serve. Just append JWT if needed.
      var url = ts.url;
      if (token != null && token.isNotEmpty) {
        final sep = url.contains('?') ? '&' : '?';
        url = '$url${sep}jwt=$token';
      }
      // Add format=mp4 for a seekable container with moov index.
      if (!url.contains('format=')) {
        url = '$url&format=mp4';
      }

      final filePath =
          '${tempDir.path}/playback_${camId}_$tsIndex.mp4';

      debugPrint('Downloading: $url → $filePath');

      try {
        final dio = Dio();
        await dio.download(url, filePath);
        dio.close();

        _downloadedFiles[camId] = filePath;

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

        // Open the local file — fully seekable.
        await player.open(Media(filePath), play: false);

        if (player.state.duration == Duration.zero) {
          await player.stream.duration
              .firstWhere((d) => d > Duration.zero)
              .timeout(const Duration(seconds: 10),
                  onTimeout: () => Duration.zero);
        }

        // Seek to the target offset within the timespan.
        final initialOffset = streamStart.difference(ts.start);
        if (initialOffset > Duration.zero) {
          await player.seek(initialOffset);
        }

        debugPrint(
            'Player ready: camera=$camId, '
            'duration=${player.state.duration}, '
            'seeked to $initialOffset');
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

      final now = DateTime.now();
      if (now.isBefore(_nextPositionUpdate)) return;
      _nextPositionUpdate = now.add(const Duration(milliseconds: 250));

      final wc = _playerToWallClock(playerPos);

      if ((_position - wc).inSeconds > 2) return;

      _position = wc;
      notifyListeners();
    });

    _completedSub = primary.stream.completed.listen((completed) {
      if (_disposed || !completed || _isSeeking) return;

      // Current timespan ended. If there's a next one, advance to it.
      final nextIndex = _currentTimespanIndex + 1;
      if (nextIndex < _timespans.length) {
        final nextStart =
            _timespans[nextIndex].start.difference(_dayStart);
        _isPlaying = true;
        seek(nextStart);
      } else if (_continuousMode) {
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

    // Clean up temp files.
    for (final path in _downloadedFiles.values) {
      try {
        File(path).deleteSync();
      } catch (_) {}
    }
    _downloadedFiles.clear();
  }

  @override
  void dispose() {
    _disposed = true;
    _disposeAllPlayers();
    super.dispose();
  }
}
