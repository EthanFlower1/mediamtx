// KAI-300 — Live-view localizable strings.
//
// Follows the pattern established by AuthStrings (KAI-297) and
// DiscoveryStrings (KAI-296). When flutter_intl / ARB wiring lands, replace
// the const bodies with AppLocalizations.of(ctx) lookups — call sites stay
// unchanged.
//
// Four locales: en, es, fr, de.

/// User-visible strings for the single-camera live view feature.
class LiveViewStringsL10n {
  const LiveViewStringsL10n({
    required this.requesting,
    required this.connecting,
    required this.streamUnavailable,
    required this.retry,
    required this.snapshot,
    required this.snapshotSaving,
    required this.snapshotSaved,
    required this.snapshotFailed,
    required this.fullscreen,
    required this.exitFullscreen,
    required this.audioMuted,
    required this.audioOn,
    required this.talkback,
    required this.talkbackHold,
    required this.back,
    required this.ptzZoom,
    required this.latencyMs,
    required this.errorNotAuthenticated,
    required this.errorNoEndpoints,
    required this.errorAllFailed,
    required this.errorRequestFailed,
  });

  final String requesting;
  final String connecting;
  final String streamUnavailable;
  final String retry;

  // Snapshot
  final String snapshot;
  final String snapshotSaving;
  final String snapshotSaved;
  final String snapshotFailed;

  // Controls
  final String fullscreen;
  final String exitFullscreen;
  final String audioMuted;
  final String audioOn;
  final String talkback;
  final String talkbackHold;
  final String back;
  final String ptzZoom;
  final String latencyMs;

  // Errors
  final String errorNotAuthenticated;
  final String errorNoEndpoints;
  final String errorAllFailed;
  final String errorRequestFailed;

  // ── Locale definitions ──────────────────────────────────────────────────

  static const LiveViewStringsL10n en = LiveViewStringsL10n(
    requesting: 'Requesting stream\u2026',
    connecting: 'Connecting\u2026',
    streamUnavailable: 'Stream unavailable',
    retry: 'Retry',
    snapshot: 'Snapshot',
    snapshotSaving: 'Saving\u2026',
    snapshotSaved: 'Snapshot saved',
    snapshotFailed: 'Snapshot failed',
    fullscreen: 'Fullscreen',
    exitFullscreen: 'Exit',
    audioMuted: 'Muted',
    audioOn: 'Audio',
    talkback: 'Talk',
    talkbackHold: 'Hold',
    back: 'Back',
    ptzZoom: 'ZOOM',
    latencyMs: 'ms',
    errorNotAuthenticated: 'Not authenticated. Please log in again.',
    errorNoEndpoints: 'No stream endpoints available for this camera.',
    errorAllFailed: 'All stream endpoints failed. Check network and camera.',
    errorRequestFailed:
        'Stream request failed. Check your connection and try again.',
  );

  static const LiveViewStringsL10n es = LiveViewStringsL10n(
    requesting: 'Solicitando transmisi\u00f3n\u2026',
    connecting: 'Conectando\u2026',
    streamUnavailable: 'Transmisi\u00f3n no disponible',
    retry: 'Reintentar',
    snapshot: 'Captura',
    snapshotSaving: 'Guardando\u2026',
    snapshotSaved: 'Captura guardada',
    snapshotFailed: 'Error al guardar la captura',
    fullscreen: 'Pantalla completa',
    exitFullscreen: 'Salir',
    audioMuted: 'Silenciado',
    audioOn: 'Audio',
    talkback: 'Hablar',
    talkbackHold: 'Mantener',
    back: 'Atr\u00e1s',
    ptzZoom: 'ZOOM',
    latencyMs: 'ms',
    errorNotAuthenticated: 'No autenticado. Inicia sesi\u00f3n de nuevo.',
    errorNoEndpoints:
        'No hay endpoints de transmisi\u00f3n disponibles para esta c\u00e1mara.',
    errorAllFailed:
        'Todos los endpoints fallaron. Verifica la red y la c\u00e1mara.',
    errorRequestFailed:
        'Error al solicitar la transmisi\u00f3n. Verifica tu conexi\u00f3n.',
  );

  static const LiveViewStringsL10n fr = LiveViewStringsL10n(
    requesting: 'Demande du flux\u2026',
    connecting: 'Connexion\u2026',
    streamUnavailable: 'Flux indisponible',
    retry: 'R\u00e9essayer',
    snapshot: 'Capture',
    snapshotSaving: 'Enregistrement\u2026',
    snapshotSaved: 'Capture enregistr\u00e9e',
    snapshotFailed: '\u00c9chec de la capture',
    fullscreen: 'Plein \u00e9cran',
    exitFullscreen: 'Quitter',
    audioMuted: 'Muet',
    audioOn: 'Audio',
    talkback: 'Parler',
    talkbackHold: 'Maintenir',
    back: 'Retour',
    ptzZoom: 'ZOOM',
    latencyMs: 'ms',
    errorNotAuthenticated:
        'Non authentifi\u00e9. Veuillez vous reconnecter.',
    errorNoEndpoints:
        'Aucun endpoint de flux disponible pour cette cam\u00e9ra.',
    errorAllFailed:
        'Tous les endpoints ont \u00e9chou\u00e9. V\u00e9rifiez le r\u00e9seau.',
    errorRequestFailed:
        'Erreur lors de la demande du flux. V\u00e9rifiez votre connexion.',
  );

  static const LiveViewStringsL10n de = LiveViewStringsL10n(
    requesting: 'Stream wird angefordert\u2026',
    connecting: 'Verbinde\u2026',
    streamUnavailable: 'Stream nicht verf\u00fcgbar',
    retry: 'Erneut versuchen',
    snapshot: 'Schnappschuss',
    snapshotSaving: 'Speichern\u2026',
    snapshotSaved: 'Schnappschuss gespeichert',
    snapshotFailed: 'Schnappschuss fehlgeschlagen',
    fullscreen: 'Vollbild',
    exitFullscreen: 'Beenden',
    audioMuted: 'Stummgeschaltet',
    audioOn: 'Audio',
    talkback: 'Sprechen',
    talkbackHold: 'Halten',
    back: 'Zur\u00fcck',
    ptzZoom: 'ZOOM',
    latencyMs: 'ms',
    errorNotAuthenticated:
        'Nicht authentifiziert. Bitte erneut anmelden.',
    errorNoEndpoints:
        'Keine Stream-Endpunkte f\u00fcr diese Kamera verf\u00fcgbar.',
    errorAllFailed:
        'Alle Endpunkte fehlgeschlagen. Netzwerk und Kamera pr\u00fcfen.',
    errorRequestFailed:
        'Stream-Anfrage fehlgeschlagen. Verbindung \u00fcberpr\u00fcfen.',
  );

  /// Returns the strings for the given locale tag (e.g. 'en', 'es', 'fr', 'de').
  /// Falls back to English for unknown locales.
  static LiveViewStringsL10n forLocale(String languageCode) {
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
