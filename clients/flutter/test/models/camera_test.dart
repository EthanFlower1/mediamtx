import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/camera.dart';

void main() {
  group('Camera', () {
    test('fromJson parses all fields', () {
      final json = {
        'id': 'cam-1',
        'name': 'Front Door',
        'rtsp_url': 'rtsp://192.168.1.10/stream1',
        'onvif_endpoint': 'http://192.168.1.10/onvif/device_service',
        'mediamtx_path': 'front_door',
        'status': 'connected',
        'ptz_capable': true,
        'ai_enabled': true,
        'ai_stream_id': 'stream-2',
        'ai_confidence': 0.75,
        'ai_track_timeout': 10,
        'sub_stream_url': 'rtsp://192.168.1.10/stream2',
        'retention_days': 14,
        'event_retention_days': 60,
        'detection_retention_days': 90,
        'motion_timeout_seconds': 15,
        'snapshot_uri': 'http://192.168.1.10/snapshot',
        'supports_events': true,
        'supports_analytics': true,
        'supports_relay': true,
        'created_at': '2026-01-01T00:00:00Z',
        'updated_at': '2026-03-15T12:00:00Z',
        'storage_path': '/data/recordings/cam-1',
        'storage_status': 'ok',
        'live_view_path': '/live/front_door',
        'live_view_codec': 'h264',
        'stream_paths': [
          {
            'name': 'Main',
            'path': '/stream/main',
            'resolution': '1920x1080',
            'video_codec': 'h265',
          },
          {
            'name': 'Sub',
            'path': '/stream/sub',
            'resolution': '640x480',
            'video_codec': 'h264',
          },
        ],
      };

      final camera = Camera.fromJson(json);

      expect(camera.id, 'cam-1');
      expect(camera.name, 'Front Door');
      expect(camera.rtspUrl, 'rtsp://192.168.1.10/stream1');
      expect(camera.onvifEndpoint, 'http://192.168.1.10/onvif/device_service');
      expect(camera.mediamtxPath, 'front_door');
      expect(camera.status, 'connected');
      expect(camera.ptzCapable, isTrue);
      expect(camera.aiEnabled, isTrue);
      expect(camera.aiStreamId, 'stream-2');
      expect(camera.aiConfidence, 0.75);
      expect(camera.aiTrackTimeout, 10);
      expect(camera.subStreamUrl, 'rtsp://192.168.1.10/stream2');
      expect(camera.retentionDays, 14);
      expect(camera.eventRetentionDays, 60);
      expect(camera.detectionRetentionDays, 90);
      expect(camera.motionTimeoutSeconds, 15);
      expect(camera.snapshotUri, 'http://192.168.1.10/snapshot');
      expect(camera.supportsEvents, isTrue);
      expect(camera.supportsAnalytics, isTrue);
      expect(camera.supportsRelay, isTrue);
      expect(camera.createdAt, '2026-01-01T00:00:00Z');
      expect(camera.updatedAt, '2026-03-15T12:00:00Z');
      expect(camera.storagePath, '/data/recordings/cam-1');
      expect(camera.storageStatus, 'ok');
      expect(camera.liveViewPath, '/live/front_door');
      expect(camera.liveViewCodec, 'h264');
      expect(camera.streamPaths, hasLength(2));
      expect(camera.streamPaths[0].name, 'Main');
      expect(camera.streamPaths[1].videoCodec, 'h264');
    });

    test('fromJson uses defaults for missing optional fields', () {
      final json = {
        'id': 'cam-2',
        'name': 'Back Yard',
      };

      final camera = Camera.fromJson(json);

      expect(camera.rtspUrl, '');
      expect(camera.onvifEndpoint, '');
      expect(camera.mediamtxPath, '');
      expect(camera.status, 'disconnected');
      expect(camera.ptzCapable, isFalse);
      expect(camera.aiEnabled, isFalse);
      expect(camera.aiStreamId, '');
      expect(camera.aiConfidence, 0.5);
      expect(camera.aiTrackTimeout, 5);
      expect(camera.subStreamUrl, '');
      expect(camera.retentionDays, 30);
      expect(camera.eventRetentionDays, 0);
      expect(camera.detectionRetentionDays, 0);
      expect(camera.motionTimeoutSeconds, 8);
      expect(camera.snapshotUri, '');
      expect(camera.supportsEvents, isFalse);
      expect(camera.supportsAnalytics, isFalse);
      expect(camera.supportsRelay, isFalse);
      expect(camera.createdAt, isNull);
      expect(camera.updatedAt, isNull);
      expect(camera.storagePath, '');
      expect(camera.storageStatus, 'default');
      expect(camera.liveViewPath, '');
      expect(camera.liveViewCodec, '');
      expect(camera.streamPaths, isEmpty);
    });

    test('fromJson handles null timestamps', () {
      final json = {
        'id': 'cam-3',
        'name': 'Test',
        'created_at': null,
        'updated_at': null,
      };

      final camera = Camera.fromJson(json);

      expect(camera.createdAt, isNull);
      expect(camera.updatedAt, isNull);
    });

    test('copyWith overrides specific fields', () {
      final camera = Camera.fromJson({
        'id': 'cam-1',
        'name': 'Original',
        'status': 'connected',
      });

      final updated = camera.copyWith(name: 'Updated', status: 'disconnected');

      expect(updated.id, 'cam-1');
      expect(updated.name, 'Updated');
      expect(updated.status, 'disconnected');
    });

    test('fromJson with empty stream_paths list', () {
      final json = {
        'id': 'cam-1',
        'name': 'Test',
        'stream_paths': [],
      };

      final camera = Camera.fromJson(json);
      expect(camera.streamPaths, isEmpty);
    });
  });

  group('StreamPath', () {
    test('fromJson parses all fields', () {
      final json = {
        'name': 'Main Stream',
        'path': '/stream/main',
        'resolution': '1920x1080',
        'video_codec': 'h265',
      };

      final sp = StreamPath.fromJson(json);

      expect(sp.name, 'Main Stream');
      expect(sp.path, '/stream/main');
      expect(sp.resolution, '1920x1080');
      expect(sp.videoCodec, 'h265');
    });

    test('fromJson uses defaults for missing fields', () {
      final sp = StreamPath.fromJson({});

      expect(sp.name, '');
      expect(sp.path, '');
      expect(sp.resolution, '');
      expect(sp.videoCodec, '');
    });

    test('copyWith overrides specific fields', () {
      final sp = StreamPath.fromJson({
        'name': 'Main',
        'path': '/main',
      });

      final updated = sp.copyWith(name: 'Sub');

      expect(updated.name, 'Sub');
      expect(updated.path, '/main');
    });
  });
}
