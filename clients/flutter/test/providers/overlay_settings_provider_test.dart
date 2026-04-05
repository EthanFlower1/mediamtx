import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/providers/overlay_settings_provider.dart';

void main() {
  group('OverlaySettings model', () {
    test('default overlayVisible is true', () {
      const settings = OverlaySettings();
      expect(settings.overlayVisible, true);
    });

    test('copyWith changes overlayVisible', () {
      const settings = OverlaySettings();
      final hidden = settings.copyWith(overlayVisible: false);
      expect(hidden.overlayVisible, false);
    });

    test('copyWith preserves value when null', () {
      const settings = OverlaySettings(overlayVisible: false);
      final same = settings.copyWith();
      expect(same.overlayVisible, false);
    });
  });

  group('OverlaySettingsNotifier', () {
    late OverlaySettingsNotifier notifier;

    setUp(() {
      notifier = OverlaySettingsNotifier();
    });

    tearDown(() {
      notifier.dispose();
    });

    test('initial state has overlayVisible true', () {
      expect(notifier.state.overlayVisible, true);
    });

    test('setOverlayVisible(false) hides overlay', () {
      notifier.setOverlayVisible(false);
      expect(notifier.state.overlayVisible, false);
    });

    test('setOverlayVisible(true) shows overlay', () {
      notifier.setOverlayVisible(false);
      notifier.setOverlayVisible(true);
      expect(notifier.state.overlayVisible, true);
    });

    test('toggleOverlay flips visibility', () {
      expect(notifier.state.overlayVisible, true);

      notifier.toggleOverlay();
      expect(notifier.state.overlayVisible, false);

      notifier.toggleOverlay();
      expect(notifier.state.overlayVisible, true);
    });

    test('multiple toggles alternate correctly', () {
      for (int i = 0; i < 5; i++) {
        notifier.toggleOverlay();
      }
      // 5 toggles from true: false, true, false, true, false
      expect(notifier.state.overlayVisible, false);
    });
  });
}
