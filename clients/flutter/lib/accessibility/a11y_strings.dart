/// Localised accessibility strings for assistive-technology announcements.
///
/// Supports en, es, fr, de. Defaults to English for unknown locales.
// i18n base: en
class A11yStringsL10n {
  A11yStringsL10n._(); // prevent instantiation

  static const _strings = <String, Map<String, String>>{
    'en': {
      'tapToActivate': 'Tap to activate',
      'doubleTapToActivate': 'Double tap to activate',
      'loading': 'Loading',
      'streamLive': 'Stream is live',
      'streamOffline': 'Stream is offline',
      'cameraSelected': 'Camera selected',
      'errorOccurred': 'An error occurred',
      'retrying': 'Retrying',
      'muted': 'Muted',
      'unmuted': 'Unmuted',
    },
    'es': {
      'tapToActivate': 'Toca para activar',
      'doubleTapToActivate': 'Toca dos veces para activar',
      'loading': 'Cargando',
      'streamLive': 'Transmision en vivo',
      'streamOffline': 'Transmision fuera de linea',
      'cameraSelected': 'Camara seleccionada',
      'errorOccurred': 'Ocurrio un error',
      'retrying': 'Reintentando',
      'muted': 'Silenciado',
      'unmuted': 'Con sonido',
    },
    'fr': {
      'tapToActivate': 'Appuyez pour activer',
      'doubleTapToActivate': 'Appuyez deux fois pour activer',
      'loading': 'Chargement',
      'streamLive': 'Flux en direct',
      'streamOffline': 'Flux hors ligne',
      'cameraSelected': 'Camera selectionnee',
      'errorOccurred': 'Une erreur est survenue',
      'retrying': 'Nouvelle tentative',
      'muted': 'Muet',
      'unmuted': 'Son active',
    },
    'de': {
      'tapToActivate': 'Tippen zum Aktivieren',
      'doubleTapToActivate': 'Doppeltippen zum Aktivieren',
      'loading': 'Wird geladen',
      'streamLive': 'Stream ist live',
      'streamOffline': 'Stream ist offline',
      'cameraSelected': 'Kamera ausgewaehlt',
      'errorOccurred': 'Ein Fehler ist aufgetreten',
      'retrying': 'Erneuter Versuch',
      'muted': 'Stummgeschaltet',
      'unmuted': 'Ton aktiviert',
    },
  };

  /// Returns the string map for the given [locale] code (e.g. "en", "es").
  ///
  /// Falls back to English for unsupported locales.
  static Map<String, String> forLocale(String locale) {
    return _strings[locale] ?? _strings['en']!;
  }

  /// All supported locale codes.
  static List<String> get supportedLocales => _strings.keys.toList();
}
