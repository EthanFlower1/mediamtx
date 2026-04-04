import 'package:flutter/material.dart';

/// All notification event types the user can toggle on/off.
enum NotificationEventType {
  motion,
  personDetected,
  vehicleDetected,
  animalDetected,
  cameraOffline,
  cameraOnline,
  recordingError,
  storageWarning,
}

/// Represents the user's persisted UI preferences.
class UserPreferences {
  /// Grid layout size (NxN) for the live view.
  final int preferredGridSize;

  /// Route the app navigates to after login (e.g. '/live', '/playback').
  final String defaultView;

  /// Theme mode: system, light, or dark.
  final ThemeMode themeMode;

  /// Which notification event types are enabled.
  final Set<NotificationEventType> enabledNotifications;

  const UserPreferences({
    this.preferredGridSize = 2,
    this.defaultView = '/live',
    this.themeMode = ThemeMode.dark,
    this.enabledNotifications = const {
      NotificationEventType.motion,
      NotificationEventType.personDetected,
      NotificationEventType.vehicleDetected,
      NotificationEventType.animalDetected,
      NotificationEventType.cameraOffline,
      NotificationEventType.cameraOnline,
      NotificationEventType.recordingError,
      NotificationEventType.storageWarning,
    },
  });

  UserPreferences copyWith({
    int? preferredGridSize,
    String? defaultView,
    ThemeMode? themeMode,
    Set<NotificationEventType>? enabledNotifications,
  }) {
    return UserPreferences(
      preferredGridSize: preferredGridSize ?? this.preferredGridSize,
      defaultView: defaultView ?? this.defaultView,
      themeMode: themeMode ?? this.themeMode,
      enabledNotifications: enabledNotifications ?? this.enabledNotifications,
    );
  }

  Map<String, dynamic> toJson() => {
    'preferredGridSize': preferredGridSize,
    'defaultView': defaultView,
    'themeMode': themeMode.index,
    'enabledNotifications':
        enabledNotifications.map((e) => e.index).toList(),
  };

  factory UserPreferences.fromJson(Map<String, dynamic> json) {
    final notifIndices = (json['enabledNotifications'] as List<dynamic>?)
        ?.map((e) => e as int)
        .toList();

    return UserPreferences(
      preferredGridSize: json['preferredGridSize'] as int? ?? 2,
      defaultView: json['defaultView'] as String? ?? '/live',
      themeMode: ThemeMode.values.elementAtOrNull(
            json['themeMode'] as int? ?? ThemeMode.dark.index,
          ) ??
          ThemeMode.dark,
      enabledNotifications: notifIndices != null
          ? notifIndices
              .map((i) => NotificationEventType.values.elementAtOrNull(i))
              .whereType<NotificationEventType>()
              .toSet()
          : const {
              NotificationEventType.motion,
              NotificationEventType.personDetected,
              NotificationEventType.vehicleDetected,
              NotificationEventType.animalDetected,
              NotificationEventType.cameraOffline,
              NotificationEventType.cameraOnline,
              NotificationEventType.recordingError,
              NotificationEventType.storageWarning,
            },
    );
  }
}
