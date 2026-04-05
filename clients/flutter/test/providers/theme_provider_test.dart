import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:nvr_client/providers/theme_provider.dart';

void main() {
  group('ThemeModeNotifier', () {
    setUp(() {
      SharedPreferences.setMockInitialValues({});
    });

    test('initial state is ThemeMode.system', () {
      final notifier = ThemeModeNotifier();
      addTeardownForNotifier(notifier);
      expect(notifier.state, ThemeMode.system);
    });

    test('setThemeMode updates state to dark', () async {
      final notifier = ThemeModeNotifier();
      addTeardownForNotifier(notifier);
      await notifier.setThemeMode(ThemeMode.dark);
      expect(notifier.state, ThemeMode.dark);
    });

    test('setThemeMode updates state to light', () async {
      final notifier = ThemeModeNotifier();
      addTeardownForNotifier(notifier);
      await notifier.setThemeMode(ThemeMode.light);
      expect(notifier.state, ThemeMode.light);
    });

    test('setThemeMode persists to SharedPreferences', () async {
      final notifier = ThemeModeNotifier();
      addTeardownForNotifier(notifier);
      await notifier.setThemeMode(ThemeMode.dark);

      final prefs = await SharedPreferences.getInstance();
      expect(prefs.getString('nvr_theme_mode'), 'dark');
    });

    test('loads persisted theme mode on construction', () async {
      SharedPreferences.setMockInitialValues({'nvr_theme_mode': 'light'});
      final notifier = ThemeModeNotifier();
      addTeardownForNotifier(notifier);

      // Allow async _load to complete
      await Future.delayed(Duration.zero);
      expect(notifier.state, ThemeMode.light);
    });

    test('loads system as default for unknown value', () async {
      SharedPreferences.setMockInitialValues({'nvr_theme_mode': 'unknown'});
      final notifier = ThemeModeNotifier();
      addTeardownForNotifier(notifier);

      await Future.delayed(Duration.zero);
      expect(notifier.state, ThemeMode.system);
    });

    test('cycle goes system -> dark -> light -> system', () async {
      final notifier = ThemeModeNotifier();
      addTeardownForNotifier(notifier);

      expect(notifier.state, ThemeMode.system);

      await notifier.cycle();
      expect(notifier.state, ThemeMode.dark);

      await notifier.cycle();
      expect(notifier.state, ThemeMode.light);

      await notifier.cycle();
      expect(notifier.state, ThemeMode.system);
    });

    test('cycle persists each change', () async {
      final notifier = ThemeModeNotifier();
      addTeardownForNotifier(notifier);

      await notifier.cycle(); // system -> dark
      final prefs = await SharedPreferences.getInstance();
      expect(prefs.getString('nvr_theme_mode'), 'dark');

      await notifier.cycle(); // dark -> light
      expect(prefs.getString('nvr_theme_mode'), 'light');
    });
  });
}

/// Helper to ensure notifier is disposed after test.
void addTeardownForNotifier(ThemeModeNotifier notifier) {
  addTearDown(() => notifier.dispose());
}
