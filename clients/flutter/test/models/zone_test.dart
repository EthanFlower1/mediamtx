import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/zone.dart';

void main() {
  group('AlertRule', () {
    test('fromJson parses all fields', () {
      final json = {
        'id': 'rule-1',
        'zone_id': 'zone-1',
        'class_name': 'person',
        'enabled': true,
        'cooldown_seconds': 60,
        'loiter_seconds': 30,
        'notify_on_enter': true,
        'notify_on_leave': true,
        'notify_on_loiter': false,
      };

      final rule = AlertRule.fromJson(json);

      expect(rule.id, 'rule-1');
      expect(rule.zoneId, 'zone-1');
      expect(rule.className, 'person');
      expect(rule.enabled, isTrue);
      expect(rule.cooldownSeconds, 60);
      expect(rule.loiterSeconds, 30);
      expect(rule.notifyOnEnter, isTrue);
      expect(rule.notifyOnLeave, isTrue);
      expect(rule.notifyOnLoiter, isFalse);
    });

    test('fromJson handles missing fields', () {
      final rule = AlertRule.fromJson({});

      expect(rule.id, '');
      expect(rule.zoneId, '');
      expect(rule.className, '');
      expect(rule.enabled, isFalse);
      expect(rule.cooldownSeconds, 0);
      expect(rule.loiterSeconds, 0);
      expect(rule.notifyOnEnter, isFalse);
      expect(rule.notifyOnLeave, isFalse);
      expect(rule.notifyOnLoiter, isFalse);
    });

    test('fromJson handles null values', () {
      final json = {
        'id': null,
        'zone_id': null,
        'class_name': null,
        'enabled': null,
        'cooldown_seconds': null,
        'loiter_seconds': null,
        'notify_on_enter': null,
        'notify_on_leave': null,
        'notify_on_loiter': null,
      };

      final rule = AlertRule.fromJson(json);

      expect(rule.id, '');
      expect(rule.enabled, isFalse);
      expect(rule.cooldownSeconds, 0);
    });

    test('fromJson handles int id via toString', () {
      final json = {
        'id': 42,
        'zone_id': 7,
        'class_name': 'vehicle',
        'enabled': true,
        'cooldown_seconds': 30,
        'loiter_seconds': 10,
        'notify_on_enter': false,
        'notify_on_leave': false,
        'notify_on_loiter': false,
      };

      final rule = AlertRule.fromJson(json);

      expect(rule.id, '42');
      expect(rule.zoneId, '7');
    });

    test('toJson roundtrips correctly', () {
      final original = AlertRule.fromJson({
        'id': 'rule-1',
        'zone_id': 'zone-1',
        'class_name': 'person',
        'enabled': true,
        'cooldown_seconds': 60,
        'loiter_seconds': 30,
        'notify_on_enter': true,
        'notify_on_leave': false,
        'notify_on_loiter': true,
      });

      final json = original.toJson();

      expect(json['id'], 'rule-1');
      expect(json['zone_id'], 'zone-1');
      expect(json['class_name'], 'person');
      expect(json['enabled'], isTrue);
      expect(json['cooldown_seconds'], 60);
      expect(json['loiter_seconds'], 30);
      expect(json['notify_on_enter'], isTrue);
      expect(json['notify_on_leave'], isFalse);
      expect(json['notify_on_loiter'], isTrue);
    });
  });

  group('DetectionZone', () {
    test('fromJson parses all fields with list polygon', () {
      final json = {
        'id': 'zone-1',
        'camera_id': 'cam-1',
        'name': 'Driveway',
        'polygon': [
          [0.1, 0.2],
          [0.3, 0.4],
          [0.5, 0.6],
          [0.1, 0.2],
        ],
        'enabled': true,
        'rules': [
          {
            'id': 'rule-1',
            'zone_id': 'zone-1',
            'class_name': 'person',
            'enabled': true,
            'cooldown_seconds': 60,
            'loiter_seconds': 30,
            'notify_on_enter': true,
            'notify_on_leave': false,
            'notify_on_loiter': false,
          },
        ],
      };

      final zone = DetectionZone.fromJson(json);

      expect(zone.id, 'zone-1');
      expect(zone.cameraId, 'cam-1');
      expect(zone.name, 'Driveway');
      expect(zone.polygon, hasLength(4));
      expect(zone.polygon[0], [0.1, 0.2]);
      expect(zone.polygon[2], [0.5, 0.6]);
      expect(zone.enabled, isTrue);
      expect(zone.rules, hasLength(1));
      expect(zone.rules[0].className, 'person');
    });

    test('fromJson parses polygon from JSON string', () {
      final json = {
        'id': 'zone-2',
        'camera_id': 'cam-2',
        'name': 'Fence',
        'polygon': '[[0.1,0.2],[0.3,0.4],[0.5,0.6]]',
        'enabled': true,
        'rules': [],
      };

      final zone = DetectionZone.fromJson(json);

      expect(zone.polygon, hasLength(3));
      expect(zone.polygon[0], [0.1, 0.2]);
    });

    test('fromJson handles missing fields', () {
      final zone = DetectionZone.fromJson({});

      expect(zone.id, '');
      expect(zone.cameraId, '');
      expect(zone.name, '');
      expect(zone.polygon, isEmpty);
      expect(zone.enabled, isFalse);
      expect(zone.rules, isEmpty);
    });

    test('fromJson handles null polygon', () {
      final json = {
        'id': 'zone-3',
        'camera_id': 'cam-3',
        'name': 'Test',
        'polygon': null,
        'enabled': false,
        'rules': [],
      };

      final zone = DetectionZone.fromJson(json);

      expect(zone.polygon, isEmpty);
    });

    test('fromJson handles malformed polygon string', () {
      final json = {
        'id': 'zone-4',
        'camera_id': 'cam-4',
        'name': 'Test',
        'polygon': 'not-json',
        'enabled': false,
        'rules': [],
      };

      final zone = DetectionZone.fromJson(json);

      expect(zone.polygon, isEmpty);
    });

    test('fromJson handles invalid polygon points', () {
      final json = {
        'id': 'zone-5',
        'camera_id': 'cam-5',
        'name': 'Test',
        'polygon': [
          [0.1, 0.2],     // valid
          [0.3],           // too short, skipped
          'invalid',       // not a list, skipped
          [0.5, 0.6],     // valid
        ],
        'enabled': false,
        'rules': [],
      };

      final zone = DetectionZone.fromJson(json);

      expect(zone.polygon, hasLength(2));
      expect(zone.polygon[0], [0.1, 0.2]);
      expect(zone.polygon[1], [0.5, 0.6]);
    });

    test('fromJson handles null rules', () {
      final json = {
        'id': 'zone-6',
        'camera_id': 'cam-6',
        'name': 'Test',
        'polygon': [],
        'enabled': false,
        'rules': null,
      };

      final zone = DetectionZone.fromJson(json);

      expect(zone.rules, isEmpty);
    });

    test('toJson roundtrips correctly', () {
      final original = DetectionZone.fromJson({
        'id': 'zone-1',
        'camera_id': 'cam-1',
        'name': 'Test Zone',
        'polygon': [
          [0.1, 0.2],
          [0.3, 0.4],
        ],
        'enabled': true,
        'rules': [
          {
            'id': 'rule-1',
            'zone_id': 'zone-1',
            'class_name': 'person',
            'enabled': true,
            'cooldown_seconds': 60,
            'loiter_seconds': 30,
            'notify_on_enter': true,
            'notify_on_leave': false,
            'notify_on_loiter': false,
          },
        ],
      });

      final json = original.toJson();

      expect(json['id'], 'zone-1');
      expect(json['camera_id'], 'cam-1');
      expect(json['name'], 'Test Zone');
      expect(json['polygon'], hasLength(2));
      expect(json['enabled'], isTrue);
      expect(json['rules'], hasLength(1));
    });

    test('fromJson handles int polygon points', () {
      final json = {
        'id': 'zone-7',
        'camera_id': 'cam-7',
        'name': 'Test',
        'polygon': [
          [0, 1],
          [1, 0],
        ],
        'enabled': false,
        'rules': [],
      };

      final zone = DetectionZone.fromJson(json);

      expect(zone.polygon, hasLength(2));
      expect(zone.polygon[0], [0.0, 1.0]);
      expect(zone.polygon[1], [1.0, 0.0]);
    });
  });
}
