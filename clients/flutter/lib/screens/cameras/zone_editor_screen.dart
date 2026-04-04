import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/zone.dart';
import '../../providers/auth_provider.dart';
import '../../theme/nvr_colors.dart';

// Normalized point [0,1] x [0,1]
typedef NPoint = Offset;

const List<Color> _zoneColors = [
  Color(0xFF3b82f6), // blue
  Color(0xFF22c55e), // green
  Color(0xFFf59e0b), // amber
  Color(0xFFef4444), // red
  Color(0xFFA855F7), // purple
  Color(0xFFEC4899), // pink
];

Color _colorForIndex(int i) => _zoneColors[i % _zoneColors.length];

class ZoneEditorScreen extends ConsumerStatefulWidget {
  final String cameraId;

  const ZoneEditorScreen({super.key, required this.cameraId});

  @override
  ConsumerState<ZoneEditorScreen> createState() => _ZoneEditorScreenState();
}

class _ZoneEditorScreenState extends ConsumerState<ZoneEditorScreen> {
  List<DetectionZone> _zones = [];
  bool _loading = true;
  String? _error;
  String? _snapshotUrl;

  // Points being drawn for the new polygon (normalized coords)
  final List<NPoint> _draftPoints = [];
  // Which zone is expanded in config sheet
  String? _expandedZoneId;

  @override
  void initState() {
    super.initState();
    _loadData();
  }

  Future<void> _loadData() async {
    setState(() {
      _loading = true;
      _error = null;
    });
    final api = ref.read(apiClientProvider);
    final auth = ref.read(authProvider);
    if (api == null) return;

    try {
      // Snapshot URL (direct image, not API call)
      final base = (auth.serverUrl ?? '').replaceAll(RegExp(r'/$'), '');
      setState(() {
        _snapshotUrl = '$base/api/nvr/cameras/${widget.cameraId}/snapshot';
      });

      final res = await api.get<dynamic>('/cameras/${widget.cameraId}/zones');
      final data = res.data as List<dynamic>? ?? [];
      setState(() {
        _zones = data
            .whereType<Map<String, dynamic>>()
            .map((e) => DetectionZone.fromJson(e))
            .toList();
        _loading = false;
      });
    } catch (e) {
      // If the endpoint doesn't exist yet (404) or network error,
      // show the editor with an empty zone list rather than crashing.
      final errStr = e.toString();
      final is404 = errStr.contains('404') || errStr.contains('Not Found');
      setState(() {
        if (is404) {
          _zones = [];
          _loading = false;
        } else {
          _error = errStr;
          _loading = false;
        }
      });
    }
  }

  void _onTapCanvas(Offset localPos, Size canvasSize) {
    final norm = Offset(localPos.dx / canvasSize.width, localPos.dy / canvasSize.height);

    // Check if tapping near the first point to close polygon
    if (_draftPoints.length >= 3) {
      final first = _draftPoints.first;
      final firstPx = Offset(first.dx * canvasSize.width, first.dy * canvasSize.height);
      if ((localPos - firstPx).distance < 20) {
        _closeDraftPolygon();
        return;
      }
    }

    setState(() => _draftPoints.add(norm));
  }

  void _onDoubleTapCanvas() {
    if (_draftPoints.length >= 3) {
      _closeDraftPolygon();
    }
  }

  Future<void> _closeDraftPolygon() async {
    final points = List<NPoint>.from(_draftPoints);
    setState(() => _draftPoints.clear());

    final name = await _promptZoneName();
    if (name == null || name.isEmpty) return;

    final api = ref.read(apiClientProvider);
    if (api == null) return;

    try {
      await api.post('/cameras/${widget.cameraId}/zones', data: {
        'name': name,
        'polygon': points.map((p) => [p.dx, p.dy]).toList(),
        'enabled': true,
        'rules': [],
      });
      await _loadData();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.of(context).danger, content: Text('Error: $e')),
        );
      }
    }
  }

  Future<String?> _promptZoneName() async {
    final ctrl = TextEditingController();
    return showDialog<String>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: NvrColors.of(context).bgSecondary,
        title: Text('Zone Name', style: TextStyle(color: NvrColors.of(context).textPrimary)),
        content: TextField(
          controller: ctrl,
          autofocus: true,
          style: TextStyle(color: NvrColors.of(context).textPrimary),
          decoration: InputDecoration(
            hintText: 'e.g. Driveway',
            hintStyle: TextStyle(color: NvrColors.of(context).textMuted),
            filled: true,
            fillColor: NvrColors.of(context).bgTertiary,
            border: OutlineInputBorder(borderRadius: BorderRadius.circular(8)),
          ),
          onSubmitted: (v) => Navigator.of(ctx).pop(v.trim()),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(),
            child: Text('Cancel', style: TextStyle(color: NvrColors.of(context).textSecondary)),
          ),
          ElevatedButton(
            style: ElevatedButton.styleFrom(backgroundColor: NvrColors.of(context).accent),
            onPressed: () => Navigator.of(ctx).pop(ctrl.text.trim()),
            child: const Text('Create', style: TextStyle(color: Colors.white)),
          ),
        ],
      ),
    );
  }

  Future<void> _deleteZone(DetectionZone zone) async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: NvrColors.of(context).bgSecondary,
        title: Text('Delete Zone', style: TextStyle(color: NvrColors.of(context).textPrimary)),
        content: Text(
          'Delete zone "${zone.name}"?',
          style: TextStyle(color: NvrColors.of(context).textSecondary),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: Text('Cancel', style: TextStyle(color: NvrColors.of(context).textSecondary)),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child: Text('Delete', style: TextStyle(color: NvrColors.of(context).danger)),
          ),
        ],
      ),
    );
    if (confirmed != true) return;

    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      await api.delete('/zones/${zone.id}');
      await _loadData();
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.of(context).danger, content: Text('Error: $e')),
        );
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_loading) {
      return Center(child: CircularProgressIndicator(color: NvrColors.of(context).accent));
    }
    if (_error != null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(_error!, style: TextStyle(color: NvrColors.of(context).danger)),
            TextButton(onPressed: _loadData, child: const Text('Retry')),
          ],
        ),
      );
    }

    return SingleChildScrollView(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Padding(
            padding: EdgeInsets.fromLTRB(16, 12, 16, 4),
            child: Text(
              'Tap to add points. Double-tap or tap near start to close a zone.',
              style: TextStyle(color: NvrColors.of(context).textMuted, fontSize: 12),
            ),
          ),
          // Canvas
          LayoutBuilder(
            builder: (context, constraints) {
              final w = constraints.maxWidth;
              final h = w * 9 / 16;
              return SizedBox(
                width: w,
                height: h,
                child: Stack(
                  children: [
                    // Snapshot image
                    if (_snapshotUrl != null)
                      Positioned.fill(
                        child: Image.network(
                          _snapshotUrl!,
                          fit: BoxFit.cover,
                          errorBuilder: (_, __, ___) => Container(
                            color: NvrColors.of(context).bgTertiary,
                            child: Center(
                              child: Icon(Icons.broken_image, color: NvrColors.of(context).textMuted),
                            ),
                          ),
                        ),
                      )
                    else
                      Positioned.fill(
                        child: Container(
                          color: NvrColors.of(context).bgTertiary,
                          child: Center(
                            child: Icon(Icons.videocam, color: NvrColors.of(context).textMuted, size: 48),
                          ),
                        ),
                      ),
                    // Existing zones paint
                    Positioned.fill(
                      child: CustomPaint(
                        painter: _ZonesPainter(
                          zones: _zones,
                          draftPoints: _draftPoints,
                        ),
                      ),
                    ),
                    // Gesture detector
                    Positioned.fill(
                      child: GestureDetector(
                        onTapUp: (d) => _onTapCanvas(d.localPosition, Size(w, h)),
                        onDoubleTap: _onDoubleTapCanvas,
                        child: Container(color: Colors.transparent),
                      ),
                    ),
                    // Cancel draft button
                    if (_draftPoints.isNotEmpty)
                      Positioned(
                        top: 8,
                        right: 8,
                        child: GestureDetector(
                          onTap: () => setState(() => _draftPoints.clear()),
                          child: Container(
                            padding: const EdgeInsets.symmetric(horizontal: 8, vertical: 4),
                            decoration: BoxDecoration(
                              color: NvrColors.of(context).danger.withValues(alpha: 0.85),
                              borderRadius: BorderRadius.circular(6),
                            ),
                            child: const Text(
                              'Cancel',
                              style: TextStyle(color: Colors.white, fontSize: 12),
                            ),
                          ),
                        ),
                      ),
                  ],
                ),
              );
            },
          ),
          // Zone list
          ..._zones.asMap().entries.map((entry) {
            final idx = entry.key;
            final zone = entry.value;
            final color = _colorForIndex(idx);
            final isExpanded = _expandedZoneId == zone.id;
            return _ZoneConfigTile(
              zone: zone,
              color: color,
              isExpanded: isExpanded,
              onToggleExpand: () {
                setState(() {
                  _expandedZoneId = isExpanded ? null : zone.id;
                });
              },
              onDelete: () => _deleteZone(zone),
              onSaved: _loadData,
              cameraId: widget.cameraId,
            );
          }),
          if (_zones.isEmpty)
            Padding(
              padding: EdgeInsets.all(24),
              child: Text(
                'No zones defined. Draw one on the canvas above.',
                style: TextStyle(color: NvrColors.of(context).textMuted, fontSize: 13),
                textAlign: TextAlign.center,
              ),
            ),
          const SizedBox(height: 24),
        ],
      ),
    );
  }
}

// ---------------------------------------------------------------------------
// Painter
// ---------------------------------------------------------------------------
class _ZonesPainter extends CustomPainter {
  final List<DetectionZone> zones;
  final List<NPoint> draftPoints;

  const _ZonesPainter({required this.zones, required this.draftPoints});

  @override
  void paint(Canvas canvas, Size size) {
    for (var i = 0; i < zones.length; i++) {
      final zone = zones[i];
      if (zone.polygon.isEmpty) continue;
      final color = _colorForIndex(i);
      final fillPaint = Paint()
        ..color = color.withValues(alpha: 0.25)
        ..style = PaintingStyle.fill;
      final strokePaint = Paint()
        ..color = color
        ..style = PaintingStyle.stroke
        ..strokeWidth = 2;

      final path = Path();
      var validPoints = 0;
      for (var j = 0; j < zone.polygon.length; j++) {
        final pt = zone.polygon[j];
        if (pt.length < 2) continue; // skip malformed points
        final x = pt[0] * size.width;
        final y = pt[1] * size.height;
        if (validPoints == 0) {
          path.moveTo(x, y);
        } else {
          path.lineTo(x, y);
        }
        validPoints++;
      }
      if (validPoints < 3) continue; // need at least 3 points for a polygon
      path.close();
      canvas.drawPath(path, fillPaint);
      canvas.drawPath(path, strokePaint);
    }

    // Draft polygon
    if (draftPoints.isNotEmpty) {
      final draftPaint = Paint()
        ..color = Colors.white
        ..style = PaintingStyle.stroke
        ..strokeWidth = 1.5;
      final dotPaint = Paint()
        ..color = Colors.white
        ..style = PaintingStyle.fill;

      for (var i = 0; i < draftPoints.length; i++) {
        final px = draftPoints[i].dx * size.width;
        final py = draftPoints[i].dy * size.height;
        canvas.drawCircle(Offset(px, py), 5, dotPaint);
        if (i > 0) {
          final prev = draftPoints[i - 1];
          canvas.drawLine(
            Offset(prev.dx * size.width, prev.dy * size.height),
            Offset(px, py),
            draftPaint,
          );
        }
      }
      // Highlight first point as close target
      final first = draftPoints.first;
      canvas.drawCircle(
        Offset(first.dx * size.width, first.dy * size.height),
        8,
        Paint()
          ..color = Colors.white.withValues(alpha: 0.5)
          ..style = PaintingStyle.stroke
          ..strokeWidth = 1.5,
      );
    }
  }

  @override
  bool shouldRepaint(_ZonesPainter old) =>
      old.zones != zones || old.draftPoints != draftPoints;
}

// ---------------------------------------------------------------------------
// Zone config tile
// ---------------------------------------------------------------------------
class _ZoneConfigTile extends ConsumerStatefulWidget {
  final DetectionZone zone;
  final Color color;
  final bool isExpanded;
  final VoidCallback onToggleExpand;
  final VoidCallback onDelete;
  final VoidCallback onSaved;
  final String cameraId;

  const _ZoneConfigTile({
    required this.zone,
    required this.color,
    required this.isExpanded,
    required this.onToggleExpand,
    required this.onDelete,
    required this.onSaved,
    required this.cameraId,
  });

  @override
  ConsumerState<_ZoneConfigTile> createState() => _ZoneConfigTileState();
}

class _ZoneConfigTileState extends ConsumerState<_ZoneConfigTile> {
  static const _allClasses = ['person', 'car', 'dog'];

  late Set<String> _enabledClasses;
  late double _cooldown;
  late double _loiter;
  late bool _notifyEnter;
  late bool _notifyLeave;
  late bool _notifyLoiter;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _initFromZone();
  }

  @override
  void didUpdateWidget(_ZoneConfigTile old) {
    super.didUpdateWidget(old);
    if (old.zone != widget.zone) _initFromZone();
  }

  void _initFromZone() {
    final rules = widget.zone.rules;
    _enabledClasses = rules.map((r) => r.className).toSet();
    final firstRule = rules.isNotEmpty ? rules.first : null;
    _cooldown = (firstRule?.cooldownSeconds ?? 30).toDouble();
    _loiter = (firstRule?.loiterSeconds ?? 60).toDouble();
    _notifyEnter = firstRule?.notifyOnEnter ?? true;
    _notifyLeave = firstRule?.notifyOnLeave ?? false;
    _notifyLoiter = firstRule?.notifyOnLoiter ?? false;
  }

  Future<void> _save() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    setState(() => _saving = true);
    try {
      final rules = _enabledClasses.map((cls) => {
        'class_name': cls,
        'enabled': true,
        'cooldown_seconds': _cooldown.round(),
        'loiter_seconds': _loiter.round(),
        'notify_on_enter': _notifyEnter,
        'notify_on_leave': _notifyLeave,
        'notify_on_loiter': _notifyLoiter,
      }).toList();

      await api.put('/zones/${widget.zone.id}', data: {
        'name': widget.zone.name,
        'enabled': widget.zone.enabled,
        'polygon': widget.zone.polygon,
        'rules': rules,
      });
      widget.onSaved();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.of(context).success,
            content: Text('Zone saved'),
          ),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(backgroundColor: NvrColors.of(context).danger, content: Text('Error: $e')),
        );
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        ListTile(
          tileColor: NvrColors.of(context).bgSecondary,
          leading: Container(
            width: 16,
            height: 16,
            decoration: BoxDecoration(color: widget.color, shape: BoxShape.circle),
          ),
          title: Text(
            widget.zone.name,
            style: TextStyle(color: NvrColors.of(context).textPrimary, fontWeight: FontWeight.w500),
          ),
          trailing: Row(
            mainAxisSize: MainAxisSize.min,
            children: [
              IconButton(
                icon: Icon(Icons.delete_outline, color: NvrColors.of(context).danger, size: 20),
                onPressed: widget.onDelete,
              ),
              Icon(
                widget.isExpanded ? Icons.expand_less : Icons.expand_more,
                color: NvrColors.of(context).textMuted,
              ),
            ],
          ),
          onTap: widget.onToggleExpand,
        ),
        if (widget.isExpanded)
          Container(
            color: NvrColors.of(context).bgSecondary.withValues(alpha: 0.5),
            padding: const EdgeInsets.fromLTRB(16, 8, 16, 16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Text('Detect classes', style: TextStyle(color: NvrColors.of(context).textSecondary, fontSize: 13)),
                const SizedBox(height: 6),
                Wrap(
                  spacing: 6,
                  children: _allClasses.map((cls) {
                    final selected = _enabledClasses.contains(cls);
                    return FilterChip(
                      label: Text(cls),
                      selected: selected,
                      onSelected: (val) {
                        setState(() {
                          if (val) {
                            _enabledClasses.add(cls);
                          } else {
                            _enabledClasses.remove(cls);
                          }
                        });
                      },
                      selectedColor: widget.color,
                      backgroundColor: NvrColors.of(context).bgTertiary,
                      labelStyle: TextStyle(
                        color: selected ? Colors.white : NvrColors.of(context).textSecondary,
                        fontSize: 12,
                      ),
                      checkmarkColor: Colors.white,
                    );
                  }).toList(),
                ),
                const SizedBox(height: 12),
                _SliderRow(
                  label: 'Cooldown',
                  value: _cooldown,
                  min: 0,
                  max: 300,
                  unit: 's',
                  onChanged: (v) => setState(() => _cooldown = v),
                ),
                const SizedBox(height: 8),
                _SliderRow(
                  label: 'Loiter threshold',
                  value: _loiter,
                  min: 0,
                  max: 300,
                  unit: 's',
                  onChanged: (v) => setState(() => _loiter = v),
                ),
                const SizedBox(height: 12),
                Text('Notify on', style: TextStyle(color: NvrColors.of(context).textSecondary, fontSize: 13)),
                Row(
                  children: [
                    _CheckboxLabel(
                      label: 'Enter',
                      value: _notifyEnter,
                      onChanged: (v) => setState(() => _notifyEnter = v ?? false),
                    ),
                    _CheckboxLabel(
                      label: 'Leave',
                      value: _notifyLeave,
                      onChanged: (v) => setState(() => _notifyLeave = v ?? false),
                    ),
                    _CheckboxLabel(
                      label: 'Loiter',
                      value: _notifyLoiter,
                      onChanged: (v) => setState(() => _notifyLoiter = v ?? false),
                    ),
                  ],
                ),
                const SizedBox(height: 12),
                SizedBox(
                  width: double.infinity,
                  child: ElevatedButton(
                    style: ElevatedButton.styleFrom(
                      backgroundColor: NvrColors.of(context).accent,
                      foregroundColor: Colors.white,
                    ),
                    onPressed: _saving ? null : _save,
                    child: _saving
                        ? const SizedBox(
                            height: 16,
                            width: 16,
                            child: CircularProgressIndicator(strokeWidth: 2, color: Colors.white),
                          )
                        : const Text('Save Zone'),
                  ),
                ),
              ],
            ),
          ),
        Divider(color: NvrColors.of(context).border, height: 1),
      ],
    );
  }
}

class _SliderRow extends StatelessWidget {
  final String label;
  final double value;
  final double min;
  final double max;
  final String unit;
  final ValueChanged<double> onChanged;

  const _SliderRow({
    required this.label,
    required this.value,
    required this.min,
    required this.max,
    required this.unit,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    return Row(
      children: [
        SizedBox(
          width: 110,
          child: Text(
            label,
            style: TextStyle(color: NvrColors.of(context).textSecondary, fontSize: 13),
          ),
        ),
        Expanded(
          child: Slider(
            value: value,
            min: min,
            max: max,
            divisions: ((max - min) ~/ 10).clamp(1, 300),
            activeColor: NvrColors.of(context).accent,
            inactiveColor: NvrColors.of(context).bgTertiary,
            onChanged: onChanged,
          ),
        ),
        SizedBox(
          width: 44,
          child: Text(
            '${value.round()}$unit',
            style: TextStyle(color: NvrColors.of(context).textPrimary, fontSize: 12),
            textAlign: TextAlign.right,
          ),
        ),
      ],
    );
  }
}

class _CheckboxLabel extends StatelessWidget {
  final String label;
  final bool value;
  final ValueChanged<bool?> onChanged;

  const _CheckboxLabel({
    required this.label,
    required this.value,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        Checkbox(
          value: value,
          onChanged: onChanged,
          activeColor: NvrColors.of(context).accent,
          side: BorderSide(color: NvrColors.of(context).border),
        ),
        Text(label, style: TextStyle(color: NvrColors.of(context).textSecondary, fontSize: 13)),
        const SizedBox(width: 8),
      ],
    );
  }
}
