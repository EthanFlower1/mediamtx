import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/providers/connectivity_provider.dart';
import 'package:nvr_client/providers/pending_actions_provider.dart';
import 'package:nvr_client/services/pending_actions_service.dart';
import 'package:nvr_client/theme/nvr_theme.dart';
import 'package:nvr_client/widgets/connection_status_banner.dart';

void main() {
  Widget wrapWithState({
    required ConnectivityStatus status,
    int pendingCount = 0,
  }) {
    return ProviderScope(
      overrides: [
        connectivityProvider.overrideWith((ref) {
          return _FakeConnectivityNotifier(ConnectivityState(status: status));
        }),
        pendingActionsProvider.overrideWith((ref) {
          return _FakePendingActionsNotifier(
            PendingActionsState(pendingCount: pendingCount),
          );
        }),
      ],
      child: MaterialApp(
        theme: NvrTheme.light(),
        darkTheme: NvrTheme.dark(),
        themeMode: ThemeMode.dark,
        home: Scaffold(body: const ConnectionStatusBanner()),
      ),
    );
  }

  group('ConnectionStatusBanner', () {
    testWidgets('renders nothing (SizedBox.shrink) when online', (tester) async {
      await tester.pumpWidget(wrapWithState(
        status: ConnectivityStatus.online,
      ));

      // Should render SizedBox.shrink, no text visible.
      expect(find.text('Offline - showing cached data'), findsNothing);
      expect(find.text('Reconnecting to server...'), findsNothing);
    });

    testWidgets('shows offline text when offline', (tester) async {
      await tester.pumpWidget(wrapWithState(
        status: ConnectivityStatus.offline,
      ));

      expect(find.text('Offline - showing cached data'), findsOneWidget);
      expect(find.byIcon(Icons.cloud_off), findsOneWidget);
    });

    testWidgets('shows reconnecting text and spinner when reconnecting', (tester) async {
      await tester.pumpWidget(wrapWithState(
        status: ConnectivityStatus.reconnecting,
      ));

      expect(find.text('Reconnecting to server...'), findsOneWidget);
      expect(find.byIcon(Icons.sync), findsOneWidget);
      expect(find.byType(CircularProgressIndicator), findsOneWidget);
    });

    testWidgets('shows pending count badge when there are pending actions', (tester) async {
      await tester.pumpWidget(wrapWithState(
        status: ConnectivityStatus.offline,
        pendingCount: 3,
      ));

      expect(find.text('3 pending'), findsOneWidget);
    });

    testWidgets('hides pending count badge when count is zero', (tester) async {
      await tester.pumpWidget(wrapWithState(
        status: ConnectivityStatus.offline,
        pendingCount: 0,
      ));

      expect(find.text('0 pending'), findsNothing);
    });
  });
}

/// Fake ConnectivityNotifier that emits a fixed state.
class _FakeConnectivityNotifier extends ConnectivityNotifier {
  _FakeConnectivityNotifier(ConnectivityState initialState)
      : super.noServer() {
    state = initialState;
  }
}

/// Fake PendingActionsNotifier that emits a fixed state.
class _FakePendingActionsNotifier extends StateNotifier<PendingActionsState>
    implements PendingActionsNotifier {
  _FakePendingActionsNotifier(PendingActionsState initialState)
      : super(initialState);

  @override
  Future<void> enqueue(PendingAction action) async {}

  @override
  Future<void> flushQueue() async {}
}
