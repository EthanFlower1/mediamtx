import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/user_preferences.dart';

void main() {
  group('UserPreferences', () {
    test('fromJson parses all fields', () {
      final json = {
        'preferredGridSize': 4,
        'defaultView': '/playback',
        'themeMode': ThemeMode.light.index,
        'enabledNotifications': [
          NotificationEventType.motion.index,
          NotificationEventType.cameraOffline.index,
        ],
      };

      final prefs = UserPreferences.fromJson(json);

      expect(prefs.preferredGridSize, 4);
      expect(prefs.defaultView, '/playback');
      expect(prefs.themeMode, ThemeMode.light);
      expect(prefs.enabledNotifications, {
        NotificationEventType.motion,
        NotificationEventType.cameraOffline,
      });
    });

    test('fromJson uses defaults for missing fields', () {
      final prefs = UserPreferences.fromJson({});

      expect(prefs.preferredGridSize, 2);
      expect(prefs.defaultView, '/live');
      expect(prefs.themeMode, ThemeMode.dark);
      expect(prefs.enabledNotifications, hasLength(8));
    });

    test('fromJson handles null enabledNotifications', () {
      final json = {
        'enabledNotifications': null,
      };

      final prefs = UserPreferences.fromJson(json);

      expect(prefs.enabledNotifications, hasLength(8));
    });

    test('fromJson handles invalid themeMode index', () {
      final json = {
        'themeMode': 99,
      };

      final prefs = UserPreferences.fromJson(json);

      expect(prefs.themeMode, ThemeMode.dark);
    });

    test('fromJson handles null themeMode', () {
      final json = {
        'themeMode': null,
      };

      final prefs = UserPreferences.fromJson(json);

      expect(prefs.themeMode, ThemeMode.dark);
    });

    test('fromJson ignores invalid notification indices', () {
      final json = {
        'enabledNotifications': [0, 1, 999],
      };

      final prefs = UserPreferences.fromJson(json);

      // 999 is out of range and should be filtered out
      expect(prefs.enabledNotifications, {
        NotificationEventType.motion,
        NotificationEventType.personDetected,
      });
    });

    test('fromJson with empty enabledNotifications list', () {
      final json = {
        'enabledNotifications': [],
      };

      final prefs = UserPreferences.fromJson(json);

      expect(prefs.enabledNotifications, isEmpty);
    });

    test('copyWith overrides specific fields', () {
      final prefs = UserPreferences();

      final updated = prefs.copyWith(
        preferredGridSize: 3,
        defaultView: '/playback',
        themeMode: ThemeMode.light,
      );

      expect(updated.preferredGridSize, 3);
      expect(updated.defaultView, '/playback');
      expect(updated.themeMode, ThemeMode.light);
      // enabledNotifications should remain unchanged
      expect(updated.enabledNotifications, hasLength(8));
    });

    test('toJson roundtrips correctly', () {
      final prefs = UserPreferences(
        preferredGridSize: 3,
        defaultView: '/playback',
        themeMode: ThemeMode.light,
        enabledNotifications: {
          NotificationEventType.motion,
          NotificationEventType.cameraOffline,
        },
      );

      final json = prefs.toJson();
      final restored = UserPreferences.fromJson(json);

      expect(restored.preferredGridSize, 3);
      expect(restored.defaultView, '/playback');
      expect(restored.themeMode, ThemeMode.light);
      expect(restored.enabledNotifications, {
        NotificationEventType.motion,
        NotificationEventType.cameraOffline,
      });
    });

    test('default constructor has all notifications enabled', () {
      final prefs = UserPreferences();

      expect(
          prefs.enabledNotifications,
          containsAll([
            NotificationEventType.motion,
            NotificationEventType.personDetected,
            NotificationEventType.vehicleDetected,
            NotificationEventType.animalDetected,
            NotificationEventType.cameraOffline,
            NotificationEventType.cameraOnline,
            NotificationEventType.recordingError,
            NotificationEventType.storageWarning,
          ]));
    });
  });
}
