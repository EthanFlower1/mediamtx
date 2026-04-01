class ScheduleTemplate {
  final String id;
  final String name;
  final String mode;
  final List<int> days;
  final String startTime;
  final String endTime;
  final int postEventSeconds;
  final bool isDefault;

  const ScheduleTemplate({
    required this.id,
    required this.name,
    required this.mode,
    required this.days,
    required this.startTime,
    required this.endTime,
    this.postEventSeconds = 30,
    this.isDefault = false,
  });

  factory ScheduleTemplate.fromJson(Map<String, dynamic> json) {
    final rawDays = json['days'];
    List<int> days;
    if (rawDays is String) {
      days = _parseDaysString(rawDays);
    } else if (rawDays is List) {
      days = rawDays.map((d) => d as int).toList();
    } else {
      days = [];
    }

    return ScheduleTemplate(
      id: json['id'] as String? ?? '',
      name: json['name'] as String? ?? '',
      mode: json['mode'] as String? ?? 'always',
      days: days,
      startTime: json['start_time'] as String? ?? '00:00',
      endTime: json['end_time'] as String? ?? '00:00',
      postEventSeconds: json['post_event_seconds'] as int? ?? 30,
      isDefault: json['is_default'] == true || json['is_default'] == 1,
    );
  }

  static List<int> _parseDaysString(String s) {
    try {
      return s
          .replaceAll('[', '')
          .replaceAll(']', '')
          .split(',')
          .where((e) => e.trim().isNotEmpty)
          .map((e) => int.parse(e.trim()))
          .toList();
    } catch (_) {
      return [];
    }
  }

  String get modeLabel => mode == 'events' ? 'Motion' : 'Continuous';

  String get daysLabel {
    if (days.length == 7) return 'All days';
    if (days.length == 5 && !days.contains(0) && !days.contains(6)) return 'Mon-Fri';
    if (days.length == 2 && days.contains(0) && days.contains(6)) return 'Weekends';
    const names = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
    return days.map((d) => names[d]).join(', ');
  }

  String get timeLabel {
    if (startTime == '00:00' && endTime == '00:00') return 'All day';
    return '$startTime-$endTime';
  }

  String get description => '$daysLabel • $timeLabel';
}
