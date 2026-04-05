import 'package:flutter_test/flutter_test.dart';
import 'package:nvr_client/models/device_info.dart';

void main() {
  group('DeviceInfo', () {
    test('fromJson parses all fields', () {
      final json = {
        'manufacturer': 'Hikvision',
        'model': 'DS-2CD2143G2-I',
        'firmware_version': 'V5.7.1',
        'serial_number': 'DS-2CD2143-20210101ABCD',
        'hardware_id': 'HW-001',
      };

      final info = DeviceInfo.fromJson(json);

      expect(info.manufacturer, 'Hikvision');
      expect(info.model, 'DS-2CD2143G2-I');
      expect(info.firmwareVersion, 'V5.7.1');
      expect(info.serialNumber, 'DS-2CD2143-20210101ABCD');
      expect(info.hardwareId, 'HW-001');
    });

    test('fromJson handles missing fields', () {
      final info = DeviceInfo.fromJson({});

      expect(info.manufacturer, '');
      expect(info.model, '');
      expect(info.firmwareVersion, '');
      expect(info.serialNumber, '');
      expect(info.hardwareId, '');
    });

    test('fromJson handles null values', () {
      final json = {
        'manufacturer': null,
        'model': null,
        'firmware_version': null,
        'serial_number': null,
        'hardware_id': null,
      };

      final info = DeviceInfo.fromJson(json);

      expect(info.manufacturer, '');
      expect(info.model, '');
      expect(info.firmwareVersion, '');
      expect(info.serialNumber, '');
      expect(info.hardwareId, '');
    });
  });

  group('ImagingSettings', () {
    test('fromJson parses all fields', () {
      final json = {
        'brightness': 50.0,
        'contrast': 60.0,
        'saturation': 70.0,
        'sharpness': 80.0,
      };

      final settings = ImagingSettings.fromJson(json);

      expect(settings.brightness, 50.0);
      expect(settings.contrast, 60.0);
      expect(settings.saturation, 70.0);
      expect(settings.sharpness, 80.0);
    });

    test('fromJson handles int values for doubles', () {
      final json = {
        'brightness': 50,
        'contrast': 60,
        'saturation': 70,
        'sharpness': 80,
      };

      final settings = ImagingSettings.fromJson(json);

      expect(settings.brightness, 50.0);
      expect(settings.contrast, 60.0);
    });

    test('fromJson handles missing fields', () {
      final settings = ImagingSettings.fromJson({});

      expect(settings.brightness, 0.0);
      expect(settings.contrast, 0.0);
      expect(settings.saturation, 0.0);
      expect(settings.sharpness, 0.0);
    });

    test('toJson roundtrips correctly', () {
      final settings = ImagingSettings.fromJson({
        'brightness': 50.0,
        'contrast': 60.0,
        'saturation': 70.0,
        'sharpness': 80.0,
      });

      final json = settings.toJson();

      expect(json['brightness'], 50.0);
      expect(json['contrast'], 60.0);
      expect(json['saturation'], 70.0);
      expect(json['sharpness'], 80.0);
    });
  });

  group('RelayOutput', () {
    test('fromJson parses all fields', () {
      final json = {
        'token': 'relay-1',
        'mode': 'bistable',
        'idle_state': 'open',
        'active': true,
      };

      final relay = RelayOutput.fromJson(json);

      expect(relay.token, 'relay-1');
      expect(relay.mode, 'bistable');
      expect(relay.idleState, 'open');
      expect(relay.active, isTrue);
    });

    test('fromJson handles missing fields', () {
      final relay = RelayOutput.fromJson({});

      expect(relay.token, '');
      expect(relay.mode, '');
      expect(relay.idleState, '');
      expect(relay.active, isFalse);
    });
  });

  group('PtzPreset', () {
    test('fromJson parses all fields', () {
      final json = {
        'token': 'preset-1',
        'name': 'Front Gate',
      };

      final preset = PtzPreset.fromJson(json);

      expect(preset.token, 'preset-1');
      expect(preset.name, 'Front Gate');
    });

    test('fromJson handles missing fields', () {
      final preset = PtzPreset.fromJson({});

      expect(preset.token, '');
      expect(preset.name, '');
    });
  });

  group('AudioCapabilities', () {
    test('fromJson parses all fields', () {
      final json = {
        'has_backchannel': true,
        'audio_sources': 2,
        'audio_outputs': 1,
      };

      final caps = AudioCapabilities.fromJson(json);

      expect(caps.hasBackchannel, isTrue);
      expect(caps.audioSources, 2);
      expect(caps.audioOutputs, 1);
    });

    test('fromJson handles missing fields', () {
      final caps = AudioCapabilities.fromJson({});

      expect(caps.hasBackchannel, isFalse);
      expect(caps.audioSources, 0);
      expect(caps.audioOutputs, 0);
    });
  });
}
