// KAI-312 — Unit tests for EventSummary, EventsPage, EventFilter, and
// FakeEventsClient. Pure Dart (no Flutter SDK dependency).

import 'package:flutter_test/flutter_test.dart';

import 'package:nvr_client/events/events_client.dart';
import 'package:nvr_client/events/events_model.dart';

void main() {
  group('EventSummary', () {
    test('round-trips through JSON', () {
      final ev = EventSummary(
        id: 'evt-1',
        timestamp: DateTime.utc(2026, 4, 8, 10, 30),
        cameraId: 'cam-1',
        cameraName: 'Lobby',
        kind: 'motion',
        severity: EventSeverity.warning,
        tenantId: 'tenant-a',
      );
      final json = ev.toJson();
      final restored = EventSummary.fromJson(json);
      expect(restored, equals(ev));
    });

    test('equality and hashCode', () {
      final a = EventSummary(
        id: 'x',
        timestamp: DateTime.utc(2026, 1, 1),
        cameraId: 'c',
        cameraName: 'C',
        kind: 'k',
        severity: EventSeverity.info,
        tenantId: 't',
      );
      final b = EventSummary(
        id: 'x',
        timestamp: DateTime.utc(2026, 1, 1),
        cameraId: 'c',
        cameraName: 'C',
        kind: 'k',
        severity: EventSeverity.info,
        tenantId: 't',
      );
      expect(a, equals(b));
      expect(a.hashCode, equals(b.hashCode));
    });
  });

  group('EventFilter', () {
    test('isEmpty for default', () {
      // Default EventFilter has no severities, no cameras, last7d, no
      // custom range — that is the "no active filter" state.
      expect(const EventFilter().isEmpty, isTrue);
    });

    test('isEmpty false when filters are active', () {
      expect(
        const EventFilter(severities: {EventSeverity.critical}).isEmpty,
        isFalse,
      );
      expect(
        const EventFilter(timeRange: EventTimeRange.today).isEmpty,
        isFalse,
      );
    });

    test('copyWith preserves other fields', () {
      const f = EventFilter(
        severities: {EventSeverity.critical},
        cameraIds: {'cam-1'},
        timeRange: EventTimeRange.today,
      );
      final f2 = f.copyWith(severities: {EventSeverity.info});
      expect(f2.cameraIds, equals({'cam-1'}));
      expect(f2.timeRange, equals(EventTimeRange.today));
      expect(f2.severities, equals({EventSeverity.info}));
    });
  });

  group('EventSeverity wire format', () {
    test('round-trips all values', () {
      for (final s in EventSeverity.values) {
        expect(eventSeverityFromWire(eventSeverityToWire(s)), equals(s));
      }
    });

    test('unknown value throws', () {
      expect(
        () => eventSeverityFromWire('unknown'),
        throwsA(isA<ArgumentError>()),
      );
    });
  });

  group('FakeEventsClient', () {
    final fixture = [
      EventSummary(
        id: 'e1',
        timestamp: DateTime.utc(2026, 4, 8, 12, 0),
        cameraId: 'cam-1',
        cameraName: 'A',
        kind: 'motion',
        severity: EventSeverity.info,
        tenantId: 'tenant',
      ),
      EventSummary(
        id: 'e2',
        timestamp: DateTime.utc(2026, 4, 8, 11, 0),
        cameraId: 'cam-2',
        cameraName: 'B',
        kind: 'alert',
        severity: EventSeverity.critical,
        tenantId: 'tenant',
      ),
      EventSummary(
        id: 'e3',
        timestamp: DateTime.utc(2026, 4, 1, 10, 0),
        cameraId: 'cam-1',
        cameraName: 'A',
        kind: 'motion',
        severity: EventSeverity.warning,
        tenantId: 'tenant',
      ),
    ];

    test('returns all items in one page when under limit', () async {
      final client = FakeEventsClient(fixture);
      final page = await client.list(
        tenantId: 'tenant',
        filter: const EventFilter(timeRange: EventTimeRange.last30d),
      );
      expect(page.items.length, equals(3));
      expect(page.nextCursor, isNull);
      // Descending timestamp order.
      expect(page.items[0].id, equals('e1'));
      expect(page.items[1].id, equals('e2'));
    });

    test('paginates with cursor', () async {
      final client = FakeEventsClient(fixture);
      final p1 = await client.list(
        tenantId: 'tenant',
        filter: const EventFilter(timeRange: EventTimeRange.last30d),
        limit: 2,
      );
      expect(p1.items.length, equals(2));
      expect(p1.hasMore, isTrue);

      final p2 = await client.list(
        tenantId: 'tenant',
        filter: const EventFilter(timeRange: EventTimeRange.last30d),
        cursor: p1.nextCursor,
        limit: 2,
      );
      expect(p2.items.length, equals(1));
      expect(p2.hasMore, isFalse);
    });

    test('filters by severity', () async {
      final client = FakeEventsClient(fixture);
      final page = await client.list(
        tenantId: 'tenant',
        filter: const EventFilter(
          severities: {EventSeverity.critical},
          timeRange: EventTimeRange.last30d,
        ),
      );
      expect(page.items.length, equals(1));
      expect(page.items[0].id, equals('e2'));
    });

    test('filters by camera', () async {
      final client = FakeEventsClient(fixture);
      final page = await client.list(
        tenantId: 'tenant',
        filter: const EventFilter(
          cameraIds: {'cam-2'},
          timeRange: EventTimeRange.last30d,
        ),
      );
      expect(page.items.length, equals(1));
      expect(page.items[0].cameraId, equals('cam-2'));
    });
  });

  group('CrossTenantEventViolation', () {
    test('formats correctly', () {
      const v = CrossTenantEventViolation(
        expectedTenantId: 'a',
        actualTenantId: 'b',
        eventId: 'evt-1',
      );
      expect(v.toString(), contains('expected=a'));
      expect(v.toString(), contains('actual=b'));
      expect(v.toString(), contains('event=evt-1'));
    });
  });
}
