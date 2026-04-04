import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/export_job.dart';
import 'package:nvr_client/providers/export_provider.dart';

void main() {
  group('ExportState', () {
    test('overallProgress is 0 when no job', () {
      const state = ExportState();
      expect(state.overallProgress, 0);
    });

    test('overallProgress scales server progress to 0-0.8', () {
      final state = ExportState(
        job: ExportJob.fromJson({
          'id': '1',
          'camera_id': 'c',
          'start_time': '2026-04-01T10:00:00Z',
          'end_time': '2026-04-01T10:05:00Z',
          'status': 'processing',
          'progress': 50.0,
        }),
      );
      expect(state.overallProgress, closeTo(0.4, 0.001));
    });

    test('overallProgress during download is 0.8 - 1.0', () {
      final state = ExportState(
        job: ExportJob.fromJson({
          'id': '1',
          'camera_id': 'c',
          'start_time': '2026-04-01T10:00:00Z',
          'end_time': '2026-04-01T10:05:00Z',
          'status': 'completed',
          'progress': 100.0,
        }),
        isDownloading: true,
        downloadProgress: 0.5,
      );
      // 0.8 + 0.5 * 0.2 = 0.9
      expect(state.overallProgress, closeTo(0.9, 0.001));
    });

    test('overallProgress is 1.0 when file is downloaded', () {
      final state = ExportState(
        job: ExportJob.fromJson({
          'id': '1',
          'camera_id': 'c',
          'start_time': '2026-04-01T10:00:00Z',
          'end_time': '2026-04-01T10:05:00Z',
          'status': 'completed',
          'progress': 100.0,
        }),
        localFilePath: '/tmp/test.mp4',
      );
      expect(state.overallProgress, 1.0);
    });

    test('statusLabel reflects state correctly', () {
      expect(const ExportState().statusLabel, 'Starting...');
      expect(
        const ExportState(error: 'oops').statusLabel,
        'Failed',
      );
      expect(
        const ExportState(localFilePath: '/tmp/x.mp4').statusLabel,
        'Ready',
      );
      expect(
        const ExportState(isDownloading: true).statusLabel,
        'Downloading...',
      );
    });

    test('isDone and isFailed flags', () {
      expect(const ExportState().isDone, isFalse);
      expect(const ExportState(localFilePath: '/x').isDone, isTrue);
      expect(const ExportState(error: 'e').isFailed, isTrue);
      expect(const ExportState().isFailed, isFalse);
    });
  });
}
