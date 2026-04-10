// KAI-303 — PushChannel abstraction tests.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/notifications/push_channel.dart';
import 'package:nvr_client/notifications/push_message.dart';

import 'fakes.dart';

void main() {
  group('PushChannel (fake)', () {
    test('round-trips an opaque PushMessage through the broadcast stream',
        () async {
      final channel = FakePushChannel();
      await channel.start();
      expect(channel.started, isTrue);

      final received = <PushMessage>[];
      final sub = channel.incoming.listen(received.add);

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
      await channel.stop();
      expect(channel.stopped, isTrue);
    });

    test('getDeviceToken returns configured value', () async {
      final channel = FakePushChannel(token: 'abc123');
      expect(await channel.getDeviceToken(), 'abc123');
    });
  });

  group('decodeRemoteAndForward', () {
    test('forwards a valid opaque payload', () {
      final out = <PushMessage>[];
      decodeRemoteAndForward(
        {'event_id': 'e', 'tenant_id': 't', 'priority': 2},
        out.add,
      );
      expect(out, hasLength(1));
      expect(out.first.eventId, 'e');
    });

    test('drops a payload with disallowed keys, no throw', () {
      final out = <PushMessage>[];
      decodeRemoteAndForward(
        {
          'event_id': 'e',
          'tenant_id': 't',
          'priority': 2,
          'camera_id': 'leak',
        },
        out.add,
      );
      expect(out, isEmpty);
    });

    test('drops a malformed payload without crashing', () {
      final out = <PushMessage>[];
      decodeRemoteAndForward(
        {'event_id': 'e'},
        out.add,
      );
      expect(out, isEmpty);
    });
  });

  group('ApnsPushChannel.debugDeliverFromRemote', () {
    test('delivers an opaque payload', () async {
      final channel = ApnsPushChannel();
      await channel.start();
      final received = <PushMessage>[];
      final sub = channel.incoming.listen(received.add);

      channel.debugDeliverFromRemote(
        {'event_id': 'e', 'tenant_id': 't', 'priority': 3},
      );
      await Future<void>.delayed(Duration.zero);

      expect(received, hasLength(1));
      expect(received.first.priority, 3);

      await sub.cancel();
      await channel.stop();
    });

    test('log + drop on payload violation (no crash)', () async {
      final channel = FcmPushChannel();
      await channel.start();
      final received = <PushMessage>[];
      final sub = channel.incoming.listen(received.add);

      channel.debugDeliverFromRemote({
        'event_id': 'e',
        'tenant_id': 't',
        'priority': 1,
        'thumbnail_url': 'https://cdn.example.com/leak.jpg',
      });
      await Future<void>.delayed(Duration.zero);

      expect(received, isEmpty,
          reason: 'violation must be dropped, not forwarded');

      await sub.cancel();
      await channel.stop();
    });
  });
}
