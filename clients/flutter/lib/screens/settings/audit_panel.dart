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
          const Center(child: CircularProgressIndicator(color: NvrColors.accent)),
      error: (e, _) => Center(
        child: Text(
          'Failed to load audit log: $e',
          style: NvrTypography.body.copyWith(color: NvrColors.danger),
        ),
      ),
      data: (entries) {
        return Column(
          children: [
            // ── Header bar ──
            Container(
              padding:
                  const EdgeInsets.symmetric(horizontal: 20, vertical: 12),
              decoration: const BoxDecoration(
                border: Border(
                  bottom: BorderSide(color: NvrColors.border),
                ),
              ),
              child: Row(
                children: [
                  Text(
                    '${entries.length} ENTR${entries.length == 1 ? 'Y' : 'IES'}',
                    style: NvrTypography.monoSection,
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
                                style: NvrTypography.monoData
                                    .copyWith(color: NvrColors.accent),
                              ),
                              backgroundColor: NvrColors.bgSecondary,
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
                      const Icon(Icons.history,
                          color: NvrColors.textMuted, size: 40),
                      const SizedBox(height: 12),
                      Text('NO AUDIT ENTRIES',
                          style: NvrTypography.monoSection),
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

    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: NvrColors.bgSecondary,
        border: Border.all(color: NvrColors.border),
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
              style: NvrTypography.monoLabel.copyWith(
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
                        style: NvrTypography.monoData.copyWith(
                          color: NvrColors.textPrimary,
                        ),
                        overflow: TextOverflow.ellipsis,
                      ),
                    ),
                    Text(
                      entry.username,
                      style: NvrTypography.monoData.copyWith(
                        color: NvrColors.accent,
                      ),
                    ),
                  ],
                ),
                const SizedBox(height: 4),
                Row(
                  children: [
                    const Icon(Icons.access_time,
                        size: 11, color: NvrColors.textMuted),
                    const SizedBox(width: 4),
                    Text(
                      entry.createdAt,
                      style: NvrTypography.monoLabel,
                    ),
                    if (entry.ipAddress != null) ...[
                      const SizedBox(width: 10),
                      const Icon(Icons.language,
                          size: 11, color: NvrColors.textMuted),
                      const SizedBox(width: 4),
                      Text(
                        entry.ipAddress!,
                        style: NvrTypography.monoLabel,
                      ),
                    ],
                  ],
                ),
                if (entry.details != null && entry.details!.isNotEmpty)
                  Padding(
                    padding: const EdgeInsets.only(top: 4),
                    child: Text(
                      entry.details!,
                      style: NvrTypography.body,
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
