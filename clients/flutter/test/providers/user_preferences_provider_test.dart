import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:nvr_client/providers/user_preferences_provider.dart';
import 'package:nvr_client/models/user_preferences.dart';

void main() {
  group('UserPreferencesNotifier', () {
    late UserPreferencesNotifier notifier;

    setUp(() {
      SharedPreferences.setMockInitialValues({});
      notifier = UserPreferencesNotifier('test_user');
    });

    tearDown(() {
      notifier.dispose();
    });

    test('initial state has correct defaults', () {
      expect(notifier.state.preferredGridSize, 2);
      expect(notifier.state.defaultView, '/live');
      expect(notifier.state.themeMode, ThemeMode.dark);
      expect(
        notifier.state.enabledNotifications,
        containsAll(NotificationEventType.values),
      );
    });

    test('setPreferredGridSize updates state', () {
      notifier.setPreferredGridSize(4);
      expect(notifier.state.preferredGridSize, 4);
    });

    test('setDefaultView updates state', () {
      notifier.setDefaultView('/playback');
      expect(notifier.state.defaultView, '/playback');
    });

    test('setThemeMode updates state', () {
      notifier.setThemeMode(ThemeMode.light);
      expect(notifier.state.themeMode, ThemeMode.light);
    });

    test('toggleNotification disables a type', () {
      notifier.toggleNotification(NotificationEventType.motion, false);
      expect(
        notifier.state.enabledNotifications.contains(NotificationEventType.motion),
        false,
      );
      // Other types should still be enabled
      expect(
        notifier.state.enabledNotifications
            .contains(NotificationEventType.personDetected),
        true,
      );
    });

    test('toggleNotification enables a type', () {
      // First disable, then re-enable
      notifier.toggleNotification(NotificationEventType.motion, false);
      notifier.toggleNotification(NotificationEventType.motion, true);
      expect(
        notifier.state.enabledNotifications.contains(NotificationEventType.motion),
        true,
      );
    });

    test('disableAllNotifications clears all types', () {
      notifier.disableAllNotifications();
      expect(notifier.state.enabledNotifications, isEmpty);
    });

    test('enableAllNotifications adds all types', () {
      notifier.disableAllNotifications();
      notifier.enableAllNotifications();
      expect(
        notifier.state.enabledNotifications,
        containsAll(NotificationEventType.values),
      );
    });

    test('preferences persist to SharedPreferences', () async {
      notifier.setPreferredGridSize(3);
      notifier.setDefaultView('/settings');

      // Allow async _save to complete
      await Future.delayed(Duration.zero);

      final prefs = await SharedPreferences.getInstance();
      final raw = prefs.getString('user_prefs_test_user');
      expect(raw, isNotNull);
    });

    test('loads persisted preferences on construction', () async {
      // Set up preferences first
      notifier.setPreferredGridSize(3);
      notifier.setDefaultView('/playback');
      notifier.setThemeMode(ThemeMode.light);

      // Allow async save
      await Future.delayed(Duration.zero);

      // Create a new notifier with same username
      final notifier2 = UserPreferencesNotifier('test_user');
      addTearDown(() => notifier2.dispose());

      // Allow async _load
      await Future.delayed(Duration.zero);

      expect(notifier2.state.preferredGridSize, 3);
      expect(notifier2.state.defaultView, '/playback');
      expect(notifier2.state.themeMode, ThemeMode.light);
    });

    test('different users have separate preferences', () async {
      notifier.setPreferredGridSize(5);
      await Future.delayed(Duration.zero);

      final otherNotifier = UserPreferencesNotifier('other_user');
      addTearDown(() => otherNotifier.dispose());
      await Future.delayed(Duration.zero);

      // other_user should have default grid size
      expect(otherNotifier.state.preferredGridSize, 2);
    });

    test('corrupted data falls back to defaults', () async {
      // Manually store invalid JSON
      final prefs = await SharedPreferences.getInstance();
      await prefs.setString('user_prefs_corrupt_user', 'not valid json{{{');

      final corruptNotifier = UserPreferencesNotifier('corrupt_user');
      addTearDown(() => corruptNotifier.dispose());
      await Future.delayed(Duration.zero);

      // Should keep defaults since JSON is corrupted
      expect(corruptNotifier.state.preferredGridSize, 2);
      expect(corruptNotifier.state.defaultView, '/live');
    });
  });
}
