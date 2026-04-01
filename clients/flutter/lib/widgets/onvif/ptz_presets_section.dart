import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../providers/onvif_providers.dart';
import '../../providers/auth_provider.dart';
import '../hud/hud_button.dart';

class PtzPresetsSection extends ConsumerWidget {
  const PtzPresetsSection({super.key, required this.cameraId});

  final String cameraId;

  Future<void> _sendPtzAction(
    WidgetRef ref,
    Map<String, dynamic> data,
  ) async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    await api.post('/cameras/$cameraId/ptz', data: data);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final presetsAsync = ref.watch(ptzPresetsProvider(cameraId));

    return presetsAsync.when(
      loading: () => const SizedBox.shrink(),
      error: (_, __) => const SizedBox.shrink(),
      data: (presets) {
        if (presets == null) return const SizedBox.shrink();

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
                child: Text('PTZ PRESETS', style: NvrTypography.monoSection),
              ),
              const Divider(height: 1, color: NvrColors.border),
              Padding(
                padding: const EdgeInsets.all(12),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    // Home button
                    HudButton(
                      label: 'GO HOME',
                      icon: Icons.home,
                      style: HudButtonStyle.tactical,
                      onPressed: () => _sendPtzAction(
                        ref,
                        {'action': 'home'},
                      ),
                    ),
                    if (presets.isNotEmpty) ...[
                      const SizedBox(height: 12),
                      const Divider(height: 1, color: NvrColors.border),
                      const SizedBox(height: 12),
                      ...presets.map((preset) => Padding(
                            padding: const EdgeInsets.only(bottom: 8),
                            child: Row(
                              children: [
                                Expanded(
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      Text(
                                        preset.name.isNotEmpty
                                            ? preset.name
                                            : preset.token,
                                        style: NvrTypography.monoData,
                                      ),
                                      const SizedBox(height: 2),
                                      Text(
                                        preset.token,
                                        style: NvrTypography.monoLabel,
                                      ),
                                    ],
                                  ),
                                ),
                                HudButton(
                                  label: 'GO TO',
                                  style: HudButtonStyle.secondary,
                                  onPressed: () => _sendPtzAction(
                                    ref,
                                    {
                                      'action': 'preset',
                                      'preset_token': preset.token,
                                    },
                                  ),
                                ),
                              ],
                            ),
                          )),
                    ],
                  ],
                ),
              ),
            ],
          ),
        );
      },
    );
  }
}
