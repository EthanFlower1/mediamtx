class RecordingSegment {
  final int id;
  final String cameraId;
  final DateTime startTime;
  final DateTime endTime;
  final int durationMs;
  final String? filePath;
  final int? fileSize;
  final String? format;
  final DateTime? mediaStartTime;

  const RecordingSegment({
    required this.id,
    required this.cameraId,
    required this.startTime,
    required this.endTime,
    required this.durationMs,
    this.filePath,
    this.fileSize,
    this.format,
    this.mediaStartTime,
  });

  /// The most accurate start time available. Prefers NTP-derived media
  /// timestamp; falls back to DB wall-clock start_time.
  DateTime get effectiveStartTime => mediaStartTime ?? startTime;

  /// Effective end time derived from media start + duration when available.
  DateTime get effectiveEndTime => mediaStartTime != null
      ? mediaStartTime!.add(Duration(milliseconds: durationMs))
      : endTime;

  factory RecordingSegment.fromJson(Map<String, dynamic> json) {
    return RecordingSegment(
      id: json['id'] as int,
      cameraId: json['camera_id'] as String,
      startTime: DateTime.parse(json['start_time'] as String).toLocal(),
      endTime: DateTime.parse(json['end_time'] as String).toLocal(),
      durationMs: json['duration_ms'] as int,
      filePath: json['file_path'] as String?,
      fileSize: json['file_size'] as int?,
      format: json['format'] as String?,
      mediaStartTime: json['media_start_time'] != null
          ? DateTime.parse(json['media_start_time'] as String).toLocal()
          : null,
    );
  }
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
      id: json['id'].toString(),
      cameraId: json['camera_id'] as String,
      startedAt: json['started_at'] as String,
      endedAt: json['ended_at'] as String?,
      thumbnailPath: json['thumbnail_path'] as String?,
      eventType: json['event_type'] as String?,
      objectClass: json['object_class'] as String?,
      confidence: (json['confidence'] as num?)?.toDouble(),
    );
  }

  DateTime get startTime => DateTime.parse(startedAt).toLocal();

  DateTime? get endTime =>
      endedAt != null ? DateTime.parse(endedAt!).toLocal() : null;
}
