import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../models/camera.dart';
import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../widgets/camera_status_badge.dart';
import '../../widgets/notification_bell.dart';

class CameraListScreen extends ConsumerWidget {
  const CameraListScreen({super.key});

  Future<void> _deleteCamera(
    BuildContext context,
    WidgetRef ref,
    Camera camera,
  ) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: NvrColors.bgSecondary,
        title: const Text(
          'Delete Camera',
          style: TextStyle(color: NvrColors.textPrimary),
        ),
        content: Text(
          'Remove "${camera.name}" from the system? This cannot be undone.',
          style: const TextStyle(color: NvrColors.textSecondary),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: const Text('Cancel', style: TextStyle(color: NvrColors.textSecondary)),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child: const Text('Delete', style: TextStyle(color: NvrColors.danger)),
          ),
        ],
      ),
    );

    if (confirmed != true) return;
    if (!context.mounted) return;

    final api = ref.read(apiClientProvider);
    if (api == null) return;

    try {
      await api.delete('/cameras/${camera.id}');
      ref.invalidate(camerasProvider);
    } catch (e) {
      if (context.mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.danger,
            content: Text('Failed to delete camera: $e'),
          ),
        );
      }
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final camerasAsync = ref.watch(camerasProvider);

    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      appBar: AppBar(
        backgroundColor: NvrColors.bgSecondary,
        title: const Text(
          'Cameras',
          style: TextStyle(color: NvrColors.textPrimary),
        ),
        actions: const [
          NotificationBell(),
          SizedBox(width: 8),
        ],
      ),
      floatingActionButton: FloatingActionButton(
        backgroundColor: NvrColors.accent,
        onPressed: () => context.push('/cameras/add'),
        child: const Icon(Icons.add, color: Colors.white),
      ),
      body: camerasAsync.when(
        loading: () => const Center(
          child: CircularProgressIndicator(color: NvrColors.accent),
        ),
        error: (err, _) => Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline, color: NvrColors.danger, size: 48),
              const SizedBox(height: 12),
              const Text(
                'Failed to load cameras',
                style: TextStyle(color: NvrColors.textPrimary, fontSize: 16),
              ),
              const SizedBox(height: 4),
              Text(
                err.toString(),
                style: const TextStyle(color: NvrColors.textMuted, fontSize: 12),
                textAlign: TextAlign.center,
              ),
              const SizedBox(height: 16),
              ElevatedButton(
                style: ElevatedButton.styleFrom(backgroundColor: NvrColors.accent),
                onPressed: () => ref.invalidate(camerasProvider),
                child: const Text('Retry'),
              ),
            ],
          ),
        ),
        data: (cameras) {
          if (cameras.isEmpty) {
            return _EmptyState(onAdd: () => context.push('/cameras/add'));
          }
          return RefreshIndicator(
            color: NvrColors.accent,
            backgroundColor: NvrColors.bgSecondary,
            onRefresh: () async => ref.invalidate(camerasProvider),
            child: ListView.separated(
              padding: const EdgeInsets.symmetric(vertical: 8),
              itemCount: cameras.length,
              separatorBuilder: (_, __) =>
                  const Divider(color: NvrColors.border, height: 1, indent: 16, endIndent: 16),
              itemBuilder: (context, index) {
                final camera = cameras[index];
                return Dismissible(
                  key: ValueKey(camera.id),
                  direction: DismissDirection.endToStart,
                  confirmDismiss: (_) async {
                    await _deleteCamera(context, ref, camera);
                    return false; // always return false; deletion is handled inside
                  },
                  background: Container(
                    color: NvrColors.danger,
                    alignment: Alignment.centerRight,
                    padding: const EdgeInsets.only(right: 20),
                    child: const Icon(Icons.delete, color: Colors.white),
                  ),
                  child: _CameraListTile(
                    camera: camera,
                    onTap: () => context.push('/cameras/${camera.id}'),
                  ),
                );
              },
            ),
          );
        },
      ),
    );
  }
}

class _CameraListTile extends StatelessWidget {
  final Camera camera;
  final VoidCallback onTap;

  const _CameraListTile({required this.camera, required this.onTap});

  @override
  Widget build(BuildContext context) {
    return ListTile(
      onTap: onTap,
      tileColor: NvrColors.bgPrimary,
      leading: const CircleAvatar(
        backgroundColor: NvrColors.bgTertiary,
        child: Icon(Icons.videocam, color: NvrColors.textSecondary, size: 20),
      ),
      title: Text(
        camera.name,
        style: const TextStyle(
          color: NvrColors.textPrimary,
          fontWeight: FontWeight.w500,
        ),
      ),
      subtitle: CameraStatusBadge(status: camera.status),
      trailing: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          if (camera.aiEnabled)
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
              decoration: BoxDecoration(
                color: NvrColors.accent.withValues(alpha: 0.15),
                borderRadius: BorderRadius.circular(4),
                border: Border.all(color: NvrColors.accent.withValues(alpha: 0.4)),
              ),
              child: const Text(
                'AI',
                style: TextStyle(
                  color: NvrColors.accent,
                  fontSize: 10,
                  fontWeight: FontWeight.w600,
                ),
              ),
            ),
          const SizedBox(width: 8),
          const Icon(Icons.chevron_right, color: NvrColors.textMuted),
        ],
      ),
    );
  }
}

class _EmptyState extends StatelessWidget {
  final VoidCallback onAdd;

  const _EmptyState({required this.onAdd});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.videocam_off, size: 64, color: NvrColors.textMuted.withValues(alpha: 0.5)),
          const SizedBox(height: 16),
          const Text(
            'No cameras added yet',
            style: TextStyle(
              color: NvrColors.textPrimary,
              fontSize: 18,
              fontWeight: FontWeight.w500,
            ),
          ),
          const SizedBox(height: 8),
          const Text(
            'Add a camera to start recording and monitoring',
            style: TextStyle(color: NvrColors.textMuted, fontSize: 13),
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 24),
          ElevatedButton.icon(
            style: ElevatedButton.styleFrom(
              backgroundColor: NvrColors.accent,
              foregroundColor: Colors.white,
              padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
            ),
            onPressed: onAdd,
            icon: const Icon(Icons.add),
            label: const Text('Add Camera'),
          ),
        ],
      ),
    );
  }
}
