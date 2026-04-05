import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/search_result.dart';

void main() {
  group('SearchResult', () {
    test('fromJson parses all fields', () {
      final json = {
        'detection_id': 'det-1',
        'event_id': 'evt-1',
        'camera_id': 'cam-1',
        'camera_name': 'Front Door',
        'class': 'person',
        'confidence': 0.95,
        'similarity': 0.87,
        'frame_time': '2026-04-01T10:00:00Z',
        'thumbnail_path': '/thumbs/det-1.jpg',
      };

      final result = SearchResult.fromJson(json);

      expect(result.detectionId, 'det-1');
      expect(result.eventId, 'evt-1');
      expect(result.cameraId, 'cam-1');
      expect(result.cameraName, 'Front Door');
      expect(result.className, 'person');
      expect(result.confidence, 0.95);
      expect(result.similarity, 0.87);
      expect(result.frameTime, '2026-04-01T10:00:00Z');
      expect(result.thumbnailPath, '/thumbs/det-1.jpg');
    });

    test('fromJson handles missing fields', () {
      final result = SearchResult.fromJson({});

      expect(result.detectionId, '');
      expect(result.eventId, '');
      expect(result.cameraId, '');
      expect(result.cameraName, '');
      expect(result.className, '');
      expect(result.confidence, 0.0);
      expect(result.similarity, 0.0);
      expect(result.frameTime, '');
      expect(result.thumbnailPath, isNull);
    });

    test('fromJson handles null values', () {
      final json = {
        'detection_id': null,
        'event_id': null,
        'camera_id': null,
        'camera_name': null,
        'class': null,
        'confidence': null,
        'similarity': null,
        'frame_time': null,
        'thumbnail_path': null,
      };

      final result = SearchResult.fromJson(json);

      expect(result.detectionId, '');
      expect(result.confidence, 0.0);
      expect(result.similarity, 0.0);
      expect(result.thumbnailPath, isNull);
    });

    test('_toDouble handles int values', () {
      final json = {
        'detection_id': 'det-1',
        'event_id': 'evt-1',
        'camera_id': 'cam-1',
        'camera_name': 'Test',
        'class': 'car',
        'confidence': 1,
        'similarity': 0,
        'frame_time': '2026-04-01T10:00:00Z',
      };

      final result = SearchResult.fromJson(json);

      expect(result.confidence, 1.0);
      expect(result.similarity, 0.0);
    });

    test('_toDouble handles String values', () {
      final json = {
        'detection_id': 'det-1',
        'event_id': 'evt-1',
        'camera_id': 'cam-1',
        'camera_name': 'Test',
        'class': 'car',
        'confidence': '0.75',
        'similarity': '0.5',
        'frame_time': '2026-04-01T10:00:00Z',
      };

      final result = SearchResult.fromJson(json);

      expect(result.confidence, 0.75);
      expect(result.similarity, 0.5);
    });

    test('_toDouble handles unparseable String', () {
      final json = {
        'detection_id': 'det-1',
        'event_id': 'evt-1',
        'camera_id': 'cam-1',
        'camera_name': 'Test',
        'class': 'car',
        'confidence': 'not-a-number',
        'similarity': '',
        'frame_time': '2026-04-01T10:00:00Z',
      };

      final result = SearchResult.fromJson(json);

      expect(result.confidence, 0.0);
      expect(result.similarity, 0.0);
    });

    test('time getter parses frameTime', () {
      final result = SearchResult.fromJson({
        'detection_id': 'd1',
        'event_id': 'e1',
        'camera_id': 'c1',
        'camera_name': 'Test',
        'class': 'person',
        'confidence': 0.5,
        'similarity': 0.5,
        'frame_time': '2026-04-01T10:00:00Z',
      });

      expect(result.time, DateTime.utc(2026, 4, 1, 10));
    });

    test('time getter returns epoch zero for empty frameTime', () {
      final result = SearchResult.fromJson({});

      expect(result.time, DateTime.fromMillisecondsSinceEpoch(0));
    });

    test('time getter returns epoch zero for unparseable frameTime', () {
      final result = SearchResult.fromJson({
        'frame_time': 'not-a-date',
      });

      expect(result.time, DateTime.fromMillisecondsSinceEpoch(0));
    });

    test('fromJson handles int id via toString', () {
      final json = {
        'detection_id': 123,
        'event_id': 456,
        'camera_id': 789,
        'camera_name': 'Test',
        'class': 'car',
        'confidence': 0.5,
        'similarity': 0.5,
        'frame_time': '2026-04-01T10:00:00Z',
      };

      final result = SearchResult.fromJson(json);

      expect(result.detectionId, '123');
      expect(result.eventId, '456');
      expect(result.cameraId, '789');
    });
  });
}
