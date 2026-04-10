// KAI-303 — EventDetailsLoader: the ONLY legitimate path from an opaque
// PushMessage to user-visible event data.
//
// Security contract (cto + lead-security gate on PR #165):
//
//   A PushMessage carries nothing but event_id + tenant_id + priority.
//   When the user taps the notification, the NotificationService calls
//   [EventDetailsLoader.loadEvent] with the opaque eventId. The loader
//   issues an AUTHENTICATED call against Directory
//   (`GET /api/v1/events/<event_id>`) using the current AppSession's
//   bearer token. The response contains camera name, thumbnail URL,
//   kind, timestamp, etc. — the human-readable fields that are forbidden
//   in the push payload itself.
//
// This separation is what gives us HIPAA + GDPR safety:
//   * OS notification history never sees PII.
//   * Push infrastructure logs never see PII.
//   * Lock-screen previews never see PII.
//   * The sensitive fetch is gated by the current tenant's auth session.

import 'dart:async';
import 'dart:convert';

import 'package:http/http.dart' as http;

import '../state/app_session.dart';

/// Full event details, fetched from Directory on notification tap.
///
/// The field set will grow as `/api/v1/events/<id>` firms up with
/// lead-cloud. For now we ship the minimal set the foreground handler
/// needs to render a user-visible notification.
class EventDetails {
  /// Opaque event id — echoed back from the PushMessage for trace.
  final String eventId;

  /// Opaque camera id the event fired on.
  final String cameraId;

  /// Human-readable camera label (e.g. "Front Door"). PII — never in a
  /// push payload, always fetched here.
  final String cameraLabel;

  /// Kind: motion / face / lpr / manual / system. Client-side string
  /// because this value comes from Directory, not the push payload.
  final String kind;

  /// Server-assigned timestamp of the event.
  final DateTime timestamp;

  /// Short-lived signed URL for a thumbnail. MAY be null. The signing
  /// scheme is Directory's responsibility — the client should treat the
  /// URL as one-shot.
  final String? thumbnailUrl;

  const EventDetails({
    required this.eventId,
    required this.cameraId,
    required this.cameraLabel,
    required this.kind,
    required this.timestamp,
    this.thumbnailUrl,
  });

  factory EventDetails.fromJson(Map<String, dynamic> json) {
    return EventDetails(
      eventId: json['event_id'] as String,
      cameraId: json['camera_id'] as String,
      cameraLabel: json['camera_label'] as String? ?? '',
      kind: json['kind'] as String? ?? 'system',
      timestamp: DateTime.parse(json['timestamp'] as String),
      thumbnailUrl: json['thumbnail_url'] as String?,
    );
  }
}

/// Loads full event details from the Directory for a given opaque
/// [eventId], using the currently-active AppSession auth headers.
///
/// This is the ONLY legitimate path from a PushMessage to user-visible
/// event data. KAI-303 security contract.
abstract class EventDetailsLoader {
  Future<EventDetails> loadEvent(String eventId);
}

/// HTTP-backed implementation. Takes an [AppSession] (for the bearer
/// token and directory base URL) and a `http.Client` (injectable for
/// tests).
///
/// TODO(KAI-303-followup): the canonical proto for this call is
/// `GetEvent(event_id) -> EventDetails` in `cloud.directory.v1.Events`,
/// waiting on lead-cloud. Until then we hand-roll the REST call against
/// `GET /api/v1/events/<id>`.
class HttpEventDetailsLoader implements EventDetailsLoader {
  final AppSession session;
  final http.Client httpClient;

  HttpEventDetailsLoader({
    required this.session,
    required this.httpClient,
  });

  @override
  Future<EventDetails> loadEvent(String eventId) async {
    final baseUrl = session.activeConnection?.endpointUrl;
    final token = session.accessToken;
    if (baseUrl == null || token == null || token.isEmpty) {
      throw StateError(
        'HttpEventDetailsLoader: no active AppSession connection — cannot '
        'fetch event details without an authenticated directory.',
      );
    }
    final uri = Uri.parse('$baseUrl/api/v1/events/$eventId');
    final resp = await httpClient.get(
      uri,
      headers: {
        'Authorization': 'Bearer $token',
        'Accept': 'application/json',
      },
    );
    if (resp.statusCode != 200) {
      throw StateError(
        'HttpEventDetailsLoader: GET $uri returned ${resp.statusCode}',
      );
    }
    final json = jsonDecode(resp.body) as Map<String, dynamic>;
    return EventDetails.fromJson(json);
  }
}

/// Fake implementation for tests — preload a map of eventId -> details.
class FakeEventDetailsLoader implements EventDetailsLoader {
  final Map<String, EventDetails> events;
  final List<String> loadedEventIds = [];
  Exception? errorToThrow;

  FakeEventDetailsLoader({Map<String, EventDetails>? events})
      : events = events ?? {};

  @override
  Future<EventDetails> loadEvent(String eventId) async {
    loadedEventIds.add(eventId);
    if (errorToThrow != null) throw errorToThrow!;
    final e = events[eventId];
    if (e == null) {
      throw StateError('FakeEventDetailsLoader: no event registered for '
          '"$eventId"');
    }
    return e;
  }
}
