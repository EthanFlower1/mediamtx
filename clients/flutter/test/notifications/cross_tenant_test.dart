// KAI-303 — Cross-tenant fetch guard test.
//
// A push that arrives for a tenant the user is no longer signed in to
// must NOT be allowed to fetch event details against the current
// tenant's Directory. This test asserts the guard in
// NotificationService.resolveForTap fires as a CrossTenantPushViolation.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/notifications/event_details_loader.dart';
import 'package:nvr_client/notifications/notification_service.dart';
import 'package:nvr_client/notifications/push_message.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';

import 'fakes.dart';

AppSession _session(String tenant) => AppSession(
      userId: 'u',
      tenantRef: tenant,
      accessToken: 'tok',
      activeConnection: const HomeDirectoryConnection(
        id: 'home-A',
        displayName: 'Home',
        endpointUrl: 'https://home.example.com',
        kind: HomeConnectionKind.onPrem,
        discoveryMethod: DiscoveryMethod.manual,
      ),
    );

void main() {
  group('CrossTenantPushViolation', () {
    test('fires when AppSession.tenantRef != msg.tenantId', () async {
      final loader = FakeEventDetailsLoader();
      final svc = NotificationService(
        channel: FakePushChannel(token: 'tok'),
        subscriptionClient: FakePushSubscriptionClient(),
        eventDetailsLoader: loader,
        readAppSession: () => _session('tenant-acme'),
      );

      const stalePush = PushMessage(
        eventId: 'evt-stale',
        tenantId: 'tenant-other',
        priority: 1,
      );

      await expectLater(
        () => svc.resolveForTap(stalePush),
        throwsA(isA<CrossTenantPushViolation>()),
      );
      expect(loader.loadedEventIds, isEmpty);
    });

    test('passes when tenants match', () async {
      final loader = FakeEventDetailsLoader(events: {
        'evt-1': EventDetails(
          eventId: 'evt-1',
          cameraId: 'c',
          cameraLabel: 'Lobby',
          kind: 'motion',
          timestamp: DateTime.utc(2026, 4, 8),
        ),
      });
      final svc = NotificationService(
        channel: FakePushChannel(token: 'tok'),
        subscriptionClient: FakePushSubscriptionClient(),
        eventDetailsLoader: loader,
        readAppSession: () => _session('tenant-acme'),
      );

      const msg = PushMessage(
        eventId: 'evt-1',
        tenantId: 'tenant-acme',
        priority: 1,
      );

      final details = await svc.resolveForTap(msg);
      expect(details.cameraLabel, 'Lobby');
      expect(loader.loadedEventIds, ['evt-1']);
    });

    test('fires on empty AppSession.tenantRef (unauthenticated)', () async {
      final svc = NotificationService(
        channel: FakePushChannel(token: 'tok'),
        subscriptionClient: FakePushSubscriptionClient(),
        eventDetailsLoader: FakeEventDetailsLoader(),
        readAppSession: () => AppSession.empty,
      );
      const msg = PushMessage(
        eventId: 'evt-1',
        tenantId: 'tenant-acme',
        priority: 1,
      );
      await expectLater(
        () => svc.resolveForTap(msg),
        throwsA(isA<CrossTenantPushViolation>()),
      );
    });
  });
}
