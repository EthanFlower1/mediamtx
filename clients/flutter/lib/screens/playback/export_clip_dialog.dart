import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:share_plus/share_plus.dart' show Share, XFile;

import '../../providers/export_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';

/// Dialog for exporting a clip from the playback timeline.
///
/// Shows time range pickers, progress during export/download,
/// and share/done actions when complete.
class ExportClipDialog extends ConsumerStatefulWidget {
  final String cameraId;
  final String cameraName;
  final DateTime dayStart;
  final Duration currentPosition;

  const ExportClipDialog({
    super.key,
    required this.cameraId,
    required this.cameraName,
    required this.dayStart,
    required this.currentPosition,
  });

  @override
  ConsumerState<ExportClipDialog> createState() => _ExportClipDialogState();
}

class _ExportClipDialogState extends ConsumerState<ExportClipDialog> {
  late Duration _startOffset;
  late Duration _endOffset;
  bool _hasStarted = false;

  @override
  void initState() {
    super.initState();
    // Default: 30 seconds before and after current position.
    final pos = widget.currentPosition;
    _startOffset = Duration(
      milliseconds: (pos.inMilliseconds - 30000).clamp(0, 86400000),
    );
    _endOffset = Duration(
      milliseconds: (pos.inMilliseconds + 30000).clamp(0, 86400000),
    );
  }

  DateTime get _startTime => widget.dayStart.add(_startOffset);
  DateTime get _endTime => widget.dayStart.add(_endOffset);
  Duration get _clipDuration => _endOffset - _startOffset;

  String _formatDuration(Duration d) {
    final h = d.inHours;
    final m = d.inMinutes.remainder(60);
    final s = d.inSeconds.remainder(60);
    if (h > 0) return '${h}h ${m}m ${s}s';
    if (m > 0) return '${m}m ${s}s';
    return '${s}s';
  }

  String _formatTimeOfDay(Duration offset) {
    final h = offset.inHours.toString().padLeft(2, '0');
    final m = (offset.inMinutes.remainder(60)).toString().padLeft(2, '0');
    final s = (offset.inSeconds.remainder(60)).toString().padLeft(2, '0');
    return '$h:$m:$s';
  }

  Future<void> _pickTime({required bool isStart}) async {
    final current = isStart ? _startOffset : _endOffset;
    final initialTime = TimeOfDay(
      hour: current.inHours,
      minute: current.inMinutes.remainder(60),
    );

    final colors = NvrColors.of(context);

    final picked = await showTimePicker(
      context: context,
      initialTime: initialTime,
      builder: (ctx, child) => Theme(
        data: Theme.of(ctx).copyWith(
          colorScheme: ColorScheme.dark(
            primary: colors.accent,
            surface: colors.bgSecondary,
          ),
        ),
        child: child!,
      ),
    );

    if (picked != null) {
      final newOffset = Duration(hours: picked.hour, minutes: picked.minute);
      setState(() {
        if (isStart) {
          _startOffset = newOffset;
          if (_startOffset >= _endOffset) {
            _endOffset = _startOffset + const Duration(minutes: 1);
          }
        } else {
          _endOffset = newOffset;
          if (_endOffset <= _startOffset) {
            _startOffset = _endOffset - const Duration(minutes: 1);
            if (_startOffset.isNegative) _startOffset = Duration.zero;
          }
        }
      });
    }
  }

  void _startExport() {
    final service = ref.read(exportServiceProvider);
    if (service == null) return;

    setState(() => _hasStarted = true);
    ref.read(exportProvider.notifier).startExport(
      cameraId: widget.cameraId,
      start: _startTime,
      end: _endTime,
    );
  }

  Future<void> _shareFile(String filePath) async {
    await Share.shareXFiles([XFile(filePath)]);
  }

  void _cancel() {
    if (_hasStarted) {
      ref.read(exportProvider.notifier).cancel();
    }
    Navigator.of(context).pop();
  }

  void _done() {
    ref.read(exportProvider.notifier).reset();
    Navigator.of(context).pop();
  }

  @override
  Widget build(BuildContext context) {
    final exportState = ref.watch(exportProvider);

    final colors = NvrColors.of(context);
    final typo = NvrTypography.of(context);

    return Dialog(
      backgroundColor: colors.bgSecondary,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(12),
        side: BorderSide(color: colors.border),
      ),
      child: ConstrainedBox(
        constraints: const BoxConstraints(maxWidth: 400),
        child: Padding(
          padding: const EdgeInsets.all(20),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              // Header
              Row(
                children: [
                  Icon(Icons.movie_creation_outlined,
                      color: colors.accent, size: 20),
                  const SizedBox(width: 8),
                  Text('Export Clip', style: typo.pageTitle),
                  const Spacer(),
                  IconButton(
                    icon: Icon(Icons.close,
                        size: 18, color: colors.textSecondary),
                    onPressed: _cancel,
                    padding: EdgeInsets.zero,
                    constraints: const BoxConstraints(),
                  ),
                ],
              ),
              const SizedBox(height: 4),
              Text(
                widget.cameraName,
                style: typo.body,
              ),
              const SizedBox(height: 16),

              if (!_hasStarted) ...[
                // Time range selection
                _buildTimeRangeSection(context),
              ] else ...[
                // Progress display
                _buildProgressSection(context, exportState),
              ],

              const SizedBox(height: 20),

              // Actions
              _buildActions(context, exportState),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildTimeRangeSection(BuildContext context) {
    final colors = NvrColors.of(context);
    final typo = NvrTypography.of(context);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('TIME RANGE', style: typo.monoLabel),
        const SizedBox(height: 8),

        // Start time
        _TimePickerRow(
          label: 'Start',
          value: _formatTimeOfDay(_startOffset),
          onTap: () => _pickTime(isStart: true),
        ),
        const SizedBox(height: 8),

        // End time
        _TimePickerRow(
          label: 'End',
          value: _formatTimeOfDay(_endOffset),
          onTap: () => _pickTime(isStart: false),
        ),
        const SizedBox(height: 12),

        // Duration display
        Container(
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
          decoration: BoxDecoration(
            color: colors.bgTertiary,
            borderRadius: BorderRadius.circular(6),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.timer_outlined,
                  size: 14, color: colors.textSecondary),
              const SizedBox(width: 6),
              Text(
                'Duration: ${_formatDuration(_clipDuration)}',
                style: typo.monoData,
              ),
            ],
          ),
        ),

        if (_clipDuration.inMinutes > 30)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(
              'Long clips may take several minutes to export.',
              style: typo.body.copyWith(color: colors.warning),
            ),
          ),
      ],
    );
  }

  Widget _buildProgressSection(BuildContext context, ExportState exportState) {
    final colors = NvrColors.of(context);
    final typo = NvrTypography.of(context);
    final progress = exportState.overallProgress;
    final statusLabel = exportState.statusLabel;
    final isFailed = exportState.isFailed;
    final isDone = exportState.isDone;

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Status label
        Row(
          children: [
            if (!isDone && !isFailed)
              SizedBox(
                width: 14,
                height: 14,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  color: colors.accent,
                ),
              ),
            if (isDone)
              Icon(Icons.check_circle,
                  size: 16, color: colors.success),
            if (isFailed)
              Icon(Icons.error, size: 16, color: colors.danger),
            const SizedBox(width: 8),
            Text(
              statusLabel,
              style: typo.monoData.copyWith(
                color: isFailed
                    ? colors.danger
                    : isDone
                        ? colors.success
                        : colors.textPrimary,
              ),
            ),
          ],
        ),
        const SizedBox(height: 12),

        // Progress bar
        ClipRRect(
          borderRadius: BorderRadius.circular(4),
          child: LinearProgressIndicator(
            value: progress,
            minHeight: 6,
            backgroundColor: colors.bgTertiary,
            valueColor: AlwaysStoppedAnimation<Color>(
              isFailed ? colors.danger : colors.accent,
            ),
          ),
        ),
        const SizedBox(height: 6),

        // Progress percentage
        Text(
          '${(progress * 100).toStringAsFixed(0)}%',
          style: typo.monoTimestamp,
        ),

        // ETA display
        if (exportState.job?.etaSeconds != null &&
            exportState.job!.isProcessing)
          Padding(
            padding: const EdgeInsets.only(top: 4),
            child: Text(
              'ETA: ${exportState.job!.etaSeconds!.toStringAsFixed(0)}s',
              style: typo.body,
            ),
          ),

        // Error message
        if (isFailed && exportState.error != null)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(
              exportState.error!,
              style: typo.alert,
              maxLines: 3,
              overflow: TextOverflow.ellipsis,
            ),
          ),

        // File info when done
        if (isDone)
          Padding(
            padding: const EdgeInsets.only(top: 8),
            child: Text(
              'Saved to: ${exportState.localFilePath}',
              style: typo.body,
              maxLines: 2,
              overflow: TextOverflow.ellipsis,
            ),
          ),
      ],
    );
  }

  Widget _buildActions(BuildContext context, ExportState exportState) {
    if (exportState.isDone) {
      // Done state — share and close buttons.
      return Row(
        mainAxisAlignment: MainAxisAlignment.end,
        children: [
          _DialogButton(
            label: 'Share',
            icon: Icons.share,
            isPrimary: true,
            onTap: () => _shareFile(exportState.localFilePath!),
          ),
          const SizedBox(width: 8),
          _DialogButton(
            label: 'Done',
            icon: Icons.check,
            onTap: _done,
          ),
        ],
      );
    }

    if (_hasStarted && exportState.isActive) {
      // In-progress state — cancel button only.
      return Row(
        mainAxisAlignment: MainAxisAlignment.end,
        children: [
          _DialogButton(
            label: 'Cancel',
            icon: Icons.close,
            onTap: _cancel,
          ),
        ],
      );
    }

    if (exportState.isFailed) {
      // Failed state — retry and close.
      return Row(
        mainAxisAlignment: MainAxisAlignment.end,
        children: [
          _DialogButton(
            label: 'Retry',
            icon: Icons.refresh,
            isPrimary: true,
            onTap: () {
              ref.read(exportProvider.notifier).reset();
              setState(() => _hasStarted = false);
            },
          ),
          const SizedBox(width: 8),
          _DialogButton(
            label: 'Close',
            icon: Icons.close,
            onTap: _cancel,
          ),
        ],
      );
    }

    // Initial state — export and cancel.
    final canExport = _clipDuration.inSeconds >= 1;
    return Row(
      mainAxisAlignment: MainAxisAlignment.end,
      children: [
        _DialogButton(
          label: 'Cancel',
          icon: Icons.close,
          onTap: _cancel,
        ),
        const SizedBox(width: 8),
        _DialogButton(
          label: 'Export',
          icon: Icons.download,
          isPrimary: true,
          onTap: canExport ? _startExport : null,
        ),
      ],
    );
  }
}

// ── Subwidgets ──────────────────────────────────────────────────────────────

class _TimePickerRow extends StatelessWidget {
  final String label;
  final String value;
  final VoidCallback onTap;

  const _TimePickerRow({
    required this.label,
    required this.value,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final colors = NvrColors.of(context);
    final typo = NvrTypography.of(context);

    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
        decoration: BoxDecoration(
          color: colors.bgTertiary,
          border: Border.all(color: colors.border),
          borderRadius: BorderRadius.circular(6),
        ),
        child: Row(
          children: [
            Text(
              label.toUpperCase(),
              style: typo.monoLabel,
            ),
            const SizedBox(width: 12),
            Text(value, style: typo.monoTimestamp),
            const Spacer(),
            Icon(Icons.access_time,
                size: 14, color: colors.textSecondary),
          ],
        ),
      ),
    );
  }
}

class _DialogButton extends StatelessWidget {
  final String label;
  final IconData icon;
  final bool isPrimary;
  final VoidCallback? onTap;

  const _DialogButton({
    required this.label,
    required this.icon,
    this.isPrimary = false,
    this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final colors = NvrColors.of(context);
    final typo = NvrTypography.of(context);
    final disabled = onTap == null;
    final bgColor = isPrimary
        ? (disabled ? colors.bgTertiary : colors.accent)
        : colors.bgTertiary;
    final textColor = disabled
        ? colors.textMuted
        : isPrimary
            ? Colors.white
            : colors.textSecondary;

    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 14, vertical: 8),
        decoration: BoxDecoration(
          color: bgColor,
          border: Border.all(
            color: isPrimary && !disabled
                ? colors.accent
                : colors.border,
          ),
          borderRadius: BorderRadius.circular(6),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(icon, size: 14, color: textColor),
            const SizedBox(width: 6),
            Text(
              label,
              style: typo.button.copyWith(
                fontSize: 12,
                color: textColor,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
