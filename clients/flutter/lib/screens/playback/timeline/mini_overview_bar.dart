import 'package:flutter/material.dart';
import '../../../models/recording.dart';
import '../../../theme/nvr_colors.dart';
import 'timeline_viewport.dart';

class MiniOverviewBar extends StatelessWidget {
  final TimelineViewport mainViewport;
  final List<RecordingSegment> segments;
  final List<MotionEvent> events;
  final DateTime dayStart;
  final Duration position;
  final ValueChanged<Duration> onViewportJump;

  const MiniOverviewBar({
    super.key,
    required this.mainViewport,
    required this.segments,
    required this.events,
    required this.dayStart,
    required this.position,
    required this.onViewportJump,
  });

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: 32,
      child: LayoutBuilder(
        builder: (context, constraints) {
          final fullDayVp = TimelineViewport(
            visibleStart: Duration.zero,
            visibleEnd: const Duration(hours: 24),
            widthPx: constraints.maxWidth,
          );

          return GestureDetector(
            onTapUp: (details) {
              final time = fullDayVp.pixelToTime(details.localPosition.dx);
              onViewportJump(time);
            },
            onHorizontalDragUpdate: (details) {
              final time = fullDayVp.pixelToTime(details.localPosition.dx);
              onViewportJump(time);
            },
            child: CustomPaint(
              size: Size(constraints.maxWidth, 32),
              painter: _MiniOverviewPainter(
                viewport: fullDayVp,
                mainViewport: mainViewport,
                segments: segments,
                events: events,
                dayStart: dayStart,
                position: position,
              ),
            ),
          );
        },
      ),
    );
  }
}

class _MiniOverviewPainter extends CustomPainter {
  final TimelineViewport viewport;
  final TimelineViewport mainViewport;
  final List<RecordingSegment> segments;
  final List<MotionEvent> events;
  final DateTime dayStart;
  final Duration position;

  _MiniOverviewPainter({
    required this.viewport,
    required this.mainViewport,
    required this.segments,
    required this.events,
    required this.dayStart,
    required this.position,
  });

  @override
  void paint(Canvas canvas, Size size) {
    // Background
    canvas.drawRect(
      Rect.fromLTWH(0, 0, size.width, size.height),
      Paint()..color = NvrColors.bgTertiary.withValues(alpha: 0.5),
    );

    // Recording segments
    final segPaint = Paint()..color = NvrColors.accent.withValues(alpha: 0.4);
    for (final seg in segments) {
      final x1 = viewport.timeToPixel(seg.startTime.difference(dayStart));
      final x2 = viewport.timeToPixel(seg.endTime.difference(dayStart));
      canvas.drawRect(Rect.fromLTRB(x1, 4, x2, size.height - 4), segPaint);
    }

    // Event dots
    for (final event in events) {
      final x = viewport.timeToPixel(event.startTime.difference(dayStart));
      canvas.drawCircle(
        Offset(x, size.height / 2),
        2,
        Paint()..color = Colors.amber.withValues(alpha: 0.7),
      );
    }

    // Visible range highlight
    final rangeX1 = viewport.timeToPixel(mainViewport.visibleStart);
    final rangeX2 = viewport.timeToPixel(mainViewport.visibleEnd);
    canvas.drawRect(
      Rect.fromLTRB(rangeX1, 0, rangeX2, size.height),
      Paint()
        ..color = NvrColors.accent.withValues(alpha: 0.15)
        ..style = PaintingStyle.fill,
    );
    canvas.drawRect(
      Rect.fromLTRB(rangeX1, 0, rangeX2, size.height),
      Paint()
        ..color = NvrColors.accent.withValues(alpha: 0.6)
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1,
    );

    // Playhead
    final px = viewport.timeToPixel(position);
    canvas.drawLine(
      Offset(px, 0),
      Offset(px, size.height),
      Paint()
        ..color = NvrColors.accent
        ..strokeWidth = 1.5,
    );
  }

  @override
  bool shouldRepaint(_MiniOverviewPainter old) => true;
}
