import 'package:flutter/material.dart';
import '../models/camera_stream.dart';
import '../models/schedule_template.dart';
import '../theme/nvr_colors.dart';
import '../theme/nvr_typography.dart';
import 'hud/analog_slider.dart';

// ---------------------------------------------------------------------------
// Data classes
// ---------------------------------------------------------------------------

/// Storage estimate returned by the backend for a single stream.
class StreamStorageEstimate {
  final String streamId;
  final int noEventBytes;
  final int eventBytes;
  final double eventFrequency;
  final String freqSource; // 'historical' or 'default'
  final int totalBytes;

  const StreamStorageEstimate({
    required this.streamId,
    required this.noEventBytes,
    required this.eventBytes,
    required this.eventFrequency,
    required this.freqSource,
    required this.totalBytes,
  });

  factory StreamStorageEstimate.fromJson(Map<String, dynamic> json) {
    return StreamStorageEstimate(
      streamId: json['stream_id'] as String? ?? '',
      noEventBytes: json['no_event_bytes'] as int? ?? 0,
      eventBytes: json['event_bytes'] as int? ?? 0,
      eventFrequency: (json['event_frequency'] as num?)?.toDouble() ?? 0.0,
      freqSource: json['freq_source'] as String? ?? 'default',
      totalBytes: json['total_bytes'] as int? ?? 0,
    );
  }
}

/// Mutable local settings state for a stream before saving.
class StreamSettingsState {
  List<String> roles;
  String templateId;
  double retentionDays;
  double eventRetentionDays;

  StreamSettingsState({
    required this.roles,
    required this.templateId,
    required this.retentionDays,
    required this.eventRetentionDays,
  });

  factory StreamSettingsState.fromStream(
    CameraStream stream, {
    required String templateId,
  }) {
    return StreamSettingsState(
      roles: List<String>.from(stream.roleList),
      templateId: templateId,
      retentionDays: stream.retentionDays.toDouble(),
      eventRetentionDays: stream.eventRetentionDays.toDouble(),
    );
  }

  StreamSettingsState copyWith({
    List<String>? roles,
    String? templateId,
    double? retentionDays,
    double? eventRetentionDays,
  }) {
    return StreamSettingsState(
      roles: roles ?? List<String>.from(this.roles),
      templateId: templateId ?? this.templateId,
      retentionDays: retentionDays ?? this.retentionDays,
      eventRetentionDays: eventRetentionDays ?? this.eventRetentionDays,
    );
  }
}

// ---------------------------------------------------------------------------
// StreamCard
// ---------------------------------------------------------------------------

class StreamCard extends StatelessWidget {
  const StreamCard({
    super.key,
    required this.stream,
    required this.settings,
    this.estimate,
    required this.templates,
    required this.expanded,
    required this.onToggleExpand,
    required this.onChanged,
  });

  final CameraStream stream;
  final StreamSettingsState settings;
  final StreamStorageEstimate? estimate;
  final List<ScheduleTemplate> templates;
  final bool expanded;
  final VoidCallback onToggleExpand;
  final ValueChanged<StreamSettingsState> onChanged;

  // -------------------------------------------------------------------------
  // Helpers
  // -------------------------------------------------------------------------

  static String _formatBytes(int bytes) {
    if (bytes <= 0) return '0 B';
    const units = ['B', 'KB', 'MB', 'GB', 'TB'];
    int unitIndex = 0;
    double value = bytes.toDouble();
    while (value >= 1024 && unitIndex < units.length - 1) {
      value /= 1024;
      unitIndex++;
    }
    final formatted =
        value >= 100 ? value.toStringAsFixed(0) : value.toStringAsFixed(1);
    return '$formatted ${units[unitIndex]}';
  }

  String _scheduleName() {
    if (settings.templateId.isEmpty) return 'None';
    final match = templates.where((t) => t.id == settings.templateId);
    if (match.isEmpty) return 'Custom';
    return match.first.name;
  }

  String _retentionSummary() {
    final r = settings.retentionDays.round();
    final e = settings.eventRetentionDays.round();
    final rLabel = r == 0 ? 'OFF' : '${r}d';
    final eLabel = e == 0 ? 'OFF' : '${e}d';
    return '$rLabel/$eLabel';
  }

  String _summaryText() {
    return '${_scheduleName()} \u00B7 ${_retentionSummary()}';
  }

  // -------------------------------------------------------------------------
  // Build
  // -------------------------------------------------------------------------

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: NvrColors.bgSecondary,
        border: Border.all(
          color: expanded ? NvrColors.accent : NvrColors.border,
        ),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          _buildHeader(),
          if (expanded) _buildExpandedContent(),
        ],
      ),
    );
  }

  // ---- Header (collapsed row) --------------------------------------------

  Widget _buildHeader() {
    return GestureDetector(
      onTap: onToggleExpand,
      behavior: HitTestBehavior.opaque,
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
        child: Row(
          children: [
            // Chevron
            Icon(
              expanded ? Icons.expand_more : Icons.chevron_right,
              size: 16,
              color: NvrColors.textSecondary,
            ),
            const SizedBox(width: 8),
            // Stream name + resolution
            Expanded(
              child: Text(
                stream.displayLabel,
                style: NvrTypography.monoData,
                overflow: TextOverflow.ellipsis,
              ),
            ),
            const SizedBox(width: 8),
            // Schedule + retention summary
            Text(
              _summaryText(),
              style: NvrTypography.monoLabel,
            ),
            const SizedBox(width: 8),
            // Total storage estimate
            if (estimate != null)
              Text(
                '~${_formatBytes(estimate!.totalBytes)}',
                style: NvrTypography.monoLabel.copyWith(
                  color: NvrColors.accent,
                ),
              ),
          ],
        ),
      ),
    );
  }

  // ---- Expanded content --------------------------------------------------

  Widget _buildExpandedContent() {
    return Container(
      decoration: const BoxDecoration(
        border: Border(top: BorderSide(color: NvrColors.border)),
      ),
      padding: const EdgeInsets.all(12),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          _buildRolesSection(),
          const SizedBox(height: 16),
          _buildScheduleSection(),
          const SizedBox(height: 16),
          _buildRetentionSection(),
        ],
      ),
    );
  }

  // ---- Roles section -----------------------------------------------------

  Widget _buildRolesSection() {
    const allRoles = ['live_view', 'recording', 'ai_detection', 'mobile'];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('ROLES', style: NvrTypography.monoSection),
        const SizedBox(height: 8),
        Wrap(
          spacing: 6,
          runSpacing: 6,
          children: allRoles.map((role) {
            final active = settings.roles.contains(role);
            return GestureDetector(
              onTap: () {
                final newRoles = List<String>.from(settings.roles);
                if (active) {
                  newRoles.remove(role);
                } else {
                  newRoles.add(role);
                }
                onChanged(settings.copyWith(roles: newRoles));
              },
              child: Container(
                padding: const EdgeInsets.symmetric(
                  horizontal: 7,
                  vertical: 3,
                ),
                decoration: BoxDecoration(
                  color: active
                      ? NvrColors.accent
                      : Colors.transparent,
                  border: Border.all(
                    color: active ? NvrColors.accent : NvrColors.border,
                  ),
                  borderRadius: BorderRadius.circular(3),
                ),
                child: Text(
                  role,
                  style: TextStyle(
                    fontFamily: 'JetBrainsMono',
                    fontSize: 9,
                    fontWeight: FontWeight.w500,
                    color: active
                        ? NvrColors.bgPrimary
                        : NvrColors.textSecondary,
                  ),
                ),
              ),
            );
          }).toList(),
        ),
      ],
    );
  }

  // ---- Recording schedule section ----------------------------------------

  Widget _buildScheduleSection() {
    // Build dropdown items: None + templates + Custom
    final items = <DropdownMenuItem<String>>[
      const DropdownMenuItem(
        value: '',
        child: Text('None'),
      ),
      ...templates.map(
        (t) => DropdownMenuItem(
          value: t.id,
          child: Text(t.name),
        ),
      ),
      const DropdownMenuItem(
        value: '__custom__',
        child: Text('Custom'),
      ),
    ];

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('RECORDING SCHEDULE', style: NvrTypography.monoSection),
        const SizedBox(height: 8),
        DropdownButtonFormField<String>(
          value: items.any((i) => i.value == settings.templateId)
              ? settings.templateId
              : '',
          items: items,
          onChanged: (value) {
            if (value != null) {
              onChanged(settings.copyWith(templateId: value));
            }
          },
          dropdownColor: NvrColors.bgTertiary,
          style: NvrTypography.monoData,
          icon: const Icon(
            Icons.expand_more,
            color: NvrColors.textSecondary,
            size: 16,
          ),
          decoration: InputDecoration(
            filled: true,
            fillColor: NvrColors.bgTertiary,
            contentPadding: const EdgeInsets.symmetric(
              horizontal: 10,
              vertical: 8,
            ),
            enabledBorder: OutlineInputBorder(
              borderSide: const BorderSide(color: NvrColors.border),
              borderRadius: BorderRadius.circular(4),
            ),
            focusedBorder: OutlineInputBorder(
              borderSide: const BorderSide(color: NvrColors.accent),
              borderRadius: BorderRadius.circular(4),
            ),
          ),
        ),
      ],
    );
  }

  // ---- Retention section -------------------------------------------------

  Widget _buildRetentionSection() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('RETENTION', style: NvrTypography.monoSection),
        const SizedBox(height: 12),
        // No-event recordings slider
        AnalogSlider(
          label: 'NO-EVENT RECORDINGS',
          value: settings.retentionDays,
          min: 0,
          max: 90,
          onChanged: (v) {
            onChanged(settings.copyWith(retentionDays: v.roundToDouble()));
          },
          valueFormatter: (v) {
            final days = v.round();
            return days == 0 ? 'OFF' : '${days}d';
          },
        ),
        if (estimate != null) _buildEstimateRow(
          bytes: estimate!.noEventBytes,
          frequency: null,
        ),
        const SizedBox(height: 16),
        // Event recordings slider
        AnalogSlider(
          label: 'EVENT RECORDINGS',
          value: settings.eventRetentionDays,
          min: 0,
          max: 730,
          onChanged: (v) {
            onChanged(
              settings.copyWith(eventRetentionDays: v.roundToDouble()),
            );
          },
          valueFormatter: (v) {
            final days = v.round();
            return days == 0 ? 'OFF' : '${days}d';
          },
        ),
        if (estimate != null) _buildEstimateRow(
          bytes: estimate!.eventBytes,
          frequency: estimate!.eventFrequency,
        ),
      ],
    );
  }

  Widget _buildEstimateRow({
    required int bytes,
    required double? frequency,
  }) {
    return Padding(
      padding: const EdgeInsets.only(top: 6),
      child: Row(
        children: [
          Text(
            '~${_formatBytes(bytes)}',
            style: const TextStyle(
              fontFamily: 'JetBrainsMono',
              fontSize: 9,
              color: NvrColors.accent,
            ),
          ),
          if (frequency != null) ...[
            Text(
              ' \u00B7 ${frequency.toStringAsFixed(1)} events/day',
              style: const TextStyle(
                fontFamily: 'JetBrainsMono',
                fontSize: 9,
                color: NvrColors.textMuted,
              ),
            ),
          ],
        ],
      ),
    );
  }
}
