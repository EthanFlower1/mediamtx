// KAI-302 — Playback timeline localizable strings.
//
// Mirrors the AuthStrings pattern: a single class of immutable fields that
// callers read for every user-visible string in the playback timeline flow.
// When flutter_intl lands, the body swaps for AppLocalizations; call sites
// stay put. Tests override individual fields instead of string-matching.

/// User-visible strings used by the KAI-302 playback timeline flow.
class PlaybackStrings {
  const PlaybackStrings({
    required this.screenTitle,
    required this.datePickerTitle,
    required this.datePickerCancel,
    required this.datePickerConfirm,
    required this.speed1x,
    required this.speed2x,
    required this.speed4x,
    required this.speed8x,
    required this.bookmarkAdd,
    required this.bookmarkDialogTitle,
    required this.bookmarkDialogNoteHint,
    required this.bookmarkDialogCancel,
    required this.bookmarkDialogSave,
    required this.bookmarkCreatedToast,
    required this.bookmarkFailedToast,
    required this.clipExport,
    required this.clipExportDialogTitle,
    required this.clipExportDialogStart,
    required this.clipExportDialogEnd,
    required this.clipExportDialogNoteHint,
    required this.clipExportDialogCancel,
    required this.clipExportDialogConfirm,
    required this.clipExportQueuedToast,
    required this.clipExportFailedToast,
    required this.videoIntegrationPending,
    required this.markerMotion,
    required this.markerFace,
    required this.markerLpr,
    required this.markerManual,
    required this.markerSystem,
    required this.recorderBoundaryLabel,
    required this.directoryBoundaryLabel,
    required this.gapLabel,
    required this.noRecordings,
    required this.loading,
  });

  final String screenTitle;

  final String datePickerTitle;
  final String datePickerCancel;
  final String datePickerConfirm;

  final String speed1x;
  final String speed2x;
  final String speed4x;
  final String speed8x;

  final String bookmarkAdd;
  final String bookmarkDialogTitle;
  final String bookmarkDialogNoteHint;
  final String bookmarkDialogCancel;
  final String bookmarkDialogSave;
  final String bookmarkCreatedToast;
  final String bookmarkFailedToast;

  final String clipExport;
  final String clipExportDialogTitle;
  final String clipExportDialogStart;
  final String clipExportDialogEnd;
  final String clipExportDialogNoteHint;
  final String clipExportDialogCancel;
  final String clipExportDialogConfirm;
  final String clipExportQueuedToast;
  final String clipExportFailedToast;

  final String videoIntegrationPending;

  final String markerMotion;
  final String markerFace;
  final String markerLpr;
  final String markerManual;
  final String markerSystem;

  final String recorderBoundaryLabel;
  final String directoryBoundaryLabel;
  final String gapLabel;
  final String noRecordings;
  final String loading;

  /// Default English strings. Tests override via a Riverpod provider.
  static const PlaybackStrings en = PlaybackStrings(
    screenTitle: 'Playback',
    datePickerTitle: 'Select date',
    datePickerCancel: 'Cancel',
    datePickerConfirm: 'OK',
    speed1x: '1x',
    speed2x: '2x',
    speed4x: '4x',
    speed8x: '8x',
    bookmarkAdd: 'Add bookmark',
    bookmarkDialogTitle: 'New bookmark',
    bookmarkDialogNoteHint: 'Optional note',
    bookmarkDialogCancel: 'Cancel',
    bookmarkDialogSave: 'Save',
    bookmarkCreatedToast: 'Bookmark saved.',
    bookmarkFailedToast: 'Could not save bookmark.',
    clipExport: 'Export clip',
    clipExportDialogTitle: 'Export clip',
    clipExportDialogStart: 'Start',
    clipExportDialogEnd: 'End',
    clipExportDialogNoteHint: 'Optional note',
    clipExportDialogCancel: 'Cancel',
    clipExportDialogConfirm: 'Export',
    clipExportQueuedToast: 'Clip export queued.',
    clipExportFailedToast: 'Could not queue clip export.',
    videoIntegrationPending: 'Video playback integration pending.',
    markerMotion: 'Motion',
    markerFace: 'Face',
    markerLpr: 'License plate',
    markerManual: 'Manual',
    markerSystem: 'System',
    recorderBoundaryLabel: 'Recorder',
    directoryBoundaryLabel: 'Directory',
    gapLabel: 'No recording',
    noRecordings: 'No recordings for this range.',
    loading: 'Loading…',
  );
}
