import 'dart:async';
import 'dart:ui';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';
import 'package:media_kit_video/media_kit_video.dart';

import '../../models/camera.dart';
import '../../providers/auth_provider.dart';
import '../../services/rtsp_player_service.dart';
import '../../services/whep_service.dart';
import '../../theme/nvr_animations.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../widgets/hud/corner_brackets.dart';
import '../../widgets/hud/hud_toggle.dart';
import '../../widgets/hud/status_badge.dart';
import '../../providers/overlay_settings_provider.dart';
import 'analytics_overlay.dart';
import 'ptz_controls.dart';

class FullscreenView extends ConsumerStatefulWidget {
  final Camera camera;

  const FullscreenView({super.key, required this.camera});

  @override
  ConsumerState<FullscreenView> createState() => _FullscreenViewState();
}

class _FullscreenViewState extends ConsumerState<FullscreenView> {
  WhepConnection? _whepConnection;
  StreamSubscription<WhepConnectionState>? _whepStateSub;
  RtspConnection? _rtspConnection;
  StreamSubscription<RtspConnectionState>? _rtspStateSub;

  bool _isConnecting = true;
  bool _isConnected = false;
  bool _isFailed = false;
  bool _useRtsp = false;

  bool _controlsVisible = true;
  Timer? _hideControlsTimer;
  bool _muted = true;
  bool _capturing = false;

  final FocusNode _keyFocusNode = FocusNode();

  @override
  void initState() {
    super.initState();
    SystemChrome.setEnabledSystemUIMode(SystemUiMode.immersiveSticky);
    _initConnection();
    _scheduleHideControls();
  }

  // ── Keyboard PTZ control ────────────────────────────────────────────
  void _onKeyEvent(KeyEvent event) {
    if (event is! KeyDownEvent && event is! KeyRepeatEvent) return;

    final apiClient = ref.read(apiClientProvider);
    if (apiClient == null || !widget.camera.ptzCapable) return;

    final String action;
    switch (event.logicalKey) {
      case LogicalKeyboardKey.arrowUp:
        action = 'up';
      case LogicalKeyboardKey.arrowDown:
        action = 'down';
      case LogicalKeyboardKey.arrowLeft:
        action = 'left';
      case LogicalKeyboardKey.arrowRight:
        action = 'right';
      case LogicalKeyboardKey.home:
        action = 'stop';
      case LogicalKeyboardKey.equal: // + or =
      case LogicalKeyboardKey.add:
        action = 'zoom_in';
      case LogicalKeyboardKey.minus:
      case LogicalKeyboardKey.numpadSubtract:
        action = 'zoom_out';
      case LogicalKeyboardKey.escape:
        Navigator.of(context).pop();
        return;
      default:
        return;
    }

    apiClient.post(
      '/cameras/${widget.camera.id}/ptz',
      data: {'action': action},
    );
  }

  bool get _isH265 {
    final codec = widget.camera.liveViewCodec.toUpperCase();
    return codec == 'H265' || codec == 'HEVC';
  }

  void _setConnState({bool connecting = false, bool connected = false, bool failed = false}) {
    if (mounted) {
      setState(() {
        _isConnecting = connecting;
        _isConnected = connected;
        _isFailed = failed;
      });
    }
  }

  void _initConnection() {
    final auth = ref.read(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final camera = widget.camera;

    final livePath = camera.liveViewPath.isNotEmpty
        ? camera.liveViewPath
        : camera.mediamtxPath;

    _useRtsp = _isH265;

    if (_useRtsp) {
      _rtspConnection = RtspConnection(
        serverUrl: serverUrl,
        mediamtxPath: livePath,
      );
      _rtspStateSub = _rtspConnection!.stateStream.listen((state) {
        if (!mounted) return;
        switch (state) {
          case RtspConnectionState.connecting:
            _setConnState(connecting: true);
          case RtspConnectionState.connected:
            _setConnState(connected: true);
            _rtspConnection?.setAudioEnabled(!_muted);
          case RtspConnectionState.failed:
            _setConnState(failed: true);
          case RtspConnectionState.disposed:
            break;
        }
      });
      _rtspConnection!.connect();
    } else {
      _whepConnection = WhepConnection(
        serverUrl: serverUrl,
        mediamtxPath: livePath,
      );
      _whepStateSub = _whepConnection!.stateStream.listen((state) {
        if (!mounted) return;
        switch (state) {
          case WhepConnectionState.connecting:
            _setConnState(connecting: true);
          case WhepConnectionState.connected:
            _setConnState(connected: true);
            _whepConnection?.setAudioEnabled(!_muted);
          case WhepConnectionState.failed:
            _setConnState(failed: true);
          case WhepConnectionState.disposed:
            break;
        }
      });
      _whepConnection!.connect();
    }
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
    _keyFocusNode.dispose();
    _hideControlsTimer?.cancel();
    _whepStateSub?.cancel();
    _whepConnection?.dispose();
    _rtspStateSub?.cancel();
    _rtspConnection?.dispose();
    SystemChrome.setEnabledSystemUIMode(SystemUiMode.edgeToEdge);
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final camera = widget.camera;
    final apiClient = ref.watch(apiClientProvider);
    final overlaySettings = ref.watch(overlaySettingsProvider);
    final aiEnabled = camera.aiEnabled && overlaySettings.overlayVisible;

    return KeyboardListener(
      focusNode: _keyFocusNode,
      autofocus: true,
      onKeyEvent: _onKeyEvent,
      child: Scaffold(
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
              if (aiEnabled)
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
                if (_useRtsp) {
                  _rtspConnection?.setAudioEnabled(!_muted);
                } else {
                  _whepConnection?.setAudioEnabled(!_muted);
                }
              },
            ),
            const SizedBox(width: 8),

            // AI toggle pill
            _AiTogglePill(
              enabled: ref.watch(overlaySettingsProvider).overlayVisible,
              onChanged: (v) => ref
                  .read(overlaySettingsProvider.notifier)
                  .setOverlayVisible(v),
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
    if (_isConnecting) {
      return const Center(
        child: CircularProgressIndicator(color: NvrColors.accent),
      );
    }

    if (_isConnected) {
      if (_useRtsp) {
        final vc = _rtspConnection?.videoController;
        if (vc == null) {
          return const Center(
            child: CircularProgressIndicator(color: NvrColors.accent),
          );
        }
        return Video(
          controller: vc,
          fill: Colors.black,
        );
      } else {
        final renderer = _whepConnection?.renderer;
        if (renderer == null) {
          return const Center(
            child: CircularProgressIndicator(color: NvrColors.accent),
          );
        }
        return RTCVideoView(
          renderer,
          objectFit: RTCVideoViewObjectFit.RTCVideoViewObjectFitContain,
        );
      }
    }

    if (_isFailed) {
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
              onPressed: () {
                if (_useRtsp) {
                  _rtspConnection?.retry();
                } else {
                  _whepConnection?.retry();
                }
              },
            ),
          ],
        ),
      );
    }

    return const SizedBox.shrink();
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
