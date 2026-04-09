// KAI-301 — Grid view localizable strings.
//
// Follows the pattern established by LiveViewStringsL10n (KAI-300) and
// AuthStrings (KAI-297). When flutter_intl / ARB wiring lands, replace the
// const bodies with AppLocalizations.of(ctx) lookups — call sites stay
// unchanged.
//
// Four locales: en, es, fr, de.

/// User-visible strings for the multi-camera grid feature.
class GridStrings {
  const GridStrings({
    required this.title,
    required this.layoutTwoByTwo,
    required this.layoutThreeByThree,
    required this.layoutFourByFour,
    required this.alwaysLiveLabel,
    required this.alwaysLiveTooltip,
    required this.connecting,
    required this.offline,
    required this.snapshotMode,
    required this.liveMode,
    required this.loadFailed,
    required this.retry,
    required this.noCameras,
  });

  final String title;

  // Layout picker
  final String layoutTwoByTwo;
  final String layoutThreeByThree;
  final String layoutFourByFour;
  final String alwaysLiveLabel;
  final String alwaysLiveTooltip;

  // Tile states
  final String connecting;
  final String offline;
  final String snapshotMode;
  final String liveMode;
  final String loadFailed;
  final String retry;
  final String noCameras;

  // ── Locale definitions ──────────────────────────────────────────────────

  static const GridStrings en = GridStrings(
    title: 'Cameras',
    layoutTwoByTwo: '2\u00d72',
    layoutThreeByThree: '3\u00d73',
    layoutFourByFour: '4\u00d74',
    alwaysLiveLabel: 'Always live',
    alwaysLiveTooltip:
        'Force WebRTC on every tile even when the grid is large.',
    connecting: 'Connecting\u2026',
    offline: 'Offline',
    snapshotMode: 'Snapshot',
    liveMode: 'Live',
    loadFailed: 'Load failed',
    retry: 'Retry',
    noCameras: 'No cameras to display',
  );

  static const GridStrings es = GridStrings(
    title: 'C\u00e1maras',
    layoutTwoByTwo: '2\u00d72',
    layoutThreeByThree: '3\u00d73',
    layoutFourByFour: '4\u00d74',
    alwaysLiveLabel: 'Siempre en vivo',
    alwaysLiveTooltip:
        'Forzar WebRTC en cada panel incluso con cuadr\u00edculas grandes.',
    connecting: 'Conectando\u2026',
    offline: 'Desconectada',
    snapshotMode: 'Captura',
    liveMode: 'En vivo',
    loadFailed: 'Error al cargar',
    retry: 'Reintentar',
    noCameras: 'No hay c\u00e1maras para mostrar',
  );

  static const GridStrings fr = GridStrings(
    title: 'Cam\u00e9ras',
    layoutTwoByTwo: '2\u00d72',
    layoutThreeByThree: '3\u00d73',
    layoutFourByFour: '4\u00d74',
    alwaysLiveLabel: 'Toujours en direct',
    alwaysLiveTooltip:
        'Forcer WebRTC sur chaque vignette m\u00eame pour une grande grille.',
    connecting: 'Connexion\u2026',
    offline: 'Hors ligne',
    snapshotMode: 'Capture',
    liveMode: 'Direct',
    loadFailed: '\u00c9chec du chargement',
    retry: 'R\u00e9essayer',
    noCameras: 'Aucune cam\u00e9ra \u00e0 afficher',
  );

  static const GridStrings de = GridStrings(
    title: 'Kameras',
    layoutTwoByTwo: '2\u00d72',
    layoutThreeByThree: '3\u00d73',
    layoutFourByFour: '4\u00d74',
    alwaysLiveLabel: 'Immer live',
    alwaysLiveTooltip:
        'WebRTC f\u00fcr jede Kachel erzwingen, auch bei gro\u00dfen Rastern.',
    connecting: 'Verbinde\u2026',
    offline: 'Offline',
    snapshotMode: 'Schnappschuss',
    liveMode: 'Live',
    loadFailed: 'Ladefehler',
    retry: 'Erneut versuchen',
    noCameras: 'Keine Kameras zum Anzeigen',
  );

  /// Returns the strings for the given locale tag. Falls back to English.
  static GridStrings forLocale(String languageCode) {
    switch (languageCode) {
      case 'es':
        return es;
      case 'fr':
        return fr;
      case 'de':
        return de;
      default:
        return en;
    }
  }
}
