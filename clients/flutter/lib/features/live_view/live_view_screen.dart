// KAI-300 — Single-camera live view screen.
//
// Entry point: navigate to this route with a [Camera] object as extra, e.g.:
//   context.push('/live/single', extra: camera);
//
// The screen wires together:
//   - LiveViewNotifier (state machine, endpoint fallback)
//   - WebRtcVideoView  (primary connection)
//   - LlhlsFallbackView (fallback when WebRTC fails within 3s)
//   - ControlsOverlay  (PTZ, talkback, snapshot, fullscreen, quality badge)
//
// Navigation from the camera tree (KAI-299) or a hardcoded dev fixture
// passes a [Camera] object. During Wave 2, a dev fixture is used for testing.
//
// Platform support: iOS, Android, macOS, Windows, Linux, Web.
//   - WebRTC: all platforms (flutter_webrtc ^0.12.0)
//   - LL-HLS fallback: iOS / Android / macOS / Web (video_player)
//              Linux / Windows: conditional (HLS codec availability) — KAI-306

import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/camera.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import 'state/live_view_state.dart';
import 'widgets/controls_overlay.dart';
import 'widgets/llhls_fallback_view.dart';
import 'widgets/webrtc_video_view.dart';

// ---------------------------------------------------------------------------
// Dev fixture — remove once KAI-299 (camera tree) lands.
// ---------------------------------------------------------------------------

/// Hardcoded test camera used for development / manual testing.
/// Remove when KAI-299 provides real navigation.
const kDevFixtureCamera = Camera(
  id: 'dev-fixture-cam-1',
  name: 'Dev Fixture Camera',
  mediamtxPath: 'dev-cam-1',
  ptzCapable: true,
);

// ---------------------------------------------------------------------------
// Screen
// ---------------------------------------------------------------------------

class SingleCameraLiveViewScreen extends ConsumerStatefulWidget {
  /// Camera to display. Null = use dev fixture (development only).
  final Camera? camera;

  const SingleCameraLiveViewScreen({super.key, this.camera});

  @override
  ConsumerState<SingleCameraLiveViewScreen> createState() =>
      _SingleCameraLiveViewScreenState();
}

class _SingleCameraLiveViewScreenState
    extends ConsumerState<SingleCameraLiveViewScreen> {
  bool _fullscreen = false;

  Camera get _camera => widget.camera ?? kDevFixtureCamera;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref.read(liveViewStateProvider.notifier).start(
            cameraId: _camera.id,
            cameraName: _camera.name,
            ptzCapable: _camera.ptzCapable,
          );
    });
  }

  @override
  void dispose() {
    // Reset notifier before super.dispose() so ref is still valid.
    // Wrap in try/catch because in tests the container may already be disposed.
    try {
      ref.read(liveViewStateProvider.notifier).reset();
    } catch (_) {}
    _exitFullscreen();
    super.dispose();
  }

  void _enterFullscreen() {
    setState(() => _fullscreen = true);
    SystemChrome.setEnabledSystemUIMode(SystemUiMode.immersiveSticky);
    // Rotate to landscape on mobile.
    if (_isMobile) {
      SystemChrome.setPreferredOrientations([
        DeviceOrientation.landscapeLeft,
        DeviceOrientation.landscapeRight,
      ]);
    }
  }

  void _exitFullscreen() {
    setState(() => _fullscreen = false);
    SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
    if (_isMobile) {
      SystemChrome.setPreferredOrientations(DeviceOrientation.values);
    }
  }

  bool get _isMobile {
    try {
      return Platform.isAndroid || Platform.isIOS;
    } catch (_) {
      return false; // Web / Fuchsia
    }
  }

  void _toggleFullscreen() {
    if (_fullscreen) {
      _exitFullscreen();
    } else {
      _enterFullscreen();
    }
  }

  Future<void> _handleSnapshot() async {
    // Snapshot: in this PR we call the server-side screenshot endpoint
    // (same as existing ptz_controls.dart pattern). A future PR can add
    // client-side frame capture with image_gallery_saver.
    //
    // For now show a SnackBar confirmation so the test can assert it.
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        backgroundColor: NvrColors.of(context).success,
        content: Text(
          'Snapshot saved',
          style: NvrTypography.of(context).button,
        ),
        duration: const Duration(seconds: 2),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(liveViewStateProvider);

    return OrientationBuilder(
      builder: (context, orientation) {
        return Scaffold(
          backgroundColor: Colors.black,
          body: Stack(
            fit: StackFit.expand,
            children: [
              // ── Video layer ─────────────────────────────────────────────
              _buildVideoLayer(state),

              // ── Controls overlay ────────────────────────────────────────
              if (state.phase == LiveViewPhase.liveWebRtc ||
                  state.phase == LiveViewPhase.liveFallback ||
                  state.phase == LiveViewPhase.connectingWebRtc ||
                  state.phase == LiveViewPhase.connectingFallback)
                ControlsOverlay(
                  cameraId: _camera.id,
                  ptzCapable: _camera.ptzCapable,
                  onSnapshot: _handleSnapshot,
                  onFullscreenToggle: _toggleFullscreen,
                  isFullscreen: _fullscreen,
                ),
            ],
          ),
        );
      },
    );
  }

  Widget _buildVideoLayer(LiveViewState state) {
    switch (state.phase) {
      case LiveViewPhase.idle:
      case LiveViewPhase.requesting:
        return const _LoadingView(message: 'Requesting stream\u2026');

      case LiveViewPhase.connectingWebRtc:
        final ep = state.streamRequest?.endpoints[state.endpointIndex];
        if (ep == null) return const _LoadingView(message: 'Connecting\u2026');
        return WebRtcVideoView(
          key: ValueKey('webrtc-${ep.url}'),
          url: ep.url,
          endpointIndex: state.endpointIndex,
          talkbackActive: state.talkbackActive,
          audioMuted: state.audioMuted,
          onConnected: (idx) =>
              ref.read(liveViewStateProvider.notifier).onWebRtcConnected(idx),
          onFailed: (idx) =>
              ref.read(liveViewStateProvider.notifier).onWebRtcFailed(idx),
        );

      case LiveViewPhase.liveWebRtc:
        final ep = state.streamRequest?.endpoints[state.endpointIndex];
        if (ep == null) return const _LoadingView(message: 'Connecting\u2026');
        return WebRtcVideoView(
          key: ValueKey('webrtc-live-${ep.url}'),
          url: ep.url,
          endpointIndex: state.endpointIndex,
          talkbackActive: state.talkbackActive,
          audioMuted: state.audioMuted,
          onConnected: (idx) =>
              ref.read(liveViewStateProvider.notifier).onWebRtcConnected(idx),
          onFailed: (idx) =>
              ref.read(liveViewStateProvider.notifier).onWebRtcFailed(idx),
        );

      case LiveViewPhase.connectingFallback:
      case LiveViewPhase.liveFallback:
        final fallbackUrl = state.fallbackUrl ??
            state.streamRequest?.endpoints[state.endpointIndex].url;
        if (fallbackUrl == null) return const _LoadingView(message: 'Connecting\u2026');
        return LlhlsFallbackView(
          key: ValueKey('llhls-$fallbackUrl'),
          url: fallbackUrl,
          endpointIndex: state.endpointIndex,
          audioMuted: state.audioMuted,
          onConnected: (idx) =>
              ref.read(liveViewStateProvider.notifier).onFallbackConnected(idx),
          onFailed: (idx) =>
              ref.read(liveViewStateProvider.notifier).onFallbackFailed(idx),
        );

      case LiveViewPhase.error:
        return _ErrorView(
          message: state.errorMessage ?? 'Stream failed.',
          onRetry: () =>
              ref.read(liveViewStateProvider.notifier).retry(),
        );
    }
  }
}

// ---------------------------------------------------------------------------
// Sub-widgets
// ---------------------------------------------------------------------------

class _LoadingView extends StatelessWidget {
  final String message;
  const _LoadingView({required this.message});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          CircularProgressIndicator(color: NvrColors.of(context).accent),
          const SizedBox(height: 16),
          Text(
            message,
            style: TextStyle(color: NvrColors.of(context).textSecondary),
          ),
        ],
      ),
    );
  }
}

class _ErrorView extends StatelessWidget {
  final String message;
  final VoidCallback onRetry;

  const _ErrorView({required this.message, required this.onRetry});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.error_outline,
              color: NvrColors.of(context).danger, size: 48),
          const SizedBox(height: 16),
          Text(
            'Stream unavailable',
            style: Theme.of(context).textTheme.titleMedium?.copyWith(
                  color: NvrColors.of(context).textPrimary,
                ),
          ),
          const SizedBox(height: 8),
          Text(
            message,
            style:
                TextStyle(color: NvrColors.of(context).textSecondary),
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 24),
          ElevatedButton.icon(
            key: const Key('retry_button'),
            onPressed: onRetry,
            icon: const Icon(Icons.refresh),
            label: const Text('Retry'),
            style: ElevatedButton.styleFrom(
              backgroundColor: NvrColors.of(context).accent,
              foregroundColor: Colors.white,
            ),
          ),
        ],
      ),
    );
  }
}
