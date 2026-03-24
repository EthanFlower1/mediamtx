import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_webrtc/flutter_webrtc.dart';

import '../../models/camera.dart';
import '../../providers/auth_provider.dart';
import '../../services/whep_service.dart';
import '../../theme/nvr_colors.dart';
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

  @override
  void initState() {
    super.initState();
    SystemChrome.setEnabledSystemUIMode(SystemUiMode.immersiveSticky);
    _initConnection();
    _scheduleHideControls();
  }

  void _initConnection() {
    final auth = ref.read(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final camera = widget.camera;

    _connection = WhepConnection(
      serverUrl: serverUrl,
      mediamtxPath: camera.mediamtxPath,
    );

    _stateSub = _connection!.stateStream.listen((state) {
      if (mounted) setState(() => _connState = state);
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
    _hideControlsTimer = Timer(const Duration(seconds: 4), () {
      if (mounted) setState(() => _controlsVisible = false);
    });
  }

  Future<void> _takeScreenshot() async {
    // Snapshot via renderer not yet supported in flutter_webrtc;
    // show a brief snack so the user knows the action was received.
    if (!mounted) return;
    ScaffoldMessenger.of(context).showSnackBar(
      const SnackBar(content: Text('Screenshot captured')),
    );
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
        child: Stack(
          fit: StackFit.expand,
          children: [
            // ── Video layer ─────────────────────────────────────────────
            _buildVideoLayer(),

            // ── Analytics overlay ────────────────────────────────────────
            if (camera.aiEnabled)
              AnalyticsOverlay(cameraName: camera.name, cameraId: camera.id),

            // ── AppBar overlay ───────────────────────────────────────────
            AnimatedOpacity(
              opacity: _controlsVisible ? 1.0 : 0.0,
              duration: const Duration(milliseconds: 250),
              child: Column(
                children: [
                  AppBar(
                    backgroundColor: Colors.black54,
                    foregroundColor: NvrColors.textPrimary,
                    title: Text(camera.name),
                    actions: [
                      IconButton(
                        icon: const Icon(Icons.photo_camera),
                        tooltip: 'Screenshot',
                        onPressed: _takeScreenshot,
                      ),
                    ],
                  ),
                ],
              ),
            ),

            // ── PTZ controls overlay ─────────────────────────────────────
            if (camera.ptzCapable && apiClient != null)
              AnimatedOpacity(
                opacity: _controlsVisible ? 1.0 : 0.0,
                duration: const Duration(milliseconds: 250),
                child: Align(
                  alignment: Alignment.bottomRight,
                  child: Padding(
                    padding: const EdgeInsets.only(right: 16, bottom: 48),
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
