import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../providers/onvif_providers.dart';

class AudioSection extends ConsumerWidget {
  const AudioSection({super.key, required this.cameraId});

  final String cameraId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final audioAsync = ref.watch(audioCapabilitiesProvider(cameraId));

    return audioAsync.when(
      loading: () => const SizedBox.shrink(),
      error: (_, __) => const SizedBox.shrink(),
      data: (audio) {
        if (audio == null) return const SizedBox.shrink();

        final hasMicrophone = audio.audioSources > 0;
        final hasSpeaker = audio.audioOutputs > 0;
        final hasBackchannel = audio.hasBackchannel;

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
                child: Text('AUDIO', style: NvrTypography.monoSection),
              ),
              const Divider(height: 1, color: NvrColors.border),
              Padding(
                padding: const EdgeInsets.all(12),
                child: Column(
                  children: [
                    _AudioCapRow(
                      label: 'MICROPHONE',
                      enabled: hasMicrophone,
                    ),
                    const SizedBox(height: 8),
                    _AudioCapRow(
                      label: 'SPEAKER',
                      enabled: hasSpeaker,
                    ),
                    const SizedBox(height: 8),
                    _AudioCapRow(
                      label: 'BACKCHANNEL',
                      enabled: hasBackchannel,
                    ),
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

class _AudioCapRow extends StatelessWidget {
  const _AudioCapRow({required this.label, required this.enabled});

  final String label;
  final bool enabled;

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        Icon(
          enabled ? Icons.check_circle_outline : Icons.cancel_outlined,
          size: 14,
          color: enabled ? NvrColors.success : NvrColors.textMuted,
        ),
        const SizedBox(width: 8),
        Text(label, style: NvrTypography.monoLabel),
        const Spacer(),
        Text(
          enabled ? 'YES' : 'NO',
          style: NvrTypography.monoData.copyWith(
            color: enabled ? NvrColors.success : NvrColors.textMuted,
          ),
        ),
      ],
    );
  }
}
