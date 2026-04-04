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
      id: json['id']?.toString() ?? '',
      cameraId: json['camera_id']?.toString() ?? '',
      name: json['name']?.toString() ?? '',
      startTime: json['start_time']?.toString() ?? '',
      endTime: json['end_time']?.toString() ?? '',
      tags: (json['tags'] as List<dynamic>?)
              ?.map((t) => t.toString())
              .toList() ??
          [],
      notes: json['notes']?.toString(),
      createdAt: json['created_at']?.toString() ?? '',
    );
  }
}
