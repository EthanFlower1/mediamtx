// KAI-303 — NotificationService tests.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/notifications/event_details_loader.dart';
import 'package:nvr_client/notifications/notification_service.dart';
import 'package:nvr_client/notifications/push_event_kind.dart';
import 'package:nvr_client/notifications/push_message.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';

import 'fakes.dart';

AppSession _sessionWithTenant(String tenant) {
  return AppSession(
    userId: 'user-1',
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
}

NotificationService _buildService({
  FakePushChannel? channel,
  FakeEventDetailsLoader? loader,
  AppSession Function()? readSession,
}) {
  return NotificationService(
    channel: channel ?? FakePushChannel(token: 'tok-A'),
    subscriptionClient: FakePushSubscriptionClient(),
    eventDetailsLoader: loader ?? FakeEventDetailsLoader(),
    readAppSession: readSession ?? () => _sessionWithTenant('tenant-acme'),
  );
}

void main() {
  group('NotificationService', () {
    test('start() registers device token against active directory', () async {
      final channel = FakePushChannel(token: 'tok-A');
      final svc = _buildService(channel: channel);

      await svc.start(directoryConnectionId: 'home-A');

      expect(svc.state.deviceToken, 'tok-A');
      expect(svc.state.subscriptionId, isNotNull);
      expect(svc.state.lastError, isNull);
    });

    test('incoming stream fans out OPAQUE PushMessages unchanged', () async {
      final channel = FakePushChannel(token: 'tok');
      final svc = _buildService(channel: channel);
      await svc.start(directoryConnectionId: 'home');

      final received = <PushMessage>[];
      final sub = svc.incoming.listen(received.add);

      const msg = PushMessage(
        eventId: 'evt-1',
        tenantId: 'tenant-acme',
        priority: 1,
      );
      channel.deliver(msg);
      await Future<void>.delayed(Duration.zero);

      expect(received, hasLength(1));
      expect(received.first.eventId, 'evt-1');
      expect(received.first.tenantId, 'tenant-acme');
      expect(received.first.priority, 1);
      await sub.cancel();
    });

    test('resolveForTap calls loader with the push eventId', () async {
      final loader = FakeEventDetailsLoader(events: {
        'evt-42': EventDetails(
          eventId: 'evt-42',
          cameraId: 'cam-front',
          cameraLabel: 'Front Door',
          kind: 'motion',
          timestamp: DateTime.utc(2026, 4, 8),
        ),
      });
      final svc = _buildService(loader: loader);

      const msg = PushMessage(
        eventId: 'evt-42',
        tenantId: 'tenant-acme',
        priority: 1,
      );
      final details = await svc.resolveForTap(msg);

      expect(loader.loadedEventIds, ['evt-42']);
      expect(details.cameraLabel, 'Front Door');
      expect(details.kind, 'motion');
    });

    test('resolveForTap throws CrossTenantPushViolation on tenant mismatch',
        () async {
      final loader = FakeEventDetailsLoader(events: {
        'evt-42': EventDetails(
          eventId: 'evt-42',
          cameraId: 'c',
          cameraLabel: '',
          kind: 'motion',
          timestamp: DateTime.utc(2026, 4, 8),
        ),
      });
      final svc = _buildService(
        loader: loader,
        readSession: () => _sessionWithTenant('tenant-acme'),
      );

      const msg = PushMessage(
        eventId: 'evt-42',
        tenantId: 'tenant-other',
        priority: 1,
      );

      await expectLater(
        () => svc.resolveForTap(msg),
        throwsA(isA<CrossTenantPushViolation>()),
      );
      expect(loader.loadedEventIds, isEmpty,
          reason: 'loader must not be called on tenant mismatch');
    });

    test('resolveForTap throws when AppSession has no active tenant',
        () async {
      final svc = _buildService(
        readSession: () => AppSession.empty,
      );
      const msg = PushMessage(
        eventId: 'evt-42',
        tenantId: 'tenant-acme',
        priority: 1,
      );
      await expectLater(
        () => svc.resolveForTap(msg),
        throwsA(isA<CrossTenantPushViolation>()),
      );
    });

    test('formatForegroundNotification uses i18n titleForKind', () {
      final svc = _buildService();
      final details = EventDetails(
        eventId: 'evt',
        cameraId: 'cam',
        cameraLabel: 'Back Yard',
        kind: 'face',
        timestamp: DateTime.utc(2026, 4, 8),
      );
      final fg = svc.formatForegroundNotification(details);
      expect(fg.title, 'Face detected');
      expect(fg.body, 'Back Yard');
    });

    test('subscribeCamera round-trip uses PushEventKind', () async {
      final svc = _buildService();
      await svc.start(directoryConnectionId: 'home');
      await svc.subscribeCamera(
        cameraId: 'cam-1',
        eventKinds: {PushEventKind.motion, PushEventKind.face},
      );
      final subs = await svc.listSubscriptions();
      expect(subs, hasLength(1));
      expect(subs.first.eventKinds,
          {PushEventKind.motion, PushEventKind.face});
    });

    test('firebaseNotInitialisedWarner fires when not initialised',
        () async {
      final warnings = <String>[];
      final svc = NotificationService(
        channel: FakePushChannel(token: 'tok'),
        subscriptionClient: FakePushSubscriptionClient(),
        eventDetailsLoader: FakeEventDetailsLoader(),
        readAppSession: () => _sessionWithTenant('tenant-acme'),
        firebaseInitialised: false,
        firebaseNotInitialisedWarner: warnings.add,
      );
      await svc.start(directoryConnectionId: 'home');
      expect(warnings, isNotEmpty);
    });
  });
}
