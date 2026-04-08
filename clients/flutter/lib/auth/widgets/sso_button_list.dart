// KAI-297 — SsoButtonList widget.
//
// Renders one branded button per advertised SSO provider. Tapping a button
// kicks off the SSO flow via `loginStateProvider.submitSso`.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../state/home_directory_connection.dart';
import '../auth_providers.dart';
import '../auth_types.dart';

class SsoButtonList extends ConsumerWidget {
  final HomeDirectoryConnection connection;
  final AvailableAuthMethods methods;

  const SsoButtonList({
    super.key,
    required this.connection,
    required this.methods,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = ref.watch(authStringsProvider);
    final loginState = ref.watch(loginStateProvider);
    final notifier = ref.read(loginStateProvider.notifier);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(s.ssoHeader, style: Theme.of(context).textTheme.titleSmall),
        const SizedBox(height: 12),
        for (final provider in methods.ssoProviders) ...[
          OutlinedButton(
            onPressed: loginState.isSubmitting
                ? null
                : () => notifier.submitSso(
                      connection: connection,
                      providerId: provider.id,
                      knownMethods: methods,
                    ),
            child: Text('${s.ssoContinueWith}${provider.displayName}'),
          ),
          const SizedBox(height: 8),
        ],
        if (loginState.phase == LoginPhase.cancelled)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(
              s.ssoCancelled,
              style: Theme.of(context).textTheme.bodySmall,
            ),
          ),
      ],
    );
  }
}
