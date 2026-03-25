import 'package:flutter/material.dart';
import '../../../providers/timeline_intensity_provider.dart';
import 'timeline_viewport.dart';

class IntensityLayer extends CustomPainter {
  final TimelineViewport viewport;
  final List<IntensityBucket> buckets;
  final int bucketSeconds;
  final DateTime dayStart;

  IntensityLayer({
    required this.viewport,
    required this.buckets,
    required this.bucketSeconds,
    required this.dayStart,
  });

  @override
  void paint(Canvas canvas, Size size) {
    if (buckets.isEmpty) return;

    final maxCount =
        buckets.map((b) => b.count).reduce((a, b) => a > b ? a : b);
    if (maxCount == 0) return;

    for (final bucket in buckets) {
      final startDur = Duration(
        seconds: bucket.bucketStart.difference(dayStart).inSeconds,
      );
      final endDur = startDur + Duration(seconds: bucketSeconds);

      final x1 = viewport.timeToPixel(startDur);
      final x2 = viewport.timeToPixel(endDur);

      if (x2 < 0 || x1 > viewport.widthPx) continue;

      final intensity = bucket.count / maxCount;
      final barHeight = size.height * 0.6 * intensity;

      final paint = Paint()
        ..color = Colors.red.withOpacity(0.15 + 0.35 * intensity)
        ..style = PaintingStyle.fill;

      canvas.drawRect(
        Rect.fromLTRB(x1, size.height - barHeight, x2, size.height),
        paint,
      );
    }
  }

  @override
  bool shouldRepaint(covariant IntensityLayer oldDelegate) =>
      buckets != oldDelegate.buckets || viewport != oldDelegate.viewport;
}
