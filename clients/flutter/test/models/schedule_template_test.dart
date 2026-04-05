import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/schedule_template.dart';

void main() {
  group('ScheduleTemplate', () {
    test('fromJson parses all fields with list days', () {
      final json = {
        'id': 'sched-1',
        'name': 'Weekday Schedule',
        'mode': 'events',
        'days': [1, 2, 3, 4, 5],
        'start_time': '08:00',
        'end_time': '18:00',
        'post_event_seconds': 60,
        'is_default': true,
      };

      final template = ScheduleTemplate.fromJson(json);

      expect(template.id, 'sched-1');
      expect(template.name, 'Weekday Schedule');
      expect(template.mode, 'events');
      expect(template.days, [1, 2, 3, 4, 5]);
      expect(template.startTime, '08:00');
      expect(template.endTime, '18:00');
      expect(template.postEventSeconds, 60);
      expect(template.isDefault, isTrue);
    });

    test('fromJson parses days from string format', () {
      final json = {
        'id': 'sched-2',
        'name': 'Test',
        'mode': 'always',
        'days': '[0,1,2,3,4,5,6]',
        'start_time': '00:00',
        'end_time': '00:00',
      };

      final template = ScheduleTemplate.fromJson(json);

      expect(template.days, [0, 1, 2, 3, 4, 5, 6]);
    });

    test('fromJson handles missing fields with defaults', () {
      final template = ScheduleTemplate.fromJson({});

      expect(template.id, '');
      expect(template.name, '');
      expect(template.mode, 'always');
      expect(template.days, isEmpty);
      expect(template.startTime, '00:00');
      expect(template.endTime, '00:00');
      expect(template.postEventSeconds, 30);
      expect(template.isDefault, isFalse);
    });

    test('fromJson handles null days', () {
      final json = {
        'id': 'sched-3',
        'name': 'Test',
        'mode': 'always',
        'days': null,
      };

      final template = ScheduleTemplate.fromJson(json);

      expect(template.days, isEmpty);
    });

    test('fromJson handles is_default as integer 1', () {
      final json = {
        'id': 'sched-4',
        'name': 'Test',
        'mode': 'always',
        'days': [],
        'is_default': 1,
      };

      final template = ScheduleTemplate.fromJson(json);

      expect(template.isDefault, isTrue);
    });

    test('fromJson handles is_default as integer 0', () {
      final json = {
        'id': 'sched-5',
        'name': 'Test',
        'mode': 'always',
        'days': [],
        'is_default': 0,
      };

      final template = ScheduleTemplate.fromJson(json);

      expect(template.isDefault, isFalse);
    });

    test('fromJson handles malformed days string gracefully', () {
      final json = {
        'id': 'sched-6',
        'name': 'Test',
        'mode': 'always',
        'days': 'not-a-list',
      };

      final template = ScheduleTemplate.fromJson(json);

      expect(template.days, isEmpty);
    });

    test('modeLabel returns Motion for events', () {
      final template = ScheduleTemplate.fromJson({
        'mode': 'events',
        'days': [],
      });

      expect(template.modeLabel, 'Motion');
    });

    test('modeLabel returns Continuous for non-events', () {
      final template = ScheduleTemplate.fromJson({
        'mode': 'always',
        'days': [],
      });

      expect(template.modeLabel, 'Continuous');
    });

    test('daysLabel returns All days for 7 days', () {
      final template = ScheduleTemplate.fromJson({
        'days': [0, 1, 2, 3, 4, 5, 6],
      });

      expect(template.daysLabel, 'All days');
    });

    test('daysLabel returns Mon-Fri for weekdays', () {
      final template = ScheduleTemplate.fromJson({
        'days': [1, 2, 3, 4, 5],
      });

      expect(template.daysLabel, 'Mon-Fri');
    });

    test('daysLabel returns Weekends for Sat/Sun', () {
      final template = ScheduleTemplate.fromJson({
        'days': [0, 6],
      });

      expect(template.daysLabel, 'Weekends');
    });

    test('daysLabel returns individual day names', () {
      final template = ScheduleTemplate.fromJson({
        'days': [1, 3, 5],
      });

      expect(template.daysLabel, 'Mon, Wed, Fri');
    });

    test('timeLabel returns All day when times are 00:00', () {
      final template = ScheduleTemplate.fromJson({
        'start_time': '00:00',
        'end_time': '00:00',
        'days': [],
      });

      expect(template.timeLabel, 'All day');
    });

    test('timeLabel returns range when times differ', () {
      final template = ScheduleTemplate.fromJson({
        'start_time': '08:00',
        'end_time': '18:00',
        'days': [],
      });

      expect(template.timeLabel, '08:00-18:00');
    });

    test('description combines daysLabel and timeLabel', () {
      final template = ScheduleTemplate.fromJson({
        'days': [1, 2, 3, 4, 5],
        'start_time': '08:00',
        'end_time': '18:00',
      });

      expect(template.description, 'Mon-Fri \u2022 08:00-18:00');
    });
  });
}
