import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../models/user_preferences.dart';
import '../../providers/user_preferences_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';

class PreferencesPanel extends ConsumerWidget {
  const PreferencesPanel({super.key});

  static const _gridSizes = [1, 2, 3, 4];

  static const _defaultViews = {
    '/live': 'Live View',
    '/playback': 'Playback',
    '/search': 'Search',
    '/screenshots': 'Screenshots',
    '/devices': 'Devices',
    '/settings': 'Settings',
    '/schedules': 'Schedules',
  };

  static const _themeModes = {
    ThemeMode.system: 'System',
    ThemeMode.light: 'Light',
    ThemeMode.dark: 'Dark',
  };

  static String _notificationLabel(NotificationEventType type) {
    switch (type) {
      case NotificationEventType.motion:
        return 'Motion';
      case NotificationEventType.personDetected:
        return 'Person Detected';
      case NotificationEventType.vehicleDetected:
        return 'Vehicle Detected';
      case NotificationEventType.animalDetected:
        return 'Animal Detected';
      case NotificationEventType.cameraOffline:
        return 'Camera Offline';
      case NotificationEventType.cameraOnline:
        return 'Camera Online';
      case NotificationEventType.recordingError:
        return 'Recording Error';
      case NotificationEventType.storageWarning:
        return 'Storage Warning';
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final prefs = ref.watch(userPreferencesProvider);
    final notifier = ref.read(userPreferencesProvider.notifier);

    return ListView(
      padding: const EdgeInsets.all(20),
      children: [
        // ── Theme ──
        _Section(
          title: 'APPEARANCE',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text('Theme', style: NvrTypography.body),
              const SizedBox(height: 8),
              Row(
                children: _themeModes.entries.map((entry) {
                  final isActive = prefs.themeMode == entry.key;
                  return Padding(
                    padding: const EdgeInsets.only(right: 8),
                    child: _ChoiceChip(
                      label: entry.value,
                      isActive: isActive,
                      onTap: () => notifier.setThemeMode(entry.key),
                    ),
                  );
                }).toList(),
              ),
            ],
          ),
        ),
        const SizedBox(height: 16),

        // ── Default View ──
        _Section(
          title: 'DEFAULT VIEW',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'Screen shown after login',
                style: NvrTypography.body,
              ),
              const SizedBox(height: 8),
              Wrap(
                spacing: 8,
                runSpacing: 8,
                children: _defaultViews.entries.map((entry) {
                  final isActive = prefs.defaultView == entry.key;
                  return _ChoiceChip(
                    label: entry.value,
                    isActive: isActive,
                    onTap: () => notifier.setDefaultView(entry.key),
                  );
                }).toList(),
              ),
            ],
          ),
        ),
        const SizedBox(height: 16),

        // ── Grid Size ──
        _Section(
          title: 'PREFERRED GRID LAYOUT',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                'Default grid size for live view',
                style: NvrTypography.body,
              ),
              const SizedBox(height: 8),
              Row(
                children: _gridSizes.map((size) {
                  final isActive = prefs.preferredGridSize == size;
                  return Padding(
                    padding: const EdgeInsets.only(right: 8),
                    child: _ChoiceChip(
                      label: '${size}x$size',
                      isActive: isActive,
                      onTap: () => notifier.setPreferredGridSize(size),
                    ),
                  );
                }).toList(),
              ),
            ],
          ),
        ),
        const SizedBox(height: 16),

        // ── Notification Preferences ──
        _Section(
          title: 'NOTIFICATIONS',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Expanded(
                    child: Text(
                      'Select which events generate alerts',
                      style: NvrTypography.body,
                    ),
                  ),
                  _SmallButton(
                    label: 'ALL',
                    onTap: notifier.enableAllNotifications,
                  ),
                  const SizedBox(width: 6),
                  _SmallButton(
                    label: 'NONE',
                    onTap: notifier.disableAllNotifications,
                  ),
                ],
              ),
              const SizedBox(height: 12),
              ...NotificationEventType.values.map((type) {
                final enabled = prefs.enabledNotifications.contains(type);
                return _ToggleRow(
                  label: _notificationLabel(type),
                  value: enabled,
                  onChanged: (v) => notifier.toggleNotification(type, v),
                );
              }),
            ],
          ),
        ),
      ],
    );
  }
}

// ── Shared widgets (local to this file) ──

class _Section extends StatelessWidget {
  final String title;
  final Widget child;

  const _Section({required this.title, required this.child});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: NvrColors.bgSecondary,
        border: Border.all(color: NvrColors.border),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(title, style: NvrTypography.monoSection),
          const SizedBox(height: 12),
          Container(height: 1, color: NvrColors.border),
          const SizedBox(height: 12),
          child,
        ],
      ),
    );
  }
}

class _ChoiceChip extends StatelessWidget {
  final String label;
  final bool isActive;
  final VoidCallback onTap;

  const _ChoiceChip({
    required this.label,
    required this.isActive,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 6),
        decoration: BoxDecoration(
          color: isActive ? NvrColors.accent.withOpacity(0.12) : Colors.transparent,
          border: Border.all(
            color: isActive ? NvrColors.accent : NvrColors.border,
          ),
          borderRadius: BorderRadius.circular(20),
        ),
        child: Text(
          label.toUpperCase(),
          style: NvrTypography.monoLabel.copyWith(
            color: isActive ? NvrColors.accent : NvrColors.textSecondary,
            letterSpacing: 1.0,
          ),
        ),
      ),
    );
  }
}

class _ToggleRow extends StatelessWidget {
  final String label;
  final bool value;
  final ValueChanged<bool> onChanged;

  const _ToggleRow({
    required this.label,
    required this.value,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        children: [
          Expanded(
            child: Text(
              label,
              style: NvrTypography.body.copyWith(
                color: value ? NvrColors.textPrimary : NvrColors.textSecondary,
              ),
            ),
          ),
          SizedBox(
            height: 24,
            child: Switch(
              value: value,
              onChanged: onChanged,
              activeColor: NvrColors.accent,
              activeTrackColor: NvrColors.accent.withOpacity(0.3),
              inactiveThumbColor: NvrColors.textMuted,
              inactiveTrackColor: NvrColors.bgTertiary,
            ),
          ),
        ],
      ),
    );
  }
}

class _SmallButton extends StatelessWidget {
  final String label;
  final VoidCallback onTap;

  const _SmallButton({required this.label, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
        decoration: BoxDecoration(
          border: Border.all(color: NvrColors.border),
          borderRadius: BorderRadius.circular(4),
        ),
        child: Text(
          label,
          style: NvrTypography.monoLabel.copyWith(
            color: NvrColors.textSecondary,
            letterSpacing: 1.0,
          ),
        ),
      ),
    );
  }
}
