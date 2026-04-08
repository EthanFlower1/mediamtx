// KAI-300 — LL-HLS fallback video widget.
//
// Used when WebRTC fails (or the endpoint list has only LL-HLS entries).
// Uses `video_player` which covers iOS, Android, macOS, Web. On Linux/Windows
// `video_player` is limited — the codebase already ships `media_kit` so a
// future PR can swap the backend; the widget API surface is unchanged.
//
// Platform coverage:
//   iOS / Android / macOS / Web — video_player (HLS supported)
//   Linux / Windows             — video_player (HLS depends on native codecs;
//                                 may need media_kit swap — tracked in KAI-306)

import 'package:flutter/material.dart';
import 'package:video_player/video_player.dart';

import '../../../theme/nvr_colors.dart';

class LlhlsFallbackView extends StatefulWidget {
  /// The LL-HLS playlist URL (`.m3u8`).
  final String url;

  /// Endpoint index — forwarded to callbacks.
  final int endpointIndex;

  final void Function(int endpointIndex) onConnected;
  final void Function(int endpointIndex) onFailed;

  final bool audioMuted;

  const LlhlsFallbackView({
    super.key,
    required this.url,
    required this.endpointIndex,
    required this.onConnected,
    required this.onFailed,
    this.audioMuted = true,
  });

  @override
  State<LlhlsFallbackView> createState() => LlhlsFallbackViewState();
}

@visibleForTesting
class LlhlsFallbackViewState extends State<LlhlsFallbackView> {
  VideoPlayerController? _controller;
  bool _initialized = false;
  bool _failed = false;

  @override
  void initState() {
    super.initState();
    _init();
  }

  @override
  void didUpdateWidget(LlhlsFallbackView oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.url != widget.url) {
      _controller?.dispose();
      _controller = null;
      _initialized = false;
      _failed = false;
      _init();
    }
    if (oldWidget.audioMuted != widget.audioMuted) {
      _applyMute();
    }
  }

  Future<void> _init() async {
    try {
      final ctrl = VideoPlayerController.networkUrl(Uri.parse(widget.url));
      _controller = ctrl;
      await ctrl.initialize();
      _applyMute();
      ctrl.play();
      ctrl.addListener(_onControllerUpdate);
      if (mounted) {
        setState(() => _initialized = true);
        widget.onConnected(widget.endpointIndex);
      }
    } catch (_) {
      if (mounted) {
        setState(() => _failed = true);
        widget.onFailed(widget.endpointIndex);
      }
    }
  }

  void _onControllerUpdate() {
    final ctrl = _controller;
    if (ctrl == null) return;
    if (ctrl.value.hasError && !_failed) {
      _failed = true;
      widget.onFailed(widget.endpointIndex);
    }
  }

  void _applyMute() {
    _controller?.setVolume(widget.audioMuted ? 0.0 : 1.0);
  }

  @override
  void dispose() {
    _controller?.removeListener(_onControllerUpdate);
    _controller?.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (_failed) {
      return Center(
        child: Icon(Icons.broken_image_outlined,
            color: NvrColors.of(context).danger, size: 48),
      );
    }

    if (!_initialized || _controller == null) {
      return Center(
        child:
            CircularProgressIndicator(color: NvrColors.of(context).accent),
      );
    }

    return ClipRect(
      child: InteractiveViewer(
        minScale: 1.0,
        maxScale: 5.0,
        child: AspectRatio(
          aspectRatio: _controller!.value.aspectRatio,
          child: VideoPlayer(_controller!),
        ),
      ),
    );
  }
}
