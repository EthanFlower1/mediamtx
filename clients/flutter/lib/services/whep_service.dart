import 'dart:async';
import 'package:dio/dio.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';

enum WhepConnectionState {
  connecting,
  connected,
  failed,
  disposed,
}

class WhepConnection {
  final String serverUrl;
  final String mediamtxPath;

  RTCPeerConnection? _pc;
  RTCVideoRenderer? _renderer;
  final _stateController = StreamController<WhepConnectionState>.broadcast();
  WhepConnectionState _state = WhepConnectionState.connecting;

  int _retryCount = 0;
  static const int _maxRetries = 5;
  static const int _baseRetryDelaySeconds = 3;

  Timer? _retryTimer;
  bool _disposed = false;

  WhepConnection({
    required this.serverUrl,
    required this.mediamtxPath,
  });

  Stream<WhepConnectionState> get stateStream => _stateController.stream;
  WhepConnectionState get state => _state;

  RTCVideoRenderer? get renderer => _renderer;

  void _setState(WhepConnectionState newState) {
    if (_disposed) return;
    _state = newState;
    _stateController.add(newState);
  }

  Future<void> connect() async {
    if (_disposed) return;
    _setState(WhepConnectionState.connecting);

    try {
      // Initialize renderer
      _renderer ??= RTCVideoRenderer();
      await _renderer!.initialize();

      // Close any existing peer connection
      await _pc?.close();
      _pc = null;

      // Create RTCPeerConnection
      final configuration = <String, dynamic>{
        'iceServers': [],
        'sdpSemantics': 'unified-plan',
      };

      _pc = await createPeerConnection(configuration);

      // Monitor connection state
      _pc!.onConnectionState = (RTCPeerConnectionState state) {
        if (_disposed) return;
        switch (state) {
          case RTCPeerConnectionState.RTCPeerConnectionStateConnected:
            _retryCount = 0;
            _setState(WhepConnectionState.connected);
            break;
          case RTCPeerConnectionState.RTCPeerConnectionStateFailed:
          case RTCPeerConnectionState.RTCPeerConnectionStateDisconnected:
            _scheduleRetry();
            break;
          default:
            break;
        }
      };

      // Attach remote stream to renderer
      _pc!.onTrack = (RTCTrackEvent event) {
        if (event.streams.isNotEmpty) {
          _renderer!.srcObject = event.streams[0];
        }
      };

      // Add recvonly transceivers for video and audio
      await _pc!.addTransceiver(
        kind: RTCRtpMediaType.RTCRtpMediaTypeVideo,
        init: RTCRtpTransceiverInit(
          direction: TransceiverDirection.RecvOnly,
        ),
      );
      await _pc!.addTransceiver(
        kind: RTCRtpMediaType.RTCRtpMediaTypeAudio,
        init: RTCRtpTransceiverInit(
          direction: TransceiverDirection.RecvOnly,
        ),
      );

      // Create SDP offer
      final offer = await _pc!.createOffer();
      await _pc!.setLocalDescription(offer);

      // Wait for ICE gathering to complete (with 5s timeout)
      await _waitForIceGathering();

      // Get the updated local description after ICE gathering
      final localDescription = await _pc!.getLocalDescription();
      if (localDescription == null) {
        throw Exception('Failed to get local description after ICE gathering');
      }

      // POST to WHEP endpoint
      final whepUrl = '$serverUrl:8889/$mediamtxPath/whep';
      final dio = Dio();
      final response = await dio.post<String>(
        whepUrl,
        data: localDescription.sdp,
        options: Options(
          headers: {'Content-Type': 'application/sdp'},
          responseType: ResponseType.plain,
        ),
      );

      if (response.statusCode != 201 && response.statusCode != 200) {
        throw Exception(
          'WHEP endpoint returned status ${response.statusCode}',
        );
      }

      final answerSdp = response.data;
      if (answerSdp == null || answerSdp.isEmpty) {
        throw Exception('WHEP endpoint returned empty SDP answer');
      }

      // Set remote description with the SDP answer
      final answer = RTCSessionDescription(answerSdp, 'answer');
      await _pc!.setRemoteDescription(answer);
    } catch (e) {
      if (!_disposed) {
        _scheduleRetry();
      }
    }
  }

  Future<void> _waitForIceGathering() async {
    const timeout = Duration(seconds: 5);
    final completer = Completer<void>();

    // Check if already complete
    if (_pc?.iceGatheringState ==
        RTCIceGatheringState.RTCIceGatheringStateComplete) {
      return;
    }

    Timer? timeoutTimer;

    void onGatheringStateChange(RTCIceGatheringState state) {
      if (state == RTCIceGatheringState.RTCIceGatheringStateComplete) {
        if (!completer.isCompleted) {
          completer.complete();
        }
      }
    }

    _pc!.onIceGatheringState = onGatheringStateChange;

    timeoutTimer = Timer(timeout, () {
      if (!completer.isCompleted) {
        completer.complete(); // Proceed with whatever ICE candidates we have
      }
    });

    await completer.future;
    timeoutTimer.cancel();
    _pc!.onIceGatheringState = null;
  }

  void _scheduleRetry() {
    if (_disposed || _retryCount >= _maxRetries) {
      _setState(WhepConnectionState.failed);
      return;
    }

    _setState(WhepConnectionState.failed);
    _retryTimer?.cancel();

    final delaySeconds = _calculateBackoffDelay(_retryCount);
    _retryCount++;

    _retryTimer = Timer(Duration(seconds: delaySeconds), () {
      if (!_disposed) {
        connect();
      }
    });
  }

  int _calculateBackoffDelay(int retryCount) {
    final delay = _baseRetryDelaySeconds * (1 << retryCount); // 3, 6, 12, 24...
    return delay > 30 ? 30 : delay;
  }

  /// Manual retry — resets the retry counter.
  Future<void> retry() async {
    _retryTimer?.cancel();
    _retryCount = 0;
    await connect();
  }

  Future<void> dispose() async {
    _disposed = true;
    _retryTimer?.cancel();
    _retryTimer = null;

    _setState(WhepConnectionState.disposed);

    await _pc?.close();
    _pc = null;

    _renderer?.srcObject = null;
    await _renderer?.dispose();
    _renderer = null;

    await _stateController.close();
  }
}
