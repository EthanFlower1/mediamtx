import 'dart:async';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

import '../models/export_job.dart';
import '../services/export_service.dart';
import 'auth_provider.dart';

/// State for a single export operation including server-side job status
/// and local download progress.
class ExportState {
  final ExportJob? job;
  final double downloadProgress; // 0.0 - 1.0 for local download phase
  final String? localFilePath;
  final String? error;
  final bool isDownloading;

  const ExportState({
    this.job,
    this.downloadProgress = 0,
    this.localFilePath,
    this.error,
    this.isDownloading = false,
  });

  ExportState copyWith({
    ExportJob? job,
    double? downloadProgress,
    String? localFilePath,
    String? error,
    bool? isDownloading,
  }) {
    return ExportState(
      job: job ?? this.job,
      downloadProgress: downloadProgress ?? this.downloadProgress,
      localFilePath: localFilePath ?? this.localFilePath,
      error: error,
      isDownloading: isDownloading ?? this.isDownloading,
    );
  }

  /// Overall progress combining server export (0-80%) and download (80-100%).
  double get overallProgress {
    if (localFilePath != null) return 1.0;
    if (isDownloading) return 0.8 + (downloadProgress * 0.2);
    if (job != null) return (job!.progress / 100.0) * 0.8;
    return 0;
  }

  String get statusLabel {
    if (error != null) return 'Failed';
    if (localFilePath != null) return 'Ready';
    if (isDownloading) return 'Downloading...';
    if (job == null) return 'Starting...';
    switch (job!.status) {
      case 'pending':
        return 'Queued...';
      case 'processing':
        return 'Exporting...';
      case 'completed':
        return 'Download ready';
      case 'failed':
        return 'Export failed';
      case 'cancelled':
        return 'Cancelled';
      default:
        return job!.status;
    }
  }

  bool get isDone => localFilePath != null;
  bool get isFailed => error != null || (job?.isFailed ?? false);
  bool get isActive =>
      !isDone && !isFailed && !(job?.isCancelled ?? false);
}

/// Manages the lifecycle of an export: create job, poll progress,
/// download file, and expose state for the UI.
class ExportNotifier extends StateNotifier<ExportState> {
  final ExportService _service;
  Timer? _pollTimer;
  int _pollErrorCount = 0;
  static const _maxPollRetries = 30; // 30 retries * 2s = 60s max

  ExportNotifier(this._service) : super(const ExportState());

  /// Start an export for the given camera and time range.
  Future<void> startExport({
    required String cameraId,
    required DateTime start,
    required DateTime end,
  }) async {
    try {
      final job = await _service.createExport(
        cameraId: cameraId,
        start: start,
        end: end,
      );
      state = ExportState(job: job);
      _startPolling(job.id);
    } catch (e) {
      state = ExportState(error: 'Failed to start export: $e');
    }
  }

  void _startPolling(String jobId) {
    _pollTimer?.cancel();
    _pollErrorCount = 0;
    _pollTimer = Timer.periodic(const Duration(seconds: 2), (_) async {
      try {
        final job = await _service.getExportStatus(jobId);
        if (!mounted) return;

        _pollErrorCount = 0; // Reset on success.
        state = state.copyWith(job: job, error: null);

        if (job.isCompleted) {
          _pollTimer?.cancel();
          _pollTimer = null;
          _downloadFile(jobId);
        } else if (job.isFailed) {
          _pollTimer?.cancel();
          _pollTimer = null;
          state = state.copyWith(
            error: job.error.isNotEmpty ? job.error : 'Export failed',
          );
        } else if (job.isCancelled) {
          _pollTimer?.cancel();
          _pollTimer = null;
        }
      } catch (e) {
        _pollErrorCount++;
        debugPrint('Export poll error ($_pollErrorCount/$_maxPollRetries): $e');
        if (_pollErrorCount >= _maxPollRetries) {
          _pollTimer?.cancel();
          _pollTimer = null;
          if (mounted) {
            state = state.copyWith(
              error: 'Export status check failed after $_maxPollRetries retries',
            );
          }
        }
      }
    });
  }

  Future<void> _downloadFile(String jobId) async {
    state = state.copyWith(isDownloading: true, downloadProgress: 0);
    try {
      final path = await _service.downloadExport(
        jobId,
        onProgress: (p) {
          if (mounted) {
            state = state.copyWith(downloadProgress: p);
          }
        },
      );
      if (mounted) {
        state = state.copyWith(
          localFilePath: path,
          isDownloading: false,
          downloadProgress: 1.0,
        );
      }
    } catch (e) {
      if (mounted) {
        state = state.copyWith(
          error: 'Download failed: $e',
          isDownloading: false,
        );
      }
    }
  }

  /// Cancel the current export job on the server.
  Future<void> cancel() async {
    _pollTimer?.cancel();
    _pollTimer = null;
    final jobId = state.job?.id;
    if (jobId != null && state.job!.isActive) {
      try {
        await _service.deleteExport(jobId);
      } catch (e) {
        debugPrint('Failed to cancel export: $e');
      }
    }
    state = const ExportState();
  }

  /// Reset state for a new export.
  void reset() {
    _pollTimer?.cancel();
    _pollTimer = null;
    state = const ExportState();
  }

  @override
  void dispose() {
    _pollTimer?.cancel();
    super.dispose();
  }
}

/// Provider for the export service.
final exportServiceProvider = Provider<ExportService?>((ref) {
  final auth = ref.watch(authProvider);
  if (auth.status != AuthStatus.authenticated || auth.serverUrl == null) {
    return null;
  }
  final authService = ref.watch(authServiceProvider);
  return ExportService(
    serverUrl: auth.serverUrl!,
    getAccessToken: () => authService.getAccessToken(),
  );
});

/// Provider for the active export state.
final exportProvider =
    StateNotifierProvider<ExportNotifier, ExportState>((ref) {
  final service = ref.watch(exportServiceProvider);
  // Provide a dummy service that will be replaced when auth is ready.
  // The UI should check for null service before starting exports.
  return ExportNotifier(service ?? ExportService(
    serverUrl: '',
    getAccessToken: () async => null,
  ));
});
