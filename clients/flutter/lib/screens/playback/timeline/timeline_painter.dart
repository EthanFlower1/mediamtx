import 'dart:ui' as ui;

import 'package:flutter/material.dart';

import '../../../models/bookmark.dart';
import '../../../models/recording.dart';
import '../../../providers/timeline_intensity_provider.dart';
import '../../../theme/nvr_colors.dart';

/// A single CustomPainter that renders all timeline layers in one paint call.
///
/// The painter uses a fixed-center playhead model: all content is positioned
/// relative to [centerTime], which sits at the horizontal center of the widget.
/// [pixelsPerSecond] controls the zoom level.
class TimelinePainter extends CustomPainter {
  /// The time at the horizontal center of the widget (duration from midnight).
  final Duration centerTime;

  /// Zoom level: higher values = more zoomed in.
  final double pixelsPerSecond;

  /// Recording segments for the selected day.
  final List<RecordingSegment> segments;

  /// Motion events for the selected day.
  final List<MotionEvent> events;

  /// User bookmarks for the selected day.
  final List<Bookmark> bookmarks;

  /// Motion intensity buckets (pre-aggregated counts).
  final List<IntensityBucket> intensityBuckets;

  /// Duration of each intensity bucket.
  final Duration bucketDuration;

  /// The start of the selected day (midnight).
  final DateTime dayStart;

  /// Theme colors (passed from the widget that owns context).
  final NvrColors colors;

  TimelinePainter({
    required this.centerTime,
    required this.pixelsPerSecond,
    required this.segments,
    required this.events,
    required this.bookmarks,
    required this.intensityBuckets,
    required this.bucketDuration,
    required this.dayStart,
    required this.colors,
  });

  // Layout constants
  static const double _recordingTop = 0;
  static const double _recordingHeight = 18;
  static const double _eventTop = 21;
  static const double _eventHeight = 14;
  static const double _motionEventTop = 37;
  static const double _motionEventHeight = 10;
  static const double _bookmarkY = 49;

  // Color map for object classes.
  static const Map<String, Color> _objectClassColors = {
    'person': Color(0xFFF59E0B),   // amber/orange
    'vehicle': Color(0xFF3B82F6),  // blue
    'car': Color(0xFF3B82F6),      // blue (alias)
    'truck': Color(0xFF3B82F6),    // blue (alias)
    'animal': Color(0xFF22C55E),   // green
    'dog': Color(0xFF22C55E),      // green (alias)
    'cat': Color(0xFF22C55E),      // green (alias)
  };
  static const Color _defaultEventColor = Color(0xFF525252); // muted gray

  /// Convert a time (seconds from midnight) to an x pixel position.
  double _timeToX(double timeSeconds, double widgetWidth) {
    final centerSeconds = centerTime.inMilliseconds / 1000.0;
    return (timeSeconds - centerSeconds) * pixelsPerSecond + widgetWidth / 2;
  }

  /// Determine the visible time range in seconds for culling.
  ({double startSeconds, double endSeconds}) _visibleRange(double widgetWidth) {
    final centerSeconds = centerTime.inMilliseconds / 1000.0;
    final halfWidthSeconds = (widgetWidth / 2) / pixelsPerSecond;
    return (
      startSeconds: centerSeconds - halfWidthSeconds,
      endSeconds: centerSeconds + halfWidthSeconds,
    );
  }

  @override
  void paint(Canvas canvas, Size size) {
    final range = _visibleRange(size.width);

    _paintTimeGrid(canvas, size, range);
    _paintRecordingSegments(canvas, size, range);
    _paintMotionIntensity(canvas, size, range);
    _paintMotionEvents(canvas, size, range);
    _paintBookmarks(canvas, size, range);
  }

  // ─── Layer 1: Time Grid ──────────────────────────────────────────────

  void _paintTimeGrid(
    Canvas canvas,
    Size size,
    ({double startSeconds, double endSeconds}) range,
  ) {
    final majorPaint = Paint()
      ..color = colors.border
      ..strokeWidth = 0.5;

    final minorPaint = Paint()
      ..color = colors.bgTertiary
      ..strokeWidth = 0.5;

    // Choose grid intervals based on zoom level.
    // The visible duration determines which intervals to show.
    final visibleDuration = range.endSeconds - range.startSeconds;
    late final double majorIntervalSeconds;
    late final double minorIntervalSeconds;

    if (visibleDuration > 7200) {
      // > 2h visible: major every 1h, minor every 15min
      majorIntervalSeconds = 3600;
      minorIntervalSeconds = 900;
    } else if (visibleDuration > 3600) {
      // 1-2h visible: major every 30min, minor every 5min
      majorIntervalSeconds = 1800;
      minorIntervalSeconds = 300;
    } else if (visibleDuration > 1200) {
      // 20min-1h visible: major every 10min, minor every 1min
      majorIntervalSeconds = 600;
      minorIntervalSeconds = 60;
    } else {
      // < 20min visible: major every 5min, minor every 1min
      majorIntervalSeconds = 300;
      minorIntervalSeconds = 60;
    }

    // Draw minor grid lines.
    final minorStart =
        (range.startSeconds / minorIntervalSeconds).floor() *
            minorIntervalSeconds;
    for (var t = minorStart;
        t <= range.endSeconds;
        t += minorIntervalSeconds) {
      if (t < 0 || t > 86400) continue;
      // Skip positions that coincide with major lines.
      if ((t % majorIntervalSeconds).abs() < 0.001) continue;
      final x = _timeToX(t, size.width);
      canvas.drawLine(Offset(x, 0), Offset(x, size.height), minorPaint);
    }

    // Draw major grid lines with time labels.
    final majorStart =
        (range.startSeconds / majorIntervalSeconds).floor() *
            majorIntervalSeconds;

    final labelStyle = ui.TextStyle(
      color: colors.textMuted,
      fontSize: 9,
      fontFamily: 'JetBrainsMono',
      fontWeight: FontWeight.w500,
    );

    for (var t = majorStart;
        t <= range.endSeconds;
        t += majorIntervalSeconds) {
      if (t < 0 || t > 86400) continue;
      final x = _timeToX(t, size.width);
      canvas.drawLine(Offset(x, 0), Offset(x, size.height), majorPaint);

      // Time label.
      final totalSeconds = t.round();
      final hours = (totalSeconds ~/ 3600) % 24;
      final minutes = (totalSeconds % 3600) ~/ 60;
      final label = '${hours.toString().padLeft(2, '0')}:'
          '${minutes.toString().padLeft(2, '0')}';

      final builder = ui.ParagraphBuilder(ui.ParagraphStyle(
        textDirection: TextDirection.ltr,
        textAlign: TextAlign.left,
      ))
        ..pushStyle(labelStyle)
        ..addText(label);

      final paragraph = builder.build()
        ..layout(const ui.ParagraphConstraints(width: 60));

      canvas.drawParagraph(paragraph, Offset(x + 3, size.height - 13));
    }
  }

  // ─── Layer 2: Recording Segments ─────────────────────────────────────

  void _paintRecordingSegments(
    Canvas canvas,
    Size size,
    ({double startSeconds, double endSeconds}) range,
  ) {
    if (segments.isEmpty) return;

    final fillPaint = Paint()..color = colors.accent.withValues(alpha: 0.20);

    // Diagonal hash paint for gaps.
    final gapPaint = Paint()
      ..color = colors.textMuted.withValues(alpha: 0.15)
      ..strokeWidth = 0.5
      ..style = PaintingStyle.stroke;

    RecordingSegment? prevSegment;

    for (final seg in segments) {
      final segStartSeconds =
          seg.startTime.difference(dayStart).inMilliseconds / 1000.0;
      final segEndSeconds =
          seg.endTime.difference(dayStart).inMilliseconds / 1000.0;

      // Cull segments completely outside the visible range.
      if (segEndSeconds < range.startSeconds ||
          segStartSeconds > range.endSeconds) {
        prevSegment = seg;
        continue;
      }

      // Draw gap hash pattern between this segment and the previous one.
      if (prevSegment != null) {
        final prevEndSeconds =
            prevSegment.endTime.difference(dayStart).inMilliseconds / 1000.0;
        if (segStartSeconds > prevEndSeconds) {
          _paintGapHash(
            canvas,
            size,
            _timeToX(prevEndSeconds, size.width),
            _timeToX(segStartSeconds, size.width),
            gapPaint,
          );
        }
      }

      final x1 = _timeToX(segStartSeconds, size.width)
          .clamp(0.0, size.width);
      final x2 = _timeToX(segEndSeconds, size.width)
          .clamp(0.0, size.width);

      canvas.drawRect(
        Rect.fromLTRB(x1, _recordingTop, x2, _recordingTop + _recordingHeight),
        fillPaint,
      );

      prevSegment = seg;
    }
  }

  void _paintGapHash(
    Canvas canvas,
    Size size,
    double gapX1,
    double gapX2,
    Paint paint,
  ) {
    // Clamp to visible area.
    final left = gapX1.clamp(0.0, size.width);
    final right = gapX2.clamp(0.0, size.width);
    if (right - left < 2) return;

    canvas.save();
    canvas.clipRect(
      Rect.fromLTRB(left, _recordingTop, right, _recordingTop + _recordingHeight),
    );

    const spacing = 6.0;
    final startX = left - _recordingHeight; // Start before clip to cover edges.
    for (var x = startX; x < right + _recordingHeight; x += spacing) {
      canvas.drawLine(
        Offset(x, _recordingTop),
        Offset(x + _recordingHeight, _recordingTop + _recordingHeight),
        paint,
      );
    }

    canvas.restore();
  }

  // ─── Layer 3: Motion/Event Intensity ─────────────────────────────────

  void _paintMotionIntensity(
    Canvas canvas,
    Size size,
    ({double startSeconds, double endSeconds}) range,
  ) {
    if (intensityBuckets.isEmpty) return;

    // Find the max count for normalization.
    int maxCount = 0;
    for (final bucket in intensityBuckets) {
      if (bucket.count > maxCount) maxCount = bucket.count;
    }
    if (maxCount == 0) return;

    final bucketSeconds = bucketDuration.inSeconds.toDouble();

    for (final bucket in intensityBuckets) {
      final bucketStartSeconds =
          bucket.bucketStart.difference(dayStart).inMilliseconds / 1000.0;
      final bucketEndSeconds = bucketStartSeconds + bucketSeconds;

      // Cull buckets outside visible range.
      if (bucketEndSeconds < range.startSeconds ||
          bucketStartSeconds > range.endSeconds) {
        continue;
      }

      final normalizedIntensity = bucket.count / maxCount;
      // Map to opacity: minimum 0.15 for any non-zero bucket, max 0.85.
      final opacity = 0.15 + normalizedIntensity * 0.70;

      final paint = Paint()
        ..color = colors.danger.withValues(alpha: opacity);

      final x1 = _timeToX(bucketStartSeconds, size.width)
          .clamp(0.0, size.width);
      final x2 = _timeToX(bucketEndSeconds, size.width)
          .clamp(0.0, size.width);

      canvas.drawRect(
        Rect.fromLTRB(x1, _eventTop, x2, _eventTop + _eventHeight),
        paint,
      );
    }
  }

  // ─── Layer 3b: Individual Motion Event Markers ──────────────────────

  void _paintMotionEvents(
    Canvas canvas,
    Size size,
    ({double startSeconds, double endSeconds}) range,
  ) {
    if (events.isEmpty) return;

    for (final evt in events) {
      final evtStartSeconds =
          evt.startTime.difference(dayStart).inMilliseconds / 1000.0;
      final evtEnd = evt.endTime ?? evt.startTime.add(const Duration(seconds: 5));
      final evtEndSeconds =
          evtEnd.difference(dayStart).inMilliseconds / 1000.0;

      // Cull events outside the visible range.
      if (evtEndSeconds < range.startSeconds ||
          evtStartSeconds > range.endSeconds) {
        continue;
      }

      final color = _objectClassColors[evt.objectClass?.toLowerCase() ?? '']
          ?? _defaultEventColor;

      final paint = Paint()..color = color.withValues(alpha: 0.7);

      final x1 = _timeToX(evtStartSeconds, size.width).clamp(0.0, size.width);
      final x2 = _timeToX(evtEndSeconds, size.width).clamp(0.0, size.width);

      // Ensure a minimum width of 2px so very short events are visible.
      final minX2 = (x1 + 2).clamp(0.0, size.width);
      final drawX2 = x2 < minX2 ? minX2 : x2;

      canvas.drawRRect(
        RRect.fromLTRBR(
          x1, _motionEventTop, drawX2, _motionEventTop + _motionEventHeight,
          const Radius.circular(1.5),
        ),
        paint,
      );
    }
  }

  // ─── Layer 4: Bookmarks ──────────────────────────────────────────────

  void _paintBookmarks(
    Canvas canvas,
    Size size,
    ({double startSeconds, double endSeconds}) range,
  ) {
    if (bookmarks.isEmpty) return;

    final paint = Paint()
      ..color = colors.accent
      ..style = PaintingStyle.fill;

    const triangleSize = 6.0;

    for (final bm in bookmarks) {
      final bmSeconds =
          bm.timestamp.difference(dayStart).inMilliseconds / 1000.0;

      // Cull bookmarks outside visible range (with a small margin for the triangle).
      if (bmSeconds < range.startSeconds - 10 ||
          bmSeconds > range.endSeconds + 10) {
        continue;
      }

      final x = _timeToX(bmSeconds, size.width);

      // Draw a small downward-pointing triangle.
      final path = Path()
        ..moveTo(x - triangleSize / 2, _bookmarkY)
        ..lineTo(x + triangleSize / 2, _bookmarkY)
        ..lineTo(x, _bookmarkY + triangleSize)
        ..close();

      canvas.drawPath(path, paint);
    }
  }

  // ─── shouldRepaint ───────────────────────────────────────────────────

  @override
  bool shouldRepaint(TimelinePainter oldDelegate) {
    return oldDelegate.centerTime != centerTime ||
        oldDelegate.pixelsPerSecond != pixelsPerSecond ||
        oldDelegate.segments != segments ||
        oldDelegate.events != events ||
        oldDelegate.bookmarks != bookmarks ||
        oldDelegate.intensityBuckets != intensityBuckets ||
        oldDelegate.bucketDuration != bucketDuration ||
        oldDelegate.dayStart != dayStart ||
        oldDelegate.colors != colors;
  }
}
