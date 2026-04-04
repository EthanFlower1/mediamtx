import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/camera.dart';
import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/dashboard_provider.dart';
import '../../providers/notifications_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/camera_thumbnail.dart';
import '../../widgets/hud/corner_brackets.dart';
import '../../widgets/hud/hud_button.dart';
import '../../widgets/hud/status_badge.dart';

class DashboardScreen extends ConsumerWidget {
  const DashboardScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final camerasAsync = ref.watch(camerasProvider);
    final statsAsync = ref.watch(dashboardStatsProvider);
    final serverUrl = ref.watch(authProvider).serverUrl ?? '';
    final notifications = ref.watch(notificationsProvider);

    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      body: SafeArea(
        child: Column(
          children: [
            // Top bar
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
              child: Row(
                children: [
                  const Text('Dashboard', style: NvrTypography.pageTitle),
                  const SizedBox(width: 10),
                  camerasAsync.maybeWhen(
                    data: (cameras) {
                      final online = cameras.where((c) =>
                        c.status == 'online' || c.status == 'connected').length;
                      return Text(
                        '$online/${cameras.length} ONLINE',
                        style: NvrTypography.monoLabel.copyWith(
                          color: online == cameras.length
                              ? NvrColors.success
                              : online == 0
                                  ? NvrColors.danger
                                  : NvrColors.warning,
                        ),
                      );
                    },
                    orElse: () => const SizedBox.shrink(),
                  ),
                  const Spacer(),
                  // System status indicator
                  _SystemStatusDot(connected: notifications.wsConnected),
                ],
              ),
            ),

            const Divider(color: NvrColors.border, height: 1),

            // Body
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
                      const SizedBox(height: 12),
                      const Text(
                        'Failed to load cameras',
                        style: TextStyle(
                            color: NvrColors.textPrimary, fontSize: 16),
                      ),
                      const SizedBox(height: 4),
                      Text(
                        err.toString(),
                        style: const TextStyle(
                            color: NvrColors.textMuted, fontSize: 12),
                        textAlign: TextAlign.center,
                      ),
                      const SizedBox(height: 16),
                      HudButton(
                        label: 'RETRY',
                        onPressed: () {
                          ref.invalidate(camerasProvider);
                          ref.invalidate(dashboardStatsProvider);
                        },
                      ),
                    ],
                  ),
                ),
                data: (cameras) {
                  if (cameras.isEmpty) {
                    return const _EmptyDashboard();
                  }

                  final stats = statsAsync.valueOrNull ?? {};
                  final lastEvents = _buildLastEventMap(notifications);

                  return RefreshIndicator(
                    color: NvrColors.accent,
                    backgroundColor: NvrColors.bgSecondary,
                    onRefresh: () async {
                      ref.invalidate(camerasProvider);
                      ref.invalidate(dashboardStatsProvider);
                      // Wait for both to complete
                      await Future.wait([
                        ref.read(camerasProvider.future),
                        ref.read(dashboardStatsProvider.future),
                      ]);
                    },
                    child: ListView.separated(
                      padding: const EdgeInsets.symmetric(
                          vertical: 12, horizontal: 12),
                      itemCount: cameras.length,
                      separatorBuilder: (_, __) => const SizedBox(height: 8),
                      itemBuilder: (context, index) {
                        final camera = cameras[index];
                        final cameraStats = stats[camera.id];
                        final lastEvent = lastEvents[camera.name];
                        return _DashboardCard(
                          camera: camera,
                          serverUrl: serverUrl,
                          stats: cameraStats,
                          lastEvent: lastEvent,
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

  /// Extracts the most recent non-detection-frame event per camera name
  /// from the WebSocket notification history.
  Map<String, _LastEventInfo> _buildLastEventMap(NotificationState state) {
    final map = <String, _LastEventInfo>{};
    for (final event in state.history) {
      if (event.camera.isEmpty) continue;
      // Skip detection_frame events (too noisy) — prefer motion, ai_detection, etc.
      if (event.isDetectionFrame) continue;
      if (!map.containsKey(event.camera)) {
        map[event.camera] = _LastEventInfo(
          type: event.type,
          message: event.message,
          time: event.time,
        );
      }
    }
    return map;
  }
}

// ---------------------------------------------------------------------------
// Last event info (from WebSocket notifications)
// ---------------------------------------------------------------------------
class _LastEventInfo {
  final String type;
  final String message;
  final DateTime time;

  const _LastEventInfo({
    required this.type,
    required this.message,
    required this.time,
  });
}

// ---------------------------------------------------------------------------
// System status dot
// ---------------------------------------------------------------------------
class _SystemStatusDot extends StatelessWidget {
  final bool connected;
  const _SystemStatusDot({required this.connected});

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          width: 7,
          height: 7,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: connected ? NvrColors.success : NvrColors.danger,
            boxShadow: [
              BoxShadow(
                color: (connected ? NvrColors.success : NvrColors.danger)
                    .withOpacity(0.5),
                blurRadius: 6,
              ),
            ],
          ),
        ),
        const SizedBox(width: 6),
        Text(
          connected ? 'CONNECTED' : 'DISCONNECTED',
          style: NvrTypography.monoLabel.copyWith(
            color: connected ? NvrColors.success : NvrColors.danger,
          ),
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// Dashboard card
// ---------------------------------------------------------------------------
class _DashboardCard extends StatelessWidget {
  final Camera camera;
  final String serverUrl;
  final CameraStats? stats;
  final _LastEventInfo? lastEvent;

  const _DashboardCard({
    required this.camera,
    required this.serverUrl,
    this.stats,
    this.lastEvent,
  });

  bool get _isOnline {
    return camera.status == 'online' || camera.status == 'connected';
  }

  bool get _isDegraded {
    return camera.status == 'degraded';
  }

  StatusBadge _statusBadge() {
    if (_isOnline) return StatusBadge.online();
    if (_isDegraded) return StatusBadge.degraded();
    return StatusBadge.offline();
  }

  @override
  Widget build(BuildContext context) {
    final offline = !_isOnline && !_isDegraded;

    return Opacity(
      opacity: offline ? 0.7 : 1.0,
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
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // Top row: thumbnail + name + status
            Padding(
              padding: const EdgeInsets.all(10),
              child: Row(
                children: [
                  // Thumbnail
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
                  // Name + ID
                  Expanded(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      children: [
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
                            _statusBadge(),
                          ],
                        ),
                        const SizedBox(height: 4),
                        Text(
                          'ID: ${camera.id.length > 8 ? camera.id.substring(0, 8) : camera.id}',
                          style: NvrTypography.monoLabel,
                          maxLines: 1,
                          overflow: TextOverflow.ellipsis,
                        ),
                      ],
                    ),
                  ),
                ],
              ),
            ),

            // Divider
            const Divider(color: NvrColors.border, height: 1),

            // Stats row
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
              child: Row(
                children: [
                  // Recording status
                  _StatChip(
                    icon: Icons.fiber_manual_record,
                    iconColor: stats != null && stats!.hasRecordings
                        ? NvrColors.danger
                        : NvrColors.textMuted,
                    label: stats != null && stats!.hasRecordings
                        ? 'REC'
                        : 'NO REC',
                  ),
                  const SizedBox(width: 12),
                  // Storage usage
                  _StatChip(
                    icon: Icons.storage,
                    iconColor: NvrColors.textSecondary,
                    label: stats?.formattedStorage ?? '--',
                  ),
                  const SizedBox(width: 12),
                  // Segment count
                  _StatChip(
                    icon: Icons.video_file_outlined,
                    iconColor: NvrColors.textSecondary,
                    label: stats != null
                        ? '${stats!.segmentCount} segs'
                        : '--',
                  ),
                  const Spacer(),
                  // Capability badges
                  if (camera.ptzCapable) ...[
                    const _MiniCapBadge(label: 'PTZ'),
                    const SizedBox(width: 4),
                  ],
                  if (camera.aiEnabled) ...[
                    const _MiniCapBadge(label: 'AI'),
                    const SizedBox(width: 4),
                  ],
                ],
              ),
            ),

            // Last event row (if available)
            if (lastEvent != null) ...[
              const Divider(color: NvrColors.border, height: 1),
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                child: Row(
                  children: [
                    Icon(
                      _eventIcon(lastEvent!.type),
                      size: 12,
                      color: _eventColor(lastEvent!.type),
                    ),
                    const SizedBox(width: 6),
                    Expanded(
                      child: Text(
                        lastEvent!.message,
                        style: NvrTypography.monoLabel.copyWith(
                          color: NvrColors.textSecondary,
                          fontSize: 8,
                        ),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    const SizedBox(width: 8),
                    Text(
                      _formatEventTime(lastEvent!.time),
                      style: NvrTypography.monoLabel.copyWith(
                        color: NvrColors.accent,
                        fontSize: 8,
                      ),
                    ),
                  ],
                ),
              ),
            ],
          ],
        ),
      ),
    );
  }

  IconData _eventIcon(String type) {
    switch (type) {
      case 'motion':
        return Icons.directions_run;
      case 'ai_detection':
        return Icons.psychology;
      case 'tampering':
        return Icons.warning_amber;
      case 'line_crossing':
        return Icons.fence;
      case 'intrusion':
        return Icons.shield;
      case 'camera_offline':
        return Icons.videocam_off;
      case 'camera_online':
        return Icons.videocam;
      case 'recording_started':
        return Icons.fiber_manual_record;
      case 'recording_stopped':
        return Icons.stop;
      default:
        return Icons.notifications;
    }
  }

  Color _eventColor(String type) {
    switch (type) {
      case 'motion':
      case 'ai_detection':
        return NvrColors.accent;
      case 'tampering':
      case 'intrusion':
      case 'line_crossing':
        return NvrColors.warning;
      case 'camera_offline':
      case 'recording_stopped':
      case 'recording_failed':
        return NvrColors.danger;
      case 'camera_online':
      case 'recording_started':
        return NvrColors.success;
      default:
        return NvrColors.textSecondary;
    }
  }

  String _formatEventTime(DateTime time) {
    final now = DateTime.now();
    final diff = now.difference(time);
    if (diff.inSeconds < 60) return '${diff.inSeconds}s ago';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    return '${diff.inDays}d ago';
  }
}

// ---------------------------------------------------------------------------
// Stat chip (inline icon + label)
// ---------------------------------------------------------------------------
class _StatChip extends StatelessWidget {
  final IconData icon;
  final Color iconColor;
  final String label;

  const _StatChip({
    required this.icon,
    required this.iconColor,
    required this.label,
  });

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, size: 10, color: iconColor),
        const SizedBox(width: 4),
        Text(
          label,
          style: NvrTypography.monoLabel.copyWith(
            color: NvrColors.textSecondary,
            fontSize: 9,
          ),
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// Mini capability badge
// ---------------------------------------------------------------------------
class _MiniCapBadge extends StatelessWidget {
  final String label;
  const _MiniCapBadge({required this.label});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 1),
      decoration: BoxDecoration(
        color: NvrColors.accent.withOpacity(0.12),
        borderRadius: BorderRadius.circular(3),
        border: Border.all(color: NvrColors.accent.withOpacity(0.35)),
      ),
      child: Text(
        label,
        style: const TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 7,
          fontWeight: FontWeight.w600,
          letterSpacing: 0.5,
          color: NvrColors.accent,
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Empty dashboard state
// ---------------------------------------------------------------------------
class _EmptyDashboard extends StatelessWidget {
  const _EmptyDashboard();

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.dashboard_outlined,
              size: 56, color: NvrColors.textMuted.withOpacity(0.4)),
          const SizedBox(height: 16),
          const Text('No cameras configured', style: NvrTypography.pageTitle),
          const SizedBox(height: 6),
          const Text(
            'Add cameras from the Devices tab to see their status here',
            style: NvrTypography.body,
            textAlign: TextAlign.center,
          ),
        ],
      ),
    );
  }
}
