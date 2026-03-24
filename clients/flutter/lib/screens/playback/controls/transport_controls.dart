import 'package:flutter/material.dart';
import '../../../theme/nvr_colors.dart';

class TransportControls extends StatelessWidget {
  final bool isPlaying;
  final VoidCallback onPlayPause;
  final VoidCallback onStepForward;
  final VoidCallback onStepBackward;
  final VoidCallback onNextEvent;
  final VoidCallback onPreviousEvent;
  final VoidCallback onNextGap;
  final VoidCallback onPreviousGap;

  const TransportControls({
    super.key,
    required this.isPlaying,
    required this.onPlayPause,
    required this.onStepForward,
    required this.onStepBackward,
    required this.onNextEvent,
    required this.onPreviousEvent,
    required this.onNextGap,
    required this.onPreviousGap,
  });

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.center,
      children: [
        _TransportButton(
          icon: Icons.skip_previous,
          tooltip: 'Previous recording',
          onPressed: onPreviousGap,
        ),
        _TransportButton(
          icon: Icons.arrow_back,
          tooltip: 'Previous event',
          onPressed: onPreviousEvent,
        ),
        _TransportButton(
          icon: Icons.chevron_left,
          tooltip: 'Frame back',
          onPressed: onStepBackward,
        ),
        _TransportButton(
          icon: isPlaying ? Icons.pause : Icons.play_arrow,
          tooltip: isPlaying ? 'Pause' : 'Play',
          onPressed: onPlayPause,
          size: 40,
          iconSize: 28,
        ),
        _TransportButton(
          icon: Icons.chevron_right,
          tooltip: 'Frame forward',
          onPressed: onStepForward,
        ),
        _TransportButton(
          icon: Icons.arrow_forward,
          tooltip: 'Next event',
          onPressed: onNextEvent,
        ),
        _TransportButton(
          icon: Icons.skip_next,
          tooltip: 'Next recording',
          onPressed: onNextGap,
        ),
      ],
    );
  }
}

class _TransportButton extends StatelessWidget {
  final IconData icon;
  final String tooltip;
  final VoidCallback onPressed;
  final double size;
  final double iconSize;

  const _TransportButton({
    required this.icon,
    required this.tooltip,
    required this.onPressed,
    this.size = 36,
    this.iconSize = 20,
  });

  @override
  Widget build(BuildContext context) {
    return Tooltip(
      message: tooltip,
      child: SizedBox(
        width: size,
        height: size,
        child: IconButton(
          padding: EdgeInsets.zero,
          icon: Icon(icon, size: iconSize, color: NvrColors.textPrimary),
          onPressed: onPressed,
        ),
      ),
    );
  }
}
