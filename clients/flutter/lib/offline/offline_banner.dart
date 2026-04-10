import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'connectivity_monitor.dart';
import 'offline_cache_service.dart';
import 'offline_strings.dart';

/// A banner displayed at the top of the screen when the device is offline or
/// experiencing degraded connectivity.
///
/// Shows the current connectivity state and how stale the cached data is.
class OfflineBanner extends ConsumerWidget {
  const OfflineBanner({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final connectivity = ref.watch(connectivityProvider);

    if (connectivity == ConnectivityState.online) {
      return const SizedBox.shrink();
    }

    final cacheService = ref.read(offlineCacheServiceProvider);
    final lastCache = cacheService.lastCacheTime();
    final locale = Localizations.localeOf(context).languageCode;
    final strings = OfflineStringsL10n.forLocale(locale);

    final isDegraded = connectivity == ConnectivityState.degraded;
    final backgroundColor = isDegraded ? Colors.orange : Colors.red;
    final icon = isDegraded ? Icons.signal_wifi_bad : Icons.wifi_off;

    final stalenessText = _buildStaleness(lastCache, strings);

    return Material(
      color: backgroundColor,
      child: SafeArea(
        bottom: false,
        child: Padding(
          padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
          child: Row(
            children: [
              Icon(icon, color: Colors.white, size: 20),
              const SizedBox(width: 8),
              Expanded(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      isDegraded ? strings.reconnecting : strings.offline,
                      style: const TextStyle(
                        color: Colors.white,
                        fontWeight: FontWeight.bold,
                        fontSize: 14,
                      ),
                    ),
                    if (stalenessText != null)
                      Text(
                        stalenessText,
                        style: const TextStyle(
                          color: Colors.white70,
                          fontSize: 12,
                        ),
                      ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  String? _buildStaleness(DateTime? lastCache, OfflineStrings strings) {
    if (lastCache == null) return null;

    final diff = DateTime.now().difference(lastCache);
    final staleness = diff.inHours >= 1
        ? strings.hoursAgo(diff.inHours)
        : strings.minutesAgo(diff.inMinutes);

    return '${strings.showingCachedData} ${strings.lastUpdated} $staleness';
  }
}
