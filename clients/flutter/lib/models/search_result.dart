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
      detectionId: json['detection_id'] as String,
      eventId: json['event_id'] as String,
      cameraId: json['camera_id'] as String,
      cameraName: json['camera_name'] as String,
      className: json['class'] as String,
      confidence: (json['confidence'] as num).toDouble(),
      similarity: (json['similarity'] as num).toDouble(),
      frameTime: json['frame_time'] as String,
      thumbnailPath: json['thumbnail_path'] as String?,
    );
  }

  DateTime get time => DateTime.parse(frameTime);
}
