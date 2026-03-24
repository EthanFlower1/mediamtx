import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/user.dart';
import '../services/auth_service.dart';
import '../services/api_client.dart';

enum AuthStatus { unknown, serverNeeded, unauthenticated, authenticated }

class AuthState {
  final AuthStatus status;
  final User? user;
  final String? serverUrl;
  final String? error;

  const AuthState({
    required this.status,
    this.user,
    this.serverUrl,
    this.error,
  });

  AuthState copyWith({
    AuthStatus? status,
    User? user,
    String? serverUrl,
    String? error,
  }) {
    return AuthState(
      status: status ?? this.status,
      user: user ?? this.user,
      serverUrl: serverUrl ?? this.serverUrl,
      error: error ?? this.error,
    );
  }
}

class AuthNotifier extends StateNotifier<AuthState> {
  final AuthService _authService;
  Timer? _refreshTimer;

  AuthNotifier(this._authService)
      : super(const AuthState(status: AuthStatus.unknown)) {
    _init();
  }

  Future<void> _init() async {
    final serverUrl = await _authService.getServerUrl();
    if (serverUrl == null || serverUrl.isEmpty) {
      state = const AuthState(status: AuthStatus.serverNeeded);
      return;
    }

    final result = await _authService.refresh(serverUrl);
    if (result != null) {
      state = AuthState(
        status: AuthStatus.authenticated,
        user: result.user,
        serverUrl: serverUrl,
      );
      _scheduleRefresh(result.expiresIn);
    } else {
      state = AuthState(
        status: AuthStatus.unauthenticated,
        serverUrl: serverUrl,
      );
    }
  }

  Future<void> setServer(String url) async {
    await _authService.setServerUrl(url);
    state = AuthState(
      status: AuthStatus.unauthenticated,
      serverUrl: url,
    );
  }

  Future<void> login(String username, String password) async {
    final serverUrl = state.serverUrl;
    if (serverUrl == null) {
      state = state.copyWith(
        status: AuthStatus.unauthenticated,
        error: 'No server URL configured',
      );
      return;
    }
    try {
      final result = await _authService.login(serverUrl, username, password);
      state = AuthState(
        status: AuthStatus.authenticated,
        user: result.user,
        serverUrl: serverUrl,
      );
      _scheduleRefresh(result.expiresIn);
    } catch (e) {
      state = state.copyWith(
        status: AuthStatus.unauthenticated,
        error: e.toString(),
      );
    }
  }

  Future<void> logout() async {
    _refreshTimer?.cancel();
    _refreshTimer = null;
    final serverUrl = state.serverUrl;
    if (serverUrl != null) {
      await _authService.logout(serverUrl);
    }
    state = AuthState(
      status: AuthStatus.unauthenticated,
      serverUrl: serverUrl,
    );
  }

  void _scheduleRefresh(int expiresIn) {
    _refreshTimer?.cancel();
    final delay = Duration(seconds: expiresIn - 60);
    if (delay.isNegative) return;
    _refreshTimer = Timer(delay, () async {
      final serverUrl = state.serverUrl;
      if (serverUrl == null) return;
      final result = await _authService.refresh(serverUrl);
      if (result != null) {
        state = state.copyWith(
          status: AuthStatus.authenticated,
          user: result.user,
        );
        _scheduleRefresh(result.expiresIn);
      } else {
        state = AuthState(
          status: AuthStatus.unauthenticated,
          serverUrl: serverUrl,
        );
      }
    });
  }

  @override
  void dispose() {
    _refreshTimer?.cancel();
    super.dispose();
  }
}

final authServiceProvider = Provider<AuthService>((ref) => AuthService());

final authProvider = StateNotifierProvider<AuthNotifier, AuthState>((ref) {
  final authService = ref.watch(authServiceProvider);
  return AuthNotifier(authService);
});

final apiClientProvider = Provider<ApiClient?>((ref) {
  final auth = ref.watch(authProvider);
  if (auth.status != AuthStatus.authenticated || auth.serverUrl == null) {
    return null;
  }
  final authService = ref.watch(authServiceProvider);
  return ApiClient(serverUrl: auth.serverUrl!, authService: authService);
});
