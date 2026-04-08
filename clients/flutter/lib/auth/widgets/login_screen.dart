// KAI-297 — LoginScreen widget.
//
// White-labeled post-discovery login UI. Composes:
//   * `LocalLoginForm` (when the directory advertises local auth)
//   * `SsoButtonList`  (when the directory advertises any SSO providers)
//   * `LoginErrorBanner` (when [loginStateProvider] is in `LoginPhase.error`)
//
// Strings come from [authStringsProvider]; brand colors are sourced from the
// active theme so per-integrator builds pick them up automatically. Nothing
// is hardcoded.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../state/home_directory_connection.dart';
import '../auth_providers.dart';
import 'local_login_form.dart';
import 'login_error_banner.dart';
import 'sso_button_list.dart';

class LoginScreen extends ConsumerWidget {
  /// Connection the user just discovered / picked. Drives the auth-methods
  /// fetch and is the target of the local / SSO login.
  final HomeDirectoryConnection connection;

  const LoginScreen({super.key, required this.connection});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = ref.watch(authStringsProvider);
    final methodsAsync = ref.watch(authMethodsProvider(connection));
    final loginState = ref.watch(loginStateProvider);

    return Scaffold(
      appBar: AppBar(title: Text(s.loginScreenTitle)),
      body: SafeArea(
        child: SingleChildScrollView(
          padding: const EdgeInsets.all(24),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.stretch,
            children: [
              Text(s.loginScreenSubtitle, style: Theme.of(context).textTheme.bodyLarge),
              const SizedBox(height: 12),
              Text(connection.displayName,
                  style: Theme.of(context).textTheme.titleMedium),
              const SizedBox(height: 24),
              if (loginState.error != null)
                LoginErrorBanner(error: loginState.error!),
              methodsAsync.when(
                data: (methods) {
                  return Column(
                    crossAxisAlignment: CrossAxisAlignment.stretch,
                    children: [
                      if (methods.localEnabled)
                        LocalLoginForm(connection: connection),
                      if (methods.localEnabled && methods.hasSso)
                        const SizedBox(height: 24),
                      if (methods.hasSso)
                        SsoButtonList(
                          connection: connection,
                          methods: methods,
                        ),
                    ],
                  );
                },
                loading: () => const Padding(
                  padding: EdgeInsets.all(32),
                  child: Center(child: CircularProgressIndicator()),
                ),
                error: (err, _) => LoginErrorBanner.fromAny(err),
              ),
            ],
          ),
        ),
      ),
    );
  }
}
