import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/auth_provider.dart';
import '../../theme/nvr_colors.dart';

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
          const SnackBar(
            content: Text('Backup created successfully'),
            backgroundColor: NvrColors.success,
          ),
        );
      }
      await _loadBackups();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Failed to create backup: $e'),
            backgroundColor: NvrColors.danger,
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
        const SnackBar(
          content: Text('Download URL copied to clipboard'),
          backgroundColor: NvrColors.accent,
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    if (!_endpointAvailable) {
      return const Center(
        child: Padding(
          padding: EdgeInsets.all(32),
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Icon(Icons.cloud_off, color: NvrColors.textMuted, size: 48),
              SizedBox(height: 12),
              Text(
                'Backup endpoint not available',
                style: TextStyle(color: NvrColors.textMuted, fontSize: 15),
              ),
              SizedBox(height: 6),
              Text(
                'This feature may not be enabled on the server.',
                style: TextStyle(color: NvrColors.textMuted, fontSize: 13),
                textAlign: TextAlign.center,
              ),
            ],
          ),
        ),
      );
    }

    return Padding(
      padding: const EdgeInsets.all(16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          // Create backup button
          ElevatedButton.icon(
            onPressed: _creating ? null : _createBackup,
            icon: _creating
                ? const SizedBox(
                    width: 16,
                    height: 16,
                    child: CircularProgressIndicator(
                      strokeWidth: 2,
                      color: Colors.white,
                    ),
                  )
                : const Icon(Icons.backup),
            label: Text(_creating ? 'Creating…' : 'Create Backup'),
            style: ElevatedButton.styleFrom(
              backgroundColor: NvrColors.accent,
              foregroundColor: Colors.white,
              padding: const EdgeInsets.symmetric(vertical: 14),
              shape: RoundedRectangleBorder(
                borderRadius: BorderRadius.circular(10),
              ),
            ),
          ),
          const SizedBox(height: 20),
          Row(
            children: [
              const Text(
                'Existing Backups',
                style: TextStyle(
                  color: NvrColors.textSecondary,
                  fontSize: 13,
                  fontWeight: FontWeight.w600,
                  letterSpacing: 0.5,
                ),
              ),
              const Spacer(),
              IconButton(
                icon: const Icon(Icons.refresh, color: NvrColors.textMuted, size: 20),
                tooltip: 'Refresh',
                onPressed: _loading ? null : _loadBackups,
              ),
            ],
          ),
          const SizedBox(height: 8),
          if (_error != null)
            Padding(
              padding: const EdgeInsets.only(bottom: 12),
              child: Text(
                _error!,
                style: const TextStyle(color: NvrColors.danger, fontSize: 13),
              ),
            ),
          Expanded(
            child: _loading
                ? const Center(child: CircularProgressIndicator())
                : _backups.isEmpty
                    ? const Center(
                        child: Column(
                          mainAxisSize: MainAxisSize.min,
                          children: [
                            Icon(Icons.folder_open, color: NvrColors.textMuted, size: 48),
                            SizedBox(height: 12),
                            Text(
                              'No backups yet',
                              style: TextStyle(color: NvrColors.textMuted),
                            ),
                          ],
                        ),
                      )
                    : ListView.separated(
                        itemCount: _backups.length,
                        separatorBuilder: (_, __) => const SizedBox(height: 8),
                        itemBuilder: (context, index) {
                          final b = _backups[index];
                          return Card(
                            color: NvrColors.bgSecondary,
                            shape: RoundedRectangleBorder(
                              borderRadius: BorderRadius.circular(10),
                              side: const BorderSide(color: NvrColors.border),
                            ),
                            child: ListTile(
                              leading: const CircleAvatar(
                                backgroundColor: Color(0x1A3B82F6),
                                child: Icon(Icons.archive, color: NvrColors.accent, size: 20),
                              ),
                              title: Text(
                                b.filename,
                                style: const TextStyle(
                                  color: NvrColors.textPrimary,
                                  fontSize: 13,
                                  fontWeight: FontWeight.w500,
                                ),
                                overflow: TextOverflow.ellipsis,
                              ),
                              subtitle: Text(
                                '${_formatBytes(b.sizeBytes)}  ·  ${b.createdAt}',
                                style: const TextStyle(
                                  color: NvrColors.textMuted,
                                  fontSize: 12,
                                ),
                              ),
                              trailing: IconButton(
                                icon: const Icon(
                                  Icons.download,
                                  color: NvrColors.accent,
                                  size: 20,
                                ),
                                tooltip: 'Copy download URL',
                                onPressed: () => _downloadBackup(b),
                              ),
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
