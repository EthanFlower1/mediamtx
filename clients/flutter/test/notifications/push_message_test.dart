// KAI-303 — PushMessage metadata-only contract tests.
//
// The hard contract (cto + lead-security gate on PR #165) says a push
// payload MUST carry ONLY `event_id` + `tenant_id` + `priority`. Every
// other key is a contract violation and must throw
// [PushPayloadViolation].

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/notifications/push_message.dart';

void main() {
  group('PushMessage.fromRemote — metadata-only contract', () {
    test('accepts a valid 3-field payload', () {
      final msg = PushMessage.fromRemote({
        'event_id': 'evt-42',
        'tenant_id': 'tenant-acme',
        'priority': 2,
      });
      expect(msg.eventId, 'evt-42');
      expect(msg.tenantId, 'tenant-acme');
      expect(msg.priority, 2);
    });

    test('round-trip via toRemote preserves the 3 allowed fields', () {
      const original = PushMessage(
        eventId: 'evt-42',
        tenantId: 'tenant-acme',
        priority: 3,
      );
      final roundTripped = PushMessage.fromRemote(original.toRemote());
      expect(roundTripped.eventId, original.eventId);
      expect(roundTripped.tenantId, original.tenantId);
      expect(roundTripped.priority, original.priority);
    });

    test('accepts numeric priority expressed as double', () {
      final msg = PushMessage.fromRemote({
        'event_id': 'e',
        'tenant_id': 't',
        'priority': 1.0,
      });
      expect(msg.priority, 1);
    });

    // --- disallowed keys: one test per blocked field ------------------

    test('rejects camera_id', () {
      expect(
        () => PushMessage.fromRemote({
          'event_id': 'e',
          'tenant_id': 't',
          'priority': 1,
          'camera_id': 'cam-1',
        }),
        throwsA(isA<PushPayloadViolation>()),
      );
    });

    test('rejects thumbnail_url', () {
      expect(
        () => PushMessage.fromRemote({
          'event_id': 'e',
          'tenant_id': 't',
          'priority': 1,
          'thumbnail_url': 'https://cdn.example.com/evt.jpg',
        }),
        throwsA(isA<PushPayloadViolation>()),
      );
    });

    test('rejects timestamp', () {
      expect(
        () => PushMessage.fromRemote({
          'event_id': 'e',
          'tenant_id': 't',
          'priority': 1,
          'timestamp': '2026-04-08T12:00:00Z',
        }),
        throwsA(isA<PushPayloadViolation>()),
      );
    });

    test('rejects kind', () {
      expect(
        () => PushMessage.fromRemote({
          'event_id': 'e',
          'tenant_id': 't',
          'priority': 1,
          'kind': 'motion',
        }),
        throwsA(isA<PushPayloadViolation>()),
      );
    });

    test('rejects arbitrary_field', () {
      expect(
        () => PushMessage.fromRemote({
          'event_id': 'e',
          'tenant_id': 't',
          'priority': 1,
          'arbitrary_field': 'whatever',
        }),
        throwsA(isA<PushPayloadViolation>()),
      );
    });

    test('rejects directoryConnectionId', () {
      expect(
        () => PushMessage.fromRemote({
          'event_id': 'e',
          'tenant_id': 't',
          'priority': 1,
          'directoryConnectionId': 'home-a',
        }),
        throwsA(isA<PushPayloadViolation>()),
      );
    });

    test('rejects label', () {
      expect(
        () => PushMessage.fromRemote({
          'event_id': 'e',
          'tenant_id': 't',
          'priority': 1,
          'label': 'Front Door',
        }),
        throwsA(isA<PushPayloadViolation>()),
      );
    });

    // --- malformed required fields ------------------------------------

    test('rejects missing event_id', () {
      expect(
        () => PushMessage.fromRemote({
          'tenant_id': 't',
          'priority': 1,
        }),
        throwsA(isA<PushPayloadViolation>()),
      );
    });

    test('rejects missing tenant_id', () {
      expect(
        () => PushMessage.fromRemote({
          'event_id': 'e',
          'priority': 1,
        }),
        throwsA(isA<PushPayloadViolation>()),
      );
    });

    test('rejects missing priority', () {
      expect(
        () => PushMessage.fromRemote({
          'event_id': 'e',
          'tenant_id': 't',
        }),
        throwsA(isA<PushPayloadViolation>()),
      );
    });

    test('rejects empty event_id', () {
      expect(
        () => PushMessage.fromRemote({
          'event_id': '',
          'tenant_id': 't',
          'priority': 1,
        }),
        throwsA(isA<PushPayloadViolation>()),
      );
    });

    test('rejects non-numeric priority', () {
      expect(
        () => PushMessage.fromRemote({
          'event_id': 'e',
          'tenant_id': 't',
          'priority': 'high',
        }),
        throwsA(isA<PushPayloadViolation>()),
      );
    });
  });
}
