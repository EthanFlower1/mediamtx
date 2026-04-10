// KAI-431 — Adapter: PbAIEvent ↔ EventSummary.
//
// Converts between the proto-generated AI event types and the app-layer
// EventSummary model used by the events screen.

import '../events/events_model.dart';
import '../src/gen/proto/kaivue/v1/directory_ingest.pb.dart';

/// Converts a proto [PbAIEvent] to the app-layer [EventSummary].
///
/// [tenantId] must be supplied by the caller (from the authenticated session)
/// because the proto event does not carry it — tenant isolation is enforced
/// at the transport layer.
///
/// [cameraNameLookup] resolves camera IDs to display names. Returns the
/// camera ID as fallback if the name is not found.
EventSummary eventSummaryFromProto(
  PbAIEvent pb, {
  required String tenantId,
  String Function(String cameraId)? cameraNameLookup,
}) {
  return EventSummary(
    id: pb.eventId,
    cameraId: pb.cameraId,
    cameraName:
        cameraNameLookup?.call(pb.cameraId) ?? pb.cameraId,
    kind: _mapEventKind(pb.kind, pb.kindLabel),
    timestamp: pb.observedAt ?? DateTime.now(),
    severity: _inferSeverity(pb.kind),
    tenantId: tenantId,
  );
}

/// Converts a list of proto AI events to EventSummary list.
List<EventSummary> eventSummariesFromProto(
  List<PbAIEvent> pbs, {
  required String tenantId,
  String Function(String cameraId)? cameraNameLookup,
}) =>
    pbs
        .map((pb) => eventSummaryFromProto(
              pb,
              tenantId: tenantId,
              cameraNameLookup: cameraNameLookup,
            ))
        .toList();

/// Maps proto AIEventKind to wire kind string.
String _mapEventKind(PbAIEventKind k, String kindLabel) {
  if (kindLabel.isNotEmpty) return kindLabel;
  switch (k) {
    case PbAIEventKind.motion:
      return 'motion';
    case PbAIEventKind.person:
      return 'person';
    case PbAIEventKind.vehicle:
      return 'vehicle';
    case PbAIEventKind.face:
      return 'face';
    case PbAIEventKind.licensePlate:
      return 'license_plate';
    case PbAIEventKind.audioAlarm:
      return 'audio_alarm';
    case PbAIEventKind.lineCrossing:
      return 'line_crossing';
    case PbAIEventKind.loitering:
      return 'loitering';
    case PbAIEventKind.tamper:
      return 'tamper';
    case PbAIEventKind.unspecified:
      return 'unknown';
  }
}

/// Infer severity from event kind.
EventSeverity _inferSeverity(PbAIEventKind k) {
  switch (k) {
    case PbAIEventKind.tamper:
    case PbAIEventKind.audioAlarm:
      return EventSeverity.critical;
    case PbAIEventKind.face:
    case PbAIEventKind.licensePlate:
    case PbAIEventKind.lineCrossing:
      return EventSeverity.warning;
    case PbAIEventKind.person:
    case PbAIEventKind.vehicle:
    case PbAIEventKind.loitering:
    case PbAIEventKind.motion:
    case PbAIEventKind.unspecified:
      return EventSeverity.info;
  }
}
