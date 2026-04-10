// KAI-301 — Camera grid view widget.
//
// Pure presentation: receives a [Camera] list, a [GridLayout], an
// `alwaysLiveOverride` flag, and whether the active connection is on LAN.
// Decides the [RenderMode] once via `decideRenderMode` and applies the same
// mode to every visible tile — the ticket does not ask for per-tile mode
// mixing. Mixed-mode grids would confuse the telemetry snapshot.
//
// Snapshot mode: one [SnapshotRefreshController] owned by this widget fans
// out frames to all snapshot tiles.

import 'dart:typed_data';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import 'camera.dart';
import 'camera_tile.dart';
import 'grid_layout_picker.dart';
import 'grid_strings.dart';
import 'render_mode.dart';
import 'snapshot_refresh_controller.dart';
import 'stream_url_minter.dart';

class CameraGridView extends ConsumerStatefulWidget {
  final List<Camera> cameras;
  final GridLayout layout;
  final bool alwaysLiveOverride;
  final bool isOnLan;
  final StreamUrlMinter minter;
  final GridStrings strings;

  /// Optional override for tests — supply a pre-built
  /// [SnapshotRefreshController] so the test doesn't need a real fetcher.
  final SnapshotRefreshController? snapshotControllerOverride;

  const CameraGridView({
    super.key,
    required this.cameras,
    required this.layout,
    required this.alwaysLiveOverride,
    required this.isOnLan,
    required this.minter,
    required this.strings,
    this.snapshotControllerOverride,
  });

  @override
  ConsumerState<CameraGridView> createState() => _CameraGridViewState();
}

class _CameraGridViewState extends ConsumerState<CameraGridView>
    with WidgetsBindingObserver {
  SnapshotRefreshController? _snapshots;

  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addObserver(this);
    _ensureSnapshotsIfNeeded();
  }

  @override
  void didUpdateWidget(covariant CameraGridView oldWidget) {
    super.didUpdateWidget(oldWidget);
    final modeChanged = _modeFor(oldWidget) != _modeFor(widget);
    final camsChanged = oldWidget.cameras != widget.cameras;
    if (modeChanged || camsChanged) {
      _ensureSnapshotsIfNeeded();
    }
  }

  @override
  void didChangeAppLifecycleState(AppLifecycleState state) {
    switch (state) {
      case AppLifecycleState.paused:
      case AppLifecycleState.inactive:
      case AppLifecycleState.hidden:
        _snapshots?.pauseForLifecycle();
        break;
      case AppLifecycleState.resumed:
        _snapshots?.resumeFromLifecycle();
        break;
      case AppLifecycleState.detached:
        break;
    }
  }

  @override
  void dispose() {
    WidgetsBinding.instance.removeObserver(this);
    // Only dispose the controller if *we* created it.
    if (widget.snapshotControllerOverride == null) {
      _snapshots?.dispose();
    }
    super.dispose();
  }

  RenderMode _modeFor(CameraGridView w) => decideRenderMode(
        cellCount: w.cameras.length,
        alwaysLiveOverride: w.alwaysLiveOverride,
        isOnLan: w.isOnLan,
      );

  void _ensureSnapshotsIfNeeded() {
    final mode = _modeFor(widget);
    if (mode != RenderMode.snapshotRefresh) {
      // Not needed — make sure we aren't paying for timers.
      if (widget.snapshotControllerOverride == null) {
        _snapshots?.dispose();
      }
      _snapshots = null;
      return;
    }
    _snapshots ??= widget.snapshotControllerOverride ??
        SnapshotRefreshController(
          minter: widget.minter,
          fetcher: (ticket) async {
            // Scaffold fetcher: the real HTTP call will plug in here when
            // the stream URL minting contract lands (lead-cloud / KAI-149).
            // For now, return an empty buffer so the tile shows the label
            // without flickering.
            return Uint8List(0);
          },
        );
    _snapshots!.start(widget.cameras);
  }

  @override
  Widget build(BuildContext context) {
    final cameras = widget.cameras;
    if (cameras.isEmpty) {
      return Center(
        child: Text(
          widget.strings.noCameras,
          key: const Key('grid-empty'),
        ),
      );
    }
    final mode = _modeFor(widget);
    final cross = widget.layout.crossAxisCount;
    return GridView.builder(
      key: const Key('camera-grid'),
      padding: const EdgeInsets.all(8),
      gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
        crossAxisCount: cross,
        crossAxisSpacing: 6,
        mainAxisSpacing: 6,
        childAspectRatio: 16 / 9,
      ),
      itemCount: cameras.length,
      itemBuilder: (context, i) {
        final cam = cameras[i];
        return CameraTile(
          key: Key('camera-tile-${cam.id}'),
          camera: cam,
          mode: mode,
          minter: widget.minter,
          strings: widget.strings,
          snapshotStream: _snapshots?.frames,
        );
      },
    );
  }
}
