import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../providers/camera_panel_provider.dart';
import '../../providers/notifications_provider.dart';
import '../alerts_panel.dart';
import 'icon_rail.dart';
import 'mobile_bottom_nav.dart';
import 'camera_panel.dart';
import 'tour_active_pill.dart';

class NavigationShell extends ConsumerWidget {
  const NavigationShell({
    super.key,
    required this.selectedIndex,
    required this.onDestinationSelected,
    required this.child,
  });

  final int selectedIndex;
  final ValueChanged<int> onDestinationSelected;
  final Widget child;

  void _onAlertsTap(BuildContext context, WidgetRef ref) {
    showAlertsPanel(context, ref);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final width = MediaQuery.of(context).size.width;
    final panelState = ref.watch(cameraPanelProvider);

    // Mobile: < 600px
    if (width < 600) {
      // Map mobile 6-item nav to router indices
      // Mobile: 0=Dashboard(0), 1=Live(1), 2=Playback(2), 3=Search(3), 4=Schedules(7), 5=Settings(6)
      final int mobileIndex;
      if (selectedIndex == 7) {
        mobileIndex = 4;
      } else if (selectedIndex == 6) {
        mobileIndex = 5;
      } else {
        mobileIndex = selectedIndex.clamp(0, 3);
      }
      return Scaffold(
        appBar: AppBar(
          backgroundColor: NvrColors.bgSecondary,
          elevation: 0,
          toolbarHeight: 44,
          titleSpacing: 12,
          title: Transform.rotate(
            angle: 0.785398,
            child: Container(
              width: 14,
              height: 14,
              decoration: BoxDecoration(
                border: Border.all(color: NvrColors.accent, width: 2),
              ),
            ),
          ),
          centerTitle: false,
          actions: [
            Padding(
              padding: const EdgeInsets.only(right: 8),
              child: _MobileNotificationBell(
                unreadCount: ref.watch(
                  notificationsProvider.select((s) => s.unreadCount),
                ),
                onTap: () => _onAlertsTap(context, ref),
              ),
            ),
          ],
        ),
        body: Stack(
          children: [
            child,
            const TourActivePill(),
          ],
        ),
        bottomNavigationBar: MobileBottomNav(
          selectedIndex: mobileIndex,
          onDestinationSelected: (i) {
            // Map mobile indices back: 0=Dashboard(0), 1=Live(1), 2=Playback(2), 3=Search(3), 4=Schedules(7), 5=Settings(6)
            if (i == 4) {
              onDestinationSelected(7);
            } else if (i == 5) {
              onDestinationSelected(6);
            } else {
              onDestinationSelected(i);
            }
          },
        ),
      );
    }

    // Desktop/Tablet: >= 600px
    final usePushPanel = width >= 1024;

    final alertsOpen = ref.watch(alertsPanelOpenProvider);

    return Scaffold(
      body: Row(
        children: [
          IconRail(
            selectedIndex: selectedIndex,
            onDestinationSelected: onDestinationSelected,
            onAlertsTap: () => _onAlertsTap(context, ref),
            onCameraPanelToggle: () => ref.read(cameraPanelProvider.notifier).toggle(),
          ),
          Container(width: 1, color: NvrColors.border),
          // Camera panel (push or overlay based on breakpoint)
          if (usePushPanel && panelState.isOpen) ...[
            const CameraPanel(),
            Container(width: 1, color: NvrColors.border),
          ],
          // Main content
          Expanded(
            child: Stack(
              children: [
                child,
                // Overlay panel for tablet portrait (600-1024)
                if (!usePushPanel && panelState.isOpen) ...[
                  // Scrim
                  GestureDetector(
                    onTap: () => ref.read(cameraPanelProvider.notifier).close(),
                    child: Container(color: Colors.black54),
                  ),
                  // Panel
                  const Positioned(left: 0, top: 0, bottom: 0, child: CameraPanel()),
                ],
                // Alerts scrim (desktop overlay)
                if (alertsOpen)
                  GestureDetector(
                    onTap: () => ref.read(alertsPanelOpenProvider.notifier).state = false,
                    child: Container(color: Colors.black45),
                  ),
                // Alerts panel overlay (desktop)
                const AlertsPanelOverlay(),
                // Tour active pill (all screens)
                const TourActivePill(),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _MobileNotificationBell extends StatelessWidget {
  const _MobileNotificationBell({
    required this.unreadCount,
    required this.onTap,
  });

  final int unreadCount;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: SizedBox(
        width: 40,
        height: 40,
        child: Stack(
          alignment: Alignment.center,
          children: [
            const Icon(
              Icons.notifications_outlined,
              size: 22,
              color: NvrColors.textSecondary,
            ),
            if (unreadCount > 0)
              Positioned(
                right: 4,
                top: 6,
                child: Container(
                  padding: const EdgeInsets.all(3),
                  decoration: BoxDecoration(
                    color: NvrColors.danger,
                    shape: BoxShape.circle,
                    border: Border.all(
                      color: NvrColors.bgSecondary,
                      width: 1.5,
                    ),
                  ),
                  child: Text(
                    unreadCount > 9 ? '9+' : '$unreadCount',
                    style: const TextStyle(
                      fontSize: 7,
                      fontWeight: FontWeight.bold,
                      color: Colors.white,
                    ),
                  ),
                ),
              ),
          ],
        ),
      ),
    );
  }
}
