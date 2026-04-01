class ProfileInfo {
  final String token;
  final String name;
  final VideoSourceInfo? videoSource;
  final VideoEncoderConfig? videoEncoder;
  final AudioEncoderConfig? audioEncoder;
  final PtzConfigInfo? ptzConfig;

  const ProfileInfo({
    required this.token,
    required this.name,
    this.videoSource,
    this.videoEncoder,
    this.audioEncoder,
    this.ptzConfig,
  });

  factory ProfileInfo.fromJson(Map<String, dynamic> json) {
    return ProfileInfo(
      token: json['token'] as String? ?? '',
      name: json['name'] as String? ?? '',
      videoSource: json['video_source'] != null
          ? VideoSourceInfo.fromJson(json['video_source'] as Map<String, dynamic>)
          : null,
      videoEncoder: json['video_encoder'] != null
          ? VideoEncoderConfig.fromJson(json['video_encoder'] as Map<String, dynamic>)
          : null,
      audioEncoder: json['audio_encoder'] != null
          ? AudioEncoderConfig.fromJson(json['audio_encoder'] as Map<String, dynamic>)
          : null,
      ptzConfig: json['ptz_config'] != null
          ? PtzConfigInfo.fromJson(json['ptz_config'] as Map<String, dynamic>)
          : null,
    );
  }
}

class VideoSourceInfo {
  final String token;
  final double framerate;
  final int width;
  final int height;

  const VideoSourceInfo({
    required this.token,
    required this.framerate,
    required this.width,
    required this.height,
  });

  factory VideoSourceInfo.fromJson(Map<String, dynamic> json) => VideoSourceInfo(
        token: json['token'] as String? ?? '',
        framerate: (json['framerate'] as num?)?.toDouble() ?? 0,
        width: json['width'] as int? ?? 0,
        height: json['height'] as int? ?? 0,
      );
}

class VideoEncoderConfig {
  final String token;
  final String name;
  final String encoding;
  final int width;
  final int height;
  final double quality;
  final int frameRate;
  final int bitrateLimit;
  final int encodingInterval;
  final int govLength;
  final String h264Profile;

  const VideoEncoderConfig({
    required this.token,
    required this.name,
    required this.encoding,
    required this.width,
    required this.height,
    required this.quality,
    required this.frameRate,
    required this.bitrateLimit,
    required this.encodingInterval,
    this.govLength = 0,
    this.h264Profile = '',
  });

  factory VideoEncoderConfig.fromJson(Map<String, dynamic> json) => VideoEncoderConfig(
        token: json['token'] as String? ?? '',
        name: json['name'] as String? ?? '',
        encoding: json['encoding'] as String? ?? '',
        width: json['width'] as int? ?? 0,
        height: json['height'] as int? ?? 0,
        quality: (json['quality'] as num?)?.toDouble() ?? 0,
        frameRate: json['frame_rate'] as int? ?? 0,
        bitrateLimit: json['bitrate_limit'] as int? ?? 0,
        encodingInterval: json['encoding_interval'] as int? ?? 0,
        govLength: json['gov_length'] as int? ?? 0,
        h264Profile: json['h264_profile'] as String? ?? '',
      );

  Map<String, dynamic> toJson() => {
        'token': token,
        'name': name,
        'encoding': encoding,
        'width': width,
        'height': height,
        'quality': quality,
        'frame_rate': frameRate,
        'bitrate_limit': bitrateLimit,
        'encoding_interval': encodingInterval,
        'gov_length': govLength,
        'h264_profile': h264Profile,
      };
}

class AudioEncoderConfig {
  final String token;
  final String name;
  final String encoding;
  final int bitrate;
  final int sampleRate;

  const AudioEncoderConfig({
    required this.token,
    required this.name,
    required this.encoding,
    required this.bitrate,
    required this.sampleRate,
  });

  factory AudioEncoderConfig.fromJson(Map<String, dynamic> json) => AudioEncoderConfig(
        token: json['token'] as String? ?? '',
        name: json['name'] as String? ?? '',
        encoding: json['encoding'] as String? ?? '',
        bitrate: json['bitrate'] as int? ?? 0,
        sampleRate: json['sample_rate'] as int? ?? 0,
      );
}

class VideoEncoderOptions {
  final List<String> encodings;
  final List<Resolution> resolutions;
  final RangeInt frameRateRange;
  final RangeInt qualityRange;
  final List<String> h264Profiles;

  const VideoEncoderOptions({
    required this.encodings,
    required this.resolutions,
    required this.frameRateRange,
    required this.qualityRange,
    this.h264Profiles = const [],
  });

  factory VideoEncoderOptions.fromJson(Map<String, dynamic> json) => VideoEncoderOptions(
        encodings:
            (json['encodings'] as List?)?.map((e) => e as String).toList() ?? [],
        resolutions: (json['resolutions'] as List?)
                ?.map((e) => Resolution.fromJson(e as Map<String, dynamic>))
                .toList() ??
            [],
        frameRateRange: json['frame_rate_range'] != null
            ? RangeInt.fromJson(json['frame_rate_range'] as Map<String, dynamic>)
            : const RangeInt(min: 1, max: 30),
        qualityRange: json['quality_range'] != null
            ? RangeInt.fromJson(json['quality_range'] as Map<String, dynamic>)
            : const RangeInt(min: 1, max: 100),
        h264Profiles:
            (json['h264_profiles'] as List?)?.map((e) => e as String).toList() ?? [],
      );
}

class Resolution {
  final int width;
  final int height;

  const Resolution({required this.width, required this.height});

  factory Resolution.fromJson(Map<String, dynamic> json) =>
      Resolution(width: json['width'] as int? ?? 0, height: json['height'] as int? ?? 0);

  @override
  String toString() => '${width}x$height';
}

class RangeInt {
  final int min;
  final int max;

  const RangeInt({required this.min, required this.max});

  factory RangeInt.fromJson(Map<String, dynamic> json) =>
      RangeInt(min: json['min'] as int? ?? 0, max: json['max'] as int? ?? 100);
}

class PtzConfigInfo {
  final String token;
  final String name;
  final String nodeToken;

  const PtzConfigInfo({
    required this.token,
    required this.name,
    required this.nodeToken,
  });

  factory PtzConfigInfo.fromJson(Map<String, dynamic> json) => PtzConfigInfo(
        token: json['token'] as String? ?? '',
        name: json['name'] as String? ?? '',
        nodeToken: json['node_token'] as String? ?? '',
      );
}
