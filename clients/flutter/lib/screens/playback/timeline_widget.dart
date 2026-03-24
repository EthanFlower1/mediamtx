import 'package:flutter/material.dart';
import '../../models/recording.dart';
import '../../theme/nvr_colors.dart';

class TimelineWidget extends StatelessWidget {
  final List<MotionEvent> motionEvents;
  final DateTime selectedDate;
  final Duration position;
  final bool vertical;
  final ValueChanged<Duration> onSeek;

  const TimelineWidget({
    super.key,
    required this.motionEvents,
    required this.selectedDate,
    required this.position,
    required this.onSeek,
    this.vertical = false,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTapDown: (details) => _handleTap(context, details),
      child: CustomPaint(
        painter: _TimelinePainter(
          motionEvents: motionEvents,
          selectedDate: selectedDate,
          position: position,
          vertical: vertical,
        ),
        child: vertical
            ? const SizedBox(width: 64, height: double.infinity)
            : const SizedBox(height: 64, width: double.infinity),
      ),
    );
  }

  void _handleTap(BuildContext context, TapDownDetails details) {
    final box = context.findRenderObject() as RenderBox;
    final size = box.size;
    double fraction;
    if (vertical) {
      fraction = details.localPosition.dy / size.height;
    } else {
      fraction = details.localPosition.dx / size.width;
    }
    fraction = fraction.clamp(0.0, 1.0);
    final seeked = Duration(seconds: (fraction * 86400).round());
    onSeek(seeked);
  }
}

class _TimelinePainter extends CustomPainter {
  final List<MotionEvent> motionEvents;
  final DateTime selectedDate;
  final Duration position;
  final bool vertical;

  _TimelinePainter({
    required this.motionEvents,
    required this.selectedDate,
    required this.position,
    required this.vertical,
  });

  @override
  void paint(Canvas canvas, Size size) {
    final bgPaint = Paint()..color = NvrColors.bgTertiary;
    canvas.drawRect(Offset.zero & size, bgPaint);

    const totalSecs = 86400.0;

    // Draw hour grid lines + labels
    final gridPaint = Paint()
      ..color = NvrColors.border
      ..strokeWidth = 0.5;

    const labelStyle = TextStyle(
      color: NvrColors.textMuted,
      fontSize: 9,
    );

    for (int h = 0; h <= 24; h++) {
      final frac = h / 24.0;
      if (vertical) {
        final y = frac * size.height;
        canvas.drawLine(Offset(0, y), Offset(size.width, y), gridPaint);
        if (h % 6 == 0) {
          _drawText(canvas, '${h.toString().padLeft(2, '0')}:00',
              Offset(2, y + 2), labelStyle);
        }
      } else {
        final x = frac * size.width;
        canvas.drawLine(Offset(x, 0), Offset(x, size.height), gridPaint);
        if (h % 6 == 0) {
          _drawText(canvas, '${h.toString().padLeft(2, '0')}:00',
              Offset(x + 2, 2), labelStyle);
        }
      }
    }

    // Draw motion event bars
    final motionPaint = Paint()..color = NvrColors.warning.withValues(alpha: 0.7);
    final dayStart = DateTime(
      selectedDate.year,
      selectedDate.month,
      selectedDate.day,
    );

    for (final event in motionEvents) {
      final startSec = event.startTime.difference(dayStart).inSeconds.toDouble();
      final endSec = event.endTime != null
          ? event.endTime!.difference(dayStart).inSeconds.toDouble()
          : startSec + 30;

      final startFrac = (startSec / totalSecs).clamp(0.0, 1.0);
      final endFrac = (endSec / totalSecs).clamp(0.0, 1.0);

      if (vertical) {
        final y1 = startFrac * size.height;
        final y2 = endFrac * size.height;
        canvas.drawRect(
          Rect.fromLTWH(4, y1, size.width - 8, (y2 - y1).clamp(2.0, double.infinity)),
          motionPaint,
        );
      } else {
        final x1 = startFrac * size.width;
        final x2 = endFrac * size.width;
        canvas.drawRect(
          Rect.fromLTWH(x1, 4, (x2 - x1).clamp(2.0, double.infinity), size.height - 8),
          motionPaint,
        );
      }
    }

    // Draw playback position marker
    final posFrac = (position.inSeconds / totalSecs).clamp(0.0, 1.0);
    final markerPaint = Paint()
      ..color = NvrColors.accent
      ..strokeWidth = 2.0;

    if (vertical) {
      final y = posFrac * size.height;
      canvas.drawLine(Offset(0, y), Offset(size.width, y), markerPaint);
      // Triangle pointing right
      final path = Path()
        ..moveTo(0, y - 5)
        ..lineTo(8, y)
        ..lineTo(0, y + 5)
        ..close();
      canvas.drawPath(path, Paint()..color = NvrColors.accent);
    } else {
      final x = posFrac * size.width;
      canvas.drawLine(Offset(x, 0), Offset(x, size.height), markerPaint);
      // Triangle pointing down
      final path = Path()
        ..moveTo(x - 5, 0)
        ..lineTo(x + 5, 0)
        ..lineTo(x, 8)
        ..close();
      canvas.drawPath(path, Paint()..color = NvrColors.accent);
    }
  }

  void _drawText(Canvas canvas, String text, Offset offset, TextStyle style) {
    final span = TextSpan(text: text, style: style);
    final painter = TextPainter(
      text: span,
      textDirection: TextDirection.ltr,
    )..layout();
    painter.paint(canvas, offset);
  }

  @override
  bool shouldRepaint(_TimelinePainter old) {
    return old.position != position ||
        old.motionEvents.length != motionEvents.length ||
        old.selectedDate != selectedDate;
  }
}
