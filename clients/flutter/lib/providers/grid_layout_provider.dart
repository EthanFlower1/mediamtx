import 'dart:async';
import 'dart:convert';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'auth_provider.dart';
import 'user_preferences_provider.dart';

/// A single grid layout with a name, grid size, and slot assignments.
class GridLayout {
  const GridLayout({
    this.name = '',
    this.gridSize = 4,
    this.slots = const {},
  });

  final String name;
  final int gridSize; // NxN
  final Map<int, String> slots; // slot index -> camera ID

  int get totalSlots => gridSize * gridSize;

  GridLayout copyWith({String? name, int? gridSize, Map<int, String>? slots}) {
    return GridLayout(
      name: name ?? this.name,
      gridSize: gridSize ?? this.gridSize,
      slots: slots ?? this.slots,
    );
  }

  Map<String, dynamic> toJson() => {
    'name': name,
    'gridSize': gridSize,
    'slots': slots.map((k, v) => MapEntry(k.toString(), v)),
  };

  factory GridLayout.fromJson(Map<String, dynamic> json) {
    final slotsRaw = json['slots'] as Map<String, dynamic>? ?? {};
    return GridLayout(
      name: json['name'] as String? ?? '',
      gridSize: json['gridSize'] as int? ?? 4,
      slots: slotsRaw.map((k, v) => MapEntry(int.parse(k), v as String)),
    );
  }
}

/// State that holds the active layout plus a list of saved named layouts.
class GridLayoutState {
  const GridLayoutState({
    required this.active,
    this.savedLayouts = const [],
  });

  final GridLayout active;
  final List<GridLayout> savedLayouts;

  GridLayoutState copyWith({
    GridLayout? active,
    List<GridLayout>? savedLayouts,
  }) {
    return GridLayoutState(
      active: active ?? this.active,
      savedLayouts: savedLayouts ?? this.savedLayouts,
    );
  }
}

class GridLayoutNotifier extends StateNotifier<GridLayoutState> {
  GridLayoutNotifier(this._userId, {int initialGridSize = 2})
      : super(GridLayoutState(active: GridLayout(gridSize: initialGridSize))) {
    _load();
  }

  final String _userId;
  Timer? _saveActiveDebounce;
  Timer? _saveSavedDebounce;

  String get _activeKey => 'grid_layout_$_userId';
  String get _savedKey => 'grid_layouts_saved_$_userId';

  // --- Convenience accessors for the active layout ---
  GridLayout get _layout => state.active;

  Future<void> _load() async {
    final prefs = await SharedPreferences.getInstance();

    // Load active layout.
    final activeJson = prefs.getString(_activeKey);
    GridLayout active = const GridLayout(gridSize: 2);
    if (activeJson != null) {
      active = GridLayout.fromJson(
        jsonDecode(activeJson) as Map<String, dynamic>,
      );
    }

    // Load saved layouts.
    final savedJson = prefs.getString(_savedKey);
    List<GridLayout> saved = [];
    if (savedJson != null) {
      final list = jsonDecode(savedJson) as List<dynamic>;
      saved = list
          .map((e) => GridLayout.fromJson(e as Map<String, dynamic>))
          .toList();
    }

    state = GridLayoutState(active: active, savedLayouts: saved);
  }

  void _saveActive() {
    _saveActiveDebounce?.cancel();
    _saveActiveDebounce = Timer(const Duration(milliseconds: 500), () async {
      final prefs = await SharedPreferences.getInstance();
      await prefs.setString(_activeKey, jsonEncode(_layout.toJson()));
    });
  }

  void _saveSavedLayouts() {
    _saveSavedDebounce?.cancel();
    _saveSavedDebounce = Timer(const Duration(milliseconds: 500), () async {
      final prefs = await SharedPreferences.getInstance();
      await prefs.setString(
        _savedKey,
        jsonEncode(state.savedLayouts.map((l) => l.toJson()).toList()),
      );
    });
  }

  @override
  void dispose() {
    _saveActiveDebounce?.cancel();
    _saveSavedDebounce?.cancel();
    super.dispose();
  }

  // --- Active layout mutations ---

  void setGridSize(int size) {
    final newSlots = Map<int, String>.from(_layout.slots)
      ..removeWhere((k, _) => k >= size * size);
    state = state.copyWith(
      active: _layout.copyWith(gridSize: size, slots: newSlots),
    );
    _saveActive();
  }

  void assignCamera(int slot, String cameraId) {
    final newSlots = Map<int, String>.from(_layout.slots)
      ..removeWhere((_, v) => v == cameraId);
    newSlots[slot] = cameraId;
    state = state.copyWith(active: _layout.copyWith(slots: newSlots));
    _saveActive();
  }

  void addCamera(String cameraId) {
    final newSlots = Map<int, String>.from(_layout.slots);
    if (newSlots.values.contains(cameraId)) return; // already present
    // Find the next available slot index.
    int next = 0;
    while (newSlots.containsKey(next)) next++;
    newSlots[next] = cameraId;
    state = state.copyWith(active: _layout.copyWith(slots: newSlots));
    _saveActive();
  }

  void removeCamera(int slot) {
    final newSlots = Map<int, String>.from(_layout.slots)..remove(slot);
    // Re-compact indices so there are no gaps.
    final compacted = <int, String>{};
    int i = 0;
    for (final cameraId in newSlots.values) {
      compacted[i++] = cameraId;
    }
    state = state.copyWith(active: _layout.copyWith(slots: compacted));
    _saveActive();
  }

  void swapSlots(int from, int to) {
    final newSlots = Map<int, String>.from(_layout.slots);
    final temp = newSlots[to];
    if (newSlots.containsKey(from)) {
      newSlots[to] = newSlots[from]!;
    }
    if (temp != null) {
      newSlots[from] = temp;
    } else {
      newSlots.remove(from);
    }
    state = state.copyWith(active: _layout.copyWith(slots: newSlots));
    _saveActive();
  }

  void fillFromGroup(List<String> cameraIds) {
    // Auto-compute the smallest grid that fits all cameras.
    int cols = 1;
    while (cols * cols < cameraIds.length) {
      cols++;
    }

    final newSlots = <int, String>{};
    for (int i = 0; i < cameraIds.length; i++) {
      newSlots[i] = cameraIds[i];
    }
    state = state.copyWith(
      active: _layout.copyWith(gridSize: cols, slots: newSlots),
    );
    _saveActive();
  }

  // --- Named layout operations ---

  /// Save the current active layout under a name.
  void saveLayout(String name) {
    final layoutToSave = _layout.copyWith(name: name);
    final existing = List<GridLayout>.from(state.savedLayouts);

    // Replace if same name exists.
    final idx = existing.indexWhere((l) => l.name == name);
    if (idx >= 0) {
      existing[idx] = layoutToSave;
    } else {
      existing.add(layoutToSave);
    }

    state = state.copyWith(
      active: layoutToSave,
      savedLayouts: existing,
    );
    _saveActive();
    _saveSavedLayouts();
  }

  /// Load a saved layout by name, making it the active layout.
  void loadLayout(String name) {
    final layout = state.savedLayouts.where((l) => l.name == name).firstOrNull;
    if (layout != null) {
      state = state.copyWith(active: layout);
      _saveActive();
    }
  }

  /// Delete a saved layout by name.
  void deleteLayout(String name) {
    final existing = List<GridLayout>.from(state.savedLayouts)
      ..removeWhere((l) => l.name == name);
    state = state.copyWith(savedLayouts: existing);
    _saveSavedLayouts();
  }

  /// Rename a saved layout.
  void renameLayout(String oldName, String newName) {
    final existing = List<GridLayout>.from(state.savedLayouts);
    final idx = existing.indexWhere((l) => l.name == oldName);
    if (idx >= 0) {
      existing[idx] = existing[idx].copyWith(name: newName);
      state = state.copyWith(savedLayouts: existing);
      // If active layout has the old name, update it too.
      if (_layout.name == oldName) {
        state = state.copyWith(active: _layout.copyWith(name: newName));
        _saveActive();
      }
      _saveSavedLayouts();
    }
  }
}

final gridLayoutProvider =
    StateNotifierProvider<GridLayoutNotifier, GridLayoutState>((ref) {
  final userId = ref.watch(authProvider).user?.id ?? 'default';
  final preferredSize = ref.watch(
    userPreferencesProvider.select((p) => p.preferredGridSize),
  );
  return GridLayoutNotifier(userId, initialGridSize: preferredSize);
});
