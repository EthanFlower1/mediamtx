import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/tour.dart';
import 'auth_provider.dart';

final toursProvider = FutureProvider<List<Tour>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  final res = await api.get('/tours');
  return (res.data as List).map((e) => Tour.fromJson(e as Map<String, dynamic>)).toList();
});

class ActiveTourState {
  const ActiveTourState({this.tour, this.currentCameraIndex = 0, this.isPaused = false});
  final Tour? tour;
  final int currentCameraIndex;
  final bool isPaused;

  bool get isActive => tour != null;
  String? get currentCameraId =>
      tour != null && tour!.cameraIds.isNotEmpty
          ? tour!.cameraIds[currentCameraIndex % tour!.cameraIds.length]
          : null;

  ActiveTourState copyWith({Tour? tour, int? currentCameraIndex, bool? isPaused, bool clearTour = false}) {
    return ActiveTourState(
      tour: clearTour ? null : (tour ?? this.tour),
      currentCameraIndex: currentCameraIndex ?? this.currentCameraIndex,
      isPaused: isPaused ?? this.isPaused,
    );
  }
}

class ActiveTourNotifier extends StateNotifier<ActiveTourState> {
  ActiveTourNotifier() : super(const ActiveTourState());
  Timer? _timer;

  void start(Tour tour) {
    stop();
    state = ActiveTourState(tour: tour);
    _startTimer(tour.dwellSeconds);
  }

  void stop() {
    _timer?.cancel();
    _timer = null;
    state = const ActiveTourState();
  }

  void pause() {
    _timer?.cancel();
    state = state.copyWith(isPaused: true);
  }

  void resume() {
    if (state.tour == null) return;
    state = state.copyWith(isPaused: false);
    _startTimer(state.tour!.dwellSeconds);
  }

  void _startTimer(int dwellSeconds) {
    _timer = Timer.periodic(Duration(seconds: dwellSeconds), (_) {
      if (!state.isPaused && state.tour != null) {
        final nextIndex = (state.currentCameraIndex + 1) % state.tour!.cameraIds.length;
        state = state.copyWith(currentCameraIndex: nextIndex);
      }
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }
}

final activeTourProvider = StateNotifierProvider<ActiveTourNotifier, ActiveTourState>(
  (ref) => ActiveTourNotifier(),
);
