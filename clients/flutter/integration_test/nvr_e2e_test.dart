// End-to-end integration tests for the NVR Flutter client.
//
// These run as a SINGLE test that walks through the entire app sequentially,
// keeping the app alive throughout. This avoids re-authentication between
// screens since Flutter integration tests don't share state between
// testWidgets blocks.
//
// Prerequisites:
//   - MediaMTX NVR server running (default: http://localhost:9997)
//   - At least one camera configured and connected
//   - Valid admin credentials (default: admin / admin)
//
// Run:
//   cd clients/flutter
//   flutter test integration_test/nvr_e2e_test.dart -d macos
//
// With custom server/credentials:
//   flutter test integration_test/nvr_e2e_test.dart -d macos \
//     --dart-define=NVR_SERVER_URL=http://192.168.1.100:9997 \
//     --dart-define=NVR_USERNAME=admin \
//     --dart-define=NVR_PASSWORD=mypassword

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:nvr_client/app.dart';

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

  testWidgets('NVR client E2E walkthrough', (tester) async {
    // =====================================================================
    // 1. Boot app from clean state → server setup screen
    // =====================================================================
    SharedPreferences.setMockInitialValues({});
    await tester.pumpWidget(const ProviderScope(child: NvrApp()));
    await tester.pumpAndSettle(const Duration(seconds: 3));

    // Should land on server setup.
    expect(find.text('SERVER URL'), findsOneWidget,
        reason: 'Step 1: Should see SERVER URL on setup screen');

    // Enter server URL and connect.
    await tester.enterText(find.byType(TextFormField).first, testServerUrl);
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(ElevatedButton, 'CONNECT'));
    await tester.pumpAndSettle(const Duration(seconds: 5));

    // =====================================================================
    // 2. Login screen → enter credentials → land on Live View
    // =====================================================================
    expect(find.text('USERNAME'), findsOneWidget,
        reason: 'Step 2: Should see USERNAME on login screen');

    final fields = find.byType(TextFormField);
    await tester.enterText(fields.at(0), testUsername);
    await tester.enterText(fields.at(1), testPassword);
    await tester.pumpAndSettle();
    await tester.tap(find.widgetWithText(ElevatedButton, 'SIGN IN'));
    await tester.pumpAndSettle(const Duration(seconds: 5));

    expect(find.text('Live View'), findsWidgets,
        reason: 'Step 2: Should land on Live View after login');

    // =====================================================================
    // 3. Live View — verify grid and controls
    // =====================================================================
    expect(find.text('ALL CAMERAS'), findsWidgets,
        reason: 'Step 3: ALL CAMERAS badge should be visible');
    expect(find.text('2×2'), findsOneWidget,
        reason: 'Step 3: Grid selector should show 2×2');
    expect(find.byType(GridView), findsOneWidget,
        reason: 'Step 3: Camera grid should exist');

    // Change grid size.
    await tester.tap(find.text('1×1'));
    await tester.pumpAndSettle();
    var grid = tester.widget<GridView>(find.byType(GridView));
    var delegate = grid.gridDelegate as SliverGridDelegateWithFixedCrossAxisCount;
    expect(delegate.crossAxisCount, 1,
        reason: 'Step 3: 1×1 grid should have crossAxisCount 1');

    await tester.tap(find.text('3×3'));
    await tester.pumpAndSettle();
    grid = tester.widget<GridView>(find.byType(GridView));
    delegate = grid.gridDelegate as SliverGridDelegateWithFixedCrossAxisCount;
    expect(delegate.crossAxisCount, 3,
        reason: 'Step 3: 3×3 grid should have crossAxisCount 3');

    // Reset to 2×2.
    await tester.tap(find.text('2×2'));
    await tester.pumpAndSettle();

    // =====================================================================
    // 4. Navigate to Playback
    // =====================================================================
    await _navigateToTab(tester, 'Playback');
    await tester.pumpAndSettle(const Duration(seconds: 3));
    expect(find.text('Playback'), findsWidgets,
        reason: 'Step 4: Playback screen should be visible');

    // =====================================================================
    // 5. Navigate to Search
    // =====================================================================
    await _navigateToTab(tester, 'Search');
    await tester.pumpAndSettle(const Duration(seconds: 2));
    expect(find.text('Search'), findsWidgets,
        reason: 'Step 5: Search screen should be visible');

    // Verify search input exists.
    expect(find.byType(TextField), findsWidgets,
        reason: 'Step 5: Search input should exist');

    // =====================================================================
    // 6. Navigate to Devices → camera detail
    // =====================================================================
    await _navigateToTab(tester, 'Devices');
    await tester.pumpAndSettle(const Duration(seconds: 3));
    expect(find.text('Devices'), findsWidgets,
        reason: 'Step 6: Devices screen should be visible');

    // Tap first device card (find by chevron icon).
    final chevrons = find.byIcon(Icons.chevron_right);
    if (chevrons.evaluate().isNotEmpty) {
      await tester.tap(chevrons.first);
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // Verify camera detail loaded.
      expect(find.byIcon(Icons.arrow_back), findsOneWidget,
          reason: 'Step 6: Camera detail should show back arrow');
      expect(find.text('UPTIME'), findsOneWidget,
          reason: 'Step 6: UPTIME stat tile should be visible');
      expect(find.text('STORAGE'), findsOneWidget,
          reason: 'Step 6: STORAGE stat tile should be visible');
      expect(find.text('EVENTS TODAY'), findsOneWidget,
          reason: 'Step 6: EVENTS TODAY stat tile should be visible');
      expect(find.text('RECORDING'), findsOneWidget,
          reason: 'Step 6: RECORDING section should be visible');
      expect(find.text('AI DETECTION'), findsOneWidget,
          reason: 'Step 6: AI DETECTION section should be visible');

      // Verify the video preview area exists.
      expect(find.byType(AspectRatio), findsWidgets,
          reason: 'Step 6: Video preview aspect ratio should exist');

      // Go back to devices list.
      await tester.tap(find.byIcon(Icons.arrow_back));
      await tester.pumpAndSettle(const Duration(seconds: 2));
    }

    // =====================================================================
    // 7. Navigate to Settings
    // =====================================================================
    await _navigateToTab(tester, 'Settings');
    await tester.pumpAndSettle(const Duration(seconds: 3));

    // Settings has a sidebar with section names.
    expect(find.text('System'), findsWidgets,
        reason: 'Step 7: Settings should show System section');
    expect(find.text('VERSION'), findsOneWidget,
        reason: 'Step 7: VERSION stat tile should be visible');
    expect(find.text('UPTIME'), findsOneWidget,
        reason: 'Step 7: UPTIME stat tile should be visible');

    // =====================================================================
    // 8. Camera panel (desktop only)
    // =====================================================================
    final width =
        tester.view.physicalSize.width / tester.view.devicePixelRatio;
    if (width >= 600) {
      // Navigate back to Live View first.
      await _navigateToTab(tester, 'Live');
      await tester.pumpAndSettle(const Duration(seconds: 2));

      // Tap the Live icon again to toggle camera panel.
      // The active Live icon is Icons.videocam (filled).
      final liveIcon = find.byIcon(Icons.videocam);
      if (liveIcon.evaluate().isNotEmpty) {
        await tester.tap(liveIcon.first);
        await tester.pumpAndSettle(const Duration(seconds: 1));

        // Verify camera panel opened.
        expect(find.text('CAMERAS'), findsOneWidget,
            reason: 'Step 8: Camera panel should show CAMERAS header');
        expect(find.text('Search cameras...'), findsOneWidget,
            reason: 'Step 8: Camera panel should have search field');

        // Close panel.
        final closeIcons = find.byIcon(Icons.close);
        if (closeIcons.evaluate().isNotEmpty) {
          await tester.tap(closeIcons.first);
          await tester.pumpAndSettle();
        }
      }
    }
  });
}

// ==========================================================================
// Helper: navigate to a tab by semantic label (desktop) or text (mobile)
// ==========================================================================
Future<void> _navigateToTab(WidgetTester tester, String tabName) async {
  // Map tab names to their icons in the icon rail / bottom nav.
  // These icons are used in both desktop (icon_rail.dart) and mobile
  // (mobile_bottom_nav.dart) navigation.
  final iconMap = <String, IconData>{
    'Live': Icons.videocam,
    'Playback': Icons.access_time_filled,
    'Search': Icons.search,
    'Devices': Icons.camera_alt,
    'Settings': Icons.settings,
  };

  // Also try the outlined variants (inactive icons).
  final outlinedMap = <String, IconData>{
    'Live': Icons.videocam_outlined,
    'Playback': Icons.access_time_outlined,
    'Search': Icons.search_outlined,
    'Devices': Icons.camera_alt_outlined,
    'Settings': Icons.settings_outlined,
  };

  // Try the filled icon first (active state), then outlined (inactive).
  final icon = iconMap[tabName];
  final outlinedIcon = outlinedMap[tabName];

  if (outlinedIcon != null) {
    final finder = find.byIcon(outlinedIcon);
    if (finder.evaluate().isNotEmpty) {
      await tester.tap(finder.first);
      await tester.pumpAndSettle();
      return;
    }
  }

  if (icon != null) {
    final finder = find.byIcon(icon);
    if (finder.evaluate().isNotEmpty) {
      await tester.tap(finder.first);
      await tester.pumpAndSettle();
      return;
    }
  }

  // Fallback: try uppercase text label (mobile bottom nav).
  final textFinder = find.text(tabName.toUpperCase());
  if (textFinder.evaluate().isNotEmpty) {
    await tester.tap(textFinder.first);
    await tester.pumpAndSettle();
  }
}
