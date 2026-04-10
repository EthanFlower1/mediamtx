// KAI-303 — Push subscription client interface.
//
// PROTO-FIRST SEAM — do not add an HTTP implementation in this PR.
//
// The backend contract for push subscriptions is a lead-cloud deliverable.
// This file defines the Dart-side *interface* the UI and NotificationService
// code against, plus a fake implementation used in tests. The real wire
// implementation must land alongside a proto file at:
//
//     pkg/api/v1/notifications.proto
//
// The RPCs lead-cloud owes us (names are proposed — final names are
// lead-cloud's call):
//
//   * RegisterDevice(deviceToken, platform, directoryConnectionId)
//       -> subscriptionId
//
//   * Subscribe(subscriptionId, cameraId, eventKinds[]) -> void
//
//   * Unsubscribe(subscriptionId, cameraId) -> void
//
//   * ListSubscriptions(subscriptionId)
//       -> repeated PushSubscription
//
//   * DeleteDevice(subscriptionId) -> void     // on explicit logout
//
// Dispatch semantics: the server is expected to apply the metadata-only
// contract at the dispatch layer — never include image bytes or PII in the
// FCM/APNs/WebPush payload. Clients also enforce the contract at receive
// time (see PushMessage).

import 'push_event_kind.dart';

/// A single per-camera subscription record owned by the directory backend.
class PushSubscription {
  /// Opaque backend-assigned id for the subscription row.
  final String id;

  /// Opaque camera this subscription targets.
  final String cameraId;

  /// Which kinds of events the user wants to be notified about on this
  /// camera. An empty set means "no pushes" and is equivalent to an
  /// unsubscribe.
  final Set<PushEventKind> eventKinds;

  const PushSubscription({
    required this.id,
    required this.cameraId,
    required this.eventKinds,
  });

  PushSubscription copyWith({
    String? id,
    String? cameraId,
    Set<PushEventKind>? eventKinds,
  }) {
    return PushSubscription(
      id: id ?? this.id,
      cameraId: cameraId ?? this.cameraId,
      eventKinds: eventKinds ?? this.eventKinds,
    );
  }

  @override
  String toString() =>
      'PushSubscription($id, camera=$cameraId, kinds=${eventKinds.map((e) => e.wire).join(",")})';
}

/// Abstract push subscription client.
///
/// The real implementation is a lead-cloud deliverable that wires these
/// methods to the notifications.proto RPCs listed at the top of this file.
/// Do NOT write a provisional HTTP implementation — the proto-first seam
/// must be respected.
abstract class PushSubscriptionClient {
  /// Register the current device token for the active directory. Returns
  /// an opaque subscription id that the caller must remember for
  /// subsequent [subscribe] / [unsubscribe] calls.
  Future<String> registerDevice({
    required String deviceToken,
    required String platform,
    required String directoryConnectionId,
  });

  /// Declare interest in events of the given [eventKinds] on [cameraId].
  Future<void> subscribe({
    required String subscriptionId,
    required String cameraId,
    required Set<PushEventKind> eventKinds,
  });

  /// Cancel interest in events from [cameraId].
  Future<void> unsubscribe({
    required String subscriptionId,
    required String cameraId,
  });

  /// List all subscriptions currently held by [subscriptionId].
  Future<List<PushSubscription>> listSubscriptions(String subscriptionId);

  /// Forget the device entirely. Called on explicit logout.
  Future<void> deleteDevice(String subscriptionId);
}
