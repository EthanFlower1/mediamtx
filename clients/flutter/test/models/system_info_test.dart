import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/providers/settings_provider.dart';

void main() {
  group('SystemInfo.fromJson — Go duration parsing', () {
    test('parses "336h12m5.123s" correctly', () {
      final info = SystemInfo.fromJson({
        'uptime': '336h12m5.123s',
      });
      // 336*3600 + 12*60 + 5 (rounded from 5.123)
      expect(info.uptimeSeconds, 336 * 3600 + 12 * 60 + 5);
    });

    test('parses "45m30s" correctly', () {
      final info = SystemInfo.fromJson({
        'uptime': '45m30s',
      });
      expect(info.uptimeSeconds, 45 * 60 + 30);
    });

    test('parses "5s" correctly', () {
      final info = SystemInfo.fromJson({
        'uptime': '5s',
      });
      expect(info.uptimeSeconds, 5);
    });

    test('parses "0s" as 0', () {
      final info = SystemInfo.fromJson({
        'uptime': '0s',
      });
      expect(info.uptimeSeconds, 0);
    });

    test('empty string yields 0', () {
      final info = SystemInfo.fromJson({
        'uptime': '',
      });
      expect(info.uptimeSeconds, 0);
    });

    test('integer value (legacy) works', () {
      final info = SystemInfo.fromJson({
        'uptime': 7200,
      });
      expect(info.uptimeSeconds, 7200);
    });

    test('double value rounds correctly', () {
      final info = SystemInfo.fromJson({
        'uptime': 3600.7,
      });
      expect(info.uptimeSeconds, 3601);
    });

    test('uptime_seconds field overrides uptime string', () {
      final info = SystemInfo.fromJson({
        'uptime': '1h0m0s',
        'uptime_seconds': 9999,
      });
      expect(info.uptimeSeconds, 9999);
    });

    test('uptime_seconds zero does not override uptime string', () {
      final info = SystemInfo.fromJson({
        'uptime': '2h0m0s',
        'uptime_seconds': 0,
      });
      // 0 is not > 0, so the string value (7200) should remain
      expect(info.uptimeSeconds, 7200);
    });
  });

  group('SystemInfo.uptimeFormatted', () {
    test('returns "--" for 0 seconds', () {
      const info = SystemInfo(
        version: '',
        platform: '',
        uptimeSeconds: 0,
        clipSearchAvailable: false,
      );
      expect(info.uptimeFormatted, '--');
    });

    test('returns minutes for < 1 hour', () {
      const info = SystemInfo(
        version: '',
        platform: '',
        uptimeSeconds: 2700, // 45 minutes
        clipSearchAvailable: false,
      );
      expect(info.uptimeFormatted, '45m');
    });

    test('returns hours and minutes for < 1 day', () {
      const info = SystemInfo(
        version: '',
        platform: '',
        uptimeSeconds: 12600, // 3h 30m
        clipSearchAvailable: false,
      );
      expect(info.uptimeFormatted, '3h 30m');
    });

    test('returns days and hours for >= 1 day', () {
      const info = SystemInfo(
        version: '',
        platform: '',
        uptimeSeconds: 14 * 86400 + 8 * 3600, // 14d 8h
        clipSearchAvailable: false,
      );
      expect(info.uptimeFormatted, '14d 8h');
    });
  });

  group('SystemInfo.fromJson — other fields', () {
    test('parses version, platform, clipSearchAvailable', () {
      final info = SystemInfo.fromJson({
        'version': '1.2.3',
        'platform': 'linux/amd64',
        'uptime': '1h0m0s',
        'clip_search_available': true,
      });
      expect(info.version, '1.2.3');
      expect(info.platform, 'linux/amd64');
      expect(info.clipSearchAvailable, true);
    });

    test('defaults missing fields gracefully', () {
      final info = SystemInfo.fromJson({});
      expect(info.version, '');
      expect(info.platform, '');
      expect(info.uptimeSeconds, 0);
      expect(info.clipSearchAvailable, false);
    });
  });
}
