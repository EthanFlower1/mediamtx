// KAI-303 — Deep-link route constants + dispatcher for push notifications.
//
// Scope is deliberately narrow: this file maps a [PushMessageKind] to a
// Route *name* (a string constant). The actual router wiring — i.e.
// navigator push / go_router call — is a follow-up, because the router
// shape is still being finalised in KAI-337 (operator workflow).
//
// Tests pin the kind -> route mapping so the follow-up can swap in real
// navigation without silently changing semantics.

import 'push_message.dart';

/// Route constants the deep-link dispatcher may return.
///
/// These are string identifiers, not go_router objects — the UI layer
/// translates them into actual navigation in a follow-up.
class NotificationRoutes {
  NotificationRoutes._();

  /// Live view + recent event panel for the camera referenced by the push.
  static const String liveEvent = '/live/event';

  /// Face-match detail screen with the event context.
  static const String faceEvent = '/faces/event';

  /// LPR detail screen with the event context.
  static const String lprEvent = '/lpr/event';

  /// Manual alert inbox (e.g. duress / panic alerts).
  static const String manualAlert = '/alerts/manual';

  /// System health / ops screen.
  static const String systemNotice = '/system/notices';
}

/// Deep-link dispatcher: given a [PushMessage], returns the route name the
/// UI layer should navigate to on tap.
///
/// Pure function — no side effects, no navigation. Making this testable in
/// isolation is the whole point.
String routeForPushMessage(PushMessage m) {
  return switch (m.kind) {
    PushMessageKind.motion => NotificationRoutes.liveEvent,
    PushMessageKind.face => NotificationRoutes.faceEvent,
    PushMessageKind.lpr => NotificationRoutes.lprEvent,
    PushMessageKind.manual => NotificationRoutes.manualAlert,
    PushMessageKind.system => NotificationRoutes.systemNotice,
  };
}
