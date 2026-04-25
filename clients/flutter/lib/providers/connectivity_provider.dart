import 'dart:async';
import 'package:dio/dio.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'auth_provider.dart';

enum ConnectivityStatus { online, offline, reconnecting }

class ConnectivityState {
  final ConnectivityStatus status;
  final DateTime? lastOnline;

  const ConnectivityState({
    required this.status,
    this.lastOnline,
  });

  bool get isOnline => status == ConnectivityStatus.online;
  bool get isOffline =>
      status == ConnectivityStatus.offline ||
      status == ConnectivityStatus.reconnecting;

  ConnectivityState copyWith({
    ConnectivityStatus? status,
    DateTime? lastOnline,
  }) {
    return ConnectivityState(
      status: status ?? this.status,
      lastOnline: lastOnline ?? this.lastOnline,
    );
  }
}

class ConnectivityNotifier extends StateNotifier<ConnectivityState> {
  final Dio? _dio;
  Timer? _pollTimer;
  static const _pollInterval = Duration(seconds: 10);
  static const _reconnectInterval = Duration(seconds: 5);

  /// Normal constructor that polls a real server.
  ConnectivityNotifier(String serverUrl)
      : _dio = Dio(BaseOptions(
          baseUrl: serverUrl,
          connectTimeout: const Duration(seconds: 3),
          receiveTimeout: const Duration(seconds: 3),
        )),
        super(const ConnectivityState(status: ConnectivityStatus.online)) {
    _startPolling();
  }

  /// Constructor for when no server URL is configured.
  ConnectivityNotifier.noServer()
      : _dio = null,
        super(const ConnectivityState(status: ConnectivityStatus.offline));

  void _startPolling() {
    // Don't block startup — assume online, verify in background.
    Future.delayed(const Duration(seconds: 2), _poll);
    _pollTimer = Timer.periodic(_pollInterval, (_) => _poll());
  }

  bool _disposed = false;

  Future<void> _poll() async {
    if (_dio == null || _disposed) return;
    try {
      await _dio.get('/api/nvr/system/health');
      final wasOffline = state.status != ConnectivityStatus.online;
      state = ConnectivityState(
        status: ConnectivityStatus.online,
        lastOnline: DateTime.now(),
      );
      if (wasOffline) {
        _pollTimer?.cancel();
        _pollTimer = Timer.periodic(_pollInterval, (_) => _poll());
      }
    } catch (_) {
      if (state.status == ConnectivityStatus.online) {
        state = state.copyWith(status: ConnectivityStatus.offline);
        _pollTimer?.cancel();
        _pollTimer = Timer.periodic(_reconnectInterval, (_) => _poll());
      } else if (state.status == ConnectivityStatus.offline) {
        state = state.copyWith(status: ConnectivityStatus.reconnecting);
      }
    }
  }

  /// Force an immediate connectivity check.
  Future<void> checkNow() => _poll();

  @override
  void dispose() {
    _disposed = true;
    _pollTimer?.cancel();
    _dio?.close();
    super.dispose();
  }
}

final connectivityProvider =
    StateNotifierProvider<ConnectivityNotifier, ConnectivityState>((ref) {
  final auth = ref.watch(authProvider);
  final serverUrl = auth.serverUrl;
  if (serverUrl == null || serverUrl.isEmpty) {
    return ConnectivityNotifier.noServer();
  }
  return ConnectivityNotifier(serverUrl);
});
