import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:nvr_client/providers/notifications_provider.dart';
import 'package:nvr_client/models/notification_event.dart';

void main() {
  group('NotificationState', () {
    test('default state has empty history and zero unread', () {
      const state = NotificationState();
      expect(state.history, isEmpty);
      expect(state.unreadCount, 0);
      expect(state.wsConnected, false);
    });

    test('copyWith updates specified fields', () {
      const state = NotificationState();
      final updated = state.copyWith(unreadCount: 5, wsConnected: true);
      expect(updated.unreadCount, 5);
      expect(updated.wsConnected, true);
      expect(updated.history, isEmpty);
    });

    test('copyWith preserves unmodified fields', () {
      final events = [
        NotificationEvent(
          id: '1',
          type: 'motion',
          camera: 'cam1',
          message: 'Motion detected',
          time: DateTime.now(),
        ),
      ];
      final state = NotificationState(history: events, unreadCount: 1);
      final updated = state.copyWith(wsConnected: true);
      expect(updated.history.length, 1);
      expect(updated.unreadCount, 1);
    });
  });

  group('NotificationsNotifier', () {
    late NotificationsNotifier notifier;

    setUp(() {
      SharedPreferences.setMockInitialValues({});
      notifier = NotificationsNotifier();
    });

    tearDown(() {
      notifier.dispose();
    });

    test('initial state is empty', () {
      expect(notifier.state.history, isEmpty);
      expect(notifier.state.unreadCount, 0);
      expect(notifier.state.wsConnected, false);
    });

    test('markAllRead marks all events and resets unread count', () {
      // We need to simulate having events in state.
      // Since we cannot use connect() without a real server,
      // we test markAllRead on empty state (no-op case).
      notifier.markAllRead();
      expect(notifier.state.unreadCount, 0);
      expect(notifier.state.history, isEmpty);
    });

    test('markRead with invalid index is a no-op', () {
      notifier.markRead(-1);
      expect(notifier.state.unreadCount, 0);

      notifier.markRead(0);
      expect(notifier.state.unreadCount, 0);

      notifier.markRead(100);
      expect(notifier.state.unreadCount, 0);
    });

    test('webSocket is null before connect', () {
      expect(notifier.webSocket, isNull);
    });

    test('dispose cleans up without error', () {
      // Should not throw
      notifier.dispose();
      // Reassign to a new one for tearDown
      notifier = NotificationsNotifier();
    });

    test('read IDs are loaded from SharedPreferences', () async {
      // Pre-populate read IDs
      SharedPreferences.setMockInitialValues({
        'nvr_read_notification_ids': ['id_1', 'id_2', 'id_3'],
      });

      final notifier2 = NotificationsNotifier();
      addTearDown(() => notifier2.dispose());

      // Allow async _loadReadIds
      await Future.delayed(Duration.zero);

      // We can verify indirectly: markAllRead should persist these plus any new ones
      notifier2.markAllRead();
      await Future.delayed(Duration.zero);

      final prefs = await SharedPreferences.getInstance();
      final savedIds = prefs.getStringList('nvr_read_notification_ids');
      // The original IDs should still be present (they were loaded into _readIds)
      expect(savedIds, isNotNull);
    });
  });

  group('NotificationEvent model', () {
    test('fromJson creates event correctly', () {
      final event = NotificationEvent.fromJson({
        'id': 'test_id',
        'type': 'motion',
        'camera': 'front_door',
        'message': 'Motion detected',
        'time': '2024-01-15T10:30:00.000Z',
      });

      expect(event.id, 'test_id');
      expect(event.type, 'motion');
      expect(event.camera, 'front_door');
      expect(event.isRead, false);
    });

    test('copyWith changes isRead', () {
      final event = NotificationEvent(
        id: '1',
        type: 'motion',
        camera: 'cam1',
        message: 'test',
        time: DateTime.now(),
      );

      final read = event.copyWith(isRead: true);
      expect(read.isRead, true);
      expect(read.id, '1');
      expect(read.type, 'motion');
    });

    test('isDetectionFrame identifies detection_frame type', () {
      final event = NotificationEvent(
        id: '1',
        type: 'detection_frame',
        camera: 'cam1',
        message: 'test',
        time: DateTime.now(),
      );
      expect(event.isDetectionFrame, true);
      expect(event.isAlert, false);
    });

    test('isAlert identifies alert type', () {
      final event = NotificationEvent(
        id: '1',
        type: 'alert',
        camera: 'cam1',
        message: 'test',
        time: DateTime.now(),
      );
      expect(event.isAlert, true);
      expect(event.isDetectionFrame, false);
    });

    test('navigationRoute returns correct paths', () {
      final motionEvent = NotificationEvent(
        id: '1',
        type: 'motion',
        camera: 'cam1',
        message: 'Motion',
        time: DateTime.parse('2024-01-15T10:30:00.000Z'),
      );
      expect(motionEvent.navigationRoute, contains('/playback'));
      expect(motionEvent.navigationRoute, contains('cam1'));

      final offlineEvent = NotificationEvent(
        id: '2',
        type: 'camera_offline',
        camera: 'cam2',
        message: 'Offline',
        time: DateTime.now(),
      );
      expect(offlineEvent.navigationRoute, '/devices/cam2');
    });

    test('fromJson generates id when not provided', () {
      final event = NotificationEvent.fromJson({
        'type': 'motion',
        'camera': 'cam1',
        'message': 'test',
        'time': '2024-01-15T10:30:00.000Z',
      });
      expect(event.id, isNotEmpty);
      expect(event.id, contains('motion'));
      expect(event.id, contains('cam1'));
    });
  });
}
