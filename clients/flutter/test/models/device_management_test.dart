import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/device_management.dart';

void main() {
  group('DateTimeInfo', () {
    test('fromJson parses all fields', () {
      final json = {
        'type': 'NTP',
        'daylight_saving': true,
        'timezone': 'America/New_York',
        'utc_time': '2026-04-01T14:00:00Z',
        'local_time': '2026-04-01T10:00:00-04:00',
      };

      final info = DateTimeInfo.fromJson(json);

      expect(info.type, 'NTP');
      expect(info.daylightSaving, isTrue);
      expect(info.timezone, 'America/New_York');
      expect(info.utcTime, '2026-04-01T14:00:00Z');
      expect(info.localTime, '2026-04-01T10:00:00-04:00');
    });

    test('fromJson handles missing fields', () {
      final info = DateTimeInfo.fromJson({});

      expect(info.type, '');
      expect(info.daylightSaving, isFalse);
      expect(info.timezone, '');
      expect(info.utcTime, '');
      expect(info.localTime, '');
    });
  });

  group('HostnameInfo', () {
    test('fromJson parses all fields', () {
      final json = {
        'from_dhcp': true,
        'name': 'camera-front',
      };

      final info = HostnameInfo.fromJson(json);

      expect(info.fromDHCP, isTrue);
      expect(info.name, 'camera-front');
    });

    test('fromJson handles missing fields', () {
      final info = HostnameInfo.fromJson({});

      expect(info.fromDHCP, isFalse);
      expect(info.name, '');
    });
  });

  group('NetworkInterfaceInfo', () {
    test('fromJson parses all fields including nested ipv4', () {
      final json = {
        'token': 'eth0',
        'enabled': true,
        'mac': 'AA:BB:CC:DD:EE:FF',
        'ipv4': {
          'enabled': true,
          'dhcp': false,
          'address': '192.168.1.100',
          'prefix_length': 24,
        },
      };

      final info = NetworkInterfaceInfo.fromJson(json);

      expect(info.token, 'eth0');
      expect(info.enabled, isTrue);
      expect(info.mac, 'AA:BB:CC:DD:EE:FF');
      expect(info.ipv4, isNotNull);
      expect(info.ipv4!.enabled, isTrue);
      expect(info.ipv4!.dhcp, isFalse);
      expect(info.ipv4!.address, '192.168.1.100');
      expect(info.ipv4!.prefix, 24);
    });

    test('fromJson handles missing ipv4', () {
      final json = {
        'token': 'eth0',
        'enabled': true,
        'mac': 'AA:BB:CC:DD:EE:FF',
      };

      final info = NetworkInterfaceInfo.fromJson(json);

      expect(info.ipv4, isNull);
    });

    test('fromJson handles null ipv4', () {
      final json = {
        'token': 'eth0',
        'enabled': true,
        'mac': 'AA:BB:CC:DD:EE:FF',
        'ipv4': null,
      };

      final info = NetworkInterfaceInfo.fromJson(json);

      expect(info.ipv4, isNull);
    });

    test('fromJson handles missing fields', () {
      final info = NetworkInterfaceInfo.fromJson({});

      expect(info.token, '');
      expect(info.enabled, isFalse);
      expect(info.mac, '');
      expect(info.ipv4, isNull);
    });
  });

  group('IPv4Config', () {
    test('fromJson parses all fields', () {
      final json = {
        'enabled': true,
        'dhcp': true,
        'address': '10.0.0.1',
        'prefix_length': 16,
      };

      final config = IPv4Config.fromJson(json);

      expect(config.enabled, isTrue);
      expect(config.dhcp, isTrue);
      expect(config.address, '10.0.0.1');
      expect(config.prefix, 16);
    });

    test('fromJson handles missing fields', () {
      final config = IPv4Config.fromJson({});

      expect(config.enabled, isFalse);
      expect(config.dhcp, isFalse);
      expect(config.address, '');
      expect(config.prefix, 0);
    });
  });

  group('NetworkProtocolInfo', () {
    test('fromJson parses all fields', () {
      final json = {
        'name': 'RTSP',
        'enabled': true,
        'port': 554,
      };

      final info = NetworkProtocolInfo.fromJson(json);

      expect(info.name, 'RTSP');
      expect(info.enabled, isTrue);
      expect(info.port, 554);
    });

    test('fromJson handles missing fields', () {
      final info = NetworkProtocolInfo.fromJson({});

      expect(info.name, '');
      expect(info.enabled, isFalse);
      expect(info.port, 0);
    });

    test('toJson roundtrips correctly', () {
      final info = NetworkProtocolInfo.fromJson({
        'name': 'HTTP',
        'enabled': true,
        'port': 80,
      });

      final json = info.toJson();

      expect(json['name'], 'HTTP');
      expect(json['enabled'], isTrue);
      expect(json['port'], 80);
    });
  });

  group('DeviceUser', () {
    test('fromJson parses all fields', () {
      final json = {
        'username': 'admin',
        'role': 'Administrator',
      };

      final user = DeviceUser.fromJson(json);

      expect(user.username, 'admin');
      expect(user.role, 'Administrator');
    });

    test('fromJson handles missing fields', () {
      final user = DeviceUser.fromJson({});

      expect(user.username, '');
      expect(user.role, '');
    });
  });
}
