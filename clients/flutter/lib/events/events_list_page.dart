// KAI-312 — Events list page (alerts/events screen).
//
// Paged, Riverpod-wired list of recent events for the current tenant. Shows
// time + severity + camera + kind per row; tap opens the detail shell. Pull
// to refresh and infinite scroll are implemented with plain ListView.builder
// plus a scroll listener — no extra deps.
//
// SECURITY INVARIANT (defense-in-depth): every row must satisfy
//   event.tenantId == AppSession.tenantRef
// Any row that violates is silently dropped from the rendered list and a
// `CrossTenantEventViolation` warning is logged via `debugPrint`. There is
// NO user-visible surface for violations — this is a belt-and-braces check
// against a rogue or compromised server, not a user-facing error path. The
// server is already the authoritative tenant scoper.
//
// Proto-first seam: the list is powered by [EventsClient] which is meant to
// become a thin wrapper around a `cloud.directory.v1.Events.List` server-
// streaming RPC. See PR body.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/app_session.dart';
import 'event_detail_page.dart';
import 'events_client.dart';
import 'events_filter.dart';
import 'events_model.dart';
import 'events_strings.dart';

/// Provider for the [EventsClient]. Tests override this with a
/// [FakeEventsClient]. Production wiring (real RPC) is a follow-up — see the
/// proto seam above.
final eventsClientProvider = Provider<EventsClient>((ref) {
  throw UnimplementedError(
    'eventsClientProvider must be overridden before use. '
    'See KAI-312 PR — real RPC binding is a follow-up.',
  );
});

/// Camera choices for the filter sheet. Overridden by the app shell with the
/// flattened site tree (KAI-299 landing follow-up). Defaults to empty, which
/// the filter sheet renders as "All cameras".
final eventsCameraChoicesProvider = Provider<List<CameraChoice>>((ref) {
  return const [];
});

class EventsListPage extends ConsumerStatefulWidget {
  const EventsListPage({super.key});

  @override
  ConsumerState<EventsListPage> createState() => _EventsListPageState();
}

class _EventsListPageState extends ConsumerState<EventsListPage> {
  final ScrollController _scrollController = ScrollController();

  EventFilter _filter = const EventFilter();
  final List<EventSummary> _rows = [];
  String? _cursor;
  bool _loading = false;
  bool _hasMore = true;
  Object? _error;

  @override
  void initState() {
    super.initState();
    _scrollController.addListener(_onScroll);
    WidgetsBinding.instance.addPostFrameCallback((_) => _loadInitial());
  }

  @override
  void dispose() {
    _scrollController.removeListener(_onScroll);
    _scrollController.dispose();
    super.dispose();
  }

  void _onScroll() {
    if (!_scrollController.hasClients) return;
    final pos = _scrollController.position;
    if (pos.pixels > pos.maxScrollExtent - 200 &&
        !_loading &&
        _hasMore &&
        _error == null) {
      _loadMore();
    }
  }

  String? _currentTenantId() {
    final session = ref.read(appSessionProvider);
    if (!session.isAuthenticated) return null;
    final t = session.tenantRef;
    return t.isEmpty ? null : t;
  }

  Future<void> _loadInitial() async {
    setState(() {
      _rows.clear();
      _cursor = null;
      _hasMore = true;
      _error = null;
    });
    await _loadMore();
  }

  Future<void> _loadMore() async {
    if (_loading || !_hasMore) return;
    final tenantId = _currentTenantId();
    if (tenantId == null) return;
    setState(() => _loading = true);
    try {
      final client = ref.read(eventsClientProvider);
      final page = await client.list(
        tenantId: tenantId,
        filter: _filter,
        cursor: _cursor,
      );
      final safeRows = <EventSummary>[];
      for (final ev in page.items) {
        if (ev.tenantId != tenantId) {
          debugPrint(
            'CrossTenantEventViolation: dropping event ${ev.id} '
            '(expected=$tenantId actual=${ev.tenantId})',
          );
          continue;
        }
        safeRows.add(ev);
      }
      if (!mounted) return;
      setState(() {
        _rows.addAll(safeRows);
        _cursor = page.nextCursor;
        _hasMore = page.hasMore;
        _loading = false;
      });
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e;
        _loading = false;
      });
    }
  }

  Future<void> _openFilterSheet() async {
    final choices = ref.read(eventsCameraChoicesProvider);
    final updated = await showEventsFilterSheet(
      context,
      current: _filter,
      cameraChoices: choices,
    );
    if (updated == null) return;
    setState(() => _filter = updated);
    await _loadInitial();
  }

  void _openDetail(EventSummary row) {
    Navigator.of(context).push(
      MaterialPageRoute<void>(
        builder: (_) => EventDetailPage(eventId: row.id, tenantId: row.tenantId),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: Text(EventsStrings.screenTitle),
        actions: [
          IconButton(
            key: const ValueKey('events-open-filter'),
            icon: const Icon(Icons.filter_list),
            tooltip: EventsStrings.filterTitle,
            onPressed: _openFilterSheet,
          ),
        ],
      ),
      body: _buildBody(),
    );
  }

  Widget _buildBody() {
    if (_error != null && _rows.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(EventsStrings.loadError),
            const SizedBox(height: 8),
            FilledButton(
              key: const ValueKey('events-retry'),
              onPressed: _loadInitial,
              child: Text(EventsStrings.retry),
            ),
          ],
        ),
      );
    }
    if (_rows.isEmpty && !_loading) {
      return RefreshIndicator(
        onRefresh: _loadInitial,
        child: ListView(
          physics: const AlwaysScrollableScrollPhysics(),
          children: [
            const SizedBox(height: 120),
            Center(child: Text(EventsStrings.emptyState)),
          ],
        ),
      );
    }
    return RefreshIndicator(
      onRefresh: _loadInitial,
      child: ListView.builder(
        key: const ValueKey('events-list'),
        controller: _scrollController,
        physics: const AlwaysScrollableScrollPhysics(),
        itemCount: _rows.length + (_hasMore || _loading ? 1 : 0),
        itemBuilder: (ctx, i) {
          if (i >= _rows.length) {
            return const Padding(
              padding: EdgeInsets.all(16),
              child: Center(child: CircularProgressIndicator()),
            );
          }
          final row = _rows[i];
          return _EventTile(
            key: ValueKey('events-row-${row.id}'),
            row: row,
            onTap: () => _openDetail(row),
          );
        },
      ),
    );
  }
}

class _EventTile extends StatelessWidget {
  final EventSummary row;
  final VoidCallback onTap;

  const _EventTile({super.key, required this.row, required this.onTap});

  Color _severityColor(BuildContext context) {
    switch (row.severity) {
      case EventSeverity.info:
        return Colors.blueGrey;
      case EventSeverity.warning:
        return Colors.orange;
      case EventSeverity.critical:
        return Colors.red;
    }
  }

  String _severityShort() {
    switch (row.severity) {
      case EventSeverity.info:
        return 'INFO';
      case EventSeverity.warning:
        return 'WARN';
      case EventSeverity.critical:
        return 'CRIT';
    }
  }

  String _formatTime(DateTime ts) {
    final local = ts.toLocal();
    final h = local.hour.toString().padLeft(2, '0');
    final m = local.minute.toString().padLeft(2, '0');
    final d = local.day.toString().padLeft(2, '0');
    final mo = local.month.toString().padLeft(2, '0');
    return '$mo-$d $h:$m';
  }

  @override
  Widget build(BuildContext context) {
    return ListTile(
      onTap: onTap,
      leading: Container(
        width: 56,
        alignment: Alignment.center,
        padding: const EdgeInsets.symmetric(vertical: 4, horizontal: 6),
        decoration: BoxDecoration(
          color: _severityColor(context).withValues(alpha: 0.15),
          borderRadius: BorderRadius.circular(6),
        ),
        child: Text(
          _severityShort(),
          style: TextStyle(
            color: _severityColor(context),
            fontWeight: FontWeight.bold,
            fontSize: 11,
          ),
        ),
      ),
      title: Text(row.cameraName),
      subtitle: Text('${row.kind} · ${_formatTime(row.timestamp)}'),
      trailing: const Icon(Icons.chevron_right),
    );
  }
}
