import 'dart:async';
import 'dart:convert';
import 'package:flutter/foundation.dart';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';
import 'package:web_socket_channel/web_socket_channel.dart';
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

  // WebSocket session
  WebSocketChannel? _ws;
  int _seq = 0;
  // ignore: unused_field
  String? _sessionId;
  bool _sessionCreated = false;

  // Players — created when session returns stream URLs
  final Map<String, Player> _players = {};
  final Map<String, VideoController> _videoControllers = {};

  // Camera paths for session creation
  final Map<String, String> _cameraPaths = {};

  static const _maxPosition = Duration(hours: 24);

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
    if (_sessionCreated) {
      _sendCommand({'cmd': 'close'});
      _sessionCreated = false;
      _sessionId = null;
      _disposeAllPlayers();
      _createSession();
    }
    notifyListeners();
  }

  void setSelectedCameraIds(List<String> ids) {
    if (_listEquals(ids, _selectedCameraIds)) return;
    _selectedCameraIds = ids;

    // If session exists, close and recreate with new cameras.
    if (_sessionCreated) {
      _sendCommand({'cmd': 'close'});
      _sessionCreated = false;
      _sessionId = null;
      _disposeAllPlayers();
      _createSession();
    }
    notifyListeners();
  }

  static bool _listEquals(List<String> a, List<String> b) {
    if (a.length != b.length) return false;
    for (int i = 0; i < a.length; i++) {
      if (a[i] != b[i]) return false;
    }
    return true;
  }

  void setCameraPaths(Map<String, String> paths) {
    _cameraPaths.addAll(paths);
  }

  // ── WebSocket management ──────────────────────────────────────────────

  void _connectWs() {
    _ws?.sink.close();
    final url = playbackService.playbackWsUrl();
    _ws = WebSocketChannel.connect(Uri.parse(url));
    _ws!.stream.listen(
      _handleWsMessage,
      onError: (e) {
        debugPrint('Playback WS error: $e');
      },
      onDone: () {
        debugPrint('Playback WS closed');
      },
    );
  }

  void _sendCommand(Map<String, dynamic> cmd) {
    cmd['seq'] = ++_seq;
    _ws?.sink.add(jsonEncode(cmd));
  }

  void _handleWsMessage(dynamic message) {
    final event = jsonDecode(message as String) as Map<String, dynamic>;
    switch (event['event']) {
      case 'created':
        _sessionId = event['session_id'] as String;
        _sessionCreated = true;
        final streams = (event['streams'] as Map<String, dynamic>)
            .map((k, v) => MapEntry(k, v as String));
        _openStreams(streams);
      case 'position':
        final secs = (event['position'] as num).toDouble();
        _position = Duration(milliseconds: (secs * 1000).round());
        notifyListeners();
      case 'state':
        if (event['playing'] != null) _isPlaying = event['playing'] as bool;
        if (event['speed'] != null) {
          _speed = (event['speed'] as num).toDouble();
        }
        if (event['position'] != null) {
          final secs = (event['position'] as num).toDouble();
          _position = Duration(milliseconds: (secs * 1000).round());
        }
        _isSeeking = false;
        notifyListeners();
      case 'stream_restart':
        final camId = event['camera_id'] as String;
        final newUrl = event['new_url'] as String;
        _reopenStream(camId, newUrl);
      case 'segment_gap':
        // Timeline already shows gaps — this is just informational
        notifyListeners();
      case 'end':
        _isPlaying = false;
        notifyListeners();
      case 'error':
        debugPrint('Playback error: ${event['message']}');
    }
  }

  void _openStreams(Map<String, String> streams) {
    final baseUrl = playbackService.streamBaseUrl();
    for (final entry in streams.entries) {
      final camId = entry.key;
      final url = baseUrl + entry.value;

      if (!_players.containsKey(camId)) {
        final player = Player();
        _players[camId] = player;
        _videoControllers[camId] = VideoController(player);
      }
      _players[camId]!.open(Media(url), play: false);
    }
    notifyListeners();
  }

  void _reopenStream(String camId, String newUrl) {
    final baseUrl = playbackService.streamBaseUrl();
    _players[camId]?.dispose();
    final player = Player();
    _players[camId] = player;
    _videoControllers[camId] = VideoController(player);
    player.open(Media(baseUrl + newUrl), play: _isPlaying);
    notifyListeners();
  }

  // ── Session lifecycle ─────────────────────────────────────────────────

  void _createSession() {
    if (_selectedCameraIds.isEmpty) return;

    final dayStart = DateTime(
        _selectedDate.year, _selectedDate.month, _selectedDate.day);
    _sendCommand({
      'cmd': 'create',
      'camera_ids': _selectedCameraIds,
      'start': PlaybackService.toLocalRfc3339(dayStart),
    });
  }

  // ── Playback controls ─────────────────────────────────────────────────

  void play() {
    _isPlaying = true;
    if (!_sessionCreated && _ws == null) {
      _connectWs();
      _createSession();
    }
    _sendCommand({'cmd': 'play'});
    notifyListeners();
  }

  void pause() {
    _isPlaying = false;
    _sendCommand({'cmd': 'pause'});
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
    _sendCommand({'cmd': 'speed', 'rate': speed});
    notifyListeners();
  }

  Future<void> seek(Duration target) async {
    final clamped = Duration(
      milliseconds:
          target.inMilliseconds.clamp(0, _maxPosition.inMilliseconds),
    );
    final secs = clamped.inMilliseconds / 1000.0;

    _isSeeking = true;
    _position = clamped;
    notifyListeners();

    _sendCommand({'cmd': 'seek', 'position': secs});
    // _isSeeking will be cleared when we receive the 'state' event back
  }

  void stepFrame(int direction) {
    if (_isPlaying) pause();
    _sendCommand({'cmd': 'step', 'direction': direction});
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

  // ── Cleanup ───────────────────────────────────────────────────────────

  void _disposeAllPlayers() {
    for (final p in _players.values) {
      p.dispose();
    }
    _players.clear();
    _videoControllers.clear();
  }

  @override
  void dispose() {
    if (_sessionCreated) {
      _sendCommand({'cmd': 'close'});
    }
    _ws?.sink.close();
    _disposeAllPlayers();
    super.dispose();
  }
}
