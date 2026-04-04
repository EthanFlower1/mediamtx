import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../providers/auth_provider.dart';
import '../../theme/nvr_colors.dart';
import '../../theme/nvr_typography.dart';
import '../../utils/snackbar_helper.dart';
import '../../widgets/hud/hud_button.dart';

class ScreenshotsScreen extends ConsumerStatefulWidget {
  const ScreenshotsScreen({super.key});

  @override
  ConsumerState<ScreenshotsScreen> createState() => _ScreenshotsScreenState();
}

class _ScreenshotsScreenState extends ConsumerState<ScreenshotsScreen> {
  List<dynamic> _screenshots = [];
  List<dynamic> _cameras = [];
  int _total = 0;
  int _page = 1;
  final int _perPage = 20;
  String _cameraFilter = '';
  String _sort = 'newest';
  bool _loading = true;

  @override
  void initState() {
    super.initState();
    _fetchScreenshots();
    _fetchCameras();
  }


  Future<void> _fetchCameras() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      final res = await api.get<dynamic>('/cameras');
      if (mounted) setState(() => _cameras = res.data as List<dynamic>? ?? []);
    } catch (e) {
      if (mounted) showErrorSnackBar(context, 'Failed to load cameras');
    }
  }

  Future<void> _fetchScreenshots({bool append = false}) async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    if (!append) setState(() => _loading = true);
    try {
      final res = await api.get<dynamic>('/screenshots', queryParameters: {
        if (_cameraFilter.isNotEmpty) 'camera_id': _cameraFilter,
        'sort': _sort,
        'page': '${append ? _page + 1 : 1}',
        'per_page': '$_perPage',
      });
      final data = res.data as Map<String, dynamic>;
      final list = data['screenshots'] as List<dynamic>? ?? [];
      if (mounted) {
        setState(() {
          if (append) {
            _screenshots.addAll(list);
            _page++;
          } else {
            _screenshots = list;
            _page = 1;
          }
          _total = data['total'] as int? ?? 0;
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) setState(() => _loading = false);
    }
  }

  void _showFullScreenDialog(dynamic screenshot) {
    final auth = ref.read(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final imageUrl = '$serverUrl${screenshot['file_path']}';
    final id = screenshot['id'] as int;
    final createdAt = screenshot['created_at'] as String? ?? '';

    final cameraId = screenshot['camera_id'] as String? ?? '';
    String cameraName = cameraId;
    for (final c in _cameras) {
      if ((c as Map)['id'] == cameraId) {
        cameraName = c['name'] as String? ?? cameraId;
        break;
      }
    }

    showDialog(
      context: context,
      builder: (ctx) => Dialog(
        backgroundColor: NvrColors.of(context).bgPrimary,
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Padding(
              padding: const EdgeInsets.all(12),
              child: Row(
                children: [
                  Text(cameraName, style: NvrTypography.of(context).cameraName),
                  const Spacer(),
                  Text(createdAt, style: NvrTypography.of(context).monoLabel),
                  const SizedBox(width: 8),
                  IconButton(
                    icon: Icon(Icons.close,
                        color: NvrColors.of(context).textMuted, size: 18),
                    onPressed: () => Navigator.of(ctx).pop(),
                  ),
                ],
              ),
            ),
            ConstrainedBox(
              constraints: const BoxConstraints(maxHeight: 500),
              child: Image.network(imageUrl, fit: BoxFit.contain),
            ),
            Padding(
              padding: const EdgeInsets.all(12),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.end,
                children: [
                  HudButton(
                    style: HudButtonStyle.danger,
                    label: 'DELETE',
                    icon: Icons.delete_outline,
                    onPressed: () async {
                      Navigator.of(ctx).pop();
                      await _deleteScreenshot(id);
                    },
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _deleteScreenshot(int id) async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    try {
      await api.delete('/screenshots/$id');
      _fetchScreenshots();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
              backgroundColor: NvrColors.of(context).success,
              content: Text('Screenshot deleted')),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
              backgroundColor: NvrColors.of(context).danger,
              content: Text('Error: $e')),
        );
      }
    }
  }

  Widget _buildCard(dynamic screenshot) {
    final auth = ref.read(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final imageUrl = '$serverUrl${screenshot['file_path']}';

    final cameraId = screenshot['camera_id'] as String? ?? '';
    String cameraName = cameraId;
    for (final c in _cameras) {
      if ((c as Map)['id'] == cameraId) {
        cameraName = c['name'] as String? ?? cameraId;
        break;
      }
    }
    final createdAt = screenshot['created_at'] as String? ?? '';

    return GestureDetector(
      onTap: () => _showFullScreenDialog(screenshot),
      child: Container(
        decoration: BoxDecoration(
          color: NvrColors.of(context).bgSecondary,
          borderRadius: BorderRadius.circular(8),
          border: Border.all(color: NvrColors.of(context).border, width: 1),
        ),
        child: ClipRRect(
          borderRadius: BorderRadius.circular(8),
          child: Column(
            children: [
              Expanded(
                child: Image.network(
                  imageUrl,
                  fit: BoxFit.cover,
                  width: double.infinity,
                  errorBuilder: (context, error, stackTrace) => Container(
                    color: NvrColors.of(context).bgTertiary,
                    child: Center(
                      child: Icon(Icons.broken_image_outlined,
                          color: NvrColors.of(context).textMuted, size: 32),
                    ),
                  ),
                ),
              ),
              Padding(
                padding: const EdgeInsets.all(8),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      cameraName,
                      style: NvrTypography.of(context).monoLabel,
                      overflow: TextOverflow.ellipsis,
                      maxLines: 1,
                    ),
                    const SizedBox(height: 2),
                    Text(
                      createdAt,
                      style: NvrTypography.of(context).monoLabel
                          .copyWith(color: NvrColors.of(context).textMuted),
                      overflow: TextOverflow.ellipsis,
                      maxLines: 1,
                    ),
                  ],
                ),
              ),
            ],
          ),
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      backgroundColor: NvrColors.of(context).bgPrimary,
      body: SafeArea(
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            // Header
            Padding(
              padding:
                  const EdgeInsets.symmetric(horizontal: 16, vertical: 12),
              child: Row(
                children: [
                  Text('SCREENSHOTS', style: NvrTypography.of(context).pageTitle),
                  const Spacer(),
                  IconButton(
                    icon: Icon(Icons.refresh, color: NvrColors.of(context).textMuted, size: 20),
                    tooltip: 'Refresh',
                    onPressed: () {
                      _fetchScreenshots();
                      _fetchCameras();
                    },
                  ),
                ],
              ),
            ),
            // Filter bar
            Padding(
              padding:
                  const EdgeInsets.symmetric(horizontal: 16, vertical: 4),
              child: Row(
                children: [
                  Expanded(
                    child: DropdownButtonFormField<String>(
                      initialValue: _cameraFilter,
                      dropdownColor: NvrColors.of(context).bgTertiary,
                      style: NvrTypography.of(context).monoData,
                      decoration: InputDecoration(
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
                        contentPadding: const EdgeInsets.symmetric(
                            horizontal: 12, vertical: 10),
                      ),
                      items: [
                        const DropdownMenuItem(
                            value: '', child: Text('All Cameras')),
                        ..._cameras.map((c) {
                          final cam = c as Map<String, dynamic>;
                          return DropdownMenuItem(
                            value: cam['id'] as String,
                            child: Text(
                                cam['name'] as String? ?? 'Unknown'),
                          );
                        }),
                      ],
                      onChanged: (v) {
                        setState(() => _cameraFilter = v ?? '');
                        _fetchScreenshots();
                      },
                    ),
                  ),
                  const SizedBox(width: 8),
                  Expanded(
                    child: DropdownButtonFormField<String>(
                    initialValue: _sort,
                    dropdownColor: NvrColors.of(context).bgTertiary,
                    style: NvrTypography.of(context).monoData,
                    decoration: InputDecoration(
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
                      contentPadding: const EdgeInsets.symmetric(
                          horizontal: 12, vertical: 10),
                    ),
                    items: const [
                      DropdownMenuItem(
                          value: 'newest', child: Text('Newest')),
                      DropdownMenuItem(
                          value: 'oldest', child: Text('Oldest')),
                    ],
                    onChanged: (v) {
                      setState(() => _sort = v ?? 'newest');
                      _fetchScreenshots();
                    },
                  ),
                  ),
                ],
              ),
            ),
            const SizedBox(height: 8),
            // Grid body
            Expanded(
              child: _loading
                  ? Center(
                      child: CircularProgressIndicator(
                          color: NvrColors.of(context).accent))
                  : _screenshots.isEmpty
                      ? Center(
                          child: Text(
                            'No screenshots yet',
                            style: TextStyle(color: NvrColors.of(context).textMuted),
                          ),
                        )
                      : GridView.builder(
                          padding: const EdgeInsets.all(16),
                          gridDelegate:
                              SliverGridDelegateWithFixedCrossAxisCount(
                            crossAxisCount:
                                MediaQuery.of(context).size.width >= 800
                                    ? 4
                                    : 2,
                            crossAxisSpacing: 8,
                            mainAxisSpacing: 8,
                            childAspectRatio: 1.2,
                          ),
                          itemCount: _screenshots.length,
                          itemBuilder: (context, index) =>
                              _buildCard(_screenshots[index]),
                        ),
            ),
            // Pagination footer
            if (_screenshots.length < _total)
              Padding(
                padding: const EdgeInsets.symmetric(
                    horizontal: 16, vertical: 12),
                child: Row(
                  mainAxisAlignment: MainAxisAlignment.center,
                  children: [
                    HudButton(
                      label: 'LOAD MORE',
                      style: HudButtonStyle.secondary,
                      onPressed: () => _fetchScreenshots(append: true),
                    ),
                    const SizedBox(width: 12),
                    Text(
                      '${_screenshots.length} / $_total',
                      style: NvrTypography.of(context).monoLabel,
                    ),
                  ],
                ),
              ),
          ],
        ),
      ),
    );
  }
}
