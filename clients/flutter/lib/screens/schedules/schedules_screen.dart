import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/schedule_template.dart';
import '../../providers/auth_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/hud_button.dart';

class SchedulesScreen extends ConsumerStatefulWidget {
  const SchedulesScreen({super.key});

  @override
  ConsumerState<SchedulesScreen> createState() => _SchedulesScreenState();
}

class _SchedulesScreenState extends ConsumerState<SchedulesScreen> {
  List<ScheduleTemplate> _templates = [];
  bool _loading = true;

  static const _dayNames = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

  @override
  void initState() {
    super.initState();
    _fetchTemplates();
  }

  Future<void> _fetchTemplates() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      final res = await api.get<dynamic>('/schedule-templates');
      final data = res.data as List<dynamic>? ?? [];
      if (mounted) {
        setState(() {
          _templates = data
              .map((e) => ScheduleTemplate.fromJson(e as Map<String, dynamic>))
              .toList();
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) setState(() => _loading = false);
    }
  }

  InputDecoration _hudInputDecoration(String label) {
    return InputDecoration(
      labelText: label,
      labelStyle: NvrTypography.of(context).monoLabel,
      filled: true,
      fillColor: NvrColors.of(context).bgTertiary,
      border: OutlineInputBorder(
        borderRadius: BorderRadius.circular(4),
        borderSide: BorderSide(color: NvrColors.of(context).border),
      ),
      enabledBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(4),
        borderSide: BorderSide(color: NvrColors.of(context).border),
      ),
      focusedBorder: OutlineInputBorder(
        borderRadius: BorderRadius.circular(4),
        borderSide: BorderSide(color: NvrColors.of(context).accent),
      ),
      contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
    );
  }

  Future<void> _showEditDialog({ScheduleTemplate? template}) async {
    final isEdit = template != null;
    final nameController = TextEditingController(text: template?.name ?? '');
    String selectedMode = template?.mode ?? 'always';
    List<int> selectedDays =
        template != null ? List<int>.from(template.days) : List.generate(7, (i) => i);
    TimeOfDay startTime = _parseTime(template?.startTime ?? '00:00');
    TimeOfDay endTime = _parseTime(template?.endTime ?? '00:00');
    final postEventController = TextEditingController(
      text: (template?.postEventSeconds ?? 30).toString(),
    );

    await showDialog<void>(
      context: context,
      builder: (ctx) {
        return StatefulBuilder(
          builder: (ctx, setDlgState) {
            return AlertDialog(
              backgroundColor: NvrColors.of(context).bgSecondary,
              title: Text(
                isEdit ? 'EDIT TEMPLATE' : 'NEW TEMPLATE',
                style: TextStyle(
                    color: NvrColors.of(context).textPrimary,
                    fontSize: 14,
                    fontWeight: FontWeight.w600),
              ),
              content: SizedBox(
                width: 360,
                child: SingleChildScrollView(
                  child: Column(
                    mainAxisSize: MainAxisSize.min,
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      TextFormField(
                        controller: nameController,
                        style: NvrTypography.of(context).monoData,
                        decoration: _hudInputDecoration('NAME'),
                      ),
                      const SizedBox(height: 12),
                      DropdownButtonFormField<String>(
                        initialValue: selectedMode,
                        dropdownColor: NvrColors.of(context).bgTertiary,
                        style: NvrTypography.of(context).monoData,
                        decoration: _hudInputDecoration('MODE'),
                        items: [
                          DropdownMenuItem(
                            value: 'always',
                            child: Text('Continuous', style: NvrTypography.of(context).monoData),
                          ),
                          DropdownMenuItem(
                            value: 'events',
                            child: Text('Motion', style: NvrTypography.of(context).monoData),
                          ),
                        ],
                        onChanged: (v) {
                          if (v != null) setDlgState(() => selectedMode = v);
                        },
                      ),
                      const SizedBox(height: 12),
                      Text(
                        'DAYS',
                        style: NvrTypography.of(context).monoLabel
                            .copyWith(color: NvrColors.of(context).textSecondary),
                      ),
                      const SizedBox(height: 6),
                      Wrap(
                        spacing: 6,
                        runSpacing: 6,
                        children: List.generate(7, (i) {
                          final selected = selectedDays.contains(i);
                          return GestureDetector(
                            onTap: () {
                              setDlgState(() {
                                if (selected) {
                                  selectedDays.remove(i);
                                } else {
                                  selectedDays.add(i);
                                }
                              });
                            },
                            child: Container(
                              padding: const EdgeInsets.symmetric(
                                  horizontal: 10, vertical: 6),
                              decoration: BoxDecoration(
                                color: selected
                                    ? NvrColors.of(context).accent
                                    : NvrColors.of(context).bgTertiary,
                                borderRadius: BorderRadius.circular(4),
                                border: Border.all(
                                  color: selected
                                      ? NvrColors.of(context).accent
                                      : NvrColors.of(context).border,
                                ),
                              ),
                              child: Text(
                                _dayNames[i],
                                style: TextStyle(
                                  fontFamily: 'JetBrainsMono',
                                  fontSize: 10,
                                  fontWeight: FontWeight.w500,
                                  color: selected
                                      ? NvrColors.of(context).bgPrimary
                                      : NvrColors.of(context).textSecondary,
                                ),
                              ),
                            ),
                          );
                        }),
                      ),
                      const SizedBox(height: 12),
                      Row(
                        children: [
                          Expanded(
                            child: _TimePickerRow(
                              label: 'START',
                              time: startTime,
                              onPick: (t) => setDlgState(() => startTime = t),
                            ),
                          ),
                          const SizedBox(width: 8),
                          Expanded(
                            child: _TimePickerRow(
                              label: 'END',
                              time: endTime,
                              onPick: (t) => setDlgState(() => endTime = t),
                            ),
                          ),
                        ],
                      ),
                      if (selectedMode == 'events') ...[
                        const SizedBox(height: 12),
                        TextFormField(
                          controller: postEventController,
                          style: NvrTypography.of(context).monoData,
                          keyboardType: TextInputType.number,
                          decoration:
                              _hudInputDecoration('POST-EVENT BUFFER (SECONDS)'),
                        ),
                      ],
                    ],
                  ),
                ),
              ),
              actions: [
                TextButton(
                  onPressed: () => Navigator.of(ctx).pop(),
                  child: Text('Cancel',
                      style: TextStyle(color: NvrColors.of(context).textSecondary)),
                ),
                ElevatedButton(
                  style: ElevatedButton.styleFrom(
                      backgroundColor: NvrColors.of(context).accent),
                  onPressed: () async {
                    final name = nameController.text.trim();
                    if (name.isEmpty) return;
                    Navigator.of(ctx).pop();
                    await _saveTemplate(
                      id: template?.id,
                      name: name,
                      mode: selectedMode,
                      days: selectedDays,
                      startTime: _formatTime(startTime),
                      endTime: _formatTime(endTime),
                      postEventSeconds:
                          int.tryParse(postEventController.text) ?? 30,
                    );
                  },
                  child: const Text('Save',
                      style: TextStyle(color: Colors.white)),
                ),
              ],
            );
          },
        );
      },
    );
  }

  Future<void> _saveTemplate({
    String? id,
    required String name,
    required String mode,
    required List<int> days,
    required String startTime,
    required String endTime,
    required int postEventSeconds,
  }) async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      final body = {
        'name': name,
        'mode': mode,
        'days': days,
        'start_time': startTime,
        'end_time': endTime,
        'post_event_seconds': postEventSeconds,
      };
      if (id == null) {
        await api.post('/schedule-templates', data: body);
      } else {
        await api.put('/schedule-templates/$id', data: body);
      }
      await _fetchTemplates();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
              backgroundColor: NvrColors.of(context).danger, content: Text('Error: $e')),
        );
      }
    }
  }

  Future<void> _deleteTemplate(ScheduleTemplate template) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: NvrColors.of(context).bgSecondary,
        title: Text('Delete Template',
            style: TextStyle(color: NvrColors.of(context).textPrimary)),
        content: Text(
          'Delete template "${template.name}"?',
          style: TextStyle(color: NvrColors.of(context).textSecondary),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: Text('Cancel',
                style: TextStyle(color: NvrColors.of(context).textSecondary)),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child:
                Text('Delete', style: TextStyle(color: NvrColors.of(context).danger)),
          ),
        ],
      ),
    );
    if (confirmed != true) return;

    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      await api.delete('/schedule-templates/${template.id}');
      await _fetchTemplates();
    } catch (e) {
      if (mounted) {
        final msg = e.toString();
        final isConflict = msg.contains('409') || msg.contains('conflict');
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.of(context).danger,
            content: Text(
              isConflict
                  ? 'Template is assigned to streams, remove assignments first'
                  : 'Error: $msg',
            ),
          ),
        );
      }
    }
  }

  TimeOfDay _parseTime(String t) {
    final parts = t.split(':');
    if (parts.length < 2) return const TimeOfDay(hour: 0, minute: 0);
    return TimeOfDay(
      hour: int.tryParse(parts[0]) ?? 0,
      minute: int.tryParse(parts[1]) ?? 0,
    );
  }

  String _formatTime(TimeOfDay t) {
    return '${t.hour.toString().padLeft(2, '0')}:${t.minute.toString().padLeft(2, '0')}';
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: NvrColors.of(context).bgPrimary,
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // Header
            Row(
              mainAxisAlignment: MainAxisAlignment.spaceBetween,
              children: [
                Text('RECORDING SCHEDULES', style: NvrTypography.of(context).pageTitle),
                HudButton(
                  label: '+ NEW TEMPLATE',
                  style: HudButtonStyle.tactical,
                  onPressed: () => _showEditDialog(),
                ),
              ],
            ),
            const SizedBox(height: 16),
            // Body
            Expanded(
              child: _loading
                  ? Center(
                      child: CircularProgressIndicator(color: NvrColors.of(context).accent))
                  : _templates.isEmpty
                      ? Center(
                          child: Text(
                            'No schedule templates. Tap + NEW TEMPLATE to create one.',
                            style: TextStyle(color: NvrColors.of(context).textMuted),
                            textAlign: TextAlign.center,
                          ),
                        )
                      : ListView.separated(
                          itemCount: _templates.length,
                          separatorBuilder: (_, __) =>
                              const SizedBox(height: 12),
                          itemBuilder: (context, index) {
                            final template = _templates[index];
                            return _TemplateRow(
                              template: template,
                              onTap: () => _showEditDialog(template: template),
                              onDelete: template.isDefault
                                  ? null
                                  : () => _deleteTemplate(template),
                            );
                          },
                        ),
            ),
          ],
        ),
      ),
    );
  }
}

class _TemplateRow extends StatelessWidget {
  final ScheduleTemplate template;
  final VoidCallback onTap;
  final VoidCallback? onDelete;

  const _TemplateRow({
    required this.template,
    required this.onTap,
    this.onDelete,
  });

  @override
  Widget build(BuildContext context) {
    final dotColor = template.mode == 'always'
        ? NvrColors.of(context).accent
        : const Color(0xFF22c55e);

    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding: const EdgeInsets.all(12),
        decoration: BoxDecoration(
          color: NvrColors.of(context).bgSecondary,
          borderRadius: BorderRadius.circular(8),
          border: Border.all(color: NvrColors.of(context).border, width: 1),
        ),
        child: Row(
          children: [
            Container(
              width: 8,
              height: 8,
              decoration: BoxDecoration(
                color: dotColor,
                shape: BoxShape.circle,
              ),
            ),
            const SizedBox(width: 12),
            Expanded(
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(template.name, style: NvrTypography.of(context).cameraName),
                  const SizedBox(height: 2),
                  Text(template.description, style: NvrTypography.of(context).monoLabel),
                ],
              ),
            ),
            if (onDelete != null)
              GestureDetector(
                onTap: onDelete,
                child: Padding(
                  padding: EdgeInsets.only(right: 4),
                  child: Icon(Icons.delete_outline,
                      color: NvrColors.of(context).danger, size: 18),
                ),
              ),
            Icon(Icons.chevron_right,
                color: NvrColors.of(context).textMuted, size: 16),
          ],
        ),
      ),
    );
  }
}

class _TimePickerRow extends StatelessWidget {
  final String label;
  final TimeOfDay time;
  final ValueChanged<TimeOfDay> onPick;

  const _TimePickerRow({
    required this.label,
    required this.time,
    required this.onPick,
  });

  @override
  Widget build(BuildContext context) {
    final display =
        '${time.hour.toString().padLeft(2, '0')}:${time.minute.toString().padLeft(2, '0')}';

    return GestureDetector(
      onTap: () async {
        final picked = await showTimePicker(
          context: context,
          initialTime: time,
          builder: (context, child) => Theme(
            data: ThemeData.dark().copyWith(
              colorScheme: ColorScheme.dark(
                primary: NvrColors.of(context).accent,
                surface: NvrColors.of(context).bgSecondary,
              ),
            ),
            child: child!,
          ),
        );
        if (picked != null) onPick(picked);
      },
      child: Container(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        decoration: BoxDecoration(
          color: NvrColors.of(context).bgTertiary,
          borderRadius: BorderRadius.circular(4),
          border: Border.all(color: NvrColors.of(context).border),
        ),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.spaceBetween,
          children: [
            Text(label, style: NvrTypography.of(context).monoLabel),
            Text(display, style: NvrTypography.of(context).monoData),
          ],
        ),
      ),
    );
  }
}
