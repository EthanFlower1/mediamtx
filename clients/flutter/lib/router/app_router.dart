import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../screens/server_setup_screen.dart';
import '../screens/login_screen.dart';
import '../screens/setup_screen.dart';
import '../screens/home_placeholder.dart';
import '../screens/live_view/live_view_screen.dart';
import '../screens/live_view/fullscreen_view.dart';
import '../screens/playback/playback_screen.dart';
import '../screens/search/clip_search_screen.dart';
import '../models/camera.dart';
import '../widgets/adaptive_layout.dart';
import '../providers/auth_provider.dart';

int _indexFromPath(String path) {
  const paths = ['/live', '/playback', '/search', '/cameras', '/settings'];
  final idx = paths.indexOf(path);
  return idx >= 0 ? idx : 0;
}

void _navigateToIndex(BuildContext context, int index) {
  const paths = ['/live', '/playback', '/search', '/cameras', '/settings'];
  context.go(paths[index]);
}

final routerProvider = Provider<GoRouter>((ref) {
  return GoRouter(
    initialLocation: '/live',
    redirect: (context, state) {
      final auth = ref.read(authProvider);
      final path = state.uri.path;
      final isAuthRoute = path == '/login' || path == '/server-setup' || path == '/setup';

      if (auth.status == AuthStatus.serverNeeded && path != '/server-setup') {
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
          GoRoute(
            path: '/live',
            builder: (_, __) => const LiveViewScreen(),
            routes: [
              GoRoute(
                path: 'fullscreen',
                builder: (_, state) {
                  final camera = state.extra as Camera;
                  return FullscreenView(camera: camera);
                },
              ),
            ],
          ),
          GoRoute(path: '/playback', builder: (_, __) => const PlaybackScreen()),
          GoRoute(path: '/search', builder: (_, __) => const ClipSearchScreen()),
          GoRoute(path: '/cameras', builder: (_, __) => const HomePlaceholder(title: 'Cameras')),
          GoRoute(path: '/settings', builder: (_, __) => const HomePlaceholder(title: 'Settings')),
        ],
      ),
    ],
  );
});
