// KAI-303 — PushEventKind: subscription-side event kind enum.
//
// IMPORTANT: this enum is used ONLY by the PushSubscriptionClient to tell
// the backend WHICH event kinds the user wants pushes for on a given
// camera. It is NOT part of the push payload — the push payload is opaque
// (see push_message.dart). Kind is resolved CLIENT-SIDE after the
// EventDetailsLoader fetches the full event from Directory on tap.
//
// Keeping this enum in a separate file from PushMessage prevents the
// mistake of using `kind` as a property of an incoming push.

/// Kinds of events a user may subscribe to for a given camera.
enum PushEventKind {
  motion,
  face,
  lpr,
  manual,
  system,
}

extension PushEventKindWire on PushEventKind {
  /// Stable wire name. Matches what the Directory subscription API expects.
  String get wire => switch (this) {
        PushEventKind.motion => 'motion',
        PushEventKind.face => 'face',
        PushEventKind.lpr => 'lpr',
        PushEventKind.manual => 'manual',
        PushEventKind.system => 'system',
      };

  static PushEventKind fromWire(String s) {
    return switch (s) {
      'motion' => PushEventKind.motion,
      'face' => PushEventKind.face,
      'lpr' => PushEventKind.lpr,
      'manual' => PushEventKind.manual,
      'system' => PushEventKind.system,
      _ => throw ArgumentError('Unknown PushEventKind wire value: $s'),
    };
  }
}
