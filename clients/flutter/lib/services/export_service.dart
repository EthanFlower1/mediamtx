import 'dart:io';

import 'package:dio/dio.dart';
import 'package:path_provider/path_provider.dart';

import '../models/export_job.dart';

/// Service for interacting with the backend export API and downloading
/// completed export files to the device.
class ExportService {
  final String serverUrl;
  final Future<String?> Function() getAccessToken;

  ExportService({
    required this.serverUrl,
    required this.getAccessToken,
  });

  Dio _makeDio() {
    return Dio(BaseOptions(
      baseUrl: '$serverUrl/api/nvr',
      connectTimeout: const Duration(seconds: 10),
      receiveTimeout: const Duration(seconds: 30),
      headers: {'Content-Type': 'application/json'},
    ));
  }

  Future<Options> _authOptions() async {
    final token = await getAccessToken();
    if (token != null && token.isNotEmpty) {
      return Options(headers: {'Authorization': 'Bearer $token'});
    }
    return Options();
  }

  /// Create a new export job on the server.
  /// POST /api/nvr/exports
  Future<ExportJob> createExport({
    required String cameraId,
    required DateTime start,
    required DateTime end,
  }) async {
    final dio = _makeDio();
    try {
      final options = await _authOptions();
      final response = await dio.post<Map<String, dynamic>>(
        '/exports',
        data: {
          'camera_id': cameraId,
          'start': start.toUtc().toIso8601String(),
          'end': end.toUtc().toIso8601String(),
        },
        options: options,
      );
      return ExportJob.fromJson(response.data!);
    } finally {
      dio.close();
    }
  }

  /// Poll the status of an export job.
  /// GET /api/nvr/exports/:id
  Future<ExportJob> getExportStatus(String jobId) async {
    final dio = _makeDio();
    try {
      final options = await _authOptions();
      final response = await dio.get<Map<String, dynamic>>(
        '/exports/$jobId',
        options: options,
      );
      return ExportJob.fromJson(response.data!);
    } finally {
      dio.close();
    }
  }

  /// Cancel/delete an export job.
  /// DELETE /api/nvr/exports/:id
  Future<void> deleteExport(String jobId) async {
    final dio = _makeDio();
    try {
      final options = await _authOptions();
      await dio.delete<dynamic>(
        '/exports/$jobId',
        options: options,
      );
    } finally {
      dio.close();
    }
  }

  /// Download the completed export file to local device storage.
  /// Returns the local file path.
  /// [onProgress] reports download progress as a value from 0.0 to 1.0.
  Future<String> downloadExport(
    String jobId, {
    void Function(double progress)? onProgress,
  }) async {
    final dio = Dio(BaseOptions(
      baseUrl: '$serverUrl/api/nvr',
      connectTimeout: const Duration(seconds: 10),
      receiveTimeout: const Duration(minutes: 10),
    ));

    try {
      final token = await getAccessToken();
      final options = Options(
        responseType: ResponseType.bytes,
        headers: {
          if (token != null && token.isNotEmpty)
            'Authorization': 'Bearer $token',
        },
      );

      // Get the download directory.
      final dir = await _getDownloadDirectory();
      final fileName = 'nvr_export_$jobId.mp4';
      final filePath = '${dir.path}/$fileName';

      await dio.download(
        '/exports/$jobId/download',
        filePath,
        options: options,
        onReceiveProgress: (received, total) {
          if (total > 0 && onProgress != null) {
            onProgress(received / total);
          }
        },
      );

      return filePath;
    } finally {
      dio.close();
    }
  }

  /// Get the appropriate download directory for the current platform.
  Future<Directory> _getDownloadDirectory() async {
    if (Platform.isIOS || Platform.isMacOS) {
      // Use the app's documents directory on Apple platforms.
      return getApplicationDocumentsDirectory();
    } else if (Platform.isAndroid) {
      // Use external storage downloads on Android when available.
      final extDir = await getExternalStorageDirectory();
      if (extDir != null) return extDir;
      return getApplicationDocumentsDirectory();
    } else {
      // Desktop fallback — use the downloads directory.
      final downloads = await getDownloadsDirectory();
      if (downloads != null) return downloads;
      return getApplicationDocumentsDirectory();
    }
  }
}
