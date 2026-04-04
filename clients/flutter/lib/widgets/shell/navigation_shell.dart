import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../providers/camera_panel_provider.dart';
import '../alerts_panel.dart';
import '../keyboard_shortcut_help.dart';
import 'icon_rail.dart';
import 'mobile_bottom_nav.dart';
import 'camera_panel.dart';
import 'tour_active_pill.dart';

class NavigationShell extends ConsumerStatefulWidget {
  const NavigationShell({
    super.key,
    required this.selectedIndex,
    required this.onDestinationSelected,
    required this.child,
  });

  final int selectedIndex;
  final ValueChanged<int> onDestinationSelected;
  final Widget child;

  @override
  ConsumerState<NavigationShell> createState() => _NavigationShellState();
}

class _NavigationShellState extends ConsumerState<NavigationShell>
    with KeyboardShortcutHelpMixin {
  final FocusNode _shellFocusNode = FocusNode();

  void _onAlertsTap(BuildContext context, WidgetRef ref) {
    showAlertsPanel(context, ref);
  }

  void _onKeyEvent(KeyEvent event) {
    // Let the mixin handle ? and Escape for the help overlay.
    if (handleShortcutHelpKey(event)) return;
  }

  @override
  void dispose() {
    _shellFocusNode.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final width = MediaQuery.of(context).size.width;
    final panelState = ref.watch(cameraPanelProvider);
    final helpOverlay = buildShortcutHelpOverlay();

    // Mobile: < 600px
    if (width < 600) {
      // Map mobile 6-item nav to router indices
      // Mobile: 0=Live, 1=Playback, 2=Search, 3=Screenshots(index 3), 4=Schedules(index 6), 5=Settings(index 5)
      final int mobileIndex;
      if (widget.selectedIndex == 6) {
        mobileIndex = 4;
      } else if (widget.selectedIndex == 5) {
        mobileIndex = 5;
      } else {
        mobileIndex = widget.selectedIndex.clamp(0, 3);
      }
      return KeyboardListener(
        focusNode: _shellFocusNode,
        autofocus: true,
        onKeyEvent: _onKeyEvent,
        child: Scaffold(
          body: Stack(
            children: [
              widget.child,
              const TourActivePill(),
              if (helpOverlay != null) helpOverlay,
            ],
          ),
          bottomNavigationBar: MobileBottomNav(
            selectedIndex: mobileIndex,
            onDestinationSelected: (i) {
              // Map mobile indices back: 0=Live, 1=Playback, 2=Search, 3=Screenshots(3), 4=Schedules(6), 5=Settings(5)
              if (i == 4) {
                widget.onDestinationSelected(6);
              } else {
                widget.onDestinationSelected(i);
              }
            },
          ),
        ),
      );
    }

    // Desktop/Tablet: >= 600px
    final usePushPanel = width >= 1024;

    final alertsOpen = ref.watch(alertsPanelOpenProvider);

    return KeyboardListener(
      focusNode: _shellFocusNode,
      autofocus: true,
      onKeyEvent: _onKeyEvent,
      child: Scaffold(
        body: Row(
          children: [
            IconRail(
              selectedIndex: widget.selectedIndex,
              onDestinationSelected: widget.onDestinationSelected,
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
                  widget.child,
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
                  // Keyboard shortcut help overlay
                  if (helpOverlay != null) helpOverlay,
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
