// KAI-300 — LiveViewState: Riverpod state notifier for single-camera live view.
//
// State machine:
//   idle → requesting → connectingWebRtc → live (WebRTC)
//                                         → connectingFallback → live (LL-HLS)
//                                                               → error
//
// The notifier owns the endpoint-selection / fallback logic. Widgets observe
// [liveViewStateProvider] and render accordingly.

import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../../api/streams_api.dart';
import '../../../state/app_session.dart';

// ---------------------------------------------------------------------------
// Enums
// ---------------------------------------------------------------------------

enum LiveViewPhase {
  /// No camera selected yet.
  idle,

  /// Calling the streams API (KAI-255) to get endpoints.
  requesting,

  /// Attempting WebRTC connection to endpoint at [endpointIndex].
  connectingWebRtc,

  /// WebRTC failed; trying LL-HLS fallback.
  connectingFallback,

  /// Streaming successfully via WebRTC.
  liveWebRtc,

  /// Streaming successfully via LL-HLS fallback.
  liveFallback,

  /// All endpoints failed. Show retry UI.
  error,
}

// ---------------------------------------------------------------------------
// State value object
// ---------------------------------------------------------------------------

class LiveViewState {
  final LiveViewPhase phase;

  /// Camera being viewed. Null in idle phase.
  final String? cameraId;

  /// Camera display name for the HUD.
  final String? cameraName;

  /// Current stream request result. Populated after [requesting] completes.
  final StreamRequest? streamRequest;

  /// Which endpoint index is currently being tried / active.
  final int endpointIndex;

  /// Active endpoint (WebRTC) URL. Non-null while in [liveWebRtc].
  final String? webRtcUrl;

  /// Active fallback (LL-HLS) URL. Non-null while in [liveFallback].
  final String? fallbackUrl;

  /// Friendly error message when [phase] == [LiveViewPhase.error].
  final String? errorMessage;

  /// Whether the camera supports PTZ.
  final bool ptzCapable;

  /// Whether the talkback mic is active (hold-to-talk engaged).
  final bool talkbackActive;

  /// Whether audio output is muted.
  final bool audioMuted;

  /// Connection quality indicator label (e.g. "LAN", "Relay").
  final String? connectionLabel;

  /// Estimated latency in ms for the active endpoint.
  final int? estimatedLatencyMs;

  const LiveViewState({
    this.phase = LiveViewPhase.idle,
    this.cameraId,
    this.cameraName,
    this.streamRequest,
    this.endpointIndex = 0,
    this.webRtcUrl,
    this.fallbackUrl,
    this.errorMessage,
    this.ptzCapable = false,
    this.talkbackActive = false,
    this.audioMuted = true,
    this.connectionLabel,
    this.estimatedLatencyMs,
  });

  static const LiveViewState idle = LiveViewState();

  LiveViewState copyWith({
    LiveViewPhase? phase,
    String? cameraId,
    String? cameraName,
    StreamRequest? streamRequest,
    int? endpointIndex,
    String? webRtcUrl,
    String? fallbackUrl,
    String? errorMessage,
    bool? ptzCapable,
    bool? talkbackActive,
    bool? audioMuted,
    String? connectionLabel,
    int? estimatedLatencyMs,
    bool clearWebRtcUrl = false,
    bool clearFallbackUrl = false,
    bool clearError = false,
  }) {
    return LiveViewState(
      phase: phase ?? this.phase,
      cameraId: cameraId ?? this.cameraId,
      cameraName: cameraName ?? this.cameraName,
      streamRequest: streamRequest ?? this.streamRequest,
      endpointIndex: endpointIndex ?? this.endpointIndex,
      webRtcUrl: clearWebRtcUrl ? null : (webRtcUrl ?? this.webRtcUrl),
      fallbackUrl:
          clearFallbackUrl ? null : (fallbackUrl ?? this.fallbackUrl),
      errorMessage: clearError ? null : (errorMessage ?? this.errorMessage),
      ptzCapable: ptzCapable ?? this.ptzCapable,
      talkbackActive: talkbackActive ?? this.talkbackActive,
      audioMuted: audioMuted ?? this.audioMuted,
      connectionLabel: connectionLabel ?? this.connectionLabel,
      estimatedLatencyMs: estimatedLatencyMs ?? this.estimatedLatencyMs,
    );
  }
}

// ---------------------------------------------------------------------------
// Notifier
// ---------------------------------------------------------------------------

/// WebRTC fallback timeout. If the connection is not established within this
/// window, the notifier moves to LL-HLS fallback.
const kWebRtcFallbackTimeout = Duration(seconds: 3);

class LiveViewNotifier extends StateNotifier<LiveViewState> {
  final StreamsApi _streamsApi;
  final Ref _ref;

  LiveViewNotifier(this._streamsApi, this._ref) : super(LiveViewState.idle);

  // ── Public actions ────────────────────────────────────────────────────────

  /// Start a live view session for [cameraId].
  Future<void> start({
    required String cameraId,
    required String cameraName,
    bool ptzCapable = false,
  }) async {
    state = LiveViewState(
      phase: LiveViewPhase.requesting,
      cameraId: cameraId,
      cameraName: cameraName,
      ptzCapable: ptzCapable,
    );

    try {
      final session = _ref.read(appSessionProvider);
      final conn = session.activeConnection;
      final token = session.accessToken;

      if (conn == null || token == null) {
        _setError('Not authenticated. Please log in again.');
        return;
      }

      final request = await _streamsApi.requestStream(
        cameraId: cameraId,
        baseUrl: conn.endpointUrl,
        accessToken: token,
      );

      if (request.endpoints.isEmpty) {
        _setError('No stream endpoints available for this camera.');
        return;
      }

      state = state.copyWith(
        streamRequest: request,
        endpointIndex: 0,
      );

      await _tryEndpoint(0, request);
    } on StreamRequestException catch (e) {
      _setError('Stream request failed (${e.statusCode}). Check connectivity.');
    } catch (e) {
      _setError('Unexpected error: $e');
    }
  }

  /// Called by the WebRTC widget when the connection succeeds.
  void onWebRtcConnected(int endpointIndex) {
    final ep = state.streamRequest?.endpoints[endpointIndex];
    if (ep == null) return;
    state = state.copyWith(
      phase: LiveViewPhase.liveWebRtc,
      webRtcUrl: ep.url,
      connectionLabel: ep.connectionLabel,
      estimatedLatencyMs: ep.estimatedLatencyMs,
      clearError: true,
    );
  }

  /// Called by the WebRTC widget when the connection fails or times out.
  Future<void> onWebRtcFailed(int endpointIndex) async {
    final request = state.streamRequest;
    if (request == null) return;

    // Look for a fallback endpoint (LL-HLS) after the current one.
    final nextLlhls = request.endpoints
        .skip(endpointIndex + 1)
        .where((e) => e.transport == StreamTransport.llhls)
        .firstOrNull;

    if (nextLlhls != null) {
      final idx = request.endpoints.indexOf(nextLlhls);
      state = state.copyWith(
        phase: LiveViewPhase.connectingFallback,
        endpointIndex: idx,
      );
      await _tryFallbackEndpoint(idx, request);
    } else {
      // Try next endpoint sequentially if it exists.
      final nextIdx = endpointIndex + 1;
      if (nextIdx < request.endpoints.length) {
        await _tryEndpoint(nextIdx, request);
      } else {
        _setError('All stream endpoints failed. Check network and camera.');
      }
    }
  }

  /// Called by the LL-HLS widget when playback starts successfully.
  void onFallbackConnected(int endpointIndex) {
    final ep = state.streamRequest?.endpoints[endpointIndex];
    if (ep == null) return;
    state = state.copyWith(
      phase: LiveViewPhase.liveFallback,
      fallbackUrl: ep.url,
      connectionLabel: ep.connectionLabel,
      estimatedLatencyMs: ep.estimatedLatencyMs,
      clearError: true,
    );
  }

  /// Called by the LL-HLS widget when playback fails.
  void onFallbackFailed(int endpointIndex) {
    final request = state.streamRequest;
    if (request == null) return;
    final nextIdx = endpointIndex + 1;
    if (nextIdx < request.endpoints.length) {
      _tryEndpoint(nextIdx, request);
    } else {
      _setError('All stream endpoints failed. Check network and camera.');
    }
  }

  /// Retry from scratch.
  Future<void> retry() async {
    final cameraId = state.cameraId;
    final cameraName = state.cameraName;
    final ptz = state.ptzCapable;
    if (cameraId == null || cameraName == null) return;
    await start(
      cameraId: cameraId,
      cameraName: cameraName,
      ptzCapable: ptz,
    );
  }

  /// Toggle hold-to-talk talkback.
  void setTalkbackActive(bool active) {
    state = state.copyWith(talkbackActive: active);
  }

  /// Toggle audio output mute.
  void toggleAudioMute() {
    state = state.copyWith(audioMuted: !state.audioMuted);
  }

  /// Reset to idle (camera deselected / screen exited).
  void reset() {
    state = LiveViewState.idle;
  }

  // ── Private helpers ───────────────────────────────────────────────────────

  Future<void> _tryEndpoint(int idx, StreamRequest request) async {
    final ep = request.endpoints[idx];
    state = state.copyWith(
      endpointIndex: idx,
      phase: ep.transport == StreamTransport.webrtc
          ? LiveViewPhase.connectingWebRtc
          : LiveViewPhase.connectingFallback,
    );
    if (ep.transport == StreamTransport.llhls) {
      await _tryFallbackEndpoint(idx, request);
    }
    // For WebRTC the widget handles success/failure callbacks.
  }

  Future<void> _tryFallbackEndpoint(int idx, StreamRequest request) async {
    final ep = request.endpoints[idx];
    state = state.copyWith(
      endpointIndex: idx,
      phase: LiveViewPhase.connectingFallback,
      fallbackUrl: ep.url,
    );
    // LL-HLS widget mounts automatically when phase == connectingFallback.
  }

  void _setError(String message) {
    state = state.copyWith(
      phase: LiveViewPhase.error,
      errorMessage: message,
    );
  }
}

// ---------------------------------------------------------------------------
// Providers
// ---------------------------------------------------------------------------

/// Override in tests with a fake [StreamsApi].
final streamsApiProvider = Provider<StreamsApi>((_) => const HttpStreamsApi());

final liveViewStateProvider =
    StateNotifierProvider<LiveViewNotifier, LiveViewState>((ref) {
  return LiveViewNotifier(ref.watch(streamsApiProvider), ref);
});
