import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../providers/notifications_provider.dart';

class IconRail extends ConsumerWidget {
  const IconRail({
    super.key,
    required this.selectedIndex,
    required this.onDestinationSelected,
    required this.onAlertsTap,
    required this.onCameraPanelToggle,
    this.expanded = false,
  });

  final int selectedIndex;
  final ValueChanged<int> onDestinationSelected;
  final VoidCallback onAlertsTap;
  final VoidCallback onCameraPanelToggle;

  /// When true, the rail shows labels next to icons (desktop mode).
  final bool expanded;

  static const _navItems = [
    (icon: Icons.dashboard_outlined, activeIcon: Icons.dashboard, label: 'Dashboard'),
    (icon: Icons.videocam_outlined, activeIcon: Icons.videocam, label: 'Live'),
    (icon: Icons.access_time_outlined, activeIcon: Icons.access_time_filled, label: 'Playback'),
    (icon: Icons.search_outlined, activeIcon: Icons.search, label: 'Search'),
    (icon: Icons.photo_library_outlined, activeIcon: Icons.photo_library, label: 'Screenshots'),
    (icon: Icons.camera_alt_outlined, activeIcon: Icons.camera_alt, label: 'Devices'),
    (icon: Icons.calendar_month_outlined, activeIcon: Icons.calendar_month, label: 'Schedules'),
  ];

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final unreadCount = ref.watch(notificationsProvider.select((s) => s.unreadCount));

    final railWidth = expanded ? 160.0 : 60.0;

    return AnimatedContainer(
      duration: const Duration(milliseconds: 200),
      curve: Curves.easeInOut,
      width: railWidth,
      color: NvrColors.of(context).bgSecondary,
      child: Column(
        children: [
          const SizedBox(height: 14),
          // Logo
          _LogoMark(expanded: expanded),
          const SizedBox(height: 16),
          // Nav items
          // Rail indices 0-5 map to router indices 0-5; rail index 6 (Schedules) maps to router index 7.
          for (int i = 0; i < _navItems.length; i++) ...[
            if (i == 5) ...[
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
                child: Container(height: 1, color: NvrColors.of(context).border),
              ),
            ],
            _NavItem(
              icon: (i < 6 ? i == selectedIndex : selectedIndex == 7) ? _navItems[i].activeIcon : _navItems[i].icon,
              label: _navItems[i].label,
              isActive: i < 6 ? i == selectedIndex : selectedIndex == 7,
              expanded: expanded,
              onTap: () {
                final routerIndex = i < 6 ? i : 7;
                if (routerIndex == selectedIndex) {
                  onCameraPanelToggle();
                } else {
                  onDestinationSelected(routerIndex);
                }
              },
            ),
            const SizedBox(height: 6),
          ],
          const Spacer(),
          // Alerts
          _NavItem(
            icon: Icons.notifications_outlined,
            label: 'Alerts',
            isActive: false,
            expanded: expanded,
            badge: unreadCount > 0 ? unreadCount : null,
            onTap: onAlertsTap,
          ),
          const SizedBox(height: 6),
          // Settings
          _NavItem(
            icon: Icons.settings_outlined,
            label: 'Settings',
            isActive: selectedIndex == 6,
            expanded: expanded,
            muted: selectedIndex != 6,
            onTap: () => onDestinationSelected(6),
          ),
          const SizedBox(height: 14),
        ],
      ),
    );
  }
}

// ── Logo mark ───────────────────────────────────────────────────────────────

class _LogoMark extends StatelessWidget {
  const _LogoMark({required this.expanded});
  final bool expanded;

  @override
  Widget build(BuildContext context) {
    final logo = Image.asset(
      'assets/raikada-logo-no-bg.png',
      width: 28,
      height: 28,
    );
    if (!expanded) return logo;
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 12),
      child: Row(
        children: [
          logo,
          const SizedBox(width: 10),
          Text(
            'RAIKADA',
            style: NvrTypography.of(context).monoSection.copyWith(
              color: NvrColors.of(context).accent,
              letterSpacing: 2,
            ),
          ),
        ],
      ),
    );
  }
}

// ── Nav item (icon-only or expanded) ────────────────────────────────────────

class _NavItem extends StatelessWidget {
  const _NavItem({
    required this.icon,
    required this.label,
    required this.isActive,
    required this.expanded,
    required this.onTap,
    this.badge,
    this.muted = false,
  });

  final IconData icon;
  final String label;
  final bool isActive;
  final bool expanded;
  final VoidCallback onTap;
  final int? badge;
  final bool muted;

  @override
  Widget build(BuildContext context) {
    return Semantics(
      label: label,
      button: true,
      child: Padding(
        padding: EdgeInsets.symmetric(horizontal: expanded ? 10 : 10),
        child: Stack(
          clipBehavior: Clip.none,
          children: [
            // Active indicator bar
            if (isActive)
              Positioned(
                left: expanded ? -10 : -10,
                top: 10,
                bottom: 10,
                child: Container(
                  width: 3,
                  decoration: BoxDecoration(
                    color: NvrColors.of(context).accent,
                    borderRadius: BorderRadius.circular(2),
                  ),
                ),
              ),
            Material(
              color: isActive ? NvrColors.of(context).accent.withOpacity(0.13) : Colors.transparent,
              borderRadius: BorderRadius.circular(8),
              child: InkWell(
                borderRadius: BorderRadius.circular(8),
                onTap: onTap,
                child: AnimatedContainer(
                  duration: const Duration(milliseconds: 200),
                  curve: Curves.easeInOut,
                  height: 40,
                  padding: expanded
                      ? const EdgeInsets.symmetric(horizontal: 10)
                      : EdgeInsets.zero,
                  decoration: isActive
                      ? BoxDecoration(
                          borderRadius: BorderRadius.circular(8),
                          border: Border.all(color: NvrColors.of(context).accent.withOpacity(0.27)),
                        )
                      : null,
                  child: Row(
                    mainAxisAlignment:
                        expanded ? MainAxisAlignment.start : MainAxisAlignment.center,
                    children: [
                      Icon(
                        icon,
                        size: 20,
                        color: isActive
                            ? NvrColors.of(context).accent
                            : muted
                                ? NvrColors.of(context).textMuted
                                : NvrColors.of(context).textSecondary,
                      ),
                      if (expanded) ...[
                        const SizedBox(width: 10),
                        Expanded(
                          child: Text(
                            label.toUpperCase(),
                            style: TextStyle(
                              fontFamily: 'JetBrainsMono',
                              fontSize: 10,
                              letterSpacing: 1.0,
                              fontWeight: isActive ? FontWeight.w600 : FontWeight.w400,
                              color: isActive
                                  ? NvrColors.of(context).accent
                                  : muted
                                      ? NvrColors.of(context).textMuted
                                      : NvrColors.of(context).textSecondary,
                            ),
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                      ],
                    ],
                  ),
                ),
              ),
            ),
            // Badge
            if (badge != null)
              Positioned(
                right: -2,
                top: -2,
                child: Container(
                  padding: const EdgeInsets.all(3),
                  decoration: BoxDecoration(
                    color: NvrColors.of(context).danger,
                    shape: BoxShape.circle,
                    border: Border.all(color: NvrColors.of(context).bgSecondary, width: 2),
                    boxShadow: [BoxShadow(color: NvrColors.of(context).danger.withOpacity(0.5), blurRadius: 6)],
                  ),
                  child: Text(
                    badge! > 9 ? '9+' : '$badge',
                    style: const TextStyle(fontSize: 7, fontWeight: FontWeight.bold, color: Colors.white),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}
