import 'dart:convert';
import 'package:shared_preferences/shared_preferences.dart';

/// Represents a queued action to be synced when connectivity is restored.
class PendingAction {
  final String type; // e.g. 'create_bookmark', 'delete_bookmark', 'toggle_favorite'
  final Map<String, dynamic> payload;
  final DateTime createdAt;

  PendingAction({
    required this.type,
    required this.payload,
    DateTime? createdAt,
  }) : createdAt = createdAt ?? DateTime.now();

  Map<String, dynamic> toJson() => {
        'type': type,
        'payload': payload,
        'created_at': createdAt.toIso8601String(),
      };

  factory PendingAction.fromJson(Map<String, dynamic> json) {
    return PendingAction(
      type: json['type'] as String,
      payload: Map<String, dynamic>.from(json['payload'] as Map),
      createdAt: DateTime.parse(json['created_at'] as String),
    );
  }
}

/// Queues write operations locally and flushes them when online.
class PendingActionsService {
  static const _queueKey = 'pending_actions_queue';

  /// Add an action to the pending queue.
  Future<void> enqueue(PendingAction action) async {
    final prefs = await SharedPreferences.getInstance();
    final existing = prefs.getStringList(_queueKey) ?? [];
    existing.add(jsonEncode(action.toJson()));
    await prefs.setStringList(_queueKey, existing);
  }

  /// Get all pending actions.
  Future<List<PendingAction>> getAll() async {
    final prefs = await SharedPreferences.getInstance();
    final list = prefs.getStringList(_queueKey) ?? [];
    return list
        .map((s) =>
            PendingAction.fromJson(jsonDecode(s) as Map<String, dynamic>))
        .toList();
  }

  /// Get the number of pending actions.
  Future<int> get pendingCount async {
    final prefs = await SharedPreferences.getInstance();
    final list = prefs.getStringList(_queueKey) ?? [];
    return list.length;
  }

  /// Clear the entire queue (after successful sync).
  Future<void> clearAll() async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(_queueKey);
  }

  /// Remove a single action by index (after it's synced).
  Future<void> removeAt(int index) async {
    final prefs = await SharedPreferences.getInstance();
    final existing = prefs.getStringList(_queueKey) ?? [];
    if (index >= 0 && index < existing.length) {
      existing.removeAt(index);
      await prefs.setStringList(_queueKey, existing);
    }
  }
}
