import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'auth_provider.dart';
import '../models/device_info.dart';
import '../models/device_management.dart';
import '../models/media_profile.dart';
import '../models/ptz_status.dart';

final deviceInfoProvider =
    FutureProvider.family<DeviceInfo?, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return null;
  try {
    final res = await api.get('/cameras/$cameraId/device-info');
    return DeviceInfo.fromJson(res.data as Map<String, dynamic>);
  } catch (_) {
    return null;
  }
});

final imagingSettingsProvider =
    FutureProvider.family<ImagingSettings?, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return null;
  try {
    final res = await api.get('/cameras/$cameraId/settings');
    return ImagingSettings.fromJson(res.data as Map<String, dynamic>);
  } catch (_) {
    return null;
  }
});

final relayOutputsProvider =
    FutureProvider.family<List<RelayOutput>, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  try {
    final res = await api.get('/cameras/$cameraId/relay-outputs');
    final data = res.data;
    if (data is List) {
      return data
          .map((e) => RelayOutput.fromJson(e as Map<String, dynamic>))
          .toList();
    }
    return [];
  } catch (_) {
    return [];
  }
});

final ptzPresetsProvider =
    FutureProvider.family<List<PtzPreset>, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  try {
    final res = await api.get('/cameras/$cameraId/ptz/presets');
    final data = res.data;
    if (data is List) {
      return data
          .map((e) => PtzPreset.fromJson(e as Map<String, dynamic>))
          .toList();
    }
    return [];
  } catch (_) {
    return [];
  }
});

final audioCapabilitiesProvider =
    FutureProvider.family<AudioCapabilities?, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return null;
  try {
    final res = await api.get('/cameras/$cameraId/audio/capabilities');
    return AudioCapabilities.fromJson(res.data as Map<String, dynamic>);
  } catch (_) {
    return null;
  }
});

final ptzStatusProvider =
    FutureProvider.family<PtzStatus?, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return null;
  try {
    final res = await api.get('/cameras/$cameraId/ptz/status');
    return PtzStatus.fromJson(res.data as Map<String, dynamic>);
  } catch (_) {
    return null;
  }
});

final mediaProfilesProvider =
    FutureProvider.family<List<ProfileInfo>, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  try {
    final res = await api.get('/cameras/$cameraId/media/profiles');
    final data = res.data;
    if (data is List) {
      return data
          .map((e) => ProfileInfo.fromJson(e as Map<String, dynamic>))
          .toList();
    }
    return [];
  } catch (_) {
    return [];
  }
});

final videoSourcesProvider =
    FutureProvider.family<List<VideoSourceInfo>, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  try {
    final res = await api.get('/cameras/$cameraId/media/video-sources');
    final data = res.data;
    if (data is List) {
      return data
          .map((e) => VideoSourceInfo.fromJson(e as Map<String, dynamic>))
          .toList();
    }
    return [];
  } catch (_) {
    return [];
  }
});

final deviceDateTimeProvider =
    FutureProvider.family<DateTimeInfo?, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return null;
  try {
    final res = await api.get('/cameras/$cameraId/device/datetime');
    return DateTimeInfo.fromJson(res.data as Map<String, dynamic>);
  } catch (_) {
    return null;
  }
});

final deviceHostnameProvider =
    FutureProvider.family<HostnameInfo?, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return null;
  try {
    final res = await api.get('/cameras/$cameraId/device/hostname');
    return HostnameInfo.fromJson(res.data as Map<String, dynamic>);
  } catch (_) {
    return null;
  }
});

final networkInterfacesProvider =
    FutureProvider.family<List<NetworkInterfaceInfo>, String>(
        (ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  try {
    final res = await api.get('/cameras/$cameraId/device/network/interfaces');
    final data = res.data;
    if (data is List) {
      return data
          .map((e) => NetworkInterfaceInfo.fromJson(e as Map<String, dynamic>))
          .toList();
    }
    return [];
  } catch (_) {
    return [];
  }
});

final networkProtocolsProvider =
    FutureProvider.family<List<NetworkProtocolInfo>, String>(
        (ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  try {
    final res = await api.get('/cameras/$cameraId/device/network/protocols');
    final data = res.data;
    if (data is List) {
      return data
          .map((e) => NetworkProtocolInfo.fromJson(e as Map<String, dynamic>))
          .toList();
    }
    return [];
  } catch (_) {
    return [];
  }
});

final deviceUsersProvider =
    FutureProvider.family<List<DeviceUser>, String>((ref, cameraId) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return [];
  try {
    final res = await api.get('/cameras/$cameraId/device/users');
    final data = res.data;
    if (data is List) {
      return data
          .map((e) => DeviceUser.fromJson(e as Map<String, dynamic>))
          .toList();
    }
    return [];
  } catch (_) {
    return [];
  }
});

typedef EncoderOptionsKey = ({String cameraId, String configToken});

final videoEncoderOptionsProvider =
    FutureProvider.family<VideoEncoderOptions?, EncoderOptionsKey>((ref, key) async {
  final api = ref.watch(apiClientProvider);
  if (api == null) return null;
  try {
    final res = await api.get(
        '/cameras/${key.cameraId}/media/video-encoder/${key.configToken}/options');
    return VideoEncoderOptions.fromJson(res.data as Map<String, dynamic>);
  } catch (_) {
    return null;
  }
});
