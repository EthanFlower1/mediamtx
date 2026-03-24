import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/notification_event.dart';
import '../providers/notifications_provider.dart';
import '../theme/nvr_colors.dart';

class NotificationBell extends ConsumerWidget {
  const NotificationBell({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final notifState = ref.watch(notificationsProvider);
    final unread = notifState.unreadCount;
    final wsConnected = notifState.wsConnected;

    return IconButton(
      tooltip: wsConnected ? 'Notifications' : 'Notifications (disconnected)',
      icon: Stack(
        clipBehavior: Clip.none,
        children: [
          Icon(
            wsConnected ? Icons.notifications : Icons.notifications_off,
            color: wsConnected ? NvrColors.textPrimary : NvrColors.textMuted,
          ),
          if (unread > 0)
            Positioned(
              top: -4,
              right: -4,
              child: Container(
                padding: const EdgeInsets.symmetric(horizontal: 4, vertical: 1),
                constraints: const BoxConstraints(minWidth: 16, minHeight: 16),
                decoration: BoxDecoration(
                  color: NvrColors.danger,
                  borderRadius: BorderRadius.circular(8),
                ),
                child: Text(
                  unread > 99 ? '99+' : unread.toString(),
                  style: const TextStyle(
                    color: Colors.white,
                    fontSize: 10,
                    fontWeight: FontWeight.bold,
                    height: 1.2,
                  ),
                  textAlign: TextAlign.center,
                ),
              ),
            ),
        ],
      ),
      onPressed: () => _showNotificationSheet(context, ref),
    );
  }

  void _showNotificationSheet(BuildContext context, WidgetRef ref) {
    // Mark all read when the sheet opens
    ref.read(notificationsProvider.notifier).markAllRead();

    showModalBottomSheet<void>(
      context: context,
      backgroundColor: NvrColors.bgSecondary,
      shape: const RoundedRectangleBorder(
        borderRadius: BorderRadius.vertical(top: Radius.circular(16)),
      ),
      isScrollControlled: true,
      builder: (ctx) => _NotificationSheet(parentRef: ref),
    );
  }
}

class _NotificationSheet extends ConsumerWidget {
  final WidgetRef parentRef;

  const _NotificationSheet({required this.parentRef});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final history = ref.watch(
      notificationsProvider.select((s) => s.history.take(50).toList()),
    );

    return DraggableScrollableSheet(
      expand: false,
      initialChildSize: 0.5,
      minChildSize: 0.25,
      maxChildSize: 0.9,
      builder: (context, scrollController) {
        return Column(
          children: [
            // Handle
            Padding(
              padding: const EdgeInsets.symmetric(vertical: 12),
              child: Container(
                width: 40,
                height: 4,
                decoration: BoxDecoration(
                  color: NvrColors.bgTertiary,
                  borderRadius: BorderRadius.circular(2),
                ),
              ),
            ),
            Padding(
              padding: const EdgeInsets.fromLTRB(16, 0, 16, 12),
              child: Row(
                children: [
                  const Text(
                    'Notifications',
                    style: TextStyle(
                      color: NvrColors.textPrimary,
                      fontSize: 16,
                      fontWeight: FontWeight.w600,
                    ),
                  ),
                  const Spacer(),
                  if (history.isNotEmpty)
                    TextButton(
                      onPressed: () {
                        ref.read(notificationsProvider.notifier).markAllRead();
                      },
                      child: const Text(
                        'Mark all read',
                        style: TextStyle(color: NvrColors.accent),
                      ),
                    ),
                ],
              ),
            ),
            const Divider(color: NvrColors.border, height: 1),
            Expanded(
              child: history.isEmpty
                  ? const Center(
                      child: Text(
                        'No notifications yet',
                        style: TextStyle(color: NvrColors.textMuted),
                      ),
                    )
                  : ListView.separated(
                      controller: scrollController,
                      itemCount: history.length,
                      separatorBuilder: (_, __) =>
                          const Divider(color: NvrColors.border, height: 1),
                      itemBuilder: (context, index) {
                        return _NotificationItem(event: history[index]);
                      },
                    ),
            ),
          ],
        );
      },
    );
  }
}

class _NotificationItem extends StatelessWidget {
  final NotificationEvent event;

  const _NotificationItem({required this.event});

  IconData _iconFor(String type) {
    switch (type) {
      case 'camera_offline':
        return Icons.videocam_off;
      case 'camera_online':
        return Icons.videocam;
      case 'motion':
        return Icons.motion_photos_on;
      case 'detection':
        return Icons.person_search;
      case 'loitering':
        return Icons.watch_later;
      default:
        return Icons.notifications;
    }
  }

  Color _colorFor(String type) {
    switch (type) {
      case 'camera_offline':
      case 'loitering':
        return NvrColors.danger;
      case 'camera_online':
        return NvrColors.success;
      default:
        return NvrColors.warning;
    }
  }

  String _relativeTime(DateTime time) {
    final diff = DateTime.now().difference(time);
    if (diff.inSeconds < 60) return '${diff.inSeconds}s ago';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    return '${diff.inDays}d ago';
  }

  @override
  Widget build(BuildContext context) {
    final color = _colorFor(event.type);
    return ListTile(
      leading: CircleAvatar(
        backgroundColor: color.withValues(alpha: 0.15),
        child: Icon(_iconFor(event.type), color: color, size: 20),
      ),
      title: Text(
        event.message,
        style: const TextStyle(
          color: NvrColors.textPrimary,
          fontSize: 13,
        ),
        maxLines: 2,
        overflow: TextOverflow.ellipsis,
      ),
      subtitle: Text(
        '${event.camera}  ·  ${_relativeTime(event.time)}',
        style: const TextStyle(color: NvrColors.textMuted, fontSize: 11),
      ),
      dense: true,
    );
  }
}
