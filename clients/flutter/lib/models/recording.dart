class RecordingSegment {
  final String start;

  const RecordingSegment({required this.start});

  factory RecordingSegment.fromJson(Map<String, dynamic> json) {
    return RecordingSegment(start: json['start'] as String);
  }

  DateTime get startTime => DateTime.parse(start);
}

class MotionEvent {
  final String id;
  final String cameraId;
  final String startedAt;
  final String? endedAt;
  final String? thumbnailPath;
  final String? eventType;
  final String? objectClass;
  final double? confidence;

  const MotionEvent({
    required this.id,
    required this.cameraId,
    required this.startedAt,
    this.endedAt,
    this.thumbnailPath,
    this.eventType,
    this.objectClass,
    this.confidence,
  });

  factory MotionEvent.fromJson(Map<String, dynamic> json) {
    return MotionEvent(
      id: json['id'] as String,
      cameraId: json['camera_id'] as String,
      startedAt: json['started_at'] as String,
      endedAt: json['ended_at'] as String?,
      thumbnailPath: json['thumbnail_path'] as String?,
      eventType: json['event_type'] as String?,
      objectClass: json['object_class'] as String?,
      confidence: (json['confidence'] as num?)?.toDouble(),
    );
  }

  DateTime get startTime => DateTime.parse(startedAt);

  DateTime? get endTime =>
      endedAt != null ? DateTime.parse(endedAt!) : null;
}
