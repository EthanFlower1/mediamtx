import 'package:flutter/material.dart';
import '../../../models/recording.dart';
import '../../../theme/nvr_colors.dart';
import 'timeline_viewport.dart';

class RecordingLayer extends CustomPainter {
  final TimelineViewport viewport;
  final List<RecordingSegment> segments;
  final DateTime dayStart;

  RecordingLayer({
    required this.viewport,
    required this.segments,
    required this.dayStart,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final paint = Paint()..color = NvrColors.accent.withValues(alpha: 0.25);

    for (final seg in segments) {
      final startDur = seg.startTime.difference(dayStart);
      final endDur = seg.endTime.difference(dayStart);

      final x1 = viewport.timeToPixel(startDur);
      final x2 = viewport.timeToPixel(endDur);

      if (x2 < 0 || x1 > viewport.widthPx) continue;

      canvas.drawRect(
        Rect.fromLTRB(
          x1.clamp(0, viewport.widthPx),
          0,
          x2.clamp(0, viewport.widthPx),
          size.height,
        ),
        paint,
      );
    }
  }

  @override
  bool shouldRepaint(RecordingLayer old) =>
      old.segments != segments ||
      old.viewport.visibleStart != viewport.visibleStart ||
      old.viewport.visibleEnd != viewport.visibleEnd;
}
