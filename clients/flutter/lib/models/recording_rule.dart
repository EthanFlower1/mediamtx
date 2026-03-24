class RecordingRule {
  final String id;
  final String cameraId;
  final String mode;
  final String? startTime;
  final String? endTime;
  final List<int>? daysOfWeek;
  final bool enabled;

  const RecordingRule({
    required this.id,
    required this.cameraId,
    required this.mode,
    this.startTime,
    this.endTime,
    this.daysOfWeek,
    required this.enabled,
  });

  factory RecordingRule.fromJson(Map<String, dynamic> json) {
    final rawDays = json['days_of_week'] as List<dynamic>?;
    final daysOfWeek = rawDays?.map((d) => d as int).toList();

    return RecordingRule(
      id: json['id'] as String,
      cameraId: json['camera_id'] as String,
      mode: json['mode'] as String,
      startTime: json['start_time'] as String?,
      endTime: json['end_time'] as String?,
      daysOfWeek: daysOfWeek,
      enabled: json['enabled'] as bool,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'camera_id': cameraId,
      'mode': mode,
      'start_time': startTime,
      'end_time': endTime,
      'days_of_week': daysOfWeek,
      'enabled': enabled,
    };
  }
}
