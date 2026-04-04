import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import '../../../models/camera.dart';
import '../../../providers/auth_provider.dart';
import '../../../theme/nvr_colors.dart';

class StorageTab extends ConsumerStatefulWidget {
  final Camera camera;
  final VoidCallback onRefresh;

  const StorageTab({super.key, required this.camera, required this.onRefresh});

  @override
  ConsumerState<StorageTab> createState() => _StorageTabState();
}

class _StorageTabState extends ConsumerState<StorageTab> {
  late TextEditingController _storagePathController;
  bool _saving = false;

  @override
  void initState() {
    super.initState();
    _storagePathController =
        TextEditingController(text: widget.camera.storagePath);
  }

  @override
  void dispose() {
    _storagePathController.dispose();
    super.dispose();
  }

  Color _statusColor(String status) {
    switch (status) {
      case 'healthy':
        return Colors.green;
      case 'degraded':
        return Colors.amber;
      default:
        return NvrColors.of(context).textMuted;
    }
  }

  String _statusLabel(String status) {
    switch (status) {
      case 'healthy':
        return 'Healthy';
      case 'degraded':
        return 'Degraded';
      default:
        return 'Default';
    }
  }

  Future<void> _save() async {
    final api = ref.read(apiClientProvider);
    if (api == null) return;
    setState(() => _saving = true);
    try {
      await api.put('/cameras/${widget.camera.id}', data: {
        'name': widget.camera.name,
        'rtsp_url': widget.camera.rtspUrl,
        'storage_path': _storagePathController.text.trim(),
      });
      widget.onRefresh();
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.of(context).success,
            content: Text('Storage path updated'),
          ),
        );
      }
    } catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            backgroundColor: NvrColors.of(context).danger,
            content: Text('Failed to update: $e'),
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _saving = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    final status = widget.camera.storageStatus;
    return ListView(
      padding: const EdgeInsets.all(16),
      children: [
        Row(
          children: [
            Text(
              'Status: ',
              style: TextStyle(
                fontWeight: FontWeight.bold,
                color: NvrColors.of(context).textPrimary,
              ),
            ),
            Chip(
              label: Text(
                _statusLabel(status),
                style: TextStyle(color: _statusColor(status)),
              ),
              backgroundColor: _statusColor(status).withOpacity(0.15),
              side: BorderSide(color: _statusColor(status)),
            ),
          ],
        ),
        const SizedBox(height: 16),
        TextFormField(
          controller: _storagePathController,
          style: TextStyle(color: NvrColors.of(context).textPrimary),
          decoration: InputDecoration(
            labelText: 'Storage Path',
            hintText: 'Leave empty to use default local storage',
            helperText:
                'Absolute path to recording storage (e.g., /mnt/nas/recordings)',
            labelStyle: TextStyle(color: NvrColors.of(context).textMuted),
            hintStyle: TextStyle(color: NvrColors.of(context).textMuted),
            helperStyle: TextStyle(color: NvrColors.of(context).textSecondary),
            filled: true,
            fillColor: NvrColors.of(context).bgInput,
            border: OutlineInputBorder(
              borderRadius: BorderRadius.circular(8),
              borderSide: BorderSide(color: NvrColors.of(context).border),
            ),
            enabledBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(8),
              borderSide: BorderSide(color: NvrColors.of(context).border),
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: BorderRadius.circular(8),
              borderSide: BorderSide(color: NvrColors.of(context).accent),
            ),
          ),
        ),
        const SizedBox(height: 24),
        ElevatedButton(
          style: ElevatedButton.styleFrom(
            backgroundColor: NvrColors.of(context).accent,
            foregroundColor: Colors.white,
            padding: const EdgeInsets.symmetric(vertical: 14),
          ),
          onPressed: _saving ? null : _save,
          child: _saving
              ? const SizedBox(
                  height: 18,
                  width: 18,
                  child: CircularProgressIndicator(
                      strokeWidth: 2, color: Colors.white),
                )
              : const Text('Save'),
        ),
      ],
    );
  }
}
