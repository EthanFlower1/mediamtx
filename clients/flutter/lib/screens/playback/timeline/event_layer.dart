import 'package:flutter/material.dart';
import '../../../models/recording.dart';
import 'timeline_viewport.dart';

class EventLayer extends CustomPainter {
  final TimelineViewport viewport;
  final List<MotionEvent> events;
  final DateTime dayStart;

  EventLayer({
    required this.viewport,
    required this.events,
    required this.dayStart,
  });

  static const _eventColors = <String, Color>{
    'person': Colors.blue,
    'vehicle': Colors.green,
    'car': Colors.green,
    'truck': Colors.green,
    'bus': Colors.green,
    'motorcycle': Colors.green,
    'motion': Colors.amber,
  };

  static Color colorForClass(String? objectClass) {
    if (objectClass == null) return Colors.red;
    return _eventColors[objectClass.toLowerCase()] ?? Colors.red;
  }

  @override
  void paint(Canvas canvas, Size size) {
    final showBars = viewport.zoomLevel > 4;

    for (final event in events) {
      final startDur = event.startTime.difference(dayStart);
      final x1 = viewport.timeToPixel(startDur);

      if (x1 > viewport.widthPx + 10) continue;

      final color = colorForClass(event.objectClass);
      final y = size.height * 0.6;

      if (showBars && event.endTime != null) {
        final endDur = event.endTime!.difference(dayStart);
        final x2 = viewport.timeToPixel(endDur);

        if (x2 < -10) continue;

        final barPaint = Paint()..color = color.withValues(alpha: 0.6);
        canvas.drawRRect(
          RRect.fromRectAndRadius(
            Rect.fromLTRB(
              x1.clamp(0, viewport.widthPx).toDouble(),
              y - 4,
              x2.clamp(0, viewport.widthPx).clamp(x1 + 2, viewport.widthPx).toDouble(),
              y + 4,
            ),
            const Radius.circular(2),
          ),
          barPaint,
        );
      } else {
        if (x1 < -10) continue;
        canvas.drawCircle(
          Offset(x1.clamp(0, viewport.widthPx), y),
          4,
          Paint()..color = color,
        );
      }
    }
  }

  @override
  bool shouldRepaint(EventLayer old) =>
      old.events != events ||
      old.viewport.visibleStart != viewport.visibleStart ||
      old.viewport.visibleEnd != viewport.visibleEnd;
}
