import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../models/camera.dart';
import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/grid_layout_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../utils/responsive.dart';
import '../../widgets/hud/segmented_control.dart';
import 'camera_tile.dart';

class LiveViewScreen extends ConsumerStatefulWidget {
  const LiveViewScreen({super.key});

  @override
  ConsumerState<LiveViewScreen> createState() => _LiveViewScreenState();
}

class _LiveViewScreenState extends ConsumerState<LiveViewScreen> {
  void _openFullscreen(Camera camera) {
    context.push('/live/fullscreen', extra: camera);
  }

  /// Grid options available per device type.
  Map<int, String> _gridOptionsForDevice(DeviceType device) {
    switch (device) {
      case DeviceType.phone:
        return const {1: '1\u00D71', 2: '2\u00D72'};
      case DeviceType.tablet:
        return const {1: '1\u00D71', 2: '2\u00D72', 3: '3\u00D73'};
      case DeviceType.desktop:
        return const {1: '1\u00D71', 2: '2\u00D72', 3: '3\u00D73', 4: '4\u00D74'};
    }
  }

  @override
  Widget build(BuildContext context) {
    final camerasAsync = ref.watch(camerasProvider);
    final gridLayout = ref.watch(gridLayoutProvider);
    final auth = ref.watch(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final device = Responsive.of(context);
    final isPhone = device == DeviceType.phone;

    final gridOptions = _gridOptionsForDevice(device);

    // Clamp current grid size to valid options for this breakpoint.
    final effectiveGridSize = gridOptions.containsKey(gridLayout.gridSize)
        ? gridLayout.gridSize
        : gridOptions.keys.last;

    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      body: Column(
        children: [
          // -- Top bar --
          Container(
            padding: EdgeInsets.symmetric(
              horizontal: isPhone ? 12 : 16,
              vertical: isPhone ? 8 : 12,
            ),
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
                if (!isPhone) ...[
                  const SizedBox(width: 12),
                  // Group badge pill
                  Container(
                    padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
                    decoration: BoxDecoration(
                      color: NvrColors.accentWith(0.07),
                      borderRadius: BorderRadius.circular(99),
                    ),
                    child: const Text(
                      'ALL CAMERAS',
                      style: TextStyle(
                        fontFamily: 'JetBrainsMono',
                        fontSize: 9,
                        fontWeight: FontWeight.w500,
                        letterSpacing: 1.5,
                        color: NvrColors.accent,
                      ),
                    ),
                  ),
                ],

                const Spacer(),

                // Grid size control
                HudSegmentedControl<int>(
                  segments: gridOptions,
                  selected: effectiveGridSize,
                  onChanged: (value) =>
                      ref.read(gridLayoutProvider.notifier).setGridSize(value),
                ),
              ],
            ),
          ),

          // -- Body --
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
                return GridView.builder(
                  padding: EdgeInsets.all(isPhone ? 6 : 10),
                  gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                    crossAxisCount: effectiveGridSize,
                    crossAxisSpacing: isPhone ? 4 : 8,
                    mainAxisSpacing: isPhone ? 4 : 8,
                    childAspectRatio: 16 / 9,
                  ),
                  itemCount: effectiveGridSize * effectiveGridSize,
                  itemBuilder: (context, index) {
                    final cameraId = gridLayout.slots[index];

                    if (cameraId != null) {
                      // Find the Camera object for this slot's ID.
                      final camera = cameras.where((c) => c.id == cameraId).firstOrNull;

                      if (camera != null) {
                        // Occupied slot -- wrap in DragTarget to allow swapping.
                        return DragTarget<String>(
                          onWillAcceptWithDetails: (details) =>
                              details.data != cameraId,
                          onAcceptWithDetails: (details) {
                            // Find the source slot index for the dragged camera.
                            final sourceIndex = gridLayout.slots.entries
                                .where((e) => e.value == details.data)
                                .map((e) => e.key)
                                .firstOrNull;

                            if (sourceIndex != null) {
                              ref
                                  .read(gridLayoutProvider.notifier)
                                  .swapSlots(sourceIndex, index);
                            } else {
                              // Camera came from the panel (no existing slot).
                              ref
                                  .read(gridLayoutProvider.notifier)
                                  .assignCamera(index, details.data);
                            }
                          },
                          builder: (context, candidateData, rejectedData) {
                            final isHovering = candidateData.isNotEmpty;
                            return AnimatedOpacity(
                              opacity: isHovering ? 0.7 : 1.0,
                              duration: const Duration(milliseconds: 150),
                              child: CameraTile(
                                camera: camera,
                                serverUrl: serverUrl,
                                onTap: () => _openFullscreen(camera),
                              ),
                            );
                          },
                        );
                      }
                    }

                    // Empty slot -- DragTarget for assignment.
                    return DragTarget<String>(
                      onWillAcceptWithDetails: (details) => true,
                      onAcceptWithDetails: (details) {
                        ref
                            .read(gridLayoutProvider.notifier)
                            .assignCamera(index, details.data);
                      },
                      builder: (context, candidateData, rejectedData) {
                        final isHovering = candidateData.isNotEmpty;
                        return Container(
                          decoration: BoxDecoration(
                            color: NvrColors.bgPrimary,
                            border: Border.all(
                              color: isHovering
                                  ? NvrColors.accent
                                  : NvrColors.border,
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
              },
            ),
          ),
        ],
      ),
    );
  }
}
