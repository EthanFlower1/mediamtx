import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/detection_frame.dart';
import '../../providers/detection_stream_provider.dart';
import '../../theme/nvr_colors.dart';

/// Color-coded class labels for bounding boxes.
Color _colorForClass(String className) {
  final c = className.toLowerCase();
  if (c == 'person') return NvrColors.accent; // blue
  if (c == 'car' || c == 'truck' || c == 'bus' || c == 'vehicle') {
    return NvrColors.success; // green
  }
  if (c == 'cat' || c == 'dog' || c == 'animal') return NvrColors.warning; // amber
  return NvrColors.danger; // red for other
}

String _labelForBox(DetectionBox box) {
  final pct = (box.confidence * 100).round();
  final cls = _capitalise(box.className);
  final id = box.trackId != null ? ' #${box.trackId}' : '';
  return '$cls$id $pct%';
}

String _capitalise(String s) =>
    s.isEmpty ? s : '${s[0].toUpperCase()}${s.substring(1)}';

class AnalyticsOverlay extends ConsumerWidget {
  final String cameraName;

  const AnalyticsOverlay({super.key, required this.cameraName});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final frameAsync = ref.watch(detectionStreamProvider(cameraName));

    return frameAsync.when(
      loading: () => const SizedBox.shrink(),
      error: (_, __) => const SizedBox.shrink(),
      data: (frame) {
        if (frame.detections.isEmpty) return const SizedBox.shrink();
        return CustomPaint(
          painter: _DetectionPainter(frame.detections),
          child: const SizedBox.expand(),
        );
      },
    );
  }
}

class _DetectionPainter extends CustomPainter {
  final List<DetectionBox> detections;

  const _DetectionPainter(this.detections);

  @override
  void paint(Canvas canvas, Size size) {
    for (final box in detections) {
      final color = _colorForClass(box.className);
      final left = box.x * size.width;
      final top = box.y * size.height;
      final right = left + box.w * size.width;
      final bottom = top + box.h * size.height;
      final rect = Rect.fromLTRB(left, top, right, bottom);

      // Draw bounding box
      final boxPaint = Paint()
        ..color = color
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2;
      canvas.drawRect(rect, boxPaint);

      // Draw label background above box
      final label = _labelForBox(box);
      final tp = TextPainter(
        text: TextSpan(
          text: label,
          style: const TextStyle(
            color: Colors.white,
            fontSize: 11,
            fontWeight: FontWeight.w600,
          ),
        ),
        textDirection: TextDirection.ltr,
      )..layout();

      const padding = 3.0;
      final labelW = tp.width + padding * 2;
      final labelH = tp.height + padding * 2;
      final labelTop = (top - labelH).clamp(0.0, size.height - labelH);
      final labelLeft = left.clamp(0.0, size.width - labelW);

      final bgRect = Rect.fromLTWH(labelLeft, labelTop, labelW, labelH);
      canvas.drawRect(bgRect, Paint()..color = color);

      tp.paint(canvas, Offset(labelLeft + padding, labelTop + padding));
    }
  }

  @override
  bool shouldRepaint(_DetectionPainter old) => old.detections != detections;
}
