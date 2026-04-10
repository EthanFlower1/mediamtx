// Integration test: Live View -> Events flow.
//
// Simulates a user who is viewing a live stream and receives AI events.
// Verifies that the events list updates with new events filtered to the
// camera being viewed, and that severity filtering works correctly in the
// context of an active live view session.

import 'package:flutter_test/flutter_test.dart';

import 'package:nvr_client/api/streams_api.dart';
import 'package:nvr_client/events/events_client.dart';
import 'package:nvr_client/events/events_model.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';
import 'package:nvr_client/state/secure_token_store.dart';

/// Minimal fake StreamsApi for this test.
class _StubStreamsApi implements StreamsApi {
  @override
  Future<StreamRequest> requestStream({
    required String cameraId,
    required String baseUrl,
    required String accessToken,
    StreamKind kind = StreamKind.live,
    StreamProtocol protocol = StreamProtocol.auto,
    StreamVariant variant = StreamVariant.auto,
  }) async {
    return StreamRequest(
      streamId: 'stream-$cameraId',
      expiresAt: DateTime.now().toUtc().add(const Duration(hours: 1)),
      endpoints: const [
        StreamEndpoint(
          url: 'http://nvr.local:8889/cam-1/whep',
          transport: StreamTransport.webrtc,
          connectionType: StreamConnectionType.lanDirect,
          priority: 0,
        ),
      ],
    );
  }
}

void main() {
  group('Live View -> Events integration', () {
    late AppSessionNotifier sessionNotifier;
    late _StubStreamsApi streamsApi;

    final connection = HomeDirectoryConnection(
      id: 'conn-1',
      kind: HomeConnectionKind.onPrem,
      endpointUrl: 'https://nvr.acme.local',
      displayName: 'Acme HQ',
      discoveryMethod: DiscoveryMethod.mdns,
    );

    final now = DateTime.now().toUtc();

    setUp(() async {
      final tokenStore = InMemorySecureTokenStore();
      sessionNotifier = AppSessionNotifier(tokenStore);
      streamsApi = _StubStreamsApi();

      await sessionNotifier.activateConnection(
        connection: connection,
        userId: 'user-1',
        tenantRef: 'tenant-acme',
      );
      await sessionNotifier.setTokens(
        accessToken: 'eyJhbGciOiJSUzI1NiJ9.eyJzdWIiOiJ1c2VyLTEifQ.sig',
        refreshToken: 'refresh-tok',
      );
    });

    test('while viewing live stream, events list shows camera-specific events',
        () async {
      // Step 1: start viewing cam-1
      final streamResult = await streamsApi.requestStream(
        cameraId: 'cam-1',
        baseUrl: connection.endpointUrl,
        accessToken: sessionNotifier.state.accessToken!,
      );
      expect(streamResult.streamId, 'stream-cam-1');

      // Step 2: events arrive including events from multiple cameras
      final allEvents = [
        EventSummary(
          id: 'evt-1',
          timestamp: now.subtract(const Duration(minutes: 5)),
          cameraId: 'cam-1',
          cameraName: 'Lobby',
          kind: 'person_detected',
          severity: EventSeverity.warning,
          tenantId: 'tenant-acme',
        ),
        EventSummary(
          id: 'evt-2',
          timestamp: now.subtract(const Duration(minutes: 3)),
          cameraId: 'cam-2',
          cameraName: 'Parking',
          kind: 'motion',
          severity: EventSeverity.info,
          tenantId: 'tenant-acme',
        ),
        EventSummary(
          id: 'evt-3',
          timestamp: now.subtract(const Duration(minutes: 1)),
          cameraId: 'cam-1',
          cameraName: 'Lobby',
          kind: 'face',
          severity: EventSeverity.critical,
          tenantId: 'tenant-acme',
        ),
      ];

      final eventsClient = FakeEventsClient(allEvents);

      // Step 3: query events filtered to the camera being viewed
      final page = await eventsClient.list(
        tenantId: 'tenant-acme',
        filter: const EventFilter(cameraIds: {'cam-1'}),
      );

      // Should only get cam-1 events, in descending timestamp order
      expect(page.items, hasLength(2));
      expect(page.items[0].id, 'evt-3'); // most recent first
      expect(page.items[1].id, 'evt-1');
      expect(page.items.every((e) => e.cameraId == 'cam-1'), isTrue);
    });

    test('events filtered by severity during live view', () async {
      final events = [
        EventSummary(
          id: 'evt-1',
          timestamp: now.subtract(const Duration(minutes: 10)),
          cameraId: 'cam-1',
          cameraName: 'Lobby',
          kind: 'motion',
          severity: EventSeverity.info,
          tenantId: 'tenant-acme',
        ),
        EventSummary(
          id: 'evt-2',
          timestamp: now.subtract(const Duration(minutes: 5)),
          cameraId: 'cam-1',
          cameraName: 'Lobby',
          kind: 'person_detected',
          severity: EventSeverity.warning,
          tenantId: 'tenant-acme',
        ),
        EventSummary(
          id: 'evt-3',
          timestamp: now.subtract(const Duration(minutes: 1)),
          cameraId: 'cam-1',
          cameraName: 'Lobby',
          kind: 'tamper',
          severity: EventSeverity.critical,
          tenantId: 'tenant-acme',
        ),
      ];

      final eventsClient = FakeEventsClient(events);

      // Filter to critical only
      final page = await eventsClient.list(
        tenantId: 'tenant-acme',
        filter: const EventFilter(
          cameraIds: {'cam-1'},
          severities: {EventSeverity.critical},
        ),
      );

      expect(page.items, hasLength(1));
      expect(page.items[0].kind, 'tamper');
      expect(page.items[0].severity, EventSeverity.critical);
    });

    test('cross-tenant events are returned by fake but should be filtered by UI',
        () async {
      final events = [
        EventSummary(
          id: 'evt-own',
          timestamp: now.subtract(const Duration(minutes: 1)),
          cameraId: 'cam-1',
          cameraName: 'Lobby',
          kind: 'motion',
          severity: EventSeverity.info,
          tenantId: 'tenant-acme',
        ),
        EventSummary(
          id: 'evt-foreign',
          timestamp: now.subtract(const Duration(minutes: 2)),
          cameraId: 'cam-1',
          cameraName: 'Lobby',
          kind: 'motion',
          severity: EventSeverity.info,
          tenantId: 'tenant-other',
        ),
      ];

      final eventsClient = FakeEventsClient(events);
      final page = await eventsClient.list(
        tenantId: 'tenant-acme',
        filter: const EventFilter(),
      );

      // FakeEventsClient returns all events; the UI layer is responsible for
      // the cross-tenant guard. Verify both are present so the UI guard has
      // something to filter.
      expect(page.items, hasLength(2));

      // Simulate UI-side cross-tenant filter
      final session = sessionNotifier.state;
      final safeEvents = page.items
          .where((e) => e.tenantId == session.tenantRef)
          .toList();
      expect(safeEvents, hasLength(1));
      expect(safeEvents.first.id, 'evt-own');
    });

    test('pagination works during active live view', () async {
      // Create 5 events, paginate with limit=2
      final events = List.generate(
        5,
        (i) => EventSummary(
          id: 'evt-$i',
          timestamp: now.subtract(Duration(minutes: i)),
          cameraId: 'cam-1',
          cameraName: 'Lobby',
          kind: 'motion',
          severity: EventSeverity.info,
          tenantId: 'tenant-acme',
        ),
      );

      final eventsClient = FakeEventsClient(events);

      // Page 1
      final page1 = await eventsClient.list(
        tenantId: 'tenant-acme',
        filter: const EventFilter(),
        limit: 2,
      );
      expect(page1.items, hasLength(2));
      expect(page1.hasMore, isTrue);

      // Page 2
      final page2 = await eventsClient.list(
        tenantId: 'tenant-acme',
        filter: const EventFilter(),
        cursor: page1.nextCursor,
        limit: 2,
      );
      expect(page2.items, hasLength(2));
      expect(page2.hasMore, isTrue);

      // Page 3 (last)
      final page3 = await eventsClient.list(
        tenantId: 'tenant-acme',
        filter: const EventFilter(),
        cursor: page2.nextCursor,
        limit: 2,
      );
      expect(page3.items, hasLength(1));
      expect(page3.hasMore, isFalse);
    });
  });
}
