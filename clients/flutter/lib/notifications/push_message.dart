// KAI-303 — PushMessage value object.
//
// Metadata-only contract. The hard rule from the ticket is:
//
//     Push payloads must NEVER contain raw image bytes or PII beyond an
//     opaque event identifier. A thumbnail, if shown, is fetched ON-TAP via
//     an authenticated Directory API call — never delivered in the push
//     payload itself.
//
// We enforce the "no raw binary" half of that contract in the constructor:
// every field is either a short opaque string or a DateTime, and the
// `thumbnailUrl` is validated to be an `https://` URL, not a `data:` URI.
//
// The "no PII" half of the contract is a semantic concern that can't be
// enforced by Dart — it is documented in the PR body and in the
// push_subscription_client contract that lead-cloud will implement against.

import 'dart:typed_data';

/// Kinds of events that may produce a push notification.
///
/// Ordering matters only in tests; do not rely on index values in the wire
/// protocol — the PushSubscriptionClient sends these as strings.
enum PushMessageKind {
  motion,
  face,
  lpr,
  manual,
  system,
}

extension PushMessageKindWire on PushMessageKind {
  /// Stable wire name. Matches what the backend dispatcher will emit.
  String get wire => switch (this) {
        PushMessageKind.motion => 'motion',
        PushMessageKind.face => 'face',
        PushMessageKind.lpr => 'lpr',
        PushMessageKind.manual => 'manual',
        PushMessageKind.system => 'system',
      };

  static PushMessageKind fromWire(String s) {
    return switch (s) {
      'motion' => PushMessageKind.motion,
      'face' => PushMessageKind.face,
      'lpr' => PushMessageKind.lpr,
      'manual' => PushMessageKind.manual,
      'system' => PushMessageKind.system,
      _ => throw ArgumentError('Unknown PushMessageKind wire value: $s'),
    };
  }
}

/// Metadata-only push payload produced by the cloud dispatcher (lead-cloud)
/// and received by a [PushChannel] implementation.
///
/// Construction is validating: any attempt to pass raw binary or a `data:`
/// URI throws an [ArgumentError]. This is the first line of defence for the
/// metadata-only contract.
class PushMessage {
  /// Opaque event identifier. The client re-fetches the full event record
  /// via an authenticated API call using this id on tap.
  final String eventId;

  /// Opaque camera identifier the event belongs to. Used for deep-linking.
  final String cameraId;

  /// Kind of event — drives the deep-link dispatcher and the localised
  /// notification title (see [NotificationStrings]).
  final PushMessageKind kind;

  /// Server-assigned timestamp. Used for sort order in the in-app feed when
  /// multiple pushes arrive out of order.
  final DateTime timestamp;

  /// Which HomeDirectoryConnection this event belongs to. Clients ignore
  /// pushes whose `directoryConnectionId` does not match the active session.
  final String directoryConnectionId;

  /// Optional authenticated URL the UI layer may fetch on tap. MUST be
  /// `https://` — `data:` URIs are rejected, because an embedded thumbnail
  /// would violate the metadata-only contract.
  final String? thumbnailUrl;

  PushMessage({
    required this.eventId,
    required this.cameraId,
    required this.kind,
    required this.timestamp,
    required this.directoryConnectionId,
    this.thumbnailUrl,
  }) {
    if (eventId.isEmpty) {
      throw ArgumentError.value(eventId, 'eventId', 'must not be empty');
    }
    if (cameraId.isEmpty) {
      throw ArgumentError.value(cameraId, 'cameraId', 'must not be empty');
    }
    if (directoryConnectionId.isEmpty) {
      throw ArgumentError.value(
        directoryConnectionId,
        'directoryConnectionId',
        'must not be empty',
      );
    }
    final url = thumbnailUrl;
    if (url != null) {
      if (url.startsWith('data:')) {
        throw ArgumentError.value(
          url,
          'thumbnailUrl',
          'data: URIs are forbidden — metadata-only payload contract '
              'requires thumbnails to be fetched on-tap via an authenticated '
              'API call, not delivered inline in the push payload.',
        );
      }
      if (!url.startsWith('https://')) {
        throw ArgumentError.value(
          url,
          'thumbnailUrl',
          'must be an https:// URL',
        );
      }
    }
  }

  /// Factory that hard-rejects any attempt to stuff raw binary into a push
  /// message. This is the guardrail referenced by the PR body and by
  /// `notification_service_test.dart`.
  ///
  /// Callers should normally use the unnamed constructor — this factory
  /// exists so tests and (mis)behaving integrators can prove the guardrail
  /// actually fires.
  factory PushMessage.rejectBinary({
    required String eventId,
    required String cameraId,
    required PushMessageKind kind,
    required DateTime timestamp,
    required String directoryConnectionId,
    String? thumbnailUrl,
    Object? embeddedImage,
  }) {
    if (embeddedImage != null) {
      // Covers every path an integrator might try to stuff bytes through.
      if (embeddedImage is Uint8List ||
          embeddedImage is List<int> ||
          embeddedImage is ByteBuffer ||
          embeddedImage is ByteData) {
        throw ArgumentError.value(
          embeddedImage.runtimeType,
          'embeddedImage',
          'Raw binary payloads are forbidden by the metadata-only contract. '
              'Deliver an https:// thumbnailUrl instead and let the client '
              'fetch it on tap via an authenticated API call.',
        );
      }
    }
    return PushMessage(
      eventId: eventId,
      cameraId: cameraId,
      kind: kind,
      timestamp: timestamp,
      directoryConnectionId: directoryConnectionId,
      thumbnailUrl: thumbnailUrl,
    );
  }

  /// Wire decoder for platform channels that deliver payloads as a
  /// `Map<String, dynamic>` from APNs / FCM / WebPush native layers.
  factory PushMessage.fromWire(Map<String, dynamic> m) {
    return PushMessage(
      eventId: m['event_id'] as String,
      cameraId: m['camera_id'] as String,
      kind: PushMessageKindWire.fromWire(m['kind'] as String),
      timestamp: DateTime.parse(m['timestamp'] as String),
      directoryConnectionId: m['directory_connection_id'] as String,
      thumbnailUrl: m['thumbnail_url'] as String?,
    );
  }

  Map<String, dynamic> toWire() => {
        'event_id': eventId,
        'camera_id': cameraId,
        'kind': kind.wire,
        'timestamp': timestamp.toUtc().toIso8601String(),
        'directory_connection_id': directoryConnectionId,
        if (thumbnailUrl != null) 'thumbnail_url': thumbnailUrl,
      };

  @override
  String toString() => 'PushMessage($eventId/$cameraId/${kind.wire})';
}
