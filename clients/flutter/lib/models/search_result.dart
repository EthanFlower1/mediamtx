class SearchResult {
  final String detectionId;
  final String eventId;
  final String cameraId;
  final String cameraName;
  final String className;
  final double confidence;
  final double similarity;
  final String frameTime;
  final String? thumbnailPath;

  const SearchResult({
    required this.detectionId,
    required this.eventId,
    required this.cameraId,
    required this.cameraName,
    required this.className,
    required this.confidence,
    required this.similarity,
    required this.frameTime,
    this.thumbnailPath,
  });

  factory SearchResult.fromJson(Map<String, dynamic> json) {
    return SearchResult(
      detectionId: json['detection_id']?.toString() ?? '',
      eventId: json['event_id']?.toString() ?? '',
      cameraId: json['camera_id']?.toString() ?? '',
      cameraName: json['camera_name']?.toString() ?? '',
      className: json['class']?.toString() ?? '',
      confidence: _toDouble(json['confidence']),
      similarity: _toDouble(json['similarity']),
      frameTime: json['frame_time']?.toString() ?? '',
      thumbnailPath: json['thumbnail_path']?.toString(),
    );
  }

  /// Safely converts a JSON value (int, double, String, or null) to double.
  static double _toDouble(dynamic value) {
    if (value == null) return 0.0;
    if (value is double) return value;
    if (value is int) return value.toDouble();
    if (value is String) return double.tryParse(value) ?? 0.0;
    return 0.0;
  }

  /// Parses [frameTime] into a [DateTime], returning epoch zero on failure.
  DateTime get time {
    if (frameTime.isEmpty) return DateTime.fromMillisecondsSinceEpoch(0);
    return DateTime.tryParse(frameTime) ??
        DateTime.fromMillisecondsSinceEpoch(0);
  }
}
