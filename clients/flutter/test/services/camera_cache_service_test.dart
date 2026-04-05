import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:nvr_client/services/camera_cache_service.dart';
import 'package:nvr_client/models/camera.dart';

void main() {
  late CameraCacheService service;

  setUp(() {
    SharedPreferences.setMockInitialValues({});
    service = CameraCacheService();
  });

  Camera _makeCamera({
    String id = 'cam-1',
    String name = 'Front Door',
    String status = 'connected',
  }) {
    return Camera(id: id, name: name, status: status);
  }

  group('CameraCacheService', () {
    test('loadCachedCameras returns empty list when nothing is cached', () async {
      final cameras = await service.loadCachedCameras();
      expect(cameras, isEmpty);
    });

    test('cacheCameras then loadCachedCameras round-trips camera data', () async {
      final original = [
        _makeCamera(id: 'cam-1', name: 'Front Door', status: 'connected'),
        _makeCamera(id: 'cam-2', name: 'Backyard', status: 'disconnected'),
      ];

      await service.cacheCameras(original);
      final loaded = await service.loadCachedCameras();

      expect(loaded.length, 2);
      expect(loaded[0].id, 'cam-1');
      expect(loaded[0].name, 'Front Door');
      expect(loaded[0].status, 'connected');
      expect(loaded[1].id, 'cam-2');
      expect(loaded[1].name, 'Backyard');
      expect(loaded[1].status, 'disconnected');
    });

    test('cacheCameras preserves complex camera fields', () async {
      final cam = Camera(
        id: 'cam-3',
        name: 'Garage',
        rtspUrl: 'rtsp://192.168.1.100/stream1',
        mediamtxPath: 'garage',
        ptzCapable: true,
        retentionDays: 14,
        liveViewCodec: 'H265',
      );

      await service.cacheCameras([cam]);
      final loaded = await service.loadCachedCameras();

      expect(loaded.length, 1);
      expect(loaded[0].rtspUrl, 'rtsp://192.168.1.100/stream1');
      expect(loaded[0].mediamtxPath, 'garage');
      expect(loaded[0].ptzCapable, true);
      expect(loaded[0].retentionDays, 14);
      expect(loaded[0].liveViewCodec, 'H265');
    });

    test('lastCachedAt returns null when never cached', () async {
      final ts = await service.lastCachedAt();
      expect(ts, isNull);
    });

    test('lastCachedAt returns a DateTime after caching', () async {
      await service.cacheCameras([_makeCamera()]);
      final ts = await service.lastCachedAt();
      expect(ts, isNotNull);
      // Should be recent (within the last few seconds).
      expect(ts!.difference(DateTime.now()).inSeconds.abs(), lessThan(5));
    });

    test('clearCache removes cached data', () async {
      await service.cacheCameras([_makeCamera()]);
      expect((await service.loadCachedCameras()).length, 1);

      await service.clearCache();

      final cameras = await service.loadCachedCameras();
      expect(cameras, isEmpty);
      expect(await service.lastCachedAt(), isNull);
    });

    test('caching an empty list stores empty list', () async {
      // First cache some cameras.
      await service.cacheCameras([_makeCamera()]);
      expect((await service.loadCachedCameras()).length, 1);

      // Now cache an empty list.
      await service.cacheCameras([]);
      final loaded = await service.loadCachedCameras();
      expect(loaded, isEmpty);
    });
  });
}
