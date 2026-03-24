import 'package:flutter/material.dart';

import '../models/notification_event.dart';
import '../theme/nvr_colors.dart';

/// Shows a Material 3 SnackBar for a [NotificationEvent].
///
/// Usage:
///   showNotificationSnackbar(context, event);
void showNotificationSnackbar(BuildContext context, NotificationEvent event) {
  final color = _colorForEvent(event);
  final icon = _iconForEvent(event);
  final title = _titleForEvent(event);

  ScaffoldMessenger.of(context).showSnackBar(
    SnackBar(
      behavior: SnackBarBehavior.floating,
      margin: const EdgeInsets.fromLTRB(12, 0, 12, 16),
      backgroundColor: NvrColors.bgSecondary,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(10),
        side: BorderSide(color: color.withValues(alpha: 0.5)),
      ),
      duration: const Duration(seconds: 4),
      content: Row(
        children: [
          Container(
            width: 4,
            height: 40,
            decoration: BoxDecoration(
              color: color,
              borderRadius: BorderRadius.circular(2),
            ),
          ),
          const SizedBox(width: 12),
          Icon(icon, color: color, size: 22),
          const SizedBox(width: 12),
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              mainAxisSize: MainAxisSize.min,
              children: [
                Text(
                  title,
                  style: const TextStyle(
                    color: NvrColors.textPrimary,
                    fontWeight: FontWeight.w600,
                    fontSize: 13,
                  ),
                ),
                const SizedBox(height: 2),
                Text(
                  '${event.camera}: ${event.message}',
                  style: const TextStyle(
                    color: NvrColors.textSecondary,
                    fontSize: 12,
                  ),
                  maxLines: 2,
                  overflow: TextOverflow.ellipsis,
                ),
              ],
            ),
          ),
        ],
      ),
    ),
  );
}

Color _colorForEvent(NotificationEvent event) {
  switch (event.type) {
    case 'camera_offline':
    case 'loitering':
      return NvrColors.danger;
    case 'camera_online':
      return NvrColors.success;
    default:
      return NvrColors.warning;
  }
}

IconData _iconForEvent(NotificationEvent event) {
  switch (event.type) {
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

String _titleForEvent(NotificationEvent event) {
  // If we have action + className, build a semantic title
  final action = event.action;
  final cls = event.className;
  if (action != null && cls != null) {
    final actionLabel = _actionLabel(action);
    final clsLabel = _capitalise(cls);
    return '$clsLabel $actionLabel';
  }
  if (action != null) return _capitalise(_actionLabel(action));
  switch (event.type) {
    case 'camera_offline':
      return 'Camera Offline';
    case 'camera_online':
      return 'Camera Online';
    case 'motion':
      return 'Motion Detected';
    case 'detection':
      return 'Object Detected';
    case 'loitering':
      return 'Loitering Alert';
    default:
      return 'Alert';
  }
}

String _actionLabel(String action) {
  switch (action.toLowerCase()) {
    case 'entered':
      return 'Entered';
    case 'left':
      return 'Left';
    case 'detected':
      return 'Detected';
    case 'loitering':
      return 'Loitering';
    default:
      return _capitalise(action);
  }
}

String _capitalise(String s) =>
    s.isEmpty ? s : '${s[0].toUpperCase()}${s.substring(1)}';
