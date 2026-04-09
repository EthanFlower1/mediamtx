// KAI-304 — Settings/Accounts: switchTo / addAccount / signOut behavior.
//
// These tests drive [AppSessionNotifier] directly against an in-memory token
// store. We assert the session-event sequence and the per-connection token
// isolation contract that the Accounts screen relies on.
//
// IMPORTANT: test tokens use `"REPLACE_ME_*"` placeholders. Never paste a real
// JWT into fixtures.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';
import 'package:nvr_client/state/secure_token_store.dart';

HomeDirectoryConnection _conn(String id, {String? name}) {
  return HomeDirectoryConnection(
    id: id,
    kind: HomeConnectionKind.onPrem,
    endpointUrl: 'https://$id.example',
    displayName: name ?? id,
    discoveryMethod: DiscoveryMethod.manual,
  );
}

void main() {
  group('AppSessionNotifier.addAccount', () {
    test('appends connection and persists tokens under the connectionId',
        () async {
      final store = InMemorySecureTokenStore();
      final notifier = AppSessionNotifier(store);

      await notifier.addAccount(
        connection: _conn('home-a'),
        accessToken: 'REPLACE_ME_ACCESS_A',
        refreshToken: 'REPLACE_ME_REFRESH_A',
      );

      expect(notifier.state.knownConnections.map((c) => c.id),
          equals(['home-a']));
      expect(await store.read(ConnectionScopedKeys.accessToken('home-a')),
          'REPLACE_ME_ACCESS_A');
      expect(await store.read(ConnectionScopedKeys.refreshToken('home-a')),
          'REPLACE_ME_REFRESH_A');
    });

    test('does not activate the new account by itself', () async {
      final store = InMemorySecureTokenStore();
      final notifier = AppSessionNotifier(store);

      await notifier.addAccount(
        connection: _conn('home-a'),
        accessToken: 'REPLACE_ME_ACCESS_A',
        refreshToken: 'REPLACE_ME_REFRESH_A',
      );

      expect(notifier.state.activeConnection, isNull);
      expect(notifier.state.accessToken, isNull);
    });

    test('tokens for different accounts are isolated by connectionId',
        () async {
      final store = InMemorySecureTokenStore();
      final notifier = AppSessionNotifier(store);

      await notifier.addAccount(
        connection: _conn('home-a'),
        accessToken: 'REPLACE_ME_ACCESS_A',
        refreshToken: 'REPLACE_ME_REFRESH_A',
      );
      await notifier.addAccount(
        connection: _conn('home-b'),
        accessToken: 'REPLACE_ME_ACCESS_B',
        refreshToken: 'REPLACE_ME_REFRESH_B',
      );

      expect(await store.read(ConnectionScopedKeys.accessToken('home-a')),
          'REPLACE_ME_ACCESS_A');
      expect(await store.read(ConnectionScopedKeys.accessToken('home-b')),
          'REPLACE_ME_ACCESS_B');
      expect(notifier.state.knownConnections.length, 2);
    });
  });

  group('AppSessionNotifier.switchTo', () {
    test('emits Switching then Switched and updates the active connection',
        () async {
      final store = InMemorySecureTokenStore();
      final sink = SessionEventSink();
      final notifier = AppSessionNotifier(store, events: sink);

      await notifier.activateConnection(
        connection: _conn('home-a'),
        userId: 'u-1',
        tenantRef: 't-1',
      );
      await notifier.setTokens(
        accessToken: 'REPLACE_ME_ACCESS_A',
        refreshToken: 'REPLACE_ME_REFRESH_A',
      );
      await notifier.addAccount(
        connection: _conn('home-b'),
        accessToken: 'REPLACE_ME_ACCESS_B',
        refreshToken: 'REPLACE_ME_REFRESH_B',
      );

      final events = <SessionEvent>[];
      final sub = sink.stream.listen(events.add);

      await notifier.switchTo('home-b');
      // Let the broadcast controller deliver.
      await Future<void>.delayed(Duration.zero);

      expect(events.length, 2);
      expect(events[0], isA<SessionSwitchingEvent>());
      final switching = events[0] as SessionSwitchingEvent;
      expect(switching.fromConnectionId, 'home-a');
      expect(switching.toConnectionId, 'home-b');
      expect(events[1], isA<SessionSwitchedEvent>());
      expect((events[1] as SessionSwitchedEvent).connectionId, 'home-b');

      expect(notifier.state.activeConnection?.id, 'home-b');
      expect(notifier.state.accessToken, 'REPLACE_ME_ACCESS_B');

      await sub.cancel();
    });

    test('times out cleanly if subscribers never drain', () async {
      final store = InMemorySecureTokenStore();
      final sink = SessionEventSink();
      // Register a subscriber that never acks — switch must still complete.
      sink.expectedDrainAcks = 1;
      final notifier = AppSessionNotifier(store, events: sink);

      await notifier.activateConnection(
        connection: _conn('home-a'),
        userId: 'u-1',
        tenantRef: 't-1',
      );
      await notifier.addAccount(
        connection: _conn('home-b'),
        accessToken: 'REPLACE_ME_ACCESS_B',
        refreshToken: 'REPLACE_ME_REFRESH_B',
      );

      final sw = Stopwatch()..start();
      await notifier.switchTo('home-b');
      sw.stop();

      // Hard cap is 2s; allow a little slack.
      expect(sw.elapsed, lessThan(const Duration(seconds: 4)));
      expect(notifier.state.activeConnection?.id, 'home-b');
    });

    test('completes immediately when subscriber acks the drain', () async {
      final store = InMemorySecureTokenStore();
      final sink = SessionEventSink();
      sink.expectedDrainAcks = 1;
      final notifier = AppSessionNotifier(store, events: sink);

      await notifier.activateConnection(
        connection: _conn('home-a'),
        userId: 'u-1',
        tenantRef: 't-1',
      );
      await notifier.addAccount(
        connection: _conn('home-b'),
        accessToken: 'REPLACE_ME_ACCESS_B',
        refreshToken: 'REPLACE_ME_REFRESH_B',
      );

      // Wire a subscriber that immediately acks on Switching.
      final sub = sink.stream.listen((event) {
        if (event is SessionSwitchingEvent) sink.ackSwitchDrained();
      });

      final sw = Stopwatch()..start();
      await notifier.switchTo('home-b');
      sw.stop();

      expect(sw.elapsed, lessThan(const Duration(milliseconds: 500)));
      expect(notifier.state.activeConnection?.id, 'home-b');

      await sub.cancel();
    });
  });

  group('AppSessionNotifier.signOut', () {
    test('deletes tokens and removes the account from the list', () async {
      final store = InMemorySecureTokenStore();
      final notifier = AppSessionNotifier(store);

      await notifier.addAccount(
        connection: _conn('home-a'),
        accessToken: 'REPLACE_ME_ACCESS_A',
        refreshToken: 'REPLACE_ME_REFRESH_A',
      );
      await notifier.addAccount(
        connection: _conn('home-b'),
        accessToken: 'REPLACE_ME_ACCESS_B',
        refreshToken: 'REPLACE_ME_REFRESH_B',
      );

      await notifier.signOut('home-a');

      expect(notifier.state.knownConnections.map((c) => c.id),
          equals(['home-b']));
      expect(await store.read(ConnectionScopedKeys.accessToken('home-a')),
          isNull);
      expect(await store.read(ConnectionScopedKeys.refreshToken('home-a')),
          isNull);
      // Unrelated account untouched.
      expect(await store.read(ConnectionScopedKeys.accessToken('home-b')),
          'REPLACE_ME_ACCESS_B');
    });

    test('signing out the active account promotes the next sibling',
        () async {
      final store = InMemorySecureTokenStore();
      final notifier = AppSessionNotifier(store);

      await notifier.activateConnection(
        connection: _conn('home-a'),
        userId: 'u-1',
        tenantRef: 't-1',
      );
      await notifier.setTokens(
        accessToken: 'REPLACE_ME_ACCESS_A',
        refreshToken: 'REPLACE_ME_REFRESH_A',
      );
      await notifier.addAccount(
        connection: _conn('home-b'),
        accessToken: 'REPLACE_ME_ACCESS_B',
        refreshToken: 'REPLACE_ME_REFRESH_B',
      );

      await notifier.signOut('home-a');

      expect(notifier.state.activeConnection?.id, 'home-b');
      expect(notifier.state.accessToken, 'REPLACE_ME_ACCESS_B');
      expect(notifier.state.knownConnections.map((c) => c.id),
          equals(['home-b']));
    });

    test('signing out the last account returns to empty session', () async {
      final store = InMemorySecureTokenStore();
      final notifier = AppSessionNotifier(store);

      await notifier.activateConnection(
        connection: _conn('home-a'),
        userId: 'u-1',
        tenantRef: 't-1',
      );
      await notifier.setTokens(
        accessToken: 'REPLACE_ME_ACCESS_A',
        refreshToken: 'REPLACE_ME_REFRESH_A',
      );

      await notifier.signOut('home-a');

      expect(notifier.state.activeConnection, isNull);
      expect(notifier.state.knownConnections, isEmpty);
      expect(notifier.state.accessToken, isNull);
    });
  });
}
