// KAI-304 — Settings-flow localizable strings.
//
// Mirrors the pattern used by [AuthStrings] and [DiscoveryStrings]: a plain
// value class with an English default, injected via Riverpod so tests can
// override individual fields and the real flutter_intl wiring can land later
// without touching call sites.

import 'package:flutter_riverpod/flutter_riverpod.dart';

/// User-visible strings used by the Settings screens (Accounts list today,
/// more panels as the settings area grows).
class SettingsStrings {
  const SettingsStrings({
    required this.accountsScreenTitle,
    required this.accountsEmptyTitle,
    required this.accountsEmptyBody,
    required this.accountsActiveBadge,
    required this.accountsSignedInAs,
    required this.accountsAddButton,
    required this.accountsSwitchAction,
    required this.accountsSignOutAction,
    required this.accountsSignOutConfirmTitle,
    required this.accountsSignOutConfirmBody,
    required this.accountsSignOutConfirmCancel,
    required this.accountsSignOutConfirmConfirm,
  });

  final String accountsScreenTitle;
  final String accountsEmptyTitle;
  final String accountsEmptyBody;
  final String accountsActiveBadge;
  final String accountsSignedInAs;
  final String accountsAddButton;
  final String accountsSwitchAction;
  final String accountsSignOutAction;
  final String accountsSignOutConfirmTitle;
  final String accountsSignOutConfirmBody;
  final String accountsSignOutConfirmCancel;
  final String accountsSignOutConfirmConfirm;

  /// Default English strings. Swap via a Riverpod override in tests.
  static const SettingsStrings en = SettingsStrings(
    accountsScreenTitle: 'Accounts',
    accountsEmptyTitle: 'No accounts yet',
    accountsEmptyBody: 'Add a directory to get started.',
    accountsActiveBadge: 'Active',
    accountsSignedInAs: 'Signed in as ',
    accountsAddButton: 'Add account',
    accountsSwitchAction: 'Switch to',
    accountsSignOutAction: 'Sign out',
    accountsSignOutConfirmTitle: 'Sign out?',
    accountsSignOutConfirmBody:
        'You will need to sign in again to use this directory.',
    accountsSignOutConfirmCancel: 'Cancel',
    accountsSignOutConfirmConfirm: 'Sign out',
  );
}

/// Injection point for [SettingsStrings]. Override in tests or in the widget
/// tree root once flutter_intl lands.
final settingsStringsProvider = Provider<SettingsStrings>((ref) {
  return SettingsStrings.en;
});
