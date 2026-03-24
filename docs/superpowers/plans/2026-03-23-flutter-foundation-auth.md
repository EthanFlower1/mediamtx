# Flutter NVR Client — Plan 1: Foundation + Auth

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Scaffold the Flutter project with theme, navigation, API client, authentication, and a working login → home screen flow across all 6 platforms.

**Architecture:** Flutter project at `clients/flutter/` using Riverpod for state, go_router for navigation, dio for HTTP, flutter_secure_storage for tokens. Backend auth endpoints are modified to return refresh tokens in JSON body (not just cookies) for mobile compatibility.

**Tech Stack:** Flutter 3.x, Dart, Riverpod, go_router, dio, flutter_secure_storage, freezed, json_serializable

**Spec:** `docs/superpowers/specs/2026-03-23-flutter-client-design.md`

**This is Plan 1 of 4.** After this plan, the app can: connect to a server, authenticate, show an adaptive shell with navigation, and display a placeholder camera list. Plans 2-4 build features on top of this.

---

## File Structure

| File | Task | Purpose |
|------|------|---------|
| `clients/flutter/pubspec.yaml` | 1 | Package dependencies |
| `clients/flutter/lib/main.dart` | 1 | App entry point |
| `clients/flutter/lib/app.dart` | 3 | MaterialApp with theme + router |
| `clients/flutter/lib/theme/nvr_colors.dart` | 2 | NVR color palette constants |
| `clients/flutter/lib/theme/nvr_theme.dart` | 2 | Material 3 dark theme |
| `clients/flutter/lib/models/user.dart` | 4 | User model (freezed) |
| `clients/flutter/lib/models/camera.dart` | 4 | Camera model (freezed) |
| `clients/flutter/lib/services/auth_service.dart` | 5 | Login, refresh, logout, secure storage |
| `clients/flutter/lib/services/api_client.dart` | 6 | dio + JWT interceptor + auto-refresh |
| `clients/flutter/lib/providers/auth_provider.dart` | 7 | Auth state (Riverpod) |
| `clients/flutter/lib/providers/cameras_provider.dart` | 7 | Camera list (Riverpod) |
| `clients/flutter/lib/router/app_router.dart` | 3 | go_router with auth guards |
| `clients/flutter/lib/screens/server_setup_screen.dart` | 8 | First-launch server URL entry |
| `clients/flutter/lib/screens/login_screen.dart` | 8 | Username/password login |
| `clients/flutter/lib/screens/setup_screen.dart` | 8 | Initial admin setup |
| `clients/flutter/lib/widgets/adaptive_layout.dart` | 3 | Bottom nav / side rail shell |
| `clients/flutter/lib/widgets/connection_banner.dart` | 9 | "Server Unreachable" banner |
| `clients/flutter/lib/screens/home_placeholder.dart` | 9 | Placeholder screens for tabs |
| `internal/nvr/api/auth.go` | 10 | Backend: refresh token in JSON body |
| `internal/nvr/api/system.go` | 10 | Backend: port discovery in /system/info |
| `clients/flutter/test/auth_service_test.dart` | 5 | Auth service unit tests |
| `clients/flutter/test/api_client_test.dart` | 6 | API client unit tests |

---

### Task 1: Flutter Project Scaffold

**Files:**
- Create: `clients/flutter/pubspec.yaml`
- Create: `clients/flutter/lib/main.dart`
- Create: `clients/flutter/analysis_options.yaml`

- [ ] **Step 1: Create the Flutter project**

```bash
cd /Users/ethanflower/personal_projects/mediamtx
flutter create clients/flutter --org com.mediamtx --project-name nvr_client --platforms ios,android,macos,windows,linux,web
```

- [ ] **Step 2: Replace pubspec.yaml with project dependencies**

```yaml
name: nvr_client
description: MediaMTX NVR cross-platform client
publish_to: 'none'
version: 1.0.0+1

environment:
  sdk: '>=3.2.0 <4.0.0'

dependencies:
  flutter:
    sdk: flutter
  # State management
  flutter_riverpod: ^2.4.0
  riverpod_annotation: ^2.3.0
  # Navigation
  go_router: ^14.0.0
  # HTTP & networking
  dio: ^5.4.0
  web_socket_channel: ^2.4.0
  # Storage
  flutter_secure_storage: ^9.0.0
  shared_preferences: ^2.2.0
  # Models
  freezed_annotation: ^2.4.0
  json_annotation: ^4.8.0
  # UI
  google_fonts: ^6.1.0

dev_dependencies:
  flutter_test:
    sdk: flutter
  flutter_lints: ^4.0.0
  build_runner: ^2.4.0
  freezed: ^2.4.0
  json_serializable: ^6.7.0
  riverpod_generator: ^2.3.0
  mockito: ^5.4.0
  http_mock_adapter: ^0.6.0

flutter:
  uses-material-design: true
```

- [ ] **Step 3: Create minimal main.dart**

```dart
// clients/flutter/lib/main.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'app.dart';

void main() {
  WidgetsFlutterBinding.ensureInitialized();
  runApp(const ProviderScope(child: NvrApp()));
}
```

- [ ] **Step 4: Verify it builds**

```bash
cd clients/flutter && flutter pub get && flutter analyze
```

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/
git commit -m "feat(flutter): scaffold Flutter project with dependencies"
```

---

### Task 2: Theme — NVR Colors + Material 3 Dark Theme

**Files:**
- Create: `clients/flutter/lib/theme/nvr_colors.dart`
- Create: `clients/flutter/lib/theme/nvr_theme.dart`

- [ ] **Step 1: Create NVR color constants**

```dart
// clients/flutter/lib/theme/nvr_colors.dart
import 'package:flutter/material.dart';

/// NVR color palette matching the React web UI.
class NvrColors {
  NvrColors._();

  static const bgPrimary = Color(0xFF0f172a);
  static const bgSecondary = Color(0xFF1e293b);
  static const bgTertiary = Color(0xFF334155);
  static const bgInput = Color(0xFF1e293b);
  static const accent = Color(0xFF3b82f6);
  static const accentHover = Color(0xFF2563eb);
  static const textPrimary = Color(0xFFf1f5f9);
  static const textSecondary = Color(0xFF94a3b8);
  static const textMuted = Color(0xFF64748b);
  static const success = Color(0xFF22c55e);
  static const warning = Color(0xFFf59e0b);
  static const danger = Color(0xFFef4444);
  static const border = Color(0xFF334155);
}
```

- [ ] **Step 2: Create Material 3 theme**

```dart
// clients/flutter/lib/theme/nvr_theme.dart
import 'package:flutter/material.dart';
import 'nvr_colors.dart';

class NvrTheme {
  NvrTheme._();

  static ThemeData dark() {
    final colorScheme = ColorScheme.fromSeed(
      seedColor: NvrColors.accent,
      brightness: Brightness.dark,
      surface: NvrColors.bgPrimary,
      onSurface: NvrColors.textPrimary,
      primary: NvrColors.accent,
      onPrimary: Colors.white,
      secondary: NvrColors.bgSecondary,
      error: NvrColors.danger,
    );

    return ThemeData(
      useMaterial3: true,
      colorScheme: colorScheme,
      scaffoldBackgroundColor: NvrColors.bgPrimary,
      appBarTheme: const AppBarTheme(
        backgroundColor: NvrColors.bgSecondary,
        foregroundColor: NvrColors.textPrimary,
        elevation: 0,
        scrolledUnderElevation: 1,
      ),
      cardTheme: const CardTheme(
        color: NvrColors.bgSecondary,
        elevation: 0,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.all(Radius.circular(12)),
          side: BorderSide(color: NvrColors.border),
        ),
      ),
      inputDecorationTheme: InputDecorationTheme(
        filled: true,
        fillColor: NvrColors.bgInput,
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: NvrColors.border),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: NvrColors.border),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(8),
          borderSide: const BorderSide(color: NvrColors.accent),
        ),
        labelStyle: const TextStyle(color: NvrColors.textSecondary),
        hintStyle: const TextStyle(color: NvrColors.textMuted),
      ),
      elevatedButtonTheme: ElevatedButtonThemeData(
        style: ElevatedButton.styleFrom(
          backgroundColor: NvrColors.accent,
          foregroundColor: Colors.white,
          shape: RoundedRectangleBorder(borderRadius: BorderRadius.circular(8)),
          padding: const EdgeInsets.symmetric(horizontal: 24, vertical: 12),
        ),
      ),
      navigationBarTheme: const NavigationBarThemeData(
        backgroundColor: NvrColors.bgSecondary,
        indicatorColor: NvrColors.accent,
      ),
      navigationRailTheme: const NavigationRailThemeData(
        backgroundColor: NvrColors.bgSecondary,
        selectedIconTheme: IconThemeData(color: NvrColors.accent),
        indicatorColor: NvrColors.accent,
      ),
      dividerTheme: const DividerThemeData(color: NvrColors.border),
      snackBarTheme: const SnackBarThemeData(
        backgroundColor: NvrColors.bgSecondary,
        contentTextStyle: TextStyle(color: NvrColors.textPrimary),
      ),
    );
  }
}
```

- [ ] **Step 3: Verify it compiles**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/theme/
git commit -m "feat(flutter): add NVR dark theme with Material 3"
```

---

### Task 3: Navigation — Adaptive Layout + Router

**Files:**
- Create: `clients/flutter/lib/app.dart`
- Create: `clients/flutter/lib/router/app_router.dart`
- Create: `clients/flutter/lib/widgets/adaptive_layout.dart`

- [ ] **Step 1: Create adaptive layout shell**

```dart
// clients/flutter/lib/widgets/adaptive_layout.dart
import 'package:flutter/material.dart';

class AdaptiveLayout extends StatelessWidget {
  final int selectedIndex;
  final ValueChanged<int> onDestinationSelected;
  final Widget child;

  const AdaptiveLayout({
    super.key,
    required this.selectedIndex,
    required this.onDestinationSelected,
    required this.child,
  });

  static const _destinations = [
    NavigationDestination(icon: Icon(Icons.videocam), label: 'Live'),
    NavigationDestination(icon: Icon(Icons.play_circle), label: 'Playback'),
    NavigationDestination(icon: Icon(Icons.search), label: 'Search'),
    NavigationDestination(icon: Icon(Icons.camera_alt), label: 'Cameras'),
    NavigationDestination(icon: Icon(Icons.settings), label: 'Settings'),
  ];

  static const _railDestinations = [
    NavigationRailDestination(icon: Icon(Icons.videocam), label: Text('Live')),
    NavigationRailDestination(icon: Icon(Icons.play_circle), label: Text('Playback')),
    NavigationRailDestination(icon: Icon(Icons.search), label: Text('Search')),
    NavigationRailDestination(icon: Icon(Icons.camera_alt), label: Text('Cameras')),
    NavigationRailDestination(icon: Icon(Icons.settings), label: Text('Settings')),
  ];

  @override
  Widget build(BuildContext context) {
    final width = MediaQuery.sizeOf(context).width;
    final useRail = width >= 600;

    if (useRail) {
      return Scaffold(
        body: Row(
          children: [
            NavigationRail(
              selectedIndex: selectedIndex,
              onDestinationSelected: onDestinationSelected,
              destinations: _railDestinations,
              labelType: NavigationRailLabelType.all,
            ),
            const VerticalDivider(width: 1),
            Expanded(child: child),
          ],
        ),
      );
    }

    return Scaffold(
      body: child,
      bottomNavigationBar: NavigationBar(
        selectedIndex: selectedIndex,
        onDestinationSelected: onDestinationSelected,
        destinations: _destinations,
      ),
    );
  }
}
```

- [ ] **Step 2: Create stub screens so the router compiles**

Create minimal placeholder files so the router imports resolve:

```dart
// clients/flutter/lib/screens/server_setup_screen.dart
import 'package:flutter/material.dart';
class ServerSetupScreen extends StatelessWidget {
  const ServerSetupScreen({super.key});
  @override
  Widget build(BuildContext context) => const Scaffold(body: Center(child: Text('Server Setup')));
}
```

```dart
// clients/flutter/lib/screens/login_screen.dart
import 'package:flutter/material.dart';
class LoginScreen extends StatelessWidget {
  const LoginScreen({super.key});
  @override
  Widget build(BuildContext context) => const Scaffold(body: Center(child: Text('Login')));
}
```

```dart
// clients/flutter/lib/screens/setup_screen.dart
import 'package:flutter/material.dart';
class SetupScreen extends StatelessWidget {
  const SetupScreen({super.key});
  @override
  Widget build(BuildContext context) => const Scaffold(body: Center(child: Text('Setup')));
}
```

```dart
// clients/flutter/lib/screens/home_placeholder.dart
import 'package:flutter/material.dart';
class HomePlaceholder extends StatelessWidget {
  final String title;
  const HomePlaceholder({super.key, required this.title});
  @override
  Widget build(BuildContext context) => Center(child: Text(title));
}
```

These stubs are replaced with full implementations in Task 8.

- [ ] **Step 3: Create go_router with auth guards**

The router uses `refreshListenable` to reactively re-evaluate redirects when auth state changes. This requires a `ChangeNotifier` that fires when the auth provider updates.

```dart
// clients/flutter/lib/router/app_router.dart
import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../screens/server_setup_screen.dart';
import '../screens/login_screen.dart';
import '../screens/setup_screen.dart';
import '../screens/home_placeholder.dart';
import '../widgets/adaptive_layout.dart';

/// Notifier that fires when auth state changes, triggering router redirects.
class AuthChangeNotifier extends ChangeNotifier {
  void notify() => notifyListeners();
}

final authChangeNotifierProvider = Provider((ref) => AuthChangeNotifier());

final routerProvider = Provider<GoRouter>((ref) {
  final authNotifier = ref.read(authChangeNotifierProvider);
  return GoRouter(
    initialLocation: '/live',
    refreshListenable: authNotifier,
    redirect: (context, state) {
      // Auth guard logic will be wired in Task 7
      return null;
    },
    routes: [
      GoRoute(path: '/server-setup', builder: (_, __) => const ServerSetupScreen()),
      GoRoute(path: '/login', builder: (_, __) => const LoginScreen()),
      GoRoute(path: '/setup', builder: (_, __) => const SetupScreen()),
      ShellRoute(
        builder: (context, state, child) {
          final index = _indexFromPath(state.uri.path);
          return AdaptiveLayout(
            selectedIndex: index,
            onDestinationSelected: (i) => _navigateToIndex(context, i),
            child: child,
          );
        },
        routes: [
          GoRoute(path: '/live', builder: (_, __) => const HomePlaceholder(title: 'Live View')),
          GoRoute(path: '/playback', builder: (_, __) => const HomePlaceholder(title: 'Playback')),
          GoRoute(path: '/search', builder: (_, __) => const HomePlaceholder(title: 'Search')),
          GoRoute(path: '/cameras', builder: (_, __) => const HomePlaceholder(title: 'Cameras')),
          GoRoute(path: '/settings', builder: (_, __) => const HomePlaceholder(title: 'Settings')),
        ],
      ),
    ],
  );
});

int _indexFromPath(String path) {
  const paths = ['/live', '/playback', '/search', '/cameras', '/settings'];
  final idx = paths.indexOf(path);
  return idx >= 0 ? idx : 0;
}

void _navigateToIndex(BuildContext context, int index) {
  const paths = ['/live', '/playback', '/search', '/cameras', '/settings'];
  context.go(paths[index]);
}
```

- [ ] **Step 3: Create app.dart**

```dart
// clients/flutter/lib/app.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'theme/nvr_theme.dart';
import 'router/app_router.dart';

class NvrApp extends ConsumerWidget {
  const NvrApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final router = ref.watch(routerProvider);
    return MaterialApp.router(
      title: 'MediaMTX NVR',
      theme: NvrTheme.dark(),
      routerConfig: router,
      debugShowCheckedModeBanner: false,
    );
  }
}
```

- [ ] **Step 4: Verify it runs**

```bash
cd clients/flutter && flutter run -d macos
```

Expected: App launches with dark theme and bottom/side navigation showing 5 placeholder tabs.

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/app.dart clients/flutter/lib/router/ clients/flutter/lib/widgets/adaptive_layout.dart
git commit -m "feat(flutter): add adaptive navigation shell with go_router"
```

---

### Task 4: Data Models — User + Camera

**Files:**
- Create: `clients/flutter/lib/models/user.dart`
- Create: `clients/flutter/lib/models/camera.dart`

- [ ] **Step 1: Create User model**

```dart
// clients/flutter/lib/models/user.dart
import 'package:freezed_annotation/freezed_annotation.dart';

part 'user.freezed.dart';
part 'user.g.dart';

@freezed
class User with _$User {
  const factory User({
    required String id,
    required String username,
    required String role,
    @JsonKey(name: 'camera_permissions') @Default('*') String cameraPermissions,
  }) = _User;

  factory User.fromJson(Map<String, dynamic> json) => _$UserFromJson(json);
}
```

- [ ] **Step 2: Create Camera model**

```dart
// clients/flutter/lib/models/camera.dart
import 'package:freezed_annotation/freezed_annotation.dart';

part 'camera.freezed.dart';
part 'camera.g.dart';

@freezed
class Camera with _$Camera {
  const factory Camera({
    required String id,
    required String name,
    @JsonKey(name: 'rtsp_url') @Default('') String rtspUrl,
    @JsonKey(name: 'onvif_endpoint') @Default('') String onvifEndpoint,
    @JsonKey(name: 'mediamtx_path') @Default('') String mediamtxPath,
    @Default('disconnected') String status,
    @JsonKey(name: 'ptz_capable') @Default(false) bool ptzCapable,
    @JsonKey(name: 'ai_enabled') @Default(false) bool aiEnabled,
    @JsonKey(name: 'sub_stream_url') @Default('') String subStreamUrl,
    @JsonKey(name: 'retention_days') @Default(30) int retentionDays,
    @JsonKey(name: 'motion_timeout_seconds') @Default(8) int motionTimeoutSeconds,
    @JsonKey(name: 'snapshot_uri') @Default('') String snapshotUri,
    @JsonKey(name: 'supports_events') @Default(false) bool supportsEvents,
    @JsonKey(name: 'supports_analytics') @Default(false) bool supportsAnalytics,
    @JsonKey(name: 'supports_relay') @Default(false) bool supportsRelay,
    @JsonKey(name: 'created_at') String? createdAt,
    @JsonKey(name: 'updated_at') String? updatedAt,
  }) = _Camera;

  factory Camera.fromJson(Map<String, dynamic> json) => _$CameraFromJson(json);
}
```

- [ ] **Step 3: Generate freezed/json code**

```bash
cd clients/flutter && dart run build_runner build --delete-conflicting-outputs
```

- [ ] **Step 4: Verify it compiles**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/models/
git commit -m "feat(flutter): add User and Camera data models with freezed"
```

---

### Task 5: Auth Service — Login, Refresh, Logout, Secure Storage

**Files:**
- Create: `clients/flutter/lib/services/auth_service.dart`
- Create: `clients/flutter/test/auth_service_test.dart`

- [ ] **Step 1: Create auth service**

```dart
// clients/flutter/lib/services/auth_service.dart
import 'package:dio/dio.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import '../models/user.dart';

class AuthResult {
  final String accessToken;
  final String refreshToken;
  final int expiresIn;
  final User user;

  AuthResult({
    required this.accessToken,
    required this.refreshToken,
    required this.expiresIn,
    required this.user,
  });
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

  /// Get stored server URL.
  Future<String?> getServerUrl() => _storage.read(key: _serverUrlKey);

  /// Save server URL after validation.
  Future<void> setServerUrl(String url) => _storage.write(key: _serverUrlKey, value: url);

  /// Validate server is reachable by calling /system/health.
  Future<bool> validateServer(String url) async {
    try {
      final res = await _dio.get('$url/api/nvr/system/health',
          options: Options(receiveTimeout: const Duration(seconds: 5)));
      return res.statusCode == 200;
    } catch (_) {
      return false;
    }
  }

  /// Login with username/password. Returns tokens + user.
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

  /// Refresh the access token using stored refresh token.
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

      // If server returns a new refresh token (rotation), store it
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

  /// Logout: revoke refresh token + clear storage.
  Future<void> logout(String serverUrl) async {
    try {
      final accessToken = await _storage.read(key: _accessTokenKey);
      final refreshToken = await _storage.read(key: _refreshTokenKey);
      if (accessToken != null && refreshToken != null) {
        await _dio.post('$serverUrl/api/nvr/auth/revoke',
            data: {'refresh_token': refreshToken},
            options: Options(headers: {'Authorization': 'Bearer $accessToken'}));
      }
    } catch (_) {
      // Best-effort revocation
    }
    await _storage.delete(key: _accessTokenKey);
    await _storage.delete(key: _refreshTokenKey);
  }

  /// Get stored access token.
  Future<String?> getAccessToken() => _storage.read(key: _accessTokenKey);

  /// Get stored refresh token.
  Future<String?> getRefreshToken() => _storage.read(key: _refreshTokenKey);
}
```

- [ ] **Step 2: Write unit tests**

```dart
// clients/flutter/test/auth_service_test.dart
import 'package:flutter_test/flutter_test.dart';
import 'package:dio/dio.dart';
import 'package:http_mock_adapter/http_mock_adapter.dart';
import 'package:nvr_client/services/auth_service.dart';

// Note: flutter_secure_storage requires mocking in tests.
// These tests focus on the HTTP/parsing logic using dio mock adapter.

void main() {
  late Dio dio;
  late DioAdapter adapter;

  setUp(() {
    dio = Dio();
    adapter = DioAdapter(dio: dio);
  });

  test('validateServer returns true for healthy server', () async {
    adapter.onGet('/api/nvr/system/health', (server) => server.reply(200, {'status': 'ok'}));
    final service = AuthService(dio: dio);
    final result = await service.validateServer('');
    expect(result, isTrue);
  });

  test('validateServer returns false for unreachable server', () async {
    adapter.onGet('/api/nvr/system/health',
        (server) => server.throws(0, DioException(requestOptions: RequestOptions())));
    final service = AuthService(dio: dio);
    final result = await service.validateServer('');
    expect(result, isFalse);
  });

  // Note: Full login/refresh tests require mocking FlutterSecureStorage.
  // Those are best done as widget tests or with a test wrapper.
  // The HTTP layer is validated by the server validation tests above.
}
```

- [ ] **Step 3: Verify**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 4: Commit**

```bash
git add clients/flutter/lib/services/auth_service.dart clients/flutter/test/
git commit -m "feat(flutter): add auth service with login, refresh, logout"
```

---

### Task 6: API Client — dio + JWT Interceptor

**Files:**
- Create: `clients/flutter/lib/services/api_client.dart`

- [ ] **Step 1: Create API client with JWT interceptor**

```dart
// clients/flutter/lib/services/api_client.dart
import 'package:dio/dio.dart';
import 'auth_service.dart';

/// HTTP client with automatic JWT attachment and refresh-on-401.
class ApiClient {
  final Dio dio;
  final AuthService _authService;
  final String serverUrl;

  ApiClient({
    required this.serverUrl,
    required AuthService authService,
  })  : _authService = authService,
        dio = Dio(BaseOptions(
          baseUrl: '$serverUrl/api/nvr',
          connectTimeout: const Duration(seconds: 10),
          receiveTimeout: const Duration(seconds: 30),
          headers: {'Content-Type': 'application/json'},
        )) {
    dio.interceptors.add(_AuthInterceptor(
      authService: _authService,
      serverUrl: serverUrl,
      dio: dio,
    ));
  }

  // Convenience methods
  Future<Response<T>> get<T>(String path, {Map<String, dynamic>? queryParameters}) =>
      dio.get<T>(path, queryParameters: queryParameters);

  Future<Response<T>> post<T>(String path, {dynamic data}) =>
      dio.post<T>(path, data: data);

  Future<Response<T>> put<T>(String path, {dynamic data}) =>
      dio.put<T>(path, data: data);

  Future<Response<T>> delete<T>(String path) =>
      dio.delete<T>(path);
}

class _AuthInterceptor extends Interceptor {
  final AuthService authService;
  final String serverUrl;
  final Dio dio;
  bool _isRefreshing = false;

  _AuthInterceptor({
    required this.authService,
    required this.serverUrl,
    required this.dio,
  });

  @override
  Future<void> onRequest(RequestOptions options, RequestInterceptorHandler handler) async {
    final token = await authService.getAccessToken();
    if (token != null) {
      options.headers['Authorization'] = 'Bearer $token';
    }
    handler.next(options);
  }

  @override
  Future<void> onError(DioException err, ErrorInterceptorHandler handler) async {
    if (err.response?.statusCode == 401 && !_isRefreshing) {
      _isRefreshing = true;
      try {
        final result = await authService.refresh(serverUrl);
        _isRefreshing = false;

        if (result != null) {
          // Retry the original request with new token
          final opts = err.requestOptions;
          opts.headers['Authorization'] = 'Bearer ${result.accessToken}';
          final response = await dio.fetch(opts);
          handler.resolve(response);
          return;
        }
      } catch (_) {
        _isRefreshing = false;
      }
    }
    handler.next(err);
  }
}
```

- [ ] **Step 2: Verify**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 3: Commit**

```bash
git add clients/flutter/lib/services/api_client.dart
git commit -m "feat(flutter): add API client with JWT interceptor and auto-refresh"
```

---

### Task 7: Riverpod Providers — Auth + Cameras

**Files:**
- Create: `clients/flutter/lib/providers/auth_provider.dart`
- Create: `clients/flutter/lib/providers/cameras_provider.dart`

- [ ] **Step 1: Create auth provider**

```dart
// clients/flutter/lib/providers/auth_provider.dart
import 'dart:async';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/user.dart';
import '../services/auth_service.dart';
import '../services/api_client.dart';

enum AuthStatus { unknown, unauthenticated, serverNeeded, authenticated }

class AuthState {
  final AuthStatus status;
  final User? user;
  final String? serverUrl;
  final String? error;

  const AuthState({
    this.status = AuthStatus.unknown,
    this.user,
    this.serverUrl,
    this.error,
  });

  AuthState copyWith({AuthStatus? status, User? user, String? serverUrl, String? error}) =>
      AuthState(
        status: status ?? this.status,
        user: user ?? this.user,
        serverUrl: serverUrl ?? this.serverUrl,
        error: error,
      );
}

class AuthNotifier extends StateNotifier<AuthState> {
  final AuthService _authService;
  Timer? _refreshTimer;

  AuthNotifier(this._authService) : super(const AuthState()) {
    _init();
  }

  Future<void> _init() async {
    final serverUrl = await _authService.getServerUrl();
    if (serverUrl == null) {
      state = state.copyWith(status: AuthStatus.serverNeeded);
      return;
    }

    final result = await _authService.refresh(serverUrl);
    if (result != null) {
      _scheduleRefresh(serverUrl, result.expiresIn);
      state = state.copyWith(
        status: AuthStatus.authenticated,
        user: result.user,
        serverUrl: serverUrl,
      );
    } else {
      state = state.copyWith(status: AuthStatus.unauthenticated, serverUrl: serverUrl);
    }
  }

  Future<void> setServer(String url) async {
    await _authService.setServerUrl(url);
    state = state.copyWith(status: AuthStatus.unauthenticated, serverUrl: url);
  }

  Future<void> login(String username, String password) async {
    final serverUrl = state.serverUrl;
    if (serverUrl == null) return;

    try {
      final result = await _authService.login(serverUrl, username, password);
      _scheduleRefresh(serverUrl, result.expiresIn);
      state = state.copyWith(status: AuthStatus.authenticated, user: result.user, error: null);
    } catch (e) {
      state = state.copyWith(error: 'Login failed. Check credentials.');
    }
  }

  Future<void> logout() async {
    _refreshTimer?.cancel();
    if (state.serverUrl != null) {
      await _authService.logout(state.serverUrl!);
    }
    state = state.copyWith(status: AuthStatus.unauthenticated, user: null);
  }

  void _scheduleRefresh(String serverUrl, int expiresIn) {
    _refreshTimer?.cancel();
    final refreshIn = Duration(seconds: expiresIn - 60);
    _refreshTimer = Timer(refreshIn, () async {
      final result = await _authService.refresh(serverUrl);
      if (result != null) {
        state = state.copyWith(user: result.user);
        _scheduleRefresh(serverUrl, result.expiresIn);
      } else {
        state = state.copyWith(status: AuthStatus.unauthenticated);
      }
    });
  }

  @override
  void dispose() {
    _refreshTimer?.cancel();
    super.dispose();
  }
}

final authServiceProvider = Provider((ref) => AuthService());
final authProvider = StateNotifierProvider<AuthNotifier, AuthState>((ref) {
  return AuthNotifier(ref.read(authServiceProvider));
});

/// Provides an ApiClient configured with the current server URL.
/// Returns null if not authenticated.
final apiClientProvider = Provider<ApiClient?>((ref) {
  final auth = ref.watch(authProvider);
  if (auth.serverUrl == null || auth.status != AuthStatus.authenticated) return null;
  return ApiClient(serverUrl: auth.serverUrl!, authService: ref.read(authServiceProvider));
});
```

- [ ] **Step 2: Create cameras provider**

```dart
// clients/flutter/lib/providers/cameras_provider.dart
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../models/camera.dart';
import 'auth_provider.dart';

final camerasProvider = FutureProvider<List<Camera>>((ref) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];

  final res = await api.get('/cameras');
  final list = (res.data as List).map((e) => Camera.fromJson(e as Map<String, dynamic>)).toList();
  return list;
});
```

- [ ] **Step 3: Wire auth guard into router**

Update `app_router.dart`'s redirect to check auth state:
```dart
redirect: (context, state) {
  final auth = ref.read(authProvider);
  final isAuthRoute = state.uri.path == '/login' || state.uri.path == '/server-setup' || state.uri.path == '/setup';

  if (auth.status == AuthStatus.serverNeeded && state.uri.path != '/server-setup') {
    return '/server-setup';
  }
  if (auth.status == AuthStatus.unauthenticated && !isAuthRoute) {
    return '/login';
  }
  if (auth.status == AuthStatus.authenticated && isAuthRoute) {
    return '/live';
  }
  return null;
},
```

- [ ] **Step 4: Verify**

```bash
cd clients/flutter && flutter analyze
```

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/providers/
git commit -m "feat(flutter): add auth and cameras Riverpod providers"
```

---

### Task 8: Screens — Server Setup, Login, Setup

**Files:**
- Create: `clients/flutter/lib/screens/server_setup_screen.dart`
- Create: `clients/flutter/lib/screens/login_screen.dart`
- Create: `clients/flutter/lib/screens/setup_screen.dart`
- Create: `clients/flutter/lib/screens/home_placeholder.dart`

- [ ] **Step 1: Create server setup screen**

First-launch screen where user enters the NVR server URL:

```dart
// clients/flutter/lib/screens/server_setup_screen.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../providers/auth_provider.dart';
import '../services/auth_service.dart';
import '../theme/nvr_colors.dart';

class ServerSetupScreen extends ConsumerStatefulWidget {
  const ServerSetupScreen({super.key});
  @override
  ConsumerState<ServerSetupScreen> createState() => _ServerSetupScreenState();
}

class _ServerSetupScreenState extends ConsumerState<ServerSetupScreen> {
  final _controller = TextEditingController(text: 'http://');
  bool _validating = false;
  String? _error;

  Future<void> _validate() async {
    setState(() { _validating = true; _error = null; });
    final url = _controller.text.trim().replaceAll(RegExp(r'/$'), '');

    final authService = ref.read(authServiceProvider);
    final ok = await authService.validateServer(url);

    if (ok) {
      ref.read(authProvider.notifier).setServer(url);
    } else {
      setState(() { _error = 'Could not reach server. Check the URL and try again.'; });
    }
    setState(() { _validating = false; });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 400),
          child: Padding(
            padding: const EdgeInsets.all(32),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(Icons.videocam, size: 48, color: NvrColors.accent),
                const SizedBox(height: 16),
                Text('Connect to NVR', style: Theme.of(context).textTheme.headlineMedium),
                const SizedBox(height: 8),
                Text('Enter your MediaMTX NVR server address',
                    style: TextStyle(color: NvrColors.textSecondary)),
                const SizedBox(height: 32),
                TextField(
                  controller: _controller,
                  decoration: InputDecoration(
                    labelText: 'Server URL',
                    hintText: 'http://192.168.1.50:9997',
                    errorText: _error,
                    prefixIcon: const Icon(Icons.dns),
                  ),
                  keyboardType: TextInputType.url,
                  onSubmitted: (_) => _validate(),
                ),
                const SizedBox(height: 24),
                SizedBox(
                  width: double.infinity,
                  child: ElevatedButton(
                    onPressed: _validating ? null : _validate,
                    child: _validating
                        ? const SizedBox(height: 20, width: 20, child: CircularProgressIndicator(strokeWidth: 2))
                        : const Text('Connect'),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
```

- [ ] **Step 2: Create login screen**

```dart
// clients/flutter/lib/screens/login_screen.dart
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../providers/auth_provider.dart';
import '../theme/nvr_colors.dart';

class LoginScreen extends ConsumerStatefulWidget {
  const LoginScreen({super.key});
  @override
  ConsumerState<LoginScreen> createState() => _LoginScreenState();
}

class _LoginScreenState extends ConsumerState<LoginScreen> {
  final _usernameController = TextEditingController();
  final _passwordController = TextEditingController();
  bool _loading = false;

  Future<void> _login() async {
    setState(() => _loading = true);
    await ref.read(authProvider.notifier).login(
      _usernameController.text.trim(),
      _passwordController.text,
    );
    setState(() => _loading = false);
  }

  @override
  Widget build(BuildContext context) {
    final auth = ref.watch(authProvider);

    return Scaffold(
      body: Center(
        child: ConstrainedBox(
          constraints: const BoxConstraints(maxWidth: 400),
          child: Padding(
            padding: const EdgeInsets.all(32),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(Icons.videocam, size: 48, color: NvrColors.accent),
                const SizedBox(height: 16),
                Text('Sign In', style: Theme.of(context).textTheme.headlineMedium),
                const SizedBox(height: 8),
                Text('MediaMTX NVR', style: TextStyle(color: NvrColors.textSecondary)),
                const SizedBox(height: 32),
                TextField(
                  controller: _usernameController,
                  decoration: const InputDecoration(
                    labelText: 'Username',
                    prefixIcon: Icon(Icons.person),
                  ),
                  textInputAction: TextInputAction.next,
                ),
                const SizedBox(height: 16),
                TextField(
                  controller: _passwordController,
                  decoration: const InputDecoration(
                    labelText: 'Password',
                    prefixIcon: Icon(Icons.lock),
                  ),
                  obscureText: true,
                  onSubmitted: (_) => _login(),
                ),
                if (auth.error != null) ...[
                  const SizedBox(height: 12),
                  Text(auth.error!, style: const TextStyle(color: NvrColors.danger, fontSize: 13)),
                ],
                const SizedBox(height: 24),
                SizedBox(
                  width: double.infinity,
                  child: ElevatedButton(
                    onPressed: _loading ? null : _login,
                    child: _loading
                        ? const SizedBox(height: 20, width: 20, child: CircularProgressIndicator(strokeWidth: 2))
                        : const Text('Sign In'),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}
```

- [ ] **Step 3: Create setup screen and home placeholder**

```dart
// clients/flutter/lib/screens/setup_screen.dart
import 'package:flutter/material.dart';

class SetupScreen extends StatelessWidget {
  const SetupScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return const Scaffold(
      body: Center(child: Text('Initial Setup — Create Admin Account')),
    );
  }
}
```

```dart
// clients/flutter/lib/screens/home_placeholder.dart
import 'package:flutter/material.dart';
import '../theme/nvr_colors.dart';

class HomePlaceholder extends StatelessWidget {
  final String title;
  const HomePlaceholder({super.key, required this.title});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Icon(Icons.construction, size: 48, color: NvrColors.textMuted),
          const SizedBox(height: 16),
          Text(title, style: Theme.of(context).textTheme.headlineMedium),
          const SizedBox(height: 8),
          Text('Coming soon', style: TextStyle(color: NvrColors.textSecondary)),
        ],
      ),
    );
  }
}
```

- [ ] **Step 4: Verify the full flow runs**

```bash
cd clients/flutter && flutter run -d macos
```

Expected: App shows server setup → enter URL → validates → login screen → enter credentials → authenticated → home with 5 navigation tabs.

- [ ] **Step 5: Commit**

```bash
git add clients/flutter/lib/screens/
git commit -m "feat(flutter): add server setup, login, and placeholder screens"
```

---

### Task 9: Connection Banner + Offline Cache

**Files:**
- Create: `clients/flutter/lib/widgets/connection_banner.dart`

- [ ] **Step 1: Create connection banner widget**

```dart
// clients/flutter/lib/widgets/connection_banner.dart
import 'package:flutter/material.dart';
import '../theme/nvr_colors.dart';

class ConnectionBanner extends StatelessWidget {
  final bool connected;
  final VoidCallback? onRetry;

  const ConnectionBanner({
    super.key,
    required this.connected,
    this.onRetry,
  });

  @override
  Widget build(BuildContext context) {
    if (connected) return const SizedBox.shrink();

    return MaterialBanner(
      backgroundColor: NvrColors.danger.withValues(alpha: 0.15),
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
      content: const Row(
        children: [
          Icon(Icons.cloud_off, color: NvrColors.danger, size: 18),
          SizedBox(width: 8),
          Text('Server unreachable — retrying...',
              style: TextStyle(color: NvrColors.danger, fontSize: 13)),
        ],
      ),
      actions: [
        if (onRetry != null)
          TextButton(
            onPressed: onRetry,
            child: const Text('Retry Now'),
          ),
      ],
    );
  }
}
```

- [ ] **Step 2: Commit**

```bash
git add clients/flutter/lib/widgets/connection_banner.dart
git commit -m "feat(flutter): add server connection banner widget"
```

---

### Task 10: Backend Changes — Auth JSON Body + Port Discovery

**Files:**
- Modify: `internal/nvr/api/auth.go`
- Modify: `internal/nvr/api/system.go` (or equivalent)

- [ ] **Step 1: Add refresh token to login response**

In `internal/nvr/api/auth.go`, find the login handler's JSON response. Add `"refresh_token": rawToken` to the response map (alongside `access_token`, `expires_in`, `user`).

- [ ] **Step 2: Accept refresh token from JSON body in Refresh AND Revoke handlers**

In both the `Refresh` and `Revoke` handlers, before the cookie check, try reading from the request body. Mobile clients cannot send cookies, so they pass the refresh token in the JSON body instead:

```go
// Try JSON body first (for mobile clients)
var bodyToken string
if c.Request.Body != nil {
    var req struct {
        RefreshToken string `json:"refresh_token"`
    }
    if err := c.ShouldBindJSON(&req); err == nil && req.RefreshToken != "" {
        bodyToken = req.RefreshToken
    }
}

// Then try cookie (for web clients)
rawToken, err := c.Cookie("refresh_token")
if (err != nil || rawToken == "") && bodyToken != "" {
    rawToken = bodyToken
}
```

- [ ] **Step 3: Add port discovery to /system/info**

Find the system info handler and add port fields to the response:

```go
// Add to the system info response:
"ws_port":       wsPort,        // API port + 1
"playback_port": 9996,          // MediaMTX playback server
"webrtc_port":   8889,          // MediaMTX WebRTC/WHEP
"clip_search_available": embedder != nil,
```

This requires the system handler to have access to the embedder pointer and port configuration. Pass them through the `RouterConfig`.

- [ ] **Step 4: Verify backend builds**

```bash
go build ./internal/nvr/...
```

- [ ] **Step 5: Commit**

```bash
git add internal/nvr/api/auth.go internal/nvr/api/system.go
git commit -m "feat(api): add refresh token in JSON body and port discovery for mobile clients"
```
