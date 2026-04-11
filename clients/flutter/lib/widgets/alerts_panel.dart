import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../theme/nvr_colors.dart';
import '../theme/nvr_typography.dart';
import '../providers/notifications_provider.dart';
import '../models/notification_event.dart';
import 'hud/hud_button.dart';

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

/// Show the alerts panel as the appropriate surface for the current breakpoint.
///
/// Desktop (width >= 600): managed via [AlertsPanelOverlay] in the navigation
/// shell's Stack.  Call this to toggle open/close state via the provider.
///
/// Mobile (width < 600): shown as a modal bottom sheet.
void showAlertsPanel(BuildContext context, WidgetRef ref) {
  final width = MediaQuery.of(context).size.width;
  if (width < 600) {
    showModalBottomSheet<void>(
      context: context,
      isScrollControlled: true,
      backgroundColor: Colors.transparent,
      builder: (_) => const _AlertsPanelSheet(),
    );
  } else {
    ref.read(alertsPanelOpenProvider.notifier).state = true;
  }
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------

/// Whether the desktop overlay panel is open.
final alertsPanelOpenProvider = StateProvider<bool>((ref) => false);

// ---------------------------------------------------------------------------
// Desktop overlay widget — embed in a Stack, positioned to the right.
// ---------------------------------------------------------------------------

class AlertsPanelOverlay extends ConsumerWidget {
  const AlertsPanelOverlay({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final isOpen = ref.watch(alertsPanelOpenProvider);

    return AnimatedPositioned(
      duration: const Duration(milliseconds: 220),
      curve: Curves.easeInOut,
      top: 0,
      bottom: 0,
      right: isOpen ? 0 : -304, // 300 panel + 4 shadow bleed
      width: 300,
      child: Material(
        color: NvrColors.of(context).bgSecondary,
        elevation: 8,
        child: _AlertsPanelContent(
          onClose: () => ref.read(alertsPanelOpenProvider.notifier).state = false,
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Mobile bottom sheet wrapper
// ---------------------------------------------------------------------------

class _AlertsPanelSheet extends ConsumerWidget {
  const _AlertsPanelSheet();

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final screenHeight = MediaQuery.of(context).size.height;
    return Container(
      height: screenHeight * 0.75,
      decoration: BoxDecoration(
        color: NvrColors.of(context).bgSecondary,
        borderRadius: BorderRadius.vertical(top: Radius.circular(12)),
      ),
      child: _AlertsPanelContent(
        onClose: () => Navigator.of(context).pop(),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Shared panel content
// ---------------------------------------------------------------------------

class _AlertsPanelContent extends ConsumerWidget {
  const _AlertsPanelContent({required this.onClose});

  final VoidCallback onClose;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final notifState = ref.watch(notificationsProvider);
    final notifier = ref.read(notificationsProvider.notifier);
    final history = notifState.history;
    final unread = notifState.unreadCount;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        // Header
        _PanelHeader(
          unreadCount: unread,
          onMarkAllRead: notifier.markAllRead,
          onClose: onClose,
        ),
        Container(height: 1, color: NvrColors.of(context).border),
        // List or empty state
        Expanded(
          child: history.isEmpty
              ? const _EmptyState()
              : ListView.separated(
                  padding: EdgeInsets.zero,
                  itemCount: history.length,
                  separatorBuilder: (_, __) =>
                      Container(height: 1, color: NvrColors.of(context).border),
                  itemBuilder: (context, index) {
                    final event = history[index];
                    return _NotificationItem(
                      event: event,
                      onTap: () {
                        notifier.markRead(index);
                        final route = event.navigationRoute;
                        if (route != null) {
                          onClose();
                          context.go(route);
                        }
                      },
                    );
                  },
                ),
        ),
        // View All Notifications link
        Container(
          decoration: BoxDecoration(
            border: Border(top: BorderSide(color: NvrColors.of(context).border)),
          ),
          child: InkWell(
            onTap: () {
              onClose();
              context.go('/notifications');
            },
            child: Padding(
              padding: const EdgeInsets.symmetric(vertical: 12),
              child: Center(
                child: Text(
                  'View all notifications',
                  style: TextStyle(
                    fontSize: 13,
                    fontWeight: FontWeight.w500,
                    color: NvrColors.of(context).accent,
                  ),
                ),
              ),
            ),
          ),
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// Header
// ---------------------------------------------------------------------------

class _PanelHeader extends StatelessWidget {
  const _PanelHeader({
    required this.unreadCount,
    required this.onMarkAllRead,
    required this.onClose,
  });

  final int unreadCount;
  final VoidCallback onMarkAllRead;
  final VoidCallback onClose;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
      child: Row(
        children: [
          // Title
          Text('ALERTS', style: NvrTypography.of(context).monoSection),
          const SizedBox(width: 8),
          // Unread badge
          if (unreadCount > 0)
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
              decoration: BoxDecoration(
                color: NvrColors.of(context).accent,
                borderRadius: BorderRadius.circular(10),
              ),
              child: Text(
                '$unreadCount',
                style: TextStyle(
                  fontFamily: 'JetBrainsMono',
                  fontSize: 9,
                  fontWeight: FontWeight.w700,
                  color: NvrColors.of(context).bgPrimary,
                ),
              ),
            ),
          const Spacer(),
          // Mark all read button
          HudButton(
            label: 'MARK ALL READ',
            onPressed: unreadCount > 0 ? onMarkAllRead : null,
            style: HudButtonStyle.tactical,
          ),
          const SizedBox(width: 8),
          // Close button
          GestureDetector(
            onTap: onClose,
            child: Icon(
              Icons.close,
              size: 18,
              color: NvrColors.of(context).textSecondary,
            ),
          ),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Notification item
// ---------------------------------------------------------------------------

class _NotificationItem extends StatelessWidget {
  const _NotificationItem({required this.event, required this.onTap});

  final NotificationEvent event;
  final VoidCallback onTap;

  Color _iconColor(BuildContext context) {
    switch (event.type) {
      case 'motion':
        return NvrColors.of(context).accent;
      case 'camera_offline':
        return NvrColors.of(context).danger;
      case 'camera_online':
        return NvrColors.of(context).success;
      case 'alert':
        return NvrColors.of(context).warning;
      default:
        return NvrColors.of(context).textSecondary;
    }
  }

  String _timeAgo(DateTime time) {
    final diff = DateTime.now().difference(time);
    if (diff.inSeconds < 60) return '${diff.inSeconds}s ago';
    if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
    if (diff.inHours < 24) return '${diff.inHours}h ago';
    return '${diff.inDays}d ago';
  }

  @override
  Widget build(BuildContext context) {
    final bg = event.isRead ? NvrColors.of(context).bgSecondary : NvrColors.of(context).bgTertiary;
    final hasRoute = event.navigationRoute != null;

    return Material(
      color: bg,
      child: InkWell(
        onTap: onTap,
        splashColor: NvrColors.of(context).accent.withValues(alpha: 0.08),
        highlightColor: NvrColors.of(context).accent.withValues(alpha: 0.04),
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
          child: Row(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Type icon
              Padding(
                padding: const EdgeInsets.only(top: 1),
                child: Icon(
                  event.typeIcon,
                  size: 16,
                  color: _iconColor(context),
                ),
              ),
              const SizedBox(width: 10),
              // Message + meta
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      event.message,
                      style: NvrTypography.of(context).body.copyWith(
                        color: event.isRead
                            ? NvrColors.of(context).textSecondary
                            : NvrColors.of(context).textPrimary,
                      ),
                    ),
                    const SizedBox(height: 3),
                    Text(
                      '${event.camera} \u00b7 ${_timeAgo(event.time)}',
                      style: NvrTypography.of(context).monoLabel,
                    ),
                  ],
                ),
              ),
              // Navigate arrow for actionable notifications
              if (hasRoute && !event.isRead)
                Padding(
                  padding: const EdgeInsets.only(top: 2, left: 8),
                  child: Icon(
                    Icons.chevron_right,
                    size: 14,
                    color: NvrColors.of(context).textMuted,
                  ),
                )
              else if (!event.isRead)
                Padding(
                  padding: const EdgeInsets.only(top: 4, left: 8),
                  child: Container(
                    width: 5,
                    height: 5,
                    decoration: BoxDecoration(
                      color: NvrColors.of(context).accent,
                      shape: BoxShape.circle,
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Empty state
// ---------------------------------------------------------------------------

class _EmptyState extends StatelessWidget {
  const _EmptyState();

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(
            Icons.notifications_none,
            color: NvrColors.of(context).textMuted,
            size: 48,
          ),
          const SizedBox(height: 12),
          Text(
            'No alerts',
            style: NvrTypography.of(context).body.copyWith(color: NvrColors.of(context).textMuted),
          ),
        ],
      ),
    );
  }
}
