import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Controls global visibility of AI detection overlays across all camera views.
class OverlaySettingsNotifier extends StateNotifier<OverlaySettings> {
  OverlaySettingsNotifier() : super(const OverlaySettings());

  void setOverlayVisible(bool visible) {
    state = state.copyWith(overlayVisible: visible);
  }

  void toggleOverlay() {
    state = state.copyWith(overlayVisible: !state.overlayVisible);
  }
}

class OverlaySettings {
  final bool overlayVisible;

  const OverlaySettings({this.overlayVisible = true});

  OverlaySettings copyWith({bool? overlayVisible}) {
    return OverlaySettings(
      overlayVisible: overlayVisible ?? this.overlayVisible,
    );
  }
}

final overlaySettingsProvider =
    StateNotifierProvider<OverlaySettingsNotifier, OverlaySettings>(
  (ref) => OverlaySettingsNotifier(),
);
