import 'package:shared_preferences/shared_preferences.dart';

/// Persists per-camera UI preferences (overlay toggle, audio mute) across
/// tab changes and login sessions.
class CameraPrefs {
  static const _overlayPrefix = 'cam_overlay_';
  static const _audioPrefix = 'cam_audio_';

  static Future<bool> getOverlayEnabled(String cameraId) async {
    final prefs = await SharedPreferences.getInstance();
    return prefs.getBool('$_overlayPrefix$cameraId') ?? true;
  }

  static Future<void> setOverlayEnabled(String cameraId, bool enabled) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setBool('$_overlayPrefix$cameraId', enabled);
  }

  static Future<bool> getAudioEnabled(String cameraId) async {
    final prefs = await SharedPreferences.getInstance();
    return prefs.getBool('$_audioPrefix$cameraId') ?? false;
  }

  static Future<void> setAudioEnabled(String cameraId, bool enabled) async {
    final prefs = await SharedPreferences.getInstance();
    await prefs.setBool('$_audioPrefix$cameraId', enabled);
  }
}
