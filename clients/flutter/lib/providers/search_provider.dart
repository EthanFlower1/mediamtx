import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/search_result.dart';
import '../models/saved_clip.dart';
import 'auth_provider.dart';

class SearchState {
  final String query;
  final List<SearchResult> results;
  final bool searching;
  final bool searched;
  final String? error;

  const SearchState({
    this.query = '',
    this.results = const [],
    this.searching = false,
    this.searched = false,
    this.error,
  });

  SearchState copyWith({
    String? query,
    List<SearchResult>? results,
    bool? searching,
    bool? searched,
    String? error,
    bool clearError = false,
  }) {
    return SearchState(
      query: query ?? this.query,
      results: results ?? this.results,
      searching: searching ?? this.searching,
      searched: searched ?? this.searched,
      error: clearError ? null : (error ?? this.error),
    );
  }
}

class SearchNotifier extends StateNotifier<SearchState> {
  final Ref _ref;

  SearchNotifier(this._ref) : super(const SearchState());

  Future<void> search(String query) async {
    if (query.trim().isEmpty) return;

    state = state.copyWith(
      query: query,
      searching: true,
      searched: false,
      results: [],
      clearError: true,
    );

    try {
      final api = _ref.read(apiClientProvider);
      if (api == null) {
        state = state.copyWith(
          searching: false,
          error: 'Not authenticated',
        );
        return;
      }

      final res = await api.get<dynamic>('/search',
        queryParameters: {
          'q': query.trim(),
          'limit': 20,
        },
        receiveTimeout: const Duration(seconds: 60),
      );

      final data = res.data;
      List<dynamic> items = [];
      if (data is List) {
        items = data;
      } else if (data is Map) {
        items = (data['results'] as List?) ?? [];
      }

      final results = items
          .map((e) => SearchResult.fromJson(e as Map<String, dynamic>))
          .toList();

      state = state.copyWith(
        searching: false,
        searched: true,
        results: results,
        clearError: true,
      );
    } catch (e) {
      state = state.copyWith(
        searching: false,
        searched: true,
        error: e.toString(),
      );
    }
  }

  void clear() {
    state = const SearchState();
  }
}

final searchProvider =
    StateNotifierProvider<SearchNotifier, SearchState>((ref) {
  return SearchNotifier(ref);
});

final savedClipsProvider = FutureProvider<List<SavedClip>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];

  final res = await api.get<dynamic>('/saved-clips');

  final data = res.data;
  if (data == null) return [];
  final list = data is List ? data : (data['clips'] as List? ?? []);
  return list
      .map((e) => SavedClip.fromJson(e as Map<String, dynamic>))
      .toList();
});
