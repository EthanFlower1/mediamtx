// KAI-325: Zod validation schemas for all 6 SSO wizard forms + local auth.
//
// Shared between the wizard components and their Vitest unit tests.
// Never embed provider-specific constants from Zitadel — these schemas
// validate generic OIDC / SAML / LDAP field shapes only.

import { z } from 'zod';

// ---------------------------------------------------------------------------
// Local (email + password)
// ---------------------------------------------------------------------------

export const localProviderSchema = z.object({
  enabled: z.boolean(),
  minLength: z
    .number({ invalid_type_error: 'auth.local.errors.minLengthMin' })
    .int()
    .min(8, 'auth.local.errors.minLengthMin')
    .max(128, 'auth.local.errors.minLengthMax'),
  requireUppercase: z.boolean(),
  requireLowercase: z.boolean(),
  requireDigit: z.boolean(),
  requireSpecial: z.boolean(),
  rotationDays: z
    .number({ invalid_type_error: 'auth.local.errors.rotationDaysMin' })
    .int()
    .min(0, 'auth.local.errors.rotationDaysMin'),
});

export type LocalProviderFormValues = z.infer<typeof localProviderSchema>;

// ---------------------------------------------------------------------------
// Microsoft Entra ID (Azure AD)
// ---------------------------------------------------------------------------

export const entraProviderSchema = z.object({
  clientId: z.string().min(1, 'auth.entra.errors.clientIdRequired'),
  clientSecret: z.string().min(1, 'auth.entra.errors.clientSecretRequired'),
  tenantId: z.string().min(1, 'auth.entra.errors.tenantIdRequired'),
});

export type EntraProviderFormValues = z.infer<typeof entraProviderSchema>;

// ---------------------------------------------------------------------------
// Google Workspace
// ---------------------------------------------------------------------------

export const googleProviderSchema = z.object({
  clientId: z.string().min(1, 'auth.google.errors.clientIdRequired'),
  clientSecret: z.string().min(1, 'auth.google.errors.clientSecretRequired'),
  hostedDomain: z
    .string()
    .min(1, 'auth.google.errors.hostedDomainRequired')
    .regex(/^[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$/, 'auth.google.errors.hostedDomainRequired'),
});

export type GoogleProviderFormValues = z.infer<typeof googleProviderSchema>;

// ---------------------------------------------------------------------------
// Okta
// ---------------------------------------------------------------------------

export const oktaProviderSchema = z.object({
  domain: z.string().min(1, 'auth.okta.errors.domainRequired'),
  clientId: z.string().min(1, 'auth.okta.errors.clientIdRequired'),
  clientSecret: z.string().min(1, 'auth.okta.errors.clientSecretRequired'),
  authorizationServerId: z
    .string()
    .min(1, 'auth.okta.errors.authorizationServerIdRequired'),
});

export type OktaProviderFormValues = z.infer<typeof oktaProviderSchema>;

// ---------------------------------------------------------------------------
// Generic OIDC
// ---------------------------------------------------------------------------

const httpsUrl = z
  .string()
  .min(1, 'auth.oidc.errors.issuerUrlRequired')
  .refine((val) => {
    try {
      return new URL(val).protocol === 'https:';
    } catch {
      return false;
    }
  }, 'auth.oidc.errors.issuerUrlInvalid');

export const oidcProviderStep1Schema = z.object({
  issuerUrl: httpsUrl,
  clientId: z.string().min(1, 'auth.oidc.errors.clientIdRequired'),
  clientSecret: z.string().min(1, 'auth.oidc.errors.clientSecretRequired'),
  scopes: z
    .string()
    .min(1, 'auth.oidc.errors.scopesRequired')
    .refine((v) => v.split(' ').includes('openid'), 'auth.oidc.errors.scopesRequired'),
});

export const oidcProviderStep2Schema = z.object({
  claimSub: z.string().min(1),
  claimEmail: z.string().min(1),
  claimName: z.string().min(1),
  claimGroups: z.string().min(1),
});

export const oidcProviderSchema = oidcProviderStep1Schema.merge(oidcProviderStep2Schema);

export type OidcProviderFormValues = z.infer<typeof oidcProviderSchema>;

// ---------------------------------------------------------------------------
// SAML 2.0
// ---------------------------------------------------------------------------

export const samlProviderStep1Schema = z
  .object({
    metadataUrl: z.string(),
    metadataXml: z.string(),
  })
  .refine(
    (v) => v.metadataUrl.trim().length > 0 || v.metadataXml.trim().length > 0,
    { message: 'auth.saml.errors.metadataRequired', path: ['metadataUrl'] },
  );

export const samlProviderStep2Schema = z.object({
  entityId: z.string().min(1, 'auth.saml.errors.entityIdRequired'),
  signingCert: z.string(),
  attrEmail: z.string().min(1),
  attrName: z.string().min(1),
  attrGroups: z.string().min(1),
});

export const samlProviderSchema = samlProviderStep1Schema.and(samlProviderStep2Schema);

export type SamlProviderFormValues = z.infer<typeof samlProviderSchema>;

// ---------------------------------------------------------------------------
// LDAP / Active Directory
// ---------------------------------------------------------------------------

export const ldapProviderStep1Schema = z.object({
  host: z.string().min(1, 'auth.ldap.errors.hostRequired'),
  port: z
    .number({ invalid_type_error: 'auth.ldap.errors.portInvalid' })
    .int()
    .min(1, 'auth.ldap.errors.portInvalid')
    .max(65535, 'auth.ldap.errors.portInvalid'),
  bindDn: z.string().min(1, 'auth.ldap.errors.bindDnRequired'),
  bindPassword: z.string().min(1, 'auth.ldap.errors.bindPasswordRequired'),
});

export const ldapProviderStep2Schema = z.object({
  baseDn: z.string().min(1, 'auth.ldap.errors.baseDnRequired'),
  userFilter: z.string().min(1, 'auth.ldap.errors.userFilterRequired'),
  groupFilter: z.string(),
  attrUid: z.string().min(1),
  attrEmail: z.string().min(1),
  attrName: z.string().min(1),
  attrMemberOf: z.string().min(1),
});

export const ldapProviderSchema = ldapProviderStep1Schema.merge(ldapProviderStep2Schema);

export type LdapProviderFormValues = z.infer<typeof ldapProviderSchema>;

// ---------------------------------------------------------------------------
// Invite user
// ---------------------------------------------------------------------------

export const inviteUserSchema = z.object({
  email: z
    .string()
    .min(1, 'users.invite.errors.emailRequired')
    .email('users.invite.errors.emailInvalid'),
  role: z.enum(['admin', 'operator', 'viewer', 'auditor'], {
    errorMap: () => ({ message: 'users.invite.errors.roleRequired' }),
  }),
  groups: z.string(),
});

export type InviteUserFormValues = z.infer<typeof inviteUserSchema>;
