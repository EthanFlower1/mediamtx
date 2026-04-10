// GENERATED — placeholder until buf generate runs. See README.md.
// Source: kaivue/v1/auth.proto

/// TenantType distinguishes integrator vs customer tenants.
enum PbTenantType {
  unspecified,
  integrator,
  customer,
}

/// TenantRef identifies a tenant.
class PbTenantRef {
  final PbTenantType type;
  final String id;

  const PbTenantRef({this.type = PbTenantType.unspecified, this.id = ''});

  Map<String, dynamic> toJson() => {
        'type': type.index,
        'id': id,
      };

  factory PbTenantRef.fromJson(Map<String, dynamic> json) => PbTenantRef(
        type: PbTenantType.values[json['type'] as int? ?? 0],
        id: json['id'] as String? ?? '',
      );
}

/// Session — result of authentication or refresh.
class PbSession {
  final String id;
  final String userId;
  final PbTenantRef tenant;
  final String accessToken;
  final String refreshToken;
  final String idToken;
  final DateTime? issuedAt;
  final DateTime? expiresAt;

  const PbSession({
    this.id = '',
    this.userId = '',
    this.tenant = const PbTenantRef(),
    this.accessToken = '',
    this.refreshToken = '',
    this.idToken = '',
    this.issuedAt,
    this.expiresAt,
  });

  Map<String, dynamic> toJson() => {
        'id': id,
        'user_id': userId,
        'tenant': tenant.toJson(),
        'access_token': accessToken,
        'refresh_token': refreshToken,
        'id_token': idToken,
        if (issuedAt != null) 'issued_at': issuedAt!.toIso8601String(),
        if (expiresAt != null) 'expires_at': expiresAt!.toIso8601String(),
      };

  factory PbSession.fromJson(Map<String, dynamic> json) => PbSession(
        id: json['id'] as String? ?? '',
        userId: json['user_id'] as String? ?? '',
        tenant: json['tenant'] != null
            ? PbTenantRef.fromJson(json['tenant'] as Map<String, dynamic>)
            : const PbTenantRef(),
        accessToken: json['access_token'] as String? ?? '',
        refreshToken: json['refresh_token'] as String? ?? '',
        idToken: json['id_token'] as String? ?? '',
        issuedAt: json['issued_at'] != null
            ? DateTime.parse(json['issued_at'] as String)
            : null,
        expiresAt: json['expires_at'] != null
            ? DateTime.parse(json['expires_at'] as String)
            : null,
      );
}

/// TokenClaims — verified, tenant-scoped result of VerifyToken.
class PbTokenClaims {
  final String userId;
  final PbTenantRef tenant;
  final List<String> groups;
  final DateTime? issuedAt;
  final DateTime? expiresAt;
  final DateTime? notBefore;
  final String sessionId;
  final List<String> siteScope;
  final List<String> integratorRelationships;

  const PbTokenClaims({
    this.userId = '',
    this.tenant = const PbTenantRef(),
    this.groups = const [],
    this.issuedAt,
    this.expiresAt,
    this.notBefore,
    this.sessionId = '',
    this.siteScope = const [],
    this.integratorRelationships = const [],
  });

  factory PbTokenClaims.fromJson(Map<String, dynamic> json) => PbTokenClaims(
        userId: json['user_id'] as String? ?? '',
        tenant: json['tenant'] != null
            ? PbTenantRef.fromJson(json['tenant'] as Map<String, dynamic>)
            : const PbTenantRef(),
        groups: (json['groups'] as List<dynamic>?)?.cast<String>() ?? const [],
        sessionId: json['session_id'] as String? ?? '',
        siteScope:
            (json['site_scope'] as List<dynamic>?)?.cast<String>() ?? const [],
        integratorRelationships:
            (json['integrator_relationships'] as List<dynamic>?)
                    ?.cast<String>() ??
                const [],
      );
}
