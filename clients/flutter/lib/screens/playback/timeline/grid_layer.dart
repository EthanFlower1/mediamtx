import 'package:flutter/material.dart';
import '../../../theme/nvr_colors.dart';
import 'timeline_viewport.dart';

class GridLayer extends CustomPainter {
  final TimelineViewport viewport;

  GridLayer({required this.viewport});

  @override
  void paint(Canvas canvas, Size size) {
    final gridPaint = Paint()
      ..color = NvrColors.border.withValues(alpha: 0.3)
      ..strokeWidth = 0.5;

    final interval = viewport.gridInterval;
    const labelStyle = TextStyle(
      color: NvrColors.textMuted,
      fontSize: 10,
    );

    var t = Duration(
      milliseconds: (viewport.visibleStart.inMilliseconds ~/
              interval.inMilliseconds) *
          interval.inMilliseconds,
    );

    while (t <= viewport.visibleEnd) {
      if (t >= viewport.visibleStart) {
        final x = viewport.timeToPixel(t);

        canvas.drawLine(
          Offset(x, 0),
          Offset(x, size.height),
          gridPaint,
        );

        final hours = t.inHours % 24;
        final minutes = t.inMinutes % 60;
        final label = minutes == 0
            ? '${hours.toString().padLeft(2, '0')}:00'
            : '${hours.toString().padLeft(2, '0')}:${minutes.toString().padLeft(2, '0')}';

        final tp = TextPainter(
          text: TextSpan(text: label, style: labelStyle),
          textDirection: TextDirection.ltr,
        )..layout();

        tp.paint(canvas, Offset(x + 4, size.height - tp.height - 4));
      }

      t += interval;
    }
  }

  @override
  bool shouldRepaint(GridLayer old) =>
      old.viewport.visibleStart != viewport.visibleStart ||
      old.viewport.visibleEnd != viewport.visibleEnd ||
      old.viewport.widthPx != viewport.widthPx;
}
