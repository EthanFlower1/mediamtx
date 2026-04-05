import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/tour.dart';

void main() {
  group('Tour', () {
    test('fromJson parses all fields', () {
      final json = {
        'id': 'tour-1',
        'name': 'Perimeter Tour',
        'camera_ids': ['cam-1', 'cam-2', 'cam-3'],
        'dwell_seconds': 15,
        'created_at': '2026-01-01T00:00:00Z',
        'updated_at': '2026-03-01T00:00:00Z',
      };

      final tour = Tour.fromJson(json);

      expect(tour.id, 'tour-1');
      expect(tour.name, 'Perimeter Tour');
      expect(tour.cameraIds, ['cam-1', 'cam-2', 'cam-3']);
      expect(tour.dwellSeconds, 15);
      expect(tour.createdAt, '2026-01-01T00:00:00Z');
      expect(tour.updatedAt, '2026-03-01T00:00:00Z');
    });

    test('fromJson uses defaults for missing optional fields', () {
      final json = {
        'id': 'tour-2',
        'name': 'Empty Tour',
        'created_at': null,
        'updated_at': null,
      };

      final tour = Tour.fromJson(json);

      expect(tour.cameraIds, isEmpty);
      expect(tour.dwellSeconds, 10);
      expect(tour.createdAt, isNull);
      expect(tour.updatedAt, isNull);
    });

    test('copyWith overrides specific fields', () {
      final tour = Tour.fromJson({
        'id': 'tour-1',
        'name': 'Original',
        'camera_ids': ['cam-1'],
        'dwell_seconds': 10,
        'created_at': '2026-01-01T00:00:00Z',
        'updated_at': null,
      });

      final updated = tour.copyWith(
        name: 'Updated Tour',
        dwellSeconds: 20,
        cameraIds: ['cam-1', 'cam-2'],
      );

      expect(updated.id, 'tour-1');
      expect(updated.name, 'Updated Tour');
      expect(updated.dwellSeconds, 20);
      expect(updated.cameraIds, hasLength(2));
    });
  });
}
