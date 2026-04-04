import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../services/pending_actions_service.dart';
import '../services/api_client.dart';
import 'auth_provider.dart';
import 'connectivity_provider.dart';

class PendingActionsState {
  final int pendingCount;
  final bool isSyncing;

  const PendingActionsState({
    this.pendingCount = 0,
    this.isSyncing = false,
  });

  PendingActionsState copyWith({int? pendingCount, bool? isSyncing}) {
    return PendingActionsState(
      pendingCount: pendingCount ?? this.pendingCount,
      isSyncing: isSyncing ?? this.isSyncing,
    );
  }
}

class PendingActionsNotifier extends StateNotifier<PendingActionsState> {
  final PendingActionsService _service;
  final ApiClient? _api;
  final Ref _ref;

  PendingActionsNotifier(this._service, this._api, this._ref)
      : super(const PendingActionsState()) {
    _loadCount();
    _listenForReconnect();
  }

  Future<void> _loadCount() async {
    final count = await _service.pendingCount;
    if (mounted) {
      state = state.copyWith(pendingCount: count);
    }
  }

  void _listenForReconnect() {
    _ref.listen<ConnectivityState>(connectivityProvider, (previous, next) {
      if (previous != null &&
          previous.isOffline &&
          next.isOnline &&
          state.pendingCount > 0) {
        flushQueue();
      }
    });
  }

  /// Queue an action for later sync.
  Future<void> enqueue(PendingAction action) async {
    await _service.enqueue(action);
    await _loadCount();
  }

  /// Attempt to flush all pending actions to the server.
  Future<void> flushQueue() async {
    if (_api == null || state.isSyncing) return;
    state = state.copyWith(isSyncing: true);

    final actions = await _service.getAll();
    var flushed = 0;

    for (final action in actions) {
      try {
        await _executeAction(action);
        flushed++;
      } catch (_) {
        // Stop flushing on first failure — remaining stay queued
        break;
      }
    }

    // Remove successfully synced actions from front of queue
    for (var i = 0; i < flushed; i++) {
      await _service.removeAt(0);
    }

    await _loadCount();
    state = state.copyWith(isSyncing: false);
  }

  Future<void> _executeAction(PendingAction action) async {
    final api = _api;
    if (api == null) throw Exception('No API client');

    switch (action.type) {
      case 'create_bookmark':
        await api.post('/bookmarks', data: action.payload);
        break;
      case 'delete_bookmark':
        final id = action.payload['id'];
        await api.delete('/bookmarks/$id');
        break;
      case 'toggle_favorite':
        final cameraId = action.payload['camera_id'];
        final favorited = action.payload['favorited'] as bool;
        await api.put('/cameras/$cameraId', data: {'favorited': favorited});
        break;
      default:
        // Unknown action type — discard so it doesn't block the queue
        break;
    }
  }
}

final pendingActionsServiceProvider =
    Provider<PendingActionsService>((_) => PendingActionsService());

final pendingActionsProvider =
    StateNotifierProvider<PendingActionsNotifier, PendingActionsState>((ref) {
  final service = ref.watch(pendingActionsServiceProvider);
  final api = ref.watch(apiClientProvider);
  return PendingActionsNotifier(service, api, ref);
});
