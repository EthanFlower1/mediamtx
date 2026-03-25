import 'package:flutter/material.dart';
import '../../../theme/nvr_colors.dart';

/// A jog slider that seeks on release. Dragging left/right accumulates a
/// time delta (up to ±60 seconds). On release the slider springs back to
/// center and fires onSeek with the absolute target position.
class JogSlider extends StatefulWidget {
  final ValueChanged<Duration> onSeek;
  final Duration currentPosition;

  const JogSlider({
    super.key,
    required this.onSeek,
    required this.currentPosition,
  });

  @override
  State<JogSlider> createState() => _JogSliderState();
}

class _JogSliderState extends State<JogSlider> {
  double _value = 0.0;

  String get _label {
    if (_value == 0) return '0s';
    final secs = (_value * 60).round();
    return '${secs > 0 ? '+' : ''}${secs}s';
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          _label,
          style: TextStyle(
            color: _value == 0 ? NvrColors.textMuted : NvrColors.accent,
            fontSize: 11,
            fontWeight: FontWeight.w500,
          ),
        ),
        SliderTheme(
          data: SliderTheme.of(context).copyWith(
            activeTrackColor: NvrColors.accent,
            inactiveTrackColor: NvrColors.bgTertiary,
            thumbColor: NvrColors.accent,
            overlayColor: NvrColors.accent.withValues(alpha: 0.12),
            trackHeight: 3,
            thumbShape: const RoundSliderThumbShape(enabledThumbRadius: 7),
          ),
          child: Slider(
            value: _value,
            min: -1.0,
            max: 1.0,
            onChanged: (v) => setState(() => _value = v),
            onChangeEnd: (v) {
              if (v != 0) {
                final delta = Duration(seconds: (v * 60).round());
                widget.onSeek(widget.currentPosition + delta);
              }
              setState(() => _value = 0.0);
            },
          ),
        ),
      ],
    );
  }
}
