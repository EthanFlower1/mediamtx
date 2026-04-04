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
        color: NvrColors.bgSecondary,
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
      decoration: const BoxDecoration(
        color: NvrColors.bgSecondary,
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
        Container(height: 1, color: NvrColors.border),
        // List or empty state
        Expanded(
          child: history.isEmpty
              ? const _EmptyState()
              : ListView.separated(
                  padding: EdgeInsets.zero,
                  itemCount: history.length,
                  separatorBuilder: (_, __) =>
                      Container(height: 1, color: NvrColors.border),
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
          const Text('ALERTS', style: NvrTypography.monoSection),
          const SizedBox(width: 8),
          // Unread badge
          if (unreadCount > 0)
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
              decoration: BoxDecoration(
                color: NvrColors.accent,
                borderRadius: BorderRadius.circular(10),
              ),
              child: Text(
                '$unreadCount',
                style: const TextStyle(
                  fontFamily: 'JetBrainsMono',
                  fontSize: 9,
                  fontWeight: FontWeight.w700,
                  color: NvrColors.bgPrimary,
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
            child: const Icon(
              Icons.close,
              size: 18,
              color: NvrColors.textSecondary,
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

  Color get _iconColor {
    switch (event.type) {
      case 'motion':
        return NvrColors.accent;
      case 'camera_offline':
        return NvrColors.danger;
      case 'camera_online':
        return NvrColors.success;
      case 'alert':
        return NvrColors.warning;
      default:
        return NvrColors.textSecondary;
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
    final bg = event.isRead ? NvrColors.bgSecondary : NvrColors.bgTertiary;
    final hasRoute = event.navigationRoute != null;

    return Material(
      color: bg,
      child: InkWell(
        onTap: onTap,
        splashColor: NvrColors.accent.withValues(alpha: 0.08),
        highlightColor: NvrColors.accent.withValues(alpha: 0.04),
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
                  color: _iconColor,
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
                      style: NvrTypography.body.copyWith(
                        color: event.isRead
                            ? NvrColors.textSecondary
                            : NvrColors.textPrimary,
                      ),
                    ),
                    const SizedBox(height: 3),
                    Text(
                      '${event.camera} \u00b7 ${_timeAgo(event.time)}',
                      style: NvrTypography.monoLabel,
                    ),
                  ],
                ),
              ),
              // Navigate arrow for actionable notifications
              if (hasRoute && !event.isRead)
                const Padding(
                  padding: EdgeInsets.only(top: 2, left: 8),
                  child: Icon(
                    Icons.chevron_right,
                    size: 14,
                    color: NvrColors.textMuted,
                  ),
                )
              else if (!event.isRead)
                Padding(
                  padding: const EdgeInsets.only(top: 4, left: 8),
                  child: Container(
                    width: 5,
                    height: 5,
                    decoration: const BoxDecoration(
                      color: NvrColors.accent,
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
          const Icon(
            Icons.notifications_none,
            color: NvrColors.textMuted,
            size: 48,
          ),
          const SizedBox(height: 12),
          Text(
            'No alerts',
            style: NvrTypography.body.copyWith(color: NvrColors.textMuted),
          ),
        ],
      ),
    );
  }
}
