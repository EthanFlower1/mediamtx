// KAI-303 — Opaque PushMessage value object.
//
// HIPAA + GDPR hard contract (cto + lead-security gate on PR #165):
//
//   Push notification payloads are observable by:
//     * the OS notification history,
//     * Apple/Google push infrastructure logs,
//     * lock-screen previews.
//
//   Therefore they MUST NOT carry ANY PII, ANY human-readable text, ANY
//   camera/person/location label, ANY timestamp, ANY kind, ANY thumbnail,
//   or ANY directory-connection id. A push payload carries EXACTLY three
//   fields:
//
//       event_id   (opaque, Directory-minted string)
//       tenant_id  (opaque tenant scope string)
//       priority   (numeric int: 0=low 1=normal 2=high 3=critical)
//
// Full event details are fetched from Directory AFTER the user taps, via
// an authenticated `GET /api/v1/events/<event_id>` call routed through
// [EventDetailsLoader]. The client stores NONE of this data in the push
// layer — it lives only in the NotificationTapDispatcher → EventDetails.
//
// See lead-security review on PR #165, cto security gate, and the proto
// ask for `cloud.directory.v1.Events.GetEvent` in the PR body.

/// Notification priority bucket. Drives only the deep-link route bucket
/// ("critical" vs "normal"), because the payload has no `kind` to branch on.
///
/// Intentionally coarse: fine-grained routing happens AFTER the client has
/// fetched the full [EventDetails] from Directory.
class PushPriority {
  PushPriority._();

  /// Low importance — the operator does not need to be disturbed.
  static const int low = 0;

  /// Normal importance — the default bucket for routine events.
  static const int normal = 1;

  /// High importance — surfaced to operators but not a panic alert.
  static const int high = 2;

  /// Critical — panic/duress/system-down. Routes to `/alerts/critical`.
  static const int critical = 3;
}

/// Opaque push payload. Metadata-only by hard contract.
///
/// HIPAA + GDPR: NO PII, NO camera/person/location, NO human-readable text
/// may live in this payload. Full event details are fetched from Directory
/// after the user taps, via authenticated GET /api/v1/events/<eventId>.
///
/// See lead-security review on PR #165 and cto security gate.
class PushMessage {
  /// Opaque event identifier, minted by Directory. The client re-fetches
  /// the full event record via an authenticated API call using this id
  /// on tap.
  final String eventId;

  /// Opaque tenant scope. The [NotificationService] REJECTS any resolve
  /// attempt whose tenantId does not match the active [AppSession] —
  /// preventing cross-tenant fetches if a stale device-token survives a
  /// session switch.
  final String tenantId;

  /// Priority bucket: 0=low 1=normal 2=high 3=critical. Drives the route
  /// returned by `routeForPushMessage`.
  final int priority;

  const PushMessage({
    required this.eventId,
    required this.tenantId,
    required this.priority,
  });

  /// Decode a push payload delivered by APNs / FCM / Web Push / Desktop.
  ///
  /// Defensive: this factory HARD-REJECTS any payload that carries keys
  /// outside the allowed set. A misbehaving dispatcher that tries to stuff
  /// `camera_id`, `kind`, `thumbnail_url`, a label, a location, or any PII
  /// into the payload will trigger a [PushPayloadViolation] here — and
  /// the platform channel will log + drop the message instead of crashing.
  ///
  /// This is the first line of defence for the metadata-only contract.
  factory PushMessage.fromRemote(Map<String, dynamic> data) {
    const allowed = {'event_id', 'tenant_id', 'priority'};
    final extras = data.keys.where((k) => !allowed.contains(k)).toList();
    if (extras.isNotEmpty) {
      throw PushPayloadViolation(
        'Disallowed keys in push payload: $extras. '
        'KAI-303 hard contract: metadata-only (event_id + tenant_id + priority).',
      );
    }

    final rawEventId = data['event_id'];
    final rawTenantId = data['tenant_id'];
    final rawPriority = data['priority'];
    if (rawEventId is! String || rawEventId.isEmpty) {
      throw PushPayloadViolation('event_id missing or not a non-empty string');
    }
    if (rawTenantId is! String || rawTenantId.isEmpty) {
      throw PushPayloadViolation('tenant_id missing or not a non-empty string');
    }
    if (rawPriority is! num) {
      throw PushPayloadViolation('priority missing or not numeric');
    }
    return PushMessage(
      eventId: rawEventId,
      tenantId: rawTenantId,
      priority: rawPriority.toInt(),
    );
  }

  /// Encode back to the same wire shape. Only the three allowed fields.
  Map<String, dynamic> toRemote() => {
        'event_id': eventId,
        'tenant_id': tenantId,
        'priority': priority,
      };

  @override
  String toString() => 'PushMessage(eventId=$eventId, tenantId=$tenantId, '
      'priority=$priority)';
}

/// Thrown when a push payload carries any field outside the
/// metadata-only contract (event_id + tenant_id + priority).
///
/// Platform channels catch this and LOG + DROP — never crash the app.
class PushPayloadViolation implements Exception {
  final String message;
  PushPayloadViolation(this.message);
  @override
  String toString() => 'PushPayloadViolation: $message';
}

/// Thrown by [NotificationService.resolveForTap] when the push message's
/// `tenantId` does not match the active [AppSession.tenantRef]. This is
/// the cross-tenant fetch guard — it prevents a stale push (e.g. from a
/// previous signed-in tenant) from fetching event details against the
/// current tenant's Directory.
class CrossTenantPushViolation implements Exception {
  final String message;
  CrossTenantPushViolation(this.message);
  @override
  String toString() => 'CrossTenantPushViolation: $message';
}
