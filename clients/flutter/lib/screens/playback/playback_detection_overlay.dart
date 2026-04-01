import 'package:flutter/material.dart';

import '../../models/detection_frame.dart';
import '../../theme/nvr_colors.dart';

/// Renders bounding boxes for historical detections during playback.
/// Takes a flat list of [DetectionBox] and paints them over the video.
class PlaybackDetectionOverlay extends StatelessWidget {
  final List<DetectionBox> detections;

  const PlaybackDetectionOverlay({
    super.key,
    required this.detections,
  });

  @override
  Widget build(BuildContext context) {
    if (detections.isEmpty) return const SizedBox.shrink();
    return SizedBox.expand(
      child: CustomPaint(
        painter: _PlaybackDetectionPainter(detections),
      ),
    );
  }
}

class _PlaybackDetectionPainter extends CustomPainter {
  final List<DetectionBox> detections;

  const _PlaybackDetectionPainter(this.detections);

  @override
  void paint(Canvas canvas, Size size) {
    const boxColor = NvrColors.accent;
    const labelTextColor = NvrColors.bgPrimary;
    const labelBgColor = NvrColors.accent;

    final boxPaint = Paint()
      ..color = boxColor
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2;

    for (final box in detections) {
      final left = box.x * size.width;
      final top = box.y * size.height;
      final right = left + box.w * size.width;
      final bottom = top + box.h * size.height;
      final rect = Rect.fromLTRB(left, top, right, bottom);

      canvas.drawRect(rect, boxPaint);

      // Label: "Class 95%"
      final pct = (box.confidence * 100).round();
      final cls =
          box.className.isEmpty ? '' : '${box.className[0].toUpperCase()}${box.className.substring(1)}';
      final label = '$cls $pct%';

      final tp = TextPainter(
        text: TextSpan(
          text: label,
          style: const TextStyle(
            fontFamily: 'JetBrainsMono',
            fontSize: 8,
            fontWeight: FontWeight.w700,
            color: labelTextColor,
          ),
        ),
        textDirection: TextDirection.ltr,
      );
      tp.layout();

      const hPad = 3.0;
      const vPad = 2.0;
      final labelW = tp.width + hPad * 2;
      final labelH = tp.height + vPad * 2;
      final labelTop = (top - labelH).clamp(0.0, size.height - labelH);
      final labelLeft = left.clamp(0.0, size.width - labelW);

      final bgRect = RRect.fromLTRBR(
        labelLeft,
        labelTop,
        labelLeft + labelW,
        labelTop + labelH,
        const Radius.circular(2),
      );
      canvas.drawRRect(bgRect, Paint()..color = labelBgColor);
      tp.paint(canvas, Offset(labelLeft + hPad, labelTop + vPad));
    }
  }

  @override
  bool shouldRepaint(_PlaybackDetectionPainter old) =>
      old.detections != detections;
}
