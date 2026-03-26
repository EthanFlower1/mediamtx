import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import '../providers/auth_provider.dart';
import '../widgets/shell/navigation_shell.dart';
import '../screens/login_screen.dart';
import '../screens/server_setup_screen.dart';
import '../screens/setup_screen.dart';
import '../screens/live_view/live_view_screen.dart';
import '../screens/live_view/fullscreen_view.dart';
import '../screens/playback/playback_screen.dart';
import '../screens/search/clip_search_screen.dart';
import '../screens/cameras/camera_list_screen.dart';
import '../screens/cameras/add_camera_screen.dart';
import '../screens/cameras/camera_detail_screen.dart';
import '../screens/settings/settings_screen.dart';
import '../models/camera.dart';

int _indexFromPath(String path) {
  if (path.startsWith('/live')) return 0;
  if (path.startsWith('/playback')) return 1;
  if (path.startsWith('/search')) return 2;
  if (path.startsWith('/devices')) return 3;
  if (path.startsWith('/settings')) return 4;
  return 0;
}

void _navigateToIndex(BuildContext context, int index) {
  const paths = ['/live', '/playback', '/search', '/devices', '/settings'];
  context.go(paths[index]);
}

final routerProvider = Provider<GoRouter>((ref) {
  final authState = ref.watch(authProvider);

  return GoRouter(
    initialLocation: '/live',
    redirect: (context, state) {
      final status = authState.status;
      final path = state.uri.path;
      final isAuthRoute = path == '/login' || path == '/server-setup' || path == '/setup';

      if (status == AuthStatus.serverNeeded && path != '/server-setup') return '/server-setup';
      if (status == AuthStatus.unauthenticated && !isAuthRoute) return '/login';
      if (status == AuthStatus.authenticated && isAuthRoute) return '/live';
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
          GoRoute(path: '/live', builder: (_, __) => const LiveViewScreen(), routes: [
            GoRoute(path: 'fullscreen', builder: (_, state) =>
              FullscreenView(camera: state.extra as Camera)),
          ]),
          GoRoute(path: '/playback', builder: (_, __) => const PlaybackScreen()),
          GoRoute(path: '/search', builder: (_, __) => const ClipSearchScreen()),
          GoRoute(path: '/devices', builder: (_, __) => const CameraListScreen(), routes: [
            GoRoute(path: 'add', builder: (_, __) => const AddCameraScreen()),
            GoRoute(path: ':id', builder: (_, state) =>
              CameraDetailScreen(cameraId: state.pathParameters['id']!)),
          ]),
          GoRoute(path: '/settings', builder: (_, __) => const SettingsScreen()),
        ],
      ),
    ],
  );
});
