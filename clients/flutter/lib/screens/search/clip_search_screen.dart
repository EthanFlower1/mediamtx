import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/search_result.dart';
import '../../providers/auth_provider.dart';
import '../../providers/cameras_provider.dart';
import '../../providers/search_provider.dart';
import '../../services/playback_service.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import 'clip_player_sheet.dart';
import 'search_result_card.dart';

// ---------------------------------------------------------------------------
// Time-range preset options
// ---------------------------------------------------------------------------
enum _TimeRange { all, lastHour, today, yesterday, last7d }

extension _TimeRangeLabel on _TimeRange {
  String get label {
    switch (this) {
      case _TimeRange.all:
        return 'ALL TIME';
      case _TimeRange.lastHour:
        return 'LAST HOUR';
      case _TimeRange.today:
        return 'TODAY';
      case _TimeRange.yesterday:
        return 'YESTERDAY';
      case _TimeRange.last7d:
        return 'LAST 7D';
    }
  }
}

// ---------------------------------------------------------------------------
// Main screen
// ---------------------------------------------------------------------------
class ClipSearchScreen extends ConsumerStatefulWidget {
  const ClipSearchScreen({super.key});

  @override
  ConsumerState<ClipSearchScreen> createState() => _ClipSearchScreenState();
}

class _ClipSearchScreenState extends ConsumerState<ClipSearchScreen> {
  final _controller = TextEditingController();
  final _focusNode = FocusNode();

  // Filter state
  String? _selectedCameraId; // null = all cameras
  _TimeRange _selectedTimeRange = _TimeRange.all;
  int _confidenceThreshold = 50; // percentage

  // Cached access token — fetched once, reused by all result cards.
  String? _accessToken;

  @override
  void initState() {
    super.initState();
    _fetchAccessToken();
  }

  Future<void> _fetchAccessToken() async {
    final authService = ref.read(authServiceProvider);
    final token = await authService.getAccessToken();
    if (mounted) setState(() => _accessToken = token);
  }

  @override
  void dispose() {
    _controller.dispose();
    _focusNode.dispose();
    super.dispose();
  }

  // ---------------------------------------------------------------------------
  // Preserved logic
  // ---------------------------------------------------------------------------
  void _search() {
    final q = _controller.text.trim();
    if (q.isEmpty) return;

    final now = DateTime.now();
    DateTime? start;
    DateTime? end;

    switch (_selectedTimeRange) {
      case _TimeRange.all:
        // No time constraint — omit start/end so server searches all time.
        break;
      case _TimeRange.lastHour:
        start = now.subtract(const Duration(hours: 1));
        end = now;
      case _TimeRange.today:
        start = DateTime(now.year, now.month, now.day);
        end = now;
      case _TimeRange.yesterday:
        start = DateTime(now.year, now.month, now.day - 1);
        end = DateTime(now.year, now.month, now.day);
      case _TimeRange.last7d:
        start = now.subtract(const Duration(days: 7));
        end = now;
    }

    ref.read(searchProvider.notifier).search(q, start: start, end: end);
    _focusNode.unfocus();
  }

  void _openClip(BuildContext context, SearchResult result) async {
    final auth = ref.read(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final svc = PlaybackService(serverUrl: serverUrl);
    final authService = ref.read(authServiceProvider);
    final token = await authService.getAccessToken();

    // Look up camera's Raikada path
    final cameras = ref.read(camerasProvider).valueOrNull ?? [];
    final camera = cameras.where((c) => c.id == result.cameraId).firstOrNull;
    if (camera == null) return;

    // Center a 30-second clip around the detection frame time
    final frameTime = result.time;
    final clipStart = frameTime.subtract(const Duration(seconds: 15));
    final url = svc.clipUrl(
      cameraPath: camera.mediamtxPath,
      start: clipStart,
      durationSecs: 30,
      token: token,
    );

    final title =
        '${result.cameraName} — ${result.className} @ ${_formatTime(frameTime)}';

    if (context.mounted) {
      ClipPlayerSheet.show(context, url: url, title: title);
    }
  }

  String _formatTime(DateTime dt) {
    final h = dt.hour.toString().padLeft(2, '0');
    final m = dt.minute.toString().padLeft(2, '0');
    final s = dt.second.toString().padLeft(2, '0');
    return '$h:$m:$s';
  }

  // ---------------------------------------------------------------------------
  // Client-side filtering
  // ---------------------------------------------------------------------------
  List<SearchResult> _applyFilters(List<SearchResult> results) {
    final now = DateTime.now();
    return results.where((r) {
      // Camera filter
      if (_selectedCameraId != null && r.cameraId != _selectedCameraId) {
        return false;
      }
      // Confidence filter
      if ((r.confidence * 100) < _confidenceThreshold) return false;
      // Time range filter
      final dt = r.time;
      switch (_selectedTimeRange) {
        case _TimeRange.all:
          break;
        case _TimeRange.lastHour:
          if (now.difference(dt).inMinutes > 60) return false;
        case _TimeRange.today:
          final today = DateTime(now.year, now.month, now.day);
          if (dt.isBefore(today)) return false;
        case _TimeRange.yesterday:
          final yesterday =
              DateTime(now.year, now.month, now.day - 1);
          final today = DateTime(now.year, now.month, now.day);
          if (dt.isBefore(yesterday) || !dt.isBefore(today)) return false;
        case _TimeRange.last7d:
          if (now.difference(dt).inDays > 7) return false;
      }
      return true;
    }).toList();
  }

  // ---------------------------------------------------------------------------
  // Build
  // ---------------------------------------------------------------------------
  @override
  Widget build(BuildContext context) {
    final search = ref.watch(searchProvider);
    final auth = ref.watch(authProvider);
    final thumbnailBaseUrl = auth.serverUrl;
    final cameras = ref.watch(camerasProvider).valueOrNull ?? [];

    return Scaffold(
      backgroundColor: NvrColors.of(context).bgPrimary,
      body: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // ----------------------------------------------------------------
            // Custom top bar
            // ----------------------------------------------------------------
            Padding(
              padding: EdgeInsets.fromLTRB(16, 16, 16, 8),
              child: Text('Search', style: NvrTypography.of(context).pageTitle),
            ),
            // ----------------------------------------------------------------
            // Search input row
            // ----------------------------------------------------------------
            Padding(
              padding: const EdgeInsets.fromLTRB(12, 0, 12, 8),
              child: Row(
                children: [
                  Expanded(
                    child: TextField(
                      controller: _controller,
                      focusNode: _focusNode,
                      style: TextStyle(
                          color: NvrColors.of(context).textPrimary, fontSize: 13),
                      decoration: InputDecoration(
                        hintText: 'Search objects, scenes, activities…',
                        hintStyle: TextStyle(
                            color: NvrColors.of(context).textMuted, fontSize: 13),
                        filled: true,
                        fillColor: NvrColors.of(context).bgTertiary,
                        border: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(4),
                          borderSide:
                              BorderSide(color: NvrColors.of(context).border),
                        ),
                        enabledBorder: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(4),
                          borderSide:
                              BorderSide(color: NvrColors.of(context).border),
                        ),
                        focusedBorder: OutlineInputBorder(
                          borderRadius: BorderRadius.circular(4),
                          borderSide: BorderSide(
                              color: NvrColors.of(context).accent, width: 1.5),
                        ),
                        contentPadding: const EdgeInsets.symmetric(
                            horizontal: 12, vertical: 10),
                        prefixIcon: Icon(Icons.search,
                            color: NvrColors.of(context).textMuted, size: 18),
                        suffixIcon: search.query.isNotEmpty
                            ? IconButton(
                                icon: Icon(Icons.close,
                                    color: NvrColors.of(context).textMuted, size: 16),
                                onPressed: () {
                                  _controller.clear();
                                  ref.read(searchProvider.notifier).clear();
                                },
                                padding: EdgeInsets.zero,
                                constraints: const BoxConstraints(
                                    minWidth: 32, minHeight: 32),
                              )
                            : null,
                      ),
                      textInputAction: TextInputAction.search,
                      onSubmitted: (_) => _search(),
                    ),
                  ),
                  const SizedBox(width: 8),
                  ElevatedButton(
                    onPressed: search.searching ? null : _search,
                    style: ElevatedButton.styleFrom(
                      backgroundColor: NvrColors.of(context).accent,
                      foregroundColor: NvrColors.of(context).bgPrimary,
                      disabledBackgroundColor:
                          NvrColors.of(context).accent.withValues(alpha: 0.4),
                      padding: const EdgeInsets.symmetric(
                          horizontal: 16, vertical: 12),
                      shape: RoundedRectangleBorder(
                          borderRadius: BorderRadius.circular(4)),
                      elevation: 0,
                    ),
                    child: const Text(
                      'SEARCH',
                      style: TextStyle(
                        fontFamily: 'JetBrainsMono',
                        fontSize: 10,
                        fontWeight: FontWeight.w700,
                        letterSpacing: 1.0,
                      ),
                    ),
                  ),
                ],
              ),
            ),
            // ----------------------------------------------------------------
            // Filter pills row
            // ----------------------------------------------------------------
            SingleChildScrollView(
              scrollDirection: Axis.horizontal,
              padding: const EdgeInsets.fromLTRB(12, 0, 12, 10),
              child: Row(
                children: [
                  // Camera dropdown pill
                  _CameraDropdownPill(
                    cameras: cameras,
                    selectedId: _selectedCameraId,
                    onChanged: (id) =>
                        setState(() => _selectedCameraId = id),
                  ),
                  const SizedBox(width: 6),
                  // Time range preset pills
                  for (final range in _TimeRange.values) ...[
                    _FilterPill(
                      label: range.label,
                      active: _selectedTimeRange == range,
                      onTap: () =>
                          setState(() => _selectedTimeRange = range),
                    ),
                    const SizedBox(width: 6),
                  ],
                  // Confidence threshold pill
                  _ConfidencePill(
                    threshold: _confidenceThreshold,
                    onChanged: (v) =>
                        setState(() => _confidenceThreshold = v),
                  ),
                ],
              ),
            ),
            Divider(color: NvrColors.of(context).border, height: 1),
            // ----------------------------------------------------------------
            // Results
            // ----------------------------------------------------------------
            Expanded(
              child: _buildResults(context, search, thumbnailBaseUrl),
            ),
          ],
        ),
      ),
    );
  }

  // ---------------------------------------------------------------------------
  // Results area
  // ---------------------------------------------------------------------------
  Widget _buildResults(
      BuildContext context, SearchState state, String? baseUrl) {
    if (state.searching) {
      return Center(
        child: CircularProgressIndicator(
            color: NvrColors.of(context).accent, strokeWidth: 2),
      );
    }

    if (state.error != null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.error_outline,
                color: NvrColors.of(context).danger, size: 36),
            const SizedBox(height: 12),
            Text('Search failed', style: NvrTypography.of(context).pageTitle),
            const SizedBox(height: 6),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 32),
              child: Text(
                state.error!,
                style: NvrTypography.of(context).body,
                textAlign: TextAlign.center,
              ),
            ),
          ],
        ),
      );
    }

    if (!state.searched) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.image_search,
                color: NvrColors.of(context).textMuted.withValues(alpha: 0.4), size: 48),
            const SizedBox(height: 16),
            Text(
              'Enter a query to search recordings',
              style: NvrTypography.of(context).body,
            ),
          ],
        ),
      );
    }

    final filtered = _applyFilters(state.results);

    if (filtered.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.search_off,
                color: NvrColors.of(context).textMuted.withValues(alpha: 0.4), size: 48),
            const SizedBox(height: 16),
            Text(
              'No results found',
              style: NvrTypography.of(context).body,
            ),
            const SizedBox(height: 4),
            Text(
              'Try adjusting your query or filters.',
              style: NvrTypography.of(context).body.copyWith(
                  color: NvrColors.of(context).textMuted, fontSize: 11),
            ),
          ],
        ),
      );
    }

    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        // Result count header
        Padding(
          padding: const EdgeInsets.fromLTRB(16, 10, 16, 8),
          child: Row(
            children: [
              Text(
                '${filtered.length} RESULT${filtered.length == 1 ? '' : 'S'}',
                style: NvrTypography.of(context).monoLabel.copyWith(
                    color: NvrColors.of(context).textPrimary),
              ),
              const SizedBox(width: 10),
              Text(
                'SORTED BY RELEVANCE',
                style: NvrTypography.of(context).monoLabel,
              ),
            ],
          ),
        ),
        // Responsive grid
        Expanded(
          child: LayoutBuilder(
            builder: (context, constraints) {
              final crossAxisCount =
                  (constraints.maxWidth / 200).floor().clamp(2, 6);
              return GridView.builder(
                padding: const EdgeInsets.fromLTRB(12, 0, 12, 16),
                gridDelegate: SliverGridDelegateWithFixedCrossAxisCount(
                  crossAxisCount: crossAxisCount,
                  crossAxisSpacing: 8,
                  mainAxisSpacing: 8,
                  // card = thumbnail (16:9) + ~50px label area
                  // 16:9 thumbnail + ~52px label area below
                  childAspectRatio: () {
                    final cardWidth =
                        (constraints.maxWidth - 12 * 2 - 8 * (crossAxisCount - 1)) /
                            crossAxisCount;
                    final thumbnailHeight = cardWidth * 9 / 16;
                    return cardWidth / (thumbnailHeight + 52);
                  }(),
                ),
                itemCount: filtered.length,
                itemBuilder: (context, i) {
                  final r = filtered[i];
                  return SearchResultCard(
                    result: r,
                    thumbnailBaseUrl: baseUrl,
                    accessToken: _accessToken,
                    onTap: () => _openClip(context, r),
                  );
                },
              );
            },
          ),
        ),
      ],
    );
  }
}

// ---------------------------------------------------------------------------
// Filter pill widgets
// ---------------------------------------------------------------------------

class _FilterPill extends StatelessWidget {
  final String label;
  final bool active;
  final VoidCallback onTap;

  const _FilterPill({
    required this.label,
    required this.active,
    required this.onTap,
  });

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      child: Container(
        padding:
            const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
        decoration: BoxDecoration(
          color: active
              ? NvrColors.of(context).accent.withValues(alpha: 0.13)
              : NvrColors.of(context).bgSecondary,
          borderRadius: BorderRadius.circular(4),
          border: Border.all(
            color:
                active ? NvrColors.of(context).accent : NvrColors.of(context).border,
          ),
        ),
        child: Text(
          label,
          style: NvrTypography.of(context).monoLabel.copyWith(
            color:
                active ? NvrColors.of(context).accent : NvrColors.of(context).textMuted,
          ),
        ),
      ),
    );
  }
}

class _CameraDropdownPill extends StatelessWidget {
  final List<dynamic> cameras;
  final String? selectedId;
  final ValueChanged<String?> onChanged;

  const _CameraDropdownPill({
    required this.cameras,
    required this.selectedId,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    final selectedCamera = cameras
        .where((c) => c.id == selectedId)
        .cast<dynamic>()
        .firstOrNull;
    final label = selectedCamera != null
        ? (selectedCamera.name as String).toUpperCase()
        : 'ALL CAMERAS';

    return GestureDetector(
      onTap: () async {
        final picked = await showMenu<String?>(
          context: context,
          position: _buttonPosition(context),
          color: NvrColors.of(context).bgSecondary,
          shape: RoundedRectangleBorder(
            borderRadius: BorderRadius.circular(4),
            side: BorderSide(color: NvrColors.of(context).border),
          ),
          items: [
            PopupMenuItem<String?>(
              value: null,
              child: _menuItem(context, 'ALL CAMERAS', selectedId == null),
            ),
            ...cameras.map(
              (c) => PopupMenuItem<String?>(
                value: c.id as String,
                child: _menuItem(context, 
                    (c.name as String).toUpperCase(), c.id == selectedId),
              ),
            ),
          ],
        );
        if (picked != selectedId) onChanged(picked);
      },
      child: Container(
        padding:
            const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
        decoration: BoxDecoration(
          color: NvrColors.of(context).bgSecondary,
          borderRadius: BorderRadius.circular(4),
          border: Border.all(color: NvrColors.of(context).border),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              label,
              style: NvrTypography.of(context).monoLabel.copyWith(
                color: selectedId != null
                    ? NvrColors.of(context).accent
                    : NvrColors.of(context).textMuted,
              ),
            ),
            const SizedBox(width: 4),
            Icon(
              Icons.keyboard_arrow_down,
              size: 12,
              color: selectedId != null
                  ? NvrColors.of(context).accent
                  : NvrColors.of(context).textMuted,
            ),
          ],
        ),
      ),
    );
  }

  Widget _menuItem(BuildContext context, String label, bool selected) {
    return Text(
      label,
      style: NvrTypography.of(context).monoLabel.copyWith(
        color: selected ? NvrColors.of(context).accent : NvrColors.of(context).textMuted,
        fontSize: 10,
      ),
    );
  }

  RelativeRect _buttonPosition(BuildContext context) {
    final box = context.findRenderObject() as RenderBox?;
    if (box == null) return RelativeRect.fill;
    final offset = box.localToGlobal(Offset.zero);
    return RelativeRect.fromLTRB(
      offset.dx,
      offset.dy + box.size.height + 4,
      offset.dx + box.size.width,
      0,
    );
  }
}

class _ConfidencePill extends StatelessWidget {
  final int threshold;
  final ValueChanged<int> onChanged;

  const _ConfidencePill({
    required this.threshold,
    required this.onChanged,
  });

  @override
  Widget build(BuildContext context) {
    final isFiltered = threshold > 0;
    return GestureDetector(
      onTap: () => _showSlider(context),
      child: Container(
        padding:
            const EdgeInsets.symmetric(horizontal: 10, vertical: 5),
        decoration: BoxDecoration(
          color: isFiltered
              ? NvrColors.of(context).accent.withValues(alpha: 0.13)
              : NvrColors.of(context).bgSecondary,
          borderRadius: BorderRadius.circular(4),
          border: Border.all(
            color: isFiltered ? NvrColors.of(context).accent : NvrColors.of(context).border,
          ),
        ),
        child: Text(
          'CONF ≥$threshold%',
          style: NvrTypography.of(context).monoLabel.copyWith(
            color: isFiltered ? NvrColors.of(context).accent : NvrColors.of(context).textMuted,
          ),
        ),
      ),
    );
  }

  void _showSlider(BuildContext context) {
    showDialog<void>(
      context: context,
      builder: (ctx) => _ConfidenceDialog(
        initial: threshold,
        onChanged: onChanged,
      ),
    );
  }
}

class _ConfidenceDialog extends StatefulWidget {
  final int initial;
  final ValueChanged<int> onChanged;

  const _ConfidenceDialog({required this.initial, required this.onChanged});

  @override
  State<_ConfidenceDialog> createState() => _ConfidenceDialogState();
}

class _ConfidenceDialogState extends State<_ConfidenceDialog> {
  late int _value;

  @override
  void initState() {
    super.initState();
    _value = widget.initial;
  }

  @override
  Widget build(BuildContext context) {
    return Dialog(
      backgroundColor: NvrColors.of(context).bgSecondary,
      shape: RoundedRectangleBorder(
        borderRadius: BorderRadius.circular(4),
        side: BorderSide(color: NvrColors.of(context).border),
      ),
      child: Padding(
        padding: const EdgeInsets.all(20),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('CONFIDENCE THRESHOLD',
                style: NvrTypography.of(context).monoSection),
            const SizedBox(height: 16),
            Row(
              children: [
                Expanded(
                  child: SliderTheme(
                    data: SliderTheme.of(context).copyWith(
                      activeTrackColor: NvrColors.of(context).accent,
                      inactiveTrackColor: NvrColors.of(context).bgTertiary,
                      thumbColor: NvrColors.of(context).accent,
                      overlayColor:
                          NvrColors.of(context).accent.withValues(alpha: 0.13),
                      trackHeight: 2,
                    ),
                    child: Slider(
                      value: _value.toDouble(),
                      min: 0,
                      max: 100,
                      divisions: 20,
                      onChanged: (v) =>
                          setState(() => _value = v.round()),
                    ),
                  ),
                ),
                const SizedBox(width: 12),
                SizedBox(
                  width: 40,
                  child: Text(
                    '$_value%',
                    style: NvrTypography.of(context).monoSection,
                    textAlign: TextAlign.right,
                  ),
                ),
              ],
            ),
            const SizedBox(height: 16),
            Row(
              mainAxisAlignment: MainAxisAlignment.end,
              children: [
                TextButton(
                  onPressed: () => Navigator.of(context).pop(),
                  child: Text('CANCEL',
                      style: NvrTypography.of(context).monoLabel.copyWith(
                          color: NvrColors.of(context).textMuted, fontSize: 10)),
                ),
                const SizedBox(width: 8),
                ElevatedButton(
                  onPressed: () {
                    widget.onChanged(_value);
                    Navigator.of(context).pop();
                  },
                  style: ElevatedButton.styleFrom(
                    backgroundColor: NvrColors.of(context).accent,
                    foregroundColor: NvrColors.of(context).bgPrimary,
                    elevation: 0,
                    shape: RoundedRectangleBorder(
                        borderRadius: BorderRadius.circular(4)),
                    padding: const EdgeInsets.symmetric(
                        horizontal: 16, vertical: 8),
                  ),
                  child: const Text(
                    'APPLY',
                    style: TextStyle(
                      fontFamily: 'JetBrainsMono',
                      fontSize: 10,
                      fontWeight: FontWeight.w700,
                      letterSpacing: 1.0,
                    ),
                  ),
                ),
              ],
            ),
          ],
        ),
      ),
    );
  }
}
