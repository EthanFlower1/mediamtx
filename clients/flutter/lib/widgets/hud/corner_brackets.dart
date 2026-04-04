import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

/// Amber corner-bracket overlay for camera tiles.
/// Wraps a child widget with L-shaped brackets at each corner.
class CornerBrackets extends StatelessWidget {
  const CornerBrackets({
    super.key,
    required this.child,
    this.bracketSize = 16.0,
    this.strokeWidth = 2.0,
    this.color,
    this.padding = 6.0,
  });

  final Widget child;
  final double bracketSize;
  final double strokeWidth;
  final Color? color;
  final double padding;

  @override
  Widget build(BuildContext context) {
    final c = color ?? NvrColors.of(context).accent.withOpacity(0.4);
    return Stack(
      children: [
        child,
        Positioned(
          top: padding, left: padding,
          child: _Bracket(size: bracketSize, stroke: strokeWidth, color: c, corner: _Corner.topLeft),
        ),
        Positioned(
          top: padding, right: padding,
          child: _Bracket(size: bracketSize, stroke: strokeWidth, color: c, corner: _Corner.topRight),
        ),
        Positioned(
          bottom: padding, left: padding,
          child: _Bracket(size: bracketSize, stroke: strokeWidth, color: c, corner: _Corner.bottomLeft),
        ),
        Positioned(
          bottom: padding, right: padding,
          child: _Bracket(size: bracketSize, stroke: strokeWidth, color: c, corner: _Corner.bottomRight),
        ),
      ],
    );
  }
}

enum _Corner { topLeft, topRight, bottomLeft, bottomRight }

class _Bracket extends StatelessWidget {
  const _Bracket({required this.size, required this.stroke, required this.color, required this.corner});
  final double size;
  final double stroke;
  final Color color;
  final _Corner corner;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: size,
      height: size,
      child: CustomPaint(painter: _BracketPainter(stroke: stroke, color: color, corner: corner)),
    );
  }
}

class _BracketPainter extends CustomPainter {
  _BracketPainter({required this.stroke, required this.color, required this.corner});
  final double stroke;
  final Color color;
  final _Corner corner;

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()..color = color..strokeWidth = stroke..style = PaintingStyle.stroke;
    final path = Path();
    switch (corner) {
      case _Corner.topLeft:
        path.moveTo(0, size.height);
        path.lineTo(0, 0);
        path.lineTo(size.width, 0);
      case _Corner.topRight:
        path.moveTo(0, 0);
        path.lineTo(size.width, 0);
        path.lineTo(size.width, size.height);
      case _Corner.bottomLeft:
        path.moveTo(0, 0);
        path.lineTo(0, size.height);
        path.lineTo(size.width, size.height);
      case _Corner.bottomRight:
        path.moveTo(size.width, 0);
        path.lineTo(size.width, size.height);
        path.lineTo(0, size.height);
    }
    canvas.drawPath(path, paint);
  }

  @override
  bool shouldRepaint(covariant _BracketPainter old) =>
      old.color != color || old.stroke != stroke || old.corner != corner;
}
