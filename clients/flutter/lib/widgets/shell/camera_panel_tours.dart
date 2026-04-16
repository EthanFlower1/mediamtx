import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../models/tour.dart';
import '../../models/camera.dart';
import '../../providers/tours_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/auth_provider.dart';
import '../hud/hud_button.dart';
import '../hud/status_badge.dart';

// ---------------------------------------------------------------------------
// CameraPanelTours
// ---------------------------------------------------------------------------

class CameraPanelTours extends ConsumerWidget {
  const CameraPanelTours({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final toursAsync = ref.watch(toursProvider);
    final activeTour = ref.watch(activeTourProvider);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Top border separator
        Divider(height: 1, thickness: 1, color: NvrColors.of(context).border),

        // Section header
        Padding(
          padding: const EdgeInsets.fromLTRB(10, 8, 10, 6),
          child: Row(
            children: [
              Text('TOURS', style: NvrTypography.of(context).monoSection),
              const Spacer(),
              HudButton(
                label: '+ NEW',
                style: HudButtonStyle.tactical,
                onPressed: () => _showCreateTourDialog(context, ref),
              ),
            ],
          ),
        ),

        toursAsync.when(
          data: (tours) {
            if (tours.isEmpty) {
              return Padding(
                padding:
                    const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
                child: Text('No tours yet', style: NvrTypography.of(context).body),
              );
            }

            return Column(
              children: tours.map((tour) {
                final isActive = activeTour.tour?.id == tour.id;
                return _TourItem(
                  tour: tour,
                  isActive: isActive,
                  onTap: () {
                    if (isActive) {
                      ref.read(activeTourProvider.notifier).stop();
                    } else {
                      ref.read(activeTourProvider.notifier).start(tour);
                    }
                  },
                  onEdit: () => _showEditTourDialog(context, ref, tour),
                  onDelete: () => _confirmDeleteTour(context, ref, tour),
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
            child: Text('Error loading tours', style: NvrTypography.of(context).alert),
          ),
        ),

        const SizedBox(height: 8),
      ],
    );
  }

  // -------------------------------------------------------------------------
  // Dialogs
  // -------------------------------------------------------------------------

  void _showCreateTourDialog(BuildContext context, WidgetRef ref) {
    showDialog<void>(
      context: context,
      builder: (ctx) => _CreateTourDialog(
        onConfirm: (name, cameraIds, dwellSeconds) async {
          final api = ref.read(apiClientProvider);
          if (api != null && name.isNotEmpty) {
            await api.post('/tours', data: {
              'name': name,
              'camera_ids': cameraIds,
              'dwell_seconds': dwellSeconds,
            });
            ref.invalidate(toursProvider);
          }
        },
      ),
    );
  }

  void _showEditTourDialog(BuildContext context, WidgetRef ref, Tour tour) {
    showDialog<void>(
      context: context,
      builder: (ctx) => _CreateTourDialog(
        initialName: tour.name,
        initialCameraIds: tour.cameraIds,
        initialDwellSeconds: tour.dwellSeconds,
        onConfirm: (name, cameraIds, dwellSeconds) async {
          final api = ref.read(apiClientProvider);
          if (api != null && name.isNotEmpty) {
            await api.put('/tours/${tour.id}', data: {
              'name': name,
              'camera_ids': cameraIds,
              'dwell_seconds': dwellSeconds,
            });
            // If this tour is currently active, restart it with new config.
            final active = ref.read(activeTourProvider);
            if (active.tour?.id == tour.id) {
              ref.read(activeTourProvider.notifier).start(
                tour.copyWith(
                  name: name,
                  cameraIds: cameraIds,
                  dwellSeconds: dwellSeconds,
                ),
              );
            }
            ref.invalidate(toursProvider);
          }
        },
      ),
    );
  }

  void _confirmDeleteTour(BuildContext context, WidgetRef ref, Tour tour) {
    showDialog<void>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: NvrColors.of(context).bgSecondary,
        shape: RoundedRectangleBorder(
          borderRadius: BorderRadius.circular(6),
          side: BorderSide(color: NvrColors.of(context).border),
        ),
        title: Text('DELETE TOUR', style: NvrTypography.of(context).monoSection),
        content: Text(
          'Delete tour "${tour.name}"?',
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
                // Stop if currently active
                final activeTour = ref.read(activeTourProvider);
                if (activeTour.tour?.id == tour.id) {
                  ref.read(activeTourProvider.notifier).stop();
                }
                await api.delete('/tours/${tour.id}');
                ref.invalidate(toursProvider);
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
// _TourItem
// ---------------------------------------------------------------------------

class _TourItem extends StatelessWidget {
  const _TourItem({
    required this.tour,
    required this.isActive,
    required this.onTap,
    required this.onEdit,
    required this.onDelete,
  });

  final Tour tour;
  final bool isActive;
  final VoidCallback onTap;
  final VoidCallback onEdit;
  final VoidCallback onDelete;

  void _showContextMenu(BuildContext context, TapDownDetails details) {
    final overlay = Overlay.of(context).context.findRenderObject() as RenderBox;
    showMenu<String>(
      context: context,
      position: RelativeRect.fromRect(
        details.globalPosition & const Size(1, 1),
        Offset.zero & overlay.size,
      ),
      color: NvrColors.of(context).bgSecondary,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(6),
        side: BorderSide(color: NvrColors.of(context).border),
      ),
      items: [
        PopupMenuItem(
          value: 'edit',
          height: 32,
          child: Row(
            children: [
              Icon(Icons.edit_outlined, size: 14, color: NvrColors.of(context).textSecondary),
              const SizedBox(width: 8),
              Text('Edit', style: TextStyle(fontSize: 12, color: NvrColors.of(context).textPrimary)),
            ],
          ),
        ),
        PopupMenuItem(
          value: 'delete',
          height: 32,
          child: Row(
            children: [
              Icon(Icons.delete_outline, size: 14, color: NvrColors.of(context).danger),
              const SizedBox(width: 8),
              Text('Delete', style: TextStyle(fontSize: 12, color: NvrColors.of(context).danger)),
            ],
          ),
        ),
      ],
    ).then((value) {
      if (value == 'edit') onEdit();
      if (value == 'delete') onDelete();
    });
  }

  @override
  Widget build(BuildContext context) {
    final camCount = tour.cameraIds.length;
    final configLabel = '$camCount CAM · ${tour.dwellSeconds}S EACH';

    return GestureDetector(
      onSecondaryTapDown: (details) => _showContextMenu(context, details),
      onLongPressStart: (details) => _showContextMenu(context,
          TapDownDetails(globalPosition: details.globalPosition)),
      child: InkWell(
        onTap: onTap,
        borderRadius: BorderRadius.circular(6),
        child: Container(
          margin: const EdgeInsets.symmetric(horizontal: 10, vertical: 3),
          padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 7),
          decoration: BoxDecoration(
            borderRadius: BorderRadius.circular(6),
            color: isActive
                ? NvrColors.of(context).accent.withValues(alpha: 0.08)
                : NvrColors.of(context).bgTertiary,
            border: Border.all(
              color: isActive
                  ? NvrColors.of(context).accent.withValues(alpha: 0.35)
                  : NvrColors.of(context).border,
            ),
          ),
          child: Row(
            children: [
              // Cycle icon
              Icon(
                Icons.refresh,
                size: 14,
                color: isActive ? NvrColors.of(context).accent : NvrColors.of(context).textMuted,
              ),
              const SizedBox(width: 8),
              // Name + config
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      tour.name,
                      style: TextStyle(
                        fontSize: 11,
                        fontWeight: FontWeight.w500,
                        color: NvrColors.of(context).textPrimary,
                      ),
                      overflow: TextOverflow.ellipsis,
                    ),
                    const SizedBox(height: 2),
                    Text(configLabel, style: NvrTypography.of(context).monoLabel),
                  ],
                ),
              ),
              // Active badge
              if (isActive) ...[
                const SizedBox(width: 6),
                StatusBadge(
                  label: 'ACTIVE',
                  color: NvrColors.of(context).success,
                  showDot: true,
                ),
              ],
            ],
          ),
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// _CreateTourDialog
// ---------------------------------------------------------------------------

class _CreateTourDialog extends ConsumerStatefulWidget {
  const _CreateTourDialog({
    required this.onConfirm,
    this.initialName,
    this.initialCameraIds,
    this.initialDwellSeconds,
  });

  final Future<void> Function(String name, List<String> cameraIds, int dwellSeconds) onConfirm;
  final String? initialName;
  final List<String>? initialCameraIds;
  final int? initialDwellSeconds;

  bool get isEditing => initialName != null;

  @override
  ConsumerState<_CreateTourDialog> createState() => _CreateTourDialogState();
}

class _CreateTourDialogState extends ConsumerState<_CreateTourDialog> {
  late final TextEditingController _nameController;
  late final Set<String> _selectedCameraIds;
  late double _dwellSeconds;

  @override
  void initState() {
    super.initState();
    _nameController = TextEditingController(text: widget.initialName ?? '');
    _selectedCameraIds = {...?widget.initialCameraIds};
    _dwellSeconds = (widget.initialDwellSeconds ?? 10).toDouble();
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
      title: Text(widget.isEditing ? 'EDIT TOUR' : 'NEW TOUR',
          style: NvrTypography.of(context).monoSection),
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
                  hintText: 'Tour name',
                  hintStyle: TextStyle(
                      color: NvrColors.of(context).textMuted, fontSize: 13),
                  isDense: true,
                  contentPadding: const EdgeInsets.symmetric(
                      horizontal: 10, vertical: 8),
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
                Text('No cameras available', style: NvrTypography.of(context).body)
              else
                ConstrainedBox(
                  constraints: const BoxConstraints(maxHeight: 160),
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
                          padding:
                              const EdgeInsets.symmetric(vertical: 3),
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
                                      ? NvrColors.of(context).accent.withOpacity(0.2)
                                      : Colors.transparent,
                                ),
                                child: selected
                                    ? Icon(Icons.check,
                                        size: 10, color: NvrColors.of(context).accent)
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

              const SizedBox(height: 14),

              // Dwell seconds slider
              Row(
                children: [
                  Text('DWELL', style: NvrTypography.of(context).monoControl),
                  const Spacer(),
                  Text(
                    '${_dwellSeconds.round()}S',
                    style: NvrTypography.of(context).monoLabel.copyWith(
                        color: NvrColors.of(context).accent),
                  ),
                ],
              ),
              SliderTheme(
                data: SliderThemeData(
                  activeTrackColor: NvrColors.of(context).accent,
                  inactiveTrackColor: NvrColors.of(context).border,
                  thumbColor: NvrColors.of(context).accent,
                  overlayColor: NvrColors.of(context).accent.withOpacity(0.12),
                  trackHeight: 2,
                  thumbShape:
                      const RoundSliderThumbShape(enabledThumbRadius: 6),
                ),
                child: Slider(
                  value: _dwellSeconds,
                  min: 3,
                  max: 60,
                  divisions: 57,
                  onChanged: (v) => setState(() => _dwellSeconds = v),
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
              _dwellSeconds.round(),
            );
            if (mounted) Navigator.of(context).pop();
          },
          child: Text(
            widget.isEditing ? 'SAVE' : 'CREATE',
            style: NvrTypography.of(context).monoControl
                .copyWith(color: NvrColors.of(context).accent),
          ),
        ),
      ],
    );
  }
}
