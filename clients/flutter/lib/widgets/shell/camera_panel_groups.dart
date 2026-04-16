import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../models/camera_group.dart';
import '../../models/camera.dart';
import '../../providers/groups_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/camera_panel_provider.dart';
import '../../providers/grid_layout_provider.dart';
import '../../providers/auth_provider.dart';
import '../hud/camera_thumbnail.dart';
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
    final serverUrl = ref.watch(authProvider).serverUrl ?? '';

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Section header row
        Padding(
          padding: const EdgeInsets.fromLTRB(10, 8, 10, 6),
          child: Row(
            children: [
              Text('GROUPS', style: NvrTypography.of(context).monoSection),
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
                child: Text('No groups yet', style: NvrTypography.of(context).body),
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
                  serverUrl: serverUrl,
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
                    ref.read(gridLayoutProvider.notifier).fillFromGroup(group.cameraIds);
                  },
                  onRename: () => _showEditGroupDialog(context, ref, group),
                  onDelete: () => _showDeleteGroupDialog(context, ref, group),
                );
              }).toList(),
            );
          },
          loading: () => Padding(
            padding: EdgeInsets.all(10),
            child: Center(
              child: CircularProgressIndicator(
                  color: NvrColors.of(context).accent, strokeWidth: 1.5),
            ),
          ),
          error: (e, _) => Padding(
            padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
            child: Text('Error loading groups', style: NvrTypography.of(context).alert),
          ),
        ),

        // Thin separator before camera list
        Divider(height: 1, thickness: 1, color: NvrColors.of(context).border),
      ],
    );
  }

  // -------------------------------------------------------------------------
  // Dialogs
  // -------------------------------------------------------------------------

  void _showCreateGroupDialog(BuildContext context, WidgetRef ref) {
    showDialog<void>(
      context: context,
      builder: (ctx) => _GroupEditDialog(
        title: 'NEW GROUP',
        onConfirm: (name, cameraIds) async {
          final api = ref.read(apiClientProvider);
          if (api != null && name.isNotEmpty) {
            await api.post('/camera-groups', data: {
              'name': name,
              'camera_ids': cameraIds,
            });
            ref.invalidate(groupsProvider);
          }
        },
      ),
    );
  }

  void _showEditGroupDialog(
      BuildContext context, WidgetRef ref, CameraGroup group) {
    showDialog<void>(
      context: context,
      builder: (ctx) => _GroupEditDialog(
        title: 'EDIT GROUP',
        initialName: group.name,
        initialCameraIds: group.cameraIds,
        onConfirm: (name, cameraIds) async {
          final api = ref.read(apiClientProvider);
          if (api != null && name.isNotEmpty) {
            await api.put('/camera-groups/${group.id}', data: {
              'name': name,
              'camera_ids': cameraIds,
            });
            ref.invalidate(groupsProvider);
          }
        },
      ),
    );
  }

  void _showDeleteGroupDialog(
      BuildContext context, WidgetRef ref, CameraGroup group) {
    showDialog<void>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: NvrColors.of(context).bgSecondary,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(6),
          side: BorderSide(color: NvrColors.of(context).border),
        ),
        title: Text('DELETE GROUP', style: NvrTypography.of(context).monoSection),
        content: Text(
          'Delete "${group.name}"? Cameras will not be removed.',
          style: NvrTypography.of(context).body,
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(),
            child: Text('CANCEL', style: NvrTypography.of(context).monoControl),
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
                style: NvrTypography.of(context).monoControl
                    .copyWith(color: NvrColors.of(context).danger)),
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
    required this.serverUrl,
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
  final String serverUrl;
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
                ? NvrColors.of(context).accent.withOpacity(0.06)
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
                    color: NvrColors.of(context).textMuted,
                  ),
                ),
                const SizedBox(width: 4),
                // Group name (tappable to filter)
                Expanded(
                  child: GestureDetector(
                    onTap: onTapFilter,
                    child: Text(
                      group.name.toUpperCase(),
                      style: NvrTypography.of(context).monoLabel.copyWith(
                        color: isActive
                            ? NvrColors.of(context).accent
                            : NvrColors.of(context).textPrimary,
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
                  style: NvrTypography.of(context).monoLabel,
                ),
                const SizedBox(width: 8),
                // Play-group button
                GestureDetector(
                  onTap: onPlayGroup,
                  child: Icon(
                    Icons.play_circle_outline,
                    size: 16,
                    color: NvrColors.of(context).accent,
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
              child: Text('No cameras', style: NvrTypography.of(context).body),
            )
          else
            ...cameras.map((cam) => _GroupCameraItem(camera: cam, serverUrl: serverUrl)),
        ],
      ],
    );
  }

  void _showContextMenu(BuildContext context) {
    final RenderBox renderBox = context.findRenderObject() as RenderBox;
    final offset = renderBox.localToGlobal(Offset.zero);

    showMenu<String>(
      context: context,
      color: NvrColors.of(context).bgSecondary,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(6),
        side: BorderSide(color: NvrColors.of(context).border),
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
          child: Text('Rename', style: NvrTypography.of(context).body),
        ),
        PopupMenuItem(
          value: 'delete',
          child: Text('Delete',
              style: NvrTypography.of(context).body.copyWith(color: NvrColors.of(context).danger)),
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
  const _GroupCameraItem({required this.camera, required this.serverUrl});
  final Camera camera;
  final String serverUrl;

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
                color: NvrColors.of(context).bgSecondary,
                border: Border.all(color: NvrColors.of(context).accent),
                borderRadius: BorderRadius.circular(6),
                boxShadow: [
                  BoxShadow(color: NvrColors.of(context).accent.withOpacity(0.2), blurRadius: 12),
                ],
              ),
              child: Text(
                camera.name,
                style: TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w500,
                  color: NvrColors.of(context).textPrimary,
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
              color: NvrColors.of(context).bgTertiary,
              border: Border.all(color: NvrColors.of(context).border),
            ),
            child: Row(
              children: [
                const SizedBox(width: 6 + 8 + 44 + 8),
                Expanded(child: Text(camera.name, style: TextStyle(fontSize: 11, color: NvrColors.of(context).textMuted))),
              ],
            ),
          ),
        ),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 5),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(6),
            color: NvrColors.of(context).bgTertiary,
            border: Border.all(color: NvrColors.of(context).border),
          ),
          child: Row(
            children: [
              // Status dot
              Container(
                width: 6,
                height: 6,
                decoration: BoxDecoration(
                  shape: BoxShape.circle,
                  color: isOnline ? NvrColors.of(context).success : NvrColors.of(context).danger,
                  boxShadow: isOnline
                      ? [
                          BoxShadow(
                              color: NvrColors.of(context).success.withOpacity(0.5),
                              blurRadius: 4)
                        ]
                      : null,
                ),
              ),
              const SizedBox(width: 8),
              // Camera thumbnail
              CameraThumbnail(
                serverUrl: serverUrl,
                cameraId: camera.id,
                width: 44,
                height: 26,
              ),
              const SizedBox(width: 8),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      camera.name,
                      style: TextStyle(
                        fontSize: 11,
                        fontWeight: FontWeight.w500,
                        color: NvrColors.of(context).textPrimary,
                      ),
                      overflow: TextOverflow.ellipsis,
                    ),
                    Text(
                      camera.id.substring(0, 8).toUpperCase(),
                      style: TextStyle(
                        fontFamily: 'JetBrainsMono',
                        fontSize: 8,
                        color: NvrColors.of(context).textMuted,
                      ),
                    ),
                  ],
                ),
              ),
              Icon(Icons.drag_handle, size: 14, color: NvrColors.of(context).border),
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

class _GroupEditDialog extends ConsumerStatefulWidget {
  const _GroupEditDialog({
    required this.title,
    required this.onConfirm,
    this.initialName,
    this.initialCameraIds,
  });

  final String title;
  final Future<void> Function(String name, List<String> cameraIds) onConfirm;
  final String? initialName;
  final List<String>? initialCameraIds;

  @override
  ConsumerState<_GroupEditDialog> createState() => _GroupEditDialogState();
}

class _GroupEditDialogState extends ConsumerState<_GroupEditDialog> {
  late final TextEditingController _nameController;
  late final Set<String> _selectedCameraIds;

  @override
  void initState() {
    super.initState();
    _nameController = TextEditingController(text: widget.initialName ?? '');
    _selectedCameraIds = {...?widget.initialCameraIds};
  }

  @override
  void dispose() {
    _nameController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final camerasAsync = ref.watch(camerasProvider);
    final cameras = camerasAsync.valueOrNull ?? <Camera>[];

    return AlertDialog(
      backgroundColor: NvrColors.of(context).bgSecondary,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(6),
        side: BorderSide(color: NvrColors.of(context).border),
      ),
      title: Text(widget.title, style: NvrTypography.of(context).monoSection),
      content: SizedBox(
        width: 280,
        child: SingleChildScrollView(
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            mainAxisSize: MainAxisSize.min,
            children: [
              // Name field
              TextField(
                controller: _nameController,
                autofocus: true,
                style: TextStyle(
                    fontSize: 13, color: NvrColors.of(context).textPrimary),
                cursorColor: NvrColors.of(context).accent,
                decoration: InputDecoration(
                  hintText: 'Group name',
                  hintStyle: TextStyle(
                      color: NvrColors.of(context).textMuted, fontSize: 13),
                  isDense: true,
                  contentPadding:
                      const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
                  filled: true,
                  fillColor: NvrColors.of(context).bgTertiary,
                  border: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(6),
                    borderSide: BorderSide(color: NvrColors.of(context).border),
                  ),
                  focusedBorder: OutlineInputBorder(
                    borderRadius: BorderRadius.circular(6),
                    borderSide:
                        BorderSide(color: NvrColors.of(context).accent),
                  ),
                ),
              ),
              const SizedBox(height: 14),

              // Camera selection
              Text('CAMERAS', style: NvrTypography.of(context).monoControl),
              const SizedBox(height: 6),
              if (cameras.isEmpty)
                Text('No cameras available',
                    style: NvrTypography.of(context).body)
              else
                ConstrainedBox(
                  constraints: const BoxConstraints(maxHeight: 200),
                  child: ListView.builder(
                    shrinkWrap: true,
                    itemCount: cameras.length,
                    itemBuilder: (_, i) {
                      final cam = cameras[i];
                      final selected = _selectedCameraIds.contains(cam.id);
                      return InkWell(
                        onTap: () => setState(() {
                          if (selected) {
                            _selectedCameraIds.remove(cam.id);
                          } else {
                            _selectedCameraIds.add(cam.id);
                          }
                        }),
                        child: Padding(
                          padding: const EdgeInsets.symmetric(vertical: 3),
                          child: Row(
                            children: [
                              Container(
                                width: 14,
                                height: 14,
                                decoration: BoxDecoration(
                                  borderRadius: BorderRadius.circular(3),
                                  border: Border.all(
                                    color: selected
                                        ? NvrColors.of(context).accent
                                        : NvrColors.of(context).border,
                                  ),
                                  color: selected
                                      ? NvrColors.of(context).accent
                                          .withValues(alpha: 0.2)
                                      : Colors.transparent,
                                ),
                                child: selected
                                    ? Icon(Icons.check,
                                        size: 10,
                                        color: NvrColors.of(context).accent)
                                    : null,
                              ),
                              const SizedBox(width: 8),
                              Expanded(
                                child: Text(
                                  cam.name,
                                  style: TextStyle(
                                    fontSize: 12,
                                    color: NvrColors.of(context).textPrimary,
                                  ),
                                  overflow: TextOverflow.ellipsis,
                                ),
                              ),
                            ],
                          ),
                        ),
                      );
                    },
                  ),
                ),
            ],
          ),
        ),
      ),
      actions: [
        TextButton(
          onPressed: () => Navigator.of(context).pop(),
          child: Text('CANCEL', style: NvrTypography.of(context).monoControl),
        ),
        TextButton(
          onPressed: () async {
            await widget.onConfirm(
              _nameController.text.trim(),
              _selectedCameraIds.toList(),
            );
            if (mounted) Navigator.of(context).pop();
          },
          child: Text(
            widget.initialName != null ? 'SAVE' : 'CREATE',
            style: NvrTypography.of(context).monoControl
                .copyWith(color: NvrColors.of(context).accent),
          ),
        ),
      ],
    );
  }
}
