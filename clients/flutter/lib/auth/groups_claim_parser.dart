// KAI-147 — Groups claim parser.
//
// Extracts IdP group memberships from the ID token's `groups` claim and maps
// them to the Casbin group format used by the permission system. Per
// lead-security's directive, this is a separate module from login_service.dart
// so it can be reviewed independently.
//
// Claim shapes supported:
//   1. OpenID Connect standard: "groups": ["admin", "viewer"]
//   2. Azure AD: "groups": ["<object-id-1>", "<object-id-2>"]
//   3. Okta/Auth0: "groups": ["Everyone", "Developers"]
//   4. Nested: "realm_access": { "roles": ["admin"] } (Keycloak)
//
// The parser normalises all of these into a flat List<String> of Casbin-
// compatible group identifiers.

import 'dart:convert';

/// Parsed group memberships from an ID token.
class ParsedGroups {
  /// Raw group strings from the token (unmodified).
  final List<String> rawGroups;

  /// Casbin-formatted groups: `role:<group>` or `group:<group>`.
  final List<String> casbinGroups;

  /// The claim key(s) the groups were extracted from.
  final List<String> sourceKeys;

  const ParsedGroups({
    required this.rawGroups,
    required this.casbinGroups,
    required this.sourceKeys,
  });

  /// No groups found in the token.
  static const ParsedGroups empty = ParsedGroups(
    rawGroups: [],
    casbinGroups: [],
    sourceKeys: [],
  );

  bool get isEmpty => rawGroups.isEmpty;
  bool get isNotEmpty => rawGroups.isNotEmpty;
}

/// Stateless utility for extracting groups from JWT claims.
class GroupsClaimParser {
  const GroupsClaimParser();

  /// Known claim keys that may contain group information, in priority order.
  static const List<String> _groupClaimKeys = [
    'groups', // OIDC standard, Azure AD, Okta
    'roles', // Some IdPs put roles at top level
    'group', // Singular variant
  ];

  /// Nested paths: claim -> sub-key that contains groups.
  static const Map<String, String> _nestedGroupPaths = {
    'realm_access': 'roles', // Keycloak
    'resource_access': 'roles', // Keycloak resource-level
  };

  /// Parse groups from a decoded JWT claims map.
  ParsedGroups parse(Map<String, dynamic> claims) {
    final allGroups = <String>[];
    final sourceKeys = <String>[];

    // 1. Check top-level claim keys.
    for (final key in _groupClaimKeys) {
      final value = claims[key];
      if (value is List) {
        final groups = _extractStrings(value);
        if (groups.isNotEmpty) {
          allGroups.addAll(groups);
          sourceKeys.add(key);
        }
      }
    }

    // 2. Check nested paths (e.g. Keycloak realm_access.roles).
    for (final entry in _nestedGroupPaths.entries) {
      final outer = claims[entry.key];
      if (outer is Map<String, dynamic>) {
        final inner = outer[entry.value];
        if (inner is List) {
          final groups = _extractStrings(inner);
          if (groups.isNotEmpty) {
            allGroups.addAll(groups);
            sourceKeys.add('${entry.key}.${entry.value}');
          }
        }
      }
    }

    if (allGroups.isEmpty) return ParsedGroups.empty;

    // Deduplicate while preserving order.
    final unique = allGroups.toSet().toList();

    return ParsedGroups(
      rawGroups: unique,
      casbinGroups: unique.map(_toCasbinGroup).toList(),
      sourceKeys: sourceKeys,
    );
  }

  /// Parse groups directly from a JWT string (convenience method).
  /// Returns [ParsedGroups.empty] if the JWT is invalid or has no groups.
  ParsedGroups parseFromJwt(String jwt) {
    final claims = decodeJwtPayload(jwt);
    if (claims == null) return ParsedGroups.empty;
    return parse(claims);
  }

  /// Decode the payload (middle segment) of a JWT. Returns null if invalid.
  ///
  /// NOTE: This does NOT verify the JWT signature — signature verification is
  /// the responsibility of flutter_appauth / the OIDC library. This is purely
  /// for claim extraction.
  static Map<String, dynamic>? decodeJwtPayload(String jwt) {
    final parts = jwt.split('.');
    if (parts.length != 3) return null;
    try {
      var payload = parts[1];
      switch (payload.length % 4) {
        case 2:
          payload += '==';
          break;
        case 3:
          payload += '=';
          break;
      }
      final decoded = utf8.decode(base64Url.decode(payload));
      final result = jsonDecode(decoded);
      return result is Map<String, dynamic> ? result : null;
    } catch (_) {
      return null;
    }
  }

  /// Convert a raw group string to Casbin format.
  /// Groups are lowercased and prefixed with `group:`.
  static String _toCasbinGroup(String raw) {
    return 'group:${raw.toLowerCase().trim()}';
  }

  /// Extract non-empty strings from a list, filtering out non-string elements.
  static List<String> _extractStrings(List<dynamic> list) {
    return list
        .whereType<String>()
        .where((s) => s.trim().isNotEmpty)
        .toList();
  }
}
