// KAI-300 — WebRTC video widget for single-camera live view.
//
// Wraps flutter_webrtc's WHEP flow. Callbacks:
//   onConnected(endpointIndex)  — WebRTC ICE + tracks established
//   onFailed(endpointIndex)     — connection could not be established within
//                                 [kWebRtcFallbackTimeout] (~3s) → triggers
//                                 LL-HLS fallback in LiveViewNotifier
//
// Platform coverage: iOS, Android, macOS, Windows, Linux, Web — all supported
// by flutter_webrtc ^0.12.0. Tested locally on macOS (CI covers Android+Web).

import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';
import 'package:dio/dio.dart';

import '../state/live_view_state.dart';
import '../../../theme/nvr_colors.dart';

class WebRtcVideoView extends StatefulWidget {
  /// The WHEP endpoint URL to connect to.
  final String url;

  /// The index of this endpoint in the ordered list; forwarded to callbacks.
  final int endpointIndex;

  final void Function(int endpointIndex) onConnected;
  final void Function(int endpointIndex) onFailed;

  /// Whether talkback (send audio) is active.
  final bool talkbackActive;

  /// Whether audio output is muted.
  final bool audioMuted;

  const WebRtcVideoView({
    super.key,
    required this.url,
    required this.endpointIndex,
    required this.onConnected,
    required this.onFailed,
    this.talkbackActive = false,
    this.audioMuted = true,
  });

  @override
  State<WebRtcVideoView> createState() => WebRtcVideoViewState();
}

@visibleForTesting
class WebRtcVideoViewState extends State<WebRtcVideoView> {
  RTCVideoRenderer? _renderer;
  RTCPeerConnection? _pc;
  MediaStream? _localAudioStream;

  Timer? _fallbackTimer;
  bool _connected = false;
  bool _disposed = false;

  @override
  void initState() {
    super.initState();
    _start();
  }

  @override
  void didUpdateWidget(WebRtcVideoView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.url != widget.url) {
      _tearDown();
      _start();
    }
    if (oldWidget.audioMuted != widget.audioMuted) {
      _applyAudioMute();
    }
    if (oldWidget.talkbackActive != widget.talkbackActive) {
      _applyTalkback();
    }
  }

  Future<void> _start() async {
    if (_disposed) return;

    _renderer = RTCVideoRenderer();
    await _renderer!.initialize();

    // Arm the 3s fallback timer — if WebRTC doesn't connect in time we bail.
    _fallbackTimer = Timer(kWebRtcFallbackTimeout, () {
      if (!_connected && !_disposed) {
        widget.onFailed(widget.endpointIndex);
      }
    });

    try {
      final config = <String, dynamic>{
        'iceServers': [],
        'sdpSemantics': 'unified-plan',
      };
      _pc = await createPeerConnection(config);

      _pc!.onConnectionState = (RTCPeerConnectionState s) {
        if (_disposed) return;
        if (s == RTCPeerConnectionState.RTCPeerConnectionStateConnected) {
          _fallbackTimer?.cancel();
          _connected = true;
          widget.onConnected(widget.endpointIndex);
        } else if (s == RTCPeerConnectionState.RTCPeerConnectionStateFailed ||
            s == RTCPeerConnectionState.RTCPeerConnectionStateDisconnected) {
          if (!_connected) {
            _fallbackTimer?.cancel();
            widget.onFailed(widget.endpointIndex);
          }
        }
      };

      _pc!.onTrack = (RTCTrackEvent event) {
        if (event.streams.isNotEmpty && _renderer != null) {
          _renderer!.srcObject = event.streams[0];
          if (mounted) setState(() {});
        }
      };

      // Recv-only video transceiver.
      await _pc!.addTransceiver(
        kind: RTCRtpMediaType.RTCRtpMediaTypeVideo,
        init: RTCRtpTransceiverInit(
          direction: TransceiverDirection.RecvOnly,
        ),
      );

      // Audio transceiver: recv-only unless talkback is active.
      await _pc!.addTransceiver(
        kind: RTCRtpMediaType.RTCRtpMediaTypeAudio,
        init: RTCRtpTransceiverInit(
          direction: widget.talkbackActive
              ? TransceiverDirection.SendRecv
              : TransceiverDirection.RecvOnly,
        ),
      );

      if (widget.talkbackActive) {
        _localAudioStream = await _getUserAudio();
        if (_localAudioStream != null) {
          for (final track in _localAudioStream!.getAudioTracks()) {
            await _pc!.addTrack(track, _localAudioStream!);
          }
        }
      }

      final offer = await _pc!.createOffer();
      await _pc!.setLocalDescription(offer);
      await _waitForIceGathering();

      final localDesc = await _pc!.getLocalDescription();
      if (localDesc == null) throw Exception('No local description after ICE');

      final dio = Dio();
      final response = await dio.post<String>(
        widget.url,
        data: localDesc.sdp,
        options: Options(
          headers: {'Content-Type': 'application/sdp'},
          responseType: ResponseType.plain,
          sendTimeout: const Duration(seconds: 5),
          receiveTimeout: const Duration(seconds: 5),
        ),
      );

      if ((response.statusCode ?? 0) < 200 ||
          (response.statusCode ?? 0) >= 300) {
        throw Exception('WHEP returned ${response.statusCode}');
      }

      final sdpAnswer = response.data ?? '';
      await _pc!.setRemoteDescription(RTCSessionDescription(sdpAnswer, 'answer'));
      _applyAudioMute();
    } catch (_) {
      if (!_disposed && !_connected) {
        _fallbackTimer?.cancel();
        widget.onFailed(widget.endpointIndex);
      }
    }
  }

  Future<void> _waitForIceGathering() async {
    if (_pc?.iceGatheringState ==
        RTCIceGatheringState.RTCIceGatheringStateComplete) {
      return;
    }
    final c = Completer<void>();
    _pc!.onIceGatheringState = (s) {
      if (s == RTCIceGatheringState.RTCIceGatheringStateComplete &&
          !c.isCompleted) {
        c.complete();
      }
    };
    await Future.any([c.future, Future.delayed(const Duration(seconds: 5))]);
    _pc?.onIceGatheringState = null;
  }

  Future<MediaStream?> _getUserAudio() async {
    try {
      return await navigator.mediaDevices
          .getUserMedia({'audio': true, 'video': false});
    } catch (_) {
      return null;
    }
  }

  void _applyAudioMute() {
    _pc?.getReceivers().then((receivers) {
      for (final r in receivers) {
        if (r.track?.kind == 'audio') {
          r.track?.enabled = !widget.audioMuted;
        }
      }
    });
  }

  void _applyTalkback() {
    _localAudioStream?.getAudioTracks().forEach((t) {
      t.enabled = widget.talkbackActive;
    });
  }

  void _tearDown() {
    _fallbackTimer?.cancel();
    _pc?.close();
    _pc = null;
    _renderer?.srcObject = null;
    _renderer?.dispose();
    _renderer = null;
    _localAudioStream?.dispose();
    _localAudioStream = null;
    _connected = false;
  }

  @override
  void dispose() {
    _disposed = true;
    _tearDown();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final renderer = _renderer;
    if (renderer == null) {
      return Center(
        child: CircularProgressIndicator(color: NvrColors.of(context).accent),
      );
    }
    return ClipRect(
      child: InteractiveViewer(
        minScale: 1.0,
        maxScale: 5.0,
        child: RTCVideoView(
          renderer,
          objectFit: RTCVideoViewObjectFit.RTCVideoViewObjectFitContain,
        ),
      ),
    );
  }
}
