// KAI-299 — Federated camera tree localizable strings.
//
// Follows the AuthStrings pattern: all user-facing text for the federated
// camera tree flow routes through this single class so the eventual
// flutter_intl / ARB migration only has to touch one file.
//
// i18n base: en — when ARB wiring lands, copy these literals into intl_en.arb
// and swap the static `en` field for `AppLocalizations.of(ctx)`.

/// User-visible strings used by the federated camera tree.
class CameraStrings {
  const CameraStrings({
    required this.treeScreenTitle,
    required this.searchHint,
    required this.searchClearTooltip,
    required this.emptyTreeMessage,
    required this.emptySearchMessage,
    required this.homeDirectoryLabel,
    required this.unknownSiteLabel,
    required this.statusOnline,
    required this.statusOffline,
    required this.statusUnknown,
    required this.thumbnailLockedTooltip,
    required this.peerDisconnectedNotice,
    required this.cameraCountSingular,
    required this.cameraCountPlural,
  });

  final String treeScreenTitle;
  final String searchHint;
  final String searchClearTooltip;
  final String emptyTreeMessage;
  final String emptySearchMessage;
  final String homeDirectoryLabel;
  final String unknownSiteLabel;
  final String statusOnline;
  final String statusOffline;
  final String statusUnknown;
  final String thumbnailLockedTooltip;
  final String peerDisconnectedNotice;
  final String cameraCountSingular;
  final String cameraCountPlural;

  /// Default English strings. Override via Riverpod in tests.
  static const CameraStrings en = CameraStrings(
    treeScreenTitle: 'Cameras',
    searchHint: 'Search cameras, sites, or directories',
    searchClearTooltip: 'Clear search',
    emptyTreeMessage: 'No cameras available yet.',
    emptySearchMessage: 'No cameras match your search.',
    homeDirectoryLabel: 'Home',
    unknownSiteLabel: 'Unassigned site',
    statusOnline: 'Online',
    statusOffline: 'Offline',
    statusUnknown: 'Unknown',
    thumbnailLockedTooltip:
        'Thumbnail hidden — you do not have view.thumbnails permission.',
    peerDisconnectedNotice: 'Directory disconnected. Status may be stale.',
    cameraCountSingular: '1 camera',
    cameraCountPlural: '{count} cameras',
  );
}
