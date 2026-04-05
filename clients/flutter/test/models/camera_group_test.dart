import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/camera_group.dart';

void main() {
  group('CameraGroup', () {
    test('fromJson parses all fields', () {
      final json = {
        'id': 'grp-1',
        'name': 'Perimeter',
        'camera_ids': ['cam-1', 'cam-2', 'cam-3'],
        'created_at': '2026-01-01T00:00:00Z',
        'updated_at': '2026-03-01T00:00:00Z',
      };

      final group = CameraGroup.fromJson(json);

      expect(group.id, 'grp-1');
      expect(group.name, 'Perimeter');
      expect(group.cameraIds, ['cam-1', 'cam-2', 'cam-3']);
      expect(group.createdAt, '2026-01-01T00:00:00Z');
      expect(group.updatedAt, '2026-03-01T00:00:00Z');
    });

    test('fromJson defaults camera_ids to empty list', () {
      final json = {
        'id': 'grp-2',
        'name': 'Empty Group',
        'created_at': null,
        'updated_at': null,
      };

      final group = CameraGroup.fromJson(json);

      expect(group.cameraIds, isEmpty);
      expect(group.createdAt, isNull);
      expect(group.updatedAt, isNull);
    });

    test('copyWith overrides specific fields', () {
      final group = CameraGroup.fromJson({
        'id': 'grp-1',
        'name': 'Original',
        'camera_ids': ['cam-1'],
        'created_at': '2026-01-01T00:00:00Z',
        'updated_at': null,
      });

      final updated = group.copyWith(name: 'Updated', cameraIds: ['cam-1', 'cam-2']);

      expect(updated.id, 'grp-1');
      expect(updated.name, 'Updated');
      expect(updated.cameraIds, hasLength(2));
    });
  });
}
