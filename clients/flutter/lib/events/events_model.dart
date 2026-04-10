// KAI-312 — Events domain model.
//
// Data shapes for the alerts/events list surface. Intentionally hand-written
// (no codegen) so the screen can land without a `build_runner` step — matches
// the style already used in `lib/state/`.
//
// Cross-tenant invariant: every [EventSummary] carries its own `tenantId`,
// and `EventsListPage` MUST reject rows whose `tenantId` does not match
// `AppSession.tenantRef`. See `events_list_page.dart` for the guard.
//
// Proto-first seam (follow-up): these types mirror the planned
//   service cloud.directory.v1.Events {
//     rpc List(ListEventsRequest) returns (stream EventSummary);
//   }
// where tenant_id is implicit from the authenticated session. Field names are
// snake_case-compatible via [toJson] / [fromJson].

import 'package:flutter/foundation.dart';

/// Severity ladder for an alert/event row. Ordered low to high.
enum EventSeverity {
  info,
  warning,
  critical,
}

String eventSeverityToWire(EventSeverity s) {
  switch (s) {
    case EventSeverity.info:
      return 'info';
    case EventSeverity.warning:
      return 'warning';
    case EventSeverity.critical:
      return 'critical';
  }
}

EventSeverity eventSeverityFromWire(String s) {
  switch (s) {
    case 'info':
      return EventSeverity.info;
    case 'warning':
      return EventSeverity.warning;
    case 'critical':
      return EventSeverity.critical;
    default:
      throw ArgumentError('Unknown EventSeverity: $s');
  }
}

/// Relative time window for the filter sheet. `custom` carries an explicit
/// [DateTimeRange] on [EventFilter.customRange]; the other values are computed
/// against `now` at query time.
enum EventTimeRange {
  today,
  last7d,
  last30d,
  custom,
}

/// A single row in the events list. Rendered by [EventsListPage] and used as
/// the navigation argument to the event detail page.
///
/// `tenantId` is the source of truth for the cross-tenant guard — it is sent
/// by the server as part of the row and the client re-checks it against the
/// active session's tenant. A mismatch drops the row and logs a
/// `CrossTenantEventViolation` warning (defense-in-depth).
@immutable
class EventSummary {
  final String id;
  final DateTime timestamp;
  final String cameraId;
  final String cameraName;

  /// Opaque server-defined event kind, e.g. `motion`, `person_detected`,
  /// `offline`, `tamper`. Rendered as a short badge; not enum'd client-side
  /// because new kinds must not break old clients.
  final String kind;

  final EventSeverity severity;

  /// Tenant ref this event belongs to. MUST equal `AppSession.tenantRef` at
  /// render time — see the guard in [EventsListPage].
  final String tenantId;

  const EventSummary({
    required this.id,
    required this.timestamp,
    required this.cameraId,
    required this.cameraName,
    required this.kind,
    required this.severity,
    required this.tenantId,
  });

  Map<String, dynamic> toJson() => {
        'id': id,
        'timestamp': timestamp.toUtc().toIso8601String(),
        'camera_id': cameraId,
        'camera_name': cameraName,
        'kind': kind,
        'severity': eventSeverityToWire(severity),
        'tenant_id': tenantId,
      };

  factory EventSummary.fromJson(Map<String, dynamic> json) {
    return EventSummary(
      id: json['id'] as String,
      timestamp: DateTime.parse(json['timestamp'] as String),
      cameraId: json['camera_id'] as String,
      cameraName: json['camera_name'] as String,
      kind: json['kind'] as String,
      severity: eventSeverityFromWire(json['severity'] as String),
      tenantId: json['tenant_id'] as String,
    );
  }

  @override
  bool operator ==(Object other) {
    if (identical(this, other)) return true;
    return other is EventSummary &&
        other.id == id &&
        other.timestamp == timestamp &&
        other.cameraId == cameraId &&
        other.cameraName == cameraName &&
        other.kind == kind &&
        other.severity == severity &&
        other.tenantId == tenantId;
  }

  @override
  int get hashCode => Object.hash(
        id,
        timestamp,
        cameraId,
        cameraName,
        kind,
        severity,
        tenantId,
      );
}

/// A single page of results returned by [EventsClient.list]. `nextCursor` is
/// `null` when the end of the stream is reached.
@immutable
class EventsPage {
  final List<EventSummary> items;
  final String? nextCursor;

  const EventsPage({required this.items, this.nextCursor});

  bool get hasMore => nextCursor != null;
}

/// Filter settings for the events query. Empty [severities] / [cameraIds]
/// mean "all". [timeRange] is always set; `custom` additionally uses
/// [customRange].
@immutable
class EventFilter {
  final Set<EventSeverity> severities;
  final Set<String> cameraIds;
  final EventTimeRange timeRange;
  final DateTimeRange? customRange;

  const EventFilter({
    this.severities = const {},
    this.cameraIds = const {},
    this.timeRange = EventTimeRange.last7d,
    this.customRange,
  });

  EventFilter copyWith({
    Set<EventSeverity>? severities,
    Set<String>? cameraIds,
    EventTimeRange? timeRange,
    DateTimeRange? customRange,
    bool clearCustomRange = false,
  }) {
    return EventFilter(
      severities: severities ?? this.severities,
      cameraIds: cameraIds ?? this.cameraIds,
      timeRange: timeRange ?? this.timeRange,
      customRange:
          clearCustomRange ? null : (customRange ?? this.customRange),
    );
  }

  bool get isEmpty =>
      severities.isEmpty &&
      cameraIds.isEmpty &&
      timeRange == EventTimeRange.last7d &&
      customRange == null;

  @override
  bool operator ==(Object other) {
    if (identical(this, other)) return true;
    return other is EventFilter &&
        setEquals(other.severities, severities) &&
        setEquals(other.cameraIds, cameraIds) &&
        other.timeRange == timeRange &&
        other.customRange == customRange;
  }

  @override
  int get hashCode => Object.hash(
        Object.hashAllUnordered(severities),
        Object.hashAllUnordered(cameraIds),
        timeRange,
        customRange,
      );
}

/// Minimal [DateTimeRange] shim. Flutter's `material.dart` ships one, but we
/// avoid importing it into the model layer so pure-Dart tests can run the
/// model without pulling the Flutter SDK for value equality.
@immutable
class DateTimeRange {
  final DateTime start;
  final DateTime end;

  const DateTimeRange({required this.start, required this.end});

  @override
  bool operator ==(Object other) =>
      other is DateTimeRange && other.start == start && other.end == end;

  @override
  int get hashCode => Object.hash(start, end);
}
