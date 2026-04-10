// KAI-147 — Unit tests for GroupsClaimParser.

import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:mediamtx/auth/groups_claim_parser.dart';

/// Build a minimal JWT with the given claims payload.
String _fakeJwt(Map<String, dynamic> claims) {
  final header = base64Url.encode(utf8.encode('{"alg":"RS256","typ":"JWT"}'));
  final payload = base64Url.encode(utf8.encode(jsonEncode(claims)));
  final signature = base64Url.encode(utf8.encode('fake-sig'));
  return '$header.$payload.$signature';
}

void main() {
  const parser = GroupsClaimParser();

  group('parse() from claims map', () {
    test('returns empty for claims with no group keys', () {
      final result = parser.parse({'sub': 'user1', 'email': 'a@b.com'});
      expect(result.isEmpty, true);
      expect(result.rawGroups, isEmpty);
      expect(result.casbinGroups, isEmpty);
    });

    test('extracts top-level "groups" claim (OIDC standard)', () {
      final result = parser.parse({
        'sub': 'user1',
        'groups': ['admin', 'viewer'],
      });
      expect(result.rawGroups, ['admin', 'viewer']);
      expect(result.casbinGroups, ['group:admin', 'group:viewer']);
      expect(result.sourceKeys, ['groups']);
    });

    test('extracts top-level "roles" claim', () {
      final result = parser.parse({
        'sub': 'user1',
        'roles': ['super-admin'],
      });
      expect(result.rawGroups, ['super-admin']);
      expect(result.casbinGroups, ['group:super-admin']);
      expect(result.sourceKeys, ['roles']);
    });

    test('extracts Keycloak realm_access.roles', () {
      final result = parser.parse({
        'sub': 'user1',
        'realm_access': {
          'roles': ['offline_access', 'uma_authorization', 'admin'],
        },
      });
      expect(result.rawGroups, ['offline_access', 'uma_authorization', 'admin']);
      expect(result.sourceKeys, ['realm_access.roles']);
    });

    test('deduplicates groups across multiple sources', () {
      final result = parser.parse({
        'sub': 'user1',
        'groups': ['admin', 'viewer'],
        'roles': ['admin'], // duplicate
      });
      expect(result.rawGroups, ['admin', 'viewer']);
      expect(result.casbinGroups, ['group:admin', 'group:viewer']);
    });

    test('lowercases groups in Casbin format', () {
      final result = parser.parse({
        'groups': ['ADMIN', 'Super-Viewer'],
      });
      expect(result.casbinGroups, ['group:admin', 'group:super-viewer']);
    });

    test('filters out non-string list elements', () {
      final result = parser.parse({
        'groups': ['admin', 42, null, 'viewer', true],
      });
      expect(result.rawGroups, ['admin', 'viewer']);
    });

    test('filters out empty/whitespace strings', () {
      final result = parser.parse({
        'groups': ['admin', '', '  ', 'viewer'],
      });
      expect(result.rawGroups, ['admin', 'viewer']);
    });

    test('handles groups as non-list (ignored)', () {
      final result = parser.parse({
        'groups': 'not-a-list',
      });
      expect(result.isEmpty, true);
    });
  });

  group('parseFromJwt()', () {
    test('extracts groups from a valid JWT', () {
      final jwt = _fakeJwt({
        'sub': 'user1',
        'groups': ['ops', 'dev'],
      });
      final result = parser.parseFromJwt(jwt);
      expect(result.rawGroups, ['ops', 'dev']);
      expect(result.casbinGroups, ['group:ops', 'group:dev']);
    });

    test('returns empty for malformed JWT', () {
      expect(parser.parseFromJwt('not-a-jwt').isEmpty, true);
      expect(parser.parseFromJwt('a.b').isEmpty, true);
      expect(parser.parseFromJwt('').isEmpty, true);
    });

    test('returns empty for JWT with no groups', () {
      final jwt = _fakeJwt({'sub': 'user1'});
      expect(parser.parseFromJwt(jwt).isEmpty, true);
    });
  });

  group('decodeJwtPayload()', () {
    test('decodes valid JWT payload', () {
      final jwt = _fakeJwt({'sub': 'user1', 'name': 'Test'});
      final claims = GroupsClaimParser.decodeJwtPayload(jwt);
      expect(claims, isNotNull);
      expect(claims!['sub'], 'user1');
      expect(claims['name'], 'Test');
    });

    test('returns null for invalid JWT', () {
      expect(GroupsClaimParser.decodeJwtPayload('not-jwt'), isNull);
      expect(GroupsClaimParser.decodeJwtPayload('a.b'), isNull);
    });

    test('returns null for non-JSON payload', () {
      final header = base64Url.encode(utf8.encode('{}'));
      final payload = base64Url.encode(utf8.encode('not-json'));
      final sig = base64Url.encode(utf8.encode('sig'));
      expect(GroupsClaimParser.decodeJwtPayload('$header.$payload.$sig'), isNull);
    });
  });

  group('ParsedGroups', () {
    test('empty constant', () {
      expect(ParsedGroups.empty.isEmpty, true);
      expect(ParsedGroups.empty.isNotEmpty, false);
      expect(ParsedGroups.empty.rawGroups, isEmpty);
      expect(ParsedGroups.empty.casbinGroups, isEmpty);
      expect(ParsedGroups.empty.sourceKeys, isEmpty);
    });
  });
}
