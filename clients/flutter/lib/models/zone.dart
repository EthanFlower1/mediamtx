import 'dart:convert';

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
      id: json['id']?.toString() ?? '',
      zoneId: json['zone_id']?.toString() ?? '',
      className: json['class_name']?.toString() ?? '',
      enabled: json['enabled'] as bool? ?? false,
      cooldownSeconds: (json['cooldown_seconds'] as num?)?.toInt() ?? 0,
      loiterSeconds: (json['loiter_seconds'] as num?)?.toInt() ?? 0,
      notifyOnEnter: json['notify_on_enter'] as bool? ?? false,
      notifyOnLeave: json['notify_on_leave'] as bool? ?? false,
      notifyOnLoiter: json['notify_on_loiter'] as bool? ?? false,
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

  /// Safely parse a single polygon point into [x, y].
  /// Returns null if the element is not a valid 2-element numeric list.
  static List<double>? _parsePoint(dynamic p) {
    if (p is! List || p.length < 2) return null;
    try {
      final x = (p[0] as num).toDouble();
      final y = (p[1] as num).toDouble();
      return [x, y];
    } catch (_) {
      return null;
    }
  }

  factory DetectionZone.fromJson(Map<String, dynamic> json) {
    final rawPoly = json['polygon'];
    List<List<double>> poly = [];
    if (rawPoly is String) {
      try {
        final parsed = jsonDecode(rawPoly);
        if (parsed is List) {
          for (final p in parsed) {
            final pt = _parsePoint(p);
            if (pt != null) poly.add(pt);
          }
        }
      } catch (_) {
        // Malformed JSON string -- leave poly empty
      }
    } else if (rawPoly is List) {
      for (final p in rawPoly) {
        final pt = _parsePoint(p);
        if (pt != null) poly.add(pt);
      }
    }

    final rawRules = json['rules'] as List<dynamic>? ?? [];
    List<AlertRule> rules;
    try {
      rules = rawRules
          .map((r) => AlertRule.fromJson(r as Map<String, dynamic>))
          .toList();
    } catch (_) {
      // Malformed rules array -- treat as empty
      rules = [];
    }

    return DetectionZone(
      id: json['id']?.toString() ?? '',
      cameraId: json['camera_id']?.toString() ?? '',
      name: json['name']?.toString() ?? '',
      polygon: poly,
      enabled: json['enabled'] as bool? ?? false,
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
