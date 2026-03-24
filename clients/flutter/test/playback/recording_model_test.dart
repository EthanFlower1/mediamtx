import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/recording.dart';

void main() {
  group('RecordingSegment', () {
    test('fromJson parses backend response correctly', () {
      final json = {
        'id': 42,
        'camera_id': 'cam-1',
        'start_time': '2026-03-24T10:00:00Z',
        'end_time': '2026-03-24T10:15:00Z',
        'duration_ms': 900000,
        'file_path': '/recordings/cam1/2026-03-24/10-00.fmp4',
        'file_size': 15000000,
        'format': 'fmp4',
      };

      final segment = RecordingSegment.fromJson(json);

      expect(segment.id, 42);
      expect(segment.cameraId, 'cam-1');
      expect(segment.startTime, DateTime.utc(2026, 3, 24, 10, 0, 0));
      expect(segment.endTime, DateTime.utc(2026, 3, 24, 10, 15, 0));
      expect(segment.durationMs, 900000);
      expect(segment.filePath, '/recordings/cam1/2026-03-24/10-00.fmp4');
      expect(segment.fileSize, 15000000);
      expect(segment.format, 'fmp4');
    });

    test('fromJson handles nullable fields', () {
      final json = {
        'id': 1,
        'camera_id': 'cam-1',
        'start_time': '2026-03-24T10:00:00Z',
        'end_time': '2026-03-24T10:15:00Z',
        'duration_ms': 900000,
      };

      final segment = RecordingSegment.fromJson(json);

      expect(segment.filePath, isNull);
      expect(segment.fileSize, isNull);
      expect(segment.format, isNull);
    });
  });

  group('MotionEvent', () {
    test('fromJson parses id as string from int', () {
      final json = {
        'id': 99,
        'camera_id': 'cam-1',
        'started_at': '2026-03-24T10:05:00Z',
        'ended_at': '2026-03-24T10:05:30Z',
        'thumbnail_path': '/thumbnails/event_99.jpg',
        'event_type': 'motion',
        'object_class': 'person',
        'confidence': 0.92,
      };

      final event = MotionEvent.fromJson(json);

      expect(event.cameraId, 'cam-1');
      expect(event.startTime, DateTime.utc(2026, 3, 24, 10, 5, 0));
      expect(event.endTime, DateTime.utc(2026, 3, 24, 10, 5, 30));
      expect(event.objectClass, 'person');
      expect(event.confidence, 0.92);
    });
  });
}
