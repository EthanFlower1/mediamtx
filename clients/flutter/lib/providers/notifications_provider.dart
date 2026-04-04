import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../models/notification_event.dart';
import '../models/user_preferences.dart';
import '../services/websocket_service.dart';
import 'auth_provider.dart';
import 'user_preferences_provider.dart';

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
  static const _readIdsKey = 'nvr_read_notification_ids';
  static const _maxReadIds = 200;

  WebSocketService? _webSocket;
  StreamSubscription<NotificationEvent>? _eventsSub;
  StreamSubscription<bool>? _connectionSub;
  Set<String> _readIds = {};

  NotificationsNotifier() : super(const NotificationState()) {
    _loadReadIds();
  }

  WebSocketService? get webSocket => _webSocket;

  Future<void> _loadReadIds() async {
    final prefs = await SharedPreferences.getInstance();
    final ids = prefs.getStringList(_readIdsKey);
    if (ids != null) {
      _readIds = ids.toSet();
    }
  }

  Future<void> _saveReadIds() async {
    final prefs = await SharedPreferences.getInstance();
    // Cap stored IDs to prevent unbounded growth
    final ids = _readIds.toList();
    if (ids.length > _maxReadIds) {
      _readIds = ids.sublist(ids.length - _maxReadIds).toSet();
    }
    await prefs.setStringList(_readIdsKey, _readIds.toList());
  }

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
        // Apply persisted read state to incoming events
        final isRead = _readIds.contains(event.id);
        final markedEvent = isRead ? event.copyWith(isRead: true) : event;

        final updated = [markedEvent, ...state.history];
        final capped =
            updated.length > 100 ? updated.sublist(0, 100) : updated;
        final unread = capped.where((e) => !e.isRead).length;
        state = state.copyWith(
          history: capped,
          unreadCount: unread,
        );
      }
    });

    _webSocket!.connect();
  }

  void markAllRead() {
    final updated =
        state.history.map((e) => e.copyWith(isRead: true)).toList();
    for (final e in updated) {
      _readIds.add(e.id);
    }
    state = NotificationState(
      history: updated,
      unreadCount: 0,
      wsConnected: state.wsConnected,
    );
    _saveReadIds();
  }

  void markRead(int index) {
    if (index < 0 || index >= state.history.length) return;
    final event = state.history[index];
    if (event.isRead) return;
    final updated = List<NotificationEvent>.from(state.history);
    updated[index] = event.copyWith(isRead: true);
    _readIds.add(event.id);
    state = NotificationState(
      history: updated,
      unreadCount: (state.unreadCount - 1).clamp(0, state.unreadCount),
      wsConnected: state.wsConnected,
    );
    _saveReadIds();
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

/// Maps a notification event type string to a [NotificationEventType].
NotificationEventType? _mapEventType(String type) {
  switch (type) {
    case 'motion':
      return NotificationEventType.motion;
    case 'person_detected':
      return NotificationEventType.personDetected;
    case 'vehicle_detected':
      return NotificationEventType.vehicleDetected;
    case 'animal_detected':
      return NotificationEventType.animalDetected;
    case 'camera_offline':
      return NotificationEventType.cameraOffline;
    case 'camera_online':
      return NotificationEventType.cameraOnline;
    case 'recording_error':
      return NotificationEventType.recordingError;
    case 'storage_warning':
      return NotificationEventType.storageWarning;
    default:
      return null; // Unknown types pass through (not filtered)
  }
}

/// Notifications filtered by the user's enabled notification preferences.
final filteredNotificationsProvider = Provider<List<NotificationEvent>>((ref) {
  final allEvents = ref.watch(notificationsProvider).history;
  final enabledTypes = ref.watch(
    userPreferencesProvider.select((p) => p.enabledNotifications),
  );
  return allEvents.where((event) {
    final mapped = _mapEventType(event.type);
    // If the event type is not in our enum, show it (don't hide unknown types)
    if (mapped == null) return true;
    return enabledTypes.contains(mapped);
  }).toList();
});
