import 'package:dio/dio.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import '../models/user.dart';

class AuthResult {
  final String accessToken;
  final String refreshToken;
  final int expiresIn;
  final User user;

  AuthResult({required this.accessToken, required this.refreshToken, required this.expiresIn, required this.user});
}

class AuthService {
  final FlutterSecureStorage _storage;
  final Dio _dio;

  static const _serverUrlKey = 'nvr_server_url';
  static const _accessTokenKey = 'nvr_access_token';
  static const _refreshTokenKey = 'nvr_refresh_token';

  AuthService({FlutterSecureStorage? storage, Dio? dio})
      : _storage = storage ?? const FlutterSecureStorage(),
        _dio = dio ?? Dio();

  Future<String?> getServerUrl() => _storage.read(key: _serverUrlKey);
  Future<void> setServerUrl(String url) => _storage.write(key: _serverUrlKey, value: url);
  Future<String?> getAccessToken() => _storage.read(key: _accessTokenKey);
  Future<String?> getRefreshToken() => _storage.read(key: _refreshTokenKey);

  /// Validate server by calling /system/health
  Future<bool> validateServer(String url) async {
    try {
      final res = await _dio.get('$url/api/nvr/system/health',
          options: Options(receiveTimeout: const Duration(seconds: 5)));
      return res.statusCode == 200;
    } catch (_) {
      return false;
    }
  }

  /// Login. Stores tokens in secure storage.
  Future<AuthResult> login(String serverUrl, String username, String password) async {
    final res = await _dio.post('$serverUrl/api/nvr/auth/login', data: {
      'username': username,
      'password': password,
    });
    final data = res.data as Map<String, dynamic>;
    final result = AuthResult(
      accessToken: data['access_token'] as String,
      refreshToken: data['refresh_token'] as String,
      expiresIn: data['expires_in'] as int,
      user: User.fromJson(data['user'] as Map<String, dynamic>),
    );
    await _storage.write(key: _accessTokenKey, value: result.accessToken);
    await _storage.write(key: _refreshTokenKey, value: result.refreshToken);
    return result;
  }

  /// Refresh access token using stored refresh token (sent in JSON body for mobile).
  Future<AuthResult?> refresh(String serverUrl) async {
    final refreshToken = await _storage.read(key: _refreshTokenKey);
    if (refreshToken == null) return null;
    try {
      final res = await _dio.post('$serverUrl/api/nvr/auth/refresh', data: {
        'refresh_token': refreshToken,
      });
      final data = res.data as Map<String, dynamic>;
      final newAccessToken = data['access_token'] as String;
      await _storage.write(key: _accessTokenKey, value: newAccessToken);
      if (data.containsKey('refresh_token')) {
        await _storage.write(key: _refreshTokenKey, value: data['refresh_token'] as String);
      }
      return AuthResult(
        accessToken: newAccessToken,
        refreshToken: data['refresh_token'] as String? ?? refreshToken,
        expiresIn: data['expires_in'] as int,
        user: User.fromJson(data['user'] as Map<String, dynamic>),
      );
    } catch (_) {
      return null;
    }
  }

  /// Logout: revoke refresh token (sent in body for mobile) + clear storage.
  Future<void> logout(String serverUrl) async {
    try {
      final accessToken = await _storage.read(key: _accessTokenKey);
      final refreshToken = await _storage.read(key: _refreshTokenKey);
      if (accessToken != null && refreshToken != null) {
        await _dio.post('$serverUrl/api/nvr/auth/revoke',
            data: {'refresh_token': refreshToken},
            options: Options(headers: {'Authorization': 'Bearer $accessToken'}));
      }
    } catch (_) {}
    await _storage.delete(key: _accessTokenKey);
    await _storage.delete(key: _refreshTokenKey);
  }
}
