import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/api/notification_api.dart';

void main() {
  group('NotificationItem', () {
    test('fromJson parses complete item', () {
      final json = {
        'id': 'abc-123',
        'type': 'motion',
        'severity': 'warning',
        'camera': 'cam1',
        'message': 'Motion detected on cam1',
        'created_at': '2026-04-10T12:00:00Z',
        'read_at': '2026-04-10T12:05:00Z',
        'archived': false,
      };

      final item = NotificationItem.fromJson(json);

      expect(item.id, 'abc-123');
      expect(item.type, 'motion');
      expect(item.severity, 'warning');
      expect(item.camera, 'cam1');
      expect(item.message, 'Motion detected on cam1');
      expect(item.isRead, true);
      expect(item.archived, false);
    });

    test('fromJson handles null read_at as unread', () {
      final json = {
        'id': 'def-456',
        'type': 'camera_offline',
        'severity': 'critical',
        'camera': 'cam2',
        'message': 'Camera went offline',
        'created_at': '2026-04-10T12:00:00Z',
        'read_at': null,
        'archived': false,
      };

      final item = NotificationItem.fromJson(json);
      expect(item.isRead, false);
    });

    test('fromJson handles missing fields gracefully', () {
      final json = <String, dynamic>{};
      final item = NotificationItem.fromJson(json);

      expect(item.id, '');
      expect(item.type, '');
      expect(item.severity, 'info');
      expect(item.camera, '');
      expect(item.message, '');
      expect(item.isRead, false);
      expect(item.archived, false);
    });

    test('copyWith updates readAt', () {
      final item = NotificationItem(
        id: 'test',
        type: 'motion',
        severity: 'warning',
        camera: 'cam1',
        message: 'test',
        createdAt: DateTime(2026, 4, 10),
      );

      expect(item.isRead, false);

      final read = item.copyWith(readAt: '2026-04-10T12:05:00Z');
      expect(read.isRead, true);
      expect(read.readAt, '2026-04-10T12:05:00Z');

      final unread = read.copyWith(clearReadAt: true);
      expect(unread.isRead, false);
      expect(unread.readAt, null);
    });

    test('copyWith updates archived', () {
      final item = NotificationItem(
        id: 'test',
        type: 'motion',
        severity: 'warning',
        camera: 'cam1',
        message: 'test',
        createdAt: DateTime(2026, 4, 10),
      );

      final archived = item.copyWith(archived: true);
      expect(archived.archived, true);
    });
  });

  group('NotificationPage', () {
    test('fromJson parses page response', () {
      final json = {
        'notifications': [
          {
            'id': 'a',
            'type': 'motion',
            'severity': 'warning',
            'camera': 'cam1',
            'message': 'Motion',
            'created_at': '2026-04-10T12:00:00Z',
            'read_at': null,
            'archived': false,
          },
          {
            'id': 'b',
            'type': 'camera_offline',
            'severity': 'critical',
            'camera': 'cam2',
            'message': 'Offline',
            'created_at': '2026-04-10T11:00:00Z',
            'read_at': '2026-04-10T11:05:00Z',
            'archived': false,
          },
        ],
        'total': 2,
        'limit': 30,
        'offset': 0,
      };

      final page = NotificationPage.fromJson(json);
      expect(page.notifications.length, 2);
      expect(page.total, 2);
      expect(page.limit, 30);
      expect(page.offset, 0);
      expect(page.notifications[0].type, 'motion');
      expect(page.notifications[1].isRead, true);
    });

    test('fromJson handles empty notifications', () {
      final json = {
        'notifications': null,
        'total': 0,
        'limit': 30,
        'offset': 0,
      };

      final page = NotificationPage.fromJson(json);
      expect(page.notifications.isEmpty, true);
      expect(page.total, 0);
    });
  });

  group('NotificationFilter', () {
    test('default filter has no active filters', () {
      const filter = NotificationFilter();
      expect(filter.hasActiveFilters, false);
    });

    test('filter with camera has active filters', () {
      const filter = NotificationFilter(camera: 'cam1');
      expect(filter.hasActiveFilters, true);
    });

    test('copyWith preserves other fields', () {
      const filter = NotificationFilter(
        camera: 'cam1',
        type: 'motion',
        severity: 'warning',
      );

      final updated = filter.copyWith(camera: 'cam2');
      expect(updated.camera, 'cam2');
      expect(updated.type, 'motion');
      expect(updated.severity, 'warning');
    });

    test('copyWith can toggle archived', () {
      const filter = NotificationFilter();
      final archived = filter.copyWith(archived: true);
      expect(archived.archived, true);
    });
  });
}
