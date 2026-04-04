import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'theme/nvr_theme.dart';
import 'router/app_router.dart';
import 'providers/user_preferences_provider.dart';

class NvrApp extends ConsumerWidget {
  const NvrApp({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final router = ref.watch(routerProvider);
    final themeMode = ref.watch(themeModeProvider);
    return MaterialApp.router(
      title: 'MediaMTX NVR',
      theme: NvrTheme.light(),
      darkTheme: NvrTheme.dark(),
      themeMode: themeMode,
      routerConfig: router,
      debugShowCheckedModeBanner: false,
    );
  }
}
