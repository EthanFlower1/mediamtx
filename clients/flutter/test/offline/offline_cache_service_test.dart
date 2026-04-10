import 'package:flutter_test/flutter_test.dart';
import 'package:mediamtx/offline/offline_cache_service.dart';

void main() {
  late InMemoryOfflineCacheService cache;

  setUp(() {
    cache = InMemoryOfflineCacheService();
  });

  test('getCachedCameraTree returns null when cache is empty', () {
    expect(cache.getCachedCameraTree(), isNull);
  });

  test('cacheCameraTree stores and retrieves camera tree', () {
    final cameras = [
      {'id': 'cam-1', 'name': 'Front Door'},
      {'id': 'cam-2', 'name': 'Garage'},
    ];
    cache.cacheCameraTree(cameras);
    expect(cache.getCachedCameraTree(), equals(cameras));
  });

  test('getCachedSnapshot returns null for unknown camera', () {
    expect(cache.getCachedSnapshot('unknown'), isNull);
  });

  test('cacheSnapshot stores and retrieves JPEG bytes', () {
    final jpeg = [0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10];
    cache.cacheSnapshot('cam-1', jpeg);
    expect(cache.getCachedSnapshot('cam-1'), equals(jpeg));
  });

  test('lastCacheTime is null initially', () {
    expect(cache.lastCacheTime(), isNull);
  });

  test('lastCacheTime is updated after cacheCameraTree', () {
    final before = DateTime.now();
    cache.cacheCameraTree([
      {'id': 'cam-1'}
    ]);
    final after = DateTime.now();

    final lastTime = cache.lastCacheTime()!;
    expect(lastTime.isAfter(before) || lastTime.isAtSameMomentAs(before),
        isTrue);
    expect(
        lastTime.isBefore(after) || lastTime.isAtSameMomentAs(after), isTrue);
  });

  test('lastCacheTime is updated after cacheSnapshot', () {
    cache.cacheSnapshot('cam-1', [0x01, 0x02]);
    expect(cache.lastCacheTime(), isNotNull);
  });

  test('cacheCameraTree overwrites previous data', () {
    cache.cacheCameraTree([
      {'id': 'cam-1'}
    ]);
    cache.cacheCameraTree([
      {'id': 'cam-2'}
    ]);
    final result = cache.getCachedCameraTree()!;
    expect(result.length, 1);
    expect(result.first['id'], 'cam-2');
  });
}
