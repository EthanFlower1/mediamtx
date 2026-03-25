import 'dart:async';
import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';
import '../../models/bookmark.dart';
import '../../models/recording.dart';
import '../../services/playback_service.dart';

/// Parsed from the HLS manifest's #EXT-X-PROGRAM-DATE-TIME tags.
/// Maps a player offset to an absolute wall-clock time so we can
/// display the correct time on the timeline.
class _DateTimeEntry {
  final Duration playerOffset;
  final DateTime wallClock;
  const _DateTimeEntry({required this.playerOffset, required this.wallClock});
}

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

  // Players
  final Map<String, Player> _players = {};
  final Map<String, VideoController> _videoControllers = {};
  StreamSubscription<Duration>? _positionSub;
  StreamSubscription<bool>? _completedSub;

  // Camera ID → MediaMTX path
  final Map<String, String> _cameraPaths = {};

  // Time map parsed from the HLS manifest. The server embeds
  // #EXT-X-PROGRAM-DATE-TIME at each recording boundary, so we know
  // the absolute wall-clock time for each section of the player timeline.
  List<_DateTimeEntry> _timeMap = [];

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
    _timeMap = [];
    notifyListeners();
  }

  void setSelectedCameraIds(List<String> ids) {
    if (_listEquals(ids, _selectedCameraIds)) return;
    _selectedCameraIds = ids;
    _disposeAllPlayers();
    _timeMap = [];
    notifyListeners();
  }

  // ── Manifest time map ─────────────────────────────────────────────

  /// Parse an HLS manifest to extract a time map from
  /// #EXT-X-PROGRAM-DATE-TIME tags and #EXTINF durations.
  static List<_DateTimeEntry> _parseManifest(String manifest) {
    final entries = <_DateTimeEntry>[];
    final lines = manifest.split('\n');
    Duration cumulative = Duration.zero;
    DateTime? pendingDateTime;

    for (final line in lines) {
      if (line.startsWith('#EXT-X-PROGRAM-DATE-TIME:')) {
        final dateStr =
            line.substring('#EXT-X-PROGRAM-DATE-TIME:'.length).trim();
        pendingDateTime = DateTime.tryParse(dateStr);
        if (pendingDateTime != null) {
          entries.add(_DateTimeEntry(
            playerOffset: cumulative,
            wallClock: pendingDateTime,
          ));
        }
      } else if (line.startsWith('#EXTINF:')) {
        final durStr = line.substring('#EXTINF:'.length).split(',').first;
        final durSec = double.tryParse(durStr) ?? 0;
        cumulative +=
            Duration(microseconds: (durSec * 1000000).round());
      }
    }
    return entries;
  }

  /// Convert player position → wall-clock Duration (since midnight).
  Duration _playerToWallClock(Duration playerPos) {
    if (_timeMap.isEmpty) return _position;

    _DateTimeEntry? active;
    for (final entry in _timeMap) {
      if (entry.playerOffset <= playerPos) {
        active = entry;
      } else {
        break;
      }
    }
    if (active == null) return _position;

    final offset = playerPos - active.playerOffset;
    return active.wallClock.add(offset).difference(_dayStart);
  }

  /// Convert wall-clock Duration (since midnight) → player position.
  Duration _wallClockToPlayer(Duration wallClock) {
    if (_timeMap.isEmpty) return Duration.zero;

    final targetTime = _dayStart.add(wallClock);

    // Find the time-map entry whose wall-clock range contains the target.
    // Walk backwards through entries to find the last one at or before target.
    _DateTimeEntry? active;
    for (final entry in _timeMap) {
      if (!entry.wallClock.isAfter(targetTime)) {
        active = entry;
      } else {
        break;
      }
    }
    if (active == null) return Duration.zero;

    final offset = targetTime.difference(active.wallClock);
    return active.playerOffset + offset;
  }

  // ── Playback controls ──────────────────────────────────────────────

  Future<void> play() async {
    if (_players.isEmpty) {
      await _openPlayers();
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

    try {
      if (_players.isEmpty) {
        await _openPlayers();
      }

      final playerPos = _wallClockToPlayer(snapped);
      debugPrint(
          'SEEK: wallClock=$snapped → playerPos=$playerPos, '
          'timeMap=${_timeMap.length} entries');

      for (final p in _players.values) {
        await p.seek(playerPos);
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

  String get _dateKey =>
      '${_selectedDate.year}-${_selectedDate.month.toString().padLeft(2, '0')}-${_selectedDate.day.toString().padLeft(2, '0')}';

  /// Open players with the HLS VOD playlist for the selected date.
  ///
  /// The server generates a manifest with byte-range addressed fMP4
  /// fragments and #EXT-X-PROGRAM-DATE-TIME tags for absolute time.
  /// The player handles seeking, buffering, and gap navigation natively.
  Future<void> _openPlayers() async {
    _disposeAllPlayers();

    final token = await getAccessToken();
    _error = null;

    final primaryCam =
        _selectedCameraIds.isNotEmpty ? _selectedCameraIds.first : null;

    // Fetch the manifest as text to build the time map, then open
    // the same URL with the player.
    if (primaryCam != null) {
      final manifestUrl = playbackService.playlistUrl(
        cameraId: primaryCam,
        date: _dateKey,
        token: token,
      );

      try {
        final dio = Dio();
        final response = await dio.get<String>(manifestUrl);
        dio.close();
        if (response.statusCode == 200 && response.data != null) {
          _timeMap = _parseManifest(response.data!);
          debugPrint(
              'HLS manifest: ${_timeMap.length} date-time entries, '
              '${response.data!.split('\n').length} lines');
        }
      } catch (e) {
        debugPrint('Manifest fetch error: $e');
      }
    }

    // Open each camera's HLS stream.
    await Future.wait(_selectedCameraIds.map((camId) async {
      final url = playbackService.playlistUrl(
        cameraId: camId,
        date: _dateKey,
        token: token,
      );

      debugPrint('Opening HLS: $url');

      try {
        final player = Player();
        player.setRate(_speed);
        _players[camId] = player;
        _videoControllers[camId] = VideoController(player);

        player.stream.error.listen((error) {
          if (_disposed) return;
          _error = error;
          debugPrint('Player error: $error');
          notifyListeners();
        });

        await player.open(Media(url), play: true);

        // Wait for the HLS playlist to load.
        if (player.state.duration == Duration.zero) {
          await player.stream.duration
              .firstWhere((d) => d > Duration.zero)
              .timeout(const Duration(seconds: 15),
                  onTimeout: () => Duration.zero);
        }

        player.pause();

        debugPrint(
            'Player ready: camera=$camId, '
            'duration=${player.state.duration}');
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
