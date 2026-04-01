import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'auth_provider.dart';
import '../models/device_info.dart';

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
