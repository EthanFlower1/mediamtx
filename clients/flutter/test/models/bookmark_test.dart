import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/bookmark.dart';

void main() {
  group('Bookmark', () {
    test('fromJson parses all fields', () {
      final json = {
        'id': 42,
        'camera_id': 'cam-1',
        'timestamp': '2026-04-01T10:30:00Z',
        'label': 'Suspicious person',
        'created_by': 'admin',
        'created_at': '2026-04-01T10:31:00Z',
      };

      final bookmark = Bookmark.fromJson(json);

      expect(bookmark.id, 42);
      expect(bookmark.cameraId, 'cam-1');
      expect(bookmark.timestamp, DateTime.utc(2026, 4, 1, 10, 30));
      expect(bookmark.label, 'Suspicious person');
      expect(bookmark.createdBy, 'admin');
      expect(bookmark.createdAt, DateTime.utc(2026, 4, 1, 10, 31));
    });

    test('fromJson handles null created_by', () {
      final json = {
        'id': 1,
        'camera_id': 'cam-2',
        'timestamp': '2026-04-01T12:00:00Z',
        'label': 'Test',
        'created_by': null,
        'created_at': '2026-04-01T12:01:00Z',
      };

      final bookmark = Bookmark.fromJson(json);

      expect(bookmark.createdBy, isNull);
    });

    test('fromJson handles missing created_by', () {
      final json = {
        'id': 1,
        'camera_id': 'cam-2',
        'timestamp': '2026-04-01T12:00:00Z',
        'label': 'Test',
        'created_at': '2026-04-01T12:01:00Z',
      };

      final bookmark = Bookmark.fromJson(json);

      expect(bookmark.createdBy, isNull);
    });
  });
}
