import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/track.dart';
import '../../providers/track_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';

/// Cross-camera person tracking timeline screen (KAI-482 -- BETA).
///
/// Shows a timeline of sightings across cameras with:
///  - Camera lanes with sighting markers
///  - Camera transition visualization
///  - Confidence badges per sighting
///  - Click-to-playback from any sighting
///  - Beta badge
class TrackTimelineScreen extends ConsumerStatefulWidget {
  /// If [trackId] is provided, loads an existing track.
  /// If [detectionId] is provided, shows "Track This Person" button.
  final int? trackId;
  final int? detectionId;

  const TrackTimelineScreen({super.key, this.trackId, this.detectionId});

  @override
  ConsumerState<TrackTimelineScreen> createState() =>
      _TrackTimelineScreenState();
}

class _TrackTimelineScreenState extends ConsumerState<TrackTimelineScreen> {
  @override
  void initState() {
    super.initState();
    if (widget.trackId != null) {
      Future.microtask(
        () => ref.read(trackProvider.notifier).fetchTrack(widget.trackId!),
      );
    }
  }

  void _onPlayback(Sighting sighting) {
    // Navigate to playback screen with camera and timestamp.
    Navigator.of(context).pushNamed(
      '/playback',
      arguments: {
        'cameraId': sighting.cameraId,
        'timestamp': sighting.timestamp,
      },
    );
  }

  @override
  Widget build(BuildContext context) {
    final colors = NvrColors.of(context);
    final state = ref.watch(trackProvider);

    return Scaffold(
      backgroundColor: colors.bgPrimary,
      appBar: AppBar(
        backgroundColor: colors.bgSecondary,
        title: Row(
          children: [
            Text('Cross-Camera Tracking', style: NvrTypography.headingSmall),
            const SizedBox(width: 8),
            _BetaBadge(colors: colors),
          ],
        ),
      ),
      body: _buildBody(state, colors),
    );
  }

  Widget _buildBody(TrackState state, NvrColors colors) {
    // Loading state
    if (state.loading) {
      return Center(
        child: CircularProgressIndicator(color: colors.accent),
      );
    }

    // Error state
    if (state.error != null && state.track == null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.error_outline, color: colors.danger, size: 48),
            const SizedBox(height: 12),
            Text(state.error!, style: TextStyle(color: colors.danger)),
          ],
        ),
      );
    }

    // No track yet -- show start button
    if (state.track == null && widget.detectionId != null) {
      return _StartTrackingView(
        detectionId: widget.detectionId!,
        starting: state.starting,
        error: state.error,
        colors: colors,
      );
    }

    // No track and no detection
    if (state.track == null) {
      return Center(
        child: Text('No track loaded', style: TextStyle(color: colors.textMuted)),
      );
    }

    return _TrackTimelineView(
      track: state.track!,
      colors: colors,
      onPlayback: _onPlayback,
    );
  }
}

// ---------------------------------------------------------------------------
// Start tracking button view
// ---------------------------------------------------------------------------
class _StartTrackingView extends ConsumerWidget {
  final int detectionId;
  final bool starting;
  final String? error;
  final NvrColors colors;

  const _StartTrackingView({
    required this.detectionId,
    required this.starting,
    this.error,
    required this.colors,
  });

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.person_search, size: 64, color: colors.accent),
            const SizedBox(height: 16),
            Text(
              'Cross-Camera Tracking',
              style: NvrTypography.headingSmall.copyWith(color: colors.textPrimary),
            ),
            const SizedBox(height: 8),
            _BetaBadge(colors: colors),
            const SizedBox(height: 12),
            Text(
              'Track this person across all cameras using visual re-identification.',
              textAlign: TextAlign.center,
              style: TextStyle(color: colors.textSecondary, fontSize: 14),
            ),
            if (error != null) ...[
              const SizedBox(height: 12),
              Text(error!, style: TextStyle(color: colors.danger, fontSize: 13)),
            ],
            const SizedBox(height: 24),
            SizedBox(
              width: 200,
              height: 44,
              child: ElevatedButton(
                onPressed: starting
                    ? null
                    : () => ref.read(trackProvider.notifier).startTracking(detectionId),
                style: ElevatedButton.styleFrom(
                  backgroundColor: colors.accent,
                  foregroundColor: Colors.white,
                  shape: RoundedRectangleBorder(
                    borderRadius: BorderRadius.circular(8),
                  ),
                ),
                child: starting
                    ? SizedBox(
                        width: 20,
                        height: 20,
                        child: CircularProgressIndicator(
                          strokeWidth: 2,
                          color: Colors.white,
                        ),
                      )
                    : const Text('Track This Person'),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Timeline view
// ---------------------------------------------------------------------------
class _TrackTimelineView extends StatelessWidget {
  final Track track;
  final NvrColors colors;
  final void Function(Sighting) onPlayback;

  const _TrackTimelineView({
    required this.track,
    required this.colors,
    required this.onPlayback,
  });

  @override
  Widget build(BuildContext context) {
    final lanes = track.sightingsByCamera;
    final transitions = track.transitions;

    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        // Header
        _TrackHeader(track: track, colors: colors),
        const SizedBox(height: 16),

        // Camera lanes
        ...lanes.entries.map((entry) => _CameraLane(
          cameraName: entry.value.isNotEmpty ? entry.value.first.cameraName : entry.key,
          sightings: entry.value,
          colors: colors,
          onSightingTap: onPlayback,
        )),

        if (transitions.isNotEmpty) ...[
          const SizedBox(height: 16),
          Text('Camera Transitions',
              style: NvrTypography.labelSmall.copyWith(color: colors.textSecondary)),
          const SizedBox(height: 8),
          ...transitions.map((t) => _TransitionRow(
            from: t.from,
            to: t.to,
            colors: colors,
          )),
        ],

        const SizedBox(height: 16),
        Text('Sighting Details',
            style: NvrTypography.labelSmall.copyWith(color: colors.textSecondary)),
        const SizedBox(height: 8),
        ...track.sightings.map((s) => _SightingTile(
          sighting: s,
          colors: colors,
          onTap: () => onPlayback(s),
        )),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// Sub-widgets
// ---------------------------------------------------------------------------

class _TrackHeader extends StatelessWidget {
  final Track track;
  final NvrColors colors;

  const _TrackHeader({required this.track, required this.colors});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: colors.bgSecondary,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: colors.border.withValues(alpha: 0.3)),
      ),
      child: Row(
        children: [
          Expanded(
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Row(
                  children: [
                    Flexible(
                      child: Text(track.label,
                          style: NvrTypography.bodyMedium.copyWith(
                              color: colors.textPrimary,
                              fontWeight: FontWeight.w600)),
                    ),
                    const SizedBox(width: 8),
                    _StatusChip(status: track.status, colors: colors),
                  ],
                ),
                const SizedBox(height: 4),
                Text(
                  '${track.cameraCount} camera${track.cameraCount != 1 ? 's' : ''}'
                  ' \u00b7 ${track.sightings.length} sighting${track.sightings.length != 1 ? 's' : ''}',
                  style: TextStyle(color: colors.textMuted, fontSize: 12),
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }
}

class _CameraLane extends StatelessWidget {
  final String cameraName;
  final List<Sighting> sightings;
  final NvrColors colors;
  final void Function(Sighting) onSightingTap;

  const _CameraLane({
    required this.cameraName,
    required this.sightings,
    required this.colors,
    required this.onSightingTap,
  });

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 4),
      child: Row(
        children: [
          SizedBox(
            width: 100,
            child: Text(
              cameraName,
              style: TextStyle(color: colors.textSecondary, fontSize: 12),
              overflow: TextOverflow.ellipsis,
            ),
          ),
          const SizedBox(width: 8),
          Expanded(
            child: Container(
              height: 32,
              decoration: BoxDecoration(
                color: colors.bgTertiary,
                borderRadius: BorderRadius.circular(4),
              ),
              child: Stack(
                children: sightings.map((s) {
                  return Positioned(
                    left: 4,
                    top: 4,
                    bottom: 4,
                    child: GestureDetector(
                      onTap: () => onSightingTap(s),
                      child: Container(
                        width: 24,
                        decoration: BoxDecoration(
                          color: colors.accent,
                          borderRadius: BorderRadius.circular(3),
                        ),
                        child: Tooltip(
                          message: '${_formatTime(s.timestamp)} - '
                              '${(s.confidence * 100).round()}%',
                          child: const SizedBox.expand(),
                        ),
                      ),
                    ),
                  );
                }).toList(),
              ),
            ),
          ),
        ],
      ),
    );
  }

  String _formatTime(String ts) {
    final dt = DateTime.tryParse(ts);
    if (dt == null) return ts;
    return '${dt.hour.toString().padLeft(2, '0')}:'
        '${dt.minute.toString().padLeft(2, '0')}:'
        '${dt.second.toString().padLeft(2, '0')}';
  }
}

class _TransitionRow extends StatelessWidget {
  final Sighting from;
  final Sighting to;
  final NvrColors colors;

  const _TransitionRow({
    required this.from,
    required this.to,
    required this.colors,
  });

  @override
  Widget build(BuildContext context) {
    final fromTime = DateTime.tryParse(from.timestamp);
    final toTime = DateTime.tryParse(to.timestamp);
    String diffStr = '';
    if (fromTime != null && toTime != null) {
      final diff = toTime.difference(fromTime);
      if (diff.inMinutes > 0) {
        diffStr = '${diff.inMinutes}m ${diff.inSeconds % 60}s';
      } else {
        diffStr = '${diff.inSeconds}s';
      }
    }

    return Padding(
      padding: const EdgeInsets.symmetric(vertical: 2, horizontal: 4),
      child: Row(
        children: [
          Text(from.cameraName,
              style: TextStyle(color: colors.textSecondary, fontSize: 12)),
          const SizedBox(width: 4),
          Icon(Icons.arrow_forward, size: 14, color: colors.textMuted),
          const SizedBox(width: 4),
          Text(to.cameraName,
              style: TextStyle(color: colors.textSecondary, fontSize: 12)),
          const SizedBox(width: 6),
          Text('($diffStr)',
              style: TextStyle(color: colors.textMuted, fontSize: 11)),
        ],
      ),
    );
  }
}

class _SightingTile extends StatelessWidget {
  final Sighting sighting;
  final NvrColors colors;
  final VoidCallback onTap;

  const _SightingTile({
    required this.sighting,
    required this.colors,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    final dt = DateTime.tryParse(sighting.timestamp);
    final timeStr = dt != null
        ? '${dt.hour.toString().padLeft(2, '0')}:'
            '${dt.minute.toString().padLeft(2, '0')}:'
            '${dt.second.toString().padLeft(2, '0')}'
        : sighting.timestamp;

    return InkWell(
      onTap: onTap,
      borderRadius: BorderRadius.circular(6),
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 6),
        child: Row(
          children: [
            Expanded(
              child: Row(
                children: [
                  Text(sighting.cameraName,
                      style: TextStyle(
                          color: colors.textPrimary,
                          fontSize: 13,
                          fontWeight: FontWeight.w500)),
                  const SizedBox(width: 8),
                  Text(timeStr,
                      style: TextStyle(color: colors.textMuted, fontSize: 12)),
                ],
              ),
            ),
            _ConfidenceBadge(
                value: sighting.confidence, colors: colors),
            const SizedBox(width: 8),
            Icon(Icons.play_circle_outline,
                size: 18, color: colors.textMuted),
          ],
        ),
      ),
    );
  }
}

class _ConfidenceBadge extends StatelessWidget {
  final double value;
  final NvrColors colors;

  const _ConfidenceBadge({required this.value, required this.colors});

  @override
  Widget build(BuildContext context) {
    final pct = (value * 100).round();
    Color bg;
    Color fg;
    if (pct >= 90) {
      bg = colors.success.withValues(alpha: 0.2);
      fg = colors.success;
    } else if (pct >= 75) {
      bg = colors.warning.withValues(alpha: 0.2);
      fg = colors.warning;
    } else {
      bg = colors.danger.withValues(alpha: 0.2);
      fg = colors.danger;
    }

    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: bg,
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text('$pct%',
          style: TextStyle(color: fg, fontSize: 11, fontWeight: FontWeight.w600)),
    );
  }
}

class _BetaBadge extends StatelessWidget {
  final NvrColors colors;

  const _BetaBadge({required this.colors});

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 2),
      decoration: BoxDecoration(
        color: Colors.purple.withValues(alpha: 0.2),
        borderRadius: BorderRadius.circular(12),
        border: Border.all(color: Colors.purple.withValues(alpha: 0.3)),
      ),
      child: const Text(
        'BETA',
        style: TextStyle(
          color: Colors.purpleAccent,
          fontSize: 10,
          fontWeight: FontWeight.w700,
          letterSpacing: 0.5,
        ),
      ),
    );
  }
}

class _StatusChip extends StatelessWidget {
  final String status;
  final NvrColors colors;

  const _StatusChip({required this.status, required this.colors});

  @override
  Widget build(BuildContext context) {
    final isActive = status == 'active';
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 6, vertical: 2),
      decoration: BoxDecoration(
        color: isActive
            ? colors.success.withValues(alpha: 0.2)
            : colors.textMuted.withValues(alpha: 0.2),
        borderRadius: BorderRadius.circular(4),
      ),
      child: Text(
        status,
        style: TextStyle(
          color: isActive ? colors.success : colors.textMuted,
          fontSize: 11,
          fontWeight: FontWeight.w500,
        ),
      ),
    );
  }
}
