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
              Text('Theme', style: NvrTypography.of(context).body),
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
                style: NvrTypography.of(context).body,
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
                style: NvrTypography.of(context).body,
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
                      style: NvrTypography.of(context).body,
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
    final colors = NvrColors.of(context);
    return Container(
      padding: const EdgeInsets.all(16),
      decoration: BoxDecoration(
        color: colors.bgSecondary,
        border: Border.all(color: colors.border),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(title, style: NvrTypography.of(context).monoSection),
          const SizedBox(height: 12),
          Container(height: 1, color: colors.border),
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
    final colors = NvrColors.of(context);
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 6),
        decoration: BoxDecoration(
          color: isActive ? colors.accent.withOpacity(0.12) : Colors.transparent,
          border: Border.all(
            color: isActive ? colors.accent : colors.border,
          ),
          borderRadius: BorderRadius.circular(20),
        ),
        child: Text(
          label.toUpperCase(),
          style: NvrTypography.of(context).monoLabel.copyWith(
            color: isActive ? colors.accent : colors.textSecondary,
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
    final colors = NvrColors.of(context);
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        children: [
          Expanded(
            child: Text(
              label,
              style: NvrTypography.of(context).body.copyWith(
                color: value ? colors.textPrimary : colors.textSecondary,
              ),
            ),
          ),
          SizedBox(
            height: 24,
            child: Switch(
              value: value,
              onChanged: onChanged,
              activeColor: colors.accent,
              activeTrackColor: colors.accent.withOpacity(0.3),
              inactiveThumbColor: colors.textMuted,
              inactiveTrackColor: colors.bgTertiary,
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
    final colors = NvrColors.of(context);
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 4),
        decoration: BoxDecoration(
          border: Border.all(color: colors.border),
          borderRadius: BorderRadius.circular(4),
        ),
        child: Text(
          label,
          style: NvrTypography.of(context).monoLabel.copyWith(
            color: colors.textSecondary,
            letterSpacing: 1.0,
          ),
        ),
      ),
    );
  }
}
