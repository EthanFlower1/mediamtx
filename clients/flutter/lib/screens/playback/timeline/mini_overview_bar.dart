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
    final colors = NvrColors.of(context);
    return SizedBox(
      height: 28,
      child: LayoutBuilder(
        builder: (context, constraints) {
          final fullDayVp = TimelineViewport(
            visibleStart: Duration.zero,
            visibleEnd: const Duration(hours: 24),
            widthPx: constraints.maxWidth,
          );

          return Stack(
            children: [
              // Outer container: bgSecondary background, 4px radius, border outline
              Container(
                width: constraints.maxWidth,
                height: 28,
                decoration: BoxDecoration(
                  color: colors.bgSecondary,
                  borderRadius: BorderRadius.circular(4),
                  border: Border.all(color: colors.border),
                ),
              ),

              // Painting layer: recording bars, viewport window, playhead
              ClipRRect(
                borderRadius: BorderRadius.circular(4),
                child: GestureDetector(
                  onTapUp: (details) {
                    final time =
                        fullDayVp.pixelToTime(details.localPosition.dx);
                    onViewportJump(time);
                  },
                  onHorizontalDragUpdate: (details) {
                    final time =
                        fullDayVp.pixelToTime(details.localPosition.dx);
                    onViewportJump(time);
                  },
                  child: CustomPaint(
                    size: Size(constraints.maxWidth, 28),
                    painter: _MiniOverviewPainter(
                      viewport: fullDayVp,
                      mainViewport: mainViewport,
                      segments: segments,
                      events: events,
                      dayStart: dayStart,
                      position: position,
                      colors: colors,
                    ),
                  ),
                ),
              ),

              // Time labels: "00:00" (left) and "24:00" (right)
              Positioned(
                left: 4,
                top: 0,
                bottom: 0,
                child: Align(
                  alignment: Alignment.centerLeft,
                  child: Text(
                    '00:00',
                    style: TextStyle(
                      fontFamily: 'JetBrainsMono',
                      fontSize: 8,
                      color: colors.textMuted,
                    ),
                  ),
                ),
              ),
              Positioned(
                right: 4,
                top: 0,
                bottom: 0,
                child: Align(
                  alignment: Alignment.centerRight,
                  child: Text(
                    '24:00',
                    style: TextStyle(
                      fontFamily: 'JetBrainsMono',
                      fontSize: 8,
                      color: colors.textMuted,
                    ),
                  ),
                ),
              ),
            ],
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
  final NvrColors colors;

  _MiniOverviewPainter({
    required this.viewport,
    required this.mainViewport,
    required this.segments,
    required this.events,
    required this.dayStart,
    required this.position,
    required this.colors,
  });

  @override
  void paint(Canvas canvas, Size size) {
    // Bar area occupies the vertical centre (10px height at centre)
    const barTop = 9.0;
    const barBottom = 19.0;

    // Recording segments: accent at 13% opacity
    final segPaint = Paint()
      ..color = colors.accent.withOpacity(0.13);
    for (final seg in segments) {
      final x1 = viewport.timeToPixel(seg.startTime.difference(dayStart));
      final x2 = viewport.timeToPixel(seg.endTime.difference(dayStart));
      canvas.drawRect(Rect.fromLTRB(x1, barTop, x2, barBottom), segPaint);
    }

    // Viewport window: accent fill at 13% opacity + accent border 1.5px
    final rangeX1 = viewport.timeToPixel(mainViewport.visibleStart);
    final rangeX2 = viewport.timeToPixel(mainViewport.visibleEnd);

    canvas.drawRect(
      Rect.fromLTRB(rangeX1, barTop, rangeX2, barBottom),
      Paint()
        ..color = colors.accent.withOpacity(0.13)
        ..style = PaintingStyle.fill,
    );
    canvas.drawRect(
      Rect.fromLTRB(rangeX1, barTop, rangeX2, barBottom),
      Paint()
        ..color = colors.accent
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1.5,
    );

    // Playhead
    final px = viewport.timeToPixel(position);
    canvas.drawLine(
      Offset(px, barTop - 2),
      Offset(px, barBottom + 2),
      Paint()
        ..color = colors.accent
        ..strokeWidth = 1.5,
    );
  }

  @override
  bool shouldRepaint(_MiniOverviewPainter old) => true;
}
