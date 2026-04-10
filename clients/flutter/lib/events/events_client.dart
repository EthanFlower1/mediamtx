// KAI-312 — EventsClient interface + in-memory fake.
//
// This file defines the client-facing contract for "list events". It does NOT
// bind to any HTTP/Connect-Go shape — that's a follow-up once the
// `cloud.directory.v1.Events` proto lands. See the PR body for the proto ask.
//
// By keeping the interface narrow (tenantId + filter + cursor + limit) we:
//   1. isolate the UI from the wire format,
//   2. let tests run against [FakeEventsClient] without mocking Dio, and
//   3. make the eventual server-streaming binding a drop-in swap — the list
//      page already consumes pages in cursor order.
//
// Proto-first seam (follow-up RPC):
//   service cloud.directory.v1.Events {
//     rpc List(ListEventsRequest) returns (stream EventSummary);
//   }
// tenant_id is implicit from the authenticated session. The Dart [tenantId]
// argument below is used for cross-tenant re-checks on the client, not for
// server dispatch.

import 'dart:async';

import 'events_model.dart';

/// Thrown by a client implementation when the caller requested a tenant the
/// current session is not authorised for. Surfaces as a non-fatal drop in the
/// UI layer — see [EventsListPage].
class CrossTenantEventViolation implements Exception {
  final String expectedTenantId;
  final String actualTenantId;
  final String eventId;

  const CrossTenantEventViolation({
    required this.expectedTenantId,
    required this.actualTenantId,
    required this.eventId,
  });

  @override
  String toString() =>
      'CrossTenantEventViolation(event=$eventId expected=$expectedTenantId '
      'actual=$actualTenantId)';
}

/// Contract for listing events. Implementations are expected to:
///   - be tenant-scoped (the caller-supplied [tenantId] MUST match the
///     authenticated session; implementations may ignore it and rely on the
///     session, but the UI re-checks every row anyway),
///   - support forward-only cursor paging,
///   - NOT mutate [filter].
abstract class EventsClient {
  /// Returns the next page of events. `cursor == null` means "start from the
  /// most recent". `limit` is a hint — the server may return fewer rows.
  Future<EventsPage> list({
    required String tenantId,
    required EventFilter filter,
    String? cursor,
    int limit = 50,
  });
}

/// In-memory fake used by widget tests and the initial scaffolding before the
/// real RPC lands. Honours the filter and returns rows in descending
/// timestamp order. Pagination is cursor-by-index.
class FakeEventsClient implements EventsClient {
  final List<EventSummary> _fixture;

  /// Optional injected delay to simulate async. `null` resolves synchronously
  /// (via `Future.value`) which keeps widget tests deterministic.
  final Duration? delay;

  FakeEventsClient(List<EventSummary> fixture, {this.delay})
      : _fixture = List.of(fixture)
          ..sort((a, b) => b.timestamp.compareTo(a.timestamp));

  @override
  Future<EventsPage> list({
    required String tenantId,
    required EventFilter filter,
    String? cursor,
    int limit = 50,
  }) async {
    if (delay != null) {
      await Future<void>.delayed(delay!);
    }
    final filtered = _fixture.where((e) => _matches(e, filter)).toList();
    final startIdx =
        cursor == null ? 0 : int.parse(cursor).clamp(0, filtered.length);
    final endIdx = (startIdx + limit).clamp(0, filtered.length);
    final page = filtered.sublist(startIdx, endIdx);
    final nextCursor = endIdx < filtered.length ? endIdx.toString() : null;
    return EventsPage(items: page, nextCursor: nextCursor);
  }

  bool _matches(EventSummary e, EventFilter f) {
    if (f.severities.isNotEmpty && !f.severities.contains(e.severity)) {
      return false;
    }
    if (f.cameraIds.isNotEmpty && !f.cameraIds.contains(e.cameraId)) {
      return false;
    }
    final now = DateTime.now();
    switch (f.timeRange) {
      case EventTimeRange.today:
        final startOfDay = DateTime(now.year, now.month, now.day);
        if (e.timestamp.isBefore(startOfDay)) return false;
        break;
      case EventTimeRange.last7d:
        if (e.timestamp.isBefore(now.subtract(const Duration(days: 7)))) {
          return false;
        }
        break;
      case EventTimeRange.last30d:
        if (e.timestamp.isBefore(now.subtract(const Duration(days: 30)))) {
          return false;
        }
        break;
      case EventTimeRange.custom:
        final r = f.customRange;
        if (r == null) break;
        if (e.timestamp.isBefore(r.start) || e.timestamp.isAfter(r.end)) {
          return false;
        }
        break;
    }
    return true;
  }
}
