// KAI-312 — Filter sheet for the events list.
//
// Modal bottom sheet with three sections: severity (multi), cameras
// (multi), and time range (single). Camera multi-select is populated from a
// caller-supplied `cameraChoices` list so this widget does NOT hard-depend on
// the federated camera tree (KAI-299). When KAI-299 lands, the list page can
// pass the flattened site-tree camera list straight in — thin adapter, no
// edits here.

import 'package:flutter/material.dart' hide DateTimeRange;

import 'events_model.dart';
import 'events_strings.dart';

/// Lightweight DTO the filter sheet needs for each camera option. Keep this
/// trivial so it can be constructed from KAI-299's richer tree node or from
/// any other source.
class CameraChoice {
  final String id;
  final String label;
  const CameraChoice({required this.id, required this.label});
}

/// Opens a modal bottom sheet to edit the filter. Returns the updated filter
/// on Apply, or `null` if the user dismissed / cancelled.
Future<EventFilter?> showEventsFilterSheet(
  BuildContext context, {
  required EventFilter current,
  required List<CameraChoice> cameraChoices,
}) {
  return showModalBottomSheet<EventFilter>(
    context: context,
    isScrollControlled: true,
    builder: (ctx) => _EventsFilterSheet(
      initial: current,
      cameraChoices: cameraChoices,
    ),
  );
}

class _EventsFilterSheet extends StatefulWidget {
  final EventFilter initial;
  final List<CameraChoice> cameraChoices;

  const _EventsFilterSheet({
    required this.initial,
    required this.cameraChoices,
  });

  @override
  State<_EventsFilterSheet> createState() => _EventsFilterSheetState();
}

class _EventsFilterSheetState extends State<_EventsFilterSheet> {
  late Set<EventSeverity> _severities;
  late Set<String> _cameraIds;
  late EventTimeRange _timeRange;
  DateTimeRange? _customRange;

  @override
  void initState() {
    super.initState();
    _severities = Set.of(widget.initial.severities);
    _cameraIds = Set.of(widget.initial.cameraIds);
    _timeRange = widget.initial.timeRange;
    _customRange = widget.initial.customRange;
  }

  void _toggleSeverity(EventSeverity s) {
    setState(() {
      if (_severities.contains(s)) {
        _severities.remove(s);
      } else {
        _severities.add(s);
      }
    });
  }

  void _toggleCamera(String id) {
    setState(() {
      if (_cameraIds.contains(id)) {
        _cameraIds.remove(id);
      } else {
        _cameraIds.add(id);
      }
    });
  }

  String _severityLabel(EventSeverity s) {
    switch (s) {
      case EventSeverity.info:
        return EventsStrings.severityInfo;
      case EventSeverity.warning:
        return EventsStrings.severityWarning;
      case EventSeverity.critical:
        return EventsStrings.severityCritical;
    }
  }

  String _timeRangeLabel(EventTimeRange r) {
    switch (r) {
      case EventTimeRange.today:
        return EventsStrings.timeRangeToday;
      case EventTimeRange.last7d:
        return EventsStrings.timeRangeLast7d;
      case EventTimeRange.last30d:
        return EventsStrings.timeRangeLast30d;
      case EventTimeRange.custom:
        return EventsStrings.timeRangeCustom;
    }
  }

  @override
  Widget build(BuildContext context) {
    return SafeArea(
      child: Padding(
        padding: const EdgeInsets.all(16),
        child: SingleChildScrollView(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Text(
                EventsStrings.filterTitle,
                style: Theme.of(context).textTheme.titleLarge,
              ),
              const SizedBox(height: 12),
              Text(EventsStrings.filterSeverity,
                  style: Theme.of(context).textTheme.labelLarge),
              Wrap(
                spacing: 8,
                children: EventSeverity.values.map((s) {
                  final selected = _severities.contains(s);
                  return FilterChip(
                    key: ValueKey('events-filter-sev-${s.name}'),
                    label: Text(_severityLabel(s)),
                    selected: selected,
                    onSelected: (_) => _toggleSeverity(s),
                  );
                }).toList(),
              ),
              const SizedBox(height: 16),
              Text(EventsStrings.filterCameras,
                  style: Theme.of(context).textTheme.labelLarge),
              if (widget.cameraChoices.isEmpty)
                Padding(
                  padding: const EdgeInsets.symmetric(vertical: 8),
                  child: Text(EventsStrings.filterCamerasAll),
                )
              else
                Wrap(
                  spacing: 8,
                  children: widget.cameraChoices.map((c) {
                    final selected = _cameraIds.contains(c.id);
                    return FilterChip(
                      key: ValueKey('events-filter-cam-${c.id}'),
                      label: Text(c.label),
                      selected: selected,
                      onSelected: (_) => _toggleCamera(c.id),
                    );
                  }).toList(),
                ),
              const SizedBox(height: 16),
              Text(EventsStrings.filterTimeRange,
                  style: Theme.of(context).textTheme.labelLarge),
              Wrap(
                spacing: 8,
                children: EventTimeRange.values.map((r) {
                  return ChoiceChip(
                    key: ValueKey('events-filter-time-${r.name}'),
                    label: Text(_timeRangeLabel(r)),
                    selected: _timeRange == r,
                    onSelected: (_) => setState(() => _timeRange = r),
                  );
                }).toList(),
              ),
              const SizedBox(height: 24),
              Row(
                mainAxisAlignment: MainAxisAlignment.end,
                children: [
                  TextButton(
                    key: const ValueKey('events-filter-reset'),
                    onPressed: () {
                      setState(() {
                        _severities = {};
                        _cameraIds = {};
                        _timeRange = EventTimeRange.last7d;
                        _customRange = null;
                      });
                    },
                    child: Text(EventsStrings.filterReset),
                  ),
                  const SizedBox(width: 8),
                  FilledButton(
                    key: const ValueKey('events-filter-apply'),
                    onPressed: () {
                      Navigator.of(context).pop(EventFilter(
                        severities: _severities,
                        cameraIds: _cameraIds,
                        timeRange: _timeRange,
                        customRange: _customRange,
                      ));
                    },
                    child: Text(EventsStrings.filterApply),
                  ),
                ],
              ),
            ],
          ),
        ),
      ),
    );
  }
}
