import 'dart:math';
import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

class RotaryKnob extends StatefulWidget {
  const RotaryKnob({
    super.key,
    required this.value,
    required this.onChanged,
    this.label,
    this.min = 0.0,
    this.max = 1.0,
    this.size = 40.0,
    this.valueFormatter,
  });

  final double value;
  final ValueChanged<double> onChanged;
  final String? label;
  final double min;
  final double max;
  final double size;
  final String Function(double)? valueFormatter;

  @override
  State<RotaryKnob> createState() => _RotaryKnobState();
}

class _RotaryKnobState extends State<RotaryKnob> {
  double? _startAngle;
  double? _startValue;

  double get _fraction => ((widget.value - widget.min) / (widget.max - widget.min)).clamp(0.0, 1.0);
  // Map fraction to angle: -135° to +135° (270° sweep)
  double get _angle => -135 + _fraction * 270;

  void _onPanStart(DragStartDetails details) {
    final center = Offset(widget.size / 2, widget.size / 2);
    _startAngle = (details.localPosition - center).direction;
    _startValue = widget.value;
  }

  void _onPanUpdate(DragUpdateDetails details) {
    if (_startAngle == null || _startValue == null) return;
    final center = Offset(widget.size / 2, widget.size / 2);
    final currentAngle = (details.localPosition - center).direction;
    final delta = currentAngle - _startAngle!;
    final valueDelta = delta / pi * (widget.max - widget.min) * 0.5;
    final newValue = (_startValue! + valueDelta).clamp(widget.min, widget.max);
    widget.onChanged(newValue);
  }

  @override
  Widget build(BuildContext context) {
    final display = widget.valueFormatter?.call(widget.value) ??
        '${(widget.value * 100).round()}%';

    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (widget.label != null)
          Padding(
            padding: const EdgeInsets.only(bottom: 6),
            child: Text(widget.label!, style: TextStyle(
              fontFamily: 'JetBrainsMono', fontSize: 9,
              letterSpacing: 1, color: NvrColors.textMuted,
            )),
          ),
        GestureDetector(
          onPanStart: _onPanStart,
          onPanUpdate: _onPanUpdate,
          child: SizedBox(
            width: widget.size,
            height: widget.size,
            child: CustomPaint(
              painter: _KnobPainter(angle: _angle),
            ),
          ),
        ),
        const SizedBox(height: 4),
        Text(display, style: TextStyle(
          fontFamily: 'JetBrainsMono', fontSize: 9, color: NvrColors.accent,
        )),
      ],
    );
  }
}

class _KnobPainter extends CustomPainter {
  _KnobPainter({required this.angle});
  final double angle;

  @override
  void paint(Canvas canvas, Size size) {
    final center = Offset(size.width / 2, size.height / 2);
    final radius = size.width / 2;

    // Body gradient
    final bodyPaint = Paint()
      ..shader = RadialGradient(
        center: const Alignment(-0.2, -0.2),
        colors: [NvrColors.bgTertiary, NvrColors.bgPrimary],
      ).createShader(Rect.fromCircle(center: center, radius: radius));
    canvas.drawCircle(center, radius, bodyPaint);

    // Border
    canvas.drawCircle(center, radius, Paint()
      ..color = NvrColors.border
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2);

    // Notch marks
    final notchPaint = Paint()..color = NvrColors.border..strokeWidth = 1;
    for (var i = 0; i < 4; i++) {
      final a = i * pi / 2;
      final outer = center + Offset(cos(a), sin(a)) * (radius + 4);
      final inner = center + Offset(cos(a), sin(a)) * radius;
      canvas.drawLine(inner, outer, notchPaint);
    }

    // Indicator line
    final rad = angle * pi / 180;
    final indicatorPaint = Paint()
      ..color = NvrColors.accent
      ..strokeWidth = 2
      ..strokeCap = StrokeCap.round;
    final from = center + Offset(cos(rad), sin(rad)) * 4;
    final to = center + Offset(cos(rad), sin(rad)) * (radius - 4);
    canvas.drawLine(from, to, indicatorPaint);
  }

  @override
  bool shouldRepaint(covariant _KnobPainter old) => old.angle != angle;
}
