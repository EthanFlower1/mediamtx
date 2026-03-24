import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/settings_provider.dart';
import '../../theme/nvr_colors.dart';

class AuditPanel extends ConsumerWidget {
  const AuditPanel({super.key});

  String _buildCsv(List<AuditEntry> entries) {
    final sb = StringBuffer();
    sb.writeln('timestamp,username,action,resource_type,resource_id,ip_address,details');
    for (final e in entries) {
      sb.writeln([
        _csvEscape(e.createdAt),
        _csvEscape(e.username),
        _csvEscape(e.action),
        _csvEscape(e.resourceType),
        _csvEscape(e.resourceId ?? ''),
        _csvEscape(e.ipAddress ?? ''),
        _csvEscape(e.details ?? ''),
      ].join(','));
    }
    return sb.toString();
  }

  String _csvEscape(String val) {
    if (val.contains(',') || val.contains('"') || val.contains('\n')) {
      return '"${val.replaceAll('"', '""')}"';
    }
    return val;
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final auditAsync = ref.watch(auditProvider);

    return auditAsync.when(
      loading: () => const Center(child: CircularProgressIndicator()),
      error: (e, _) => Center(
        child: Text(
          'Failed to load audit log: $e',
          style: const TextStyle(color: NvrColors.danger),
        ),
      ),
      data: (entries) {
        return Column(
          children: [
            // Export button bar
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
              child: Row(
                children: [
                  Text(
                    '${entries.length} entr${entries.length == 1 ? 'y' : 'ies'}',
                    style: const TextStyle(
                      color: NvrColors.textSecondary,
                      fontSize: 13,
                    ),
                  ),
                  const Spacer(),
                  if (entries.isNotEmpty)
                    OutlinedButton.icon(
                      onPressed: () async {
                        final csv = _buildCsv(entries);
                        await Clipboard.setData(ClipboardData(text: csv));
                        if (context.mounted) {
                          ScaffoldMessenger.of(context).showSnackBar(
                            const SnackBar(
                              content: Text('Audit log CSV copied to clipboard'),
                              backgroundColor: NvrColors.accent,
                            ),
                          );
                        }
                      },
                      icon: const Icon(Icons.download, size: 16),
                      label: const Text('Export CSV'),
                      style: OutlinedButton.styleFrom(
                        foregroundColor: NvrColors.accent,
                        side: const BorderSide(color: NvrColors.accent),
                        padding: const EdgeInsets.symmetric(
                          horizontal: 12,
                          vertical: 8,
                        ),
                      ),
                    ),
                ],
              ),
            ),
            const Divider(color: NvrColors.border, height: 1),
            if (entries.isEmpty)
              const Expanded(
                child: Center(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(Icons.history, color: NvrColors.textMuted, size: 48),
                      SizedBox(height: 12),
                      Text(
                        'No audit entries',
                        style: TextStyle(color: NvrColors.textMuted),
                      ),
                    ],
                  ),
                ),
              )
            else
              Expanded(
                child: ListView.separated(
                  padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
                  itemCount: entries.length,
                  separatorBuilder: (_, __) => const SizedBox(height: 6),
                  itemBuilder: (context, index) {
                    final e = entries[index];
                    return _AuditEntryCard(entry: e);
                  },
                ),
              ),
          ],
        );
      },
    );
  }
}

class _AuditEntryCard extends StatelessWidget {
  final AuditEntry entry;

  const _AuditEntryCard({required this.entry});

  Color _actionColor(String action) {
    switch (action.toLowerCase()) {
      case 'delete':
        return NvrColors.danger;
      case 'create':
        return NvrColors.success;
      case 'update':
      case 'login':
        return NvrColors.accent;
      case 'logout':
        return NvrColors.textMuted;
      default:
        return NvrColors.warning;
    }
  }

  @override
  Widget build(BuildContext context) {
    final actionColor = _actionColor(entry.action);
    return Card(
      color: NvrColors.bgSecondary,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(8),
        side: const BorderSide(color: NvrColors.border),
      ),
      child: Padding(
        padding: const EdgeInsets.all(12),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // Action badge
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
              decoration: BoxDecoration(
                color: actionColor.withValues(alpha: 0.15),
                borderRadius: BorderRadius.circular(4),
                border: Border.all(color: actionColor.withValues(alpha: 0.3)),
              ),
              child: Text(
                entry.action.toUpperCase(),
                style: TextStyle(
                  color: actionColor,
                  fontSize: 10,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 0.5,
                ),
              ),
            ),
            const SizedBox(width: 10),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Row(
                    children: [
                      Expanded(
                        child: Text(
                          entry.resourceType +
                              (entry.resourceId != null ? ':${entry.resourceId}' : ''),
                          style: const TextStyle(
                            color: NvrColors.textPrimary,
                            fontSize: 13,
                            fontWeight: FontWeight.w500,
                          ),
                          overflow: TextOverflow.ellipsis,
                        ),
                      ),
                      Text(
                        entry.username,
                        style: const TextStyle(
                          color: NvrColors.accent,
                          fontSize: 12,
                        ),
                      ),
                    ],
                  ),
                  const SizedBox(height: 4),
                  Row(
                    children: [
                      const Icon(Icons.access_time, size: 12, color: NvrColors.textMuted),
                      const SizedBox(width: 4),
                      Text(
                        entry.createdAt,
                        style: const TextStyle(
                          color: NvrColors.textMuted,
                          fontSize: 11,
                        ),
                      ),
                      if (entry.ipAddress != null) ...[
                        const SizedBox(width: 10),
                        const Icon(Icons.language, size: 12, color: NvrColors.textMuted),
                        const SizedBox(width: 4),
                        Text(
                          entry.ipAddress!,
                          style: const TextStyle(
                            color: NvrColors.textMuted,
                            fontSize: 11,
                          ),
                        ),
                      ],
                    ],
                  ),
                  if (entry.details != null && entry.details!.isNotEmpty)
                    Padding(
                      padding: const EdgeInsets.only(top: 4),
                      child: Text(
                        entry.details!,
                        style: const TextStyle(
                          color: NvrColors.textSecondary,
                          fontSize: 12,
                        ),
                        maxLines: 2,
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}
