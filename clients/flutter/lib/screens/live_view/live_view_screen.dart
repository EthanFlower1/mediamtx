import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../models/camera.dart';
import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/grid_layout_provider.dart';
import '../../providers/overlay_settings_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/hud_toggle.dart';
import '../../widgets/hud/segmented_control.dart';
import 'camera_tile.dart';

class LiveViewScreen extends ConsumerStatefulWidget {
  const LiveViewScreen({super.key});

  @override
  ConsumerState<LiveViewScreen> createState() => _LiveViewScreenState();
}

class _LiveViewScreenState extends ConsumerState<LiveViewScreen> {
  static const _gridOptions = <int, String>{
    1: '1×1',
    2: '2×2',
    3: '3×3',
    4: '4×4',
  };

  void _openFullscreen(Camera camera) {
    context.push('/live/fullscreen', extra: camera);
  }

  @override
  Widget build(BuildContext context) {
    final camerasAsync = ref.watch(camerasProvider);
    final gridLayout = ref.watch(gridLayoutProvider);
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

                const Spacer(),

                // AI overlay toggle
                _AiOverlayToggle(),
                const SizedBox(width: 12),

                // Grid size control — wired to gridLayoutProvider
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
                return GridView.builder(
                  padding: const EdgeInsets.all(10),
                  gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                    crossAxisCount: gridLayout.gridSize,
                    crossAxisSpacing: 8,
                    mainAxisSpacing: 8,
                    childAspectRatio: 16 / 9,
                  ),
                  itemCount: gridLayout.totalSlots,
                  itemBuilder: (context, index) {
                    final cameraId = gridLayout.slots[index];

                    if (cameraId != null) {
                      // Find the Camera object for this slot's ID.
                      final camera = cameras.where((c) => c.id == cameraId).firstOrNull;

                      if (camera != null) {
                        // Occupied slot — wrap in DragTarget to allow swapping.
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

                    // Empty slot — DragTarget for assignment.
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
          style: NvrTypography.monoLabel.copyWith(
            color: visible ? NvrColors.accent : NvrColors.textMuted,
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
