import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../models/recording.dart';
import '../../providers/recordings_provider.dart';
import '../../theme/nvr_colors.dart';

class TimelineWidget extends ConsumerStatefulWidget {
  final List<String> cameraIds;
  final DateTime selectedDate;
  final Duration position;
  final bool vertical;
  final ValueChanged<Duration> onSeek;

  const TimelineWidget({
    super.key,
    required this.cameraIds,
    required this.selectedDate,
    required this.position,
    required this.onSeek,
    this.vertical = false,
  });

  @override
  ConsumerState<TimelineWidget> createState() => _TimelineWidgetState();
}

class _TimelineWidgetState extends ConsumerState<TimelineWidget> {
  bool _dragging = false;

  String get _dateKey =>
      '${widget.selectedDate.year}-'
      '${widget.selectedDate.month.toString().padLeft(2, '0')}-'
      '${widget.selectedDate.day.toString().padLeft(2, '0')}';

  Duration _pixelToDuration(double pixel, double totalPixels) {
    final fraction = (pixel / totalPixels).clamp(0.0, 1.0);
    return Duration(milliseconds: (fraction * 86400000).round());
  }

  void _handleDragUpdate(DragUpdateDetails details, BoxConstraints constraints) {
    final totalPixels =
        widget.vertical ? constraints.maxHeight : constraints.maxWidth;
    final pixel = widget.vertical
        ? details.localPosition.dy
        : details.localPosition.dx;
    widget.onSeek(_pixelToDuration(pixel, totalPixels));
  }

  void _handleTapDown(TapDownDetails details, BoxConstraints constraints) {
    final totalPixels =
        widget.vertical ? constraints.maxHeight : constraints.maxWidth;
    final pixel = widget.vertical
        ? details.localPosition.dy
        : details.localPosition.dx;
    widget.onSeek(_pixelToDuration(pixel, totalPixels));
  }

  @override
  Widget build(BuildContext context) {
    // Fetch data for all selected cameras; merge results.
    final List<RecordingSegment> allSegments = [];
    final List<MotionEvent> allEvents = [];

    for (final cameraId in widget.cameraIds) {
      final key = (cameraId: cameraId, date: _dateKey);

      final segmentsAsync = ref.watch(recordingSegmentsProvider(key));
      segmentsAsync.whenData((segs) => allSegments.addAll(segs));

      final eventsAsync = ref.watch(motionEventsProvider(key));
      eventsAsync.whenData((evts) => allEvents.addAll(evts));
    }

    return LayoutBuilder(
      builder: (context, constraints) {
        return GestureDetector(
          onTapDown: (d) => _handleTapDown(d, constraints),
          onVerticalDragStart: widget.vertical
              ? (_) => setState(() => _dragging = true)
              : null,
          onVerticalDragUpdate: widget.vertical
              ? (d) => _handleDragUpdate(d, constraints)
              : null,
          onVerticalDragEnd: widget.vertical
              ? (_) => setState(() => _dragging = false)
              : null,
          onHorizontalDragStart: !widget.vertical
              ? (_) => setState(() => _dragging = true)
              : null,
          onHorizontalDragUpdate: !widget.vertical
              ? (d) => _handleDragUpdate(d, constraints)
              : null,
          onHorizontalDragEnd: !widget.vertical
              ? (_) => setState(() => _dragging = false)
              : null,
          child: CustomPaint(
            painter: _TimelinePainter(
              segments: allSegments,
              motionEvents: allEvents,
              selectedDate: widget.selectedDate,
              position: widget.position,
              vertical: widget.vertical,
              dragging: _dragging,
            ),
            size: Size(constraints.maxWidth, constraints.maxHeight),
          ),
        );
      },
    );
  }
}

// ── Painter ───────────────────────────────────────────────────────────────────

class _TimelinePainter extends CustomPainter {
  final List<RecordingSegment> segments;
  final List<MotionEvent> motionEvents;
  final DateTime selectedDate;
  final Duration position;
  final bool vertical;
  final bool dragging;

  _TimelinePainter({
    required this.segments,
    required this.motionEvents,
    required this.selectedDate,
    required this.position,
    required this.vertical,
    required this.dragging,
  });

  static const double _totalMs = 86400000.0;

  double _frac(DateTime dt) {
    final dayStart =
        DateTime(selectedDate.year, selectedDate.month, selectedDate.day);
    return (dt.difference(dayStart).inMilliseconds / _totalMs).clamp(0.0, 1.0);
  }

  double _posFrac() =>
      (position.inMilliseconds / _totalMs).clamp(0.0, 1.0);

  @override
  void paint(Canvas canvas, Size size) {
    // Background
    final bgPaint = Paint()..color = NvrColors.bgPrimary;
    canvas.drawRect(Offset.zero & size, bgPaint);

    if (vertical) {
      _paintVertical(canvas, size);
    } else {
      _paintHorizontal(canvas, size);
    }
  }

  // ── Vertical (wide layout) ─────────────────────────────────────────────────

  void _paintVertical(Canvas canvas, Size size) {
    final w = size.width;
    final h = size.height;

    // Recording segments band (left 40% of width)
    final segPaint = Paint()
      ..color = NvrColors.accent.withValues(alpha: 0.4);
    final segBandRight = w * 0.42;
    const segBandLeft = 0.0;

    final dayStart =
        DateTime(selectedDate.year, selectedDate.month, selectedDate.day);

    for (int i = 0; i < segments.length; i++) {
      final seg = segments[i];
      final startFrac = _frac(seg.startTime);
      // Use next segment start as end, or 15 minutes if last segment
      final double endFrac;
      if (i + 1 < segments.length) {
        endFrac = (_frac(segments[i + 1].startTime)).clamp(0.0, 1.0);
      } else {
        final endMs =
            seg.startTime.difference(dayStart).inMilliseconds + 15 * 60 * 1000;
        endFrac = (endMs / _totalMs).clamp(0.0, 1.0);
      }
      final y1 = startFrac * h;
      final y2 = endFrac * h;
      canvas.drawRect(
        Rect.fromLTWH(
            segBandLeft, y1, segBandRight, (y2 - y1).clamp(2.0, double.infinity)),
        segPaint,
      );
    }

    // Hour grid lines + labels
    final gridPaint = Paint()
      ..color = NvrColors.border.withValues(alpha: 0.6)
      ..strokeWidth = 0.5;

    for (int h2 = 0; h2 <= 24; h2++) {
      final frac = h2 / 24.0;
      final y = frac * h;
      canvas.drawLine(Offset(0, y), Offset(w, y), gridPaint);
      if (h2 % 3 == 0) {
        _drawText(
          canvas,
          '${h2.toString().padLeft(2, '0')}:00',
          Offset(segBandRight + 3, y + 2),
          const TextStyle(color: NvrColors.textMuted, fontSize: 8),
        );
      }
    }

    // Motion event dots (right band)
    final eventX = w * 0.7;
    for (final event in motionEvents) {
      final frac = _frac(event.startTime);
      final y = frac * h;
      final color = _eventColor(event);
      canvas.drawCircle(
        Offset(eventX, y),
        3.5,
        Paint()..color = color,
      );
    }

    // Playhead
    final posFrac = _posFrac();
    final py = posFrac * h;
    final playheadPaint = Paint()
      ..color = NvrColors.accent
      ..strokeWidth = 2.0;
    canvas.drawLine(Offset(0, py), Offset(w, py), playheadPaint);

    // Circular handle
    final handleRadius = dragging ? 7.0 : 6.0;
    canvas.drawCircle(
      Offset(w / 2, py),
      handleRadius,
      Paint()..color = NvrColors.accent,
    );
    canvas.drawCircle(
      Offset(w / 2, py),
      handleRadius,
      Paint()
        ..color = Colors.white
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1.5,
    );

    // Current time label
    final timeStr = _formatDuration(position);
    _drawText(
      canvas,
      timeStr,
      Offset(2, (py - 18).clamp(0.0, h - 18)),
      const TextStyle(
        color: Colors.white,
        fontSize: 9,
        fontWeight: FontWeight.bold,
      ),
    );
  }

  // ── Horizontal (narrow layout) ─────────────────────────────────────────────

  void _paintHorizontal(Canvas canvas, Size size) {
    final w = size.width;
    final h = size.height;

    // Recording segments band (top 35% of height)
    final segPaint = Paint()
      ..color = NvrColors.accent.withValues(alpha: 0.4);
    final segBandBottom = h * 0.40;
    const segBandTop = 0.0;

    final dayStart =
        DateTime(selectedDate.year, selectedDate.month, selectedDate.day);

    for (int i = 0; i < segments.length; i++) {
      final seg = segments[i];
      final startFrac = _frac(seg.startTime);
      final double endFrac;
      if (i + 1 < segments.length) {
        endFrac = (_frac(segments[i + 1].startTime)).clamp(0.0, 1.0);
      } else {
        final endMs =
            seg.startTime.difference(dayStart).inMilliseconds + 15 * 60 * 1000;
        endFrac = (endMs / _totalMs).clamp(0.0, 1.0);
      }
      final x1 = startFrac * w;
      final x2 = endFrac * w;
      canvas.drawRect(
        Rect.fromLTWH(
            x1, segBandTop, (x2 - x1).clamp(2.0, double.infinity), segBandBottom),
        segPaint,
      );
    }

    // Hour grid lines + labels
    final gridPaint = Paint()
      ..color = NvrColors.border.withValues(alpha: 0.6)
      ..strokeWidth = 0.5;

    for (int h2 = 0; h2 <= 24; h2++) {
      final frac = h2 / 24.0;
      final x = frac * w;
      canvas.drawLine(Offset(x, 0), Offset(x, h), gridPaint);
      if (h2 % 6 == 0) {
        _drawText(
          canvas,
          '${h2.toString().padLeft(2, '0')}:00',
          Offset(x + 2, segBandBottom + 2),
          const TextStyle(color: NvrColors.textMuted, fontSize: 8),
        );
      }
    }

    // Motion event dots (lower band)
    final eventY = h * 0.72;
    for (final event in motionEvents) {
      final frac = _frac(event.startTime);
      final x = frac * w;
      final color = _eventColor(event);
      canvas.drawCircle(
        Offset(x, eventY),
        3.5,
        Paint()..color = color,
      );
    }

    // Playhead
    final posFrac = _posFrac();
    final px = posFrac * w;
    final playheadPaint = Paint()
      ..color = NvrColors.accent
      ..strokeWidth = 2.0;
    canvas.drawLine(Offset(px, 0), Offset(px, h), playheadPaint);

    // Circular handle (at top)
    final handleRadius = dragging ? 7.0 : 6.0;
    canvas.drawCircle(
      Offset(px, h / 2),
      handleRadius,
      Paint()..color = NvrColors.accent,
    );
    canvas.drawCircle(
      Offset(px, h / 2),
      handleRadius,
      Paint()
        ..color = Colors.white
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1.5,
    );

    // Current time label above playhead
    final timeStr = _formatDuration(position);
    final labelX = (px + 4).clamp(0.0, w - 48);
    _drawText(
      canvas,
      timeStr,
      Offset(labelX, 2),
      const TextStyle(
        color: Colors.white,
        fontSize: 9,
        fontWeight: FontWeight.bold,
      ),
    );
  }

  // ── Helpers ────────────────────────────────────────────────────────────────

  Color _eventColor(MotionEvent event) {
    final cls = event.objectClass?.toLowerCase() ?? '';
    if (cls == 'person') return Colors.blue;
    if (cls == 'car' || cls == 'vehicle' || cls == 'truck') {
      return Colors.green;
    }
    if (event.eventType?.toLowerCase() == 'motion') return Colors.amber;
    return Colors.red;
  }

  String _formatDuration(Duration d) {
    final h = d.inHours.remainder(24).toString().padLeft(2, '0');
    final m = d.inMinutes.remainder(60).toString().padLeft(2, '0');
    final s = d.inSeconds.remainder(60).toString().padLeft(2, '0');
    return '$h:$m:$s';
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
        old.segments.length != segments.length ||
        old.motionEvents.length != motionEvents.length ||
        old.selectedDate != selectedDate ||
        old.dragging != dragging;
  }
}
