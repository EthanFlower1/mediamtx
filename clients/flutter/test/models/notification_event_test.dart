import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/notification_event.dart';

void main() {
  group('NotificationEvent', () {
    test('fromJson parses all fields', () {
      final json = {
        'id': 'evt-1',
        'type': 'motion',
        'camera': 'front_door',
        'message': 'Motion detected',
        'time': '2026-04-01T10:00:00Z',
        'zone': 'driveway',
        'class': 'person',
        'action': 'enter',
        'trackId': 'track-1',
        'confidence': 0.95,
        'isRead': true,
      };

      final event = NotificationEvent.fromJson(json);

      expect(event.id, 'evt-1');
      expect(event.type, 'motion');
      expect(event.camera, 'front_door');
      expect(event.message, 'Motion detected');
      expect(event.time, DateTime.utc(2026, 4, 1, 10));
      expect(event.zone, 'driveway');
      expect(event.className, 'person');
      expect(event.action, 'enter');
      expect(event.trackId, 'track-1');
      expect(event.confidence, 0.95);
      expect(event.isRead, isTrue);
    });

    test('fromJson handles missing optional fields', () {
      final json = {
        'id': 'evt-2',
        'type': 'camera_offline',
        'camera': 'back_yard',
        'message': 'Camera went offline',
        'time': '2026-04-01T10:00:00Z',
      };

      final event = NotificationEvent.fromJson(json);

      expect(event.zone, isNull);
      expect(event.className, isNull);
      expect(event.action, isNull);
      expect(event.trackId, isNull);
      expect(event.confidence, isNull);
      expect(event.isRead, isFalse);
    });

    test('fromJson generates id when missing', () {
      final json = {
        'type': 'motion',
        'camera': 'cam-1',
        'message': 'Test',
        'time': '2026-04-01T10:00:00Z',
      };

      final event = NotificationEvent.fromJson(json);

      expect(event.id, contains('motion'));
      expect(event.id, contains('cam-1'));
    });

    test('fromJson handles missing fields gracefully', () {
      final event = NotificationEvent.fromJson({});

      expect(event.type, '');
      expect(event.camera, '');
      expect(event.message, '');
      expect(event.isRead, isFalse);
    });

    test('fromJson handles int confidence', () {
      final json = {
        'id': 'evt-3',
        'type': 'detection_frame',
        'camera': 'cam-1',
        'message': 'Detected',
        'time': '2026-04-01T10:00:00Z',
        'confidence': 1,
      };

      final event = NotificationEvent.fromJson(json);

      expect(event.confidence, 1.0);
    });

    test('copyWith overrides isRead', () {
      final event = NotificationEvent.fromJson({
        'id': 'evt-1',
        'type': 'motion',
        'camera': 'cam-1',
        'message': 'Test',
        'time': '2026-04-01T10:00:00Z',
      });

      expect(event.isRead, isFalse);

      final read = event.copyWith(isRead: true);

      expect(read.isRead, isTrue);
      expect(read.id, 'evt-1');
      expect(read.type, 'motion');
    });

    test('isDetectionFrame returns true for detection_frame type', () {
      final event = _makeEvent(type: 'detection_frame');
      expect(event.isDetectionFrame, isTrue);
      expect(event.isAlert, isFalse);
    });

    test('isAlert returns true for alert type', () {
      final event = _makeEvent(type: 'alert');
      expect(event.isAlert, isTrue);
      expect(event.isDetectionFrame, isFalse);
    });

    group('typeIcon', () {
      test('motion returns directions_run', () {
        expect(_makeEvent(type: 'motion').typeIcon, Icons.directions_run);
      });

      test('camera_offline returns videocam_off', () {
        expect(
            _makeEvent(type: 'camera_offline').typeIcon, Icons.videocam_off);
      });

      test('camera_online returns videocam', () {
        expect(_makeEvent(type: 'camera_online').typeIcon, Icons.videocam);
      });

      test('alert returns warning_amber', () {
        expect(_makeEvent(type: 'alert').typeIcon, Icons.warning_amber);
      });

      test('detection_frame returns center_focus_strong', () {
        expect(_makeEvent(type: 'detection_frame').typeIcon,
            Icons.center_focus_strong);
      });

      test('recording_started returns fiber_manual_record', () {
        expect(_makeEvent(type: 'recording_started').typeIcon,
            Icons.fiber_manual_record);
      });

      test('recording_stopped returns stop_circle_outlined', () {
        expect(_makeEvent(type: 'recording_stopped').typeIcon,
            Icons.stop_circle_outlined);
      });

      test('unknown type returns notifications_outlined', () {
        expect(_makeEvent(type: 'unknown').typeIcon,
            Icons.notifications_outlined);
      });
    });

    group('navigationRoute', () {
      test('camera_offline navigates to device page', () {
        final event = _makeEvent(type: 'camera_offline', camera: 'cam-1');
        expect(event.navigationRoute, '/devices/cam-1');
      });

      test('camera_online navigates to device page', () {
        final event = _makeEvent(type: 'camera_online', camera: 'cam-2');
        expect(event.navigationRoute, '/devices/cam-2');
      });

      test('motion navigates to playback with timestamp', () {
        final event = _makeEvent(type: 'motion', camera: 'cam-1');
        expect(event.navigationRoute, contains('/playback'));
        expect(event.navigationRoute, contains('cameraId=cam-1'));
        expect(event.navigationRoute, contains('timestamp='));
      });

      test('detection_frame navigates to playback', () {
        final event = _makeEvent(type: 'detection_frame', camera: 'cam-1');
        expect(event.navigationRoute, contains('/playback'));
      });

      test('alert navigates to playback', () {
        final event = _makeEvent(type: 'alert', camera: 'cam-1');
        expect(event.navigationRoute, contains('/playback'));
      });

      test('unknown type returns null', () {
        final event = _makeEvent(type: 'unknown');
        expect(event.navigationRoute, isNull);
      });

      test('recording_started returns null', () {
        final event = _makeEvent(type: 'recording_started');
        expect(event.navigationRoute, isNull);
      });
    });
  });
}

NotificationEvent _makeEvent({
  required String type,
  String camera = 'test-cam',
}) {
  return NotificationEvent.fromJson({
    'id': 'test-id',
    'type': type,
    'camera': camera,
    'message': 'Test message',
    'time': '2026-04-01T10:00:00Z',
  });
}
