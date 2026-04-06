import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:video_player/video_player.dart';
import '../../models/bookmark.dart';
import '../../models/detection_frame.dart';
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
  final Set<String> _mutedCameras = {};
  bool _isInGap = false;
  DateTime _nextPositionUpdate = DateTime(2000);
  Timer? _seekDebounce;
  DateTime _selectedDate = DateTime.now();
  List<String> _selectedCameraIds = [];
  List<RecordingSegment> _segments = [];
  List<MotionEvent> _events = [];
  List<Bookmark> _bookmarks = [];
  String? _error;

  // Gap timer — advances position when between recording segments
  Timer? _gapTimer;
  static const _gapTickInterval = Duration(milliseconds: 250);

  // Players — one per selected camera
  final Map<String, VideoPlayerController> _players = {};

  // Camera ID → MediaMTX path
  final Map<String, String> _cameraPaths = {};

  // The recording segment currently loaded in the player.
  RecordingSegment? _currentSegment;

  // Detection cache — prefetched per segment for playback overlay.
  final Map<String, List<PlaybackDetection>> _detectionCache = {};
  final Set<String> _overlayDisabledCameras = {};

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
  Set<String> get mutedCameras => _mutedCameras;
  bool get isInGap => _isInGap;
  DateTime get selectedDate => _selectedDate;
  List<String> get selectedCameraIds => _selectedCameraIds;
  List<RecordingSegment> get segments => _segments;
  List<MotionEvent> get events => _events;
  List<Bookmark> get bookmarks => _bookmarks;
  Map<String, VideoPlayerController> get videoControllers => _players;
  String? get error => _error;

  /// Whether the given camera has a recording segment covering the current
  /// playback position.  Used by the camera player to distinguish between
  /// "loading" and "no recording at this time" when the player is null.
  bool hasCoverageAtPosition(String cameraId) {
    final targetTime = _dayStart.add(_position);
    return _findSegmentForCamera(cameraId, targetTime) != null;
  }

  Map<String, List<PlaybackDetection>> get detectionCache => _detectionCache;
  bool isOverlayDisabled(String cameraId) =>
      _overlayDisabledCameras.contains(cameraId);

  bool hasDetectionsForCamera(String cameraId) =>
      _detectionCache.containsKey(cameraId) &&
      _detectionCache[cameraId]!.isNotEmpty;

  // ── Data setters ────────────────────────────────────────────────────

  void setSegments(List<RecordingSegment> s) {
    final changed = !_segmentsEqual(_segments, s);
    _segments = s;
    if (changed && _position == Duration.zero && _segments.isNotEmpty) {
      final first = _segments.first.startTime;
      final last = _segments.last.endTime;
      final mid = first.add(last.difference(first) ~/ 2);
      _position = mid.difference(_dayStart);
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

  /// Convert player position to wall-clock time.
  ///
  /// The DB's startTime is when the recording process started, but the
  /// first video frame arrives slightly later. We compute the offset
  /// from (wallClockSpan - mediaDuration) and shift the start forward
  /// so player position 0 aligns with the actual first frame.
  Duration _playerToWallClock(Duration playerPos) {
    if (_currentSegment == null) return _position;

    final primary = _players.values.firstOrNull;
    final mediaDuration = primary?.value.duration ?? Duration.zero;
    final wallClockDuration =
        _currentSegment!.endTime.difference(_currentSegment!.startTime);

    // The gap between recording start and first frame.
    final startOffset = mediaDuration > Duration.zero
        ? wallClockDuration - mediaDuration
        : Duration.zero;

    final wc = _currentSegment!.startTime
        .add(startOffset)
        .add(playerPos);
    return wc.difference(_dayStart);
  }

  /// Convert wall-clock time to player position (inverse of above).
  Duration _wallClockToPlayerPos(DateTime targetTime, RecordingSegment seg) {
    final primary = _players.values.firstOrNull;
    final mediaDuration = primary?.value.duration ?? Duration.zero;
    final wallClockDuration = seg.endTime.difference(seg.startTime);

    final startOffset = mediaDuration > Duration.zero
        ? wallClockDuration - mediaDuration
        : Duration.zero;

    // wallClock = startTime + startOffset + playerPos
    // → playerPos = wallClock - startTime - startOffset
    final playerPos = targetTime.difference(seg.startTime) - startOffset;
    return playerPos < Duration.zero ? Duration.zero : playerPos;
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
    _seekDebounce?.cancel();
    _stopGapTimer();
    _isInGap = false;
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

    // Update position immediately for smooth playhead movement.
    _isInGap = false;
    _stopGapTimer();
    _position = snapped;
    notifyListeners();

    // Debounce the actual media seek to avoid overwhelming the player
    // during scrubbing (timeline fires onPositionChanged per pixel).
    _seekDebounce?.cancel();
    _seekDebounce = Timer(const Duration(milliseconds: 150), () {
      _performSeek(snapped);
    });
  }

  Future<void> _performSeek(Duration snapped) async {
    _isSeeking = true;
    final wasPlaying = _isPlaying;

    try {
      final targetTime = _dayStart.add(snapped);

      // Find the recording segment containing the target time.
      final targetSeg = _findSegmentForTime(targetTime);

      if (targetSeg != null &&
          _currentSegment != null &&
          targetSeg.id == _currentSegment!.id &&
          _players.isNotEmpty) {
        // Same segment — seek within the local file.
        // Pause before seeking so the player renders the target frame
        // immediately; without this, some backends (AVPlayer on macOS/iOS)
        // keep showing the old position until playback is toggled.
        if (wasPlaying) {
          for (final p in _players.values) {
            await p.pause();
          }
        }
        final seekPos = _wallClockToPlayerPos(targetTime, targetSeg);
        for (final p in _players.values) {
          await p.seekTo(seekPos);
        }
        if (wasPlaying) {
          for (final p in _players.values) {
            await p.play();
          }
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

  bool _speedAutoMuted = false;

  bool _isSpeedAudible(double speed) => speed >= 0.5 && speed <= 2.0;

  void setSpeed(double s) {
    _speed = s;
    for (final p in _players.values) {
      p.setPlaybackSpeed(s);
    }
    // Auto-mute when speed is outside audible range, restore when back.
    if (!_isSpeedAudible(s) && !_speedAutoMuted) {
      _speedAutoMuted = true;
      for (final entry in _players.entries) {
        if (!_mutedCameras.contains(entry.key)) {
          entry.value.setVolume(0.0);
        }
      }
    } else if (_isSpeedAudible(s) && _speedAutoMuted) {
      _speedAutoMuted = false;
      for (final entry in _players.entries) {
        if (!_mutedCameras.contains(entry.key)) {
          entry.value.setVolume(1.0);
        }
      }
    }
    notifyListeners();
  }

  void toggleMute(String cameraId) {
    if (_mutedCameras.contains(cameraId)) {
      _mutedCameras.remove(cameraId);
    } else {
      _mutedCameras.add(cameraId);
    }
    final player = _players[cameraId];
    if (player != null) {
      player.setVolume(_mutedCameras.contains(cameraId) ? 0.0 : 1.0);
    }
    notifyListeners();
  }

  bool isCameraMuted(String cameraId) => _mutedCameras.contains(cameraId);

  void toggleOverlay(String cameraId) {
    if (_overlayDisabledCameras.contains(cameraId)) {
      _overlayDisabledCameras.remove(cameraId);
    } else {
      _overlayDisabledCameras.add(cameraId);
    }
    notifyListeners();
  }

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

  /// Build the URL for serving a recording file by its database ID.
  /// The server's /vod/segments/:id endpoint serves the raw file with
  /// HTTP Range support, making it fully seekable.
  String _segmentFileUrl(RecordingSegment seg, String? token) {
    final serverUrl = playbackService.serverUrl;
    final uri = Uri.parse(serverUrl);

    final params = <String, String>{};
    if (token != null && token.isNotEmpty) {
      params['jwt'] = token;
    }

    return Uri(
      scheme: uri.scheme,
      host: uri.host,
      port: uri.port,
      path: '/api/nvr/vod/segments/${seg.id}',
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
      // Check if there's a gap before the next segment
      final gapDuration =
          nextSeg.startTime.difference(_currentSegment!.endTime);
      if (gapDuration > const Duration(seconds: 1)) {
        // Enter gap mode — dispose players, advance position via timer
        final gapStart = _currentSegment!.endTime.difference(_dayStart);
        _disposeAllPlayers();
        _currentSegment = null;
        _isInGap = true;
        _position = gapStart;
        notifyListeners();
        _startGapTimer(nextSeg);
        return;
      }

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

  void _startGapTimer(RecordingSegment nextSegment) {
    _stopGapTimer();
    final targetPosition = nextSegment.startTime.difference(_dayStart);

    _gapTimer = Timer.periodic(_gapTickInterval, (timer) {
      if (_disposed || !_isPlaying) {
        _stopGapTimer();
        return;
      }

      final advance = Duration(
        milliseconds: (_gapTickInterval.inMilliseconds * _speed).round(),
      );
      _position += advance;

      if (_position >= targetPosition) {
        _position = targetPosition;
        _isInGap = false;
        _stopGapTimer();
        _openSegmentAt(_position).then((_) {
          if (_isPlaying) {
            for (final p in _players.values) {
              p.play();
            }
          }
          notifyListeners();
        });
      }
      notifyListeners();
    });
  }

  void _stopGapTimer() {
    _gapTimer?.cancel();
    _gapTimer = null;
  }

  /// Clock drift tolerance — segments whose boundaries are within this
  /// duration of the target time are considered to contain it.  This accounts
  /// for minor clock differences between cameras.
  static const _clockDriftTolerance = Duration(seconds: 2);

  /// Find the best segment for [cameraId] that contains (or is closest to)
  /// [time], accounting for clock drift tolerance.
  RecordingSegment? _findSegmentForCamera(String cameraId, DateTime time) {
    RecordingSegment? best;
    Duration bestDistance = const Duration(days: 1);

    for (final seg in _segments) {
      if (seg.cameraId != cameraId) continue;

      // Exact containment.
      if (!time.isBefore(seg.startTime) && !time.isAfter(seg.endTime)) {
        return seg;
      }

      // Within drift tolerance.
      final startDiff = seg.startTime.difference(time).abs();
      final endDiff = seg.endTime.difference(time).abs();
      final minDiff = startDiff < endDiff ? startDiff : endDiff;
      if (minDiff <= _clockDriftTolerance && minDiff < bestDistance) {
        bestDistance = minDiff;
        best = seg;
      }
    }
    return best;
  }

  /// Find the next segment for [cameraId] at or after [time].
  RecordingSegment? _findNextSegmentForCamera(String cameraId, DateTime time) {
    for (final seg in _segments) {
      if (seg.cameraId != cameraId) continue;
      if (seg.contains(time) || seg.startTime.isAfter(time)) {
        return seg;
      }
    }
    return null;
  }

  /// Open the recording segment that contains [wallClock].
  /// Each camera gets its own segment lookup so cameras with different
  /// recording boundaries are handled correctly.
  /// Serves the raw fMP4 file via HTTP with Range support — fully seekable.
  Future<void> _openSegmentAt(Duration wallClock) async {
    _disposeAllPlayers();
    _stopGapTimer();

    final token = await getAccessToken();
    _error = null;

    final targetTime = _dayStart.add(wallClock);

    // Use the primary camera to determine gap/segment state.
    final primaryCam =
        _selectedCameraIds.isNotEmpty ? _selectedCameraIds.first : null;
    if (primaryCam == null) {
      _error = 'No cameras selected';
      notifyListeners();
      return;
    }

    // Find the segment containing the target for the primary camera.
    var primarySeg = _findSegmentForCamera(primaryCam, targetTime);
    DateTime seekTarget = targetTime;

    if (primarySeg == null) {
      primarySeg = _findNextSegmentForCamera(primaryCam, targetTime);
      if (primarySeg == null) {
        _isInGap = false;
        _error = 'No recordings at this time';
        notifyListeners();
        return;
      }
      // We're in a gap — if playing, advance through it with timer
      if (_isPlaying &&
          primarySeg.startTime.difference(targetTime) >
              const Duration(seconds: 1)) {
        _isInGap = true;
        notifyListeners();
        _startGapTimer(primarySeg);
        return;
      }
      seekTarget = primarySeg.startTime;
      _position = seekTarget.difference(_dayStart);
    }

    _isInGap = false;
    _currentSegment = primarySeg;

    // Open a player for each selected camera using its own segment.
    for (final camId in _selectedCameraIds) {
      // Find the best segment for this specific camera.
      var camSeg = _findSegmentForCamera(camId, seekTarget);
      camSeg ??= _findNextSegmentForCamera(camId, seekTarget);

      if (camSeg == null || camSeg.filePath == null || camSeg.filePath!.isEmpty) {
        debugPrint('No segment available for camera $camId at $seekTarget');
        continue;
      }

      final url = _segmentFileUrl(camSeg, token);
      final wallOffset = seekTarget.difference(camSeg.startTime);

      debugPrint(
          'Opening file: camera=$camId, segment ${camSeg.id}, '
          'start=${camSeg.startTime}, url=$url, wallOffset=$wallOffset');

      try {
        final controller = VideoPlayerController.networkUrl(
          Uri.parse(url),
        );

        await controller.initialize();
        controller.setPlaybackSpeed(_speed);
        // Set volume immediately after init — await to ensure it's applied
        // before any audio frames play.
        await controller.setVolume(_mutedCameras.contains(camId) ? 0.0 : 1.0);
        controller.addListener(_onPositionUpdate);

        // Seek to the target position within the file.
        if (wallOffset > Duration.zero) {
          final mediaDuration = controller.value.duration;
          final wallClockDuration =
              camSeg.endTime.difference(camSeg.startTime);
          final startOffset = mediaDuration > Duration.zero
              ? wallClockDuration - mediaDuration
              : Duration.zero;
          var seekPos = wallOffset - startOffset;
          if (seekPos < Duration.zero) seekPos = Duration.zero;
          await controller.seekTo(seekPos);
        }

        _players[camId] = controller;

        debugPrint(
            'Player ready: camera=$camId, '
            'duration=${controller.value.duration}, '
            'seeked to $wallOffset');
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

    // Fetch detections for the primary segment (non-blocking).
    _fetchDetectionsForSegment(primarySeg);

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
    _detectionCache.clear();
  }

  @override
  void dispose() {
    _disposed = true;
    _seekDebounce?.cancel();
    _stopGapTimer();
    _disposeAllPlayers();
    super.dispose();
  }
}

extension on RecordingSegment {
  bool contains(DateTime time) =>
      !time.isBefore(startTime) && !time.isAfter(endTime);
}
