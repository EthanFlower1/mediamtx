// KAI-296 — MdnsBrowseList widget stub.
//
// Subscribes to [mdnsBrowseProvider] and renders a live list of LAN-advertised
// Kaivue directories. Taps fire the caller-supplied callback with a
// [DiscoveryCandidate] ready to be probed. On platforms where mDNS is not
// supported ([MdnsDiscovery.isSupported] == false), renders an explanatory
// message from [DiscoveryStrings] instead of attempting to start the client.

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../discovery.dart';
import '../discovery_providers.dart';

class MdnsBrowseList extends ConsumerWidget {
  final ValueChanged<DiscoveryCandidate> onPicked;

  const MdnsBrowseList({super.key, required this.onPicked});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final s = ref.watch(discoveryStringsProvider);
    final mdns = ref.watch(mdnsDiscoveryProvider);
    if (!mdns.isSupported) {
      return Padding(
        padding: const EdgeInsets.all(16),
        child: Text(s.mdnsUnsupported),
      );
    }

    final async = ref.watch(mdnsBrowseProvider);
    return async.when(
      loading: () => Padding(
        padding: const EdgeInsets.all(16),
        child: Row(
          children: [
            const SizedBox(
              width: 16,
              height: 16,
              child: CircularProgressIndicator(strokeWidth: 2),
            ),
            const SizedBox(width: 12),
            Expanded(child: Text(s.mdnsSearching)),
          ],
        ),
      ),
      error: (e, st) => Padding(
        padding: const EdgeInsets.all(16),
        child: Text(s.errorUnreachable),
      ),
      data: (candidates) {
        if (candidates.isEmpty) {
          return Padding(
            padding: const EdgeInsets.all(16),
            child: Text(s.mdnsEmpty),
          );
        }
        return ListView.builder(
          shrinkWrap: true,
          itemCount: candidates.length,
          itemBuilder: (ctx, i) {
            final c = candidates[i];
            return ListTile(
              title: Text(c.label),
              subtitle: Text(c.rawUrl),
              onTap: () => onPicked(c),
            );
          },
        );
      },
    );
  }
}
