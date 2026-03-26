import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../models/camera_group.dart';
import '../../models/camera.dart';
import '../../providers/groups_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/camera_panel_provider.dart';
import '../../providers/auth_provider.dart';
import '../hud/hud_button.dart';

// ---------------------------------------------------------------------------
// Local state: which groups are collapsed
// ---------------------------------------------------------------------------

final _collapsedGroupsProvider =
    StateProvider<Set<String>>((_) => <String>{});

// ---------------------------------------------------------------------------
// CameraPanelGroups
// ---------------------------------------------------------------------------

class CameraPanelGroups extends ConsumerWidget {
  const CameraPanelGroups({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final groupsAsync = ref.watch(groupsProvider);
    final camerasAsync = ref.watch(camerasProvider);
    final panelState = ref.watch(cameraPanelProvider);
    final collapsed = ref.watch(_collapsedGroupsProvider);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Section header row
        Padding(
          padding: const EdgeInsets.fromLTRB(10, 8, 10, 6),
          child: Row(
            children: [
              Text('GROUPS', style: NvrTypography.monoSection),
              const Spacer(),
              HudButton(
                label: '+ GROUP',
                style: HudButtonStyle.tactical,
                onPressed: () => _showCreateGroupDialog(context, ref),
              ),
            ],
          ),
        ),

        groupsAsync.when(
          data: (groups) {
            if (groups.isEmpty) {
              return Padding(
                padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
                child: Text('No groups yet', style: NvrTypography.body),
              );
            }

            return Column(
              children: groups.map((group) {
                final isCollapsed = collapsed.contains(group.id);
                final isActive = panelState.activeGroupId == group.id;
                final cameras = camerasAsync.valueOrNull ?? [];
                final groupCameras = cameras
                    .where((c) => group.cameraIds.contains(c.id))
                    .toList();

                return _GroupSection(
                  group: group,
                  cameras: groupCameras,
                  isCollapsed: isCollapsed,
                  isActive: isActive,
                  onToggleCollapse: () {
                    ref.read(_collapsedGroupsProvider.notifier).update((s) {
                      final next = Set<String>.from(s);
                      if (next.contains(group.id)) {
                        next.remove(group.id);
                      } else {
                        next.add(group.id);
                      }
                      return next;
                    });
                  },
                  onTapFilter: () =>
                      ref.read(cameraPanelProvider.notifier).setGroupFilter(group.id),
                  onPlayGroup: () {
                    // TODO: trigger group playback / tour when implemented
                  },
                  onRename: () => _showRenameGroupDialog(context, ref, group),
                  onDelete: () => _showDeleteGroupDialog(context, ref, group),
                );
              }).toList(),
            );
          },
          loading: () => const Padding(
            padding: EdgeInsets.all(10),
            child: Center(
              child: CircularProgressIndicator(
                  color: NvrColors.accent, strokeWidth: 1.5),
            ),
          ),
          error: (e, _) => Padding(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
            child: Text('Error loading groups', style: NvrTypography.alert),
          ),
        ),

        // Thin separator before camera list
        const Divider(height: 1, thickness: 1, color: NvrColors.border),
      ],
    );
  }

  // -------------------------------------------------------------------------
  // Dialogs
  // -------------------------------------------------------------------------

  void _showCreateGroupDialog(BuildContext context, WidgetRef ref) {
    final controller = TextEditingController();
    showDialog<void>(
      context: context,
      builder: (ctx) => _GroupNameDialog(
        title: 'NEW GROUP',
        controller: controller,
        onConfirm: () async {
          final name = controller.text.trim();
          if (name.isNotEmpty) {
            final api = ref.read(apiClientProvider);
            if (api != null) {
              await api.post('/camera-groups', data: {'name': name});
              ref.invalidate(groupsProvider);
            }
          }
          if (ctx.mounted) Navigator.of(ctx).pop();
        },
      ),
    );
  }

  void _showRenameGroupDialog(
      BuildContext context, WidgetRef ref, CameraGroup group) {
    final controller = TextEditingController(text: group.name);
    showDialog<void>(
      context: context,
      builder: (ctx) => _GroupNameDialog(
        title: 'RENAME GROUP',
        controller: controller,
        onConfirm: () async {
          final name = controller.text.trim();
          if (name.isNotEmpty) {
            final api = ref.read(apiClientProvider);
            if (api != null) {
              await api.put('/camera-groups/${group.id}', data: {'name': name});
              ref.invalidate(groupsProvider);
            }
          }
          if (ctx.mounted) Navigator.of(ctx).pop();
        },
      ),
    );
  }

  void _showDeleteGroupDialog(
      BuildContext context, WidgetRef ref, CameraGroup group) {
    showDialog<void>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: NvrColors.bgSecondary,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(6),
          side: const BorderSide(color: NvrColors.border),
        ),
        title: Text('DELETE GROUP', style: NvrTypography.monoSection),
        content: Text(
          'Delete "${group.name}"? Cameras will not be removed.',
          style: NvrTypography.body,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(),
            child: Text('CANCEL', style: NvrTypography.monoControl),
          ),
          TextButton(
            onPressed: () async {
              final api = ref.read(apiClientProvider);
              if (api != null) {
                await api.delete('/camera-groups/${group.id}');
                ref.invalidate(groupsProvider);
              }
              if (ctx.mounted) Navigator.of(ctx).pop();
            },
            child: Text('DELETE',
                style: NvrTypography.monoControl
                    .copyWith(color: NvrColors.danger)),
          ),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// _GroupSection — one collapsible group row
// ---------------------------------------------------------------------------

class _GroupSection extends StatelessWidget {
  const _GroupSection({
    required this.group,
    required this.cameras,
    required this.isCollapsed,
    required this.isActive,
    required this.onToggleCollapse,
    required this.onTapFilter,
    required this.onPlayGroup,
    required this.onRename,
    required this.onDelete,
  });

  final CameraGroup group;
  final List<Camera> cameras;
  final bool isCollapsed;
  final bool isActive;
  final VoidCallback onToggleCollapse;
  final VoidCallback onTapFilter;
  final VoidCallback onPlayGroup;
  final VoidCallback onRename;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Header row
        GestureDetector(
          onLongPress: () => _showContextMenu(context),
          child: Container(
            color: isActive
                ? NvrColors.accent.withOpacity(0.06)
                : Colors.transparent,
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
            child: Row(
              children: [
                // Collapse arrow
                GestureDetector(
                  onTap: onToggleCollapse,
                  child: Icon(
                    isCollapsed ? Icons.chevron_right : Icons.expand_more,
                    size: 16,
                    color: NvrColors.textMuted,
                  ),
                ),
                const SizedBox(width: 4),
                // Group name (tappable to filter)
                Expanded(
                  child: GestureDetector(
                    onTap: onTapFilter,
                    child: Text(
                      group.name.toUpperCase(),
                      style: NvrTypography.monoLabel.copyWith(
                        color: isActive
                            ? NvrColors.accent
                            : NvrColors.textPrimary,
                        fontSize: 10,
                        letterSpacing: 1.2,
                      ),
                      overflow: TextOverflow.ellipsis,
                    ),
                  ),
                ),
                const SizedBox(width: 6),
                // Camera count
                Text(
                  '${cameras.length}',
                  style: NvrTypography.monoLabel,
                ),
                const SizedBox(width: 8),
                // Play-group button
                GestureDetector(
                  onTap: onPlayGroup,
                  child: const Icon(
                    Icons.play_circle_outline,
                    size: 16,
                    color: NvrColors.accent,
                  ),
                ),
              ],
            ),
          ),
        ),

        // Expanded camera list
        if (!isCollapsed) ...[
          if (cameras.isEmpty)
            Padding(
              padding: const EdgeInsets.only(left: 28, bottom: 4),
              child: Text('No cameras', style: NvrTypography.body),
            )
          else
            ...cameras.map((cam) => _GroupCameraItem(camera: cam)),
        ],
      ],
    );
  }

  void _showContextMenu(BuildContext context) {
    final RenderBox renderBox = context.findRenderObject() as RenderBox;
    final offset = renderBox.localToGlobal(Offset.zero);

    showMenu<String>(
      context: context,
      color: NvrColors.bgSecondary,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(6),
        side: const BorderSide(color: NvrColors.border),
      ),
      position: RelativeRect.fromLTRB(
        offset.dx,
        offset.dy + renderBox.size.height,
        offset.dx + 120,
        0,
      ),
      items: [
        PopupMenuItem(
          value: 'rename',
          child: Text('Rename', style: NvrTypography.body),
        ),
        PopupMenuItem(
          value: 'delete',
          child: Text('Delete',
              style: NvrTypography.body.copyWith(color: NvrColors.danger)),
        ),
      ],
    ).then((value) {
      if (value == 'rename') onRename();
      if (value == 'delete') onDelete();
    });
  }
}

// ---------------------------------------------------------------------------
// _GroupCameraItem — compact camera row inside a group
// ---------------------------------------------------------------------------

class _GroupCameraItem extends StatelessWidget {
  const _GroupCameraItem({required this.camera});
  final Camera camera;

  @override
  Widget build(BuildContext context) {
    final isOnline = camera.status == 'online';
    return Padding(
      padding: const EdgeInsets.only(left: 26, right: 10, bottom: 3),
      child: LongPressDraggable<String>(
        data: camera.id,
        feedback: Material(
          color: Colors.transparent,
          child: Opacity(
            opacity: 0.85,
            child: Container(
              width: 160,
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
              decoration: BoxDecoration(
                color: NvrColors.bgSecondary,
                border: Border.all(color: NvrColors.accent),
                borderRadius: BorderRadius.circular(6),
                boxShadow: [
                  BoxShadow(color: NvrColors.accent.withOpacity(0.2), blurRadius: 12),
                ],
              ),
              child: Text(
                camera.name,
                style: const TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w500,
                  color: NvrColors.textPrimary,
                  decoration: TextDecoration.none,
                ),
                overflow: TextOverflow.ellipsis,
              ),
            ),
          ),
        ),
        childWhenDragging: Opacity(
          opacity: 0.3,
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 5),
            decoration: BoxDecoration(
              borderRadius: BorderRadius.circular(6),
              color: NvrColors.bgTertiary,
              border: Border.all(color: NvrColors.border),
            ),
            child: Row(
              children: [
                const SizedBox(width: 6 + 8 + 44 + 8),
                Expanded(child: Text(camera.name, style: const TextStyle(fontSize: 11, color: NvrColors.textMuted))),
              ],
            ),
          ),
        ),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 5),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(6),
            color: NvrColors.bgTertiary,
            border: Border.all(color: NvrColors.border),
          ),
          child: Row(
            children: [
              // Status dot
              Container(
                width: 6,
                height: 6,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: isOnline ? NvrColors.success : NvrColors.danger,
                  boxShadow: isOnline
                      ? [
                          BoxShadow(
                              color: NvrColors.success.withOpacity(0.5),
                              blurRadius: 4)
                        ]
                      : null,
                ),
              ),
              const SizedBox(width: 8),
              // Thumbnail placeholder
              Container(
                width: 44,
                height: 26,
                decoration: BoxDecoration(
                  color: NvrColors.border,
                  borderRadius: BorderRadius.circular(3),
                ),
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      camera.name,
                      style: const TextStyle(
                        fontSize: 11,
                        fontWeight: FontWeight.w500,
                        color: NvrColors.textPrimary,
                      ),
                      overflow: TextOverflow.ellipsis,
                    ),
                    Text(
                      camera.id.substring(0, 8).toUpperCase(),
                      style: const TextStyle(
                        fontFamily: 'JetBrainsMono',
                        fontSize: 8,
                        color: NvrColors.textMuted,
                      ),
                    ),
                  ],
                ),
              ),
              const Icon(Icons.drag_handle, size: 14, color: NvrColors.border),
            ],
          ),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// _GroupNameDialog — reusable name input dialog
// ---------------------------------------------------------------------------

class _GroupNameDialog extends StatelessWidget {
  const _GroupNameDialog({
    required this.title,
    required this.controller,
    required this.onConfirm,
  });

  final String title;
  final TextEditingController controller;
  final VoidCallback onConfirm;

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      backgroundColor: NvrColors.bgSecondary,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(6),
        side: const BorderSide(color: NvrColors.border),
      ),
      title: Text(title, style: NvrTypography.monoSection),
      content: TextField(
        controller: controller,
        autofocus: true,
        style: const TextStyle(fontSize: 13, color: NvrColors.textPrimary),
        cursorColor: NvrColors.accent,
        decoration: InputDecoration(
          hintText: 'Group name',
          hintStyle:
              const TextStyle(color: NvrColors.textMuted, fontSize: 13),
          isDense: true,
          contentPadding:
              const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
          filled: true,
          fillColor: NvrColors.bgTertiary,
          border: OutlineInputBorder(
            borderRadius: BorderRadius.circular(6),
            borderSide: const BorderSide(color: NvrColors.border),
          ),
          focusedBorder: OutlineInputBorder(
            borderRadius: BorderRadius.circular(6),
            borderSide: const BorderSide(color: NvrColors.accent),
          ),
        ),
        onSubmitted: (_) => onConfirm(),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: Text('CANCEL', style: NvrTypography.monoControl),
        ),
        TextButton(
          onPressed: onConfirm,
          child: Text(
            'CONFIRM',
            style: NvrTypography.monoControl.copyWith(color: NvrColors.accent),
          ),
        ),
      ],
    );
  }
}
