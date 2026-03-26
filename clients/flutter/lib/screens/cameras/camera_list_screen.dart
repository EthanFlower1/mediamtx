import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

import '../../models/camera.dart';
import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/camera_thumbnail.dart';
import '../../widgets/hud/corner_brackets.dart';
import '../../widgets/hud/hud_button.dart';
import '../../widgets/hud/status_badge.dart';

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
    final serverUrl = ref.watch(authProvider).serverUrl ?? '';

    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      body: SafeArea(
        child: Column(
          children: [
            // ── Top bar ────────────────────────────────────────────────
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
              child: Row(
                children: [
                  const Text('Devices', style: NvrTypography.pageTitle),
                  const SizedBox(width: 10),
                  camerasAsync.maybeWhen(
                    data: (cameras) => Text(
                      '${cameras.length} CAMERAS',
                      style: NvrTypography.monoLabel,
                    ),
                    orElse: () => const SizedBox.shrink(),
                  ),
                  const Spacer(),
                  HudButton(
                    label: 'DISCOVER',
                    style: HudButtonStyle.secondary,
                    icon: Icons.radar,
                    onPressed: () {},
                  ),
                  const SizedBox(width: 8),
                  HudButton(
                    label: 'ADD',
                    style: HudButtonStyle.primary,
                    icon: Icons.add,
                    onPressed: () => context.push('/devices/add'),
                  ),
                ],
              ),
            ),

            // ── Divider ────────────────────────────────────────────────
            const Divider(color: NvrColors.border, height: 1),

            // ── Body ───────────────────────────────────────────────────
            Expanded(
              child: camerasAsync.when(
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
                      HudButton(
                        label: 'RETRY',
                        onPressed: () => ref.invalidate(camerasProvider),
                      ),
                    ],
                  ),
                ),
                data: (cameras) {
                  if (cameras.isEmpty) {
                    return _EmptyState(onAdd: () => context.push('/devices/add'));
                  }
                  return RefreshIndicator(
                    color: NvrColors.accent,
                    backgroundColor: NvrColors.bgSecondary,
                    onRefresh: () async => ref.invalidate(camerasProvider),
                    child: ListView.separated(
                      padding: const EdgeInsets.symmetric(vertical: 12, horizontal: 12),
                      itemCount: cameras.length,
                      separatorBuilder: (_, __) => const SizedBox(height: 8),
                      itemBuilder: (context, index) {
                        final camera = cameras[index];
                        return Dismissible(
                          key: ValueKey(camera.id),
                          direction: DismissDirection.endToStart,
                          confirmDismiss: (_) async {
                            await _deleteCamera(context, ref, camera);
                            return false;
                          },
                          background: Container(
                            color: NvrColors.danger,
                            alignment: Alignment.centerRight,
                            padding: const EdgeInsets.only(right: 20),
                            child: const Icon(Icons.delete, color: Colors.white),
                          ),
                          child: _DeviceCard(
                            camera: camera,
                            serverUrl: serverUrl,
                            onTap: () => context.push('/devices/${camera.id}'),
                          ),
                        );
                      },
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

// ---------------------------------------------------------------------------
// Device card
// ---------------------------------------------------------------------------
class _DeviceCard extends StatelessWidget {
  final Camera camera;
  final String serverUrl;
  final VoidCallback onTap;

  const _DeviceCard({required this.camera, required this.serverUrl, required this.onTap});

  StatusBadge _statusBadge(String status) {
    switch (status) {
      case 'online':
      case 'connected':
        return StatusBadge.online();
      case 'degraded':
        return StatusBadge.degraded();
      default:
        return StatusBadge.offline();
    }
  }

  bool get _isOffline {
    return camera.status != 'online' && camera.status != 'connected' && camera.status != 'degraded';
  }

  @override
  Widget build(BuildContext context) {
    final offline = _isOffline;

    return Opacity(
      opacity: offline ? 0.7 : 1.0,
      child: GestureDetector(
        onTap: onTap,
        child: Container(
          decoration: BoxDecoration(
            color: NvrColors.bgSecondary,
            borderRadius: BorderRadius.circular(8),
            border: Border.all(
              color: offline
                  ? NvrColors.danger.withOpacity(0.35)
                  : NvrColors.border,
            ),
          ),
          padding: const EdgeInsets.all(10),
          child: Row(
            children: [
              // ── Thumbnail ─────────────────────────────────────────
              CornerBrackets(
                bracketSize: 5,
                padding: 3,
                child: CameraThumbnail(
                  serverUrl: serverUrl,
                  cameraId: camera.id,
                  width: 80,
                  height: 48,
                  borderRadius: 4,
                ),
              ),

              const SizedBox(width: 12),

              // ── Info column ─────────────────────────────────────────
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    // Name row + status badge
                    Row(
                      children: [
                        Expanded(
                          child: Text(
                            camera.name,
                            style: NvrTypography.cameraName,
                            maxLines: 1,
                            overflow: TextOverflow.ellipsis,
                          ),
                        ),
                        const SizedBox(width: 8),
                        _statusBadge(camera.status),
                      ],
                    ),

                    const SizedBox(height: 4),

                    // Connection metadata
                    Text(
                      'ID: ${camera.id.length > 8 ? camera.id.substring(0, 8) : camera.id}',
                      style: NvrTypography.monoLabel,
                      maxLines: 1,
                      overflow: TextOverflow.ellipsis,
                    ),

                    const SizedBox(height: 6),

                    // Capability badges
                    Row(
                      children: [
                        if (camera.ptzCapable) ...[
                          _CapabilityBadge(label: 'PTZ'),
                          const SizedBox(width: 4),
                        ],
                        if (camera.aiEnabled) ...[
                          _CapabilityBadge(label: 'AI'),
                          const SizedBox(width: 4),
                        ],
                        _CapabilityBadge(label: 'REC'),
                      ],
                    ),
                  ],
                ),
              ),

              const SizedBox(width: 8),

              // ── Chevron ─────────────────────────────────────────────
              const Icon(Icons.chevron_right, color: NvrColors.textMuted, size: 18),
            ],
          ),
        ),
      ),
    );
  }
}

class _CapabilityBadge extends StatelessWidget {
  final String label;

  const _CapabilityBadge({required this.label});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 5, vertical: 2),
      decoration: BoxDecoration(
        color: NvrColors.accent.withOpacity(0.12),
        borderRadius: BorderRadius.circular(3),
        border: Border.all(color: NvrColors.accent.withOpacity(0.35)),
      ),
      child: Text(
        label,
        style: const TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 8,
          fontWeight: FontWeight.w600,
          letterSpacing: 0.5,
          color: NvrColors.accent,
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------
class _EmptyState extends StatelessWidget {
  final VoidCallback onAdd;

  const _EmptyState({required this.onAdd});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.videocam_off, size: 56, color: NvrColors.textMuted.withOpacity(0.4)),
          const SizedBox(height: 16),
          const Text(
            'No cameras added yet',
            style: NvrTypography.pageTitle,
          ),
          const SizedBox(height: 6),
          const Text(
            'Add a camera to start recording and monitoring',
            style: NvrTypography.body,
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: 24),
          HudButton(
            label: 'ADD CAMERA',
            icon: Icons.add,
            onPressed: onAdd,
          ),
        ],
      ),
    );
  }
}
