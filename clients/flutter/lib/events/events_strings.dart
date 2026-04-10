// KAI-312 — English base strings for the events screen.
//
// i18n base: en
//
// All user-visible text on the events list / filter sheet / detail shell
// lives here as static getters, so the i18n follow-up (KAI-389) can swap in
// `arb`-backed lookups without touching UI code.

class EventsStrings {
  const EventsStrings._();

  // i18n base: en
  static String get screenTitle => 'Alerts & Events';
  static String get emptyState => 'No events match your filters.';
  static String get loadError => 'Could not load events. Pull to retry.';
  static String get retry => 'Retry';
  static String get pullToRefresh => 'Pull down to refresh';
  static String get loadingMore => 'Loading more events…';

  // Filter sheet
  static String get filterTitle => 'Filter events';
  static String get filterSeverity => 'Severity';
  static String get filterCameras => 'Cameras';
  static String get filterTimeRange => 'Time range';
  static String get filterApply => 'Apply';
  static String get filterReset => 'Reset';
  static String get filterCamerasAll => 'All cameras';

  static String get severityInfo => 'Info';
  static String get severityWarning => 'Warning';
  static String get severityCritical => 'Critical';

  static String get timeRangeToday => 'Today';
  static String get timeRangeLast7d => 'Last 7 days';
  static String get timeRangeLast30d => 'Last 30 days';
  static String get timeRangeCustom => 'Custom…';

  // Detail shell
  static String get detailTitle => 'Event detail';
  static String get detailLoading => 'Loading event…';
  static String get detailNotAvailable =>
      'Event detail is not available on this build.';
}
