import 'package:flutter/material.dart';

class NotificationEvent {
  final String id;
  final String type;
  final String camera;
  final String message;
  final DateTime time;
  final String? zone;
  final String? className;
  final String? action;
  final String? trackId;
  final double? confidence;
  final bool isRead;

  const NotificationEvent({
    required this.id,
    required this.type,
    required this.camera,
    required this.message,
    required this.time,
    this.zone,
    this.className,
    this.action,
    this.trackId,
    this.confidence,
    this.isRead = false,
  });

  factory NotificationEvent.fromJson(Map<String, dynamic> json) {
    return NotificationEvent(
      id: json['id'] as String? ??
          '${json['type']}_${json['camera']}_${json['time'] ?? DateTime.now().toIso8601String()}',
      type: json['type'] as String? ?? '',
      camera: json['camera'] as String? ?? '',
      message: json['message'] as String? ?? '',
      time: json['time'] != null
          ? DateTime.parse(json['time'] as String)
          : DateTime.now(),
      zone: json['zone'] as String?,
      className: json['class'] as String?,
      action: json['action'] as String?,
      trackId: json['trackId'] as String?,
      confidence: (json['confidence'] as num?)?.toDouble(),
      isRead: json['isRead'] as bool? ?? false,
    );
  }

  NotificationEvent copyWith({bool? isRead}) {
    return NotificationEvent(
      id: id,
      type: type,
      camera: camera,
      message: message,
      time: time,
      zone: zone,
      className: className,
      action: action,
      trackId: trackId,
      confidence: confidence,
      isRead: isRead ?? this.isRead,
    );
  }

  bool get isDetectionFrame => type == 'detection_frame';

  bool get isAlert => type == 'alert';

  /// Returns a Material icon appropriate for this notification type.
  IconData get typeIcon {
    switch (type) {
      case 'motion':
        return Icons.directions_run;
      case 'camera_offline':
        return Icons.videocam_off;
      case 'camera_online':
        return Icons.videocam;
      case 'alert':
        return Icons.warning_amber;
      case 'detection_frame':
        return Icons.center_focus_strong;
      case 'recording_started':
        return Icons.fiber_manual_record;
      case 'recording_stopped':
        return Icons.stop_circle_outlined;
      default:
        return Icons.notifications_outlined;
    }
  }

  /// Returns the go_router path this notification should navigate to, or null
  /// if no specific destination applies.
  String? get navigationRoute {
    switch (type) {
      case 'camera_offline':
      case 'camera_online':
        return '/devices/$camera';
      case 'motion':
      case 'detection_frame':
      case 'alert':
        final ts = time.toIso8601String();
        return '/playback?cameraId=$camera&timestamp=$ts';
      default:
        return null;
    }
  }
}
