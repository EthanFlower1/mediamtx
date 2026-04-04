import 'dart:convert';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../models/user_preferences.dart';
import 'auth_provider.dart';

class UserPreferencesNotifier extends StateNotifier<UserPreferences> {
  UserPreferencesNotifier(this._username) : super(const UserPreferences()) {
    _load();
  }

  final String _username;

  String get _key => 'user_prefs_$_username';

  Future<void> _load() async {
    final prefs = await SharedPreferences.getInstance();
    final raw = prefs.getString(_key);
    if (raw != null) {
      try {
        state = UserPreferences.fromJson(
          jsonDecode(raw) as Map<String, dynamic>,
        );
      } catch (_) {
        // Corrupted data — keep defaults
      }
    }
  }

  Future<void> _save() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_key, jsonEncode(state.toJson()));
  }

  void setPreferredGridSize(int size) {
    state = state.copyWith(preferredGridSize: size);
    _save();
  }

  void setDefaultView(String route) {
    state = state.copyWith(defaultView: route);
    _save();
  }

  void setThemeMode(ThemeMode mode) {
    state = state.copyWith(themeMode: mode);
    _save();
  }

  void toggleNotification(NotificationEventType type, bool enabled) {
    final updated = Set<NotificationEventType>.from(state.enabledNotifications);
    if (enabled) {
      updated.add(type);
    } else {
      updated.remove(type);
    }
    state = state.copyWith(enabledNotifications: updated);
    _save();
  }

  void enableAllNotifications() {
    state = state.copyWith(
      enabledNotifications: Set.from(NotificationEventType.values),
    );
    _save();
  }

  void disableAllNotifications() {
    state = state.copyWith(enabledNotifications: const {});
    _save();
  }
}

final userPreferencesProvider =
    StateNotifierProvider<UserPreferencesNotifier, UserPreferences>((ref) {
  final username = ref.watch(authProvider).user?.username ?? 'default';
  return UserPreferencesNotifier(username);
});
