import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../models/camera.dart';
import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/grid_layout_provider.dart';
import '../../theme/nvr_animations.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/segmented_control.dart';
import 'camera_tile.dart';

class LiveViewScreen extends ConsumerStatefulWidget {
  const LiveViewScreen({super.key});

  @override
  ConsumerState<LiveViewScreen> createState() => _LiveViewScreenState();
}

class _LiveViewScreenState extends ConsumerState<LiveViewScreen> {
  static const _gridOptions = <int, String>{
    1: '1x1',
    2: '2x2',
    3: '3x3',
    4: '4x4',
  };

  void _openFullscreen(Camera camera) {
    context.push('/live/fullscreen', extra: camera);
  }

  // ── Layout save/load ──────────────────────────────────────────────────────

  void _showLayoutMenu() {
    final layoutState = ref.read(gridLayoutProvider);
    final savedLayouts = layoutState.savedLayouts;

    showModalBottomSheet<void>(
      context: context,
      backgroundColor: NvrColors.bgSecondary,
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
                  const Text('Saved Layouts', style: NvrTypography.pageTitle),
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
                      style: NvrTypography.monoLabel.copyWith(
                        color: NvrColors.textMuted,
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
          backgroundColor: NvrColors.bgSecondary,
          title: const Text('Save Layout', style: NvrTypography.pageTitle),
          content: TextField(
            controller: controller,
            autofocus: true,
            style: const TextStyle(color: NvrColors.textPrimary),
            decoration: InputDecoration(
              hintText: 'Layout name',
              hintStyle: const TextStyle(color: NvrColors.textMuted),
              enabledBorder: OutlineInputBorder(
                borderSide: const BorderSide(color: NvrColors.border),
                borderRadius: BorderRadius.circular(6),
              ),
              focusedBorder: OutlineInputBorder(
                borderSide: const BorderSide(color: NvrColors.accent),
                borderRadius: BorderRadius.circular(6),
              ),
            ),
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.pop(ctx),
              child: const Text('Cancel',
                  style: TextStyle(color: NvrColors.textMuted)),
            ),
            TextButton(
              onPressed: () {
                final name = controller.text.trim();
                if (name.isNotEmpty) {
                  ref.read(gridLayoutProvider.notifier).saveLayout(name);
                  Navigator.pop(ctx);
                }
              },
              child: const Text('Save',
                  style: TextStyle(color: NvrColors.accent)),
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

    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      body: Column(
        children: [
          // ── Top bar ───────────────────────────────────────────────────────
          Container(
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
            decoration: const BoxDecoration(
              color: NvrColors.bgPrimary,
              border: Border(
                bottom: BorderSide(color: NvrColors.border, width: 1),
              ),
            ),
            child: Row(
              children: [
                // Page title
                const Text('Live View', style: NvrTypography.pageTitle),
                const SizedBox(width: 12),

                // Active layout name pill (or ALL CAMERAS)
                Container(
                  padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                  decoration: BoxDecoration(
                    color: NvrColors.accentWith(0.07),
                    borderRadius: BorderRadius.circular(99),
                  ),
                  child: Text(
                    gridLayout.name.isNotEmpty
                        ? gridLayout.name.toUpperCase()
                        : 'ALL CAMERAS',
                    style: const TextStyle(
                      fontFamily: 'JetBrainsMono',
                      fontSize: 9,
                      fontWeight: FontWeight.w500,
                      letterSpacing: 1.5,
                      color: NvrColors.accent,
                    ),
                  ),
                ),

                const Spacer(),

                // Layouts button
                GestureDetector(
                  onTap: _showLayoutMenu,
                  child: Container(
                    padding: const EdgeInsets.symmetric(
                        horizontal: 8, vertical: 5),
                    decoration: BoxDecoration(
                      color: NvrColors.bgPrimary,
                      border: Border.all(color: NvrColors.border),
                      borderRadius: BorderRadius.circular(4),
                    ),
                    child: Row(
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        const Icon(Icons.bookmark_border,
                            color: NvrColors.textMuted, size: 14),
                        const SizedBox(width: 4),
                        Text(
                          'Layouts',
                          style: NvrTypography.monoLabel.copyWith(
                            fontSize: 9,
                            color: NvrColors.textMuted,
                          ),
                        ),
                      ],
                    ),
                  ),
                ),
                const SizedBox(width: 8),

                // Grid size control
                HudSegmentedControl<int>(
                  segments: _gridOptions,
                  selected: gridLayout.gridSize,
                  onChanged: (value) =>
                      ref.read(gridLayoutProvider.notifier).setGridSize(value),
                ),
              ],
            ),
          ),

          // ── Body ─────────────────────────────────────────────────────────
          Expanded(
            child: camerasAsync.when(
              loading: () => const Center(
                child: CircularProgressIndicator(color: NvrColors.accent),
              ),
              error: (err, _) => Center(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    const Icon(Icons.error_outline,
                        color: NvrColors.danger, size: 48),
                    const SizedBox(height: 16),
                    Text(
                      'Failed to load cameras',
                      style: Theme.of(context)
                          .textTheme
                          .titleMedium
                          ?.copyWith(color: NvrColors.textPrimary),
                    ),
                    const SizedBox(height: 8),
                    Text(
                      err.toString(),
                      style: const TextStyle(color: NvrColors.textSecondary),
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
                    key: ValueKey(gridLayout.gridSize),
                    gridSize: gridLayout.gridSize,
                    totalSlots: gridLayout.totalSlots,
                    slots: gridLayout.slots,
                    cameras: cameras,
                    serverUrl: serverUrl,
                    onDoubleTap: _openFullscreen,
                    onAssignCamera: (index, cameraId) =>
                        ref.read(gridLayoutProvider.notifier).assignCamera(index, cameraId),
                    onSwapSlots: (from, to) =>
                        ref.read(gridLayoutProvider.notifier).swapSlots(from, to),
                  ),
                );
              },
            ),
          ),
        ],
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
    required this.onAssignCamera,
    required this.onSwapSlots,
  });

  final int gridSize;
  final int totalSlots;
  final Map<int, String> slots;
  final List<Camera> cameras;
  final String serverUrl;
  final void Function(Camera) onDoubleTap;
  final void Function(int index, String cameraId) onAssignCamera;
  final void Function(int from, int to) onSwapSlots;

  @override
  Widget build(BuildContext context) {
    return GridView.builder(
      padding: const EdgeInsets.all(10),
      gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
        crossAxisCount: gridSize,
        crossAxisSpacing: 8,
        mainAxisSpacing: 8,
        childAspectRatio: 16 / 9,
      ),
      itemCount: totalSlots,
      itemBuilder: (context, index) {
        final cameraId = slots[index];

        if (cameraId != null) {
          final camera =
              cameras.where((c) => c.id == cameraId).firstOrNull;

          if (camera != null) {
            return DragTarget<String>(
              onWillAcceptWithDetails: (details) =>
                  details.data != cameraId,
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
                return AnimatedOpacity(
                  opacity: isHovering ? 0.7 : 1.0,
                  duration: NvrAnimations.microDuration,
                  child: CameraTile(
                    camera: camera,
                    serverUrl: serverUrl,
                    onDoubleTap: () => onDoubleTap(camera),
                  ),
                );
              },
            );
          }
        }

        // Empty slot
        return DragTarget<String>(
          onWillAcceptWithDetails: (details) => true,
          onAcceptWithDetails: (details) {
            onAssignCamera(index, details.data);
          },
          builder: (context, candidateData, rejectedData) {
            final isHovering = candidateData.isNotEmpty;
            return Container(
              decoration: BoxDecoration(
                color: NvrColors.bgPrimary,
                border: Border.all(
                  color:
                      isHovering ? NvrColors.accent : NvrColors.border,
                  width: isHovering ? 2 : 1,
                ),
                borderRadius: BorderRadius.circular(6),
              ),
              child: Column(
                mainAxisAlignment: MainAxisAlignment.center,
                children: [
                  Icon(
                    Icons.add,
                    color: isHovering
                        ? NvrColors.accent
                        : NvrColors.border,
                    size: 24,
                  ),
                  const SizedBox(height: 4),
                  Text(
                    'DROP HERE',
                    style: NvrTypography.monoLabel.copyWith(
                      color: isHovering
                          ? NvrColors.accent
                          : NvrColors.textMuted,
                    ),
                  ),
                ],
              ),
            );
          },
        );
      },
    );
  }
}

// ── Supporting widgets ──────────────────────────────────────────────────────

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
          color: NvrColors.accentWith(0.1),
          borderRadius: BorderRadius.circular(4),
          border: Border.all(color: NvrColors.accent.withValues(alpha: 0.3)),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 12, color: NvrColors.accent),
            const SizedBox(width: 4),
            Text(
              label,
              style: NvrTypography.monoLabel.copyWith(
                fontSize: 9,
                color: NvrColors.accent,
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
                ? NvrColors.accentWith(0.07)
                : NvrColors.bgPrimary,
            border: Border.all(
              color: isActive ? NvrColors.accent : NvrColors.border,
            ),
            borderRadius: BorderRadius.circular(6),
          ),
          child: Row(
            children: [
              Icon(
                isActive ? Icons.bookmark : Icons.bookmark_border,
                size: 16,
                color: isActive ? NvrColors.accent : NvrColors.textMuted,
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
                            ? NvrColors.accent
                            : NvrColors.textPrimary,
                      ),
                    ),
                    Text(
                      '${layout.gridSize}x${layout.gridSize} grid  /  ${layout.slots.length} cameras',
                      style: NvrTypography.monoLabel.copyWith(
                        fontSize: 9,
                        color: NvrColors.textMuted,
                      ),
                    ),
                  ],
                ),
              ),
              GestureDetector(
                onTap: onDelete,
                child: const Padding(
                  padding: EdgeInsets.all(4),
                  child: Icon(Icons.close,
                      size: 14, color: NvrColors.textMuted),
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
