// KAI-301 — Single camera tile that decides between WebRTC and snapshot mode.
//
// The tile is dumb: it's told a [RenderMode] by the parent grid (which in
// turn asks `decideRenderMode`) and renders the appropriate sub-widget. It
// does NOT own the snapshot refresh controller — one controller lives at the
// grid level and pushes frames down to all snapshot tiles at once.

import 'dart:async';
import 'dart:typed_data';

import 'package:flutter/material.dart';

import 'camera.dart';
import 'grid_strings.dart';
import 'render_mode.dart';
import 'snapshot_refresh_controller.dart';
import 'stream_url_minter.dart';
import 'webrtc_tile.dart';

class CameraTile extends StatefulWidget {
  final Camera camera;
  final RenderMode mode;
  final StreamUrlMinter minter;
  final GridStrings strings;

  /// Stream of frames produced by the grid-level snapshot refresh controller.
  /// Only consulted when [mode] is [RenderMode.snapshotRefresh].
  final Stream<SnapshotFrame>? snapshotStream;

  const CameraTile({
    super.key,
    required this.camera,
    required this.mode,
    required this.minter,
    required this.strings,
    this.snapshotStream,
  });

  @override
  State<CameraTile> createState() => _CameraTileState();
}

class _CameraTileState extends State<CameraTile> {
  StreamSubscription<SnapshotFrame>? _sub;
  Uint8List? _lastFrame;

  @override
  void initState() {
    super.initState();
    _listenIfSnapshot();
  }

  @override
  void didUpdateWidget(covariant CameraTile oldWidget) {
    super.didUpdateWidget(oldWidget);
    if (oldWidget.snapshotStream != widget.snapshotStream ||
        oldWidget.mode != widget.mode) {
      _sub?.cancel();
      _listenIfSnapshot();
    }
  }

  void _listenIfSnapshot() {
    if (widget.mode != RenderMode.snapshotRefresh) return;
    final s = widget.snapshotStream;
    if (s == null) return;
    _sub = s.where((f) => f.cameraId == widget.camera.id).listen((f) {
      if (!mounted) return;
      setState(() => _lastFrame = f.bytes);
    });
  }

  @override
  void dispose() {
    _sub?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    if (widget.mode == RenderMode.webrtc) {
      return WebRtcTile(
        camera: widget.camera,
        minter: widget.minter,
        strings: widget.strings,
      );
    }
    return _SnapshotTile(
      camera: widget.camera,
      frameBytes: _lastFrame,
      strings: widget.strings,
    );
  }
}

class _SnapshotTile extends StatelessWidget {
  final Camera camera;
  final Uint8List? frameBytes;
  final GridStrings strings;

  const _SnapshotTile({
    required this.camera,
    required this.frameBytes,
    required this.strings,
  });

  @override
  Widget build(BuildContext context) {
    return ColoredBox(
      color: const Color(0xFF0E0E0E),
      child: Stack(
        fit: StackFit.expand,
        children: [
          if (frameBytes != null && frameBytes!.isNotEmpty)
            Image.memory(
              frameBytes!,
              fit: BoxFit.cover,
              gaplessPlayback: true,
              key: Key('snapshot-image-${camera.id}'),
            )
          else
            Center(
              child: Text(
                strings.connecting,
                key: Key('snapshot-tile-connecting-${camera.id}'),
                style: const TextStyle(color: Colors.white54),
              ),
            ),
          Positioned(
            left: 8,
            bottom: 8,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
              decoration: BoxDecoration(
                color: Colors.black.withValues(alpha: 0.65),
                borderRadius: BorderRadius.circular(4),
              ),
              child: Text(
                camera.label,
                style: const TextStyle(color: Colors.white, fontSize: 11),
              ),
            ),
          ),
          Positioned(
            right: 8,
            top: 8,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
              decoration: BoxDecoration(
                color: Colors.black.withValues(alpha: 0.7),
                borderRadius: BorderRadius.circular(4),
              ),
              child: Text(
                strings.snapshotMode,
                style: const TextStyle(
                    color: Colors.white70,
                    fontSize: 10,
                    fontWeight: FontWeight.w600),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
