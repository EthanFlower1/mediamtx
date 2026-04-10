/// Per-brand push notification configuration for white-label builds.
///
/// Each brand may supply its own FCM / APNs / Web Push credentials so that
/// notifications are routed through the brand's own project rather than the
/// default development project.
class BrandPushConfig {
  const BrandPushConfig({
    this.fcmProjectId,
    this.fcmSenderId,
    this.apnsTeamId,
    this.apnsKeyId,
    this.apnsBundleId,
    this.webVapidPublicKey,
  });

  /// Firebase Cloud Messaging project identifier.
  final String? fcmProjectId;

  /// Firebase Cloud Messaging sender identifier.
  final String? fcmSenderId;

  /// Apple Push Notification service team identifier.
  final String? apnsTeamId;

  /// Apple Push Notification service key identifier.
  final String? apnsKeyId;

  /// Apple Push Notification service bundle identifier.
  final String? apnsBundleId;

  /// Web Push VAPID public key (base64-url encoded).
  final String? webVapidPublicKey;

  /// Placeholder development configuration used when no brand-specific
  /// configuration is provided.
  static const defaultConfig = BrandPushConfig(
    fcmProjectId: 'dev-project-placeholder',
    fcmSenderId: '000000000000',
    apnsTeamId: 'DEV_TEAM_ID',
    apnsKeyId: 'DEV_KEY_ID',
    apnsBundleId: 'com.example.dev',
    webVapidPublicKey: 'DEV_VAPID_PUBLIC_KEY',
  );

  /// Deserialise from a JSON map (e.g. fetched from a branding endpoint).
  factory BrandPushConfig.fromJson(Map<String, dynamic> json) {
    return BrandPushConfig(
      fcmProjectId: json['fcmProjectId'] as String?,
      fcmSenderId: json['fcmSenderId'] as String?,
      apnsTeamId: json['apnsTeamId'] as String?,
      apnsKeyId: json['apnsKeyId'] as String?,
      apnsBundleId: json['apnsBundleId'] as String?,
      webVapidPublicKey: json['webVapidPublicKey'] as String?,
    );
  }

  /// Serialise to a JSON-compatible map.
  Map<String, dynamic> toJson() {
    return {
      'fcmProjectId': fcmProjectId,
      'fcmSenderId': fcmSenderId,
      'apnsTeamId': apnsTeamId,
      'apnsKeyId': apnsKeyId,
      'apnsBundleId': apnsBundleId,
      'webVapidPublicKey': webVapidPublicKey,
    };
  }

  @override
  bool operator ==(Object other) {
    if (identical(this, other)) return true;
    return other is BrandPushConfig &&
        other.fcmProjectId == fcmProjectId &&
        other.fcmSenderId == fcmSenderId &&
        other.apnsTeamId == apnsTeamId &&
        other.apnsKeyId == apnsKeyId &&
        other.apnsBundleId == apnsBundleId &&
        other.webVapidPublicKey == webVapidPublicKey;
  }

  @override
  int get hashCode => Object.hash(
        fcmProjectId,
        fcmSenderId,
        apnsTeamId,
        apnsKeyId,
        apnsBundleId,
        webVapidPublicKey,
      );

  @override
  String toString() =>
      'BrandPushConfig(fcm=$fcmProjectId/$fcmSenderId, '
      'apns=$apnsTeamId/$apnsKeyId/$apnsBundleId, '
      'vapid=$webVapidPublicKey)';
}
