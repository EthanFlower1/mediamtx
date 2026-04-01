import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../providers/onvif_providers.dart';
import '../../providers/auth_provider.dart';
import '../hud/hud_toggle.dart';

class RelaySection extends ConsumerWidget {
  const RelaySection({super.key, required this.cameraId});

  final String cameraId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final relaysAsync = ref.watch(relayOutputsProvider(cameraId));

    return relaysAsync.when(
      loading: () => const SizedBox.shrink(),
      error: (_, __) => const SizedBox.shrink(),
      data: (relays) {
        if (relays == null || relays.isEmpty) return const SizedBox.shrink();

        return Container(
          decoration: BoxDecoration(
            color: NvrColors.bgSecondary,
            border: Border.all(color: NvrColors.border),
            borderRadius: BorderRadius.circular(4),
          ),
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Header
              Padding(
                padding: const EdgeInsets.fromLTRB(12, 10, 12, 8),
                child: Text('RELAY OUTPUTS', style: NvrTypography.monoSection),
              ),
              const Divider(height: 1, color: NvrColors.border),
              Padding(
                padding: const EdgeInsets.all(12),
                child: Column(
                  children: relays.asMap().entries.map((entry) {
                    final index = entry.key;
                    final relay = entry.value;
                    return Column(
                      children: [
                        if (index > 0) ...[
                          const Divider(height: 1, color: NvrColors.border),
                          const SizedBox(height: 8),
                        ],
                        Row(
                          children: [
                            HudToggle(
                              value: relay.active,
                              showStateLabel: false,
                              onChanged: (v) async {
                                final api = ref.read(apiClientProvider);
                                if (api == null) return;
                                await api.post(
                                  '/cameras/$cameraId/relay-outputs/${relay.token}/state',
                                  data: {'active': v},
                                );
                                ref.invalidate(relayOutputsProvider(cameraId));
                              },
                            ),
                            const SizedBox(width: 12),
                            Expanded(
                              child: Column(
                                crossAxisAlignment: CrossAxisAlignment.start,
                                children: [
                                  Text(
                                    relay.token,
                                    style: NvrTypography.monoData,
                                  ),
                                  const SizedBox(height: 2),
                                  Text(
                                    relay.mode.toUpperCase(),
                                    style: NvrTypography.monoLabel,
                                  ),
                                ],
                              ),
                            ),
                            Text(
                              relay.active ? 'ACTIVE' : 'IDLE',
                              style: NvrTypography.monoLabel.copyWith(
                                color: relay.active
                                    ? NvrColors.success
                                    : NvrColors.textMuted,
                              ),
                            ),
                          ],
                        ),
                        const SizedBox(height: 8),
                      ],
                    );
                  }).toList(),
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}
