import 'dart:async';
import 'dart:ui';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';

import '../../models/camera.dart';
import '../../providers/auth_provider.dart';
import '../../services/whep_service.dart';
import '../../theme/nvr_animations.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/corner_brackets.dart';
import '../../widgets/hud/hud_toggle.dart';
import '../../widgets/hud/status_badge.dart';
import 'analytics_overlay.dart';
import 'ptz_controls.dart';

class FullscreenView extends ConsumerStatefulWidget {
  final Camera camera;

  const FullscreenView({super.key, required this.camera});

  @override
  ConsumerState<FullscreenView> createState() => _FullscreenViewState();
}

class _FullscreenViewState extends ConsumerState<FullscreenView> {
  WhepConnection? _connection;
  WhepConnectionState _connState = WhepConnectionState.connecting;
  StreamSubscription<WhepConnectionState>? _stateSub;
  bool _controlsVisible = true;
  Timer? _hideControlsTimer;
  bool _aiEnabled = false;
  bool _muted = true;
  bool _capturing = false;

  @override
  void initState() {
    super.initState();
    _aiEnabled = widget.camera.aiEnabled;
    SystemChrome.setEnabledSystemUIMode(SystemUiMode.immersiveSticky);
    _initConnection();
    _scheduleHideControls();
  }

  void _initConnection() {
    final auth = ref.read(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final camera = widget.camera;

    final livePath = camera.liveViewPath.isNotEmpty
        ? camera.liveViewPath
        : camera.mediamtxPath;
    _connection = WhepConnection(
      serverUrl: serverUrl,
      mediamtxPath: livePath,
    );

    _stateSub = _connection!.stateStream.listen((state) {
      if (mounted) {
        setState(() => _connState = state);
        if (state == WhepConnectionState.connected) {
          _connection?.setAudioEnabled(!_muted);
        }
      }
    });

    _connection!.connect();
  }

  void _toggleControls() {
    setState(() => _controlsVisible = !_controlsVisible);
    if (_controlsVisible) {
      _scheduleHideControls();
    } else {
      _hideControlsTimer?.cancel();
    }
  }

  void _scheduleHideControls() {
    _hideControlsTimer?.cancel();
    _hideControlsTimer = Timer(NvrAnimations.overlayHideDelay, () {
      if (mounted) setState(() => _controlsVisible = false);
    });
  }

  Future<void> _takeScreenshot() async {
    if (_capturing || !mounted) return;
    setState(() => _capturing = true);
    final api = ref.read(apiClientProvider);
    if (api == null) {
      if (mounted) setState(() => _capturing = false);
      return;
    }
    try {
      await api.post<dynamic>('/cameras/${widget.camera.id}/screenshot');
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          const SnackBar(backgroundColor: NvrColors.success, content: Text('Screenshot saved')),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.danger, content: Text('Screenshot failed: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _capturing = false);
    }
  }

  @override
  void dispose() {
    _hideControlsTimer?.cancel();
    _stateSub?.cancel();
    _connection?.dispose();
    SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final camera = widget.camera;
    final apiClient = ref.watch(apiClientProvider);

    return Scaffold(
      backgroundColor: Colors.black,
      body: GestureDetector(
        behavior: HitTestBehavior.opaque,
        onTap: _toggleControls,
        child: CornerBrackets(
          bracketSize: 24,
          child: Stack(
            fit: StackFit.expand,
            children: [
              // ── Video layer ───────────────────────────────────────────────
              _buildVideoLayer(),

              // ── Analytics overlay ─────────────────────────────────────────
              if (_aiEnabled)
                AnalyticsOverlay(
                  cameraName: camera.name,
                  cameraId: camera.id,
                ),

              // ── Top HUD overlay ───────────────────────────────────────────
              AnimatedOpacity(
                opacity: _controlsVisible ? 1.0 : 0.0,
                duration: NvrAnimations.overlayFadeDuration,
                child: _buildTopOverlay(camera),
              ),

              // ── Bottom HUD overlay ────────────────────────────────────────
              AnimatedOpacity(
                opacity: _controlsVisible ? 1.0 : 0.0,
                duration: NvrAnimations.overlayFadeDuration,
                child: _buildBottomOverlay(context, camera),
              ),

              // ── PTZ controls overlay ──────────────────────────────────────
              if (camera.ptzCapable && apiClient != null)
                AnimatedOpacity(
                  opacity: _controlsVisible ? 1.0 : 0.0,
                  duration: NvrAnimations.overlayFadeDuration,
                  child: Align(
                    alignment: Alignment.centerRight,
                    child: Padding(
                      padding: const EdgeInsets.only(right: 16),
                      child: PtzControls(
                        apiClient: apiClient,
                        cameraId: camera.id,
                      ),
                    ),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildTopOverlay(Camera camera) {
    return Align(
      alignment: Alignment.topCenter,
      child: Container(
        decoration: const BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.topCenter,
            end: Alignment.bottomCenter,
            colors: [Colors.black, Colors.transparent],
          ),
        ),
        padding: const EdgeInsets.fromLTRB(16, 40, 16, 24),
        child: Row(
          crossAxisAlignment: CrossAxisAlignment.center,
          children: [
            // Live status badge
            StatusBadge.live(),
            const SizedBox(width: 10),

            // Camera name
            Text(
              camera.name,
              style: const TextStyle(
                fontFamily: 'IBMPlexSans',
                fontSize: 15,
                fontWeight: FontWeight.w600,
                color: NvrColors.textPrimary,
              ),
            ),
            const SizedBox(width: 10),

            // Camera ID
            Text(
              camera.id,
              style: NvrTypography.monoControl,
            ),

            const Spacer(),

            // REC indicator
            StatusBadge.recording(),
            const SizedBox(width: 10),

            // Timestamp
            _LiveTimestamp(),
          ],
        ),
      ),
    );
  }

  Widget _buildBottomOverlay(BuildContext context, Camera camera) {
    return Align(
      alignment: Alignment.bottomCenter,
      child: Container(
        decoration: const BoxDecoration(
          gradient: LinearGradient(
            begin: Alignment.bottomCenter,
            end: Alignment.topCenter,
            colors: [Colors.black, Colors.transparent],
          ),
        ),
        padding: const EdgeInsets.fromLTRB(16, 24, 16, 32),
        child: Row(
          mainAxisAlignment: MainAxisAlignment.center,
          children: [
            // Audio pill
            _PillButton(
              icon: _muted ? Icons.volume_off : Icons.volume_up,
              label: _muted ? 'Muted' : 'Audio',
              onTap: () {
                setState(() => _muted = !_muted);
                _connection?.setAudioEnabled(!_muted);
              },
            ),
            const SizedBox(width: 8),

            // AI toggle pill
            _AiTogglePill(
              enabled: _aiEnabled,
              onChanged: (v) => setState(() => _aiEnabled = v),
            ),
            const SizedBox(width: 8),

            // Snapshot pill
            _PillButton(
              icon: _capturing ? Icons.hourglass_empty : Icons.photo_camera,
              label: _capturing ? 'Saving...' : 'Snapshot',
              onTap: _takeScreenshot,
            ),
            const SizedBox(width: 8),

            // Grid return pill
            _PillButton(
              icon: Icons.grid_view,
              label: 'Grid',
              onTap: () => Navigator.of(context).pop(),
            ),
            const SizedBox(width: 8),

            // Exit fullscreen pill
            _PillButton(
              icon: Icons.fullscreen_exit,
              label: 'Exit',
              onTap: () => Navigator.of(context).pop(),
            ),
          ],
        ),
      ),
    );
  }

  Widget _buildVideoLayer() {
    switch (_connState) {
      case WhepConnectionState.connecting:
        return const Center(
          child: CircularProgressIndicator(color: NvrColors.accent),
        );

      case WhepConnectionState.connected:
        final renderer = _connection?.renderer;
        if (renderer == null) {
          return const Center(
            child: CircularProgressIndicator(color: NvrColors.accent),
          );
        }
        return RTCVideoView(
          renderer,
          objectFit: RTCVideoViewObjectFit.RTCVideoViewObjectFitContain,
        );

      case WhepConnectionState.failed:
        return Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              const Icon(Icons.error_outline, color: NvrColors.danger, size: 48),
              const SizedBox(height: 16),
              const Text(
                'Connection failed',
                style: TextStyle(color: NvrColors.textSecondary),
              ),
              const SizedBox(height: 16),
              ElevatedButton.icon(
                icon: const Icon(Icons.refresh),
                label: const Text('Retry'),
                onPressed: () => _connection?.retry(),
              ),
            ],
          ),
        );

      case WhepConnectionState.disposed:
        return const SizedBox.shrink();
    }
  }
}

// ── Supporting widgets ─────────────────────────────────────────────────────────

class _PillButton extends StatelessWidget {
  final IconData icon;
  final String label;
  final VoidCallback onTap;

  const _PillButton({
    required this.icon,
    required this.label,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: ClipRRect(
        borderRadius: BorderRadius.circular(20),
        child: BackdropFilter(
          filter: ImageFilter.blur(sigmaX: 6, sigmaY: 6),
          child: Container(
            padding: const EdgeInsets.symmetric(horizontal: 12, vertical: 7),
            decoration: BoxDecoration(
              color: NvrColors.bgSecondary.withValues(alpha: 0.75),
              borderRadius: BorderRadius.circular(20),
              border: Border.all(
                color: NvrColors.border,
                width: 1,
              ),
            ),
            child: Row(
              mainAxisSize: MainAxisSize.min,
              children: [
                Icon(icon, size: 14, color: NvrColors.textPrimary),
                const SizedBox(width: 6),
                Text(label, style: NvrTypography.button.copyWith(fontSize: 11)),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _AiTogglePill extends StatelessWidget {
  final bool enabled;
  final ValueChanged<bool> onChanged;

  const _AiTogglePill({required this.enabled, required this.onChanged});

  @override
  Widget build(BuildContext context) {
    return ClipRRect(
      borderRadius: BorderRadius.circular(20),
      child: BackdropFilter(
        filter: ImageFilter.blur(sigmaX: 6, sigmaY: 6),
        child: Container(
          padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
          decoration: BoxDecoration(
            color: NvrColors.bgSecondary.withValues(alpha: 0.75),
            borderRadius: BorderRadius.circular(20),
            border: Border.all(
              color: enabled ? NvrColors.accent.withValues(alpha: 0.4) : NvrColors.border,
              width: 1,
            ),
          ),
          child: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(
                'AI',
                style: NvrTypography.monoSection.copyWith(fontSize: 10),
              ),
              const SizedBox(width: 8),
              HudToggle(
                value: enabled,
                onChanged: onChanged,
                showStateLabel: false,
              ),
            ],
          ),
        ),
      ),
    );
  }
}

class _LiveTimestamp extends StatefulWidget {
  @override
  State<_LiveTimestamp> createState() => _LiveTimestampState();
}

class _LiveTimestampState extends State<_LiveTimestamp> {
  late Timer _timer;
  late DateTime _now;

  @override
  void initState() {
    super.initState();
    _now = DateTime.now();
    _timer = Timer.periodic(const Duration(seconds: 1), (_) {
      if (mounted) setState(() => _now = DateTime.now());
    });
  }

  @override
  void dispose() {
    _timer.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final h = _now.hour.toString().padLeft(2, '0');
    final m = _now.minute.toString().padLeft(2, '0');
    final s = _now.second.toString().padLeft(2, '0');
    return Text('$h:$m:$s', style: NvrTypography.monoTimestamp);
  }
}
