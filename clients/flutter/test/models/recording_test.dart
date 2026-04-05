import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/recording.dart';

void main() {
  group('RecordingSegment', () {
    test('fromJson parses all fields', () {
      final json = {
        'id': 1,
        'camera_id': 'cam-1',
        'start_time': '2026-04-01T10:00:00Z',
        'end_time': '2026-04-01T10:05:00Z',
        'duration_ms': 300000,
        'file_path': '/recordings/cam-1/segment1.mp4',
        'file_size': 52428800,
        'format': 'mp4',
      };

      final segment = RecordingSegment.fromJson(json);

      expect(segment.id, 1);
      expect(segment.cameraId, 'cam-1');
      expect(segment.startTime, DateTime.utc(2026, 4, 1, 10));
      expect(segment.endTime, DateTime.utc(2026, 4, 1, 10, 5));
      expect(segment.durationMs, 300000);
      expect(segment.filePath, '/recordings/cam-1/segment1.mp4');
      expect(segment.fileSize, 52428800);
      expect(segment.format, 'mp4');
    });

    test('fromJson handles null optional fields', () {
      final json = {
        'id': 2,
        'camera_id': 'cam-2',
        'start_time': '2026-04-01T10:00:00Z',
        'end_time': '2026-04-01T10:01:00Z',
        'duration_ms': 60000,
        'file_path': null,
        'file_size': null,
        'format': null,
      };

      final segment = RecordingSegment.fromJson(json);

      expect(segment.filePath, isNull);
      expect(segment.fileSize, isNull);
      expect(segment.format, isNull);
    });

    test('fromJson handles missing optional fields', () {
      final json = {
        'id': 3,
        'camera_id': 'cam-3',
        'start_time': '2026-04-01T10:00:00Z',
        'end_time': '2026-04-01T10:01:00Z',
        'duration_ms': 60000,
      };

      final segment = RecordingSegment.fromJson(json);

      expect(segment.filePath, isNull);
      expect(segment.fileSize, isNull);
      expect(segment.format, isNull);
    });
  });

  group('MotionEvent', () {
    test('fromJson parses all fields', () {
      final json = {
        'id': 'evt-1',
        'camera_id': 'cam-1',
        'started_at': '2026-04-01T10:00:00Z',
        'ended_at': '2026-04-01T10:00:30Z',
        'thumbnail_path': '/thumbs/evt-1.jpg',
        'event_type': 'motion',
        'object_class': 'person',
        'confidence': 0.92,
      };

      final event = MotionEvent.fromJson(json);

      expect(event.id, 'evt-1');
      expect(event.cameraId, 'cam-1');
      expect(event.startedAt, '2026-04-01T10:00:00Z');
      expect(event.endedAt, '2026-04-01T10:00:30Z');
      expect(event.thumbnailPath, '/thumbs/evt-1.jpg');
      expect(event.eventType, 'motion');
      expect(event.objectClass, 'person');
      expect(event.confidence, 0.92);
    });

    test('fromJson handles missing optional fields', () {
      final json = {
        'id': 'evt-2',
        'camera_id': 'cam-2',
        'started_at': '2026-04-01T10:00:00Z',
      };

      final event = MotionEvent.fromJson(json);

      expect(event.endedAt, isNull);
      expect(event.thumbnailPath, isNull);
      expect(event.eventType, isNull);
      expect(event.objectClass, isNull);
      expect(event.confidence, isNull);
    });

    test('fromJson handles int id via toString', () {
      final json = {
        'id': 42,
        'camera_id': 'cam-1',
        'started_at': '2026-04-01T10:00:00Z',
      };

      final event = MotionEvent.fromJson(json);

      expect(event.id, '42');
    });

    test('fromJson handles int confidence', () {
      final json = {
        'id': 'evt-3',
        'camera_id': 'cam-1',
        'started_at': '2026-04-01T10:00:00Z',
        'confidence': 1,
      };

      final event = MotionEvent.fromJson(json);

      expect(event.confidence, 1.0);
    });

    test('startTime getter parses startedAt', () {
      final event = MotionEvent.fromJson({
        'id': 'evt-1',
        'camera_id': 'cam-1',
        'started_at': '2026-04-01T10:00:00Z',
      });

      expect(event.startTime, DateTime.utc(2026, 4, 1, 10));
    });

    test('endTime getter parses endedAt when present', () {
      final event = MotionEvent.fromJson({
        'id': 'evt-1',
        'camera_id': 'cam-1',
        'started_at': '2026-04-01T10:00:00Z',
        'ended_at': '2026-04-01T10:00:30Z',
      });

      expect(event.endTime, DateTime.utc(2026, 4, 1, 10, 0, 30));
    });

    test('endTime getter returns null when endedAt is null', () {
      final event = MotionEvent.fromJson({
        'id': 'evt-1',
        'camera_id': 'cam-1',
        'started_at': '2026-04-01T10:00:00Z',
      });

      expect(event.endTime, isNull);
    });
  });
}
