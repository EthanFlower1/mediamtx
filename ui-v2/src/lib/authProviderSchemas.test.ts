// KAI-325: Vitest unit tests for all 6 SSO wizard Zod schemas.

import { describe, it, expect } from 'vitest';
import {
  localProviderSchema,
  entraProviderSchema,
  googleProviderSchema,
  oktaProviderSchema,
  oidcProviderSchema,
  oidcProviderStep1Schema,
  samlProviderStep1Schema,
  samlProviderStep2Schema,
  ldapProviderSchema,
  ldapProviderStep1Schema,
  inviteUserSchema,
} from './authProviderSchemas';

// ---------------------------------------------------------------------------
// Local provider
// ---------------------------------------------------------------------------

describe('localProviderSchema', () => {
  it('accepts valid NIST-aligned defaults', () => {
    const result = localProviderSchema.safeParse({
      enabled: true,
      minLength: 12,
      requireUppercase: true,
      requireLowercase: true,
      requireDigit: true,
      requireSpecial: true,
      rotationDays: 0,
    });
    expect(result.success).toBe(true);
  });

  it('rejects minLength < 8', () => {
    const result = localProviderSchema.safeParse({
      enabled: true,
      minLength: 6,
      requireUppercase: false,
      requireLowercase: false,
      requireDigit: false,
      requireSpecial: false,
      rotationDays: 0,
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      expect(result.error.issues[0]?.message).toBe('auth.local.errors.minLengthMin');
    }
  });

  it('rejects minLength > 128', () => {
    const result = localProviderSchema.safeParse({
      enabled: true,
      minLength: 200,
      requireUppercase: false,
      requireLowercase: false,
      requireDigit: false,
      requireSpecial: false,
      rotationDays: 0,
    });
    expect(result.success).toBe(false);
  });

  it('rejects negative rotationDays', () => {
    const result = localProviderSchema.safeParse({
      enabled: true,
      minLength: 12,
      requireUppercase: false,
      requireLowercase: false,
      requireDigit: false,
      requireSpecial: false,
      rotationDays: -1,
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Microsoft Entra ID
// ---------------------------------------------------------------------------

describe('entraProviderSchema', () => {
  it('accepts complete credentials', () => {
    const result = entraProviderSchema.safeParse({
      clientId: 'abc-123',
      clientSecret: 'secret!',
      tenantId: 'tenant-guid',
    });
    expect(result.success).toBe(true);
  });

  it('rejects missing clientId', () => {
    const result = entraProviderSchema.safeParse({
      clientId: '',
      clientSecret: 'secret!',
      tenantId: 'tenant-guid',
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const msgs = result.error.issues.map((i) => i.message);
      expect(msgs).toContain('auth.entra.errors.clientIdRequired');
    }
  });

  it('rejects missing tenantId', () => {
    const result = entraProviderSchema.safeParse({
      clientId: 'abc',
      clientSecret: 'secret!',
      tenantId: '',
    });
    expect(result.success).toBe(false);
  });

  it('rejects missing clientSecret', () => {
    const result = entraProviderSchema.safeParse({
      clientId: 'abc',
      clientSecret: '',
      tenantId: 'tenant-guid',
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Google Workspace
// ---------------------------------------------------------------------------

describe('googleProviderSchema', () => {
  it('accepts valid credentials + hosted domain', () => {
    const result = googleProviderSchema.safeParse({
      clientId: 'client-id.apps.googleusercontent.com',
      clientSecret: 'secret!',
      hostedDomain: 'acme.com',
    });
    expect(result.success).toBe(true);
  });

  it('rejects invalid hosted domain format', () => {
    const result = googleProviderSchema.safeParse({
      clientId: 'client-id',
      clientSecret: 'secret!',
      hostedDomain: 'notadomain',
    });
    expect(result.success).toBe(false);
  });

  it('rejects empty clientId', () => {
    const result = googleProviderSchema.safeParse({
      clientId: '',
      clientSecret: 'secret!',
      hostedDomain: 'acme.com',
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Okta
// ---------------------------------------------------------------------------

describe('oktaProviderSchema', () => {
  it('accepts valid Okta credentials', () => {
    const result = oktaProviderSchema.safeParse({
      domain: 'acme.okta.com',
      clientId: 'cid',
      clientSecret: 'sec',
      authorizationServerId: 'default',
    });
    expect(result.success).toBe(true);
  });

  it('rejects missing domain', () => {
    const result = oktaProviderSchema.safeParse({
      domain: '',
      clientId: 'cid',
      clientSecret: 'sec',
      authorizationServerId: 'default',
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const msgs = result.error.issues.map((i) => i.message);
      expect(msgs).toContain('auth.okta.errors.domainRequired');
    }
  });

  it('rejects missing authorizationServerId', () => {
    const result = oktaProviderSchema.safeParse({
      domain: 'acme.okta.com',
      clientId: 'cid',
      clientSecret: 'sec',
      authorizationServerId: '',
    });
    expect(result.success).toBe(false);
  });
});

// ---------------------------------------------------------------------------
// Generic OIDC
// ---------------------------------------------------------------------------

describe('oidcProviderStep1Schema', () => {
  it('accepts a valid HTTPS issuer URL with openid scope', () => {
    const result = oidcProviderStep1Schema.safeParse({
      issuerUrl: 'https://idp.example.com',
      clientId: 'cid',
      clientSecret: 'sec',
      scopes: 'openid email profile',
    });
    expect(result.success).toBe(true);
  });

  it('rejects HTTP (non-HTTPS) issuer URL', () => {
    const result = oidcProviderStep1Schema.safeParse({
      issuerUrl: 'http://insecure.example.com',
      clientId: 'cid',
      clientSecret: 'sec',
      scopes: 'openid',
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const msgs = result.error.issues.map((i) => i.message);
      expect(msgs).toContain('auth.oidc.errors.issuerUrlInvalid');
    }
  });

  it('rejects scopes that do not include openid', () => {
    const result = oidcProviderStep1Schema.safeParse({
      issuerUrl: 'https://idp.example.com',
      clientId: 'cid',
      clientSecret: 'sec',
      scopes: 'email profile',
    });
    expect(result.success).toBe(false);
  });

  it('rejects empty issuerUrl', () => {
    const result = oidcProviderStep1Schema.safeParse({
      issuerUrl: '',
      clientId: 'cid',
      clientSecret: 'sec',
      scopes: 'openid',
    });
    expect(result.success).toBe(false);
  });
});

describe('oidcProviderSchema (full)', () => {
  it('accepts valid step 1 + step 2 data', () => {
    const result = oidcProviderSchema.safeParse({
      issuerUrl: 'https://idp.example.com',
      clientId: 'cid',
      clientSecret: 'sec',
      scopes: 'openid email',
      claimSub: 'sub',
      claimEmail: 'email',
      claimName: 'name',
      claimGroups: 'groups',
    });
    expect(result.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// SAML 2.0
// ---------------------------------------------------------------------------

describe('samlProviderStep1Schema', () => {
  it('accepts a metadata URL', () => {
    const result = samlProviderStep1Schema.safeParse({
      metadataUrl: 'https://idp.example.com/metadata',
      metadataXml: '',
    });
    expect(result.success).toBe(true);
  });

  it('accepts raw metadata XML with no URL', () => {
    const result = samlProviderStep1Schema.safeParse({
      metadataUrl: '',
      metadataXml: '<EntityDescriptor>...</EntityDescriptor>',
    });
    expect(result.success).toBe(true);
  });

  it('rejects when both metadataUrl and metadataXml are empty', () => {
    const result = samlProviderStep1Schema.safeParse({
      metadataUrl: '',
      metadataXml: '',
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const msgs = result.error.issues.map((i) => i.message);
      expect(msgs).toContain('auth.saml.errors.metadataRequired');
    }
  });
});

describe('samlProviderStep2Schema', () => {
  it('accepts valid service provider config', () => {
    const result = samlProviderStep2Schema.safeParse({
      entityId: 'https://app.example.com/saml/metadata',
      signingCert: '-----BEGIN CERTIFICATE-----',
      attrEmail: 'email',
      attrName: 'name',
      attrGroups: 'groups',
    });
    expect(result.success).toBe(true);
  });

  it('rejects missing entityId', () => {
    const result = samlProviderStep2Schema.safeParse({
      entityId: '',
      signingCert: '',
      attrEmail: 'email',
      attrName: 'name',
      attrGroups: 'groups',
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const msgs = result.error.issues.map((i) => i.message);
      expect(msgs).toContain('auth.saml.errors.entityIdRequired');
    }
  });
});

// ---------------------------------------------------------------------------
// LDAP / Active Directory
// ---------------------------------------------------------------------------

describe('ldapProviderStep1Schema', () => {
  it('accepts valid connection settings', () => {
    const result = ldapProviderStep1Schema.safeParse({
      host: 'ldap.acme.com',
      port: 636,
      bindDn: 'cn=svc,dc=acme,dc=com',
      bindPassword: 'p@ssword',
    });
    expect(result.success).toBe(true);
  });

  it('rejects port 0', () => {
    const result = ldapProviderStep1Schema.safeParse({
      host: 'ldap.acme.com',
      port: 0,
      bindDn: 'cn=svc,dc=acme,dc=com',
      bindPassword: 'pass',
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const msgs = result.error.issues.map((i) => i.message);
      expect(msgs).toContain('auth.ldap.errors.portInvalid');
    }
  });

  it('rejects port > 65535', () => {
    const result = ldapProviderStep1Schema.safeParse({
      host: 'ldap.acme.com',
      port: 99999,
      bindDn: 'cn=svc,dc=acme,dc=com',
      bindPassword: 'pass',
    });
    expect(result.success).toBe(false);
  });

  it('rejects empty host', () => {
    const result = ldapProviderStep1Schema.safeParse({
      host: '',
      port: 636,
      bindDn: 'cn=svc,dc=acme,dc=com',
      bindPassword: 'pass',
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const msgs = result.error.issues.map((i) => i.message);
      expect(msgs).toContain('auth.ldap.errors.hostRequired');
    }
  });

  it('rejects empty bindPassword', () => {
    const result = ldapProviderStep1Schema.safeParse({
      host: 'ldap.acme.com',
      port: 636,
      bindDn: 'cn=svc,dc=acme,dc=com',
      bindPassword: '',
    });
    expect(result.success).toBe(false);
  });
});

describe('ldapProviderSchema (full)', () => {
  it('accepts a complete LDAP config', () => {
    const result = ldapProviderSchema.safeParse({
      host: 'ldap.acme.com',
      port: 636,
      bindDn: 'cn=svc,dc=acme,dc=com',
      bindPassword: 'p@ssword!',
      baseDn: 'dc=acme,dc=com',
      userFilter: '(objectClass=person)',
      groupFilter: '(objectClass=groupOfNames)',
      attrUid: 'sAMAccountName',
      attrEmail: 'mail',
      attrName: 'displayName',
      attrMemberOf: 'memberOf',
    });
    expect(result.success).toBe(true);
  });
});

// ---------------------------------------------------------------------------
// Invite user
// ---------------------------------------------------------------------------

describe('inviteUserSchema', () => {
  it('accepts a valid invite', () => {
    const result = inviteUserSchema.safeParse({
      email: 'alice@example.com',
      role: 'viewer',
      groups: 'security-ops, review-team',
    });
    expect(result.success).toBe(true);
  });

  it('rejects invalid email', () => {
    const result = inviteUserSchema.safeParse({
      email: 'not-an-email',
      role: 'viewer',
      groups: '',
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const msgs = result.error.issues.map((i) => i.message);
      expect(msgs).toContain('users.invite.errors.emailInvalid');
    }
  });

  it('rejects empty email', () => {
    const result = inviteUserSchema.safeParse({
      email: '',
      role: 'viewer',
      groups: '',
    });
    expect(result.success).toBe(false);
    if (!result.success) {
      const msgs = result.error.issues.map((i) => i.message);
      expect(msgs).toContain('users.invite.errors.emailRequired');
    }
  });

  it('rejects invalid role', () => {
    const result = inviteUserSchema.safeParse({
      email: 'alice@example.com',
      role: 'superuser',
      groups: '',
    });
    expect(result.success).toBe(false);
  });

  it('accepts empty groups string', () => {
    const result = inviteUserSchema.safeParse({
      email: 'alice@example.com',
      role: 'admin',
      groups: '',
    });
    expect(result.success).toBe(true);
  });
});
