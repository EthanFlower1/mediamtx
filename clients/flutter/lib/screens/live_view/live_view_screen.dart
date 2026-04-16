import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../models/camera.dart';
import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/grid_layout_provider.dart';
import '../../providers/overlay_settings_provider.dart';
import '../../providers/tours_provider.dart';
import '../../theme/nvr_animations.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../utils/responsive.dart';
import '../../widgets/hud/hud_toggle.dart';

import 'camera_tile.dart';

class LiveViewScreen extends ConsumerStatefulWidget {
  const LiveViewScreen({super.key});

  @override
  ConsumerState<LiveViewScreen> createState() => _LiveViewScreenState();
}

class _LiveViewScreenState extends ConsumerState<LiveViewScreen> {
  final FocusNode _focusNode = FocusNode();

  void _openFullscreen(Camera camera) {
    context.push('/live/fullscreen', extra: camera);
  }

  /// Grid options available per device type.
  /// Maps digit key labels to grid slot indices.
  int? _digitToSlot(LogicalKeyboardKey key) {
    if (key == LogicalKeyboardKey.digit1) return 0;
    if (key == LogicalKeyboardKey.digit2) return 1;
    if (key == LogicalKeyboardKey.digit3) return 2;
    if (key == LogicalKeyboardKey.digit4) return 3;
    if (key == LogicalKeyboardKey.digit5) return 4;
    if (key == LogicalKeyboardKey.digit6) return 5;
    if (key == LogicalKeyboardKey.digit7) return 6;
    if (key == LogicalKeyboardKey.digit8) return 7;
    if (key == LogicalKeyboardKey.digit9) return 8;
    return null;
  }

  void _onKeyEvent(KeyEvent event) {
    if (event is! KeyDownEvent) return;

    final slotIndex = _digitToSlot(event.logicalKey);
    if (slotIndex != null) {
      final layoutState = ref.read(gridLayoutProvider);
      final gridLayout = layoutState.active;
      if (slotIndex < gridLayout.totalSlots) {
        // If there's a camera in that slot, open it fullscreen.
        final camerasAsync = ref.read(camerasProvider);
        final cameras = camerasAsync.valueOrNull;
        if (cameras != null) {
          final cameraId = gridLayout.slots[slotIndex];
          if (cameraId != null) {
            final camera =
                cameras.where((c) => c.id == cameraId).firstOrNull;
            if (camera != null) {
              _openFullscreen(camera);
            }
          }
        }
      }
    }
  }

  @override
  void dispose() {
    _focusNode.dispose();
    super.dispose();
  }

  // ── Layout save/load ──────────────────────────────────────────────────────

  void _showLayoutMenu() {
    final layoutState = ref.read(gridLayoutProvider);
    final savedLayouts = layoutState.savedLayouts;

    showModalBottomSheet<void>(
      context: context,
      backgroundColor: NvrColors.of(context).bgSecondary,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(12)),
      ),
      builder: (ctx) {
        return Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 20),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  Text('Saved Layouts', style: NvrTypography.of(context).pageTitle),
                  const Spacer(),
                  _LayoutActionButton(
                    icon: Icons.add,
                    label: 'Save Current',
                    onTap: () {
                      Navigator.pop(ctx);
                      _showSaveLayoutDialog();
                    },
                  ),
                ],
              ),
              const SizedBox(height: 12),
              if (savedLayouts.isEmpty)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 24),
                  child: Center(
                    child: Text(
                      'No saved layouts yet',
                      style: NvrTypography.of(context).monoLabel.copyWith(
                        color: NvrColors.of(context).textMuted,
                      ),
                    ),
                  ),
                )
              else
                ...savedLayouts.map((layout) {
                  final isActive = layoutState.active.name == layout.name &&
                      layout.name.isNotEmpty;
                  return _SavedLayoutTile(
                    layout: layout,
                    isActive: isActive,
                    onLoad: () {
                      ref.read(gridLayoutProvider.notifier).loadLayout(layout.name);
                      Navigator.pop(ctx);
                    },
                    onDelete: () {
                      ref.read(gridLayoutProvider.notifier).deleteLayout(layout.name);
                      Navigator.pop(ctx);
                    },
                  );
                }),
              const SizedBox(height: 8),
            ],
          ),
        );
      },
    );
  }

  void _showSaveLayoutDialog() {
    final controller = TextEditingController();
    showDialog<void>(
      context: context,
      builder: (ctx) {
        return AlertDialog(
          backgroundColor: NvrColors.of(context).bgSecondary,
          title: Text('Save Layout', style: NvrTypography.of(context).pageTitle),
          content: TextField(
            controller: controller,
            autofocus: true,
            style: TextStyle(color: NvrColors.of(context).textPrimary),
            decoration: InputDecoration(
              hintText: 'Layout name',
              hintStyle: TextStyle(color: NvrColors.of(context).textMuted),
              enabledBorder: OutlineInputBorder(
                borderSide: BorderSide(color: NvrColors.of(context).border),
                borderRadius: BorderRadius.circular(6),
              ),
              focusedBorder: OutlineInputBorder(
                borderSide: BorderSide(color: NvrColors.of(context).accent),
                borderRadius: BorderRadius.circular(6),
              ),
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: Text('Cancel',
                  style: TextStyle(color: NvrColors.of(context).textMuted)),
            ),
            TextButton(
              onPressed: () {
                final name = controller.text.trim();
                if (name.isNotEmpty) {
                  ref.read(gridLayoutProvider.notifier).saveLayout(name);
                  Navigator.pop(ctx);
                }
              },
              child: Text('Save',
                  style: TextStyle(color: NvrColors.of(context).accent)),
            ),
          ],
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final camerasAsync = ref.watch(camerasProvider);
    final layoutState = ref.watch(gridLayoutProvider);
    final gridLayout = layoutState.active;
    final auth = ref.watch(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final device = Responsive.of(context);
    final isPhone = device == DeviceType.phone;
    final tourState = ref.watch(activeTourProvider);

    // When a tour is active, override to 1x1 showing the current tour camera.
    final int effectiveGridSize;
    final Map<int, String> effectiveSlots;
    if (tourState.isActive && tourState.currentCameraId != null) {
      effectiveGridSize = 1;
      effectiveSlots = {0: tourState.currentCameraId!};
    } else {
      effectiveSlots = gridLayout.slots;
      // Compute columns from the number of tiles — auto-fit layout.
      final n = effectiveSlots.length;
      int cols = 1;
      while (cols * cols < n) cols++;
      effectiveGridSize = cols;
    }

    return KeyboardListener(
      focusNode: _focusNode,
      autofocus: true,
      onKeyEvent: _onKeyEvent,
      child: Scaffold(
        backgroundColor: NvrColors.of(context).bgPrimary,
        body: Column(
          children: [
            // -- Top bar --
            Container(
              padding: EdgeInsets.symmetric(
                horizontal: isPhone ? 12 : 16,
                vertical: isPhone ? 8 : 12,
              ),
              decoration: BoxDecoration(
                color: NvrColors.of(context).bgPrimary,
                border: Border(
                  bottom: BorderSide(color: NvrColors.of(context).border, width: 1),
                ),
              ),
              child: Row(
                children: [
                  // Page title
                  Text('Live View', style: NvrTypography.of(context).pageTitle),
                  if (!isPhone) ...[
                    const SizedBox(width: 12),
                    // Active layout name pill (or ALL CAMERAS)
                    Container(
                      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                      decoration: BoxDecoration(
                        color: NvrColors.of(context).accentWith(0.07),
                        borderRadius: BorderRadius.circular(99),
                      ),
                      child: Text(
                        gridLayout.name.isNotEmpty
                            ? gridLayout.name.toUpperCase()
                            : 'ALL CAMERAS',
                        style: TextStyle(
                          fontFamily: 'JetBrainsMono',
                          fontSize: 9,
                          fontWeight: FontWeight.w500,
                          letterSpacing: 1.5,
                          color: NvrColors.of(context).accent,
                        ),
                      ),
                    ),
                  ],

                  const Spacer(),

                  // Layouts button
                  GestureDetector(
                    onTap: _showLayoutMenu,
                    child: Container(
                      padding: const EdgeInsets.symmetric(
                          horizontal: 8, vertical: 5),
                      decoration: BoxDecoration(
                        color: NvrColors.of(context).bgPrimary,
                        border: Border.all(color: NvrColors.of(context).border),
                        borderRadius: BorderRadius.circular(4),
                      ),
                      child: Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          Icon(Icons.bookmark_border,
                              color: NvrColors.of(context).textMuted, size: 14),
                          const SizedBox(width: 4),
                          Text(
                            'Layouts',
                            style: NvrTypography.of(context).monoLabel.copyWith(
                              fontSize: 9,
                              color: NvrColors.of(context).textMuted,
                            ),
                          ),
                        ],
                      ),
                    ),
                  ),
                  const SizedBox(width: 8),

                  // AI overlay toggle
                  _AiOverlayToggle(),
                ],
              ),
            ),

            // -- Body --
            Expanded(
              child: camerasAsync.when(
                loading: () => Center(
                  child: CircularProgressIndicator(color: NvrColors.of(context).accent),
                ),
                error: (err, _) => Center(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(Icons.error_outline,
                          color: NvrColors.of(context).danger, size: 48),
                      const SizedBox(height: 16),
                      Text(
                        'Failed to load cameras',
                        style: Theme.of(context)
                            .textTheme
                            .titleMedium
                            ?.copyWith(color: NvrColors.of(context).textPrimary),
                      ),
                      const SizedBox(height: 8),
                      Text(
                        err.toString(),
                        style: TextStyle(color: NvrColors.of(context).textSecondary),
                        textAlign: TextAlign.center,
                      ),
                      const SizedBox(height: 16),
                      ElevatedButton(
                        onPressed: () => ref.invalidate(camerasProvider),
                        child: const Text('Retry'),
                      ),
                    ],
                  ),
                ),
                data: (cameras) {
                  return AnimatedSwitcher(
                    duration: NvrAnimations.panelDuration,
                    switchInCurve: NvrAnimations.panelCurve,
                    switchOutCurve: NvrAnimations.panelCurve,
                    layoutBuilder: (currentChild, previousChildren) {
                      return Stack(
                        alignment: Alignment.center,
                        children: [
                          ...previousChildren,
                          if (currentChild != null) currentChild,
                        ],
                      );
                    },
                    child: _LiveGrid(
                      key: ValueKey('${effectiveGridSize}_${effectiveSlots.length}_${tourState.currentCameraId}'),
                      gridSize: effectiveGridSize,
                      totalSlots: effectiveSlots.length,
                      slots: effectiveSlots,
                      cameras: cameras,
                      serverUrl: serverUrl,
                      isPhone: isPhone,
                      onDoubleTap: _openFullscreen,
                      onAssignCamera: (index, cameraId) =>
                          ref.read(gridLayoutProvider.notifier).assignCamera(index, cameraId),
                      onSwapSlots: (from, to) =>
                          ref.read(gridLayoutProvider.notifier).swapSlots(from, to),
                      onAddCamera: (cameraId) =>
                          ref.read(gridLayoutProvider.notifier).addCamera(cameraId),
                      onRemoveCamera: (slot) =>
                          ref.read(gridLayoutProvider.notifier).removeCamera(slot),
                    ),
                  );
                },
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Extracted grid widget so AnimatedSwitcher can key on gridSize changes.
class _LiveGrid extends StatelessWidget {
  const _LiveGrid({
    super.key,
    required this.gridSize,
    required this.totalSlots,
    required this.slots,
    required this.cameras,
    required this.serverUrl,
    required this.onDoubleTap,
    required this.onRemoveCamera,
    required this.onAssignCamera,
    required this.onSwapSlots,
    required this.onAddCamera,
    this.isPhone = false,
  });

  final int gridSize;
  final int totalSlots;
  final Map<int, String> slots;
  final List<Camera> cameras;
  final String serverUrl;
  final void Function(Camera) onDoubleTap;
  final void Function(int index, String cameraId) onAssignCamera;
  final void Function(int from, int to) onSwapSlots;
  final void Function(String cameraId) onAddCamera;
  final void Function(int slot) onRemoveCamera;
  final bool isPhone;

  void _showTileMenu(BuildContext context, Offset globalPosition, int slot) {
    final overlay = Overlay.of(context).context.findRenderObject() as RenderBox;
    showMenu<String>(
      context: context,
      position: RelativeRect.fromRect(
        globalPosition & const Size(1, 1),
        Offset.zero & overlay.size,
      ),
      color: NvrColors.of(context).bgSecondary,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(6),
        side: BorderSide(color: NvrColors.of(context).border),
      ),
      items: [
        PopupMenuItem(
          value: 'remove',
          height: 32,
          child: Row(
            children: [
              Icon(Icons.close, size: 14, color: NvrColors.of(context).danger),
              const SizedBox(width: 8),
              Text('Remove', style: TextStyle(fontSize: 12, color: NvrColors.of(context).danger)),
            ],
          ),
        ),
      ],
    ).then((value) {
      if (value == 'remove') onRemoveCamera(slot);
    });
  }

  Widget _buildTile(BuildContext context, int index) {
    final cameraId = slots[index];
    if (cameraId == null) return const SizedBox.shrink();

    final camera = cameras.where((c) => c.id == cameraId).firstOrNull;
    if (camera == null) return const SizedBox.shrink();

    return DragTarget<String>(
      onWillAcceptWithDetails: (details) => details.data != cameraId,
      onAcceptWithDetails: (details) {
        final sourceIndex = slots.entries
            .where((e) => e.value == details.data)
            .map((e) => e.key)
            .firstOrNull;
        if (sourceIndex != null) {
          onSwapSlots(sourceIndex, index);
        } else {
          onAssignCamera(index, details.data);
        }
      },
      builder: (context, candidateData, rejectedData) {
        final isHovering = candidateData.isNotEmpty;
        return GestureDetector(
          onSecondaryTapDown: (details) =>
              _showTileMenu(context, details.globalPosition, index),
          onLongPressStart: (details) =>
              _showTileMenu(context, details.globalPosition, index),
          child: AnimatedOpacity(
            opacity: isHovering ? 0.7 : 1.0,
            duration: NvrAnimations.microDuration,
            child: CameraTile(
              camera: camera,
              serverUrl: serverUrl,
              onDoubleTap: () => onDoubleTap(camera),
            ),
          ),
        );
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final gap = isPhone ? 4.0 : 8.0;
    final pad = isPhone ? 6.0 : 10.0;

    // Build rows where each tile is Expanded. The last row's tiles
    // naturally stretch to fill the available width.
    final indices = List.generate(totalSlots, (i) => i);
    final rows = <List<int>>[];
    for (int i = 0; i < indices.length; i += gridSize) {
      rows.add(indices.sublist(i, (i + gridSize).clamp(0, indices.length)));
    }

    return DragTarget<String>(
      onWillAcceptWithDetails: (details) =>
          !slots.values.contains(details.data),
      onAcceptWithDetails: (details) => onAddCamera(details.data),
      builder: (context, candidateData, rejectedData) {
        return LayoutBuilder(
          builder: (context, constraints) {
            final availW = constraints.maxWidth - pad * 2;
            final availH = constraints.maxHeight - pad * 2;
            final n = totalSlots;

            if (n == 0) return const SizedBox.shrink();

            // Find the column count that maximizes tile size at 16:9.
            int bestCols = 1;
            double bestTileW = 0;
            for (int cols = 1; cols <= n; cols++) {
              final numRows = (n / cols).ceil();
              final tileW = (availW - gap * (cols - 1)) / cols;
              final tileH = tileW * 9 / 16;
              final totalH = tileH * numRows + gap * (numRows - 1);
              if (totalH <= availH && tileW > bestTileW) {
                bestTileW = tileW;
                bestCols = cols;
              }
            }

            // Also check fitting by height first (landscape containers).
            for (int cols = 1; cols <= n; cols++) {
              final numRows = (n / cols).ceil();
              final tileH = (availH - gap * (numRows - 1)) / numRows;
              final tileW = tileH * 16 / 9;
              final totalW = tileW * cols + gap * (cols - 1);
              if (totalW <= availW && tileW > bestTileW) {
                bestTileW = tileW;
                bestCols = cols;
              }
            }

            final numRows = (n / bestCols).ceil();
            final tileW = bestTileW;
            final tileH = tileW * 9 / 16;

            // Build rows.
            final rowWidgets = <Widget>[];
            int idx = 0;
            for (int r = 0; r < numRows; r++) {
              final tilesInRow = (r < numRows - 1) ? bestCols : n - idx;
              final rowChildren = <Widget>[];
              for (int c = 0; c < tilesInRow; c++) {
                if (c > 0) rowChildren.add(SizedBox(width: gap));
                rowChildren.add(
                  SizedBox(
                    width: tileW,
                    height: tileH,
                    child: _buildTile(context, idx),
                  ),
                );
                idx++;
              }
              if (r > 0) rowWidgets.add(SizedBox(height: gap));
              rowWidgets.add(Row(
                mainAxisAlignment: MainAxisAlignment.center,
                children: rowChildren,
              ));
            }

            return Center(
              child: Padding(
                padding: EdgeInsets.all(pad),
                child: Column(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: rowWidgets,
                ),
              ),
            );
          },
        );
      },
    );
  }
}

// ── Supporting widgets ──────────────────────────────────────────────────────

class _AiOverlayToggle extends ConsumerWidget {
  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final visible = ref.watch(
      overlaySettingsProvider.select((s) => s.overlayVisible),
    );
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          'AI',
          style: NvrTypography.of(context).monoLabel.copyWith(
            color: visible ? NvrColors.of(context).accent : NvrColors.of(context).textMuted,
          ),
        ),
        const SizedBox(width: 6),
        HudToggle(
          value: visible,
          onChanged: (v) =>
              ref.read(overlaySettingsProvider.notifier).setOverlayVisible(v),
          showStateLabel: false,
        ),
      ],
    );
  }
}

class _LayoutActionButton extends StatelessWidget {
  const _LayoutActionButton({
    required this.icon,
    required this.label,
    required this.onTap,
  });

  final IconData icon;
  final String label;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
        decoration: BoxDecoration(
          color: NvrColors.of(context).accentWith(0.1),
          borderRadius: BorderRadius.circular(4),
          border: Border.all(color: NvrColors.of(context).accent.withValues(alpha: 0.3)),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 12, color: NvrColors.of(context).accent),
            const SizedBox(width: 4),
            Text(
              label,
              style: NvrTypography.of(context).monoLabel.copyWith(
                fontSize: 9,
                color: NvrColors.of(context).accent,
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _SavedLayoutTile extends StatelessWidget {
  const _SavedLayoutTile({
    required this.layout,
    required this.isActive,
    required this.onLoad,
    required this.onDelete,
  });

  final GridLayout layout;
  final bool isActive;
  final VoidCallback onLoad;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(bottom: 6),
      child: GestureDetector(
        onTap: onLoad,
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
          decoration: BoxDecoration(
            color: isActive
                ? NvrColors.of(context).accentWith(0.07)
                : NvrColors.of(context).bgPrimary,
            border: Border.all(
              color: isActive ? NvrColors.of(context).accent : NvrColors.of(context).border,
            ),
            borderRadius: BorderRadius.circular(6),
          ),
          child: Row(
            children: [
              Icon(
                isActive ? Icons.bookmark : Icons.bookmark_border,
                size: 16,
                color: isActive ? NvrColors.of(context).accent : NvrColors.of(context).textMuted,
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      layout.name,
                      style: TextStyle(
                        fontFamily: 'IBMPlexSans',
                        fontSize: 13,
                        fontWeight: FontWeight.w500,
                        color: isActive
                            ? NvrColors.of(context).accent
                            : NvrColors.of(context).textPrimary,
                      ),
                    ),
                    Text(
                      '${layout.gridSize}x${layout.gridSize} grid  /  ${layout.slots.length} cameras',
                      style: NvrTypography.of(context).monoLabel.copyWith(
                        fontSize: 9,
                        color: NvrColors.of(context).textMuted,
                      ),
                    ),
                  ],
                ),
              ),
              GestureDetector(
                onTap: onDelete,
                child: Padding(
                  padding: const EdgeInsets.all(4),
                  child: Icon(Icons.close,
                      size: 14, color: NvrColors.of(context).textMuted),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
