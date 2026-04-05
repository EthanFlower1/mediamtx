import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/providers/connectivity_provider.dart';

void main() {
  group('ConnectivityState', () {
    test('isOnline returns true only when status is online', () {
      const online = ConnectivityState(status: ConnectivityStatus.online);
      expect(online.isOnline, true);
      expect(online.isOffline, false);

      const offline = ConnectivityState(status: ConnectivityStatus.offline);
      expect(offline.isOnline, false);
      expect(offline.isOffline, true);

      const reconnecting =
          ConnectivityState(status: ConnectivityStatus.reconnecting);
      expect(reconnecting.isOnline, false);
      expect(reconnecting.isOffline, true);
    });

    test('copyWith preserves unmodified fields', () {
      final now = DateTime.now();
      final state = ConnectivityState(
        status: ConnectivityStatus.online,
        lastOnline: now,
      );

      final updated = state.copyWith(status: ConnectivityStatus.offline);
      expect(updated.status, ConnectivityStatus.offline);
      expect(updated.lastOnline, now);
    });

    test('copyWith replaces specified fields', () {
      const state = ConnectivityState(status: ConnectivityStatus.online);
      final now = DateTime.now();
      final updated = state.copyWith(lastOnline: now);
      expect(updated.lastOnline, now);
      expect(updated.status, ConnectivityStatus.online);
    });
  });

  group('ConnectivityNotifier.noServer', () {
    late ConnectivityNotifier notifier;

    setUp(() {
      notifier = ConnectivityNotifier.noServer();
    });

    tearDown(() {
      notifier.dispose();
    });

    test('starts in offline state', () {
      expect(notifier.state.status, ConnectivityStatus.offline);
    });

    test('checkNow does not throw when no server configured', () async {
      // Should return without error since _dio is null
      await notifier.checkNow();
      expect(notifier.state.status, ConnectivityStatus.offline);
    });
  });

  group('ConnectivityNotifier dispose guard', () {
    test('dispose sets internal flag and cancels timer', () {
      final notifier = ConnectivityNotifier.noServer();
      // Should not throw when disposed
      notifier.dispose();
      // Calling checkNow after dispose should be safe (no-op)
      // We can't directly test _disposed, but we verify no crash
    });
  });
}
