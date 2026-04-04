import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../providers/auth_provider.dart';
import '../widgets/shell/navigation_shell.dart';
import '../screens/login_screen.dart';
import '../screens/server_setup_screen.dart';
import '../screens/setup_screen.dart';
import '../screens/dashboard/dashboard_screen.dart';
import '../screens/live_view/live_view_screen.dart';
import '../screens/live_view/fullscreen_view.dart';
import '../screens/playback/playback_screen.dart';
import '../screens/search/clip_search_screen.dart';
import '../screens/cameras/camera_list_screen.dart';
import '../screens/cameras/add_camera_screen.dart';
import '../screens/cameras/camera_detail_screen.dart';
import '../screens/settings/settings_screen.dart';
import '../screens/schedules/schedules_screen.dart';
import '../screens/screenshots/screenshots_screen.dart';
import '../models/camera.dart';

int _indexFromPath(String path) {
  if (path.startsWith('/dashboard')) return 0;
  if (path.startsWith('/live')) return 1;
  if (path.startsWith('/playback')) return 2;
  if (path.startsWith('/search')) return 3;
  if (path.startsWith('/screenshots')) return 4;
  if (path.startsWith('/devices')) return 5;
  if (path.startsWith('/settings')) return 6;
  if (path.startsWith('/schedules')) return 7;
  return 0;
}

void _navigateToIndex(BuildContext context, int index) {
  const paths = ['/dashboard', '/live', '/playback', '/search', '/screenshots', '/devices', '/settings', '/schedules'];
  context.go(paths[index]);
}

final routerProvider = Provider<GoRouter>((ref) {
  final authState = ref.watch(authProvider);

  return GoRouter(
    initialLocation: '/dashboard',
    redirect: (context, state) {
      final status = authState.status;
      final path = state.uri.path;
      final isAuthRoute = path == '/login' || path == '/server-setup' || path == '/setup';

      if (status == AuthStatus.serverNeeded && path != '/server-setup') return '/server-setup';
      if (status == AuthStatus.unauthenticated && !isAuthRoute) return '/login';
      if (status == AuthStatus.authenticated && isAuthRoute) return '/dashboard';
      return null;
    },
    routes: [
      GoRoute(path: '/server-setup', builder: (_, __) => const ServerSetupScreen()),
      GoRoute(path: '/login', builder: (_, __) => const LoginScreen()),
      GoRoute(path: '/setup', builder: (_, __) => const SetupScreen()),
      ShellRoute(
        builder: (context, state, child) {
          final index = _indexFromPath(state.uri.path);
          return NavigationShell(
            selectedIndex: index,
            onDestinationSelected: (i) => _navigateToIndex(context, i),
            child: child,
          );
        },
        routes: [
          GoRoute(path: '/dashboard', builder: (_, __) => const DashboardScreen()),
          GoRoute(path: '/live', builder: (_, __) => const LiveViewScreen(), routes: [
            GoRoute(path: 'fullscreen', builder: (_, state) =>
              FullscreenView(camera: state.extra as Camera)),
          ]),
          GoRoute(
            path: '/playback',
            builder: (_, state) {
              final cameraId = state.uri.queryParameters['cameraId'];
              final timestamp = state.uri.queryParameters['timestamp'];
              return PlaybackScreen(
                initialCameraId: cameraId,
                initialTimestamp: timestamp != null
                    ? DateTime.tryParse(timestamp)
                    : null,
              );
            },
          ),
          GoRoute(path: '/search', builder: (_, __) => const ClipSearchScreen()),
          GoRoute(
            path: '/screenshots',
            builder: (context, state) => const ScreenshotsScreen(),
          ),
          GoRoute(path: '/devices', builder: (_, __) => const CameraListScreen(), routes: [
            GoRoute(path: 'add', builder: (_, __) => const AddCameraScreen()),
            GoRoute(path: ':id', builder: (_, state) =>
              CameraDetailScreen(cameraId: state.pathParameters['id']!)),
          ]),
          GoRoute(path: '/settings', builder: (_, __) => const SettingsScreen()),
          GoRoute(path: '/schedules', builder: (_, __) => const SchedulesScreen()),
        ],
      ),
    ],
  );
});
