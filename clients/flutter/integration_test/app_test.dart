// End-to-end integration tests for the NVR Flutter client.
//
// Prerequisites:
//   - Raikada server running at http://localhost:9997
//   - At least one camera configured (e.g. "Doorbell")
//   - Default admin/admin credentials
//
// Run with:
//   flutter test integration_test/app_test.dart -d macos

import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:integration_test/integration_test.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:media_kit/media_kit.dart';
import 'package:shared_preferences/shared_preferences.dart';

import 'package:nvr_client/app.dart';

const _serverUrl = 'http://localhost:9997';
const _username = 'admin';
const _password = 'admin';

void main() {
  IntegrationTestWidgetsFlutterBinding.ensureInitialized();
  MediaKit.ensureInitialized();

  group('Auth flow', () {
    testWidgets('connects to server and logs in', (tester) async {
      // Clear any stored prefs so we start fresh at server setup.
      SharedPreferences.setMockInitialValues({});

      await tester.pumpWidget(const ProviderScope(child: NvrApp()));
      await tester.pumpAndSettle();

      // ── Server setup screen ──
      expect(find.text('Connect to NVR'), findsOneWidget);

      // Enter server URL.
      final urlField = find.widgetWithText(TextFormField, 'Server URL');
      expect(urlField, findsOneWidget);
      await tester.enterText(urlField, _serverUrl);
      await tester.pumpAndSettle();

      // Tap Connect.
      await tester.tap(find.text('Connect'));
      await tester.pumpAndSettle(const Duration(seconds: 5));

      // ── Login screen ──
      expect(find.text('Sign In'), findsWidgets); // heading + button

      // Enter credentials.
      final usernameField = find.widgetWithText(TextFormField, 'Username');
      final passwordField = find.widgetWithText(TextFormField, 'Password');
      expect(usernameField, findsOneWidget);
      expect(passwordField, findsOneWidget);

      await tester.enterText(usernameField, _username);
      await tester.enterText(passwordField, _password);
      await tester.pumpAndSettle();

      // Tap Sign In button.
      final signInButton = find.widgetWithText(FilledButton, 'Sign In');
      expect(signInButton, findsOneWidget);
      await tester.tap(signInButton);
      await tester.pumpAndSettle(const Duration(seconds: 5));

      // ── Should land on Live View ──
      expect(find.text('Live View'), findsOneWidget);
    });
  });

  group('Live view', () {
    testWidgets('shows cameras after login', (tester) async {
      await _loginAndLand(tester);

      // Should be on Live View with at least one camera tile.
      expect(find.text('Live View'), findsOneWidget);

      // Wait for cameras to load from API.
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // At least one camera name should appear (e.g. "Doorbell").
      // We don't hardcode the name — just verify a camera tile exists.
      final cameraGrid = find.byType(GridView);
      expect(cameraGrid, findsOneWidget);
    });
  });

  group('Camera management', () {
    testWidgets('can view camera list and open detail', (tester) async {
      await _loginAndLand(tester);

      // Navigate to Cameras tab.
      await tester.tap(find.text('Cameras'));
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // Should see the camera list screen.
      expect(find.byType(ListTile), findsWidgets);

      // Tap the first camera to open detail.
      await tester.tap(find.byType(ListTile).first);
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // Should see camera detail with tabs.
      expect(find.text('General'), findsOneWidget);
      expect(find.text('AI'), findsOneWidget);
      expect(find.text('Advanced'), findsOneWidget);
    });

    testWidgets('can view camera detail tabs', (tester) async {
      await _loginAndLand(tester);

      // Navigate to Cameras tab.
      await tester.tap(find.text('Cameras'));
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // Open first camera.
      await tester.tap(find.byType(ListTile).first);
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // General tab should show name and RTSP URL fields.
      expect(find.widgetWithText(TextFormField, 'Name'), findsOneWidget);
      expect(find.widgetWithText(TextFormField, 'RTSP URL'), findsOneWidget);

      // Tap AI tab.
      await tester.tap(find.text('AI'));
      await tester.pumpAndSettle();

      // Should show AI detection switch.
      expect(find.text('Enable AI detection'), findsOneWidget);

      // Tap Advanced tab.
      await tester.tap(find.text('Advanced'));
      await tester.pumpAndSettle();

      // Should show retention field.
      expect(find.text('Motion timeout'), findsOneWidget);
    });
  });

  group('Playback', () {
    testWidgets('can open playback screen and see controls', (tester) async {
      await _loginAndLand(tester);

      // Navigate to Playback tab.
      await tester.tap(find.text('Playback'));
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // Should see the playback screen.
      expect(find.text('Playback'), findsWidgets); // nav + appbar

      // Should see camera selection chips.
      expect(find.byType(FilterChip), findsWidgets);

      // Should see transport controls (play button).
      expect(find.byIcon(Icons.play_arrow), findsWidgets);
    });

    testWidgets('play button triggers WebSocket session', (tester) async {
      await _loginAndLand(tester);

      // Navigate to Playback tab.
      await tester.tap(find.text('Playback'));
      await tester.pumpAndSettle(const Duration(seconds: 3));

      // Tap play button.
      final playButton = find.byIcon(Icons.play_arrow);
      expect(playButton, findsWidgets);
      await tester.tap(playButton.first);

      // Wait for WebSocket connection + session creation.
      await tester.pumpAndSettle(const Duration(seconds: 5));

      // After play, the button should switch to pause icon.
      expect(find.byIcon(Icons.pause), findsWidgets);
    });
  });

  group('Navigation', () {
    testWidgets('all nav tabs are accessible', (tester) async {
      await _loginAndLand(tester);

      // Live (already here)
      expect(find.text('Live View'), findsOneWidget);

      // Playback
      await tester.tap(find.text('Playback'));
      await tester.pumpAndSettle(const Duration(seconds: 2));
      expect(find.byType(FilterChip), findsWidgets);

      // Search
      await tester.tap(find.text('Search'));
      await tester.pumpAndSettle(const Duration(seconds: 2));
      // Search screen should have a text input.
      expect(find.byType(TextField), findsWidgets);

      // Cameras
      await tester.tap(find.text('Cameras'));
      await tester.pumpAndSettle(const Duration(seconds: 2));
      expect(find.byType(ListTile), findsWidgets);

      // Settings
      await tester.tap(find.text('Settings'));
      await tester.pumpAndSettle(const Duration(seconds: 2));
      // Settings screen exists (just verify navigation worked).
      expect(find.text('Settings'), findsWidgets);
    });
  });
}

/// Helper: runs through server setup + login and lands on Live View.
Future<void> _loginAndLand(WidgetTester tester) async {
  SharedPreferences.setMockInitialValues({});

  await tester.pumpWidget(const ProviderScope(child: NvrApp()));
  await tester.pumpAndSettle();

  // Server setup.
  final urlField = find.widgetWithText(TextFormField, 'Server URL');
  await tester.enterText(urlField, _serverUrl);
  await tester.pumpAndSettle();
  await tester.tap(find.text('Connect'));
  await tester.pumpAndSettle(const Duration(seconds: 5));

  // Login.
  await tester.enterText(
      find.widgetWithText(TextFormField, 'Username'), _username);
  await tester.enterText(
      find.widgetWithText(TextFormField, 'Password'), _password);
  await tester.pumpAndSettle();
  await tester.tap(find.widgetWithText(FilledButton, 'Sign In'));
  await tester.pumpAndSettle(const Duration(seconds: 5));
}
