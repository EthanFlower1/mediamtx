// KAI-312 — Widget tests for the events list + filter + cross-tenant guard.
//
// These tests use [FakeEventsClient] with a canned fixture and verify:
//   1. List renders rows with correct severity, camera name, and kind.
//   2. Filter toggle changes rendered rows (severity filter).
//   3. Cross-tenant rows are silently dropped (defense-in-depth).
//   4. Detail page navigation on tap.
//   5. Empty state renders when no events match.
//   6. Detail page cross-tenant guard blocks access.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:nvr_client/events/events_client.dart';
import 'package:nvr_client/events/events_list_page.dart';
import 'package:nvr_client/events/events_model.dart';
import 'package:nvr_client/events/event_detail_page.dart';
import 'package:nvr_client/state/app_session.dart';
import 'package:nvr_client/state/home_directory_connection.dart';
import 'package:nvr_client/state/secure_token_store.dart';

const _tenantA = 'tenant-a';
const _tenantB = 'tenant-b';

final _now = DateTime.utc(2026, 4, 8, 12, 0);

List<EventSummary> _fixture() => [
      EventSummary(
        id: 'evt-1',
        timestamp: _now.subtract(const Duration(hours: 1)),
        cameraId: 'cam-1',
        cameraName: 'Front Door',
        kind: 'motion',
        severity: EventSeverity.info,
        tenantId: _tenantA,
      ),
      EventSummary(
        id: 'evt-2',
        timestamp: _now.subtract(const Duration(hours: 2)),
        cameraId: 'cam-2',
        cameraName: 'Parking Lot',
        kind: 'person_detected',
        severity: EventSeverity.warning,
        tenantId: _tenantA,
      ),
      EventSummary(
        id: 'evt-3',
        timestamp: _now.subtract(const Duration(hours: 3)),
        cameraId: 'cam-3',
        cameraName: 'Server Room',
        kind: 'tamper',
        severity: EventSeverity.critical,
        tenantId: _tenantA,
      ),
      // Cross-tenant row that MUST be dropped.
      EventSummary(
        id: 'evt-rogue',
        timestamp: _now.subtract(const Duration(hours: 4)),
        cameraId: 'cam-99',
        cameraName: 'Rogue Camera',
        kind: 'offline',
        severity: EventSeverity.warning,
        tenantId: _tenantB,
      ),
    ];

HomeDirectoryConnection _conn() => const HomeDirectoryConnection(
      id: 'conn-1',
      kind: HomeConnectionKind.cloud,
      endpointUrl: 'https://test.kaivue.dev',
      displayName: 'Test Cloud',
      discoveryMethod: DiscoveryMethod.manual,
    );

/// Creates an authenticated AppSessionNotifier for testing.
Future<AppSessionNotifier> _buildSession({
  required InMemorySecureTokenStore store,
  String tenantRef = _tenantA,
}) async {
  final notifier = AppSessionNotifier(store);
  await notifier.activateConnection(
    connection: _conn(),
    userId: 'user-1',
    tenantRef: tenantRef,
  );
  await notifier.setTokens(
    accessToken: 'fake-token',
    refreshToken: 'fake-refresh',
  );
  return notifier;
}

/// Wraps the widget under test with Riverpod overrides for AppSession,
/// EventsClient, and a MaterialApp shell. Must be called within testWidgets
/// so SharedPreferences is available.
Future<Widget> _harness({
  required List<EventSummary> fixture,
  String tenantRef = _tenantA,
  List<Override> extraOverrides = const [],
}) async {
  final store = InMemorySecureTokenStore();
  final notifier = await _buildSession(store: store, tenantRef: tenantRef);

  return ProviderScope(
    overrides: [
      secureTokenStoreProvider.overrideWithValue(store),
      appSessionProvider.overrideWith((_) => notifier),
      eventsClientProvider.overrideWithValue(FakeEventsClient(fixture)),
      ...extraOverrides,
    ],
    child: const MaterialApp(
      home: EventsListPage(),
    ),
  );
}

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  group('EventsListPage', () {
    testWidgets('renders event rows with severity, camera, kind',
        (tester) async {
      await tester.pumpWidget(await _harness(fixture: _fixture()));
      await tester.pumpAndSettle();

      // Three rows for tenant-a; the rogue row is dropped.
      expect(find.text('Front Door'), findsOneWidget);
      expect(find.text('Parking Lot'), findsOneWidget);
      expect(find.text('Server Room'), findsOneWidget);

      // Severity badges.
      expect(find.text('INFO'), findsOneWidget);
      expect(find.text('WARN'), findsOneWidget);
      expect(find.text('CRIT'), findsOneWidget);
    });

    testWidgets('cross-tenant rows are dropped silently', (tester) async {
      await tester.pumpWidget(await _harness(fixture: _fixture()));
      await tester.pumpAndSettle();

      // The rogue camera should NOT appear.
      expect(find.text('Rogue Camera'), findsNothing);
    });

    testWidgets('empty state shows when no events match', (tester) async {
      await tester.pumpWidget(await _harness(fixture: const []));
      await tester.pumpAndSettle();

      expect(find.text('No events match your filters.'), findsOneWidget);
    });

    testWidgets('tap on row navigates to detail page', (tester) async {
      await tester.pumpWidget(await _harness(fixture: _fixture()));
      await tester.pumpAndSettle();

      await tester.tap(find.text('Front Door'));
      await tester.pumpAndSettle();

      // Should be on the detail page now.
      expect(find.text('Event detail'), findsOneWidget);
      expect(find.text('Event ID: evt-1'), findsOneWidget);
    });

    testWidgets('filter sheet opens on icon tap', (tester) async {
      await tester.pumpWidget(await _harness(fixture: _fixture()));
      await tester.pumpAndSettle();

      await tester.tap(find.byKey(const ValueKey('events-open-filter')));
      await tester.pumpAndSettle();

      // Filter sheet should be visible.
      expect(find.text('Filter events'), findsOneWidget);
      expect(find.text('Severity'), findsOneWidget);
      expect(find.text('Time range'), findsOneWidget);
    });

    testWidgets('filter sheet apply filters rows by severity', (tester) async {
      await tester.pumpWidget(await _harness(fixture: _fixture()));
      await tester.pumpAndSettle();

      // Open filter.
      await tester.tap(find.byKey(const ValueKey('events-open-filter')));
      await tester.pumpAndSettle();

      // Select only critical severity.
      await tester
          .tap(find.byKey(const ValueKey('events-filter-sev-critical')));
      await tester.pumpAndSettle();

      // Apply.
      await tester.tap(find.byKey(const ValueKey('events-filter-apply')));
      await tester.pumpAndSettle();

      // Only the critical event should be visible.
      expect(find.text('Server Room'), findsOneWidget);
      expect(find.text('Front Door'), findsNothing);
      expect(find.text('Parking Lot'), findsNothing);
    });

    testWidgets('filter reset restores all events', (tester) async {
      await tester.pumpWidget(await _harness(fixture: _fixture()));
      await tester.pumpAndSettle();

      // Open filter and select only critical.
      await tester.tap(find.byKey(const ValueKey('events-open-filter')));
      await tester.pumpAndSettle();
      await tester
          .tap(find.byKey(const ValueKey('events-filter-sev-critical')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const ValueKey('events-filter-apply')));
      await tester.pumpAndSettle();
      expect(find.text('Front Door'), findsNothing);

      // Now open filter and reset.
      await tester.tap(find.byKey(const ValueKey('events-open-filter')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const ValueKey('events-filter-reset')));
      await tester.pumpAndSettle();
      await tester.tap(find.byKey(const ValueKey('events-filter-apply')));
      await tester.pumpAndSettle();

      // All tenant-a rows should be back.
      expect(find.text('Front Door'), findsOneWidget);
      expect(find.text('Parking Lot'), findsOneWidget);
      expect(find.text('Server Room'), findsOneWidget);
    });
  });

  group('EventDetailPage', () {
    testWidgets('shows stub when no loader is wired', (tester) async {
      final store = InMemorySecureTokenStore();
      final notifier = await _buildSession(store: store);

      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            secureTokenStoreProvider.overrideWithValue(store),
            appSessionProvider.overrideWith((_) => notifier),
          ],
          child: const MaterialApp(
            home: EventDetailPage(eventId: 'evt-1', tenantId: _tenantA),
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('Event detail'), findsOneWidget);
      expect(
        find.text('Event detail is not available on this build.'),
        findsOneWidget,
      );
      expect(find.text('Event ID: evt-1'), findsOneWidget);
    });

    testWidgets('blocks cross-tenant access', (tester) async {
      final store = InMemorySecureTokenStore();
      final notifier = await _buildSession(store: store);

      await tester.pumpWidget(
        ProviderScope(
          overrides: [
            secureTokenStoreProvider.overrideWithValue(store),
            appSessionProvider.overrideWith((_) => notifier),
          ],
          child: const MaterialApp(
            home: EventDetailPage(eventId: 'evt-rogue', tenantId: _tenantB),
          ),
        ),
      );
      await tester.pumpAndSettle();

      expect(find.text('Access denied.'), findsOneWidget);
    });
  });
}
