import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/camera.dart';
import '../../models/camera_stream.dart';
import '../../models/schedule_template.dart';
import '../../providers/auth_provider.dart';
import '../../providers/recordings_provider.dart';
import '../../providers/settings_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/analog_slider.dart';
import '../../widgets/hud/hud_button.dart';
import '../../widgets/hud/hud_toggle.dart';
import '../../widgets/hud/status_badge.dart';
import '../../utils/snackbar_helper.dart';
import '../live_view/camera_tile.dart';

class CameraDetailScreen extends ConsumerStatefulWidget {
  final String cameraId;

  const CameraDetailScreen({super.key, required this.cameraId});

  @override
  ConsumerState<CameraDetailScreen> createState() => _CameraDetailScreenState();
}

class _CameraDetailScreenState extends ConsumerState<CameraDetailScreen> {
  Camera? _camera;
  bool _loading = true;
  String? _error;
  bool _showAdvanced = false;

  // ── Recording controls ──────────────────────────────────────────────────
  List<ScheduleTemplate> _templates = [];
  Map<String, String> _streamTemplateMap = {}; // streamID → templateID

  // ── AI controls ─────────────────────────────────────────────────────────
  bool _aiEnabled = false;
  double _confidence = 0.5;
  String _aiStreamId = '';
  double _trackTimeout = 5;
  List<CameraStream> _streams = [];

  // ── Retention ───────────────────────────────────────────────────────────
  double _retentionDays = 30;

  // ── Advanced / ONVIF ────────────────────────────────────────────────────
  late final TextEditingController _nameCtrl;
  late final TextEditingController _rtspCtrl;
  late final TextEditingController _onvifCtrl;
  late final TextEditingController _userCtrl;
  late final TextEditingController _passCtrl;
  late final TextEditingController _subStreamCtrl;
  late final TextEditingController _snapshotCtrl;

  // ── Imaging sliders ──────────────────────────────────────────────────────
  double _brightness = 0.5;
  double _contrast = 0.5;
  double _saturation = 0.5;

  bool _savingGeneral = false;
  bool _savingAi = false;
  bool _savingAdvanced = false;
  bool _refreshing = false;
  bool _loadingProfiles = false;
  List<Map<String, dynamic>> _profiles = [];

  @override
  void initState() {
    super.initState();
    _nameCtrl = TextEditingController();
    _rtspCtrl = TextEditingController();
    _onvifCtrl = TextEditingController();
    _userCtrl = TextEditingController();
    _passCtrl = TextEditingController();
    _subStreamCtrl = TextEditingController();
    _snapshotCtrl = TextEditingController();
    _fetchCamera();
  }

  @override
  void dispose() {
    _nameCtrl.dispose();
    _rtspCtrl.dispose();
    _onvifCtrl.dispose();
    _userCtrl.dispose();
    _passCtrl.dispose();
    _subStreamCtrl.dispose();
    _snapshotCtrl.dispose();
    super.dispose();
  }

  Future<void> _fetchCamera() async {
    if (!mounted) return;
    setState(() {
      _loading = true;
      _error = null;
    });
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      final res = await api.get<dynamic>('/cameras/${widget.cameraId}');
      final camera = Camera.fromJson(res.data as Map<String, dynamic>);
      if (!mounted) return;
      setState(() {
        _camera = camera;
        _loading = false;
        // Populate controllers & local state from camera data
        _nameCtrl.text = camera.name;
        _rtspCtrl.text = camera.rtspUrl;
        _onvifCtrl.text = camera.onvifEndpoint;
        _subStreamCtrl.text = camera.subStreamUrl;
        _snapshotCtrl.text = camera.snapshotUri;
        _aiEnabled = camera.aiEnabled;
        _confidence = camera.aiConfidence.clamp(0.2, 0.9);
        _trackTimeout = camera.aiTrackTimeout.toDouble().clamp(1, 30);
        _retentionDays = camera.retentionDays.toDouble().clamp(7, 90);
        // Don't set _aiStreamId yet — wait for streams to load so the
        // dropdown always has a matching item.
      });
      // Fetch streams, then set the AI stream ID.
      try {
        final streamsRes = await api.get<dynamic>('/cameras/${widget.cameraId}/streams');
        final rawList = streamsRes.data;
        debugPrint('Streams response for ${widget.cameraId}: $rawList');
        final streamsList = (rawList as List)
            .map((e) => CameraStream.fromJson(e as Map<String, dynamic>))
            .toList();
        debugPrint('Parsed ${streamsList.length} streams');
        if (mounted) {
          setState(() {
            _streams = streamsList;
            // Only set the stream ID if it exists in the loaded streams.
            final savedId = camera.aiStreamId;
            _aiStreamId = streamsList.any((s) => s.id == savedId) ? savedId : '';
          });
        }
      } catch (e) {
        debugPrint('Failed to fetch streams: $e');
      }

      // Fetch schedule templates.
      try {
        final tmplRes = await api.get<dynamic>('/schedule-templates');
        final tmplList = (tmplRes.data as List)
            .map((e) => ScheduleTemplate.fromJson(e as Map<String, dynamic>))
            .toList();
        if (mounted) setState(() => _templates = tmplList);
      } catch (e) {
        if (mounted) showErrorSnackBar(context, 'Failed to load schedule templates');
      }

      // Build stream → template assignment map from recording rules.
      try {
        final rulesRes = await api.get<dynamic>('/cameras/${widget.cameraId}/recording-rules');
        final rules = rulesRes.data as List<dynamic>? ?? [];
        final map = <String, String>{};
        for (final r in rules) {
          final rule = r as Map<String, dynamic>;
          final streamId = rule['stream_id'] as String? ?? '';
          final templateId = rule['template_id'] as String? ?? '';
          if (templateId.isNotEmpty) {
            map[streamId] = templateId;
          } else {
            map[streamId] = '__custom__';
          }
        }
        if (mounted) setState(() => _streamTemplateMap = map);
      } catch (e) {
        if (mounted) showErrorSnackBar(context, 'Failed to load recording rules');
      }
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  // ── Save: general (name / rtsp / onvif) ──────────────────────────────────
  Future<void> _saveGeneral() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    setState(() => _savingGeneral = true);
    try {
      await api.put('/cameras/${widget.cameraId}', data: {
        'name': _nameCtrl.text.trim(),
        'rtsp_url': _rtspCtrl.text.trim(),
        'onvif_endpoint': _onvifCtrl.text.trim(),
      });
      _fetchCamera();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(backgroundColor: NvrColors.success, content: Text('Saved')),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Error: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _savingGeneral = false);
    }
  }

  // ── Save: AI settings ────────────────────────────────────────────────────
  Future<void> _saveAi() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    if (mounted) setState(() => _savingAi = true);
    try {
      await api.put('/cameras/${widget.cameraId}/ai', data: {
        'ai_enabled': _aiEnabled,
        'stream_id': _aiStreamId,
        'confidence': _confidence,
        'track_timeout': _trackTimeout.round(),
      });
      _fetchCamera();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(backgroundColor: NvrColors.success, content: Text('AI settings saved')),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Error: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _savingAi = false);
    }
  }

  // ── Save: advanced / retention ───────────────────────────────────────────
  Future<void> _saveAdvanced() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    setState(() => _savingAdvanced = true);
    try {
      await api.put('/cameras/${widget.cameraId}', data: {
        'retention_days': _retentionDays.round(),
      });
      _fetchCamera();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(backgroundColor: NvrColors.success, content: Text('Saved')),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Error: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _savingAdvanced = false);
    }
  }

  // ── ONVIF probe ──────────────────────────────────────────────────────────
  Future<void> _fetchProfiles() async {
    final endpoint = _onvifCtrl.text.trim();
    if (endpoint.isEmpty) return;
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    setState(() {
      _loadingProfiles = true;
      _profiles = [];
    });
    try {
      final res = await api.post<dynamic>('/cameras/probe', data: {
        'endpoint': endpoint,
        'username': _userCtrl.text.trim(),
        'password': _passCtrl.text,
      });
      final data = res.data;
      if (data is Map<String, dynamic>) {
        final raw = data['profiles'];
        if (raw is List) {
          setState(() {
            _profiles = raw.whereType<Map<String, dynamic>>().toList();
          });
        }
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.danger,
            content: Text('Failed to fetch profiles: $e'),
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _loadingProfiles = false);
    }
  }

  Future<void> _refreshCapabilities() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    setState(() => _refreshing = true);
    try {
      await api.post<dynamic>('/cameras/${widget.cameraId}/refresh');
      await _fetchCamera();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(backgroundColor: NvrColors.success, content: Text('Capabilities refreshed')),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Refresh failed: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _refreshing = false);
    }
  }

  Future<void> _assignSchedule(String streamId, String templateId) async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      await api.put('/cameras/${widget.cameraId}/stream-schedule', data: {
        'stream_id': streamId,
        'template_id': templateId,
      });
      if (mounted) {
        setState(() {
          if (templateId.isEmpty) {
            _streamTemplateMap.remove(streamId);
          } else {
            _streamTemplateMap[streamId] = templateId;
          }
        });
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.success,
            content: Text(templateId.isEmpty ? 'Schedule removed' : 'Schedule updated'),
          ),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Error: $e')),
        );
      }
    }
  }

  Future<void> _toggleRole(CameraStream stream, String role, bool currentlyActive) async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;

    final roles = List<String>.from(stream.roleList);
    if (currentlyActive) {
      roles.remove(role);
    } else {
      roles.add(role);
    }

    try {
      await api.put('/streams/${stream.id}/roles', data: {
        'roles': roles.join(','),
      });
      _fetchCamera(); // reload streams
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Error: $e')),
        );
      }
    }
  }

  Widget _buildStreamInfoCard(CameraStream stream) {
    final details = <String>[
      if (stream.resolutionLabel.isNotEmpty) stream.resolutionLabel,
      if (stream.effectiveVideoCodec.isNotEmpty) stream.effectiveVideoCodec.toUpperCase(),
      if (stream.effectiveAudioCodec.isNotEmpty) stream.effectiveAudioCodec.toUpperCase(),
    ];

    return Container(
      padding: const EdgeInsets.all(10),
      decoration: BoxDecoration(
        color: NvrColors.bgTertiary,
        borderRadius: BorderRadius.circular(6),
        border: Border.all(color: NvrColors.border),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          // Stream name + details summary
          Row(
            children: [
              Expanded(
                child: Text(stream.name, style: NvrTypography.cameraName),
              ),
              if (details.isNotEmpty)
                Text(
                  details.join(' · '),
                  style: NvrTypography.monoLabel,
                ),
            ],
          ),

          // Roles as tappable tags
          const SizedBox(height: 6),
          Wrap(
            spacing: 4,
            runSpacing: 4,
            children: ['live_view', 'recording', 'ai_detection', 'mobile'].map((role) {
              final active = stream.roleList.contains(role);
              return GestureDetector(
                onTap: () => _toggleRole(stream, role, active),
                child: Container(
                  padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                  decoration: BoxDecoration(
                    color: active
                        ? NvrColors.accent.withValues(alpha: 0.15)
                        : NvrColors.bgSecondary,
                    borderRadius: BorderRadius.circular(4),
                    border: Border.all(
                      color: active
                          ? NvrColors.accent.withValues(alpha: 0.3)
                          : NvrColors.border,
                    ),
                  ),
                  child: Text(
                    role.replaceAll('_', ' ').toUpperCase(),
                    style: TextStyle(
                      fontFamily: 'JetBrainsMono',
                      fontSize: 9,
                      color: active ? NvrColors.accent : NvrColors.textMuted,
                      letterSpacing: 0.5,
                    ),
                  ),
                ),
              );
            }).toList(),
          ),

          // RTSP URL
          if (stream.rtspUrl.isNotEmpty) ...[
            const SizedBox(height: 6),
            Text(
              stream.rtspUrl,
              style: NvrTypography.monoLabel,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
          ],
        ],
      ),
    );
  }

  Widget _buildScheduleDropdown(String streamId, String label) {
    final currentTemplateId = _streamTemplateMap[streamId] ?? '';
    final validValue = currentTemplateId == '__custom__'
        ? '__custom__'
        : (_templates.any((t) => t.id == currentTemplateId) ? currentTemplateId : '');

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text(label, style: NvrTypography.monoLabel),
        const SizedBox(height: 4),
        DropdownButtonFormField<String>(
          value: validValue,
          dropdownColor: NvrColors.bgTertiary,
          style: NvrTypography.monoData,
          isExpanded: true,
          decoration: InputDecoration(
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
              child: Text('None', style: NvrTypography.monoData),
            ),
            ..._templates.map((t) => DropdownMenuItem(
              value: t.id,
              child: Text('${t.name} (${t.description})', style: NvrTypography.monoData),
            )),
            if (validValue == '__custom__')
              const DropdownMenuItem(
                value: '__custom__',
                child: Text('Custom', style: TextStyle(
                  fontFamily: 'JetBrainsMono',
                  fontSize: 12,
                  fontStyle: FontStyle.italic,
                  color: Color(0xFF737373),
                )),
              ),
          ],
          onChanged: (v) {
            if (v != null && v != '__custom__') {
              _assignSchedule(streamId, v);
            }
          },
        ),
      ],
    );
  }

  StatusBadge _statusBadge(String status) {
    switch (status) {
      case 'online':
      case 'connected':
        return StatusBadge.online();
      case 'degraded':
        return StatusBadge.degraded();
      default:
        return StatusBadge.offline();
    }
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      body: SafeArea(
        child: _loading
            ? const Center(child: CircularProgressIndicator(color: NvrColors.accent))
            : _error != null
                ? _buildError()
                : _buildContent(),
      ),
    );
  }

  Widget _buildError() {
    return Center(
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Text(_error!, style: const TextStyle(color: NvrColors.danger)),
          const SizedBox(height: 12),
          HudButton(label: 'RETRY', onPressed: _fetchCamera),
        ],
      ),
    );
  }

  Widget _buildContent() {
    final camera = _camera!;
    final wide = MediaQuery.of(context).size.width > 800;
    final serverUrl = ref.watch(authProvider).serverUrl ?? '';

    return Column(
      children: [
        // ── Header ────────────────────────────────────────────────────
        _buildHeader(camera),
        const Divider(color: NvrColors.border, height: 1),

        // ── Scrollable body ───────────────────────────────────────────
        Expanded(
          child: SingleChildScrollView(
            padding: const EdgeInsets.all(16),
            child: wide
                ? Row(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Expanded(child: _buildLeftColumn(camera, serverUrl)),
                      const SizedBox(width: 16),
                      Expanded(child: _buildRightColumn(camera)),
                    ],
                  )
                : Column(
                    children: [
                      _buildLeftColumn(camera, serverUrl),
                      const SizedBox(height: 16),
                      _buildRightColumn(camera),
                    ],
                  ),
          ),
        ),
      ],
    );
  }

  // ── Header row ───────────────────────────────────────────────────────────
  Widget _buildHeader(Camera camera) {
    return Padding(
      padding: const EdgeInsets.fromLTRB(8, 8, 12, 8),
      child: Row(
        children: [
          IconButton(
            icon: const Icon(Icons.arrow_back, color: NvrColors.textPrimary, size: 20),
            onPressed: () => Navigator.of(context).pop(),
          ),
          Expanded(
            child: Text(
              camera.name,
              style: NvrTypography.pageTitle,
              maxLines: 1,
              overflow: TextOverflow.ellipsis,
            ),
          ),
          const SizedBox(width: 8),
          _statusBadge(camera.status),
          const Spacer(),
          _refreshing
              ? const SizedBox(
                  width: 20,
                  height: 20,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    color: NvrColors.accent,
                  ),
                )
              : IconButton(
                  icon: const Icon(Icons.refresh, color: NvrColors.textMuted, size: 20),
                  tooltip: 'Refresh capabilities',
                  onPressed: _refreshCapabilities,
                ),
          IconButton(
            icon: Icon(
              Icons.settings,
              color: _showAdvanced ? NvrColors.accent : NvrColors.textMuted,
              size: 20,
            ),
            tooltip: _showAdvanced ? 'Hide advanced' : 'Show advanced',
            onPressed: () => setState(() => _showAdvanced = !_showAdvanced),
          ),
        ],
      ),
    );
  }

  // ── Left/top column: preview + stats ────────────────────────────────────
  Widget _buildLeftColumn(Camera camera, String serverUrl) {
    // Fetch live data for stat tiles
    final systemAsync = ref.watch(systemInfoProvider);
    final storageAsync = ref.watch(storageInfoProvider);
    final today = DateTime.now();
    final dateStr =
        '${today.year}-${today.month.toString().padLeft(2, '0')}-${today.day.toString().padLeft(2, '0')}';
    final eventsAsync = ref.watch(
      motionEventsProvider((cameraId: camera.id, date: dateStr)),
    );

    // Format uptime
    final uptimeStr = systemAsync.whenOrNull(
      data: (info) => info.uptimeFormatted,
    ) ?? '--';

    // Find per-camera storage
    final storageStr = storageAsync.whenOrNull(data: (info) {
      final cam = info.perCamera
          .where((c) => c.cameraId == camera.id)
          .firstOrNull;
      if (cam == null) return '0 GB';
      final gb = cam.totalBytes / (1024 * 1024 * 1024);
      if (gb >= 1) return '${gb.toStringAsFixed(1)} GB';
      final mb = cam.totalBytes / (1024 * 1024);
      return '${mb.toStringAsFixed(0)} MB';
    }) ?? '-- GB';

    // Count today's events
    final eventsStr = eventsAsync.whenOrNull(
      data: (events) => '${events.length}',
    ) ?? '0';

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        // Live preview
        AspectRatio(
          aspectRatio: 16 / 9,
          child: CameraTile(
            camera: camera,
            serverUrl: serverUrl,
            onTap: () {},
          ),
        ),

        const SizedBox(height: 12),

        // Quick stat tiles 2x2
        GridView.count(
          crossAxisCount: 2,
          shrinkWrap: true,
          physics: const NeverScrollableScrollPhysics(),
          crossAxisSpacing: 8,
          mainAxisSpacing: 8,
          childAspectRatio: 2.2,
          children: [
            _StatTile(label: 'UPTIME', value: uptimeStr, valueStyle: NvrTypography.monoDataLarge),
            _StatTile(
              label: 'STORAGE',
              value: storageStr,
              valueStyle: NvrTypography.monoDataLarge.copyWith(color: NvrColors.accent),
            ),
            _StatTile(label: 'EVENTS TODAY', value: eventsStr, valueStyle: NvrTypography.monoDataLarge),
            _StatTile(
              label: 'RETENTION',
              value: '${_retentionDays.round()}d',
              valueStyle: NvrTypography.monoDataLarge,
            ),
          ],
        ),
      ],
    );
  }

  // ── Right/bottom column: controls + info ─────────────────────────────────
  Widget _buildRightColumn(Camera camera) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        // Streams section
        if (_streams.isNotEmpty)
          _SectionCard(
            header: 'STREAMS',
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                for (int i = 0; i < _streams.length; i++) ...[
                  _buildStreamInfoCard(_streams[i]),
                  if (i < _streams.length - 1) const SizedBox(height: 8),
                ],
              ],
            ),
          ),

        if (_streams.isNotEmpty) const SizedBox(height: 12),

        // Recording section
        _SectionCard(
          header: 'RECORDING',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              if (_streams.isEmpty) ...[
                _buildScheduleDropdown('', 'Default'),
              ] else ...[
                for (final stream in _streams) ...[
                  _buildScheduleDropdown(stream.id, stream.displayLabel),
                  if (stream != _streams.last) const SizedBox(height: 8),
                ],
              ],
            ],
          ),
        ),

        const SizedBox(height: 12),

        // AI Detection section
        _SectionCard(
          header: 'AI DETECTION',
          child: Column(
            crossAxisAlignment: CrossAxisAlignment.start,
            children: [
              Row(
                children: [
                  HudToggle(
                    value: _aiEnabled,
                    onChanged: (v) => setState(() => _aiEnabled = v),
                  ),
                  const SizedBox(width: 12),
                  const Text('Enable AI detection', style: NvrTypography.body),
                ],
              ),
              if (_aiEnabled) ...[
                const SizedBox(height: 12),
                AnalogSlider(
                  label: 'CONFIDENCE',
                  value: _confidence,
                  min: 0.2,
                  max: 0.9,
                  onChanged: (v) => setState(() => _confidence = v),
                  valueFormatter: (v) => '${(v * 100).round()}%',
                ),
                const SizedBox(height: 12),
                DropdownButtonFormField<String>(
                  value: _streams.any((s) => s.id == _aiStreamId) || _aiStreamId.isEmpty
                      ? _aiStreamId
                      : '',
                  dropdownColor: NvrColors.bgTertiary,
                  style: NvrTypography.monoData,
                  decoration: InputDecoration(
                    labelText: 'DETECTION STREAM',
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
                  onChanged: (v) => setState(() => _aiStreamId = v ?? ''),
                ),
                const SizedBox(height: 12),
                AnalogSlider(
                  label: 'TRACK TIMEOUT',
                  value: _trackTimeout,
                  min: 1,
                  max: 30,
                  onChanged: (v) => setState(() => _trackTimeout = v),
                  valueFormatter: (v) => '${v.round()}s',
                ),
                const SizedBox(height: 12),
              ],
              SizedBox(
                width: double.infinity,
                child: HudButton(
                  style: HudButtonStyle.tactical,
                  onPressed: _savingAi ? null : _saveAi,
                  label: _savingAi ? 'SAVING...' : 'SAVE AI SETTINGS',
                ),
              ),
            ],
          ),
        ),

        const SizedBox(height: 12),

        // Retention section
        _SectionCard(
          header: 'RETENTION',
          child: AnalogSlider(
            label: 'RETENTION',
            value: _retentionDays,
            min: 7,
            max: 90,
            onChanged: (v) => setState(() => _retentionDays = v),
            valueFormatter: (v) => '${v.round()} DAYS',
          ),
        ),

        const SizedBox(height: 12),

        // Connection info section
        _SectionCard(
          header: 'CONNECTION',
          child: Column(
            children: [
              _KvRow(label: 'Protocol', value: camera.rtspUrl.startsWith('rtsp') ? 'RTSP' : 'HTTP'),
              const SizedBox(height: 6),
              _KvRow(label: 'ONVIF', value: camera.onvifEndpoint.isEmpty ? 'Not configured' : 'Configured'),
              const SizedBox(height: 6),
              _KvRow(label: 'AI Stream', value: camera.subStreamUrl.isEmpty ? 'None' : 'Configured'),
            ],
          ),
        ),

        // ── Advanced sections (behind gear icon) ────────────────────
        if (_showAdvanced) ...[
          const SizedBox(height: 16),
          _buildAdvancedSections(camera),
        ],

        const SizedBox(height: 24),

        // Save button
        HudButton(
          label: _savingGeneral || _savingAdvanced ? 'SAVING...' : 'SAVE CHANGES',
          onPressed: (_savingGeneral || _savingAdvanced)
              ? null
              : () async {
                  await _saveGeneral();
                  if (_showAdvanced) await _saveAdvanced();
                },
        ),

        const SizedBox(height: 16),
      ],
    );
  }

  // ── Advanced expandable sections ─────────────────────────────────────────
  Widget _buildAdvancedSections(Camera camera) {
    return Column(
      children: [
        // ONVIF Configuration
        _ExpandableSection(
          title: 'ONVIF CONFIGURATION',
          children: [
            _NvrField(controller: _onvifCtrl, label: 'ONVIF ENDPOINT'),
            const SizedBox(height: 10),
            _NvrField(controller: _userCtrl, label: 'USERNAME'),
            const SizedBox(height: 10),
            _NvrField(controller: _passCtrl, label: 'PASSWORD', obscure: true),
            const SizedBox(height: 10),
            HudButton(
              label: _loadingProfiles ? 'PROBING...' : 'PROBE DEVICE',
              style: HudButtonStyle.secondary,
              icon: Icons.search,
              onPressed: _loadingProfiles ? null : _fetchProfiles,
            ),
            if (_profiles.isNotEmpty) ...[
              const SizedBox(height: 10),
              const Text('AVAILABLE PROFILES', style: NvrTypography.monoSection),
              const SizedBox(height: 8),
              ..._profiles.map((profile) {
                final name = profile['name'] as String? ?? 'Profile';
                final resolution = profile['resolution'] as String? ?? '';
                final codec = profile['codec'] as String? ?? '';
                final rtspUrl = profile['rtsp_url'] as String? ?? '';
                return Container(
                  margin: const EdgeInsets.only(bottom: 6),
                  padding: const EdgeInsets.all(10),
                  decoration: BoxDecoration(
                    color: NvrColors.bgTertiary,
                    borderRadius: BorderRadius.circular(6),
                    border: Border.all(color: NvrColors.border),
                  ),
                  child: Column(
                    crossAxisAlignment: CrossAxisAlignment.start,
                    children: [
                      Text(name, style: NvrTypography.cameraName),
                      if (resolution.isNotEmpty || codec.isNotEmpty)
                        Padding(
                          padding: const EdgeInsets.only(top: 2),
                          child: Text(
                            [if (resolution.isNotEmpty) resolution, if (codec.isNotEmpty) codec].join(' · '),
                            style: NvrTypography.monoLabel,
                          ),
                        ),
                      if (rtspUrl.isNotEmpty)
                        Row(
                          children: [
                            Expanded(
                              child: Text(
                                rtspUrl,
                                style: NvrTypography.monoLabel,
                                maxLines: 1,
                                overflow: TextOverflow.ellipsis,
                              ),
                            ),
                            TextButton(
                              style: TextButton.styleFrom(
                                foregroundColor: NvrColors.accent,
                                padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
                                minimumSize: Size.zero,
                                tapTargetSize: MaterialTapTargetSize.shrinkWrap,
                              ),
                              onPressed: () {
                                _rtspCtrl.text = rtspUrl;
                                ScaffoldMessenger.of(context).showSnackBar(
                                  SnackBar(
                                    backgroundColor: NvrColors.success,
                                    content: Text('Using profile: $name'),
                                  ),
                                );
                              },
                              child: const Text('USE', style: TextStyle(fontSize: 10)),
                            ),
                          ],
                        ),
                    ],
                  ),
                );
              }),
            ],
          ],
        ),

        const SizedBox(height: 8),

        // Stream Settings
        _ExpandableSection(
          title: 'STREAM SETTINGS',
          children: [
            _NvrField(controller: _nameCtrl, label: 'CAMERA NAME'),
            const SizedBox(height: 10),
            _NvrField(controller: _rtspCtrl, label: 'RTSP URL'),
            const SizedBox(height: 10),
            _NvrField(controller: _subStreamCtrl, label: 'SUB-STREAM URL'),
            const SizedBox(height: 10),
            _NvrField(controller: _snapshotCtrl, label: 'SNAPSHOT URI'),
          ],
        ),

        const SizedBox(height: 8),

        // Imaging
        _ExpandableSection(
          title: 'IMAGING',
          children: [
            AnalogSlider(
              label: 'BRIGHTNESS',
              value: _brightness,
              min: 0.0,
              max: 1.0,
              onChanged: (v) => setState(() => _brightness = v),
              valueFormatter: (v) => '${(v * 100).round()}%',
            ),
            const SizedBox(height: 12),
            AnalogSlider(
              label: 'CONTRAST',
              value: _contrast,
              min: 0.0,
              max: 1.0,
              onChanged: (v) => setState(() => _contrast = v),
              valueFormatter: (v) => '${(v * 100).round()}%',
            ),
            const SizedBox(height: 12),
            AnalogSlider(
              label: 'SATURATION',
              value: _saturation,
              min: 0.0,
              max: 1.0,
              onChanged: (v) => setState(() => _saturation = v),
              valueFormatter: (v) => '${(v * 100).round()}%',
            ),
          ],
        ),

        const SizedBox(height: 8),

        // Stubs
        _ExpandableSection(
          title: 'DETECTION ZONES',
          children: [
            const Text('Detection zone editor coming soon.', style: NvrTypography.body),
          ],
        ),

        const SizedBox(height: 8),

        _ExpandableSection(
          title: 'RECORDING RULES',
          children: [
            const Text('Recording rules coming soon.', style: NvrTypography.body),
          ],
        ),

        const SizedBox(height: 8),

        _ExpandableSection(
          title: 'AUDIO',
          children: [
            const Text('Audio settings coming soon.', style: NvrTypography.body),
          ],
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// Section card with monoSection header
// ---------------------------------------------------------------------------
class _SectionCard extends StatelessWidget {
  final String header;
  final Widget child;

  const _SectionCard({required this.header, required this.child});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: NvrColors.bgSecondary,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: NvrColors.border),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(header, style: NvrTypography.monoSection),
          const SizedBox(height: 10),
          child,
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Expandable advanced section
// ---------------------------------------------------------------------------
class _ExpandableSection extends StatelessWidget {
  final String title;
  final List<Widget> children;

  const _ExpandableSection({required this.title, required this.children});

  @override
  Widget build(BuildContext context) {
    return Container(
      decoration: BoxDecoration(
        color: NvrColors.bgSecondary,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: NvrColors.border),
      ),
      child: Theme(
        data: Theme.of(context).copyWith(
          dividerColor: Colors.transparent,
          splashColor: NvrColors.accent.withOpacity(0.05),
        ),
        child: ExpansionTile(
          tilePadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 2),
          childrenPadding: const EdgeInsets.fromLTRB(12, 0, 12, 12),
          title: Text(title, style: NvrTypography.monoSection),
          iconColor: NvrColors.textMuted,
          collapsedIconColor: NvrColors.textMuted,
          children: children,
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Quick stat tile
// ---------------------------------------------------------------------------
class _StatTile extends StatelessWidget {
  final String label;
  final String value;
  final TextStyle valueStyle;

  const _StatTile({
    required this.label,
    required this.value,
    required this.valueStyle,
  });

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 8),
      decoration: BoxDecoration(
        color: NvrColors.bgSecondary,
        borderRadius: BorderRadius.circular(6),
        border: Border.all(color: NvrColors.border),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisAlignment: MainAxisAlignment.center,
        children: [
          Text(label, style: NvrTypography.monoLabel),
          const SizedBox(height: 4),
          Text(value, style: valueStyle),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Key-value row
// ---------------------------------------------------------------------------
class _KvRow extends StatelessWidget {
  final String label;
  final String value;

  const _KvRow({required this.label, required this.value});

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(label, style: NvrTypography.body),
        Text(value, style: NvrTypography.monoData),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// NVR text field
// ---------------------------------------------------------------------------
class _NvrField extends StatelessWidget {
  final TextEditingController controller;
  final String label;
  final String? hint;
  final TextInputType? keyboardType;
  final String? Function(String?)? validator;
  final bool obscure;

  const _NvrField({
    required this.controller,
    required this.label,
    this.hint,
    this.keyboardType,
    this.validator,
    this.obscure = false,
  });

  @override
  Widget build(BuildContext context) {
    return TextFormField(
      controller: controller,
      keyboardType: keyboardType,
      obscureText: obscure,
      style: const TextStyle(color: NvrColors.textPrimary, fontFamily: 'JetBrainsMono', fontSize: 12),
      validator: validator,
      decoration: InputDecoration(
        labelText: label,
        hintText: hint,
        labelStyle: NvrTypography.monoLabel,
        hintStyle: const TextStyle(color: NvrColors.textMuted),
        filled: true,
        fillColor: NvrColors.bgInput,
        contentPadding: const EdgeInsets.symmetric(horizontal: 10, vertical: 10),
        border: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: NvrColors.border),
        ),
        enabledBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: NvrColors.border),
        ),
        focusedBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: NvrColors.accent),
        ),
        errorBorder: OutlineInputBorder(
          borderRadius: BorderRadius.circular(6),
          borderSide: const BorderSide(color: NvrColors.danger),
        ),
      ),
    );
  }
}
