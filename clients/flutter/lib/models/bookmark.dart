class Bookmark {
  final int id;
  final String cameraId;
  final DateTime timestamp;
  final String label;
  final String? createdBy;
  final DateTime createdAt;

  const Bookmark({
    required this.id,
    required this.cameraId,
    required this.timestamp,
    required this.label,
    this.createdBy,
    required this.createdAt,
  });

  factory Bookmark.fromJson(Map<String, dynamic> json) {
    return Bookmark(
      id: json['id'] as int,
      cameraId: json['camera_id'] as String,
      timestamp: DateTime.parse(json['timestamp'] as String),
      label: json['label'] as String,
      createdBy: json['created_by'] as String?,
      createdAt: DateTime.parse(json['created_at'] as String),
    );
  }
}
