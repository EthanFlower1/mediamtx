import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/saved_clip.dart';

void main() {
  group('SavedClip', () {
    test('fromJson parses all fields', () {
      final json = {
        'id': 'clip-1',
        'camera_id': 'cam-1',
        'name': 'Suspicious Activity',
        'start_time': '2026-04-01T10:00:00Z',
        'end_time': '2026-04-01T10:05:00Z',
        'tags': ['security', 'person'],
        'notes': 'Person near fence at 10am',
        'created_at': '2026-04-01T10:06:00Z',
      };

      final clip = SavedClip.fromJson(json);

      expect(clip.id, 'clip-1');
      expect(clip.cameraId, 'cam-1');
      expect(clip.name, 'Suspicious Activity');
      expect(clip.startTime, '2026-04-01T10:00:00Z');
      expect(clip.endTime, '2026-04-01T10:05:00Z');
      expect(clip.tags, ['security', 'person']);
      expect(clip.notes, 'Person near fence at 10am');
      expect(clip.createdAt, '2026-04-01T10:06:00Z');
    });

    test('fromJson handles missing optional fields', () {
      final json = <String, dynamic>{};

      final clip = SavedClip.fromJson(json);

      expect(clip.id, '');
      expect(clip.cameraId, '');
      expect(clip.name, '');
      expect(clip.startTime, '');
      expect(clip.endTime, '');
      expect(clip.tags, isEmpty);
      expect(clip.notes, isNull);
      expect(clip.createdAt, '');
    });

    test('fromJson handles null values via toString', () {
      final json = {
        'id': null,
        'camera_id': null,
        'name': null,
        'start_time': null,
        'end_time': null,
        'tags': null,
        'notes': null,
        'created_at': null,
      };

      final clip = SavedClip.fromJson(json);

      expect(clip.id, '');
      expect(clip.tags, isEmpty);
      // notes?.toString() on null returns null
      expect(clip.notes, isNull);
    });

    test('fromJson handles int id via toString', () {
      final json = {
        'id': 42,
        'camera_id': 'cam-1',
        'name': 'Test',
        'start_time': '2026-04-01T10:00:00Z',
        'end_time': '2026-04-01T10:05:00Z',
        'tags': [],
        'created_at': '2026-04-01T10:06:00Z',
      };

      final clip = SavedClip.fromJson(json);

      expect(clip.id, '42');
    });

    test('fromJson handles tags with non-string elements', () {
      final json = {
        'id': 'clip-1',
        'camera_id': 'cam-1',
        'name': 'Test',
        'start_time': 't1',
        'end_time': 't2',
        'tags': [1, true, 'text'],
        'created_at': 't3',
      };

      final clip = SavedClip.fromJson(json);

      expect(clip.tags, ['1', 'true', 'text']);
    });
  });
}
