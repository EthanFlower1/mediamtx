// KAI-303 — Deep-link dispatcher for opaque push notifications.
//
// The push payload carries ONLY (event_id, tenant_id, priority), so
// coarse routing uses priority alone. Fine-grained routing (by kind,
// camera, etc.) happens AFTER the client has fetched the full
// EventDetails from Directory on tap.
//
// This function is pure — no side effects, no navigation.

import 'push_message.dart';

/// Route constants the deep-link dispatcher may return.
class NotificationRoutes {
  NotificationRoutes._();

  /// High-visibility alerts screen for critical (priority=3) pushes:
  /// panic, duress, system-down.
  static const String criticalAlerts = '/alerts/critical';

  /// Default alerts inbox for low/normal/high pushes. Fine-grained
  /// routing (live view, face inspector, etc.) happens AFTER the client
  /// has loaded EventDetails from Directory.
  static const String alerts = '/alerts';
}

/// Deep-link dispatcher: given an opaque [PushMessage], returns the
/// route name the UI layer should navigate to on tap.
///
/// Since the payload has no `kind`, routing is by priority alone.
String routeForPushMessage(PushMessage m) {
  if (m.priority >= PushPriority.critical) {
    return NotificationRoutes.criticalAlerts;
  }
  return NotificationRoutes.alerts;
}
