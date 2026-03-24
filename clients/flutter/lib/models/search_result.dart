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
      confidence: (json['confidence'] as num?)?.toDouble() ?? 0.0,
      similarity: (json['similarity'] as num?)?.toDouble() ?? 0.0,
      frameTime: json['frame_time']?.toString() ?? '',
      thumbnailPath: json['thumbnail_path']?.toString(),
    );
  }

  DateTime get time => DateTime.parse(frameTime);
}
