class CameraStream {
  final String id;
  final String cameraId;
  final String name;
  final String rtspUrl;
  final String profileToken;
  final String videoCodec;
  final String audioCodec;
  final int width;
  final int height;
  final String roles;
  final String liveVideoCodec;
  final String liveAudioCodec;
  final int liveWidth;
  final int liveHeight;

  const CameraStream({
    required this.id,
    required this.cameraId,
    required this.name,
    required this.rtspUrl,
    this.profileToken = '',
    this.videoCodec = '',
    this.audioCodec = '',
    this.width = 0,
    this.height = 0,
    this.roles = '',
    this.liveVideoCodec = '',
    this.liveAudioCodec = '',
    this.liveWidth = 0,
    this.liveHeight = 0,
  });

  factory CameraStream.fromJson(Map<String, dynamic> json) {
    return CameraStream(
      id: json['id'] as String? ?? '',
      cameraId: json['camera_id'] as String? ?? '',
      name: json['name'] as String? ?? '',
      rtspUrl: json['rtsp_url'] as String? ?? '',
      profileToken: json['profile_token'] as String? ?? '',
      videoCodec: json['video_codec'] as String? ?? '',
      audioCodec: json['audio_codec'] as String? ?? '',
      width: json['width'] as int? ?? 0,
      height: json['height'] as int? ?? 0,
      roles: json['roles'] as String? ?? '',
      liveVideoCodec: json['live_video_codec'] as String? ?? '',
      liveAudioCodec: json['live_audio_codec'] as String? ?? '',
      liveWidth: json['live_width'] as int? ?? 0,
      liveHeight: json['live_height'] as int? ?? 0,
    );
  }

  String get displayLabel {
    if (width > 0 && height > 0) {
      return '$name (${width}x$height)';
    }
    return name;
  }

  List<String> get roleList {
    if (roles.isEmpty) return [];
    return roles.split(',').map((r) => r.trim()).where((r) => r.isNotEmpty).toList();
  }

  String get effectiveVideoCodec => liveVideoCodec.isNotEmpty ? liveVideoCodec : videoCodec;
  String get effectiveAudioCodec => liveAudioCodec.isNotEmpty ? liveAudioCodec : audioCodec;
  int get effectiveWidth => liveWidth > 0 ? liveWidth : width;
  int get effectiveHeight => liveHeight > 0 ? liveHeight : height;

  String get resolutionLabel {
    final w = effectiveWidth;
    final h = effectiveHeight;
    if (w > 0 && h > 0) return '${w}x$h';
    return '';
  }
}
