// KAI-301 — WebRTC grid tile (scaffold).
//
// ─────────────────────────────────────────────────────────────────────────
//  KAI-301 scaffold — real signaling lands with lead-onprem streaming
//  contract. Do NOT fill in the WHEP negotiation here; the single-camera
//  live view (lib/features/live_view/widgets/webrtc_video_view.dart) already
//  implements the real flow and will be refactored into a shared
//  `WebRtcSession` primitive when the lead-onprem contract is finalised.
// ─────────────────────────────────────────────────────────────────────────
//
// This widget exists so the grid's render path compiles and ships with a
// deterministic "connecting…" placeholder state. Widget tests assert that
// placeholder. Once `WebRtcSession` lands, replace the body of `_build`
// with the real renderer wiring and delete the TODO below.
//
// Platform coverage: flutter_webrtc ^0.12.0 supports all six targets this
// client builds for. If a target breaks analyzer we'll gate with
// `kIsWeb` / `Platform.isLinux` but we don't need to yet.

import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';

import 'camera.dart';
import 'grid_strings.dart';
import 'stream_url_minter.dart';

/// Seam the real signaling layer plugs into. lead-onprem owns the
/// implementation; for the v1 PR we only need the interface shape.
///
/// TODO(lead-onprem): provide a concrete `WebRtcSession` implementation that
/// performs WHEP negotiation against the minted ticket URL and exposes a
/// stream-of-frame-events for the renderer. See KAI-149 for the URL-minting
/// contract and `features/live_view/widgets/webrtc_video_view.dart` for the
/// existing single-camera reference flow.
abstract class WebRtcSession {
  Future<void> start(StreamTicket ticket);
  Future<void> stop();
}

/// Grid-tile widget. Renders a placeholder until a real `WebRtcSession`
/// lands. Keeps the widget test hook: finds `Key('webrtc-tile-connecting')`.
class WebRtcTile extends StatefulWidget {
  final Camera camera;
  final StreamUrlMinter minter;
  final GridStrings strings;

  /// If provided, used in tests to stub out the real signaling path entirely.
  final WebRtcSession? sessionOverride;

  const WebRtcTile({
    super.key,
    required this.camera,
    required this.minter,
    required this.strings,
    this.sessionOverride,
  });

  @override
  State<WebRtcTile> createState() => _WebRtcTileState();
}

class _WebRtcTileState extends State<WebRtcTile> {
  bool _ticketRequested = false;

  @override
  void initState() {
    super.initState();
    // Fire-and-forget: request a sub-stream ticket so the minter exercises
    // the auth token path even in the scaffold. Failures are swallowed —
    // the placeholder stays on screen.
    _bootstrap();
  }

  Future<void> _bootstrap() async {
    try {
      await widget.minter.mintSubStream(widget.camera.id);
    } catch (_) {
      // Scaffold: ignore. The real impl will surface connection errors.
    }
    if (!mounted) return;
    setState(() => _ticketRequested = true);
  }

  @override
  Widget build(BuildContext context) {
    final strings = widget.strings;
    return ColoredBox(
      color: const Color(0xFF1A1A1A),
      child: Stack(
        fit: StackFit.expand,
        children: [
          Center(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                const Icon(Icons.videocam_outlined,
                    color: Colors.white54, size: 32),
                const SizedBox(height: 8),
                Text(
                  strings.connecting,
                  key: const Key('webrtc-tile-connecting'),
                  style: const TextStyle(color: Colors.white70),
                ),
              ],
            ),
          ),
          Positioned(
            left: 8,
            bottom: 8,
            child: _LabelChip(
              text: widget.camera.label,
              online: widget.camera.isOnline,
            ),
          ),
          Positioned(
            right: 8,
            top: 8,
            child: Container(
              padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
              decoration: BoxDecoration(
                color: Colors.redAccent.withValues(alpha: 0.9),
                borderRadius: BorderRadius.circular(4),
              ),
              child: Text(
                strings.liveMode,
                style: const TextStyle(
                    color: Colors.white,
                    fontSize: 10,
                    fontWeight: FontWeight.w600),
              ),
            ),
          ),
          if (!kIsWeb && !_ticketRequested)
            const Positioned(
              right: 8,
              bottom: 8,
              child: SizedBox(
                width: 14,
                height: 14,
                child: CircularProgressIndicator(
                  strokeWidth: 2,
                  color: Colors.white70,
                ),
              ),
            ),
        ],
      ),
    );
  }
}

class _LabelChip extends StatelessWidget {
  final String text;
  final bool online;
  const _LabelChip({required this.text, required this.online});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 3),
      decoration: BoxDecoration(
        color: Colors.black.withValues(alpha: 0.65),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Row(
        mainAxisSize: MainAxisSize.min,
        children: [
          Container(
            width: 6,
            height: 6,
            decoration: BoxDecoration(
              shape: BoxShape.circle,
              color: online ? Colors.greenAccent : Colors.grey,
            ),
          ),
          const SizedBox(width: 6),
          Text(text, style: const TextStyle(color: Colors.white, fontSize: 11)),
        ],
      ),
    );
  }
}
