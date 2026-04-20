import 'package:flutter_riverpod/flutter_riverpod.dart';

class PlaybackSession {
  final Set<String> selectedCameraIds;
  final DateTime selectedDate;
  final Duration position;
  final int zoomIndex;

  PlaybackSession({
    Set<String>? selectedCameraIds,
    DateTime? selectedDate,
    this.position = Duration.zero,
    this.zoomIndex = 2,
  })  : selectedCameraIds = selectedCameraIds ?? {},
        selectedDate = selectedDate ?? DateTime.now();
}

class PlaybackSessionNotifier extends StateNotifier<PlaybackSession> {
  PlaybackSessionNotifier() : super(PlaybackSession());

  void save({
    required Set<String> cameraIds,
    required DateTime date,
    required Duration position,
    required int zoomIndex,
  }) {
    state = PlaybackSession(
      selectedCameraIds: {...cameraIds},
      selectedDate: date,
      position: position,
      zoomIndex: zoomIndex,
    );
  }
}

final playbackSessionProvider =
    StateNotifierProvider<PlaybackSessionNotifier, PlaybackSession>(
  (ref) => PlaybackSessionNotifier(),
);
