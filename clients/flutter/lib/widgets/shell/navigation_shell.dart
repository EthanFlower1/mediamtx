import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../providers/camera_panel_provider.dart';
import 'icon_rail.dart';
import 'mobile_bottom_nav.dart';
import 'camera_panel.dart';

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

  void _onAlertsTap(BuildContext context) {
    // TODO: Show alerts panel (Plan 2)
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final width = MediaQuery.of(context).size.width;
    final panelState = ref.watch(cameraPanelProvider);

    // Mobile: < 600px
    if (width < 600) {
      // Map mobile 4-item nav to router indices
      // Mobile: 0=Live, 1=Playback, 2=Search, 3=Settings(index 4 in router)
      final mobileIndex = selectedIndex == 4 ? 3 : selectedIndex.clamp(0, 2);
      return Scaffold(
        body: child,
        bottomNavigationBar: MobileBottomNav(
          selectedIndex: mobileIndex,
          onDestinationSelected: (i) {
            // Map mobile indices back: 0=Live, 1=Playback, 2=Search, 3=Settings(4)
            onDestinationSelected(i == 3 ? 4 : i);
          },
        ),
      );
    }

    // Desktop/Tablet: >= 600px
    final usePushPanel = width >= 1024;

    return Scaffold(
      body: Row(
        children: [
          IconRail(
            selectedIndex: selectedIndex.clamp(0, 3),
            onDestinationSelected: onDestinationSelected,
            onAlertsTap: () => _onAlertsTap(context),
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
              ],
            ),
          ),
        ],
      ),
    );
  }
}
