import 'package:flutter/material.dart';
import '../../theme/nvr_colors.dart';

class MobileBottomNav extends StatelessWidget {
  const MobileBottomNav({
    super.key,
    required this.selectedIndex,
    required this.onDestinationSelected,
  });

  final int selectedIndex;
  final ValueChanged<int> onDestinationSelected;

  static const _items = [
    (icon: Icons.videocam_outlined, activeIcon: Icons.videocam, label: 'LIVE'),
    (icon: Icons.access_time_outlined, activeIcon: Icons.access_time_filled, label: 'PLAYBACK'),
    (icon: Icons.search_outlined, activeIcon: Icons.search, label: 'SEARCH'),
    (icon: Icons.settings_outlined, activeIcon: Icons.settings, label: 'SETTINGS'),
  ];

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: const BoxDecoration(
        color: NvrColors.bgSecondary,
        border: Border(top: BorderSide(color: NvrColors.border)),
      ),
      child: SafeArea(
        child: SizedBox(
          height: 56,
          child: Row(
            mainAxisAlignment: MainAxisAlignment.spaceAround,
            children: [
              for (int i = 0; i < _items.length; i++)
                Expanded(
                  child: GestureDetector(
                    behavior: HitTestBehavior.opaque,
                    onTap: () => onDestinationSelected(i),
                    child: Column(
                      mainAxisAlignment: MainAxisAlignment.center,
                      children: [
                        Icon(
                          i == selectedIndex ? _items[i].activeIcon : _items[i].icon,
                          size: 20,
                          color: i == selectedIndex ? NvrColors.accent : NvrColors.textSecondary,
                        ),
                        const SizedBox(height: 3),
                        Text(
                          _items[i].label,
                          style: TextStyle(
                            fontFamily: 'JetBrainsMono',
                            fontSize: 8,
                            letterSpacing: 0.5,
                            color: i == selectedIndex ? NvrColors.accent : NvrColors.textSecondary,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}
