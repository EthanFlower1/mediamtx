import 'package:flutter_riverpod/flutter_riverpod.dart';

class CameraPanelState {
  const CameraPanelState({
    this.isOpen = false,
    this.searchQuery = '',
    this.activeGroupId,
  });

  final bool isOpen;
  final String searchQuery;
  final String? activeGroupId;

  CameraPanelState copyWith({
    bool? isOpen,
    String? searchQuery,
    String? activeGroupId,
    bool clearGroupFilter = false,
  }) {
    return CameraPanelState(
      isOpen: isOpen ?? this.isOpen,
      searchQuery: searchQuery ?? this.searchQuery,
      activeGroupId: clearGroupFilter ? null : (activeGroupId ?? this.activeGroupId),
    );
  }
}

class CameraPanelNotifier extends StateNotifier<CameraPanelState> {
  CameraPanelNotifier() : super(const CameraPanelState());

  void toggle() => state = state.copyWith(isOpen: !state.isOpen);
  void open() => state = state.copyWith(isOpen: true);
  void close() => state = state.copyWith(isOpen: false);
  void setSearch(String query) => state = state.copyWith(searchQuery: query);
  void setGroupFilter(String? groupId) {
    if (groupId == state.activeGroupId) {
      state = state.copyWith(clearGroupFilter: true);
    } else {
      state = state.copyWith(activeGroupId: groupId);
    }
  }
}

final cameraPanelProvider =
    StateNotifierProvider<CameraPanelNotifier, CameraPanelState>(
  (ref) => CameraPanelNotifier(),
);
