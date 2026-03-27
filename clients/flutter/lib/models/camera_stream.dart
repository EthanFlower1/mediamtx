class CameraStream {
  final String id;
  final String cameraId;
  final String name;
  final String rtspUrl;
  final int width;
  final int height;
  final String roles;

  const CameraStream({
    required this.id,
    required this.cameraId,
    required this.name,
    required this.rtspUrl,
    this.width = 0,
    this.height = 0,
    this.roles = '',
  });

  factory CameraStream.fromJson(Map<String, dynamic> json) {
    return CameraStream(
      id: json['id'] as String? ?? '',
      cameraId: json['camera_id'] as String? ?? '',
      name: json['name'] as String? ?? '',
      rtspUrl: json['rtsp_url'] as String? ?? '',
      width: json['width'] as int? ?? 0,
      height: json['height'] as int? ?? 0,
      roles: json['roles'] as String? ?? '',
    );
  }

  String get displayLabel {
    if (width > 0 && height > 0) {
      return '$name (${width}x$height)';
    }
    return name;
  }
}
