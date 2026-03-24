import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/notification_event.dart';
import '../services/websocket_service.dart';
import 'auth_provider.dart';

class NotificationState {
  final List<NotificationEvent> history;
  final int unreadCount;
  final bool wsConnected;

  const NotificationState({
    this.history = const [],
    this.unreadCount = 0,
    this.wsConnected = false,
  });

  NotificationState copyWith({
    List<NotificationEvent>? history,
    int? unreadCount,
    bool? wsConnected,
  }) {
    return NotificationState(
      history: history ?? this.history,
      unreadCount: unreadCount ?? this.unreadCount,
      wsConnected: wsConnected ?? this.wsConnected,
    );
  }
}

class NotificationsNotifier extends StateNotifier<NotificationState> {
  WebSocketService? _webSocket;
  StreamSubscription<NotificationEvent>? _eventsSub;
  StreamSubscription<bool>? _connectionSub;

  NotificationsNotifier() : super(const NotificationState());

  WebSocketService? get webSocket => _webSocket;

  void connect(String serverUrl) {
    _cleanup();

    _webSocket = WebSocketService(serverUrl: serverUrl);

    _connectionSub = _webSocket!.connectionState.listen((connected) {
      if (mounted) {
        state = state.copyWith(wsConnected: connected);
      }
    });

    _eventsSub = _webSocket!.events.listen((event) {
      if (mounted) {
        final updated = [event, ...state.history];
        final capped =
            updated.length > 100 ? updated.sublist(0, 100) : updated;
        state = state.copyWith(
          history: capped,
          unreadCount: state.unreadCount + 1,
        );
      }
    });

    _webSocket!.connect();
  }

  void markAllRead() {
    state = state.copyWith(unreadCount: 0);
  }

  void _cleanup() {
    _eventsSub?.cancel();
    _connectionSub?.cancel();
    _webSocket?.dispose();
    _webSocket = null;
    _eventsSub = null;
    _connectionSub = null;
  }

  @override
  void dispose() {
    _cleanup();
    super.dispose();
  }
}

final notificationsProvider =
    StateNotifierProvider<NotificationsNotifier, NotificationState>((ref) {
  final notifier = NotificationsNotifier();

  ref.listen<AuthState>(authProvider, (previous, next) {
    if (next.status == AuthStatus.authenticated &&
        next.serverUrl != null) {
      notifier.connect(next.serverUrl!);
    }
  }, fireImmediately: true);

  return notifier;
});
