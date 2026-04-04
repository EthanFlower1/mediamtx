import 'dart:convert';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'auth_provider.dart';
import 'user_preferences_provider.dart';

class GridLayout {
  const GridLayout({this.gridSize = 4, this.slots = const {}});
  final int gridSize; // NxN
  final Map<int, String> slots; // slot index → camera ID

  int get totalSlots => gridSize * gridSize;

  GridLayout copyWith({int? gridSize, Map<int, String>? slots}) {
    return GridLayout(
      gridSize: gridSize ?? this.gridSize,
      slots: slots ?? this.slots,
    );
  }

  Map<String, dynamic> toJson() => {
    'gridSize': gridSize,
    'slots': slots.map((k, v) => MapEntry(k.toString(), v)),
  };

  factory GridLayout.fromJson(Map<String, dynamic> json) {
    final slotsRaw = json['slots'] as Map<String, dynamic>? ?? {};
    return GridLayout(
      gridSize: json['gridSize'] as int? ?? 4,
      slots: slotsRaw.map((k, v) => MapEntry(int.parse(k), v as String)),
    );
  }
}

class GridLayoutNotifier extends StateNotifier<GridLayout> {
  GridLayoutNotifier(this._userId, {int initialGridSize = 2})
      : super(GridLayout(gridSize: initialGridSize)) {
    _load();
  }

  final String _userId;

  String get _key => 'grid_layout_$_userId';

  Future<void> _load() async {
    final prefs = await SharedPreferences.getInstance();
    final json = prefs.getString(_key);
    if (json != null) {
      state = GridLayout.fromJson(jsonDecode(json) as Map<String, dynamic>);
    }
  }

  Future<void> _save() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(_key, jsonEncode(state.toJson()));
  }

  void setGridSize(int size) {
    // Preserve slots that still fit
    final newSlots = Map<int, String>.from(state.slots)
      ..removeWhere((k, _) => k >= size * size);
    state = state.copyWith(gridSize: size, slots: newSlots);
    _save();
  }

  void assignCamera(int slot, String cameraId) {
    // Remove camera from any existing slot
    final newSlots = Map<int, String>.from(state.slots)
      ..removeWhere((_, v) => v == cameraId);
    newSlots[slot] = cameraId;
    state = state.copyWith(slots: newSlots);
    _save();
  }

  void removeCamera(int slot) {
    final newSlots = Map<int, String>.from(state.slots)..remove(slot);
    state = state.copyWith(slots: newSlots);
    _save();
  }

  void swapSlots(int from, int to) {
    final newSlots = Map<int, String>.from(state.slots);
    final temp = newSlots[to];
    if (newSlots.containsKey(from)) newSlots[to] = newSlots[from]!;
    if (temp != null) newSlots[from] = temp; else newSlots.remove(from);
    state = state.copyWith(slots: newSlots);
    _save();
  }

  void fillFromGroup(List<String> cameraIds) {
    final newSlots = <int, String>{};
    for (int i = 0; i < cameraIds.length && i < state.totalSlots; i++) {
      newSlots[i] = cameraIds[i];
    }
    state = state.copyWith(slots: newSlots);
    _save();
  }
}

final gridLayoutProvider = StateNotifierProvider<GridLayoutNotifier, GridLayout>((ref) {
  final userId = ref.watch(authProvider).user?.id ?? 'default';
  final preferredSize = ref.watch(
    userPreferencesProvider.select((p) => p.preferredGridSize),
  );
  return GridLayoutNotifier(userId, initialGridSize: preferredSize);
});
