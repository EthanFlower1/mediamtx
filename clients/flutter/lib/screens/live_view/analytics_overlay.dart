import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/detection_frame.dart';
import '../../providers/detection_stream_provider.dart';
import '../../theme/nvr_colors.dart';

String _labelForBox(DetectionBox box) {
  final pct = (box.confidence * 100).round();
  final cls = _capitalise(box.className);
  final id = box.trackId != null ? ' #${box.trackId}' : '';
  return '$cls$id $pct%';
}

String _capitalise(String s) =>
    s.isEmpty ? s : '${s[0].toUpperCase()}${s.substring(1)}';

class AnalyticsOverlay extends ConsumerStatefulWidget {
  final String cameraName;
  final String cameraId;

  const AnalyticsOverlay({
    super.key,
    required this.cameraName,
    required this.cameraId,
  });

  @override
  ConsumerState<AnalyticsOverlay> createState() => _AnalyticsOverlayState();
}

class _AnalyticsOverlayState extends ConsumerState<AnalyticsOverlay> {
  @override
  Widget build(BuildContext context) {
    final frameAsync = ref.watch(detectionStreamProvider(
      (cameraId: widget.cameraId, cameraName: widget.cameraName),
    ));

    final detections = frameAsync.maybeWhen(
      data: (frame) => frame.detections,
      orElse: () => <DetectionBox>[],
    );

    if (detections.isEmpty) return const SizedBox.expand();

    return SizedBox.expand(
      child: CustomPaint(
        painter: _DetectionPainter(detections, NvrColors.of(context)),
      ),
    );
  }
}

class _DetectionPainter extends CustomPainter {
  final List<DetectionBox> detections;
  final NvrColors colors;

  const _DetectionPainter(this.detections, this.colors);

  @override
  void paint(Canvas canvas, Size size) {
    final boxColor = colors.accent;
    final labelTextColor = colors.bgPrimary;
    final labelBgColor = colors.accent;

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

      // Draw bounding box -- 2px solid accent
      canvas.drawRect(rect, boxPaint);

      // Draw label above box: class + confidence on accent background pill
      final label = _labelForBox(box);
      final tp = TextPainter(
        text: const TextSpan(
          text: '', // placeholder; rebuilt per box below
        ),
        textDirection: TextDirection.ltr,
      );

      final labelSpan = TextSpan(
        text: label,
        style: TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 8,
          fontWeight: FontWeight.w700,
          color: labelTextColor,
        ),
      );
      tp.text = labelSpan;
      tp.layout();

      const hPad = 3.0;
      const vPad = 2.0;
      final labelW = tp.width + hPad * 2;
      final labelH = tp.height + vPad * 2;
      final labelTop = (top - labelH).clamp(0.0, size.height - labelH);
      final labelLeft = left.clamp(0.0, size.width - labelW);

      // Pill background (2px border-radius)
      final bgRect =
          RRect.fromLTRBR(labelLeft, labelTop, labelLeft + labelW,
              labelTop + labelH, const Radius.circular(2));
      canvas.drawRRect(bgRect, Paint()..color = labelBgColor);

      tp.paint(canvas, Offset(labelLeft + hPad, labelTop + vPad));
    }
  }

  @override
  bool shouldRepaint(_DetectionPainter old) =>
      old.detections != detections || old.colors != colors;
}
