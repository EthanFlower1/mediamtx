import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../api/notification_api.dart';
import '../../providers/notification_center_provider.dart';
import '../../theme/nvr_colors.dart';

/// Event type labels for display.
const _eventTypeLabels = <String, String>{
  'motion': 'Motion',
  'camera_offline': 'Camera Offline',
  'camera_online': 'Camera Online',
  'recording_started': 'Recording Started',
  'recording_stopped': 'Recording Stopped',
  'recording_stalled': 'Recording Stalled',
  'recording_recovered': 'Recording Recovered',
  'recording_failed': 'Recording Failed',
  'tampering': 'Tampering',
  'intrusion': 'Intrusion',
  'line_crossing': 'Line Crossing',
  'loitering': 'Loitering',
  'ai_detection': 'AI Detection',
  'object_count': 'Object Count',
};

const _eventTypes = [
  'motion', 'camera_offline', 'camera_online',
  'recording_started', 'recording_stopped', 'recording_stalled',
  'recording_recovered', 'recording_failed',
  'tampering', 'intrusion', 'line_crossing', 'loitering',
  'ai_detection', 'object_count',
];

const _severityLevels = ['critical', 'warning', 'info'];

String _typeLabel(String type) => _eventTypeLabels[type] ?? type;

String _relativeTime(DateTime dt) {
  final diff = DateTime.now().difference(dt);
  if (diff.inSeconds < 5) return 'just now';
  if (diff.inSeconds < 60) return '${diff.inSeconds}s ago';
  if (diff.inMinutes < 60) return '${diff.inMinutes}m ago';
  if (diff.inHours < 24) return '${diff.inHours}h ago';
  if (diff.inDays < 30) return '${diff.inDays}d ago';
  return '${dt.month}/${dt.day}/${dt.year}';
}

Color _severityColor(String severity) {
  switch (severity) {
    case 'critical':
      return Colors.red;
    case 'warning':
      return Colors.orange;
    case 'info':
    default:
      return Colors.blue;
  }
}

class NotificationCenterScreen extends ConsumerStatefulWidget {
  const NotificationCenterScreen({super.key});

  @override
  ConsumerState<NotificationCenterScreen> createState() =>
      _NotificationCenterScreenState();
}

class _NotificationCenterScreenState
    extends ConsumerState<NotificationCenterScreen> {
  final _selectedIds = <String>{};
  final _searchController = TextEditingController();

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      ref.read(notificationCenterProvider.notifier).fetch();
      ref.read(notificationCenterProvider.notifier).startAutoRefresh();
    });
  }

  @override
  void dispose() {
    ref.read(notificationCenterProvider.notifier).stopAutoRefresh();
    _searchController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final colors = NvrColors.of(context);
    final state = ref.watch(notificationCenterProvider);
    final notifier = ref.read(notificationCenterProvider.notifier);

    final startItem = state.total == 0 ? 0 : state.offset + 1;
    final endItem = (state.offset + state.pageSize).clamp(0, state.total);

    return Scaffold(
      backgroundColor: colors.bgPrimary,
      body: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Header
          Padding(
            padding: const EdgeInsets.fromLTRB(16, 16, 16, 8),
            child: Row(
              children: [
                Expanded(
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(
                        'Notification Center',
                        style: TextStyle(
                          fontSize: 20,
                          fontWeight: FontWeight.bold,
                          color: colors.textPrimary,
                        ),
                      ),
                      const SizedBox(height: 4),
                      Text(
                        '${state.total} notification${state.total != 1 ? 's' : ''}'
                        '${state.filter.archived ? ' in archive' : ''}',
                        style: TextStyle(
                          fontSize: 13,
                          color: colors.textMuted,
                        ),
                      ),
                    ],
                  ),
                ),
                // Archive toggle
                _ChipButton(
                  label: state.filter.archived ? 'Viewing Archive' : 'View Archive',
                  active: state.filter.archived,
                  onTap: () {
                    _selectedIds.clear();
                    notifier.setFilter(
                      state.filter.copyWith(archived: !state.filter.archived),
                    );
                  },
                ),
                const SizedBox(width: 8),
                if (!state.filter.archived)
                  _ChipButton(
                    label: 'Mark All Read',
                    active: false,
                    onTap: () => notifier.markAllRead(),
                  ),
                const SizedBox(width: 8),
                IconButton(
                  icon: Icon(
                    Icons.refresh,
                    color: colors.textMuted,
                    size: 20,
                  ),
                  onPressed: () => notifier.fetch(),
                ),
              ],
            ),
          ),

          // Filter bar
          _FilterBar(
            filter: state.filter,
            searchController: _searchController,
            onFilterChanged: (f) {
              _selectedIds.clear();
              notifier.setFilter(f);
            },
          ),

          // Bulk action bar
          if (_selectedIds.isNotEmpty)
            _BulkActionBar(
              count: _selectedIds.length,
              archived: state.filter.archived,
              onMarkRead: () async {
                await notifier.markRead(_selectedIds.toList());
                setState(() => _selectedIds.clear());
              },
              onMarkUnread: () async {
                await notifier.markUnread(_selectedIds.toList());
                setState(() => _selectedIds.clear());
              },
              onArchive: () async {
                await notifier.archive(_selectedIds.toList());
                setState(() => _selectedIds.clear());
              },
              onRestore: () async {
                await notifier.restore(_selectedIds.toList());
                setState(() => _selectedIds.clear());
              },
              onDelete: () async {
                final confirmed = await showDialog<bool>(
                  context: context,
                  builder: (ctx) => AlertDialog(
                    title: const Text('Delete Notifications'),
                    content: Text(
                      'Permanently delete ${_selectedIds.length} notification(s)?',
                    ),
                    actions: [
                      TextButton(
                        onPressed: () => Navigator.pop(ctx, false),
                        child: const Text('Cancel'),
                      ),
                      TextButton(
                        onPressed: () => Navigator.pop(ctx, true),
                        child: const Text('Delete'),
                      ),
                    ],
                  ),
                );
                if (confirmed == true) {
                  await notifier.deleteNotifications(_selectedIds.toList());
                  setState(() => _selectedIds.clear());
                }
              },
            ),

          // Select-all header
          Container(
            color: colors.bgSecondary,
            padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 8),
            child: Row(
              children: [
                SizedBox(
                  width: 24,
                  height: 24,
                  child: Checkbox(
                    value: state.notifications.isNotEmpty &&
                        state.notifications.every((n) => _selectedIds.contains(n.id)),
                    onChanged: (_) {
                      setState(() {
                        if (state.notifications.every((n) => _selectedIds.contains(n.id))) {
                          _selectedIds.clear();
                        } else {
                          _selectedIds.addAll(state.notifications.map((n) => n.id));
                        }
                      });
                    },
                  ),
                ),
                const SizedBox(width: 8),
                Text(
                  state.notifications.every((n) => _selectedIds.contains(n.id)) && state.notifications.isNotEmpty
                      ? 'Deselect all'
                      : 'Select all',
                  style: TextStyle(fontSize: 12, color: colors.textMuted),
                ),
              ],
            ),
          ),

          // Notification list
          Expanded(
            child: state.loading && state.notifications.isEmpty
                ? Center(
                    child: CircularProgressIndicator(color: colors.accent),
                  )
                : state.notifications.isEmpty
                    ? Center(
                        child: Text(
                          state.filter.archived
                              ? 'No archived notifications'
                              : 'No notifications',
                          style: TextStyle(
                            fontSize: 14,
                            color: colors.textMuted,
                          ),
                        ),
                      )
                    : ListView.builder(
                        itemCount: state.notifications.length,
                        itemBuilder: (context, index) {
                          final n = state.notifications[index];
                          return _NotificationRow(
                            item: n,
                            selected: _selectedIds.contains(n.id),
                            onToggleSelect: () {
                              setState(() {
                                if (_selectedIds.contains(n.id)) {
                                  _selectedIds.remove(n.id);
                                } else {
                                  _selectedIds.add(n.id);
                                }
                              });
                            },
                            onMarkRead: () => notifier.markRead([n.id]),
                            onMarkUnread: () => notifier.markUnread([n.id]),
                            onArchive: () => notifier.archive([n.id]),
                            onRestore: () => notifier.restore([n.id]),
                          );
                        },
                      ),
          ),

          // Pagination
          if (state.total > 0)
            Container(
              padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
              decoration: BoxDecoration(
                border: Border(
                  top: BorderSide(color: colors.border),
                ),
              ),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.spaceBetween,
                children: [
                  Text(
                    'Showing $startItem-$endItem of ${state.total}',
                    style: TextStyle(fontSize: 12, color: colors.textMuted),
                  ),
                  Row(
                    children: [
                      TextButton(
                        onPressed: state.offset > 0 ? () => notifier.prevPage() : null,
                        child: const Text('Previous'),
                      ),
                      const SizedBox(width: 8),
                      TextButton(
                        onPressed: state.offset + state.pageSize < state.total
                            ? () => notifier.nextPage()
                            : null,
                        child: const Text('Next'),
                      ),
                    ],
                  ),
                ],
              ),
            ),
        ],
      ),
    );
  }
}

// -------------------------------------------------------------------
// Filter bar widget
// -------------------------------------------------------------------
class _FilterBar extends StatelessWidget {
  final NotificationFilter filter;
  final TextEditingController searchController;
  final ValueChanged<NotificationFilter> onFilterChanged;

  const _FilterBar({
    required this.filter,
    required this.searchController,
    required this.onFilterChanged,
  });

  @override
  Widget build(BuildContext context) {
    final colors = NvrColors.of(context);

    return Container(
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: colors.bgSecondary,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: colors.border),
      ),
      child: Wrap(
        spacing: 8,
        runSpacing: 8,
        crossAxisAlignment: WrapCrossAlignment.center,
        children: [
          // Search
          SizedBox(
            width: 180,
            height: 34,
            child: TextField(
              controller: searchController,
              onSubmitted: (v) => onFilterChanged(filter.copyWith(query: v)),
              style: TextStyle(fontSize: 13, color: colors.textPrimary),
              decoration: InputDecoration(
                hintText: 'Search...',
                hintStyle: TextStyle(fontSize: 13, color: colors.textMuted),
                isDense: true,
                contentPadding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
                border: OutlineInputBorder(borderRadius: BorderRadius.circular(6)),
                suffixIcon: IconButton(
                  icon: const Icon(Icons.search, size: 18),
                  onPressed: () =>
                      onFilterChanged(filter.copyWith(query: searchController.text)),
                ),
              ),
            ),
          ),

          // Type dropdown
          _FilterDropdown(
            value: filter.type,
            hint: 'All Types',
            items: _eventTypes,
            labelBuilder: _typeLabel,
            onChanged: (v) => onFilterChanged(filter.copyWith(type: v ?? '')),
          ),

          // Severity dropdown
          _FilterDropdown(
            value: filter.severity,
            hint: 'All Severities',
            items: _severityLevels,
            labelBuilder: (s) => s[0].toUpperCase() + s.substring(1),
            onChanged: (v) => onFilterChanged(filter.copyWith(severity: v ?? '')),
          ),

          // Read filter
          _FilterDropdown(
            value: filter.read,
            hint: 'Read & Unread',
            items: const ['false', 'true'],
            labelBuilder: (v) => v == 'false' ? 'Unread Only' : 'Read Only',
            onChanged: (v) => onFilterChanged(filter.copyWith(read: v ?? '')),
          ),

          // Camera text input
          SizedBox(
            width: 120,
            height: 34,
            child: TextField(
              onSubmitted: (v) => onFilterChanged(filter.copyWith(camera: v)),
              style: TextStyle(fontSize: 13, color: colors.textPrimary),
              decoration: InputDecoration(
                hintText: 'Camera...',
                hintStyle: TextStyle(fontSize: 13, color: colors.textMuted),
                isDense: true,
                contentPadding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
                border: OutlineInputBorder(borderRadius: BorderRadius.circular(6)),
              ),
            ),
          ),

          // Clear filters
          if (filter.hasActiveFilters)
            TextButton(
              onPressed: () {
                searchController.clear();
                onFilterChanged(const NotificationFilter());
              },
              child: Text(
                'Clear Filters',
                style: TextStyle(fontSize: 12, color: colors.textMuted),
              ),
            ),
        ],
      ),
    );
  }
}

// -------------------------------------------------------------------
// Filter dropdown helper
// -------------------------------------------------------------------
class _FilterDropdown extends StatelessWidget {
  final String value;
  final String hint;
  final List<String> items;
  final String Function(String) labelBuilder;
  final ValueChanged<String?> onChanged;

  const _FilterDropdown({
    required this.value,
    required this.hint,
    required this.items,
    required this.labelBuilder,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    final colors = NvrColors.of(context);
    return Container(
      height: 34,
      padding: const EdgeInsets.symmetric(horizontal: 8),
      decoration: BoxDecoration(
        border: Border.all(color: colors.border),
        borderRadius: BorderRadius.circular(6),
      ),
      child: DropdownButtonHideUnderline(
        child: DropdownButton<String>(
          value: value.isEmpty ? null : value,
          hint: Text(hint, style: TextStyle(fontSize: 13, color: colors.textMuted)),
          isDense: true,
          style: TextStyle(fontSize: 13, color: colors.textPrimary),
          dropdownColor: colors.bgSecondary,
          items: [
            DropdownMenuItem<String>(
              value: '',
              child: Text(hint, style: TextStyle(color: colors.textMuted)),
            ),
            ...items.map(
              (v) => DropdownMenuItem<String>(
                value: v,
                child: Text(labelBuilder(v)),
              ),
            ),
          ],
          onChanged: onChanged,
        ),
      ),
    );
  }
}

// -------------------------------------------------------------------
// Notification row widget
// -------------------------------------------------------------------
class _NotificationRow extends StatelessWidget {
  final NotificationItem item;
  final bool selected;
  final VoidCallback onToggleSelect;
  final VoidCallback onMarkRead;
  final VoidCallback onMarkUnread;
  final VoidCallback onArchive;
  final VoidCallback onRestore;

  const _NotificationRow({
    required this.item,
    required this.selected,
    required this.onToggleSelect,
    required this.onMarkRead,
    required this.onMarkUnread,
    required this.onArchive,
    required this.onRestore,
  });

  @override
  Widget build(BuildContext context) {
    final colors = NvrColors.of(context);
    final isRead = item.isRead;

    return Container(
      decoration: BoxDecoration(
        color: isRead ? null : colors.bgSecondary.withOpacity(0.3),
        border: Border(bottom: BorderSide(color: colors.border, width: 0.5)),
      ),
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
      child: Row(
        children: [
          // Checkbox
          SizedBox(
            width: 24,
            height: 24,
            child: Checkbox(value: selected, onChanged: (_) => onToggleSelect()),
          ),
          const SizedBox(width: 8),

          // Unread dot
          SizedBox(
            width: 10,
            child: isRead
                ? const SizedBox.shrink()
                : Container(
                    width: 8,
                    height: 8,
                    decoration: BoxDecoration(
                      shape: BoxShape.circle,
                      color: colors.accent,
                    ),
                  ),
          ),
          const SizedBox(width: 8),

          // Content
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    _SeverityBadge(severity: item.severity),
                    const SizedBox(width: 6),
                    Text(
                      _typeLabel(item.type),
                      style: TextStyle(
                        fontSize: 12,
                        fontWeight: FontWeight.w600,
                        color: colors.textPrimary,
                      ),
                    ),
                    if (item.camera.isNotEmpty) ...[
                      const SizedBox(width: 6),
                      Flexible(
                        child: Text(
                          item.camera,
                          overflow: TextOverflow.ellipsis,
                          style: TextStyle(
                            fontSize: 12,
                            color: colors.textMuted,
                          ),
                        ),
                      ),
                    ],
                  ],
                ),
                const SizedBox(height: 2),
                Text(
                  item.message,
                  overflow: TextOverflow.ellipsis,
                  style: TextStyle(
                    fontSize: 13,
                    color: colors.textSecondary,
                  ),
                ),
              ],
            ),
          ),

          // Timestamp
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 8),
            child: Text(
              _relativeTime(item.createdAt),
              style: TextStyle(fontSize: 11, color: colors.textMuted),
            ),
          ),

          // Actions
          if (isRead)
            IconButton(
              icon: const Icon(Icons.mark_email_unread_outlined, size: 18),
              tooltip: 'Mark unread',
              onPressed: onMarkUnread,
              color: colors.textMuted,
              visualDensity: VisualDensity.compact,
            )
          else
            IconButton(
              icon: const Icon(Icons.drafts_outlined, size: 18),
              tooltip: 'Mark read',
              onPressed: onMarkRead,
              color: colors.textMuted,
              visualDensity: VisualDensity.compact,
            ),
          if (item.archived)
            IconButton(
              icon: const Icon(Icons.unarchive_outlined, size: 18),
              tooltip: 'Restore',
              onPressed: onRestore,
              color: colors.accent,
              visualDensity: VisualDensity.compact,
            )
          else
            IconButton(
              icon: const Icon(Icons.archive_outlined, size: 18),
              tooltip: 'Archive',
              onPressed: onArchive,
              color: colors.textMuted,
              visualDensity: VisualDensity.compact,
            ),
        ],
      ),
    );
  }
}

// -------------------------------------------------------------------
// Severity badge
// -------------------------------------------------------------------
class _SeverityBadge extends StatelessWidget {
  final String severity;
  const _SeverityBadge({required this.severity});

  @override
  Widget build(BuildContext context) {
    final color = _severityColor(severity);
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: color.withOpacity(0.15),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        severity.toUpperCase(),
        style: TextStyle(
          fontSize: 9,
          fontWeight: FontWeight.bold,
          color: color,
          letterSpacing: 0.5,
        ),
      ),
    );
  }
}

// -------------------------------------------------------------------
// Bulk action bar
// -------------------------------------------------------------------
class _BulkActionBar extends StatelessWidget {
  final int count;
  final bool archived;
  final VoidCallback onMarkRead;
  final VoidCallback onMarkUnread;
  final VoidCallback onArchive;
  final VoidCallback onRestore;
  final VoidCallback onDelete;

  const _BulkActionBar({
    required this.count,
    required this.archived,
    required this.onMarkRead,
    required this.onMarkUnread,
    required this.onArchive,
    required this.onRestore,
    required this.onDelete,
  });

  @override
  Widget build(BuildContext context) {
    final colors = NvrColors.of(context);
    return Container(
      margin: const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
      padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
      decoration: BoxDecoration(
        color: colors.accent.withOpacity(0.1),
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: colors.accent.withOpacity(0.2)),
      ),
      child: Row(
        children: [
          Text(
            '$count selected',
            style: TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w600,
              color: colors.accent,
            ),
          ),
          const SizedBox(width: 12),
          _ActionChip(label: 'Mark Read', onTap: onMarkRead),
          const SizedBox(width: 4),
          _ActionChip(label: 'Mark Unread', onTap: onMarkUnread),
          const SizedBox(width: 4),
          if (archived)
            _ActionChip(label: 'Restore', onTap: onRestore)
          else
            _ActionChip(label: 'Archive', onTap: onArchive),
          const SizedBox(width: 4),
          _ActionChip(label: 'Delete', onTap: onDelete, danger: true),
        ],
      ),
    );
  }
}

class _ActionChip extends StatelessWidget {
  final String label;
  final VoidCallback onTap;
  final bool danger;

  const _ActionChip({required this.label, required this.onTap, this.danger = false});

  @override
  Widget build(BuildContext context) {
    final colors = NvrColors.of(context);
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(4),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
        decoration: BoxDecoration(
          color: danger ? Colors.red.withOpacity(0.1) : colors.bgSecondary,
          borderRadius: BorderRadius.circular(4),
          border: Border.all(
            color: danger ? Colors.red.withOpacity(0.2) : colors.border,
          ),
        ),
        child: Text(
          label,
          style: TextStyle(
            fontSize: 11,
            color: danger ? Colors.red[300] : colors.textSecondary,
          ),
        ),
      ),
    );
  }
}

// -------------------------------------------------------------------
// Chip button helper
// -------------------------------------------------------------------
class _ChipButton extends StatelessWidget {
  final String label;
  final bool active;
  final VoidCallback onTap;

  const _ChipButton({
    required this.label,
    required this.active,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final colors = NvrColors.of(context);
    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(8),
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 6),
        decoration: BoxDecoration(
          color: active ? colors.accent.withOpacity(0.2) : colors.bgSecondary,
          borderRadius: BorderRadius.circular(8),
          border: Border.all(
            color: active ? colors.accent.withOpacity(0.3) : colors.border,
          ),
        ),
        child: Text(
          label,
          style: TextStyle(
            fontSize: 13,
            color: active ? colors.accent : colors.textSecondary,
          ),
        ),
      ),
    );
  }
}
