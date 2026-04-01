class PtzStatus {
  final double panPosition;
  final double tiltPosition;
  final double zoomPosition;
  final bool isMoving;

  const PtzStatus({
    required this.panPosition,
    required this.tiltPosition,
    required this.zoomPosition,
    required this.isMoving,
  });

  factory PtzStatus.fromJson(Map<String, dynamic> json) {
    return PtzStatus(
      panPosition: (json['pan_position'] as num?)?.toDouble() ?? 0,
      tiltPosition: (json['tilt_position'] as num?)?.toDouble() ?? 0,
      zoomPosition: (json['zoom_position'] as num?)?.toDouble() ?? 0,
      isMoving: json['is_moving'] as bool? ?? false,
    );
  }
}
