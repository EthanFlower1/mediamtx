import 'dart:io';
import 'package:dio/dio.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:path_provider/path_provider.dart';
import 'package:share_plus/share_plus.dart';
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

  // Selection mode state
  bool _selectionMode = false;
  final Set<int> _selectedIds = {};

  @override
  void initState() {
    super.initState();
    _fetchScreenshots();
    _fetchCameras();
  }

  Future<String?> _getAccessToken() async {
    final authService = ref.read(authServiceProvider);
    return authService.getAccessToken();
  }

  Map<String, String> _authHeaders(String token) {
    return {'Authorization': 'Bearer $token'};
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

  String _cameraNameForId(String cameraId) {
    for (final c in _cameras) {
      if ((c as Map)['id'] == cameraId) {
        return c['name'] as String? ?? cameraId;
      }
    }
    return cameraId;
  }

  Future<Directory> _getDownloadDir() async {
    try {
      final dir = await getDownloadsDirectory();
      if (dir != null) return dir;
    } catch (_) {
      // getDownloadsDirectory() can return null or throw on some platforms
    }
    try {
      return await getApplicationDocumentsDirectory();
    } catch (_) {
      return await getTemporaryDirectory();
    }
  }

  void _showFullScreenDialog(dynamic screenshot) {
    final auth = ref.read(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final imageUrl = '$serverUrl${screenshot['file_path']}';
    final id = screenshot['id'] as int;
    final createdAt = screenshot['created_at'] as String? ?? '';
    final cameraId = screenshot['camera_id'] as String? ?? '';
    final cameraName = _cameraNameForId(cameraId);

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
              child: FutureBuilder<String?>(
                future: _getAccessToken(),
                builder: (context, snap) {
                  final token = snap.data;
                  return Image.network(
                    imageUrl,
                    fit: BoxFit.contain,
                    headers: token != null ? _authHeaders(token) : null,
                  );
                },
              ),
            ),
            Padding(
              padding: const EdgeInsets.all(12),
              child: Row(
                mainAxisAlignment: MainAxisAlignment.end,
                children: [
                  HudButton(
                    style: HudButtonStyle.secondary,
                    label: 'DOWNLOAD',
                    icon: Icons.download,
                    onPressed: () => _downloadScreenshot(imageUrl, cameraName, createdAt),
                  ),
                  const SizedBox(width: 8),
                  HudButton(
                    style: HudButtonStyle.secondary,
                    label: 'SHARE',
                    icon: Icons.share,
                    onPressed: () => _shareScreenshot(imageUrl, cameraName, createdAt),
                  ),
                  const SizedBox(width: 8),
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

  Future<void> _downloadScreenshot(String imageUrl, String cameraName, String createdAt) async {
    try {
      final dir = await _getDownloadDir();
      final safeCamera = cameraName.replaceAll(RegExp(r'[^\w\-]'), '_');
      final safeTime = createdAt.replaceAll(RegExp(r'[^\w\-]'), '_');
      final filePath = '${dir.path}/${safeCamera}_$safeTime.jpg';

      final token = await _getAccessToken();
      await Dio().download(
        imageUrl,
        filePath,
        options: Options(
          headers: token != null ? _authHeaders(token) : null,
        ),
      );

      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.of(context).success,
            content: Text('Saved to $filePath'),
          ),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.of(context).danger,
            content: Text('Download failed: $e'),
          ),
        );
      }
    }
  }

  Future<void> _shareScreenshot(String imageUrl, String cameraName, String createdAt) async {
    try {
      final dir = await getTemporaryDirectory();
      final filePath = '${dir.path}/screenshot_${DateTime.now().millisecondsSinceEpoch}.jpg';

      final token = await _getAccessToken();
      await Dio().download(
        imageUrl,
        filePath,
        options: Options(
          headers: token != null ? _authHeaders(token) : null,
        ),
      );

      await Share.shareXFiles(
        [XFile(filePath)],
        text: '$cameraName - $createdAt',
      );
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.of(context).danger,
            content: Text('Share failed: $e'),
          ),
        );
      }
    }
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
              content: const Text('Screenshot deleted')),
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

  // --- Selection mode helpers ---

  void _toggleSelectionMode() {
    setState(() {
      _selectionMode = !_selectionMode;
      if (!_selectionMode) _selectedIds.clear();
    });
  }

  void _toggleSelection(int id) {
    setState(() {
      if (_selectedIds.contains(id)) {
        _selectedIds.remove(id);
      } else {
        _selectedIds.add(id);
      }
    });
  }

  void _selectAll() {
    setState(() {
      for (final s in _screenshots) {
        _selectedIds.add(s['id'] as int);
      }
    });
  }

  void _deselectAll() {
    setState(() => _selectedIds.clear());
  }

  List<dynamic> get _selectedScreenshots =>
      _screenshots.where((s) => _selectedIds.contains(s['id'] as int)).toList();

  Future<void> _downloadAllSelected() async {
    final auth = ref.read(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    int successCount = 0;
    int failCount = 0;

    for (final screenshot in _selectedScreenshots) {
      final imageUrl = '$serverUrl${screenshot['file_path']}';
      final cameraId = screenshot['camera_id'] as String? ?? '';
      final cameraName = _cameraNameForId(cameraId);
      final createdAt = screenshot['created_at'] as String? ?? '';
      try {
        final dir = await _getDownloadDir();
        final safeCamera = cameraName.replaceAll(RegExp(r'[^\w\-]'), '_');
        final safeTime = createdAt.replaceAll(RegExp(r'[^\w\-]'), '_');
        final filePath = '${dir.path}/${safeCamera}_$safeTime.jpg';

        final token = await _getAccessToken();
        await Dio().download(
          imageUrl,
          filePath,
          options: Options(
            headers: token != null ? _authHeaders(token) : null,
          ),
        );
        successCount++;
      } catch (_) {
        failCount++;
      }
    }

    if (mounted) {
      final msg = failCount == 0
          ? 'Downloaded $successCount screenshots'
          : 'Downloaded $successCount, failed $failCount';
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          backgroundColor: failCount == 0
              ? NvrColors.of(context).success
              : NvrColors.of(context).danger,
          content: Text(msg),
        ),
      );
    }
  }

  Future<void> _shareAllSelected() async {
    final auth = ref.read(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final dir = await getTemporaryDirectory();
    final files = <XFile>[];

    for (final screenshot in _selectedScreenshots) {
      final imageUrl = '$serverUrl${screenshot['file_path']}';
      try {
        final filePath =
            '${dir.path}/screenshot_${screenshot['id']}_${DateTime.now().millisecondsSinceEpoch}.jpg';
        final token = await _getAccessToken();
        await Dio().download(
          imageUrl,
          filePath,
          options: Options(
            headers: token != null ? _authHeaders(token) : null,
          ),
        );
        files.add(XFile(filePath));
      } catch (_) {
        // skip failed downloads
      }
    }

    if (files.isNotEmpty) {
      await Share.shareXFiles(files, text: '${files.length} screenshots');
    } else if (mounted) {
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          backgroundColor: NvrColors.of(context).danger,
          content: const Text('Failed to prepare files for sharing'),
        ),
      );
    }
  }

  Future<void> _deleteAllSelected() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (ctx) => AlertDialog(
        backgroundColor: NvrColors.of(context).bgSecondary,
        title: Text('Delete ${_selectedIds.length} screenshots?',
            style: TextStyle(color: NvrColors.of(context).textPrimary)),
        content: Text('This action cannot be undone.',
            style: TextStyle(color: NvrColors.of(context).textMuted)),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(false),
            child: Text('Cancel',
                style: TextStyle(color: NvrColors.of(context).textMuted)),
          ),
          TextButton(
            onPressed: () => Navigator.of(ctx).pop(true),
            child: Text('Delete',
                style: TextStyle(color: NvrColors.of(context).danger)),
          ),
        ],
      ),
    );

    if (confirmed != true) return;

    final api = ref.read(apiClientProvider);
    if (api == null) return;

    int successCount = 0;
    int failCount = 0;
    for (final id in _selectedIds.toList()) {
      try {
        await api.delete('/screenshots/$id');
        successCount++;
      } catch (_) {
        failCount++;
      }
    }

    setState(() {
      _selectionMode = false;
      _selectedIds.clear();
    });
    _fetchScreenshots();

    if (mounted) {
      final msg = failCount == 0
          ? 'Deleted $successCount screenshots'
          : 'Deleted $successCount, failed $failCount';
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(
          backgroundColor: failCount == 0
              ? NvrColors.of(context).success
              : NvrColors.of(context).danger,
          content: Text(msg),
        ),
      );
    }
  }

  Widget _buildCard(dynamic screenshot) {
    final auth = ref.read(authProvider);
    final serverUrl = auth.serverUrl ?? '';
    final imageUrl = '$serverUrl${screenshot['file_path']}';
    final id = screenshot['id'] as int;

    final cameraId = screenshot['camera_id'] as String? ?? '';
    final cameraName = _cameraNameForId(cameraId);
    final createdAt = screenshot['created_at'] as String? ?? '';
    final isSelected = _selectedIds.contains(id);

    return GestureDetector(
      onTap: () {
        if (_selectionMode) {
          _toggleSelection(id);
        } else {
          _showFullScreenDialog(screenshot);
        }
      },
      onLongPress: () {
        if (!_selectionMode) {
          setState(() => _selectionMode = true);
          _toggleSelection(id);
        }
      },
      child: Container(
        decoration: BoxDecoration(
          color: NvrColors.of(context).bgSecondary,
          borderRadius: BorderRadius.circular(8),
          border: Border.all(
            color: isSelected
                ? NvrColors.of(context).accent
                : NvrColors.of(context).border,
            width: isSelected ? 2 : 1,
          ),
        ),
        child: ClipRRect(
          borderRadius: BorderRadius.circular(isSelected ? 6 : 7),
          child: Stack(
            children: [
              Column(
                children: [
                  Expanded(
                    child: FutureBuilder<String?>(
                      future: _getAccessToken(),
                      builder: (context, snap) {
                        final token = snap.data;
                        return Image.network(
                          imageUrl,
                          fit: BoxFit.cover,
                          width: double.infinity,
                          headers: token != null ? _authHeaders(token) : null,
                          errorBuilder: (context, error, stackTrace) => Container(
                            color: NvrColors.of(context).bgTertiary,
                            child: Center(
                              child: Icon(Icons.broken_image_outlined,
                                  color: NvrColors.of(context).textMuted, size: 32),
                            ),
                          ),
                        );
                      },
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
              if (_selectionMode)
                Positioned(
                  top: 6,
                  right: 6,
                  child: Container(
                    decoration: BoxDecoration(
                      color: isSelected
                          ? NvrColors.of(context).accent
                          : NvrColors.of(context).bgPrimary.withValues(alpha: 0.7),
                      shape: BoxShape.circle,
                      border: Border.all(
                        color: isSelected
                            ? NvrColors.of(context).accent
                            : NvrColors.of(context).textMuted,
                        width: 2,
                      ),
                    ),
                    child: isSelected
                        ? const Icon(Icons.check, size: 16, color: Colors.white)
                        : const SizedBox(width: 16, height: 16),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }

  Widget _buildBottomActionBar() {
    return Container(
      padding: const EdgeInsets.symmetric(horizontal: 16, vertical: 10),
      decoration: BoxDecoration(
        color: NvrColors.of(context).bgSecondary,
        border: Border(
          top: BorderSide(color: NvrColors.of(context).border),
        ),
      ),
      child: SafeArea(
        top: false,
        child: Row(
          children: [
            Text(
              '${_selectedIds.length} selected',
              style: NvrTypography.of(context).monoLabel,
            ),
            const Spacer(),
            HudButton(
              style: HudButtonStyle.secondary,
              label: 'DOWNLOAD',
              icon: Icons.download,
              onPressed: _selectedIds.isEmpty ? null : _downloadAllSelected,
            ),
            const SizedBox(width: 6),
            HudButton(
              style: HudButtonStyle.secondary,
              label: 'SHARE',
              icon: Icons.share,
              onPressed: _selectedIds.isEmpty ? null : _shareAllSelected,
            ),
            const SizedBox(width: 6),
            HudButton(
              style: HudButtonStyle.danger,
              label: 'DELETE',
              icon: Icons.delete_outline,
              onPressed: _selectedIds.isEmpty ? null : _deleteAllSelected,
            ),
            const SizedBox(width: 6),
            HudButton(
              style: HudButtonStyle.secondary,
              label: 'CANCEL',
              icon: Icons.close,
              onPressed: _toggleSelectionMode,
            ),
          ],
        ),
      ),
    );
  }

  @override
  Widget build(BuildContext context) {
    final allSelected = _screenshots.isNotEmpty &&
        _screenshots.every((s) => _selectedIds.contains(s['id'] as int));

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
                  if (_selectionMode) ...[
                    IconButton(
                      icon: Icon(
                        allSelected ? Icons.deselect : Icons.select_all,
                        color: NvrColors.of(context).textMuted,
                        size: 20,
                      ),
                      tooltip: allSelected ? 'Deselect All' : 'Select All',
                      onPressed: allSelected ? _deselectAll : _selectAll,
                    ),
                  ],
                  HudButton(
                    style: _selectionMode
                        ? HudButtonStyle.tactical
                        : HudButtonStyle.secondary,
                    label: _selectionMode ? 'SELECTING' : 'SELECT',
                    icon: _selectionMode ? Icons.check_box : Icons.check_box_outline_blank,
                    onPressed: _toggleSelectionMode,
                  ),
                  const SizedBox(width: 8),
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
            if (!_selectionMode && _screenshots.length < _total)
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
            // Selection action bar
            if (_selectionMode) _buildBottomActionBar(),
          ],
        ),
      ),
    );
  }
}
