import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_animations.dart';

class AnalogSlider extends StatefulWidget {
  const AnalogSlider({
    super.key,
    required this.value,
    required this.onChanged,
    this.label,
    this.min = 0.0,
    this.max = 1.0,
    this.tickCount = 11,
    this.valueFormatter,
  });

  final double value;
  final ValueChanged<double> onChanged;
  final String? label;
  final double min;
  final double max;
  final int tickCount;
  final String Function(double)? valueFormatter;

  @override
  State<AnalogSlider> createState() => _AnalogSliderState();
}

class _AnalogSliderState extends State<AnalogSlider> {
  bool _dragging = false;

  double get _fraction => ((widget.value - widget.min) / (widget.max - widget.min)).clamp(0.0, 1.0);

  void _onPanUpdate(DragUpdateDetails details, BoxConstraints constraints) {
    final dx = details.localPosition.dx.clamp(0.0, constraints.maxWidth);
    final fraction = dx / constraints.maxWidth;
    final value = widget.min + fraction * (widget.max - widget.min);
    widget.onChanged(value.clamp(widget.min, widget.max));
  }

  @override
  Widget build(BuildContext context) {
    final displayValue = widget.valueFormatter?.call(widget.value) ??
        '${(widget.value * 100).round()}%';

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        if (widget.label != null)
          Padding(
            padding: const EdgeInsets.only(bottom: 6),
            child: Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text(widget.label!, style: TextStyle(
                  fontFamily: 'JetBrainsMono', fontSize: 9,
                  letterSpacing: 1, color: NvrColors.textMuted,
                )),
                Text(displayValue, style: TextStyle(
                  fontFamily: 'JetBrainsMono', fontSize: 9, color: NvrColors.accent,
                )),
              ],
            ),
          ),
        LayoutBuilder(builder: (context, constraints) {
          return GestureDetector(
            onPanStart: (_) => setState(() => _dragging = true),
            onPanUpdate: (d) => _onPanUpdate(d, constraints),
            onPanEnd: (_) => setState(() => _dragging = false),
            onTapDown: (d) {
              final fraction = d.localPosition.dx / constraints.maxWidth;
              final value = widget.min + fraction * (widget.max - widget.min);
              widget.onChanged(value.clamp(widget.min, widget.max));
            },
            child: SizedBox(
              height: 24,
              child: Stack(
                clipBehavior: Clip.none,
                alignment: Alignment.centerLeft,
                children: [
                  // Track
                  Container(
                    height: 6,
                    decoration: BoxDecoration(
                      color: NvrColors.bgTertiary,
                      border: Border.all(color: NvrColors.border),
                      borderRadius: BorderRadius.circular(3),
                    ),
                  ),
                  // Fill
                  FractionallySizedBox(
                    widthFactor: _fraction,
                    child: Container(
                      height: 6,
                      decoration: BoxDecoration(
                        gradient: LinearGradient(
                          colors: [NvrColors.accent, NvrColors.accent.withOpacity(0.4)],
                        ),
                        borderRadius: BorderRadius.circular(3),
                      ),
                    ),
                  ),
                  // Thumb
                  Positioned(
                    left: _fraction * constraints.maxWidth - 9,
                    child: AnimatedContainer(
                      duration: NvrAnimations.microDuration,
                      width: _dragging ? 20 : 18,
                      height: _dragging ? 20 : 18,
                      decoration: BoxDecoration(
                        shape: BoxShape.circle,
                        color: NvrColors.bgTertiary,
                        border: Border.all(color: NvrColors.accent, width: 2),
                        boxShadow: [
                          BoxShadow(
                            color: NvrColors.accent.withOpacity(_dragging ? 0.5 : 0.25),
                            blurRadius: _dragging ? 10 : 6,
                          ),
                        ],
                      ),
                    ),
                  ),
                ],
              ),
            ),
          );
        }),
        // Tick marks
        Padding(
          padding: const EdgeInsets.only(top: 4),
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceBetween,
            children: List.generate(widget.tickCount, (_) =>
              Container(width: 1, height: 4, color: NvrColors.border),
            ),
          ),
        ),
      ],
    );
  }
}
