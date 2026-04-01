class NotificationEvent {
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
}
