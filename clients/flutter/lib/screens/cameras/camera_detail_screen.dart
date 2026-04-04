import 'dart:async';

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
import '../../widgets/stream_card.dart';
import '../../utils/snackbar_helper.dart';
import '../../widgets/onvif/device_info_section.dart';
import '../../widgets/onvif/imaging_section.dart';
import '../../widgets/onvif/relay_section.dart';
import '../../widgets/onvif/ptz_enhanced_section.dart';
import '../../widgets/onvif/audio_section.dart';
import '../../widgets/onvif/media_config_section.dart';
import '../../widgets/onvif/device_mgmt_section.dart';
import '../live_view/camera_tile.dart';
import 'zone_editor_screen.dart';

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

  // ── Per-stream settings (held locally until save) ────────────────────
  Map<String, StreamSettingsState> _streamSettings = {};
  Set<String> _expandedStreams = {};
  Map<String, StreamStorageEstimate> _storageEstimates = {};
  Timer? _estimateTimer;

  // ── Advanced / ONVIF ────────────────────────────────────────────────────
  late final TextEditingController _nameCtrl;
  late final TextEditingController _rtspCtrl;
  late final TextEditingController _onvifCtrl;
  late final TextEditingController _userCtrl;
  late final TextEditingController _passCtrl;
  late final TextEditingController _subStreamCtrl;
  late final TextEditingController _snapshotCtrl;

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
    _estimateTimer?.cancel();
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

      // Initialize per-stream settings state.
      final newSettings = <String, StreamSettingsState>{};
      for (final stream in _streams) {
        final tmplId = _streamTemplateMap[stream.id] ?? '';
        newSettings[stream.id] = StreamSettingsState.fromStream(stream, templateId: tmplId);
      }
      if (mounted) {
        setState(() => _streamSettings = newSettings);
      }
      _fetchStorageEstimates();
    } catch (e) {
      if (!mounted) return;
      setState(() {
        _error = e.toString();
        _loading = false;
      });
    }
  }

  // ── Save all settings at once ─────────────────────────────────────────
  Future<void> _saveAll() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;

    setState(() {
      _savingGeneral = true;
      _savingAi = true;
      _savingAdvanced = true;
    });

    try {
      // 1. Save general settings.
      await api.put('/cameras/${widget.cameraId}', data: {
        'name': _nameCtrl.text.trim(),
        'rtsp_url': _rtspCtrl.text.trim(),
        'onvif_endpoint': _onvifCtrl.text.trim(),
      });

      // 2. Save AI settings.
      await api.put('/cameras/${widget.cameraId}/ai', data: {
        'ai_enabled': _aiEnabled,
        'stream_id': _aiStreamId,
        'confidence': _confidence,
        'track_timeout': _trackTimeout.round(),
      });

      // 3. Save per-stream settings.
      for (final entry in _streamSettings.entries) {
        final streamId = entry.key;
        final state = entry.value;

        await api.put('/streams/$streamId/roles', data: {
          'roles': state.roles.join(','),
        });

        final oldTemplateId = _streamTemplateMap[streamId] ?? '';
        if (state.templateId != oldTemplateId) {
          await api.put('/cameras/${widget.cameraId}/stream-schedule', data: {
            'stream_id': streamId,
            'template_id': state.templateId,
          });
        }

        await api.put('/streams/$streamId/retention', data: {
          'retention_days': state.retentionDays.round(),
          'event_retention_days': state.eventRetentionDays.round(),
        });
      }

      // 4. Refresh from server.
      await _fetchCamera();

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
      if (mounted) {
        setState(() {
          _savingGeneral = false;
          _savingAi = false;
          _savingAdvanced = false;
        });
      }
    }
  }

  // ── Fetch storage estimates (debounced) ─────────────────────────────────
  void _fetchStorageEstimates() {
    _estimateTimer?.cancel();
    _estimateTimer = Timer(const Duration(milliseconds: 300), () async {
      final api = ref.read(apiClientProvider);
      if (api == null || _camera == null) return;

      int retDays = 0;
      int eventDays = 0;
      for (final s in _streamSettings.values) {
        if (s.retentionDays > retDays) retDays = s.retentionDays.round();
        if (s.eventRetentionDays > eventDays) eventDays = s.eventRetentionDays.round();
      }

      try {
        final res = await api.get('/cameras/${widget.cameraId}/storage-estimate',
          queryParameters: {
            'retention_days': retDays,
            'event_retention_days': eventDays,
          },
        );
        final data = res.data as Map<String, dynamic>;
        final streams = (data['streams'] as List<dynamic>? ?? []);
        final estimates = <String, StreamStorageEstimate>{};
        for (final s in streams) {
          final est = StreamStorageEstimate.fromJson(s as Map<String, dynamic>);
          estimates[est.streamId] = est;
        }
        if (mounted) setState(() => _storageEstimates = estimates);
      } catch (_) {}
    });
  }

  // ── Retention summary for stat tile ─────────────────────────────────────
  String _retentionSummary() {
    if (_streamSettings.isEmpty) return '--';
    final retentions = _streamSettings.values
        .map((s) => '${s.retentionDays.round()}d/${s.eventRetentionDays.round()}d')
        .toSet();
    if (retentions.length == 1) return retentions.first;
    return 'Mixed';
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
              value: _retentionSummary(),
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
        // ── Stream cards ──
        if (_streams.isNotEmpty) ...[
          Padding(
            padding: const EdgeInsets.only(bottom: 10),
            child: Text('STREAMS', style: NvrTypography.monoSection),
          ),
          for (final stream in _streams)
            Padding(
              padding: const EdgeInsets.only(bottom: 8),
              child: StreamCard(
                stream: stream,
                settings: _streamSettings[stream.id] ?? StreamSettingsState.fromStream(stream, templateId: ''),
                estimate: _storageEstimates[stream.id],
                templates: _templates,
                expanded: _expandedStreams.contains(stream.id),
                onToggleExpand: () {
                  setState(() {
                    if (_expandedStreams.contains(stream.id)) {
                      _expandedStreams.remove(stream.id);
                    } else {
                      _expandedStreams.add(stream.id);
                    }
                  });
                },
                onChanged: (newState) {
                  setState(() => _streamSettings[stream.id] = newState);
                  _fetchStorageEstimates();
                },
              ),
            ),
        ],

        const SizedBox(height: 12),

        // ── AI Detection (camera-level, no dedicated save) ──
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
                    const DropdownMenuItem(value: '', child: Text('Auto', style: NvrTypography.monoData)),
                    ..._streams.map((s) => DropdownMenuItem(
                      value: s.id,
                      child: Text(s.displayLabel, style: NvrTypography.monoData),
                    )),
                  ],
                  onChanged: (v) {
                    if (v != null) setState(() => _aiStreamId = v);
                  },
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
              ],
            ],
          ),
        ),

        const SizedBox(height: 12),

        // ── Device info (ONVIF) ──
        if (_camera?.onvifEndpoint.isNotEmpty == true) ...[
          DeviceInfoSection(cameraId: widget.cameraId),
          const SizedBox(height: 12),
        ],

        // ── Connection info ──
        _SectionCard(
          header: 'CONNECTION',
          child: Column(
            children: [
              _KvRow(label: 'Protocol', value: camera.rtspUrl.startsWith('rtsp') ? 'RTSP' : 'HTTP'),
              const SizedBox(height: 6),
              _KvRow(label: 'ONVIF', value: camera.onvifEndpoint.isEmpty ? 'Not configured' : 'Configured'),
            ],
          ),
        ),

        // ── Advanced sections ──
        if (_showAdvanced) ...[
          const SizedBox(height: 16),
          _buildAdvancedSections(camera),
        ],

        const SizedBox(height: 24),

        // ── Single save button ──
        HudButton(
          label: _savingGeneral || _savingAdvanced || _savingAi ? 'SAVING...' : 'SAVE CHANGES',
          onPressed: (_savingGeneral || _savingAdvanced || _savingAi)
              ? null
              : _saveAll,
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
        _ExpandableSection(
          title: 'MEDIA CONFIGURATION',
          children: [
            MediaConfigSection(cameraId: widget.cameraId),
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
            ImagingSection(cameraId: widget.cameraId),
          ],
        ),

        const SizedBox(height: 8),

        // Stubs
        _ExpandableSection(
          title: 'DETECTION ZONES',
          children: [
            ZoneEditorScreen(cameraId: widget.cameraId),
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
            AudioSection(cameraId: widget.cameraId),
          ],
        ),

        const SizedBox(height: 8),
        if (_camera?.supportsRelay == true)
          _ExpandableSection(
            title: 'RELAY OUTPUTS',
            children: [
              RelaySection(cameraId: widget.cameraId),
            ],
          ),

        const SizedBox(height: 8),
        if (_camera?.ptzCapable == true)
          _ExpandableSection(
            title: 'PTZ CONTROL',
            children: [
              PtzEnhancedSection(cameraId: widget.cameraId),
            ],
          ),

        const SizedBox(height: 8),
        _ExpandableSection(
          title: 'DEVICE MANAGEMENT',
          children: [
            DeviceMgmtSection(cameraId: widget.cameraId),
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
