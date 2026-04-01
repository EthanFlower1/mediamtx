import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/auth_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/hud_button.dart';

class _BackupEntry {
  final String filename;
  final int sizeBytes;
  final String createdAt;

  const _BackupEntry({
    required this.filename,
    required this.sizeBytes,
    required this.createdAt,
  });

  factory _BackupEntry.fromJson(Map<String, dynamic> json) {
    return _BackupEntry(
      filename: json['filename'] as String? ?? '',
      sizeBytes: json['size_bytes'] as int? ?? 0,
      createdAt: json['created_at'] as String? ?? '',
    );
  }
}

class BackupPanel extends ConsumerStatefulWidget {
  const BackupPanel({super.key});

  @override
  ConsumerState<BackupPanel> createState() => _BackupPanelState();
}

class _BackupPanelState extends ConsumerState<BackupPanel> {
  List<_BackupEntry> _backups = [];
  bool _loading = false;
  bool _creating = false;
  String? _error;
  bool _endpointAvailable = true;

  @override
  void initState() {
    super.initState();
    _loadBackups();
  }

  String _formatBytes(int bytes) {
    if (bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB'];
    int i = 0;
    double val = bytes.toDouble();
    while (val >= 1024 && i < units.length - 1) {
      val /= 1024;
      i++;
    }
    return '${val.toStringAsFixed(i == 0 ? 0 : 1)} ${units[i]}';
  }

  Future<void> _loadBackups() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;

    setState(() {
      _loading = true;
      _error = null;
    });

    try {
      final res = await api.get('/system/backups');
      final list = res.data as List? ?? [];
      setState(() {
        _backups = list
            .map((e) => _BackupEntry.fromJson(e as Map<String, dynamic>))
            .toList();
        _loading = false;
        _endpointAvailable = true;
      });
    } catch (e) {
      final isNotFound = e.toString().contains('404') ||
          e.toString().contains('Not Found');
      setState(() {
        _loading = false;
        _endpointAvailable = !isNotFound;
        if (!isNotFound) {
          _error = e.toString();
        }
      });
    }
  }

  Future<void> _createBackup() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;

    setState(() => _creating = true);

    try {
      await api.post('/system/backup');
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              'Backup created successfully',
              style: NvrTypography.monoData.copyWith(color: NvrColors.success),
            ),
            backgroundColor: NvrColors.bgSecondary,
          ),
        );
      }
      await _loadBackups();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              'Failed to create backup: $e',
              style: NvrTypography.monoData.copyWith(color: NvrColors.danger),
            ),
            backgroundColor: NvrColors.bgSecondary,
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _creating = false);
    }
  }

  Future<void> _downloadBackup(_BackupEntry backup) async {
    final auth = ref.read(authProvider);
    final serverUrl = auth.serverUrl;
    if (serverUrl == null) return;

    final url = '$serverUrl/api/nvr/system/backups/${backup.filename}';
    await Clipboard.setData(ClipboardData(text: url));
    if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          content: Text(
            'Download URL copied to clipboard',
            style: NvrTypography.monoData.copyWith(color: NvrColors.accent),
          ),
          backgroundColor: NvrColors.bgSecondary,
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    if (!_endpointAvailable) {
      return Center(
        child: Padding(
          padding: const EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.cloud_off,
                  color: NvrColors.textMuted, size: 40),
              const SizedBox(height: 12),
              Text(
                'BACKUP ENDPOINT UNAVAILABLE',
                style: NvrTypography.monoSection,
              ),
              const SizedBox(height: 6),
              Text(
                'This feature may not be enabled on the server.',
                style: NvrTypography.body,
                textAlign: TextAlign.center,
              ),
            ],
          ),
        ),
      );
    }

    return Padding(
      padding: const EdgeInsets.all(20),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // ── Section header + create button ──
          Row(
            children: [
              Text('BACKUPS', style: NvrTypography.monoSection),
              const Spacer(),
              HudButton(
                label: _creating ? 'CREATING…' : 'CREATE BACKUP',
                icon: Icons.backup,
                onPressed: _creating ? null : _createBackup,
              ),
            ],
          ),
          const SizedBox(height: 16),

          // ── Existing backups header ──
          Row(
            children: [
              Text('EXISTING BACKUPS', style: NvrTypography.monoSection),
              const Spacer(),
              IconButton(
                icon: const Icon(Icons.refresh,
                    color: NvrColors.textMuted, size: 18),
                tooltip: 'Refresh',
                onPressed: _loading ? null : _loadBackups,
                padding: EdgeInsets.zero,
                constraints:
                    const BoxConstraints(minWidth: 28, minHeight: 28),
              ),
            ],
          ),
          const SizedBox(height: 8),

          if (_error != null)
            Padding(
              padding: const EdgeInsets.only(bottom: 12),
              child: Text(
                _error!,
                style: NvrTypography.body.copyWith(color: NvrColors.danger),
              ),
            ),

          Expanded(
            child: _loading
                ? const Center(
                    child:
                        CircularProgressIndicator(color: NvrColors.accent),
                  )
                : _backups.isEmpty
                    ? Center(
                        child: Column(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            const Icon(Icons.folder_open,
                                color: NvrColors.textMuted, size: 40),
                            const SizedBox(height: 12),
                            Text(
                              'NO BACKUPS YET',
                              style: NvrTypography.monoSection,
                            ),
                          ],
                        ),
                      )
                    : ListView.separated(
                        itemCount: _backups.length,
                        separatorBuilder: (_, __) =>
                            const SizedBox(height: 8),
                        itemBuilder: (context, index) {
                          final b = _backups[index];
                          return Container(
                            padding: const EdgeInsets.symmetric(
                                horizontal: 14, vertical: 12),
                            decoration: BoxDecoration(
                              color: NvrColors.bgSecondary,
                              border: Border.all(color: NvrColors.border),
                              borderRadius: BorderRadius.circular(4),
                            ),
                            child: Row(
                              children: [
                                Container(
                                  width: 32,
                                  height: 32,
                                  decoration: BoxDecoration(
                                    color: NvrColors.accent.withOpacity(0.1),
                                    borderRadius: BorderRadius.circular(4),
                                  ),
                                  alignment: Alignment.center,
                                  child: const Icon(
                                    Icons.archive,
                                    color: NvrColors.accent,
                                    size: 16,
                                  ),
                                ),
                                const SizedBox(width: 12),
                                Expanded(
                                  child: Column(
                                    crossAxisAlignment:
                                        CrossAxisAlignment.start,
                                    children: [
                                      Text(
                                        b.filename,
                                        style: NvrTypography.monoData
                                            .copyWith(
                                                color: NvrColors.textPrimary),
                                        overflow: TextOverflow.ellipsis,
                                      ),
                                      const SizedBox(height: 3),
                                      Text(
                                        '${_formatBytes(b.sizeBytes)}  ·  ${b.createdAt}',
                                        style: NvrTypography.monoLabel,
                                      ),
                                    ],
                                  ),
                                ),
                                const SizedBox(width: 8),
                                IconButton(
                                  icon: const Icon(Icons.download,
                                      color: NvrColors.accent, size: 18),
                                  tooltip: 'Copy download URL',
                                  onPressed: () => _downloadBackup(b),
                                  padding: EdgeInsets.zero,
                                  constraints: const BoxConstraints(
                                      minWidth: 28, minHeight: 28),
                                ),
                              ],
                            ),
                          );
                        },
                      ),
          ),
        ],
      ),
    );
  }
}
