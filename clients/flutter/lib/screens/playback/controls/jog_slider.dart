import 'dart:async';
import 'package:flutter/material.dart';
import '../../../theme/nvr_colors.dart';

class JogSlider extends StatefulWidget {
  final ValueChanged<double> onSpeedChange;
  final VoidCallback onRelease;

  const JogSlider({
    super.key,
    required this.onSpeedChange,
    required this.onRelease,
  });

  @override
  State<JogSlider> createState() => _JogSliderState();
}

class _JogSliderState extends State<JogSlider>
    with SingleTickerProviderStateMixin {
  double _value = 0.0;
  double _valueAtRelease = 0.0;
  late final AnimationController _springController;
  Timer? _jogTimer;

  @override
  void initState() {
    super.initState();
    _springController = AnimationController(
      vsync: this,
      duration: const Duration(milliseconds: 200),
    );
    _springController.addListener(() {
      setState(() {
        _value = _valueAtRelease * (1 - _springController.value);
      });
    });
  }

  @override
  void dispose() {
    _jogTimer?.cancel();
    _springController.dispose();
    super.dispose();
  }

  void _startJog() {
    _jogTimer?.cancel();
    _jogTimer = Timer.periodic(const Duration(milliseconds: 100), (_) {
      if (_value.abs() > 0.05) {
        widget.onSpeedChange(_value);
      }
    });
  }

  void _stopJog() {
    _jogTimer?.cancel();
    _jogTimer = null;
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          _value.abs() < 0.05
              ? '0x'
              : '${_value > 0 ? '+' : ''}${_value.toStringAsFixed(1)}x',
          style: const TextStyle(
            color: NvrColors.textSecondary,
            fontSize: 11,
          ),
        ),
        const SizedBox(height: 2),
        SizedBox(
          height: 32,
          child: SliderTheme(
            data: SliderThemeData(
              trackHeight: 4,
              thumbShape: const RoundSliderThumbShape(enabledThumbRadius: 8),
              activeTrackColor: NvrColors.accent,
              inactiveTrackColor: NvrColors.bgTertiary,
              thumbColor: NvrColors.accent,
              overlayColor: NvrColors.accent.withValues(alpha: 0.2),
            ),
            child: Slider(
              value: _value,
              min: -2.0,
              max: 2.0,
              onChangeStart: (_) {
                _springController.reset();
                _startJog();
              },
              onChanged: (v) {
                setState(() => _value = v);
              },
              onChangeEnd: (_) {
                _valueAtRelease = _value;
                _stopJog();
                widget.onRelease();
                _springController.forward(from: 0);
              },
            ),
          ),
        ),
      ],
    );
  }
}
