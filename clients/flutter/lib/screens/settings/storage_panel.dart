import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/settings_provider.dart';
import '../../theme/nvr_colors.dart';

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

  Color _progressColor(double percent) {
    if (percent > 95) return NvrColors.danger;
    if (percent > 85) return NvrColors.warning;
    return NvrColors.success;
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final storageAsync = ref.watch(storageInfoProvider);

    return storageAsync.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) => Center(
        child: Text(
          'Failed to load storage info: $e',
          style: const TextStyle(color: NvrColors.danger),
        ),
      ),
      data: (info) {
        final percent = info.usagePercent;
        final barColor = _progressColor(percent);

        return ListView(
          padding: const EdgeInsets.all(16),
          children: [
            // Disk usage card
            Card(
              color: NvrColors.bgSecondary,
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(12),
                side: const BorderSide(color: NvrColors.border),
              ),
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Row(
                      children: [
                        const Icon(Icons.storage, color: NvrColors.accent, size: 20),
                        const SizedBox(width: 8),
                        const Text(
                          'Disk Usage',
                          style: TextStyle(
                            color: NvrColors.textPrimary,
                            fontSize: 16,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                        const Spacer(),
                        Text(
                          '${percent.toStringAsFixed(1)}%',
                          style: TextStyle(
                            color: barColor,
                            fontSize: 14,
                            fontWeight: FontWeight.w600,
                          ),
                        ),
                      ],
                    ),
                    const SizedBox(height: 12),
                    ClipRRect(
                      borderRadius: BorderRadius.circular(4),
                      child: LinearProgressIndicator(
                        value: info.totalBytes > 0 ? info.usedBytes / info.totalBytes : 0,
                        backgroundColor: NvrColors.bgTertiary,
                        valueColor: AlwaysStoppedAnimation<Color>(barColor),
                        minHeight: 8,
                      ),
                    ),
                    const SizedBox(height: 12),
                    Row(
                      children: [
                        _StatChip(
                          label: 'Total',
                          value: _formatBytes(info.totalBytes),
                          color: NvrColors.textSecondary,
                        ),
                        const SizedBox(width: 12),
                        _StatChip(
                          label: 'Used',
                          value: _formatBytes(info.usedBytes),
                          color: barColor,
                        ),
                        const SizedBox(width: 12),
                        _StatChip(
                          label: 'Free',
                          value: _formatBytes(info.freeBytes),
                          color: NvrColors.success,
                        ),
                      ],
                    ),
                    const SizedBox(height: 8),
                    Row(
                      children: [
                        const Icon(Icons.videocam, color: NvrColors.textMuted, size: 16),
                        const SizedBox(width: 6),
                        Text(
                          'Recordings: ${_formatBytes(info.recordingsBytes)}',
                          style: const TextStyle(
                            color: NvrColors.textSecondary,
                            fontSize: 13,
                          ),
                        ),
                      ],
                    ),
                  ],
                ),
              ),
            ),
            const SizedBox(height: 16),
            // Per-camera breakdown
            if (info.perCamera.isNotEmpty) ...[
              const Padding(
                padding: EdgeInsets.only(bottom: 8),
                child: Text(
                  'Per-Camera Storage',
                  style: TextStyle(
                    color: NvrColors.textSecondary,
                    fontSize: 13,
                    fontWeight: FontWeight.w600,
                    letterSpacing: 0.5,
                  ),
                ),
              ),
              ...info.perCamera.map((cam) => Padding(
                padding: const EdgeInsets.only(bottom: 8),
                child: Card(
                  color: NvrColors.bgSecondary,
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(10),
                    side: const BorderSide(color: NvrColors.border),
                  ),
                  child: ListTile(
                    leading: const CircleAvatar(
                      backgroundColor: Color(0x1A3B82F6),
                      child: Icon(Icons.videocam, color: NvrColors.accent, size: 20),
                    ),
                    title: Text(
                      cam.cameraName.isNotEmpty ? cam.cameraName : cam.cameraId,
                      style: const TextStyle(
                        color: NvrColors.textPrimary,
                        fontSize: 14,
                        fontWeight: FontWeight.w500,
                      ),
                    ),
                    subtitle: Text(
                      '${cam.segmentCount} segment${cam.segmentCount == 1 ? '' : 's'}',
                      style: const TextStyle(color: NvrColors.textMuted, fontSize: 12),
                    ),
                    trailing: Text(
                      _formatBytes(cam.totalBytes),
                      style: const TextStyle(
                        color: NvrColors.textSecondary,
                        fontSize: 13,
                        fontWeight: FontWeight.w500,
                      ),
                    ),
                  ),
                ),
              )),
            ] else
              const Center(
                child: Padding(
                  padding: EdgeInsets.all(32),
                  child: Text(
                    'No per-camera data available',
                    style: TextStyle(color: NvrColors.textMuted),
                  ),
                ),
              ),
          ],
        );
      },
    );
  }
}

class _StatChip extends StatelessWidget {
  final String label;
  final String value;
  final Color color;

  const _StatChip({required this.label, required this.value, required this.color});

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(
          label,
          style: const TextStyle(color: NvrColors.textMuted, fontSize: 11),
        ),
        Text(
          value,
          style: TextStyle(
            color: color,
            fontSize: 13,
            fontWeight: FontWeight.w600,
          ),
        ),
      ],
    );
  }
}
