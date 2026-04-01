class DetectionBox {
  final String className;
  final double confidence;
  final String? trackId;
  final double x;
  final double y;
  final double w;
  final double h;

  const DetectionBox({
    required this.className,
    required this.confidence,
    this.trackId,
    required this.x,
    required this.y,
    required this.w,
    required this.h,
  });

  factory DetectionBox.fromJson(Map<String, dynamic> json) {
    return DetectionBox(
      className: json['class'] as String? ?? '',
      confidence: (json['confidence'] as num?)?.toDouble() ?? 0.0,
      trackId: json['trackId'] as String?,
      x: (json['x'] as num?)?.toDouble() ?? 0.0,
      y: (json['y'] as num?)?.toDouble() ?? 0.0,
      w: (json['w'] as num?)?.toDouble() ?? 0.0,
      h: (json['h'] as num?)?.toDouble() ?? 0.0,
    );
  }
}

class DetectionFrame {
  final String camera;
  final DateTime time;
  final List<DetectionBox> detections;

  const DetectionFrame({
    required this.camera,
    required this.time,
    required this.detections,
  });

  factory DetectionFrame.fromJson(Map<String, dynamic> json) {
    final rawDetections = json['detections'] as List<dynamic>? ?? [];
    return DetectionFrame(
      camera: json['camera'] as String? ?? '',
      time: json['time'] != null
          ? DateTime.parse(json['time'] as String)
          : DateTime.now(),
      detections: rawDetections
          .map((d) => DetectionBox.fromJson(d as Map<String, dynamic>))
          .toList(),
    );
  }
}

/// A detection from a recorded segment, used during playback.
/// Includes [frameTime] for timestamp-based lookup that [DetectionBox] lacks.
class PlaybackDetection {
  final DateTime frameTime;
  final String className;
  final double confidence;
  final double x;
  final double y;
  final double w;
  final double h;

  const PlaybackDetection({
    required this.frameTime,
    required this.className,
    required this.confidence,
    required this.x,
    required this.y,
    required this.w,
    required this.h,
  });

  factory PlaybackDetection.fromJson(Map<String, dynamic> json) {
    return PlaybackDetection(
      frameTime: DateTime.parse(json['frame_time'] as String),
      className: json['class'] as String? ?? '',
      confidence: (json['confidence'] as num?)?.toDouble() ?? 0.0,
      x: (json['box_x'] as num?)?.toDouble() ?? 0.0,
      y: (json['box_y'] as num?)?.toDouble() ?? 0.0,
      w: (json['box_w'] as num?)?.toDouble() ?? 0.0,
      h: (json['box_h'] as num?)?.toDouble() ?? 0.0,
    );
  }

  DetectionBox toDetectionBox() => DetectionBox(
        className: className,
        confidence: confidence,
        x: x,
        y: y,
        w: w,
        h: h,
      );
}
