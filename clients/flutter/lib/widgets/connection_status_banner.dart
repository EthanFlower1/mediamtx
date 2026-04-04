import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../providers/connectivity_provider.dart';
import '../providers/pending_actions_provider.dart';
import '../theme/nvr_colors.dart';

/// A banner that appears at the top of the screen when the server is
/// unreachable, showing connectivity state and pending action count.
class ConnectionStatusBanner extends ConsumerWidget {
  const ConnectionStatusBanner({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final connectivity = ref.watch(connectivityProvider);
    final pendingActions = ref.watch(pendingActionsProvider);

    if (connectivity.isOnline) {
      // Show a brief "Back online" banner that auto-dismisses
      return const SizedBox.shrink();
    }

    final isReconnecting =
        connectivity.status == ConnectivityStatus.reconnecting;
    final pendingCount = pendingActions.pendingCount;

    return Material(
      color: isReconnecting
          ? NvrColors.warning.withOpacity(0.15)
          : NvrColors.danger.withOpacity(0.15),
      child: SafeArea(
        bottom: false,
        child: Container(
          width: double.infinity,
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          decoration: BoxDecoration(
            border: Border(
              bottom: BorderSide(
                color: isReconnecting
                    ? NvrColors.warning.withOpacity(0.3)
                    : NvrColors.danger.withOpacity(0.3),
              ),
            ),
          ),
          child: Row(
            children: [
              Icon(
                isReconnecting ? Icons.sync : Icons.cloud_off,
                size: 16,
                color: isReconnecting ? NvrColors.warning : NvrColors.danger,
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Text(
                  isReconnecting
                      ? 'Reconnecting to server...'
                      : 'Offline - showing cached data',
                  style: TextStyle(
                    fontFamily: 'IBMPlexSans',
                    fontSize: 12,
                    fontWeight: FontWeight.w500,
                    color:
                        isReconnecting ? NvrColors.warning : NvrColors.danger,
                  ),
                ),
              ),
              if (pendingCount > 0) ...[
                const SizedBox(width: 8),
                Container(
                  padding:
                      const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
                  decoration: BoxDecoration(
                    color: NvrColors.bgTertiary,
                    borderRadius: BorderRadius.circular(10),
                    border: Border.all(color: NvrColors.border),
                  ),
                  child: Text(
                    '$pendingCount pending',
                    style: const TextStyle(
                      fontFamily: 'JetBrainsMono',
                      fontSize: 10,
                      color: NvrColors.textSecondary,
                    ),
                  ),
                ),
              ],
              const SizedBox(width: 8),
              SizedBox(
                width: 14,
                height: 14,
                child: isReconnecting
                    ? const CircularProgressIndicator(
                        strokeWidth: 2,
                        color: NvrColors.warning,
                      )
                    : const SizedBox.shrink(),
              ),
            ],
          ),
        ),
      ),
    );
  }
}

/// Animated banner that briefly shows "Back online" then disappears.
class ReconnectedBanner extends ConsumerStatefulWidget {
  const ReconnectedBanner({super.key});

  @override
  ConsumerState<ReconnectedBanner> createState() => _ReconnectedBannerState();
}

class _ReconnectedBannerState extends ConsumerState<ReconnectedBanner> {
  bool _wasOffline = false;
  bool _showReconnected = false;

  @override
  Widget build(BuildContext context) {
    final connectivity = ref.watch(connectivityProvider);

    if (connectivity.isOffline) {
      _wasOffline = true;
    } else if (_wasOffline && connectivity.isOnline) {
      _wasOffline = false;
      _showReconnected = true;
      Future.delayed(const Duration(seconds: 3), () {
        if (mounted) setState(() => _showReconnected = false);
      });
    }

    if (!_showReconnected) return const SizedBox.shrink();

    return Material(
      color: NvrColors.success.withOpacity(0.15),
      child: Container(
        width: double.infinity,
        padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
        decoration: BoxDecoration(
          border: Border(
            bottom: BorderSide(
              color: NvrColors.success.withOpacity(0.3),
            ),
          ),
        ),
        child: const Row(
          children: [
            Icon(Icons.cloud_done, size: 16, color: NvrColors.success),
            SizedBox(width: 8),
            Text(
              'Back online',
              style: TextStyle(
                fontFamily: 'IBMPlexSans',
                fontSize: 12,
                fontWeight: FontWeight.w500,
                color: NvrColors.success,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
