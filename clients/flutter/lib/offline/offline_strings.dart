// i18n base: en

/// Localized strings for the offline mode UI.
abstract class OfflineStrings {
  String get offline;
  String get showingCachedData;
  String get lastUpdated;
  String minutesAgo(int minutes);
  String hoursAgo(int hours);
  String get actionQueued;
  String get reconnecting;
}

/// Localization registry for [OfflineStrings].
///
/// Currently ships en, es, fr, and de. Defaults to English for unknown locales.
class OfflineStringsL10n {
  OfflineStringsL10n._();

  static final Map<String, OfflineStrings> _locales = {
    'en': _EnOfflineStrings(),
    'es': _EsOfflineStrings(),
    'fr': _FrOfflineStrings(),
    'de': _DeOfflineStrings(),
  };

  /// Return the [OfflineStrings] for the given BCP-47 language code.
  ///
  /// Falls back to English when the locale is not supported.
  static OfflineStrings forLocale(String languageCode) =>
      _locales[languageCode] ?? _locales['en']!;

  /// All supported locale codes.
  static List<String> get supportedLocales => _locales.keys.toList();
}

// ---------------------------------------------------------------------------
// English
// ---------------------------------------------------------------------------
class _EnOfflineStrings implements OfflineStrings {
  @override
  String get offline => 'Offline';
  @override
  String get showingCachedData => 'Showing cached data.';
  @override
  String get lastUpdated => 'Last updated';
  @override
  String minutesAgo(int minutes) => '$minutes min ago';
  @override
  String hoursAgo(int hours) => '$hours hr ago';
  @override
  String get actionQueued => 'Action queued for when you reconnect.';
  @override
  String get reconnecting => 'Reconnecting...';
}

// ---------------------------------------------------------------------------
// Spanish
// ---------------------------------------------------------------------------
class _EsOfflineStrings implements OfflineStrings {
  @override
  String get offline => 'Sin conexion';
  @override
  String get showingCachedData => 'Mostrando datos en cache.';
  @override
  String get lastUpdated => 'Ultima actualizacion';
  @override
  String minutesAgo(int minutes) => 'hace $minutes min';
  @override
  String hoursAgo(int hours) => 'hace $hours h';
  @override
  String get actionQueued => 'Accion en cola para cuando se reconecte.';
  @override
  String get reconnecting => 'Reconectando...';
}

// ---------------------------------------------------------------------------
// French
// ---------------------------------------------------------------------------
class _FrOfflineStrings implements OfflineStrings {
  @override
  String get offline => 'Hors ligne';
  @override
  String get showingCachedData => 'Donnees en cache affichees.';
  @override
  String get lastUpdated => 'Derniere mise a jour';
  @override
  String minutesAgo(int minutes) => 'il y a $minutes min';
  @override
  String hoursAgo(int hours) => 'il y a $hours h';
  @override
  String get actionQueued => 'Action mise en file d\'attente.';
  @override
  String get reconnecting => 'Reconnexion...';
}

// ---------------------------------------------------------------------------
// German
// ---------------------------------------------------------------------------
class _DeOfflineStrings implements OfflineStrings {
  @override
  String get offline => 'Offline';
  @override
  String get showingCachedData => 'Zwischengespeicherte Daten werden angezeigt.';
  @override
  String get lastUpdated => 'Zuletzt aktualisiert';
  @override
  String minutesAgo(int minutes) => 'vor $minutes Min.';
  @override
  String hoursAgo(int hours) => 'vor $hours Std.';
  @override
  String get actionQueued => 'Aktion in die Warteschlange gestellt.';
  @override
  String get reconnecting => 'Verbindung wird hergestellt...';
}
