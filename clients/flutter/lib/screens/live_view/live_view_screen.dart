import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../models/camera.dart';
import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../widgets/notification_bell.dart';
import 'camera_tile.dart';

class LiveViewScreen extends ConsumerWidget {
  const LiveViewScreen({super.key});

  int _columnCount(double width) {
    if (width >= 1200) return 4;
    if (width >= 900) return 3;
    if (width >= 600) return 2;
    return 1;
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final camerasAsync = ref.watch(camerasProvider);
    final auth = ref.watch(authProvider);
    final serverUrl = auth.serverUrl ?? '';

    return Scaffold(
      appBar: AppBar(
        title: const Text('Live View'),
        backgroundColor: NvrColors.bgSecondary,
        foregroundColor: NvrColors.textPrimary,
        actions: [
          IconButton(
            icon: const Icon(Icons.refresh),
            tooltip: 'Refresh cameras',
            onPressed: () => ref.invalidate(camerasProvider),
          ),
          const NotificationBell(),
          const SizedBox(width: 8),
        ],
      ),
      backgroundColor: NvrColors.bgPrimary,
      body: camerasAsync.when(
        loading: () => const Center(
          child: CircularProgressIndicator(color: NvrColors.accent),
        ),
        error: (err, _) => Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline, color: NvrColors.danger, size: 48),
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
          if (cameras.isEmpty) {
            return Center(
              child: Column(
                mainAxisSize: MainAxisSize.min,
                children: [
                  const Icon(Icons.videocam_off,
                      color: NvrColors.textMuted, size: 64),
                  const SizedBox(height: 16),
                  Text(
                    'No cameras configured',
                    style: Theme.of(context)
                        .textTheme
                        .titleMedium
                        ?.copyWith(color: NvrColors.textSecondary),
                  ),
                  const SizedBox(height: 8),
                  const Text(
                    'Add cameras in the Cameras section',
                    style: TextStyle(color: NvrColors.textMuted),
                  ),
                ],
              ),
            );
          }

          return LayoutBuilder(
            builder: (context, constraints) {
              final cols = _columnCount(constraints.maxWidth);
              return GridView.builder(
                padding: const EdgeInsets.all(12),
                gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                  crossAxisCount: cols,
                  crossAxisSpacing: 8,
                  mainAxisSpacing: 8,
                  childAspectRatio: 16 / 9,
                ),
                itemCount: cameras.length,
                itemBuilder: (context, index) {
                  final camera = cameras[index];
                  return CameraTile(
                    camera: camera,
                    serverUrl: serverUrl,
                    onTap: () => _openFullscreen(context, camera),
                  );
                },
              );
            },
          );
        },
      ),
    );
  }

  void _openFullscreen(BuildContext context, Camera camera) {
    context.push('/live/fullscreen', extra: camera);
  }
}
