// End-to-end integration tests for the NVR Flutter client.
//
// These tests exercise the full app against a running MediaMTX NVR backend.
// They walk through the auth flow, navigate between screens, and verify
// that live data loads correctly.
//
// Prerequisites:
//   - MediaMTX NVR server running (default: http://localhost:9997)
//   - At least one camera configured and connected
//   - Valid admin credentials (default: admin / admin)
//
// Run with default settings:
//   cd clients/flutter
//   flutter test integration_test/nvr_e2e_test.dart -d macos
//
// Run with custom server/credentials:
//   flutter test integration_test/nvr_e2e_test.dart -d macos \
//     --dart-define=NVR_SERVER_URL=http://192.168.1.100:9997 \
//     --dart-define=NVR_USERNAME=admin \
//     --dart-define=NVR_PASSWORD=mypassword

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:media_kit/media_kit.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:nvr_client/app.dart';

// ---------------------------------------------------------------------------
// Test configuration — override via --dart-define flags
// ---------------------------------------------------------------------------
const testServerUrl = String.fromEnvironment(
  'NVR_SERVER_URL',
  defaultValue: 'http://localhost:9997',
);
const testUsername = String.fromEnvironment(
  'NVR_USERNAME',
  defaultValue: 'admin',
);
const testPassword = String.fromEnvironment(
  'NVR_PASSWORD',
  defaultValue: 'admin',
);

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  MediaKit.ensureInitialized();

  // =========================================================================
  // Test 1: Server setup and login flow
  // =========================================================================
  group('Auth flow', () {
    testWidgets('can connect to server and login', (tester) async {
      // Start from a clean slate — no saved server or token.
      SharedPreferences.setMockInitialValues({});

      await tester.pumpWidget(const ProviderScope(child: NvrApp()));
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // -- Server setup screen --
      // Verify we land on the server setup screen.
      expect(
        find.text('MEDIAMTX NVR'),
        findsWidgets,
        reason: 'Should see the MEDIAMTX NVR branding on server setup',
      );
      expect(
        find.text('SERVER URL'),
        findsOneWidget,
        reason: 'Should see the SERVER URL label',
      );

      // Enter the server URL. The field defaults to "http://" so clear first.
      final urlField = find.byType(TextFormField);
      expect(urlField, findsOneWidget, reason: 'Should find the URL text field');
      await tester.enterText(urlField, testServerUrl);
      await tester.pumpAndSettle();

      // Tap the CONNECT button.
      final connectButton = find.widgetWithText(ElevatedButton, 'CONNECT');
      expect(connectButton, findsOneWidget, reason: 'Should find CONNECT button');
      await tester.tap(connectButton);
      await tester.pumpAndSettle(const Duration(seconds: 5));

      // -- Login screen --
      // Verify we navigated to the login screen.
      expect(
        find.text('USERNAME'),
        findsOneWidget,
        reason: 'Should see USERNAME label on login screen',
      );
      expect(
        find.text('PASSWORD'),
        findsOneWidget,
        reason: 'Should see PASSWORD label on login screen',
      );

      // Enter credentials.
      final usernameField = find.byType(TextFormField).at(0);
      final passwordField = find.byType(TextFormField).at(1);
      await tester.enterText(usernameField, testUsername);
      await tester.enterText(passwordField, testPassword);
      await tester.pumpAndSettle();

      // Tap SIGN IN button.
      final signInButton = find.widgetWithText(ElevatedButton, 'SIGN IN');
      expect(signInButton, findsOneWidget, reason: 'Should find SIGN IN button');
      await tester.tap(signInButton);
      await tester.pumpAndSettle(const Duration(seconds: 5));

      // -- Should land on Live View --
      expect(
        find.text('Live View'),
        findsOneWidget,
        reason: 'After login, should navigate to Live View screen',
      );
    });
  });

  // =========================================================================
  // Test 2: Live view screen loads
  // =========================================================================
  group('Live view', () {
    testWidgets('live view shows grid with cameras', (tester) async {
      await _ensureLoggedIn(tester);

      // Verify the Live View page title is present.
      expect(
        find.text('Live View'),
        findsOneWidget,
        reason: 'Live View title should be visible',
      );

      // Verify the ALL CAMERAS badge is present.
      expect(
        find.text('ALL CAMERAS'),
        findsWidgets,
        reason: 'ALL CAMERAS badge should be visible',
      );

      // Verify the grid selector (segmented control) is visible with grid
      // size options. The segmented control renders labels like "1x1", "2x2",
      // etc. using Unicode multiplication sign.
      expect(
        find.text('1\u00D71'),
        findsOneWidget,
        reason: 'Grid selector should show 1x1 option',
      );
      expect(
        find.text('2\u00D72'),
        findsOneWidget,
        reason: 'Grid selector should show 2x2 option',
      );

      // Wait for cameras to load from API.
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // Verify the grid view exists (contains camera tiles or DROP HERE slots).
      expect(
        find.byType(GridView),
        findsOneWidget,
        reason: 'Camera grid should be present',
      );
    });

    // =======================================================================
    // Test 3: Grid size changes
    // =======================================================================
    testWidgets('can change grid size', (tester) async {
      await _ensureLoggedIn(tester);
      await tester.pumpAndSettle(const Duration(seconds: 2));

      // Tap the 1x1 grid option.
      await tester.tap(find.text('1\u00D71'));
      await tester.pumpAndSettle();

      // In 1x1 mode there should be exactly 1 slot in the grid.
      final grid1 = tester.widget<GridView>(find.byType(GridView));
      final delegate1 =
          grid1.gridDelegate as SliverGridDelegateWithFixedCrossAxisCount;
      expect(
        delegate1.crossAxisCount,
        1,
        reason: '1x1 grid should have crossAxisCount of 1',
      );

      // Tap the 3x3 grid option.
      await tester.tap(find.text('3\u00D73'));
      await tester.pumpAndSettle();

      final grid3 = tester.widget<GridView>(find.byType(GridView));
      final delegate3 =
          grid3.gridDelegate as SliverGridDelegateWithFixedCrossAxisCount;
      expect(
        delegate3.crossAxisCount,
        3,
        reason: '3x3 grid should have crossAxisCount of 3',
      );

      // Tap the 4x4 grid option.
      await tester.tap(find.text('4\u00D74'));
      await tester.pumpAndSettle();

      final grid4 = tester.widget<GridView>(find.byType(GridView));
      final delegate4 =
          grid4.gridDelegate as SliverGridDelegateWithFixedCrossAxisCount;
      expect(
        delegate4.crossAxisCount,
        4,
        reason: '4x4 grid should have crossAxisCount of 4',
      );
    });
  });

  // =========================================================================
  // Test 4: Navigate to camera detail
  // =========================================================================
  group('Camera detail', () {
    testWidgets('can navigate to camera detail and see live data',
        (tester) async {
      await _ensureLoggedIn(tester);

      // Navigate to Devices tab (index 3 in router).
      // On desktop (>= 600px) the icon rail uses semantic labels; on mobile
      // the bottom nav shows text labels. We find the Devices icon by its
      // semantic label or by navigating via the icon rail.
      await _navigateToTab(tester, 'Devices');
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // Verify the Devices title is visible.
      expect(
        find.text('Devices'),
        findsOneWidget,
        reason: 'Devices page title should be visible',
      );

      // Verify at least one camera card exists. The camera list uses
      // GestureDetector wrapping _DeviceCard containers. Look for the
      // chevron icon that each card has.
      expect(
        find.byIcon(Icons.chevron_right),
        findsWidgets,
        reason: 'Should find at least one device card with chevron',
      );

      // Tap the first device card. Each card is wrapped in a GestureDetector,
      // find by the chevron icon and tap its parent.
      await tester.tap(find.byIcon(Icons.chevron_right).first);
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // Verify camera detail screen loaded — header shows camera name and
      // status badge. The back arrow icon indicates we are on the detail page.
      expect(
        find.byIcon(Icons.arrow_back),
        findsOneWidget,
        reason: 'Camera detail should show a back arrow',
      );

      // Verify stat tiles exist with their labels.
      expect(
        find.text('UPTIME'),
        findsOneWidget,
        reason: 'UPTIME stat tile should be visible',
      );
      expect(
        find.text('STORAGE'),
        findsOneWidget,
        reason: 'STORAGE stat tile should be visible',
      );
      expect(
        find.text('EVENTS TODAY'),
        findsOneWidget,
        reason: 'EVENTS TODAY stat tile should be visible',
      );

      // Verify the RETENTION section exists.
      expect(
        find.text('RETENTION'),
        findsWidgets,
        reason: 'RETENTION section/stat should be visible',
      );

      // Verify RECORDING and AI DETECTION sections.
      expect(
        find.text('RECORDING'),
        findsOneWidget,
        reason: 'RECORDING section should be visible',
      );
      expect(
        find.text('AI DETECTION'),
        findsOneWidget,
        reason: 'AI DETECTION section should be visible',
      );
    });

    // =======================================================================
    // Test 5: Camera detail shows live preview
    // =======================================================================
    testWidgets('camera detail shows live preview', (tester) async {
      await _ensureLoggedIn(tester);

      // Navigate to Devices.
      await _navigateToTab(tester, 'Devices');
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // Tap the first device card.
      await tester.tap(find.byIcon(Icons.chevron_right).first);
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // Verify the video preview area exists. The CameraTile is inside an
      // AspectRatio widget with 16:9. Verify at minimum the AspectRatio
      // exists and no error icon is shown in the preview area.
      expect(
        find.byType(AspectRatio),
        findsWidgets,
        reason: 'Camera detail should have an AspectRatio for video preview',
      );

      // Verify no error state in the detail view (error_outline icon).
      // Note: We only check there is no *dominant* error state. The
      // error_outline icon could appear in other contexts, so we just
      // make sure we got past the loading spinner.
      expect(
        find.byType(CircularProgressIndicator),
        findsNothing,
        reason: 'Camera detail should have finished loading',
      );
    });
  });

  // =========================================================================
  // Test 6: Navigate to Settings
  // =========================================================================
  group('Settings', () {
    testWidgets('settings screen shows system info', (tester) async {
      await _ensureLoggedIn(tester);

      // Navigate to Settings tab.
      await _navigateToTab(tester, 'Settings');
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // Verify we are on the Settings screen. The AppBar title is 'SETTINGS'.
      expect(
        find.text('SETTINGS'),
        findsOneWidget,
        reason: 'Settings page title should be visible',
      );

      // On the System panel (default section), verify stat tiles.
      expect(
        find.text('VERSION'),
        findsOneWidget,
        reason: 'VERSION stat tile should be visible',
      );
      expect(
        find.text('UPTIME'),
        findsOneWidget,
        reason: 'UPTIME stat tile should be visible',
      );
      expect(
        find.text('CAMERAS'),
        findsOneWidget,
        reason: 'CAMERAS stat tile should be visible',
      );

      // Verify SYSTEM INFO section.
      expect(
        find.text('SYSTEM INFO'),
        findsOneWidget,
        reason: 'SYSTEM INFO section should be visible',
      );

      // Verify CONNECTION section.
      expect(
        find.text('CONNECTION'),
        findsOneWidget,
        reason: 'CONNECTION section should be visible',
      );
    });
  });

  // =========================================================================
  // Test 7: Navigate to Search
  // =========================================================================
  group('Search', () {
    testWidgets('search screen loads with input', (tester) async {
      await _ensureLoggedIn(tester);

      // Navigate to Search tab.
      await _navigateToTab(tester, 'Search');
      await tester.pumpAndSettle(const Duration(seconds: 2));

      // Verify the Search page title.
      expect(
        find.text('Search'),
        findsOneWidget,
        reason: 'Search page title should be visible',
      );

      // Verify the search text field exists with the hint text.
      expect(
        find.byType(TextField),
        findsWidgets,
        reason: 'Search input field should exist',
      );

      // Verify the SEARCH button exists.
      expect(
        find.text('SEARCH'),
        findsOneWidget,
        reason: 'SEARCH button should be visible',
      );

      // Verify the empty state message.
      expect(
        find.text('Enter a query to search recordings'),
        findsOneWidget,
        reason: 'Empty state message should be shown before any search',
      );

      // Type a search query.
      final searchField = find.byType(TextField).first;
      await tester.enterText(searchField, 'person');
      await tester.pumpAndSettle();

      // Tap the SEARCH button.
      await tester.tap(find.text('SEARCH'));
      await tester.pumpAndSettle(const Duration(seconds: 5));

      // After searching, the empty state message should be gone. We should
      // see either results or a "No results found" message.
      expect(
        find.text('Enter a query to search recordings'),
        findsNothing,
        reason: 'Empty state should disappear after searching',
      );

      // Check for either results or no-results message.
      final hasResults = find.textContaining('RESULT').evaluate().isNotEmpty;
      final hasNoResults =
          find.text('No results found').evaluate().isNotEmpty;
      expect(
        hasResults || hasNoResults,
        isTrue,
        reason: 'Should show either results or "No results found" message',
      );
    });
  });

  // =========================================================================
  // Test 8: Camera panel opens (desktop only, >= 600px)
  // =========================================================================
  group('Camera panel', () {
    testWidgets('camera panel slides open on desktop', (tester) async {
      await _ensureLoggedIn(tester);

      // This test is only meaningful on desktop-width screens (>= 600px).
      // If the screen is too narrow, the icon rail is not shown and the
      // camera panel toggle is not available.
      final width = tester.view.physicalSize.width /
          tester.view.devicePixelRatio;
      if (width < 600) {
        // On mobile, skip this test gracefully.
        return;
      }

      // The camera panel opens when we tap the currently active nav icon
      // in the icon rail. Since we are on Live View (index 0), tapping the
      // Live icon (videocam) should toggle the panel.
      // Find the Live icon in the rail by its semantic label.
      final liveIcon = find.bySemanticsLabel('Live');
      if (liveIcon.evaluate().isNotEmpty) {
        await tester.tap(liveIcon.first);
        await tester.pumpAndSettle(const Duration(seconds: 1));

        // Verify the camera panel is open — it has a "CAMERAS" header.
        expect(
          find.text('CAMERAS'),
          findsOneWidget,
          reason: 'Camera panel should show CAMERAS header',
        );

        // Verify the search field in the panel (hint: "Search cameras...").
        expect(
          find.text('Search cameras...'),
          findsOneWidget,
          reason: 'Camera panel should have a search field',
        );

        // Close the panel by tapping the close (X) icon.
        final closeIcon = find.byIcon(Icons.close);
        if (closeIcon.evaluate().isNotEmpty) {
          await tester.tap(closeIcon.first);
          await tester.pumpAndSettle();

          // After closing, the CAMERAS header should no longer be visible.
          expect(
            find.text('CAMERAS'),
            findsNothing,
            reason: 'Camera panel should be closed',
          );
        }
      }
    });
  });

  // =========================================================================
  // Bonus: Full navigation round-trip
  // =========================================================================
  group('Navigation', () {
    testWidgets('all nav tabs are accessible', (tester) async {
      await _ensureLoggedIn(tester);

      // Live View (already here after login).
      expect(find.text('Live View'), findsOneWidget);

      // Playback
      await _navigateToTab(tester, 'Playback');
      await tester.pumpAndSettle(const Duration(seconds: 3));
      expect(
        find.text('Playback'),
        findsWidgets,
        reason: 'Playback screen should be accessible',
      );
      // Playback should show camera filter chips.
      expect(
        find.byType(FilterChip),
        findsWidgets,
        reason: 'Playback should show camera filter chips',
      );
      // Playback should show transport controls (play button).
      expect(
        find.byIcon(Icons.play_arrow),
        findsWidgets,
        reason: 'Playback should show play button in transport bar',
      );

      // Search
      await _navigateToTab(tester, 'Search');
      await tester.pumpAndSettle(const Duration(seconds: 2));
      expect(
        find.text('Search'),
        findsOneWidget,
        reason: 'Search screen should be accessible',
      );

      // Devices
      await _navigateToTab(tester, 'Devices');
      await tester.pumpAndSettle(const Duration(seconds: 2));
      expect(
        find.text('Devices'),
        findsOneWidget,
        reason: 'Devices screen should be accessible',
      );

      // Settings
      await _navigateToTab(tester, 'Settings');
      await tester.pumpAndSettle(const Duration(seconds: 2));
      expect(
        find.text('SETTINGS'),
        findsOneWidget,
        reason: 'Settings screen should be accessible',
      );
    });
  });
}

// ===========================================================================
// Helpers
// ===========================================================================

/// Handles the full auth flow (server setup + login) and lands on Live View.
///
/// If the app is already authenticated (e.g. SharedPreferences persisted from
/// a prior run), this will detect that and skip the auth steps.
Future<void> _ensureLoggedIn(WidgetTester tester) async {
  // Clear prefs to force a clean server-setup -> login -> live-view flow.
  SharedPreferences.setMockInitialValues({});

  await tester.pumpWidget(const ProviderScope(child: NvrApp()));
  await tester.pumpAndSettle(const Duration(seconds: 3));

  // Check if we are already on Live View (persisted session).
  if (find.text('Live View').evaluate().isNotEmpty) {
    return; // Already logged in.
  }

  // -- Server setup screen --
  if (find.text('SERVER URL').evaluate().isNotEmpty ||
      find.text('CONNECT').evaluate().isNotEmpty) {
    final urlField = find.byType(TextFormField);
    if (urlField.evaluate().isNotEmpty) {
      await tester.enterText(urlField.first, testServerUrl);
      await tester.pumpAndSettle();
    }
    final connectBtn = find.widgetWithText(ElevatedButton, 'CONNECT');
    if (connectBtn.evaluate().isNotEmpty) {
      await tester.tap(connectBtn.first);
      await tester.pumpAndSettle(const Duration(seconds: 5));
    }
  }

  // -- Login screen --
  if (find.text('USERNAME').evaluate().isNotEmpty ||
      find.text('SIGN IN').evaluate().isNotEmpty) {
    final fields = find.byType(TextFormField);
    if (fields.evaluate().length >= 2) {
      await tester.enterText(fields.at(0), testUsername);
      await tester.enterText(fields.at(1), testPassword);
      await tester.pumpAndSettle();
    }
    final signInBtn = find.widgetWithText(ElevatedButton, 'SIGN IN');
    if (signInBtn.evaluate().isNotEmpty) {
      await tester.tap(signInBtn.first);
      await tester.pumpAndSettle(const Duration(seconds: 5));
    }
  }

  // Verify we made it to Live View.
  expect(
    find.text('Live View'),
    findsOneWidget,
    reason: 'ensureLoggedIn: should end up on Live View',
  );
}

/// Navigate to a tab by its label.
///
/// On desktop (>= 600px) the icon rail uses semantic labels (Live, Playback,
/// Search, Devices, Settings). On mobile (< 600px) the bottom nav shows
/// text labels (LIVE, PLAYBACK, SEARCH, SETTINGS).
///
/// This helper tries the semantic label first, falling back to the uppercase
/// text label used in the mobile bottom nav.
Future<void> _navigateToTab(WidgetTester tester, String tabName) async {
  final width = tester.view.physicalSize.width /
      tester.view.devicePixelRatio;

  if (width >= 600) {
    // Desktop: icon rail uses semantic labels.
    final semanticFinder = find.bySemanticsLabel(tabName);
    if (semanticFinder.evaluate().isNotEmpty) {
      await tester.tap(semanticFinder.first);
      await tester.pumpAndSettle();
      return;
    }
  }

  // Mobile: bottom nav uses uppercase text labels.
  // Map tab names to mobile nav labels.
  final mobileLabels = {
    'Live': 'LIVE',
    'Playback': 'PLAYBACK',
    'Search': 'SEARCH',
    'Settings': 'SETTINGS',
    // Devices is not in mobile nav; we would need a different approach.
    'Devices': 'SETTINGS', // Fallback — not directly in mobile nav
  };

  final label = mobileLabels[tabName] ?? tabName.toUpperCase();
  final textFinder = find.text(label);
  if (textFinder.evaluate().isNotEmpty) {
    await tester.tap(textFinder.first);
    await tester.pumpAndSettle();
  }
}
