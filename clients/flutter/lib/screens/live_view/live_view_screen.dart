import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../models/camera.dart';
import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
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
  int _gridSize = 2;

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

                // Grid size control
                HudSegmentedControl<int>(
                  segments: _gridOptions,
                  selected: _gridSize,
                  onChanged: (value) => setState(() => _gridSize = value),
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
                final totalSlots = _gridSize * _gridSize;
                return GridView.builder(
                  padding: const EdgeInsets.all(10),
                  gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                    crossAxisCount: _gridSize,
                    crossAxisSpacing: 8,
                    mainAxisSpacing: 8,
                    childAspectRatio: 16 / 9,
                  ),
                  itemCount: totalSlots,
                  itemBuilder: (context, index) {
                    if (index < cameras.length) {
                      final camera = cameras[index];
                      return CameraTile(
                        camera: camera,
                        serverUrl: serverUrl,
                        onTap: () => _openFullscreen(camera),
                      );
                    }
                    // Empty slot placeholder
                    return _EmptySlot();
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

class _EmptySlot extends StatelessWidget {
  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: NvrColors.bgPrimary,
        border: Border.all(color: NvrColors.border, width: 2),
        borderRadius: BorderRadius.circular(8),
      ),
      child: const Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.add, color: NvrColors.border, size: 20),
            SizedBox(height: 6),
            Text(
              'DROP HERE',
              style: NvrTypography.monoLabel,
            ),
          ],
        ),
      ),
    );
  }
}
