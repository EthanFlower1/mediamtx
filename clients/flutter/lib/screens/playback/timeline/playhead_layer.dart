import 'package:flutter/material.dart';
import '../../../theme/nvr_colors.dart';
import 'timeline_viewport.dart';

/// Purely visual — paints the playhead line and handle.
/// All gesture handling lives in InteractionLayer.
class PlayheadLayer extends CustomPainter {
  final TimelineViewport viewport;
  final Duration position;
  final bool isDragging;
  final double? dragX;

  PlayheadLayer({
    required this.viewport,
    required this.position,
    this.isDragging = false,
    this.dragX,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final x = dragX ?? viewport.timeToPixel(position);

    final paint = Paint()
      ..color = NvrColors.accent
      ..strokeWidth = 2;

    canvas.drawLine(Offset(x, 0), Offset(x, size.height), paint);

    final radius = isDragging ? 8.0 : 6.0;
    canvas.drawCircle(
      Offset(x, size.height / 2),
      radius,
      Paint()..color = NvrColors.accent,
    );
    canvas.drawCircle(
      Offset(x, size.height / 2),
      radius,
      Paint()
        ..color = Colors.white
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2,
    );
  }

  @override
  bool shouldRepaint(PlayheadLayer old) =>
      old.position != position ||
      old.isDragging != isDragging ||
      old.dragX != dragX;
}
