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
        _selectedDate.day == date.day) {
      return;
    }
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
      if (_disposed) return;
      final wasPlaying = _isPlaying;
      _disposeAllPlayers();
      await _openPlayers(snapped);

      // Briefly play then pause to force the player to decode past the
      // keyframe pre-roll. The server starts from the nearest keyframe
      // before the requested time, so position 0 in the player may
      // correspond to a frame a few seconds before our target. Playing
      // briefly lets mpv decode forward to the actual requested time.
      for (final p in _players.values) {
        p.play();
      }
      await Future.delayed(const Duration(milliseconds: 200));
      if (_disposed) return;

      if (!wasPlaying) {
        for (final p in _players.values) {
          p.pause();
        }
        _isPlaying = false;
      } else {
        _isPlaying = true;
      }

      _isSeeking = false;
      notifyListeners();
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
