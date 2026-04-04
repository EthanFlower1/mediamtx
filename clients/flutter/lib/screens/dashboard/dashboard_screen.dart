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
    final colors = NvrColors.of(context);
    final typo = NvrTypography.of(context);

    final camerasAsync = ref.watch(camerasProvider);
    final statsAsync = ref.watch(dashboardStatsProvider);
    final serverUrl = ref.watch(authProvider).serverUrl ?? '';
    final notifications = ref.watch(notificationsProvider);

    return Scaffold(
      backgroundColor: colors.bgPrimary,
      body: SafeArea(
        child: Column(
          children: [
            // Top bar
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 12, 16, 12),
              child: Row(
                children: [
                  Text('Dashboard', style: typo.pageTitle),
                  const SizedBox(width: 10),
                  camerasAsync.maybeWhen(
                    data: (cameras) {
                      final online = cameras.where((c) =>
                        c.status == 'online' || c.status == 'connected').length;
                      return Text(
                        '$online/${cameras.length} ONLINE',
                        style: typo.monoLabel.copyWith(
                          color: online == cameras.length
                              ? colors.success
                              : online == 0
                                  ? colors.danger
                                  : colors.warning,
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

            Divider(color: colors.border, height: 1),

            // Body
            Expanded(
              child: camerasAsync.when(
                loading: () => Center(
                  child: CircularProgressIndicator(color: colors.accent),
                ),
                error: (err, _) => Center(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(Icons.error_outline,
                          color: colors.danger, size: 48),
                      const SizedBox(height: 12),
                      Text(
                        'Failed to load cameras',
                        style: TextStyle(
                            color: colors.textPrimary, fontSize: 16),
                      ),
                      const SizedBox(height: 4),
                      Text(
                        err.toString(),
                        style: TextStyle(
                            color: colors.textMuted, fontSize: 12),
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
                    color: colors.accent,
                    backgroundColor: colors.bgSecondary,
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
    final colors = NvrColors.of(context);
    final typo = NvrTypography.of(context);

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          width: 7,
          height: 7,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            color: connected ? colors.success : colors.danger,
            boxShadow: [
              BoxShadow(
                color: (connected ? colors.success : colors.danger)
                    .withOpacity(0.5),
                blurRadius: 6,
              ),
            ],
          ),
        ),
        const SizedBox(width: 6),
        Text(
          connected ? 'CONNECTED' : 'DISCONNECTED',
          style: typo.monoLabel.copyWith(
            color: connected ? colors.success : colors.danger,
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

  StatusBadge _statusBadge(BuildContext context) {
    if (_isOnline) return StatusBadge.online(context);
    if (_isDegraded) return StatusBadge.degraded(context);
    return StatusBadge.offline(context);
  }

  @override
  Widget build(BuildContext context) {
    final colors = NvrColors.of(context);
    final typo = NvrTypography.of(context);

    final offline = !_isOnline && !_isDegraded;

    return Opacity(
      opacity: offline ? 0.7 : 1.0,
      child: Container(
        decoration: BoxDecoration(
          color: colors.bgSecondary,
          borderRadius: BorderRadius.circular(8),
          border: Border.all(
            color: offline
                ? colors.danger.withOpacity(0.35)
                : colors.border,
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
                                style: typo.cameraName,
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                              ),
                            ),
                            const SizedBox(width: 8),
                            _statusBadge(context),
                          ],
                        ),
                        const SizedBox(height: 4),
                        Text(
                          'ID: ${camera.id.length > 8 ? camera.id.substring(0, 8) : camera.id}',
                          style: typo.monoLabel,
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
            Divider(color: colors.border, height: 1),

            // Stats row
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
              child: Row(
                children: [
                  // Recording status
                  _StatChip(
                    icon: Icons.fiber_manual_record,
                    iconColor: stats != null && stats!.hasRecordings
                        ? colors.danger
                        : colors.textMuted,
                    label: stats != null && stats!.hasRecordings
                        ? 'REC'
                        : 'NO REC',
                  ),
                  const SizedBox(width: 12),
                  // Storage usage
                  _StatChip(
                    icon: Icons.storage,
                    iconColor: colors.textSecondary,
                    label: stats?.formattedStorage ?? '--',
                  ),
                  const SizedBox(width: 12),
                  // Segment count
                  _StatChip(
                    icon: Icons.video_file_outlined,
                    iconColor: colors.textSecondary,
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
              Divider(color: colors.border, height: 1),
              Padding(
                padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
                child: Row(
                  children: [
                    Icon(
                      _eventIcon(lastEvent!.type),
                      size: 12,
                      color: _eventColor(context, lastEvent!.type),
                    ),
                    const SizedBox(width: 6),
                    Expanded(
                      child: Text(
                        lastEvent!.message,
                        style: typo.monoLabel.copyWith(
                          color: colors.textSecondary,
                          fontSize: 8,
                        ),
                        maxLines: 1,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    const SizedBox(width: 8),
                    Text(
                      _formatEventTime(lastEvent!.time),
                      style: typo.monoLabel.copyWith(
                        color: colors.accent,
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

  Color _eventColor(BuildContext context, String type) {
    final colors = NvrColors.of(context);
    switch (type) {
      case 'motion':
      case 'ai_detection':
        return colors.accent;
      case 'tampering':
      case 'intrusion':
      case 'line_crossing':
        return colors.warning;
      case 'camera_offline':
      case 'recording_stopped':
      case 'recording_failed':
        return colors.danger;
      case 'camera_online':
      case 'recording_started':
        return colors.success;
      default:
        return colors.textSecondary;
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
    final colors = NvrColors.of(context);
    final typo = NvrTypography.of(context);

    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Icon(icon, size: 10, color: iconColor),
        const SizedBox(width: 4),
        Text(
          label,
          style: typo.monoLabel.copyWith(
            color: colors.textSecondary,
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
    final colors = NvrColors.of(context);

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 1),
      decoration: BoxDecoration(
        color: colors.accent.withOpacity(0.12),
        borderRadius: BorderRadius.circular(3),
        border: Border.all(color: colors.accent.withOpacity(0.35)),
      ),
      child: Text(
        label,
        style: TextStyle(
          fontFamily: 'JetBrainsMono',
          fontSize: 7,
          fontWeight: FontWeight.w600,
          letterSpacing: 0.5,
          color: colors.accent,
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
    final colors = NvrColors.of(context);
    final typo = NvrTypography.of(context);

    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.dashboard_outlined,
              size: 56, color: colors.textMuted.withOpacity(0.4)),
          const SizedBox(height: 16),
          Text('No cameras configured', style: typo.pageTitle),
          const SizedBox(height: 6),
          Text(
            'Add cameras from the Devices tab to see their status here',
            style: typo.body,
            textAlign: TextAlign.center,
          ),
        ],
      ),
    );
  }
}
