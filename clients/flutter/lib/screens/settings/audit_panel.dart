import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../providers/settings_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/hud_button.dart';

class AuditPanel extends ConsumerWidget {
  const AuditPanel({super.key});

  String _buildCsv(List<AuditEntry> entries) {
    final sb = StringBuffer();
    sb.writeln(
        'timestamp,username,action,resource_type,resource_id,ip_address,details');
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
      loading: () =>
          Center(child: CircularProgressIndicator(color: NvrColors.of(context).accent)),
      error: (e, _) => Center(
        child: Text(
          'Failed to load audit log: $e',
          style: NvrTypography.of(context).body.copyWith(color: NvrColors.of(context).danger),
        ),
      ),
      data: (entries) {
        return Column(
          children: [
            // ── Header bar ──
            Container(
              padding:
                  const EdgeInsets.symmetric(horizontal: 20, vertical: 12),
              decoration: BoxDecoration(
                border: Border(
                  bottom: BorderSide(color: NvrColors.of(context).border),
                ),
              ),
              child: Row(
                children: [
                  Text(
                    '${entries.length} ENTR${entries.length == 1 ? 'Y' : 'IES'}',
                    style: NvrTypography.of(context).monoSection,
                  ),
                  const Spacer(),
                  if (entries.isNotEmpty)
                    HudButton(
                      label: 'EXPORT CSV',
                      icon: Icons.download,
                      style: HudButtonStyle.tactical,
                      onPressed: () async {
                        final csv = _buildCsv(entries);
                        await Clipboard.setData(ClipboardData(text: csv));
                        if (context.mounted) {
                          ScaffoldMessenger.of(context).showSnackBar(
                            SnackBar(
                              content: Text(
                                'Audit log CSV copied to clipboard',
                                style: NvrTypography.of(context).monoData
                                    .copyWith(color: NvrColors.of(context).accent),
                              ),
                              backgroundColor: NvrColors.of(context).bgSecondary,
                            ),
                          );
                        }
                      },
                    ),
                ],
              ),
            ),
            // ── Entry list ──
            if (entries.isEmpty)
              Expanded(
                child: Center(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    children: [
                      Icon(Icons.history,
                          color: NvrColors.of(context).textMuted, size: 40),
                      const SizedBox(height: 12),
                      Text('NO AUDIT ENTRIES',
                          style: NvrTypography.of(context).monoSection),
                    ],
                  ),
                ),
              )
            else
              Expanded(
                child: ListView.separated(
                  padding: const EdgeInsets.symmetric(
                      horizontal: 20, vertical: 12),
                  itemCount: entries.length,
                  separatorBuilder: (_, __) => const SizedBox(height: 6),
                  itemBuilder: (context, index) {
                    return _AuditEntryCard(entry: entries[index]);
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

  Color _actionColor(BuildContext context, String action) {
    switch (action.toLowerCase()) {
      case 'delete':
        return NvrColors.of(context).danger;
      case 'create':
        return NvrColors.of(context).success;
      case 'update':
      case 'login':
        return NvrColors.of(context).accent;
      case 'logout':
        return NvrColors.of(context).textMuted;
      default:
        return NvrColors.of(context).warning;
    }
  }

  @override
  Widget build(BuildContext context) {
    final actionColor = _actionColor(context, entry.action);

    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: NvrColors.of(context).bgSecondary,
        border: Border.all(color: NvrColors.of(context).border),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Action badge
          Container(
            padding:
                const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
            decoration: BoxDecoration(
              color: actionColor.withOpacity(0.10),
              border: Border.all(color: actionColor.withOpacity(0.27)),
              borderRadius: BorderRadius.circular(4),
            ),
            child: Text(
              entry.action.toUpperCase(),
              style: NvrTypography.of(context).monoLabel.copyWith(
                color: actionColor,
                letterSpacing: 0.8,
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
                            (entry.resourceId != null
                                ? ':${entry.resourceId}'
                                : ''),
                        style: NvrTypography.of(context).monoData.copyWith(
                          color: NvrColors.of(context).textPrimary,
                        ),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    Text(
                      entry.username,
                      style: NvrTypography.of(context).monoData.copyWith(
                        color: NvrColors.of(context).accent,
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 4),
                Row(
                  children: [
                    Icon(Icons.access_time,
                        size: 11, color: NvrColors.of(context).textMuted),
                    const SizedBox(width: 4),
                    Text(
                      entry.createdAt,
                      style: NvrTypography.of(context).monoLabel,
                    ),
                    if (entry.ipAddress != null) ...[
                      const SizedBox(width: 10),
                      Icon(Icons.language,
                          size: 11, color: NvrColors.of(context).textMuted),
                      const SizedBox(width: 4),
                      Text(
                        entry.ipAddress!,
                        style: NvrTypography.of(context).monoLabel,
                      ),
                    ],
                  ],
                ),
                if (entry.details != null && entry.details!.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(top: 4),
                    child: Text(
                      entry.details!,
                      style: NvrTypography.of(context).body,
                      maxLines: 2,
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}
