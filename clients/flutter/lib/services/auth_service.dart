import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';
import 'package:shared_preferences/shared_preferences.dart';
import '../models/user.dart';

class AuthResult {
  final String accessToken;
  final String refreshToken;
  final int expiresIn;
  final User user;

  AuthResult({required this.accessToken, required this.refreshToken, required this.expiresIn, required this.user});
}

class AuthService {
  final Dio _dio;

  static const _serverUrlKey = 'nvr_server_url';
  static const _accessTokenKey = 'nvr_access_token';
  static const _refreshTokenKey = 'nvr_refresh_token';

  AuthService({Dio? dio}) : _dio = dio ?? Dio();

  Future<String?> _read(String key) async {
    final prefs = await SharedPreferences.getInstance();
    return prefs.getString(key);
  }

  Future<void> _write(String key, String value) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setString(key, value);
  }

  Future<void> _delete(String key) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.remove(key);
  }

  Future<String?> getServerUrl() => _read(_serverUrlKey);
  Future<void> setServerUrl(String url) => _write(_serverUrlKey, url);
  Future<String?> getAccessToken() => _read(_accessTokenKey);
  Future<String?> getRefreshToken() => _read(_refreshTokenKey);

  Future<bool> validateServer(String url) async {
    try {
      final res = await _dio.get('$url/api/nvr/system/health',
          options: Options(receiveTimeout: const Duration(seconds: 5)));
      return res.statusCode == 200;
    } catch (e) {
      debugPrint('[AuthService] validateServer failed: $e');
      return false;
    }
  }

  Future<AuthResult> login(String serverUrl, String username, String password) async {
    final res = await _dio.post('$serverUrl/api/nvr/auth/login', data: {
      'username': username,
      'password': password,
    });
    final data = res.data as Map<String, dynamic>;
    final refreshToken = data['refresh_token']?.toString() ?? '';
    final result = AuthResult(
      accessToken: data['access_token']?.toString() ?? '',
      refreshToken: refreshToken,
      expiresIn: (data['expires_in'] as num?)?.toInt() ?? 900,
      user: User.fromJson(Map<String, dynamic>.from(data['user'] as Map)),
    );
    await _write(_accessTokenKey, result.accessToken);
    if (refreshToken.isNotEmpty) {
      await _write(_refreshTokenKey, refreshToken);
    }
    return result;
  }

  Future<AuthResult?> refresh(String serverUrl) async {
    final refreshToken = await _read(_refreshTokenKey);
    if (refreshToken == null) return null;
    try {
      final res = await _dio.post('$serverUrl/api/nvr/auth/refresh', data: {
        'refresh_token': refreshToken,
      });
      final data = res.data as Map<String, dynamic>;
      final newAccessToken = data['access_token']?.toString() ?? '';
      await _write(_accessTokenKey, newAccessToken);
      final newRefreshToken = data['refresh_token']?.toString();
      if (newRefreshToken != null && newRefreshToken.isNotEmpty) {
        await _write(_refreshTokenKey, newRefreshToken);
      }
      return AuthResult(
        accessToken: newAccessToken,
        refreshToken: newRefreshToken ?? refreshToken,
        expiresIn: (data['expires_in'] as num?)?.toInt() ?? 900,
        user: User.fromJson(Map<String, dynamic>.from(data['user'] as Map)),
      );
    } catch (e) {
      debugPrint('[AuthService] token refresh failed: $e');
      return null;
    }
  }

  Future<void> logout(String serverUrl) async {
    try {
      final accessToken = await _read(_accessTokenKey);
      final refreshToken = await _read(_refreshTokenKey);
      if (accessToken != null && refreshToken != null) {
        await _dio.post('$serverUrl/api/nvr/auth/revoke',
            data: {'refresh_token': refreshToken},
            options: Options(headers: {'Authorization': 'Bearer $accessToken'}));
      }
    } catch (e) {
      debugPrint('[AuthService] logout revoke failed: $e');
    }
    await _delete(_accessTokenKey);
    await _delete(_refreshTokenKey);
  }
}
