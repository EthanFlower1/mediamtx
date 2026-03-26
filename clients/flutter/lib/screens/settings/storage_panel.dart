import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/settings_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/status_badge.dart';

class StoragePanel extends ConsumerWidget {
  const StoragePanel({super.key});

  String _formatBytes(int bytes) {
    if (bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    int i = 0;
    double val = bytes.toDouble();
    while (val >= 1024 && i < units.length - 1) {
      val /= 1024;
      i++;
    }
    return '${val.toStringAsFixed(i == 0 ? 0 : 1)} ${units[i]}';
  }

  Color _healthColor(StorageInfo info) {
    if (info.critical) return NvrColors.danger;
    if (info.warning) return NvrColors.warning;
    return NvrColors.success;
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final storageAsync = ref.watch(storageInfoProvider);

    return storageAsync.when(
      loading: () => const Center(
        child: CircularProgressIndicator(color: NvrColors.accent),
      ),
      error: (e, _) => Center(
        child: Text(
          'Failed to load storage info: $e',
          style: NvrTypography.body.copyWith(color: NvrColors.danger),
        ),
      ),
      data: (info) {
        final percent = info.usagePercent;
        final usedFraction =
            info.totalBytes > 0 ? info.usedBytes / info.totalBytes : 0.0;
        final healthColor = _healthColor(info);
        final healthLabel = info.critical
            ? 'CRITICAL'
            : info.warning
                ? 'WARNING'
                : 'HEALTHY';

        return ListView(
          padding: const EdgeInsets.all(20),
          children: [
            // ── Section header ──
            Text('STORAGE OVERVIEW', style: NvrTypography.monoSection),
            const SizedBox(height: 12),

            // ── Primary disk card ──
            Container(
              padding: const EdgeInsets.all(16),
              decoration: BoxDecoration(
                color: NvrColors.bgSecondary,
                border: Border.all(color: NvrColors.border),
                borderRadius: BorderRadius.circular(4),
              ),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  // Header row: disk name + health badge
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          'PRIMARY DISK',
                          style: NvrTypography.monoLabel,
                        ),
                      ),
                      StatusBadge(label: healthLabel, color: healthColor),
                    ],
                  ),
                  const SizedBox(height: 8),
                  // Used / Total
                  Text(
                    '${_formatBytes(info.usedBytes)} / ${_formatBytes(info.totalBytes)}',
                    style: NvrTypography.monoData.copyWith(
                      color: NvrColors.accent,
                      fontSize: 14,
                    ),
                  ),
                  const SizedBox(height: 2),
                  Text(
                    '${percent.toStringAsFixed(1)}% used',
                    style: NvrTypography.monoLabel,
                  ),
                  const SizedBox(height: 12),
                  // Usage bar
                  Container(
                    height: 8,
                    decoration: BoxDecoration(
                      color: NvrColors.bgTertiary,
                      border: Border.all(color: NvrColors.border),
                      borderRadius: BorderRadius.circular(4),
                    ),
                    child: ClipRRect(
                      borderRadius: BorderRadius.circular(4),
                      child: FractionallySizedBox(
                        alignment: Alignment.centerLeft,
                        widthFactor: usedFraction.clamp(0.0, 1.0),
                        child: Container(
                          decoration: BoxDecoration(
                            gradient: LinearGradient(
                              colors: [
                                NvrColors.accent,
                                NvrColors.accent.withOpacity(0.7),
                              ],
                            ),
                            borderRadius: BorderRadius.circular(4),
                          ),
                        ),
                      ),
                    ),
                  ),
                  const SizedBox(height: 12),
                  // Legend row
                  Row(
                    children: [
                      _LegendDot(color: NvrColors.accent),
                      const SizedBox(width: 6),
                      Text(
                        'Recordings ${_formatBytes(info.recordingsBytes)}',
                        style: NvrTypography.monoData,
                      ),
                      const SizedBox(width: 16),
                      _LegendDot(color: const Color(0xFF3B82F6)),
                      const SizedBox(width: 6),
                      Text(
                        'System',
                        style: NvrTypography.monoData,
                      ),
                      const SizedBox(width: 16),
                      _LegendDot(color: NvrColors.textSecondary),
                      const SizedBox(width: 6),
                      Text(
                        'Free ${_formatBytes(info.freeBytes)}',
                        style: NvrTypography.monoData,
                      ),
                    ],
                  ),
                ],
              ),
            ),

            // ── Per-camera breakdown ──
            if (info.perCamera.isNotEmpty) ...[
              const SizedBox(height: 24),
              Text('PER-CAMERA STORAGE', style: NvrTypography.monoSection),
              const SizedBox(height: 12),
              ...info.perCamera.map((cam) {
                final camFraction = info.totalBytes > 0
                    ? (cam.totalBytes / info.totalBytes).clamp(0.0, 1.0)
                    : 0.0;
                return Padding(
                  padding: const EdgeInsets.only(bottom: 10),
                  child: Container(
                    padding: const EdgeInsets.symmetric(
                        horizontal: 14, vertical: 12),
                    decoration: BoxDecoration(
                      color: NvrColors.bgSecondary,
                      border: Border.all(color: NvrColors.border),
                      borderRadius: BorderRadius.circular(4),
                    ),
                    child: Row(
                      children: [
                        Expanded(
                          child: Column(
                            crossAxisAlignment: CrossAxisAlignment.start,
                            children: [
                              Text(
                                cam.cameraName.isNotEmpty
                                    ? cam.cameraName
                                    : cam.cameraId,
                                style: NvrTypography.monoData.copyWith(
                                  color: NvrColors.textPrimary,
                                ),
                                overflow: TextOverflow.ellipsis,
                              ),
                              const SizedBox(height: 6),
                              // Mini usage bar (4px)
                              Container(
                                height: 4,
                                decoration: BoxDecoration(
                                  color: NvrColors.bgTertiary,
                                  border: Border.all(color: NvrColors.border),
                                  borderRadius: BorderRadius.circular(2),
                                ),
                                child: ClipRRect(
                                  borderRadius: BorderRadius.circular(2),
                                  child: FractionallySizedBox(
                                    alignment: Alignment.centerLeft,
                                    widthFactor: camFraction,
                                    child: Container(
                                      color: NvrColors.accent,
                                    ),
                                  ),
                                ),
                              ),
                            ],
                          ),
                        ),
                        const SizedBox(width: 16),
                        Text(
                          _formatBytes(cam.totalBytes),
                          style: NvrTypography.monoData.copyWith(
                            color: NvrColors.accent,
                          ),
                        ),
                      ],
                    ),
                  ),
                );
              }),
            ] else ...[
              const SizedBox(height: 24),
              Center(
                child: Padding(
                  padding: const EdgeInsets.all(32),
                  child: Text(
                    'No per-camera data available',
                    style: NvrTypography.body,
                  ),
                ),
              ),
            ],
          ],
        );
      },
    );
  }
}

class _LegendDot extends StatelessWidget {
  final Color color;
  const _LegendDot({required this.color});

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 8,
      height: 8,
      decoration: BoxDecoration(
        color: color,
        shape: BoxShape.circle,
      ),
    );
  }
}
