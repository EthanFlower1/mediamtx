import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../providers/onvif_providers.dart';

class DeviceInfoSection extends ConsumerWidget {
  const DeviceInfoSection({super.key, required this.cameraId});

  final String cameraId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final infoAsync = ref.watch(deviceInfoProvider(cameraId));

    return infoAsync.when(
      loading: () => const SizedBox.shrink(),
      error: (_, __) => const SizedBox.shrink(),
      data: (info) {
        if (info == null) return const SizedBox.shrink();

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
                child: Text('DEVICE INFO', style: NvrTypography.monoSection),
              ),
              const Divider(height: 1, color: NvrColors.border),
              // Rows
              Padding(
                padding: const EdgeInsets.all(12),
                child: Column(
                  children: [
                    _InfoRow(label: 'MANUFACTURER', value: info.manufacturer),
                    const SizedBox(height: 8),
                    _InfoRow(label: 'MODEL', value: info.model),
                    const SizedBox(height: 8),
                    _InfoRow(label: 'FIRMWARE', value: info.firmwareVersion),
                    const SizedBox(height: 8),
                    _InfoRow(label: 'SERIAL', value: info.serialNumber),
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

class _InfoRow extends StatelessWidget {
  const _InfoRow({required this.label, required this.value});

  final String label;
  final String value;

  @override
  Widget build(BuildContext context) {
    return Row(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        SizedBox(
          width: 100,
          child: Text(label, style: NvrTypography.monoLabel),
        ),
        Expanded(
          child: Text(
            value.isEmpty ? '—' : value,
            style: NvrTypography.monoData,
          ),
        ),
      ],
    );
  }
}
