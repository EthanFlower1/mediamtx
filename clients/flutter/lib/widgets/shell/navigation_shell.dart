import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../providers/camera_panel_provider.dart';
import '../../utils/responsive.dart';
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
    final device = Responsive.deviceType(width);
    final panelState = ref.watch(cameraPanelProvider);

    // ── Phone: bottom navigation bar ────────────────────────────────────
    if (device == DeviceType.phone) {
      // Map mobile 6-item nav to router indices
      // Mobile: 0=Live, 1=Playback, 2=Search, 3=Screenshots(index 3), 4=Schedules(index 6), 5=Settings(index 5)
      final int mobileIndex;
      if (selectedIndex == 6) {
        mobileIndex = 4;
      } else if (selectedIndex == 5) {
        mobileIndex = 5;
      } else {
        mobileIndex = selectedIndex.clamp(0, 3);
      }
      return Scaffold(
        body: Stack(
          children: [
            child,
            const TourActivePill(),
          ],
        ),
        bottomNavigationBar: MobileBottomNav(
          selectedIndex: mobileIndex,
          onDestinationSelected: (i) {
            // Map mobile indices back: 0=Live, 1=Playback, 2=Search, 3=Screenshots(3), 4=Schedules(6), 5=Settings(5)
            if (i == 4) {
              onDestinationSelected(6);
            } else {
              onDestinationSelected(i);
            }
          },
        ),
      );
    }

    // ── Tablet: compact icon rail, overlay camera panel ─────────────────
    // ── Desktop: expanded nav rail with labels, push camera panel ───────

    final isDesktop = device == DeviceType.desktop;

    // Desktop pushes panel into layout; tablet overlays it.
    final usePushPanel = isDesktop;

    final alertsOpen = ref.watch(alertsPanelOpenProvider);

    return Scaffold(
      body: Row(
        children: [
          // Navigation rail — expanded with labels on desktop, compact on tablet
          IconRail(
            selectedIndex: selectedIndex,
            onDestinationSelected: onDestinationSelected,
            onAlertsTap: () => _onAlertsTap(context, ref),
            onCameraPanelToggle: () => ref.read(cameraPanelProvider.notifier).toggle(),
            expanded: isDesktop,
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
                // Overlay panel for tablet (600-1200)
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
