import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/camera_stream.dart';
import '../../models/recording_rule.dart';
import '../../providers/auth_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';

class RecordingRulesScreen extends ConsumerStatefulWidget {
  final String cameraId;

  const RecordingRulesScreen({super.key, required this.cameraId});

  @override
  ConsumerState<RecordingRulesScreen> createState() => _RecordingRulesScreenState();
}

class _RecordingRulesScreenState extends ConsumerState<RecordingRulesScreen> {
  List<RecordingRule> _rules = [];
  List<CameraStream> _streams = [];
  bool _loading = true;
  String? _error;

  static const _modes = ['continuous', 'motion', 'schedule'];
  static const _dayNames = ['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'];

  @override
  void initState() {
    super.initState();
    _fetchRules();
    _fetchStreams();
  }

  Future<void> _fetchStreams() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      final res = await api.get<dynamic>('/cameras/${widget.cameraId}/streams');
      final list = (res.data as List)
          .map((e) => CameraStream.fromJson(e as Map<String, dynamic>))
          .toList();
      if (mounted) setState(() => _streams = list);
    } catch (_) {}
  }

  Future<void> _fetchRules() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      final res = await api.get<dynamic>('/cameras/${widget.cameraId}/recording-rules');
      final data = res.data as List<dynamic>? ?? [];
      setState(() {
        _rules = data
            .map((e) => RecordingRule.fromJson(e as Map<String, dynamic>))
            .toList();
        _loading = false;
      });
    } catch (e) {
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  Future<void> _toggleRule(RecordingRule rule) async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      await api.put('/recording-rules/${rule.id}', data: {
        ...rule.toJson(),
        'enabled': !rule.enabled,
      });
      await _fetchRules();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Error: $e')),
        );
      }
    }
  }

  Future<void> _deleteRule(RecordingRule rule) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: NvrColors.bgSecondary,
        title: const Text('Delete Rule', style: TextStyle(color: NvrColors.textPrimary)),
        content: Text(
          'Delete the "${rule.mode}" recording rule?',
          style: const TextStyle(color: NvrColors.textSecondary),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: const Text('Cancel', style: TextStyle(color: NvrColors.textSecondary)),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child: const Text('Delete', style: TextStyle(color: NvrColors.danger)),
          ),
        ],
      ),
    );
    if (confirmed != true) return;

    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      await api.delete('/recording-rules/${rule.id}');
      await _fetchRules();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Error: $e')),
        );
      }
    }
  }

  Future<void> _showAddDialog() async {
    String selectedMode = 'motion';
    String selectedStreamId = '';
    TimeOfDay startTime = const TimeOfDay(hour: 0, minute: 0);
    TimeOfDay endTime = const TimeOfDay(hour: 23, minute: 59);
    List<int> selectedDays = List.generate(7, (i) => i + 1);

    await showDialog<void>(
      context: context,
      builder: (ctx) {
        return StatefulBuilder(
          builder: (ctx, setDlgState) {
            return AlertDialog(
              backgroundColor: NvrColors.bgSecondary,
              title: const Text('Add Recording Rule', style: TextStyle(color: NvrColors.textPrimary)),
              content: SingleChildScrollView(
                child: Column(
                  mainAxisSize: MainAxisSize.min,
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    DropdownButtonFormField<String>(
                      initialValue: selectedStreamId,
                      dropdownColor: NvrColors.bgTertiary,
                      style: NvrTypography.monoData,
                      decoration: InputDecoration(
                        labelText: 'STREAM',
                        labelStyle: NvrTypography.monoLabel,
                        filled: true,
                        fillColor: NvrColors.bgTertiary,
                        border: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(4),
                          borderSide: const BorderSide(color: NvrColors.border),
                        ),
                        enabledBorder: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(4),
                          borderSide: const BorderSide(color: NvrColors.border),
                        ),
                        focusedBorder: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(4),
                          borderSide: const BorderSide(color: NvrColors.accent),
                        ),
                        contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
                      ),
                      items: [
                        const DropdownMenuItem(
                          value: '',
                          child: Text('Default', style: NvrTypography.monoData),
                        ),
                        ..._streams.map((s) => DropdownMenuItem(
                          value: s.id,
                          child: Text(s.displayLabel, style: NvrTypography.monoData),
                        )),
                      ],
                      onChanged: (v) => setDlgState(() => selectedStreamId = v ?? ''),
                    ),
                    const SizedBox(height: 12),
                    const Text('Mode', style: TextStyle(color: NvrColors.textSecondary, fontSize: 13)),
                    const SizedBox(height: 6),
                    DropdownButtonFormField<String>(
                      initialValue: selectedMode,
                      dropdownColor: NvrColors.bgTertiary,
                      style: const TextStyle(color: NvrColors.textPrimary),
                      decoration: InputDecoration(
                        filled: true,
                        fillColor: NvrColors.bgTertiary,
                        border: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(8),
                          borderSide: const BorderSide(color: NvrColors.border),
                        ),
                        contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                      ),
                      items: _modes
                          .map((m) => DropdownMenuItem(
                                value: m,
                                child: Text(_modeLabel(m)),
                              ))
                          .toList(),
                      onChanged: (v) {
                        if (v != null) setDlgState(() => selectedMode = v);
                      },
                    ),
                    if (selectedMode == 'schedule') ...[
                      const SizedBox(height: 12),
                      Row(
                        children: [
                          Expanded(
                            child: _TimePickerButton(
                              label: 'Start',
                              time: startTime,
                              onPick: (t) => setDlgState(() => startTime = t),
                            ),
                          ),
                          const SizedBox(width: 8),
                          Expanded(
                            child: _TimePickerButton(
                              label: 'End',
                              time: endTime,
                              onPick: (t) => setDlgState(() => endTime = t),
                            ),
                          ),
                        ],
                      ),
                      const SizedBox(height: 12),
                      const Text('Days', style: TextStyle(color: NvrColors.textSecondary, fontSize: 13)),
                      const SizedBox(height: 6),
                      Wrap(
                        spacing: 6,
                        children: List.generate(7, (i) {
                          final day = i + 1;
                          final selected = selectedDays.contains(day);
                          return FilterChip(
                            label: Text(_dayNames[i]),
                            selected: selected,
                            onSelected: (val) {
                              setDlgState(() {
                                if (val) {
                                  selectedDays.add(day);
                                } else {
                                  selectedDays.remove(day);
                                }
                              });
                            },
                            selectedColor: NvrColors.accent,
                            backgroundColor: NvrColors.bgTertiary,
                            labelStyle: TextStyle(
                              color: selected ? Colors.white : NvrColors.textSecondary,
                              fontSize: 12,
                            ),
                            checkmarkColor: Colors.white,
                          );
                        }),
                      ),
                    ],
                  ],
                ),
              ),
              actions: [
                TextButton(
                  onPressed: () => Navigator.of(ctx).pop(),
                  child: const Text('Cancel', style: TextStyle(color: NvrColors.textSecondary)),
                ),
                ElevatedButton(
                  style: ElevatedButton.styleFrom(backgroundColor: NvrColors.accent),
                  onPressed: () async {
                    Navigator.of(ctx).pop();
                    await _saveNewRule(
                      mode: selectedMode,
                      streamId: selectedStreamId,
                      startTime: selectedMode == 'schedule'
                          ? '${startTime.hour.toString().padLeft(2, '0')}:${startTime.minute.toString().padLeft(2, '0')}'
                          : null,
                      endTime: selectedMode == 'schedule'
                          ? '${endTime.hour.toString().padLeft(2, '0')}:${endTime.minute.toString().padLeft(2, '0')}'
                          : null,
                      daysOfWeek: selectedMode == 'schedule' ? selectedDays : null,
                    );
                  },
                  child: const Text('Save', style: TextStyle(color: Colors.white)),
                ),
              ],
            );
          },
        );
      },
    );
  }

  Future<void> _saveNewRule({
    required String mode,
    String streamId = '',
    String? startTime,
    String? endTime,
    List<int>? daysOfWeek,
  }) async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      await api.post('/cameras/${widget.cameraId}/recording-rules', data: {
        'mode': mode,
        'enabled': true,
        if (streamId.isNotEmpty) 'stream_id': streamId,
        if (startTime != null) 'start_time': startTime,
        if (endTime != null) 'end_time': endTime,
        if (daysOfWeek != null) 'days_of_week': daysOfWeek,
      });
      await _fetchRules();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Error: $e')),
        );
      }
    }
  }

  String _modeLabel(String mode) {
    switch (mode) {
      case 'continuous':
        return 'Continuous';
      case 'motion':
        return 'Motion-triggered';
      case 'schedule':
        return 'Schedule';
      default:
        return mode;
    }
  }

  String _timeRangeLabel(RecordingRule rule) {
    if (rule.mode != 'schedule') return '';
    final start = rule.startTime ?? '--:--';
    final end = rule.endTime ?? '--:--';
    final days = rule.daysOfWeek;
    if (days == null || days.isEmpty) return '$start – $end';
    final dayStr = days.map((d) => _dayNames[(d - 1) % 7]).join(', ');
    return '$start – $end  |  $dayStr';
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return const Center(child: CircularProgressIndicator(color: NvrColors.accent));
    }
    if (_error != null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(_error!, style: const TextStyle(color: NvrColors.danger)),
            TextButton(onPressed: _fetchRules, child: const Text('Retry')),
          ],
        ),
      );
    }

    return Column(
      children: [
        Expanded(
          child: _rules.isEmpty
              ? const Center(
                  child: Text(
                    'No recording rules. Tap + to add one.',
                    style: TextStyle(color: NvrColors.textMuted),
                    textAlign: TextAlign.center,
                  ),
                )
              : ListView.separated(
                  padding: const EdgeInsets.symmetric(vertical: 8),
                  itemCount: _rules.length,
                  separatorBuilder: (_, __) =>
                      const Divider(color: NvrColors.border, height: 1),
                  itemBuilder: (context, index) {
                    final rule = _rules[index];
                    String streamLabel = '';
                    if (rule.streamId.isNotEmpty) {
                      final stream = _streams.where((s) => s.id == rule.streamId).firstOrNull;
                      streamLabel = stream != null ? ' — ${stream.displayLabel}' : ' — Custom stream';
                    }
                    return ListTile(
                      tileColor: NvrColors.bgSecondary,
                      title: Text(
                        '${_modeLabel(rule.mode)}$streamLabel',
                        style: const TextStyle(
                          color: NvrColors.textPrimary,
                          fontWeight: FontWeight.w500,
                        ),
                      ),
                      subtitle: rule.mode == 'schedule'
                          ? Text(
                              _timeRangeLabel(rule),
                              style: const TextStyle(
                                color: NvrColors.textMuted,
                                fontSize: 12,
                              ),
                            )
                          : null,
                      trailing: Row(
                        mainAxisSize: MainAxisSize.min,
                        children: [
                          Switch(
                            value: rule.enabled,
                            onChanged: (_) => _toggleRule(rule),
                            activeThumbColor: NvrColors.accent,
                          ),
                          IconButton(
                            icon: const Icon(Icons.delete_outline, color: NvrColors.danger, size: 20),
                            onPressed: () => _deleteRule(rule),
                          ),
                        ],
                      ),
                    );
                  },
                ),
        ),
        Padding(
          padding: const EdgeInsets.all(16),
          child: SizedBox(
            width: double.infinity,
            child: ElevatedButton.icon(
              style: ElevatedButton.styleFrom(
                backgroundColor: NvrColors.accent,
                foregroundColor: Colors.white,
                padding: const EdgeInsets.symmetric(vertical: 12),
              ),
              onPressed: _showAddDialog,
              icon: const Icon(Icons.add),
              label: const Text('Add Rule'),
            ),
          ),
        ),
      ],
    );
  }
}

class _TimePickerButton extends StatelessWidget {
  final String label;
  final TimeOfDay time;
  final ValueChanged<TimeOfDay> onPick;

  const _TimePickerButton({
    required this.label,
    required this.time,
    required this.onPick,
  });

  @override
  Widget build(BuildContext context) {
    return OutlinedButton(
      style: OutlinedButton.styleFrom(
        side: const BorderSide(color: NvrColors.border),
        foregroundColor: NvrColors.textPrimary,
      ),
      onPressed: () async {
        final picked = await showTimePicker(
          context: context,
          initialTime: time,
          builder: (context, child) => Theme(
            data: ThemeData.dark().copyWith(
              colorScheme: const ColorScheme.dark(
                primary: NvrColors.accent,
                surface: NvrColors.bgSecondary,
              ),
            ),
            child: child!,
          ),
        );
        if (picked != null) onPick(picked);
      },
      child: Text(
        '$label  ${time.hour.toString().padLeft(2, '0')}:${time.minute.toString().padLeft(2, '0')}',
        style: const TextStyle(fontSize: 12),
      ),
    );
  }
}
