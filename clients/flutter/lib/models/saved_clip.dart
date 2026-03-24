class SavedClip {
  final String id;
  final String cameraId;
  final String name;
  final String startTime;
  final String endTime;
  final List<String> tags;
  final String? notes;
  final String createdAt;

  const SavedClip({
    required this.id,
    required this.cameraId,
    required this.name,
    required this.startTime,
    required this.endTime,
    required this.tags,
    this.notes,
    required this.createdAt,
  });

  factory SavedClip.fromJson(Map<String, dynamic> json) {
    return SavedClip(
      id: json['id'] as String,
      cameraId: json['camera_id'] as String,
      name: json['name'] as String,
      startTime: json['start_time'] as String,
      endTime: json['end_time'] as String,
      tags: (json['tags'] as List<dynamic>?)
              ?.map((t) => t as String)
              .toList() ??
          [],
      notes: json['notes'] as String?,
      createdAt: json['created_at'] as String,
    );
  }
}
