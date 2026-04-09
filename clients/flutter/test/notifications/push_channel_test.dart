// KAI-303 — PushChannel abstraction tests.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/notifications/push_message.dart';

import 'fakes.dart';

void main() {
  group('PushChannel (fake)', () {
    test('round-trips a PushMessage through the broadcast stream', () async {
      final channel = FakePushChannel();
      await channel.start();
      expect(channel.started, isTrue);

      final received = <PushMessage>[];
      final sub = channel.incoming.listen(received.add);

      final msg = PushMessage(
        eventId: 'evt-1',
        cameraId: 'cam-1',
        kind: PushMessageKind.motion,
        timestamp: DateTime.utc(2026, 4, 8, 12),
        directoryConnectionId: 'home-1',
      );
      channel.deliver(msg);
      await Future<void>.delayed(Duration.zero);

      expect(received, hasLength(1));
      expect(received.first.eventId, 'evt-1');
      expect(received.first.kind, PushMessageKind.motion);

      await sub.cancel();
      await channel.stop();
      expect(channel.stopped, isTrue);
    });

    test('getDeviceToken returns configured value', () async {
      final channel = FakePushChannel(token: 'abc123');
      expect(await channel.getDeviceToken(), 'abc123');
    });

    test('PushMessage.fromWire/toWire round-trip', () {
      final original = PushMessage(
        eventId: 'evt-2',
        cameraId: 'cam-2',
        kind: PushMessageKind.face,
        timestamp: DateTime.utc(2026, 4, 8, 13),
        directoryConnectionId: 'home-2',
        thumbnailUrl: 'https://cdn.example.com/t/evt-2.jpg',
      );
      final roundTripped = PushMessage.fromWire(original.toWire());
      expect(roundTripped.eventId, 'evt-2');
      expect(roundTripped.kind, PushMessageKind.face);
      expect(roundTripped.thumbnailUrl, 'https://cdn.example.com/t/evt-2.jpg');
      expect(roundTripped.timestamp.toUtc(), original.timestamp.toUtc());
    });
  });
}
