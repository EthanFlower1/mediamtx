// KAI-297 — LoginErrorBanner widget.
//
// Translates a `LoginError` (or any `Object` from a `FutureProvider` failure)
// into a localized message + recovery hint. Strings come from
// `authStringsProvider`. Never displays raw exception text — `LoginError`'s
// `debugMessage` is intended for logs only.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../auth_providers.dart';
import '../auth_strings.dart';
import '../auth_types.dart';

class LoginErrorBanner extends ConsumerWidget {
  final LoginError error;

  const LoginErrorBanner({super.key, required this.error});

  /// Wrap an arbitrary `Object` (e.g. from `AsyncError.error`) in a banner.
  /// If it's already a `LoginError`, surfaces its kind; otherwise renders the
  /// generic "server returned an unexpected response" message.
  static Widget fromAny(Object err) {
    if (err is LoginError) return LoginErrorBanner(error: err);
    return LoginErrorBanner(
      error: LoginError(LoginErrorKind.malformed, err.toString()),
    );
  }

  String _messageFor(AuthStrings s) {
    switch (error.kind) {
      case LoginErrorKind.wrongCredentials:
        return s.errorWrongCredentials;
      case LoginErrorKind.network:
        return s.errorNetwork;
      case LoginErrorKind.unknownProvider:
        return s.errorUnknownProvider;
      case LoginErrorKind.refreshExpired:
        return s.errorRefreshExpired;
      case LoginErrorKind.server:
        return s.errorServer;
      case LoginErrorKind.malformed:
        return s.errorMalformed;
      case LoginErrorKind.cancelled:
        return s.ssoCancelled;
      case LoginErrorKind.idpRejected:
        return s.errorServer; // IdP rejection surfaces as server error to user
      case LoginErrorKind.ssoPlugin:
        return s.errorNetwork; // Plugin errors are typically network-related
      case LoginErrorKind.unknown:
        return s.errorMalformed; // Catch-all surfaces as generic error
    }
  }

  String _recoveryFor(AuthStrings s) {
    switch (error.kind) {
      case LoginErrorKind.wrongCredentials:
        return s.recoveryCheckPassword;
      case LoginErrorKind.network:
        return s.recoveryCheckNetwork;
      case LoginErrorKind.refreshExpired:
        return s.recoverySignInAgain;
      default:
        return s.recoveryRetry;
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = ref.watch(authStringsProvider);
    final scheme = Theme.of(context).colorScheme;
    return Container(
      margin: const EdgeInsets.only(bottom: 16),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: scheme.errorContainer,
        borderRadius: BorderRadius.circular(8),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              Icon(Icons.error_outline, color: scheme.onErrorContainer),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  _messageFor(s),
                  style: TextStyle(
                    color: scheme.onErrorContainer,
                    fontWeight: FontWeight.w600,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 4),
          Text(
            _recoveryFor(s),
            style: TextStyle(color: scheme.onErrorContainer),
          ),
        ],
      ),
    );
  }
}
