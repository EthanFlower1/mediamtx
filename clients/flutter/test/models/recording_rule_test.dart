import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/recording_rule.dart';

void main() {
  group('RecordingRule', () {
    test('fromJson parses all fields', () {
      final json = {
        'id': 'rule-1',
        'camera_id': 'cam-1',
        'mode': 'continuous',
        'start_time': '08:00',
        'end_time': '18:00',
        'days_of_week': [1, 2, 3, 4, 5],
        'enabled': true,
        'stream_id': 'stream-main',
      };

      final rule = RecordingRule.fromJson(json);

      expect(rule.id, 'rule-1');
      expect(rule.cameraId, 'cam-1');
      expect(rule.mode, 'continuous');
      expect(rule.startTime, '08:00');
      expect(rule.endTime, '18:00');
      expect(rule.daysOfWeek, [1, 2, 3, 4, 5]);
      expect(rule.enabled, isTrue);
      expect(rule.streamId, 'stream-main');
    });

    test('fromJson handles missing optional fields', () {
      final json = {
        'id': 'rule-2',
        'camera_id': 'cam-2',
        'mode': 'events',
        'enabled': false,
      };

      final rule = RecordingRule.fromJson(json);

      expect(rule.startTime, isNull);
      expect(rule.endTime, isNull);
      expect(rule.daysOfWeek, isNull);
      expect(rule.streamId, '');
    });

    test('fromJson handles null optional fields', () {
      final json = {
        'id': 'rule-3',
        'camera_id': 'cam-3',
        'mode': 'continuous',
        'start_time': null,
        'end_time': null,
        'days_of_week': null,
        'enabled': true,
        'stream_id': null,
      };

      final rule = RecordingRule.fromJson(json);

      expect(rule.startTime, isNull);
      expect(rule.endTime, isNull);
      expect(rule.daysOfWeek, isNull);
      expect(rule.streamId, '');
    });

    test('toJson roundtrips correctly', () {
      final original = RecordingRule.fromJson({
        'id': 'rule-1',
        'camera_id': 'cam-1',
        'mode': 'continuous',
        'start_time': '08:00',
        'end_time': '18:00',
        'days_of_week': [1, 2, 3, 4, 5],
        'enabled': true,
        'stream_id': 'stream-1',
      });

      final json = original.toJson();

      expect(json['id'], 'rule-1');
      expect(json['camera_id'], 'cam-1');
      expect(json['mode'], 'continuous');
      expect(json['start_time'], '08:00');
      expect(json['end_time'], '18:00');
      expect(json['days_of_week'], [1, 2, 3, 4, 5]);
      expect(json['enabled'], isTrue);
      expect(json['stream_id'], 'stream-1');
    });
  });
}
