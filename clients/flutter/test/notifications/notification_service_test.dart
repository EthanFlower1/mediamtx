// KAI-303 — NotificationService tests.

import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/notifications/notification_service.dart';
import 'package:nvr_client/notifications/push_message.dart';

import 'fakes.dart';

void main() {
  group('NotificationService', () {
    test('start() registers device token against active directory', () async {
      final channel = FakePushChannel(token: 'tok-A');
      final client = FakePushSubscriptionClient();
      final svc = NotificationService(channel: channel, subscriptionClient: client);

      await svc.start(directoryConnectionId: 'home-A');

      expect(client.registerCalls, 1);
      expect(client.lastRegisteredDirectory, 'home-A');
      expect(client.lastRegisteredToken, 'tok-A');
      expect(client.lastRegisteredPlatform, 'fake_test');
      expect(svc.state.subscriptionId, isNotNull);
      expect(svc.state.lastError, isNull);
    });

    test('re-registers on SessionSwitchedEvent', () async {
      final channel = FakePushChannel(token: 'tok-B');
      final client = FakePushSubscriptionClient();
      final svc = NotificationService(channel: channel, subscriptionClient: client);

      await svc.start(directoryConnectionId: 'home-A');
      final firstSubId = svc.state.subscriptionId;

      await svc.onSessionSwitched(const SessionSwitchedEvent(
        fromConnectionId: 'home-A',
        toConnectionId: 'home-B',
      ));

      expect(client.registerCalls, 2);
      expect(client.lastRegisteredDirectory, 'home-B');
      expect(svc.state.subscriptionId, isNotNull);
      expect(svc.state.subscriptionId, isNot(equals(firstSubId)));
    });

    test('subscribe/unsubscribe round-trip', () async {
      final svc = NotificationService(
        channel: FakePushChannel(token: 'tok'),
        subscriptionClient: FakePushSubscriptionClient(),
      );
      await svc.start(directoryConnectionId: 'home');

      await svc.subscribeCamera(
        cameraId: 'cam-1',
        eventKinds: {PushMessageKind.motion, PushMessageKind.face},
      );
      var subs = await svc.listSubscriptions();
      expect(subs, hasLength(1));
      expect(subs.first.cameraId, 'cam-1');
      expect(subs.first.eventKinds, {PushMessageKind.motion, PushMessageKind.face});

      await svc.unsubscribeCamera(cameraId: 'cam-1');
      subs = await svc.listSubscriptions();
      expect(subs, isEmpty);
    });

    test('subscribeCamera throws before register', () async {
      final svc = NotificationService(
        channel: FakePushChannel(),
        subscriptionClient: FakePushSubscriptionClient(),
      );
      expect(
        () => svc.subscribeCamera(cameraId: 'cam', eventKinds: {PushMessageKind.motion}),
        throwsStateError,
      );
    });

    test('incoming stream forwards messages to UI stream', () async {
      final channel = FakePushChannel(token: 'tok');
      final svc = NotificationService(
        channel: channel,
        subscriptionClient: FakePushSubscriptionClient(),
      );
      await svc.start(directoryConnectionId: 'home');

      final received = <PushMessage>[];
      final sub = svc.incoming.listen(received.add);

      channel.deliver(PushMessage(
        eventId: 'e1',
        cameraId: 'c1',
        kind: PushMessageKind.motion,
        timestamp: DateTime.utc(2026, 4, 8),
        directoryConnectionId: 'home',
      ));
      await Future<void>.delayed(Duration.zero);

      expect(received, hasLength(1));
      await sub.cancel();
    });

    test('metadata-only: data: URIs rejected at construction', () {
      expect(
        () => PushMessage(
          eventId: 'e',
          cameraId: 'c',
          kind: PushMessageKind.motion,
          timestamp: DateTime.utc(2026, 4, 8),
          directoryConnectionId: 'home',
          thumbnailUrl: 'data:image/jpeg;base64,AAAA',
        ),
        throwsArgumentError,
      );
    });

    test('metadata-only: embedded binary rejected via rejectBinary factory', () {
      expect(
        () => PushMessage.rejectBinary(
          eventId: 'e',
          cameraId: 'c',
          kind: PushMessageKind.motion,
          timestamp: DateTime.utc(2026, 4, 8),
          directoryConnectionId: 'home',
          embeddedImage: Uint8List.fromList([1, 2, 3]),
        ),
        throwsArgumentError,
      );
    });

    test('firebaseNotInitialisedWarner fires when firebaseInitialised is false',
        () async {
      final warnings = <String>[];
      final svc = NotificationService(
        channel: FakePushChannel(token: 'tok'),
        subscriptionClient: FakePushSubscriptionClient(),
        firebaseInitialised: false,
        firebaseNotInitialisedWarner: warnings.add,
      );
      await svc.start(directoryConnectionId: 'home');
      expect(warnings, isNotEmpty);
    });
  });
}
