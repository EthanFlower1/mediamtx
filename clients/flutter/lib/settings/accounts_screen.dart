// KAI-304 — Settings → Accounts screen.
//
// Surfaces the known-accounts list backed by [AppSessionNotifier]. Each row
// shows the display name, directory URL, signed-in user, and an "Active"
// badge for the currently-active account. Per-row actions are "Switch to"
// (for inactive rows) and "Sign out".
//
// Add-Account is delegated back to the caller via [onAddAccount] so this
// screen doesn't have to know about router paths or the DiscoveryPicker
// wiring — that's the caller's problem. The caller is expected to run the
// existing discovery → login flow and then call
// `AppSessionNotifier.addAccount(...)`.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../state/app_session.dart';
import '../state/home_directory_connection.dart';
import 'settings_strings.dart';

class AccountsScreen extends ConsumerWidget {
  /// Called when the user taps "Add account". The caller wires this to the
  /// existing discovery + login flow. Left as a callback so this widget stays
  /// decoupled from router internals.
  final VoidCallback onAddAccount;

  /// Called after the last remaining account has been signed out. The caller
  /// routes back to discovery. Optional — when absent, the screen simply shows
  /// the empty state.
  final VoidCallback? onLastAccountSignedOut;

  const AccountsScreen({
    super.key,
    required this.onAddAccount,
    this.onLastAccountSignedOut,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = ref.watch(settingsStringsProvider);
    final session = ref.watch(appSessionProvider);
    final accounts = session.knownConnections;
    final activeId = session.activeConnection?.id;

    return Scaffold(
      appBar: AppBar(title: Text(s.accountsScreenTitle)),
      body: accounts.isEmpty
          ? _EmptyState(strings: s)
          : ListView.builder(
              itemCount: accounts.length,
              itemBuilder: (ctx, i) {
                final account = accounts[i];
                return _AccountTile(
                  account: account,
                  isActive: account.id == activeId,
                  strings: s,
                  signedInAs: account.id == activeId ? session.userId : null,
                  onSwitch: account.id == activeId
                      ? null
                      : () => ref
                          .read(appSessionProvider.notifier)
                          .switchTo(account.id),
                  onSignOut: () => _confirmAndSignOut(context, ref, account.id),
                );
              },
            ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: onAddAccount,
        icon: const Icon(Icons.add),
        label: Text(s.accountsAddButton),
      ),
    );
  }

  Future<void> _confirmAndSignOut(
    BuildContext context,
    WidgetRef ref,
    String connectionId,
  ) async {
    final s = ref.read(settingsStringsProvider);
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        title: Text(s.accountsSignOutConfirmTitle),
        content: Text(s.accountsSignOutConfirmBody),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: Text(s.accountsSignOutConfirmCancel),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child: Text(s.accountsSignOutConfirmConfirm),
          ),
        ],
      ),
    );
    if (confirmed != true) return;

    await ref.read(appSessionProvider.notifier).signOut(connectionId);
    if (ref.read(appSessionProvider).knownConnections.isEmpty) {
      onLastAccountSignedOut?.call();
    }
  }
}

class _EmptyState extends StatelessWidget {
  final SettingsStrings strings;
  const _EmptyState({required this.strings});

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              strings.accountsEmptyTitle,
              style: Theme.of(context).textTheme.titleMedium,
            ),
            const SizedBox(height: 8),
            Text(
              strings.accountsEmptyBody,
              style: Theme.of(context).textTheme.bodyMedium,
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }
}

class _AccountTile extends StatelessWidget {
  final HomeDirectoryConnection account;
  final bool isActive;
  final String? signedInAs;
  final SettingsStrings strings;
  final VoidCallback? onSwitch;
  final VoidCallback onSignOut;

  const _AccountTile({
    required this.account,
    required this.isActive,
    required this.signedInAs,
    required this.strings,
    required this.onSwitch,
    required this.onSignOut,
  });

  @override
  Widget build(BuildContext context) {
    final subtitleLines = <String>[account.endpointUrl];
    if (signedInAs != null && signedInAs!.isNotEmpty) {
      subtitleLines.add('${strings.accountsSignedInAs}$signedInAs');
    }
    return ListTile(
      title: Row(
        children: [
          Expanded(child: Text(account.displayName)),
          if (isActive)
            Padding(
              padding: const EdgeInsets.only(left: 8),
              child: Chip(label: Text(strings.accountsActiveBadge)),
            ),
        ],
      ),
      subtitle: Text(subtitleLines.join('\n')),
      isThreeLine: subtitleLines.length > 1,
      trailing: PopupMenuButton<String>(
        onSelected: (action) {
          if (action == 'switch') {
            onSwitch?.call();
          } else if (action == 'signout') {
            onSignOut();
          }
        },
        itemBuilder: (ctx) => [
          if (onSwitch != null)
            PopupMenuItem<String>(
              value: 'switch',
              child: Text(strings.accountsSwitchAction),
            ),
          PopupMenuItem<String>(
            value: 'signout',
            child: Text(strings.accountsSignOutAction),
          ),
        ],
      ),
    );
  }
}
