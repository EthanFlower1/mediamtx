import 'brand_push_config.dart';

/// Resolves a [BrandPushConfig] from an optional brand configuration map.
///
/// If the map is null, empty, or missing the `push` key the resolver falls
/// back to [BrandPushConfig.defaultConfig].
class PushBrandResolver {
  const PushBrandResolver();

  /// Extract push configuration from [brandConfig].
  ///
  /// Expects the push-specific keys to live under a top-level `"push"` key:
  /// ```json
  /// {
  ///   "push": {
  ///     "fcmProjectId": "...",
  ///     ...
  ///   }
  /// }
  /// ```
  ///
  /// Returns [BrandPushConfig.defaultConfig] when [brandConfig] is null,
  /// empty, or lacks a valid `push` entry.
  BrandPushConfig resolve(Map<String, dynamic>? brandConfig) {
    if (brandConfig == null || brandConfig.isEmpty) {
      return BrandPushConfig.defaultConfig;
    }

    final pushSection = brandConfig['push'];
    if (pushSection == null || pushSection is! Map<String, dynamic>) {
      return BrandPushConfig.defaultConfig;
    }

    return BrandPushConfig.fromJson(pushSection);
  }
}
