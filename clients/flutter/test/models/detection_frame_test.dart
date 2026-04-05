import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/detection_frame.dart';

void main() {
  group('DetectionBox', () {
    test('fromJson parses all fields', () {
      final json = {
        'class': 'person',
        'confidence': 0.95,
        'trackId': 'track-123',
        'x': 0.1,
        'y': 0.2,
        'w': 0.3,
        'h': 0.4,
      };

      final box = DetectionBox.fromJson(json);

      expect(box.className, 'person');
      expect(box.confidence, 0.95);
      expect(box.trackId, 'track-123');
      expect(box.x, 0.1);
      expect(box.y, 0.2);
      expect(box.w, 0.3);
      expect(box.h, 0.4);
    });

    test('fromJson handles missing optional fields', () {
      final json = <String, dynamic>{};

      final box = DetectionBox.fromJson(json);

      expect(box.className, '');
      expect(box.confidence, 0.0);
      expect(box.trackId, isNull);
      expect(box.x, 0.0);
      expect(box.y, 0.0);
      expect(box.w, 0.0);
      expect(box.h, 0.0);
    });

    test('fromJson handles int values for doubles', () {
      final json = {
        'class': 'car',
        'confidence': 1,
        'x': 0,
        'y': 0,
        'w': 1,
        'h': 1,
      };

      final box = DetectionBox.fromJson(json);

      expect(box.confidence, 1.0);
      expect(box.x, 0.0);
      expect(box.w, 1.0);
    });
  });

  group('DetectionFrame', () {
    test('fromJson parses all fields with detections', () {
      final json = {
        'camera': 'front_door',
        'time': '2026-04-01T10:00:00Z',
        'detections': [
          {
            'class': 'person',
            'confidence': 0.9,
            'x': 0.1,
            'y': 0.2,
            'w': 0.3,
            'h': 0.4,
          },
          {
            'class': 'car',
            'confidence': 0.8,
            'x': 0.5,
            'y': 0.6,
            'w': 0.2,
            'h': 0.1,
          },
        ],
      };

      final frame = DetectionFrame.fromJson(json);

      expect(frame.camera, 'front_door');
      expect(frame.time, DateTime.utc(2026, 4, 1, 10));
      expect(frame.detections, hasLength(2));
      expect(frame.detections[0].className, 'person');
      expect(frame.detections[1].className, 'car');
    });

    test('fromJson handles missing fields', () {
      final json = <String, dynamic>{};

      final frame = DetectionFrame.fromJson(json);

      expect(frame.camera, '');
      expect(frame.detections, isEmpty);
    });

    test('fromJson handles null detections', () {
      final json = {
        'camera': 'test',
        'time': '2026-04-01T10:00:00Z',
        'detections': null,
      };

      final frame = DetectionFrame.fromJson(json);

      expect(frame.detections, isEmpty);
    });

    test('fromJson uses DateTime.now when time is null', () {
      final before = DateTime.now();
      final frame = DetectionFrame.fromJson({
        'camera': 'test',
      });
      final after = DateTime.now();

      expect(frame.time.isAfter(before.subtract(const Duration(seconds: 1))),
          isTrue);
      expect(frame.time.isBefore(after.add(const Duration(seconds: 1))),
          isTrue);
    });
  });

  group('PlaybackDetection', () {
    test('fromJson parses all fields', () {
      final json = {
        'frame_time': '2026-04-01T10:00:00Z',
        'class': 'person',
        'confidence': 0.85,
        'box_x': 0.1,
        'box_y': 0.2,
        'box_w': 0.3,
        'box_h': 0.4,
      };

      final det = PlaybackDetection.fromJson(json);

      expect(det.frameTime, DateTime.utc(2026, 4, 1, 10));
      expect(det.className, 'person');
      expect(det.confidence, 0.85);
      expect(det.x, 0.1);
      expect(det.y, 0.2);
      expect(det.w, 0.3);
      expect(det.h, 0.4);
    });

    test('fromJson handles missing optional numeric fields', () {
      final json = {
        'frame_time': '2026-04-01T10:00:00Z',
      };

      final det = PlaybackDetection.fromJson(json);

      expect(det.className, '');
      expect(det.confidence, 0.0);
      expect(det.x, 0.0);
      expect(det.y, 0.0);
      expect(det.w, 0.0);
      expect(det.h, 0.0);
    });

    test('toDetectionBox converts correctly', () {
      final det = PlaybackDetection.fromJson({
        'frame_time': '2026-04-01T10:00:00Z',
        'class': 'vehicle',
        'confidence': 0.9,
        'box_x': 0.1,
        'box_y': 0.2,
        'box_w': 0.3,
        'box_h': 0.4,
      });

      final box = det.toDetectionBox();

      expect(box.className, 'vehicle');
      expect(box.confidence, 0.9);
      expect(box.x, 0.1);
      expect(box.y, 0.2);
      expect(box.w, 0.3);
      expect(box.h, 0.4);
    });
  });
}
