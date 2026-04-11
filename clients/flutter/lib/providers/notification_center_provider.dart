import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../api/notification_api.dart';
import 'auth_provider.dart';

/// State for the in-product notification center page.
class NotificationCenterState {
  final List<NotificationItem> notifications;
  final int total;
  final int offset;
  final int pageSize;
  final bool loading;
  final NotificationFilter filter;

  const NotificationCenterState({
    this.notifications = const [],
    this.total = 0,
    this.offset = 0,
    this.pageSize = 30,
    this.loading = false,
    this.filter = const NotificationFilter(),
  });

  NotificationCenterState copyWith({
    List<NotificationItem>? notifications,
    int? total,
    int? offset,
    int? pageSize,
    bool? loading,
    NotificationFilter? filter,
  }) {
    return NotificationCenterState(
      notifications: notifications ?? this.notifications,
      total: total ?? this.total,
      offset: offset ?? this.offset,
      pageSize: pageSize ?? this.pageSize,
      loading: loading ?? this.loading,
      filter: filter ?? this.filter,
    );
  }
}

class NotificationCenterNotifier extends StateNotifier<NotificationCenterState> {
  final NotificationApi _api;
  Timer? _refreshTimer;

  NotificationCenterNotifier(this._api) : super(const NotificationCenterState());

  Future<void> fetch({int? offset}) async {
    state = state.copyWith(loading: true);
    try {
      final page = await _api.list(
        filter: state.filter,
        limit: state.pageSize,
        offset: offset ?? state.offset,
      );
      state = state.copyWith(
        notifications: page.notifications,
        total: page.total,
        offset: page.offset,
        loading: false,
      );
    } catch (_) {
      state = state.copyWith(loading: false);
    }
  }

  void setFilter(NotificationFilter filter) {
    state = state.copyWith(filter: filter, offset: 0);
    fetch(offset: 0);
  }

  void nextPage() {
    final next = state.offset + state.pageSize;
    if (next < state.total) fetch(offset: next);
  }

  void prevPage() {
    final prev = (state.offset - state.pageSize).clamp(0, state.total);
    fetch(offset: prev);
  }

  Future<void> markRead(List<String> ids) async {
    await _api.markRead(ids);
    final now = DateTime.now().toIso8601String();
    state = state.copyWith(
      notifications: state.notifications
          .map((n) => ids.contains(n.id) ? n.copyWith(readAt: now) : n)
          .toList(),
    );
  }

  Future<void> markUnread(List<String> ids) async {
    await _api.markUnread(ids);
    state = state.copyWith(
      notifications: state.notifications
          .map((n) => ids.contains(n.id) ? n.copyWith(clearReadAt: true) : n)
          .toList(),
    );
  }

  Future<void> markAllRead() async {
    await _api.markAllRead();
    final now = DateTime.now().toIso8601String();
    state = state.copyWith(
      notifications: state.notifications
          .map((n) => n.isRead ? n : n.copyWith(readAt: now))
          .toList(),
    );
  }

  Future<void> archive(List<String> ids) async {
    await _api.archive(ids);
    state = state.copyWith(
      notifications: state.notifications.where((n) => !ids.contains(n.id)).toList(),
      total: (state.total - ids.length).clamp(0, state.total),
    );
  }

  Future<void> restore(List<String> ids) async {
    await _api.restore(ids);
    state = state.copyWith(
      notifications: state.notifications.where((n) => !ids.contains(n.id)).toList(),
      total: (state.total - ids.length).clamp(0, state.total),
    );
  }

  Future<void> deleteNotifications(List<String> ids) async {
    await _api.deleteNotifications(ids);
    state = state.copyWith(
      notifications: state.notifications.where((n) => !ids.contains(n.id)).toList(),
      total: (state.total - ids.length).clamp(0, state.total),
    );
  }

  /// Start periodic refresh (30s fallback).
  void startAutoRefresh() {
    _refreshTimer?.cancel();
    _refreshTimer = Timer.periodic(const Duration(seconds: 30), (_) {
      fetch();
    });
  }

  void stopAutoRefresh() {
    _refreshTimer?.cancel();
    _refreshTimer = null;
  }

  @override
  void dispose() {
    _refreshTimer?.cancel();
    super.dispose();
  }
}

/// Provider for the notification API client.
final notificationApiProvider = Provider<NotificationApi>((ref) {
  final apiClient = ref.watch(apiClientProvider);
  if (apiClient == null) {
    throw StateError('Cannot create NotificationApi: not authenticated');
  }
  return NotificationApi(apiClient);
});

/// Provider for the notification center state.
final notificationCenterProvider =
    StateNotifierProvider<NotificationCenterNotifier, NotificationCenterState>((ref) {
  final api = ref.watch(notificationApiProvider);
  return NotificationCenterNotifier(api);
});
