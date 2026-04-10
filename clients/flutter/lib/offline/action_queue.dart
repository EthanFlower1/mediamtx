import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Categories of actions that can be queued while offline.
enum ActionType {
  /// An HTTP API call to be replayed when connectivity is restored.
  apiCall,

  /// A local state mutation to sync upstream.
  stateUpdate,

  /// A notification to deliver once online.
  notification,
}

/// A single action waiting to be executed when the device reconnects.
class QueuedAction {
  QueuedAction({
    required this.id,
    required this.actionType,
    required this.payload,
    required this.createdAt,
    this.retryCount = 0,
  });

  final String id;
  final ActionType actionType;
  final Map<String, dynamic> payload;
  final DateTime createdAt;
  final int retryCount;

  /// Return a copy with an incremented retry count.
  QueuedAction incrementRetry() => QueuedAction(
        id: id,
        actionType: actionType,
        payload: payload,
        createdAt: createdAt,
        retryCount: retryCount + 1,
      );

  @override
  bool operator ==(Object other) =>
      identical(this, other) || other is QueuedAction && other.id == id;

  @override
  int get hashCode => id.hashCode;
}

/// FIFO queue of [QueuedAction]s accumulated while offline.
///
/// When connectivity is restored the consumer should [dequeue] or [drain] the
/// queue and replay the actions against the server.
class ActionQueue extends StateNotifier<List<QueuedAction>> {
  ActionQueue() : super([]);

  /// Maximum number of retries before an action is dropped.
  static const maxRetries = 3;

  /// Add an action to the back of the queue.
  void enqueue(QueuedAction action) {
    state = [...state, action];
  }

  /// Remove and return the action at the front, or `null` if empty.
  QueuedAction? dequeue() {
    if (state.isEmpty) return null;
    final first = state.first;
    state = state.sublist(1);
    return first;
  }

  /// Return the front action without removing it, or `null` if empty.
  QueuedAction? peek() => state.isEmpty ? null : state.first;

  /// Remove and return all queued actions, leaving the queue empty.
  List<QueuedAction> drain() {
    final all = List<QueuedAction>.from(state);
    state = [];
    return all;
  }

  /// Remove a specific action by its [id].
  void removeById(String id) {
    state = state.where((a) => a.id != id).toList();
  }
}

/// Riverpod provider for [ActionQueue].
final actionQueueProvider =
    StateNotifierProvider<ActionQueue, List<QueuedAction>>(
  (ref) => ActionQueue(),
);
