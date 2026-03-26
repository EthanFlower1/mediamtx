import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../providers/notifications_provider.dart';

class IconRail extends ConsumerWidget {
  const IconRail({
    super.key,
    required this.selectedIndex,
    required this.onDestinationSelected,
    required this.onAlertsTap,
    required this.onCameraPanelToggle,
  });

  final int selectedIndex;
  final ValueChanged<int> onDestinationSelected;
  final VoidCallback onAlertsTap;
  final VoidCallback onCameraPanelToggle;

  static const _navItems = [
    (icon: Icons.videocam_outlined, activeIcon: Icons.videocam, label: 'Live'),
    (icon: Icons.access_time_outlined, activeIcon: Icons.access_time_filled, label: 'Playback'),
    (icon: Icons.search_outlined, activeIcon: Icons.search, label: 'Search'),
    (icon: Icons.camera_alt_outlined, activeIcon: Icons.camera_alt, label: 'Devices'),
  ];

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final unreadCount = ref.watch(notificationsProvider.select((s) => s.unreadCount));

    return Container(
      width: 60,
      color: NvrColors.bgSecondary,
      child: Column(
        children: [
          const SizedBox(height: 14),
          // Logo
          Transform.rotate(
            angle: 0.785398, // 45 degrees
            child: Container(
              width: 18, height: 18,
              decoration: BoxDecoration(
                border: Border.all(color: NvrColors.accent, width: 2),
              ),
            ),
          ),
          const SizedBox(height: 16),
          // Nav items
          for (int i = 0; i < _navItems.length; i++) ...[
            if (i == 3) ...[
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                child: Container(height: 1, color: NvrColors.border),
              ),
            ],
            _NavIcon(
              icon: i == selectedIndex ? _navItems[i].activeIcon : _navItems[i].icon,
              isActive: i == selectedIndex,
              onTap: () {
                if (i == selectedIndex) {
                  onCameraPanelToggle();
                } else {
                  onDestinationSelected(i);
                }
              },
              semanticLabel: _navItems[i].label,
            ),
            const SizedBox(height: 6),
          ],
          const Spacer(),
          // Alerts
          _NavIcon(
            icon: Icons.notifications_outlined,
            isActive: false,
            badge: unreadCount > 0 ? unreadCount : null,
            onTap: onAlertsTap,
            semanticLabel: 'Alerts',
          ),
          const SizedBox(height: 6),
          // Settings
          _NavIcon(
            icon: Icons.settings_outlined,
            isActive: false,
            muted: true,
            onTap: () => onDestinationSelected(4),
            semanticLabel: 'Settings',
          ),
          const SizedBox(height: 14),
        ],
      ),
    );
  }
}

class _NavIcon extends StatelessWidget {
  const _NavIcon({
    required this.icon,
    required this.isActive,
    required this.onTap,
    required this.semanticLabel,
    this.badge,
    this.muted = false,
  });

  final IconData icon;
  final bool isActive;
  final VoidCallback onTap;
  final String semanticLabel;
  final int? badge;
  final bool muted;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: semanticLabel,
      button: true,
      child: Stack(
        clipBehavior: Clip.none,
        children: [
          // Active indicator bar
          if (isActive)
            Positioned(
              left: -10, top: 10, bottom: 10,
              child: Container(width: 3, decoration: BoxDecoration(
                color: NvrColors.accent,
                borderRadius: BorderRadius.circular(2),
              )),
            ),
          Material(
            color: isActive ? NvrColors.accent.withOpacity(0.13) : Colors.transparent,
            borderRadius: BorderRadius.circular(8),
            child: InkWell(
              borderRadius: BorderRadius.circular(8),
              onTap: onTap,
              child: Container(
                width: 40, height: 40,
                decoration: isActive ? BoxDecoration(
                  borderRadius: BorderRadius.circular(8),
                  border: Border.all(color: NvrColors.accent.withOpacity(0.27)),
                ) : null,
                child: Icon(
                  icon, size: 20,
                  color: isActive ? NvrColors.accent : muted ? NvrColors.textMuted : NvrColors.textSecondary,
                ),
              ),
            ),
          ),
          // Badge
          if (badge != null)
            Positioned(
              right: -2, top: -2,
              child: Container(
                padding: const EdgeInsets.all(3),
                decoration: BoxDecoration(
                  color: NvrColors.danger,
                  shape: BoxShape.circle,
                  border: Border.all(color: NvrColors.bgSecondary, width: 2),
                  boxShadow: [BoxShadow(color: NvrColors.danger.withOpacity(0.5), blurRadius: 6)],
                ),
                child: Text(
                  badge! > 9 ? '9+' : '$badge',
                  style: const TextStyle(fontSize: 7, fontWeight: FontWeight.bold, color: Colors.white),
                ),
              ),
            ),
        ],
      ),
    );
  }
}
