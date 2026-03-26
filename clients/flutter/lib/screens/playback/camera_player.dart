import 'package:flutter/material.dart';
import 'package:video_player/video_player.dart';

import '../../theme/nvr_colors.dart';
import 'playback_controller.dart';

class CameraPlayer extends StatefulWidget {
  final String cameraId;
  final String cameraName;
  final PlaybackController controller;

  const CameraPlayer({
    super.key,
    required this.cameraId,
    required this.cameraName,
    required this.controller,
  });

  @override
  State<CameraPlayer> createState() => _CameraPlayerState();
}

class _CameraPlayerState extends State<CameraPlayer> {
  bool _showAnalyticsOverlay = false;

  PlaybackController get _ctrl => widget.controller;
  String get _camId => widget.cameraId;

  @override
  Widget build(BuildContext context) {
    final vc = _ctrl.videoControllers[_camId];
    final isInGap = _ctrl.isInGap;
    final isMuted = _ctrl.isCameraMuted(_camId);

    return Stack(
      fit: StackFit.expand,
      children: [
        ColoredBox(
          color: Colors.black,
          child: _buildVideoContent(vc, isInGap),
        ),
        // "No video" overlay when in gap
        if (isInGap)
          Container(
            color: Colors.black87,
            child: const Center(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(Icons.videocam_off, color: NvrColors.textMuted, size: 40),
                  SizedBox(height: 8),
                  Text(
                    'No video available',
                    style: TextStyle(
                      color: NvrColors.textMuted,
                      fontSize: 14,
                      fontWeight: FontWeight.w500,
                    ),
                  ),
                ],
              ),
            ),
          ),
        // Analytics overlay indicator
        if (_showAnalyticsOverlay && !isInGap)
          Positioned(
            top: 8,
            right: 8,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
              decoration: BoxDecoration(
                color: NvrColors.accent.withValues(alpha: 0.7),
                borderRadius: BorderRadius.circular(4),
              ),
              child: const Row(
                mainAxisSize: MainAxisSize.min,
                children: [
                  Icon(Icons.analytics, color: Colors.white, size: 12),
                  SizedBox(width: 4),
                  Text(
                    'AI',
                    style: TextStyle(
                      color: Colors.white,
                      fontSize: 10,
                      fontWeight: FontWeight.bold,
                    ),
                  ),
                ],
              ),
            ),
          ),
        // Camera name badge
        Positioned(
          top: 8,
          left: 8,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
            decoration: BoxDecoration(
              color: Colors.black54,
              borderRadius: BorderRadius.circular(4),
            ),
            child: Text(
              widget.cameraName,
              style: const TextStyle(
                color: NvrColors.textPrimary,
                fontSize: 12,
                fontWeight: FontWeight.w500,
              ),
            ),
          ),
        ),
        // Per-camera controls (bottom-right)
        if (!isInGap)
          Positioned(
            bottom: 6,
            right: 6,
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                _TileButton(
                  icon: isMuted ? Icons.volume_off : Icons.volume_up,
                  tooltip: isMuted ? 'Unmute' : 'Mute',
                  onPressed: () => _ctrl.toggleMute(_camId),
                ),
                const SizedBox(width: 4),
                _TileButton(
                  icon: Icons.analytics,
                  tooltip: _showAnalyticsOverlay
                      ? 'Hide AI overlay'
                      : 'Show AI overlay',
                  active: _showAnalyticsOverlay,
                  onPressed: () => setState(() {
                    _showAnalyticsOverlay = !_showAnalyticsOverlay;
                  }),
                ),
              ],
            ),
          ),
      ],
    );
  }

  Widget _buildVideoContent(VideoPlayerController? vc, bool isInGap) {
    if (isInGap) return const SizedBox.expand();
    if (vc == null || !vc.value.isInitialized) {
      return const Center(
        child: CircularProgressIndicator(color: NvrColors.accent),
      );
    }
    return FittedBox(
      fit: BoxFit.contain,
      child: SizedBox(
        width: vc.value.size.width,
        height: vc.value.size.height,
        child: VideoPlayer(vc),
      ),
    );
  }
}

class _TileButton extends StatelessWidget {
  final IconData icon;
  final String tooltip;
  final VoidCallback onPressed;
  final bool active;

  const _TileButton({
    required this.icon,
    required this.tooltip,
    required this.onPressed,
    this.active = false,
  });

  @override
  Widget build(BuildContext context) {
    return Tooltip(
      message: tooltip,
      child: GestureDetector(
        onTap: onPressed,
        child: Container(
          width: 28,
          height: 28,
          decoration: BoxDecoration(
            color: Colors.black54,
            borderRadius: BorderRadius.circular(4),
          ),
          child: Icon(
            icon,
            size: 16,
            color: active ? NvrColors.accent : NvrColors.textSecondary,
          ),
        ),
      ),
    );
  }
}
