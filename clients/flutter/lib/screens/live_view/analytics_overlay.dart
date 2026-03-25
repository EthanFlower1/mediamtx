import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/detection_frame.dart';
import '../../providers/auth_provider.dart';
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

/// Parses a detection object from the REST `/cameras/:id/detections/latest`
/// response, which uses `box_x`, `box_y`, `box_w`, `box_h` keys.
DetectionBox _boxFromRestJson(Map<String, dynamic> json) {
  return DetectionBox(
    className: json['class'] as String? ?? '',
    confidence: (json['confidence'] as num?)?.toDouble() ?? 0.0,
    trackId: json['track_id']?.toString(),
    x: (json['box_x'] as num?)?.toDouble() ?? 0.0,
    y: (json['box_y'] as num?)?.toDouble() ?? 0.0,
    w: (json['box_w'] as num?)?.toDouble() ?? 0.0,
    h: (json['box_h'] as num?)?.toDouble() ?? 0.0,
  );
}

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
  Timer? _pollTimer;
  List<DetectionBox> _polledDetections = [];
  bool _isPolling = false;

  @override
  void initState() {
    super.initState();
    _pollTimer = Timer.periodic(const Duration(seconds: 1), (_) => _pollDetections());
  }

  Future<void> _pollDetections() async {
    if (_isPolling) return; // skip if previous request still in-flight
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    _isPolling = true;
    try {
      final res = await api.get<dynamic>('/cameras/${widget.cameraId}/detections/latest');
      final raw = res.data;
      if (raw is List && mounted) {
        final boxes = raw
            .whereType<Map<String, dynamic>>()
            .map(_boxFromRestJson)
            .toList();
        setState(() => _polledDetections = boxes);
      }
    } catch (_) {
      // Silently ignore poll errors — overlay degrades gracefully.
    } finally {
      _isPolling = false;
    }
  }

  @override
  void dispose() {
    _pollTimer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final frameAsync = ref.watch(detectionStreamProvider(widget.cameraName));

    // Prefer live WebSocket data when available; fall back to REST poll.
    final detections = frameAsync.maybeWhen(
      data: (frame) => frame.detections.isNotEmpty ? frame.detections : _polledDetections,
      orElse: () => _polledDetections,
    );

    if (detections.isEmpty) return const SizedBox.expand();

    return SizedBox.expand(
      child: CustomPaint(
        painter: _DetectionPainter(detections),
      ),
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
