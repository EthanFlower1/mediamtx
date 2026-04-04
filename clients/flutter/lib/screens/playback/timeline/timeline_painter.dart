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

  TimelinePainter({
    required this.centerTime,
    required this.pixelsPerSecond,
    required this.segments,
    required this.events,
    required this.bookmarks,
    required this.intensityBuckets,
    required this.bucketDuration,
    required this.dayStart,
  });

  // Layout constants
  static const double _recordingTop = 0;
  static const double _recordingHeight = 18;
  static const double _eventTop = 21;
  static const double _eventHeight = 14;
  static const double _motionEventTop = 37;
  static const double _motionEventHeight = 10;
  static const double _bookmarkY = 49;

  // Color-coded segment colors per acceptance criteria.
  static const Color _continuousColor = Color(0xFF3B82F6); // blue
  static const Color _eventColor = Color(0xFFF97316);       // orange
  static const Color _gapColor = Color(0xFF525252);         // gray

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
      ..color = NvrColors.border
      ..strokeWidth = 0.5;

    final minorPaint = Paint()
      ..color = NvrColors.bgTertiary
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
      color: NvrColors.textMuted,
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

    // Build a set of times covered by motion events for overlap detection.
    final eventIntervals = <({double start, double end})>[];
    for (final evt in events) {
      final evtStart =
          evt.startTime.difference(dayStart).inMilliseconds / 1000.0;
      final evtEnd = (evt.endTime ?? evt.startTime.add(const Duration(seconds: 5)))
          .difference(dayStart).inMilliseconds / 1000.0;
      eventIntervals.add((start: evtStart, end: evtEnd));
    }

    final continuousPaint = Paint()
      ..color = _continuousColor.withValues(alpha: 0.35);
    final eventOverlayPaint = Paint()
      ..color = _eventColor.withValues(alpha: 0.45);
    final gapPaint = Paint()
      ..color = _gapColor.withValues(alpha: 0.20);

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

      // Draw gray gap fill between this segment and the previous one.
      if (prevSegment != null) {
        final prevEndSeconds =
            prevSegment.endTime.difference(dayStart).inMilliseconds / 1000.0;
        if (segStartSeconds > prevEndSeconds) {
          final gx1 = _timeToX(prevEndSeconds, size.width)
              .clamp(0.0, size.width);
          final gx2 = _timeToX(segStartSeconds, size.width)
              .clamp(0.0, size.width);
          canvas.drawRect(
            Rect.fromLTRB(
                gx1, _recordingTop, gx2, _recordingTop + _recordingHeight),
            gapPaint,
          );
        }
      }

      final x1 = _timeToX(segStartSeconds, size.width)
          .clamp(0.0, size.width);
      final x2 = _timeToX(segEndSeconds, size.width)
          .clamp(0.0, size.width);

      // Draw the continuous recording bar (blue).
      canvas.drawRect(
        Rect.fromLTRB(x1, _recordingTop, x2, _recordingTop + _recordingHeight),
        continuousPaint,
      );

      // Overlay orange on portions that overlap with motion events.
      for (final interval in eventIntervals) {
        // Check for overlap with this segment.
        final overlapStart =
            interval.start < segStartSeconds ? segStartSeconds : interval.start;
        final overlapEnd =
            interval.end > segEndSeconds ? segEndSeconds : interval.end;
        if (overlapStart >= overlapEnd) continue;

        // Cull if outside visible range.
        if (overlapEnd < range.startSeconds ||
            overlapStart > range.endSeconds) {
          continue;
        }

        final ox1 = _timeToX(overlapStart, size.width).clamp(0.0, size.width);
        final ox2 = _timeToX(overlapEnd, size.width).clamp(0.0, size.width);
        canvas.drawRect(
          Rect.fromLTRB(
              ox1, _recordingTop, ox2, _recordingTop + _recordingHeight),
          eventOverlayPaint,
        );
      }

      prevSegment = seg;
    }
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
        ..color = _eventColor.withValues(alpha: opacity);

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

      final paint = Paint()..color = _eventColor.withValues(alpha: 0.7);

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
      ..color = NvrColors.accent
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
        oldDelegate.dayStart != dayStart;
  }
}
