class AlertRule {
  final String id;
  final String zoneId;
  final String className;
  final bool enabled;
  final int cooldownSeconds;
  final int loiterSeconds;
  final bool notifyOnEnter;
  final bool notifyOnLeave;
  final bool notifyOnLoiter;

  const AlertRule({
    required this.id,
    required this.zoneId,
    required this.className,
    required this.enabled,
    required this.cooldownSeconds,
    required this.loiterSeconds,
    required this.notifyOnEnter,
    required this.notifyOnLeave,
    required this.notifyOnLoiter,
  });

  factory AlertRule.fromJson(Map<String, dynamic> json) {
    return AlertRule(
      id: json['id'] as String,
      zoneId: json['zone_id'] as String,
      className: json['class_name'] as String,
      enabled: json['enabled'] as bool,
      cooldownSeconds: json['cooldown_seconds'] as int,
      loiterSeconds: json['loiter_seconds'] as int,
      notifyOnEnter: json['notify_on_enter'] as bool,
      notifyOnLeave: json['notify_on_leave'] as bool,
      notifyOnLoiter: json['notify_on_loiter'] as bool,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'zone_id': zoneId,
      'class_name': className,
      'enabled': enabled,
      'cooldown_seconds': cooldownSeconds,
      'loiter_seconds': loiterSeconds,
      'notify_on_enter': notifyOnEnter,
      'notify_on_leave': notifyOnLeave,
      'notify_on_loiter': notifyOnLoiter,
    };
  }
}

class DetectionZone {
  final String id;
  final String cameraId;
  final String name;
  final List<List<double>> polygon;
  final bool enabled;
  final List<AlertRule> rules;

  const DetectionZone({
    required this.id,
    required this.cameraId,
    required this.name,
    required this.polygon,
    required this.enabled,
    required this.rules,
  });

  factory DetectionZone.fromJson(Map<String, dynamic> json) {
    final rawPolygon = json['polygon'] as List<dynamic>;
    final polygon = rawPolygon
        .map((point) => (point as List<dynamic>).map((v) => (v as num).toDouble()).toList())
        .toList();

    final rawRules = json['rules'] as List<dynamic>? ?? [];
    final rules = rawRules
        .map((r) => AlertRule.fromJson(r as Map<String, dynamic>))
        .toList();

    return DetectionZone(
      id: json['id'] as String,
      cameraId: json['camera_id'] as String,
      name: json['name'] as String,
      polygon: polygon,
      enabled: json['enabled'] as bool,
      rules: rules,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'id': id,
      'camera_id': cameraId,
      'name': name,
      'polygon': polygon,
      'enabled': enabled,
      'rules': rules.map((r) => r.toJson()).toList(),
    };
  }
}
