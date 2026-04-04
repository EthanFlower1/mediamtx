import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/export_job.dart';

void main() {
  group('ExportJob', () {
    test('fromJson parses all fields', () {
      final json = {
        'id': 'abc-123',
        'camera_id': 'cam-1',
        'start_time': '2026-04-01T10:00:00Z',
        'end_time': '2026-04-01T10:05:00Z',
        'status': 'processing',
        'progress': 42.5,
        'output_path': '/exports/clip.mp4',
        'error': '',
        'created_at': '2026-04-01T09:59:00Z',
        'completed_at': null,
        'eta_seconds': 12.3,
      };

      final job = ExportJob.fromJson(json);

      expect(job.id, 'abc-123');
      expect(job.cameraId, 'cam-1');
      expect(job.startTime, '2026-04-01T10:00:00Z');
      expect(job.endTime, '2026-04-01T10:05:00Z');
      expect(job.status, 'processing');
      expect(job.progress, 42.5);
      expect(job.outputPath, '/exports/clip.mp4');
      expect(job.error, '');
      expect(job.createdAt, '2026-04-01T09:59:00Z');
      expect(job.completedAt, isNull);
      expect(job.etaSeconds, 12.3);
    });

    test('fromJson handles missing optional fields', () {
      final json = {
        'id': 'abc-123',
        'camera_id': 'cam-1',
        'start_time': '2026-04-01T10:00:00Z',
        'end_time': '2026-04-01T10:05:00Z',
        'status': 'pending',
      };

      final job = ExportJob.fromJson(json);

      expect(job.progress, 0);
      expect(job.outputPath, '');
      expect(job.error, '');
      expect(job.etaSeconds, isNull);
    });

    test('status convenience getters', () {
      expect(
        ExportJob.fromJson(_jobJson(status: 'pending')).isPending,
        isTrue,
      );
      expect(
        ExportJob.fromJson(_jobJson(status: 'processing')).isProcessing,
        isTrue,
      );
      expect(
        ExportJob.fromJson(_jobJson(status: 'completed')).isCompleted,
        isTrue,
      );
      expect(
        ExportJob.fromJson(_jobJson(status: 'failed')).isFailed,
        isTrue,
      );
      expect(
        ExportJob.fromJson(_jobJson(status: 'cancelled')).isCancelled,
        isTrue,
      );
    });

    test('isActive is true for pending and processing', () {
      expect(
        ExportJob.fromJson(_jobJson(status: 'pending')).isActive,
        isTrue,
      );
      expect(
        ExportJob.fromJson(_jobJson(status: 'processing')).isActive,
        isTrue,
      );
      expect(
        ExportJob.fromJson(_jobJson(status: 'completed')).isActive,
        isFalse,
      );
      expect(
        ExportJob.fromJson(_jobJson(status: 'failed')).isActive,
        isFalse,
      );
    });
  });
}

Map<String, dynamic> _jobJson({required String status}) => {
      'id': 'test-id',
      'camera_id': 'cam-1',
      'start_time': '2026-04-01T10:00:00Z',
      'end_time': '2026-04-01T10:05:00Z',
      'status': status,
    };
