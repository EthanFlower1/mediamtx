import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

const _speeds = [0.5, 1.0, 1.5, 2.0, 4.0, 8.0];

class PlaybackControls extends StatelessWidget {
  final bool playing;
  final double speed;
  final VoidCallback onPlayPause;
  final VoidCallback onBack10;
  final VoidCallback onForward10;
  final ValueChanged<double> onSpeedChange;

  const PlaybackControls({
    super.key,
    required this.playing,
    required this.speed,
    required this.onPlayPause,
    required this.onBack10,
    required this.onForward10,
    required this.onSpeedChange,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        color: NvrColors.bgSecondary,
        border: Border(top: BorderSide(color: NvrColors.border)),
      ),
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      child: Row(
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          // Back 10s
          IconButton(
            tooltip: 'Back 10s',
            onPressed: onBack10,
            icon: const Icon(Icons.replay_10, color: NvrColors.textPrimary),
          ),
          // Play / Pause
          IconButton(
            tooltip: playing ? 'Pause' : 'Play',
            onPressed: onPlayPause,
            iconSize: 36,
            icon: Icon(
              playing ? Icons.pause_circle_filled : Icons.play_circle_filled,
              color: NvrColors.accent,
            ),
          ),
          // Forward 10s
          IconButton(
            tooltip: 'Forward 10s',
            onPressed: onForward10,
            icon: const Icon(Icons.forward_10, color: NvrColors.textPrimary),
          ),
          const SizedBox(width: 24),
          // Speed dropdown
          DropdownButton<double>(
            value: _speeds.contains(speed) ? speed : 1.0,
            dropdownColor: NvrColors.bgSecondary,
            underline: const SizedBox.shrink(),
            style: const TextStyle(color: NvrColors.textPrimary, fontSize: 13),
            items: _speeds
                .map((s) => DropdownMenuItem(
                      value: s,
                      child: Text(
                        s == s.truncateToDouble() ? '${s.toInt()}x' : '${s}x',
                        style: const TextStyle(color: NvrColors.textPrimary),
                      ),
                    ))
                .toList(),
            onChanged: (v) {
              if (v != null) onSpeedChange(v);
            },
          ),
        ],
      ),
    );
  }
}
