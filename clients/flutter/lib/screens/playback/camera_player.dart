import 'package:flutter/material.dart';
import 'package:video_player/video_player.dart';

import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/corner_brackets.dart';
import 'playback_controller.dart';
import '../../models/detection_frame.dart';
import 'playback_detection_overlay.dart';

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
  PlaybackController get _ctrl => widget.controller;
  String get _camId => widget.cameraId;

  String _formatTimestamp(Duration d) {
    final h = d.inHours.remainder(24).toString().padLeft(2, '0');
    final m = d.inMinutes.remainder(60).toString().padLeft(2, '0');
    final s = d.inSeconds.remainder(60).toString().padLeft(2, '0');
    return '$h:$m:$s';
  }

  @override
  Widget build(BuildContext context) {
    final vc = _ctrl.videoControllers[_camId];
    final isInGap = _ctrl.isInGap;

    return Container(
      decoration: BoxDecoration(
        color: NvrColors.bgSecondary,
        border: Border.all(color: NvrColors.border),
        borderRadius: BorderRadius.circular(4),
      ),
      clipBehavior: Clip.antiAlias,
      child: CornerBrackets(
        child: Stack(
          fit: StackFit.expand,
          children: [
            // Video content
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
                      Icon(Icons.videocam_off,
                          color: NvrColors.textMuted, size: 36),
                      SizedBox(height: 8),
                      Text(
                        'No video available',
                        style: TextStyle(
                          color: NvrColors.textMuted,
                          fontSize: 12,
                          fontWeight: FontWeight.w500,
                        ),
                      ),
                    ],
                  ),
                ),
              ),

            // Top-left: camera name
            Positioned(
              top: 10,
              left: 10,
              child: Container(
                padding:
                    const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
                decoration: BoxDecoration(
                  color: Colors.black54,
                  borderRadius: BorderRadius.circular(3),
                ),
                child: Text(
                  widget.cameraName,
                  style: const TextStyle(
                    fontFamily: 'IBMPlexSans',
                    fontSize: 12,
                    fontWeight: FontWeight.w500,
                    color: NvrColors.textPrimary,
                  ),
                ),
              ),
            ),

            // Top-right: playback timestamp
            if (!isInGap)
              Positioned(
                top: 10,
                right: 10,
                child: Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
                  decoration: BoxDecoration(
                    color: Colors.black54,
                    borderRadius: BorderRadius.circular(3),
                  ),
                  child: Text(
                    _formatTimestamp(_ctrl.position),
                    style: NvrTypography.monoTimestamp.copyWith(fontSize: 11),
                  ),
                ),
              ),

            // Top-right: speed indicator (when not 1x)
            if (!isInGap && _ctrl.speed != 1.0)
              Positioned(
                top: 10,
                right: 80,
                child: Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 6, vertical: 3),
                  decoration: BoxDecoration(
                    color: NvrColors.accent.withValues(alpha: 0.8),
                    borderRadius: BorderRadius.circular(3),
                  ),
                  child: Text(
                    '${_ctrl.speed}×',
                    style: const TextStyle(
                      fontFamily: 'JetBrainsMono',
                      fontSize: 10,
                      fontWeight: FontWeight.w600,
                      color: Colors.white,
                    ),
                  ),
                ),
              ),

            // Bottom-left: detection event info pill
            if (!isInGap) _buildEventPill(),

            // Bottom-right: per-camera controls
            if (!isInGap)
              Positioned(
                bottom: 10,
                right: 10,
                child: Row(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    if (_ctrl.hasDetectionsForCamera(_camId))
                      Padding(
                        padding: const EdgeInsets.only(right: 4),
                        child: _TileButton(
                          icon: _ctrl.isOverlayDisabled(_camId)
                              ? Icons.visibility_off
                              : Icons.visibility,
                          tooltip: _ctrl.isOverlayDisabled(_camId)
                              ? 'Show detections'
                              : 'Hide detections',
                          onPressed: () => _ctrl.toggleOverlay(_camId),
                        ),
                      ),
                    _TileButton(
                      icon: _ctrl.isCameraMuted(_camId)
                          ? Icons.volume_off
                          : Icons.volume_up,
                      tooltip: _ctrl.isCameraMuted(_camId)
                          ? 'Unmute'
                          : 'Mute',
                      onPressed: () => _ctrl.toggleMute(_camId),
                    ),
                  ],
                ),
              ),
          ],
        ),
      ),
    );
  }

  /// Build a detection event info pill at bottom-left if there's an active
  /// event at the current position.
  Widget _buildEventPill() {
    // Find an active event for this camera at the current position.
    final now = DateTime(
      _ctrl.selectedDate.year,
      _ctrl.selectedDate.month,
      _ctrl.selectedDate.day,
    ).add(_ctrl.position);

    String? eventLabel;
    for (final evt in _ctrl.events) {
      final evtEnd = evt.endTime ?? evt.startTime.add(const Duration(seconds: 5));
      if (evt.cameraId == _camId &&
          !now.isBefore(evt.startTime) &&
          !now.isAfter(evtEnd)) {
        eventLabel = evt.eventType ?? 'Motion Detected';
        break;
      }
    }

    if (eventLabel == null) return const SizedBox.shrink();

    return Positioned(
      bottom: 10,
      left: 10,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        decoration: BoxDecoration(
          color: NvrColors.accent.withValues(alpha: 0.07),
          borderRadius: BorderRadius.circular(4),
          border: Border.all(
            color: NvrColors.accent.withValues(alpha: 0.2),
          ),
        ),
        child: Text(
          eventLabel,
          style: const TextStyle(
            fontFamily: 'JetBrainsMono',
            fontSize: 10,
            fontWeight: FontWeight.w500,
            color: NvrColors.accent,
          ),
        ),
      ),
    );
  }

  Widget _buildVideoContent(VideoPlayerController? vc, bool isInGap) {
    if (isInGap) return const SizedBox.expand();
    if (vc == null || !vc.value.isInitialized) {
      // If the controller has segments loaded but this camera has no
      // coverage at the current position, show a "no recording" state
      // instead of an infinite spinner.
      final hasSegments = _ctrl.segments.isNotEmpty;
      final hasCoverage = _ctrl.hasCoverageAtPosition(_camId);
      if (hasSegments && !hasCoverage && !_ctrl.isSeeking) {
        return const Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.videocam_off,
                  color: NvrColors.textMuted, size: 32),
              SizedBox(height: 6),
              Text(
                'No recording at this time',
                style: TextStyle(
                  color: NvrColors.textMuted,
                  fontSize: 11,
                  fontWeight: FontWeight.w500,
                ),
              ),
            ],
          ),
        );
      }
      return const Center(
        child: CircularProgressIndicator(color: NvrColors.accent),
      );
    }

    // Get detections for the current playback time.
    final dayStart = DateTime(
      _ctrl.selectedDate.year,
      _ctrl.selectedDate.month,
      _ctrl.selectedDate.day,
    );
    final currentTime = dayStart.add(_ctrl.position);
    final showOverlay = !_ctrl.isOverlayDisabled(_camId);
    final detections = showOverlay
        ? _ctrl.getDetectionsAtTime(_camId, currentTime)
        : <DetectionBox>[];

    return FittedBox(
      fit: BoxFit.contain,
      child: SizedBox(
        width: vc.value.size.width,
        height: vc.value.size.height,
        child: Stack(
          children: [
            VideoPlayer(vc),
            if (detections.isNotEmpty)
              PlaybackDetectionOverlay(detections: detections),
          ],
        ),
      ),
    );
  }
}

// ── Tile Button ──────────────────────────────────────────────────────────────

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
