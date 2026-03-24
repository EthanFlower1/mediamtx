import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../../models/search_result.dart';
import '../../providers/auth_provider.dart';
import '../../providers/search_provider.dart';
import '../../services/playback_service.dart';
import '../../theme/nvr_colors.dart';
import 'clip_player_sheet.dart';
import 'search_result_card.dart';

class ClipSearchScreen extends ConsumerStatefulWidget {
  const ClipSearchScreen({super.key});

  @override
  ConsumerState<ClipSearchScreen> createState() => _ClipSearchScreenState();
}

class _ClipSearchScreenState extends ConsumerState<ClipSearchScreen> {
  final _controller = TextEditingController();
  final _focusNode = FocusNode();

  @override
  void dispose() {
    _controller.dispose();
    _focusNode.dispose();
    super.dispose();
  }

  void _search() {
    final q = _controller.text.trim();
    if (q.isEmpty) return;
    ref.read(searchProvider.notifier).search(q);
    _focusNode.unfocus();
  }

  void _openClip(BuildContext context, SearchResult result) {
    final auth = ref.read(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final svc = PlaybackService(serverUrl: serverUrl);

    // Center a 30-second clip around the detection frame time
    final frameTime = result.time;
    final clipStart = frameTime.subtract(const Duration(seconds: 15));
    final url = svc.playbackUrl(
      result.cameraId,
      clipStart,
      durationSecs: 30,
    );

    final title =
        '${result.cameraName} — ${result.className} @ ${_formatTime(frameTime)}';

    ClipPlayerSheet.show(context, url: url, title: title);
  }

  void _saveClip(BuildContext context, SearchResult result) {
    ScaffoldMessenger.of(context).showSnackBar(
      SnackBar(
        backgroundColor: NvrColors.bgSecondary,
        content: Text(
          'Save clip: ${result.className} at ${_formatTime(result.time)}',
          style: const TextStyle(color: NvrColors.textPrimary),
        ),
        action: SnackBarAction(
          label: 'OK',
          textColor: NvrColors.accent,
          onPressed: () {},
        ),
      ),
    );
  }

  String _formatTime(DateTime dt) {
    final h = dt.hour.toString().padLeft(2, '0');
    final m = dt.minute.toString().padLeft(2, '0');
    final s = dt.second.toString().padLeft(2, '0');
    return '$h:$m:$s';
  }

  @override
  Widget build(BuildContext context) {
    final search = ref.watch(searchProvider);
    final auth = ref.watch(authProvider);
    final thumbnailBaseUrl = auth.serverUrl;

    return Scaffold(
      backgroundColor: NvrColors.bgPrimary,
      appBar: AppBar(
        backgroundColor: NvrColors.bgSecondary,
        title: const Text('Clip Search',
            style: TextStyle(color: NvrColors.textPrimary)),
      ),
      body: Column(
        children: [
          // Search bar
          Container(
            color: NvrColors.bgSecondary,
            padding: const EdgeInsets.fromLTRB(12, 8, 12, 12),
            child: Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _controller,
                    focusNode: _focusNode,
                    style: const TextStyle(color: NvrColors.textPrimary),
                    decoration: InputDecoration(
                      hintText: 'Search for objects, scenes, activities…',
                      hintStyle:
                          const TextStyle(color: NvrColors.textMuted, fontSize: 13),
                      filled: true,
                      fillColor: NvrColors.bgTertiary,
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(8),
                        borderSide: BorderSide.none,
                      ),
                      contentPadding:
                          const EdgeInsets.symmetric(horizontal: 12, vertical: 10),
                      prefixIcon: const Icon(Icons.search,
                          color: NvrColors.textMuted, size: 20),
                      suffixIcon: search.query.isNotEmpty
                          ? IconButton(
                              icon: const Icon(Icons.clear,
                                  color: NvrColors.textMuted, size: 18),
                              onPressed: () {
                                _controller.clear();
                                ref.read(searchProvider.notifier).clear();
                              },
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
                    backgroundColor: NvrColors.accent,
                    foregroundColor: Colors.white,
                    padding: const EdgeInsets.symmetric(
                        horizontal: 16, vertical: 12),
                    shape: RoundedRectangleBorder(
                        borderRadius: BorderRadius.circular(8)),
                  ),
                  child: const Text('Search'),
                ),
              ],
            ),
          ),
          const Divider(color: NvrColors.border, height: 1),
          // Help text
          const Padding(
            padding: EdgeInsets.symmetric(horizontal: 16, vertical: 6),
            child: Row(
              children: [
                Icon(Icons.info_outline, size: 14, color: NvrColors.textMuted),
                SizedBox(width: 6),
                Expanded(
                  child: Text(
                    'Uses AI semantic search. Try: "red car", "person with backpack", "package at door".',
                    style: TextStyle(color: NvrColors.textMuted, fontSize: 11),
                  ),
                ),
              ],
            ),
          ),
          // Results area
          Expanded(child: _buildResults(context, search, thumbnailBaseUrl)),
        ],
      ),
    );
  }

  Widget _buildResults(
      BuildContext context, SearchState state, String? baseUrl) {
    if (state.searching) {
      return const Center(
        child: CircularProgressIndicator(color: NvrColors.accent),
      );
    }

    if (state.error != null) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.error_outline, color: NvrColors.danger, size: 40),
            const SizedBox(height: 12),
            const Text(
              'Search failed',
              style: TextStyle(
                  color: NvrColors.textPrimary,
                  fontSize: 15,
                  fontWeight: FontWeight.w600),
            ),
            const SizedBox(height: 4),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 32),
              child: Text(
                state.error!,
                style: const TextStyle(
                    color: NvrColors.textMuted, fontSize: 12),
                textAlign: TextAlign.center,
              ),
            ),
          ],
        ),
      );
    }

    if (!state.searched) {
      return const Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Icon(Icons.image_search, color: NvrColors.textMuted, size: 48),
            SizedBox(height: 16),
            Text(
              'Enter a search query above',
              style: TextStyle(color: NvrColors.textMuted, fontSize: 14),
            ),
          ],
        ),
      );
    }

    if (state.results.isEmpty) {
      return Center(
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            const Icon(Icons.search_off, color: NvrColors.textMuted, size: 48),
            const SizedBox(height: 16),
            Text(
              'No results for "${state.query}"',
              style: const TextStyle(
                  color: NvrColors.textPrimary, fontSize: 14),
            ),
            const SizedBox(height: 4),
            const Text(
              'Try a different description.',
              style:
                  TextStyle(color: NvrColors.textMuted, fontSize: 12),
            ),
          ],
        ),
      );
    }

    return ListView.builder(
      itemCount: state.results.length,
      itemBuilder: (context, i) {
        final r = state.results[i];
        return SearchResultCard(
          result: r,
          thumbnailBaseUrl: baseUrl,
          onPlay: () => _openClip(context, r),
          onSave: () => _saveClip(context, r),
        );
      },
    );
  }
}
