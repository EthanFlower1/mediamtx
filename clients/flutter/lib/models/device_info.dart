class DeviceInfo {
  final String manufacturer;
  final String model;
  final String firmwareVersion;
  final String serialNumber;
  final String hardwareId;

  const DeviceInfo({
    required this.manufacturer,
    required this.model,
    required this.firmwareVersion,
    required this.serialNumber,
    required this.hardwareId,
  });

  factory DeviceInfo.fromJson(Map<String, dynamic> json) {
    return DeviceInfo(
      manufacturer: json['manufacturer'] as String? ?? '',
      model: json['model'] as String? ?? '',
      firmwareVersion: json['firmware_version'] as String? ?? '',
      serialNumber: json['serial_number'] as String? ?? '',
      hardwareId: json['hardware_id'] as String? ?? '',
    );
  }
}

class ImagingSettings {
  final double brightness;
  final double contrast;
  final double saturation;
  final double sharpness;

  const ImagingSettings({
    required this.brightness,
    required this.contrast,
    required this.saturation,
    required this.sharpness,
  });

  factory ImagingSettings.fromJson(Map<String, dynamic> json) {
    return ImagingSettings(
      brightness: (json['brightness'] as num?)?.toDouble() ?? 0.0,
      contrast: (json['contrast'] as num?)?.toDouble() ?? 0.0,
      saturation: (json['saturation'] as num?)?.toDouble() ?? 0.0,
      sharpness: (json['sharpness'] as num?)?.toDouble() ?? 0.0,
    );
  }

  Map<String, dynamic> toJson() {
    return {
      'brightness': brightness,
      'contrast': contrast,
      'saturation': saturation,
      'sharpness': sharpness,
    };
  }
}

class RelayOutput {
  final String token;
  final String mode;
  final String idleState;
  bool active;

  RelayOutput({
    required this.token,
    required this.mode,
    required this.idleState,
    required this.active,
  });

  factory RelayOutput.fromJson(Map<String, dynamic> json) {
    return RelayOutput(
      token: json['token'] as String? ?? '',
      mode: json['mode'] as String? ?? '',
      idleState: json['idle_state'] as String? ?? '',
      active: json['active'] as bool? ?? false,
    );
  }
}

class PtzPreset {
  final String token;
  final String name;

  const PtzPreset({
    required this.token,
    required this.name,
  });

  factory PtzPreset.fromJson(Map<String, dynamic> json) {
    return PtzPreset(
      token: json['token'] as String? ?? '',
      name: json['name'] as String? ?? '',
    );
  }
}

class AudioCapabilities {
  final bool hasBackchannel;
  final int audioSources;
  final int audioOutputs;

  const AudioCapabilities({
    required this.hasBackchannel,
    required this.audioSources,
    required this.audioOutputs,
  });

  factory AudioCapabilities.fromJson(Map<String, dynamic> json) {
    return AudioCapabilities(
      hasBackchannel: json['has_backchannel'] as bool? ?? false,
      audioSources: json['audio_sources'] as int? ?? 0,
      audioOutputs: json['audio_outputs'] as int? ?? 0,
    );
  }
}
