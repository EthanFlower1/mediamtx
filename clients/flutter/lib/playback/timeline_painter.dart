// KAI-302 — Timeline CustomPainter.
//
// Draws the assembled `TimelineSpan` into a rect. Pure paint logic —
// widget tests can drive it with a RecordingCanvas and assert on the
// captured draw calls.
//
// Rendering layers (bottom → top):
//   1. background bar
//   2. segments (filled rect per segment; stripe pattern if `hasGap`)
//   3. recorder boundaries (vertical dividers; wider + labelled when the
//      boundary crosses a directory)
//   4. event markers (small triangles above the bar, colored by kind)

import 'package:flutter/material.dart';

import 'timeline_model.dart';

/// Palette used by the painter. Overridable from tests.
class TimelinePalette {
  final Color background;
  final Color segmentFill;
  final Color gapStripe;
  final Color recorderBoundary;
  final Color directoryBoundary;
  final Color markerMotion;
  final Color markerFace;
  final Color markerLpr;
  final Color markerManual;
  final Color markerSystem;

  const TimelinePalette({
    this.background = const Color(0xFF1A1A1F),
    this.segmentFill = const Color(0xFF3B82F6),
    this.gapStripe = const Color(0xFF52525B),
    this.recorderBoundary = const Color(0xFFA1A1AA),
    this.directoryBoundary = const Color(0xFFFBBF24),
    this.markerMotion = const Color(0xFF10B981),
    this.markerFace = const Color(0xFFF59E0B),
    this.markerLpr = const Color(0xFFEF4444),
    this.markerManual = const Color(0xFF8B5CF6),
    this.markerSystem = const Color(0xFF64748B),
  });

  Color forKind(EventKind kind) {
    switch (kind) {
      case EventKind.motion:
        return markerMotion;
      case EventKind.face:
        return markerFace;
      case EventKind.lpr:
        return markerLpr;
      case EventKind.manual:
        return markerManual;
      case EventKind.system:
        return markerSystem;
    }
  }
}

class TimelinePainter extends CustomPainter {
  final TimelineSpan span;
  final double zoom;
  final TimelinePalette palette;

  TimelinePainter({
    required this.span,
    this.zoom = 1.0,
    this.palette = const TimelinePalette(),
  });

  double _xFor(DateTime t, Rect rect) {
    final totalMs = span.duration.inMilliseconds;
    if (totalMs == 0) return rect.left;
    final tMs = t.difference(span.start).inMilliseconds;
    final frac = (tMs / totalMs).clamp(0.0, 1.0);
    return rect.left + frac * rect.width;
  }

  @override
  void paint(Canvas canvas, Size size) {
    final rect = Offset.zero & size;
    final barTop = size.height * 0.35;
    final barBottom = size.height * 0.70;
    final barRect = Rect.fromLTRB(rect.left, barTop, rect.right, barBottom);

    // 1. background
    canvas.drawRect(rect, Paint()..color = palette.background);

    // 2. segments
    for (final seg in span.segments) {
      final x0 = _xFor(seg.startedAt, rect);
      final x1 = _xFor(seg.endedAt, rect);
      final r = Rect.fromLTRB(x0, barRect.top, x1, barRect.bottom);
      canvas.drawRect(r, Paint()..color = palette.segmentFill);

      if (seg.hasGap) {
        // Stripe pattern on the leading edge.
        final stripeWidth = 2.0;
        final stripePaint = Paint()..color = palette.gapStripe;
        for (var sx = x0; sx < x0 + 8 && sx < x1; sx += stripeWidth * 2) {
          canvas.drawRect(
              Rect.fromLTWH(sx, barRect.top, stripeWidth, barRect.height),
              stripePaint);
        }
      }
    }

    // 3. boundaries
    for (final b in span.boundaries) {
      final x = _xFor(b.at, rect);
      final isDir = b.crossesDirectory;
      final p = Paint()
        ..color = isDir ? palette.directoryBoundary : palette.recorderBoundary
        ..strokeWidth = isDir ? 2.5 : 1.5;
      canvas.drawLine(
          Offset(x, barRect.top - 4), Offset(x, barRect.bottom + 4), p);
    }

    // 4. markers
    for (final m in span.markers) {
      final x = _xFor(m.at, rect);
      final color = palette.forKind(m.kind);
      final triTop = barRect.top - 8;
      final path = Path()
        ..moveTo(x - 4, triTop)
        ..lineTo(x + 4, triTop)
        ..lineTo(x, triTop + 7)
        ..close();
      canvas.drawPath(path, Paint()..color = color);

      final dur = m.durationMs;
      if (dur != null && dur > 0) {
        final endX = _xFor(
            m.at.add(Duration(milliseconds: dur)).isAfter(span.end)
                ? span.end
                : m.at.add(Duration(milliseconds: dur)),
            rect);
        canvas.drawLine(Offset(x, barRect.bottom + 2),
            Offset(endX, barRect.bottom + 2), Paint()..color = color..strokeWidth = 2);
      }
    }
  }

  @override
  bool shouldRepaint(covariant TimelinePainter old) =>
      old.span != span || old.zoom != zoom;
}
