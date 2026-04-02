import 'dart:async';
import 'package:media_kit/media_kit.dart';
import 'package:media_kit_video/media_kit_video.dart';

enum RtspConnectionState {
  connecting,
  connected,
  failed,
  disposed,
}

class RtspConnection {
  final String serverUrl;
  final String mediamtxPath;

  Player? _player;
  VideoController? _videoController;
  final _stateController = StreamController<RtspConnectionState>.broadcast();
  RtspConnectionState _state = RtspConnectionState.connecting;
  bool _disposed = false;
  StreamSubscription<bool>? _playingSub;
  StreamSubscription<String>? _errorSub;

  RtspConnection({
    required this.serverUrl,
    required this.mediamtxPath,
  });

  Stream<RtspConnectionState> get stateStream => _stateController.stream;
  RtspConnectionState get state => _state;
  VideoController? get videoController => _videoController;

  void _setState(RtspConnectionState newState) {
    if (_disposed) return;
    _state = newState;
    _stateController.add(newState);
  }

  Future<void> connect() async {
    if (_disposed) return;
    _setState(RtspConnectionState.connecting);

    try {
      _player = Player();
      _videoController = VideoController(_player!);

      _playingSub = _player!.stream.playing.listen((playing) {
        if (_disposed) return;
        if (playing && _state != RtspConnectionState.connected) {
          _setState(RtspConnectionState.connected);
        }
      });

      _errorSub = _player!.stream.error.listen((error) {
        if (_disposed) return;
        if (error.isNotEmpty) {
          _setState(RtspConnectionState.failed);
        }
      });

      final serverUri = Uri.parse(serverUrl);
      final rtspUrl = 'rtsp://${serverUri.host}:8554/$mediamtxPath';

      await _player!.open(
        Media(rtspUrl),
        play: true,
      );
    } catch (e) {
      if (!_disposed) {
        _setState(RtspConnectionState.failed);
      }
    }
  }

  Future<void> setAudioEnabled(bool enabled) async {
    if (_player == null) return;
    await _player!.setVolume(enabled ? 100.0 : 0.0);
  }

  Future<void> retry() async {
    await _disposePlayer();
    await connect();
  }

  Future<void> _disposePlayer() async {
    _playingSub?.cancel();
    _playingSub = null;
    _errorSub?.cancel();
    _errorSub = null;
    await _player?.dispose();
    _player = null;
    _videoController = null;
  }

  Future<void> dispose() async {
    _disposed = true;
    _setState(RtspConnectionState.disposed);
    await _disposePlayer();
    await _stateController.close();
  }
}
