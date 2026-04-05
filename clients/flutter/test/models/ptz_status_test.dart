import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/ptz_status.dart';

void main() {
  group('PtzStatus', () {
    test('fromJson parses all fields', () {
      final json = {
        'pan_position': 0.5,
        'tilt_position': -0.3,
        'zoom_position': 0.8,
        'is_moving': true,
      };

      final status = PtzStatus.fromJson(json);

      expect(status.panPosition, 0.5);
      expect(status.tiltPosition, -0.3);
      expect(status.zoomPosition, 0.8);
      expect(status.isMoving, isTrue);
    });

    test('fromJson handles missing fields', () {
      final status = PtzStatus.fromJson({});

      expect(status.panPosition, 0.0);
      expect(status.tiltPosition, 0.0);
      expect(status.zoomPosition, 0.0);
      expect(status.isMoving, isFalse);
    });

    test('fromJson handles null values', () {
      final json = {
        'pan_position': null,
        'tilt_position': null,
        'zoom_position': null,
        'is_moving': null,
      };

      final status = PtzStatus.fromJson(json);

      expect(status.panPosition, 0.0);
      expect(status.tiltPosition, 0.0);
      expect(status.zoomPosition, 0.0);
      expect(status.isMoving, isFalse);
    });

    test('fromJson handles int values for doubles', () {
      final json = {
        'pan_position': 1,
        'tilt_position': 0,
        'zoom_position': 2,
        'is_moving': false,
      };

      final status = PtzStatus.fromJson(json);

      expect(status.panPosition, 1.0);
      expect(status.tiltPosition, 0.0);
      expect(status.zoomPosition, 2.0);
    });
  });
}
