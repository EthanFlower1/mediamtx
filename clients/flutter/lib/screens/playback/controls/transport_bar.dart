import 'package:flutter/material.dart';
import '../../../theme/nvr_colors.dart';
import '../../../theme/nvr_typography.dart';
import '../../../widgets/hud/rotary_knob.dart';
import '../../../widgets/hud/segmented_control.dart';

// Speed preset list — index maps to a normalized 0–1 value for the knob.
const List<double> _speedPresets = [0.25, 0.5, 1.0, 2.0, 4.0, 8.0];

/// Converts a speed value to the normalised 0–1 range used by [RotaryKnob].
double _speedToKnob(double speed) {
  final idx = _speedPresets.indexWhere((s) => (s - speed).abs() < 0.001);
  final i = idx < 0 ? 2 : idx; // default to 1× (index 2) if not found
  return i / (_speedPresets.length - 1);
}

/// Converts a knob 0–1 value back to the nearest speed preset.
double _knobToSpeed(double knob) {
  final idx = (knob * (_speedPresets.length - 1)).round()
      .clamp(0, _speedPresets.length - 1);
  return _speedPresets[idx];
}

String _formatSpeed(double speed) {
  if (speed < 1.0) return '${speed}×';
  return '${speed.toStringAsFixed(speed.truncateToDouble() == speed ? 0 : 1)}×';
}

String _formatTimestamp(Duration d) {
  final h = d.inHours.remainder(24).toString().padLeft(2, '0');
  final m = d.inMinutes.remainder(60).toString().padLeft(2, '0');
  final s = d.inSeconds.remainder(60).toString().padLeft(2, '0');
  return '$h:$m:$s';
}

class TransportBar extends StatelessWidget {
  const TransportBar({
    super.key,
    required this.isPlaying,
    required this.currentTime,
    required this.speed,
    required this.zoomLevel,
    required this.onPlayPause,
    required this.onStepBack,
    required this.onStepForward,
    required this.onSkipPrevious,
    required this.onSkipNext,
    required this.onSpeedChanged,
    required this.onZoomChanged,
  });

  /// Whether the player is currently playing.
  final bool isPlaying;

  /// Current playback position expressed as a [Duration] from midnight.
  final Duration currentTime;

  /// Playback speed — one of [0.25, 0.5, 1.0, 2.0, 4.0, 8.0].
  final double speed;

  /// Index into the zoom preset list {0: 1H, 1: 30M, 2: 10M, 3: 5M}.
  final int zoomLevel;

  final VoidCallback onPlayPause;
  final VoidCallback onStepBack;
  final VoidCallback onStepForward;

  /// Skip to the previous recorded event.
  final VoidCallback onSkipPrevious;

  /// Skip to the next recorded event.
  final VoidCallback onSkipNext;

  final ValueChanged<double> onSpeedChanged;
  final ValueChanged<int> onZoomChanged;

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        color: NvrColors.bgPrimary,
        border: Border(
          top: BorderSide(color: NvrColors.border),
        ),
      ),
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.center,
        children: [
          // ── 1. Transport buttons ─────────────────────────────────────────
          _TransportButton(
            size: 28,
            bg: NvrColors.bgSecondary,
            icon: Icons.skip_previous,
            onTap: onSkipPrevious,
          ),
          const SizedBox(width: 4),
          _TransportButton(
            size: 28,
            bg: NvrColors.bgSecondary,
            icon: Icons.chevron_left,
            onTap: onStepBack,
          ),
          const SizedBox(width: 6),
          _TransportButton(
            size: 36,
            bg: NvrColors.accent,
            iconColor: NvrColors.bgPrimary,
            icon: isPlaying ? Icons.pause : Icons.play_arrow,
            onTap: onPlayPause,
          ),
          const SizedBox(width: 6),
          _TransportButton(
            size: 28,
            bg: NvrColors.bgSecondary,
            icon: Icons.chevron_right,
            onTap: onStepForward,
          ),
          const SizedBox(width: 4),
          _TransportButton(
            size: 28,
            bg: NvrColors.bgSecondary,
            icon: Icons.skip_next,
            onTap: onSkipNext,
          ),

          const SizedBox(width: 16),

          // ── 2. Speed RotaryKnob ──────────────────────────────────────────
          RotaryKnob(
            label: 'SPEED',
            value: _speedToKnob(speed),
            size: 28,
            onChanged: (v) => onSpeedChanged(_knobToSpeed(v)),
            valueFormatter: (_) => _formatSpeed(speed),
          ),

          const SizedBox(width: 16),

          // ── 3. Vertical divider ──────────────────────────────────────────
          Container(
            width: 1,
            height: 24,
            color: NvrColors.border,
          ),

          const SizedBox(width: 16),

          // ── 4. Current time ──────────────────────────────────────────────
          Text(
            _formatTimestamp(currentTime),
            style: NvrTypography.monoTimestamp.copyWith(fontSize: 13),
          ),

          const Spacer(),

          // ── 5. Zoom HudSegmentedControl ──────────────────────────────────
          HudSegmentedControl<int>(
            segments: const {0: '1H', 1: '30M', 2: '10M', 3: '5M'},
            selected: zoomLevel,
            onChanged: onZoomChanged,
          ),
        ],
      ),
    );
  }
}

// ── Helper widget ────────────────────────────────────────────────────────────

class _TransportButton extends StatelessWidget {
  const _TransportButton({
    required this.size,
    required this.bg,
    required this.icon,
    required this.onTap,
    this.iconColor,
  });

  final double size;
  final Color bg;
  final IconData icon;
  final VoidCallback onTap;

  /// Icon color — defaults to [NvrColors.textPrimary].
  final Color? iconColor;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        width: size,
        height: size,
        decoration: BoxDecoration(
          color: bg,
          border: Border.all(color: NvrColors.border),
          borderRadius: BorderRadius.circular(6),
        ),
        child: Center(
          child: Icon(
            icon,
            size: size * 0.55,
            color: iconColor ?? NvrColors.textPrimary,
          ),
        ),
      ),
    );
  }
}
