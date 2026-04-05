import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/media_profile.dart';

void main() {
  group('ProfileInfo', () {
    test('fromJson parses all fields with nested objects', () {
      final json = {
        'token': 'profile-1',
        'name': 'Main Stream',
        'video_source': {
          'token': 'vs-1',
          'framerate': 30.0,
          'width': 1920,
          'height': 1080,
        },
        'video_encoder': {
          'token': 've-1',
          'name': 'H264 Main',
          'encoding': 'H264',
          'width': 1920,
          'height': 1080,
          'quality': 85.0,
          'frame_rate': 30,
          'bitrate_limit': 4096,
          'encoding_interval': 1,
          'gov_length': 30,
          'h264_profile': 'Main',
        },
        'audio_encoder': {
          'token': 'ae-1',
          'name': 'G711',
          'encoding': 'G711',
          'bitrate': 64,
          'sample_rate': 8000,
        },
        'ptz_config': {
          'token': 'ptz-1',
          'name': 'PTZ Config 1',
          'node_token': 'node-1',
        },
      };

      final profile = ProfileInfo.fromJson(json);

      expect(profile.token, 'profile-1');
      expect(profile.name, 'Main Stream');
      expect(profile.videoSource, isNotNull);
      expect(profile.videoSource!.token, 'vs-1');
      expect(profile.videoSource!.framerate, 30.0);
      expect(profile.videoSource!.width, 1920);
      expect(profile.videoSource!.height, 1080);
      expect(profile.videoEncoder, isNotNull);
      expect(profile.videoEncoder!.encoding, 'H264');
      expect(profile.videoEncoder!.quality, 85.0);
      expect(profile.videoEncoder!.frameRate, 30);
      expect(profile.videoEncoder!.bitrateLimit, 4096);
      expect(profile.videoEncoder!.govLength, 30);
      expect(profile.videoEncoder!.h264Profile, 'Main');
      expect(profile.audioEncoder, isNotNull);
      expect(profile.audioEncoder!.encoding, 'G711');
      expect(profile.audioEncoder!.bitrate, 64);
      expect(profile.audioEncoder!.sampleRate, 8000);
      expect(profile.ptzConfig, isNotNull);
      expect(profile.ptzConfig!.token, 'ptz-1');
      expect(profile.ptzConfig!.nodeToken, 'node-1');
    });

    test('fromJson handles missing nested objects', () {
      final json = {
        'token': 'profile-2',
        'name': 'Sub Stream',
      };

      final profile = ProfileInfo.fromJson(json);

      expect(profile.token, 'profile-2');
      expect(profile.name, 'Sub Stream');
      expect(profile.videoSource, isNull);
      expect(profile.videoEncoder, isNull);
      expect(profile.audioEncoder, isNull);
      expect(profile.ptzConfig, isNull);
    });

    test('fromJson handles null nested objects', () {
      final json = {
        'token': 'profile-3',
        'name': 'Test',
        'video_source': null,
        'video_encoder': null,
        'audio_encoder': null,
        'ptz_config': null,
      };

      final profile = ProfileInfo.fromJson(json);

      expect(profile.videoSource, isNull);
      expect(profile.videoEncoder, isNull);
      expect(profile.audioEncoder, isNull);
      expect(profile.ptzConfig, isNull);
    });

    test('fromJson handles missing top-level fields', () {
      final profile = ProfileInfo.fromJson({});

      expect(profile.token, '');
      expect(profile.name, '');
    });
  });

  group('VideoSourceInfo', () {
    test('fromJson handles int framerate', () {
      final json = {
        'token': 'vs-1',
        'framerate': 25,
        'width': 1920,
        'height': 1080,
      };

      final vs = VideoSourceInfo.fromJson(json);

      expect(vs.framerate, 25.0);
    });

    test('fromJson handles missing fields', () {
      final vs = VideoSourceInfo.fromJson({});

      expect(vs.token, '');
      expect(vs.framerate, 0.0);
      expect(vs.width, 0);
      expect(vs.height, 0);
    });
  });

  group('VideoEncoderConfig', () {
    test('toJson roundtrips correctly', () {
      final config = VideoEncoderConfig.fromJson({
        'token': 've-1',
        'name': 'Test',
        'encoding': 'H264',
        'width': 1920,
        'height': 1080,
        'quality': 80.0,
        'frame_rate': 30,
        'bitrate_limit': 4096,
        'encoding_interval': 1,
        'gov_length': 30,
        'h264_profile': 'High',
      });

      final json = config.toJson();

      expect(json['token'], 've-1');
      expect(json['encoding'], 'H264');
      expect(json['frame_rate'], 30);
      expect(json['bitrate_limit'], 4096);
      expect(json['gov_length'], 30);
      expect(json['h264_profile'], 'High');
    });

    test('fromJson handles missing optional fields', () {
      final config = VideoEncoderConfig.fromJson({});

      expect(config.govLength, 0);
      expect(config.h264Profile, '');
    });
  });

  group('AudioEncoderConfig', () {
    test('fromJson handles missing fields', () {
      final config = AudioEncoderConfig.fromJson({});

      expect(config.token, '');
      expect(config.name, '');
      expect(config.encoding, '');
      expect(config.bitrate, 0);
      expect(config.sampleRate, 0);
    });
  });

  group('VideoEncoderOptions', () {
    test('fromJson parses all fields', () {
      final json = {
        'encodings': ['H264', 'H265'],
        'resolutions': [
          {'width': 1920, 'height': 1080},
          {'width': 1280, 'height': 720},
        ],
        'frame_rate_range': {'min': 1, 'max': 30},
        'quality_range': {'min': 1, 'max': 100},
        'h264_profiles': ['Baseline', 'Main', 'High'],
      };

      final opts = VideoEncoderOptions.fromJson(json);

      expect(opts.encodings, ['H264', 'H265']);
      expect(opts.resolutions, hasLength(2));
      expect(opts.resolutions[0].width, 1920);
      expect(opts.resolutions[0].height, 1080);
      expect(opts.frameRateRange.min, 1);
      expect(opts.frameRateRange.max, 30);
      expect(opts.qualityRange.min, 1);
      expect(opts.qualityRange.max, 100);
      expect(opts.h264Profiles, ['Baseline', 'Main', 'High']);
    });

    test('fromJson handles missing fields with defaults', () {
      final opts = VideoEncoderOptions.fromJson({});

      expect(opts.encodings, isEmpty);
      expect(opts.resolutions, isEmpty);
      expect(opts.frameRateRange.min, 1);
      expect(opts.frameRateRange.max, 30);
      expect(opts.qualityRange.min, 1);
      expect(opts.qualityRange.max, 100);
      expect(opts.h264Profiles, isEmpty);
    });
  });

  group('Resolution', () {
    test('toString formats correctly', () {
      final res = Resolution(width: 1920, height: 1080);
      expect(res.toString(), '1920x1080');
    });

    test('equality works', () {
      final a = Resolution(width: 1920, height: 1080);
      final b = Resolution(width: 1920, height: 1080);
      final c = Resolution(width: 1280, height: 720);

      expect(a, equals(b));
      expect(a, isNot(equals(c)));
      expect(a.hashCode, b.hashCode);
    });

    test('fromJson handles missing fields', () {
      final res = Resolution.fromJson({});

      expect(res.width, 0);
      expect(res.height, 0);
    });
  });

  group('RangeInt', () {
    test('fromJson parses correctly', () {
      final range = RangeInt.fromJson({'min': 5, 'max': 60});

      expect(range.min, 5);
      expect(range.max, 60);
    });

    test('fromJson handles missing fields', () {
      final range = RangeInt.fromJson({});

      expect(range.min, 0);
      expect(range.max, 100);
    });
  });

  group('PtzConfigInfo', () {
    test('fromJson parses all fields', () {
      final json = {
        'token': 'ptz-cfg-1',
        'name': 'PTZ Config',
        'node_token': 'node-1',
      };

      final config = PtzConfigInfo.fromJson(json);

      expect(config.token, 'ptz-cfg-1');
      expect(config.name, 'PTZ Config');
      expect(config.nodeToken, 'node-1');
    });

    test('fromJson handles missing fields', () {
      final config = PtzConfigInfo.fromJson({});

      expect(config.token, '');
      expect(config.name, '');
      expect(config.nodeToken, '');
    });
  });
}
