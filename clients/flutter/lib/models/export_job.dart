/// Model for a server-side clip export job.
///
/// Maps to the backend's ExportJob struct returned by
/// POST /api/nvr/exports and GET /api/nvr/exports/:id.
class ExportJob {
  final String id;
  final String cameraId;
  final String startTime;
  final String endTime;
  final String status; // pending, processing, completed, failed, cancelled
  final double progress;
  final String outputPath;
  final String error;
  final String createdAt;
  final String? completedAt;
  final double? etaSeconds;

  const ExportJob({
    required this.id,
    required this.cameraId,
    required this.startTime,
    required this.endTime,
    required this.status,
    this.progress = 0,
    this.outputPath = '',
    this.error = '',
    this.createdAt = '',
    this.completedAt,
    this.etaSeconds,
  });

  factory ExportJob.fromJson(Map<String, dynamic> json) {
    return ExportJob(
      id: json['id'] as String,
      cameraId: json['camera_id'] as String,
      startTime: json['start_time'] as String,
      endTime: json['end_time'] as String,
      status: json['status'] as String,
      progress: (json['progress'] as num?)?.toDouble() ?? 0,
      outputPath: json['output_path'] as String? ?? '',
      error: json['error'] as String? ?? '',
      createdAt: json['created_at'] as String? ?? '',
      completedAt: json['completed_at'] as String?,
      etaSeconds: (json['eta_seconds'] as num?)?.toDouble(),
    );
  }

  bool get isPending => status == 'pending';
  bool get isProcessing => status == 'processing';
  bool get isCompleted => status == 'completed';
  bool get isFailed => status == 'failed';
  bool get isCancelled => status == 'cancelled';
  bool get isActive => isPending || isProcessing;
}
