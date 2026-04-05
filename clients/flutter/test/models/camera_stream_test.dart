import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/camera_stream.dart';

void main() {
  group('CameraStream', () {
    test('fromJson parses all fields', () {
      final json = {
        'id': 'stream-1',
        'camera_id': 'cam-1',
        'name': 'Main',
        'rtsp_url': 'rtsp://192.168.1.10/main',
        'profile_token': 'profile_1',
        'video_codec': 'h265',
        'audio_codec': 'aac',
        'width': 1920,
        'height': 1080,
        'roles': 'primary,recording',
        'live_video_codec': 'h264',
        'live_audio_codec': 'opus',
        'live_width': 1280,
        'live_height': 720,
        'retention_days': 30,
        'event_retention_days': 90,
      };

      final stream = CameraStream.fromJson(json);

      expect(stream.id, 'stream-1');
      expect(stream.cameraId, 'cam-1');
      expect(stream.name, 'Main');
      expect(stream.rtspUrl, 'rtsp://192.168.1.10/main');
      expect(stream.profileToken, 'profile_1');
      expect(stream.videoCodec, 'h265');
      expect(stream.audioCodec, 'aac');
      expect(stream.width, 1920);
      expect(stream.height, 1080);
      expect(stream.roles, 'primary,recording');
      expect(stream.liveVideoCodec, 'h264');
      expect(stream.liveAudioCodec, 'opus');
      expect(stream.liveWidth, 1280);
      expect(stream.liveHeight, 720);
      expect(stream.retentionDays, 30);
      expect(stream.eventRetentionDays, 90);
    });

    test('fromJson uses defaults for missing fields', () {
      final stream = CameraStream.fromJson({});

      expect(stream.id, '');
      expect(stream.cameraId, '');
      expect(stream.name, '');
      expect(stream.rtspUrl, '');
      expect(stream.profileToken, '');
      expect(stream.videoCodec, '');
      expect(stream.audioCodec, '');
      expect(stream.width, 0);
      expect(stream.height, 0);
      expect(stream.roles, '');
      expect(stream.liveVideoCodec, '');
      expect(stream.liveAudioCodec, '');
      expect(stream.liveWidth, 0);
      expect(stream.liveHeight, 0);
      expect(stream.retentionDays, 0);
      expect(stream.eventRetentionDays, 0);
    });

    test('fromJson handles null values gracefully', () {
      final json = {
        'id': null,
        'camera_id': null,
        'name': null,
        'rtsp_url': null,
        'width': null,
        'height': null,
      };

      final stream = CameraStream.fromJson(json);

      expect(stream.id, '');
      expect(stream.cameraId, '');
      expect(stream.name, '');
      expect(stream.rtspUrl, '');
      expect(stream.width, 0);
      expect(stream.height, 0);
    });

    test('displayLabel includes resolution when available', () {
      final stream = CameraStream.fromJson({
        'name': 'Main',
        'width': 1920,
        'height': 1080,
      });

      expect(stream.displayLabel, 'Main (1920x1080)');
    });

    test('displayLabel is just name when no resolution', () {
      final stream = CameraStream.fromJson({'name': 'Main'});

      expect(stream.displayLabel, 'Main');
    });

    test('roleList splits comma-separated roles', () {
      final stream = CameraStream.fromJson({
        'roles': 'primary, recording, analytics',
      });

      expect(stream.roleList, ['primary', 'recording', 'analytics']);
    });

    test('roleList returns empty list for empty roles', () {
      final stream = CameraStream.fromJson({});

      expect(stream.roleList, isEmpty);
    });

    test('effectiveVideoCodec prefers live codec', () {
      final stream = CameraStream.fromJson({
        'video_codec': 'h265',
        'live_video_codec': 'h264',
      });

      expect(stream.effectiveVideoCodec, 'h264');
    });

    test('effectiveVideoCodec falls back to source codec', () {
      final stream = CameraStream.fromJson({
        'video_codec': 'h265',
      });

      expect(stream.effectiveVideoCodec, 'h265');
    });

    test('effectiveAudioCodec prefers live codec', () {
      final stream = CameraStream.fromJson({
        'audio_codec': 'aac',
        'live_audio_codec': 'opus',
      });

      expect(stream.effectiveAudioCodec, 'opus');
    });

    test('effectiveWidth/Height prefers live dimensions', () {
      final stream = CameraStream.fromJson({
        'width': 1920,
        'height': 1080,
        'live_width': 1280,
        'live_height': 720,
      });

      expect(stream.effectiveWidth, 1280);
      expect(stream.effectiveHeight, 720);
    });

    test('effectiveWidth/Height falls back to source dimensions', () {
      final stream = CameraStream.fromJson({
        'width': 1920,
        'height': 1080,
      });

      expect(stream.effectiveWidth, 1920);
      expect(stream.effectiveHeight, 1080);
    });

    test('resolutionLabel shows effective resolution', () {
      final stream = CameraStream.fromJson({
        'width': 1920,
        'height': 1080,
      });

      expect(stream.resolutionLabel, '1920x1080');
    });

    test('resolutionLabel empty when no resolution', () {
      final stream = CameraStream.fromJson({});

      expect(stream.resolutionLabel, '');
    });
  });
}
