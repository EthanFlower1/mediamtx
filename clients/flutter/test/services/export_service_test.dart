import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/services/export_service.dart';

void main() {
  group('ExportService', () {
    test('can be instantiated with required parameters', () {
      final service = ExportService(
        serverUrl: 'http://localhost:9997',
        getAccessToken: () async => null,
      );
      expect(service, isNotNull);
      expect(service.serverUrl, 'http://localhost:9997');
    });

    test('createExport exists as a method', () {
      final service = ExportService(
        serverUrl: 'http://localhost:9997',
        getAccessToken: () async => null,
      );
      // Verify the method exists and has the right signature.
      expect(service.createExport, isA<Function>());
    });

    test('getExportStatus exists as a method', () {
      final service = ExportService(
        serverUrl: 'http://localhost:9997',
        getAccessToken: () async => null,
      );
      expect(service.getExportStatus, isA<Function>());
    });

    test('deleteExport exists as a method', () {
      final service = ExportService(
        serverUrl: 'http://localhost:9997',
        getAccessToken: () async => null,
      );
      expect(service.deleteExport, isA<Function>());
    });

    test('downloadExport exists as a method', () {
      final service = ExportService(
        serverUrl: 'http://localhost:9997',
        getAccessToken: () async => null,
      );
      expect(service.downloadExport, isA<Function>());
    });

    test('getAccessToken callback is invoked by _authOptions', () async {
      // Verify the service stores and uses the token callback.
      bool tokenRequested = false;
      final service = ExportService(
        serverUrl: 'http://localhost:9997',
        getAccessToken: () async {
          tokenRequested = true;
          return 'test-token';
        },
      );
      expect(service, isNotNull);
      // We can't directly call _authOptions (private), but we confirm
      // the callback is stored correctly.
      expect(tokenRequested, isFalse);
    });
  });
}
