// KAI-300 — Unit tests for LiveViewStringsL10n.
//
// Verifies all four locale instances compile and return non-empty strings.

import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/features/live_view/live_view_strings.dart';

void main() {
  group('LiveViewStringsL10n', () {
    test('en locale: all strings non-empty', () {
      const s = LiveViewStringsL10n.en;
      expect(s.requesting, isNotEmpty);
      expect(s.connecting, isNotEmpty);
      expect(s.streamUnavailable, isNotEmpty);
      expect(s.retry, isNotEmpty);
      expect(s.snapshot, isNotEmpty);
      expect(s.snapshotSaving, isNotEmpty);
      expect(s.snapshotSaved, isNotEmpty);
      expect(s.snapshotFailed, isNotEmpty);
      expect(s.fullscreen, isNotEmpty);
      expect(s.exitFullscreen, isNotEmpty);
      expect(s.audioMuted, isNotEmpty);
      expect(s.audioOn, isNotEmpty);
      expect(s.talkback, isNotEmpty);
      expect(s.talkbackHold, isNotEmpty);
      expect(s.back, isNotEmpty);
      expect(s.ptzZoom, isNotEmpty);
      expect(s.latencyMs, isNotEmpty);
      expect(s.errorNotAuthenticated, isNotEmpty);
      expect(s.errorNoEndpoints, isNotEmpty);
      expect(s.errorAllFailed, isNotEmpty);
      expect(s.errorRequestFailed, isNotEmpty);
    });

    test('es locale: retry is Spanish', () {
      const s = LiveViewStringsL10n.es;
      expect(s.retry, 'Reintentar');
    });

    test('fr locale: retry is French', () {
      const s = LiveViewStringsL10n.fr;
      expect(s.retry, 'R\u00e9essayer');
    });

    test('de locale: retry is German', () {
      const s = LiveViewStringsL10n.de;
      expect(s.retry, 'Erneut versuchen');
    });

    test('forLocale falls back to en for unknown locale', () {
      final s = LiveViewStringsL10n.forLocale('ja');
      expect(s.retry, LiveViewStringsL10n.en.retry);
    });

    test('forLocale returns correct locale for each supported code', () {
      expect(LiveViewStringsL10n.forLocale('en').retry, 'Retry');
      expect(LiveViewStringsL10n.forLocale('es').retry, 'Reintentar');
      expect(LiveViewStringsL10n.forLocale('fr').retry, 'R\u00e9essayer');
      expect(LiveViewStringsL10n.forLocale('de').retry, 'Erneut versuchen');
    });
  });
}
