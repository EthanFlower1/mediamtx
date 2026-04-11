import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/track.dart';
import 'auth_provider.dart';

// ---------------------------------------------------------------------------
// State
// ---------------------------------------------------------------------------
class TrackState {
  final Track? track;
  final List<Track> tracks;
  final bool loading;
  final bool starting;
  final String? error;

  const TrackState({
    this.track,
    this.tracks = const [],
    this.loading = false,
    this.starting = false,
    this.error,
  });

  TrackState copyWith({
    Track? track,
    List<Track>? tracks,
    bool? loading,
    bool? starting,
    String? error,
    bool clearTrack = false,
    bool clearError = false,
  }) {
    return TrackState(
      track: clearTrack ? null : (track ?? this.track),
      tracks: tracks ?? this.tracks,
      loading: loading ?? this.loading,
      starting: starting ?? this.starting,
      error: clearError ? null : (error ?? this.error),
    );
  }
}

// ---------------------------------------------------------------------------
// Notifier
// ---------------------------------------------------------------------------
class TrackNotifier extends StateNotifier<TrackState> {
  final Ref _ref;

  TrackNotifier(this._ref) : super(const TrackState());

  /// Start tracking a person from a detection event.
  Future<void> startTracking(int detectionId) async {
    state = state.copyWith(starting: true, clearError: true);

    try {
      final api = _ref.read(apiClientProvider);
      if (api == null) {
        state = state.copyWith(starting: false, error: 'Not authenticated');
        return;
      }

      final res = await api.post<dynamic>('/detections/$detectionId/track');
      final data = res.data as Map<String, dynamic>;
      final trackJson = data['track'] as Map<String, dynamic>;
      final track = Track.fromJson(trackJson);

      state = state.copyWith(
        starting: false,
        track: track,
        clearError: true,
      );
    } catch (e) {
      state = state.copyWith(
        starting: false,
        error: e.toString(),
      );
    }
  }

  /// Fetch a track by ID.
  Future<void> fetchTrack(int trackId) async {
    state = state.copyWith(loading: true, clearError: true);

    try {
      final api = _ref.read(apiClientProvider);
      if (api == null) {
        state = state.copyWith(loading: false, error: 'Not authenticated');
        return;
      }

      final res = await api.get<dynamic>('/tracks/$trackId');
      final data = res.data as Map<String, dynamic>;
      final trackJson = data['track'] as Map<String, dynamic>;
      final track = Track.fromJson(trackJson);

      state = state.copyWith(loading: false, track: track, clearError: true);
    } catch (e) {
      state = state.copyWith(loading: false, error: e.toString());
    }
  }

  /// Fetch recent tracks.
  Future<void> fetchTracks({int limit = 50}) async {
    state = state.copyWith(loading: true, clearError: true);

    try {
      final api = _ref.read(apiClientProvider);
      if (api == null) {
        state = state.copyWith(loading: false, error: 'Not authenticated');
        return;
      }

      final res = await api.get<dynamic>(
        '/tracks',
        queryParameters: {'limit': limit},
      );
      final data = res.data as Map<String, dynamic>;
      final tracksJson = data['tracks'] as List<dynamic>;
      final tracks = tracksJson
          .map((e) => Track.fromJson(e as Map<String, dynamic>))
          .toList();

      state = state.copyWith(loading: false, tracks: tracks, clearError: true);
    } catch (e) {
      state = state.copyWith(loading: false, error: e.toString());
    }
  }

  void clearTrack() {
    state = state.copyWith(clearTrack: true);
  }
}

// ---------------------------------------------------------------------------
// Provider
// ---------------------------------------------------------------------------
final trackProvider = StateNotifierProvider<TrackNotifier, TrackState>((ref) {
  return TrackNotifier(ref);
});
