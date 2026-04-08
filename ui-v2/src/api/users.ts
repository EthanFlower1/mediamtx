// KAI-325: Stub users + permissions + auth-providers API client.
//
// Mock-only deterministic data. Real Connect-Go generated client
// (KAI-226) will replace these. Signatures are promise-based so
// migration is mechanical. All user queries are tenant-scoped.
//
// Sensitive data contract:
//   - client_secret / bind_password are write-only: after save the API
//     returns a masked sentinel ("••••••••"). The UI stores the sentinel
//     in display fields only, never in URL params or localStorage.
//
// TODO(KAI-325): wire POST /api/v1/auth/providers/test once KAI-226
//     HTTP gateway exposes the TestProvider RPC from KAI-222.

// ---------------------------------------------------------------------------
// Types — Users
// ---------------------------------------------------------------------------

export type UserRole = 'admin' | 'operator' | 'viewer' | 'auditor';
export type UserStatus = 'active' | 'suspended';

export interface TenantUser {
  id: string;
  tenantId: string;
  displayName: string;
  email: string;
  role: UserRole;
  groups: string[];
  status: UserStatus;
  lastLoginAt: string | null; // ISO-8601
  ssoProvider: string | null; // provider id or null for local
  createdAt: string;
}

export interface InviteUserArgs {
  tenantId: string;
  email: string;
  role: UserRole;
  groups: string[];
}

export interface UpdateUserArgs {
  tenantId: string;
  userId: string;
  role?: UserRole;
  groups?: string[];
  status?: UserStatus;
}

export interface DeleteUserArgs {
  tenantId: string;
  userId: string;
}

// ---------------------------------------------------------------------------
// Types — Permissions
// ---------------------------------------------------------------------------

export type ResourceAction =
  | 'view.live'
  | 'view.playback'
  | 'ptz.control'
  | 'recording.configure'
  | 'users.manage'
  | 'audit.view'
  | 'cameras.manage'
  | 'recorders.manage'
  | 'integrations.manage';

export interface RolePermission {
  role: UserRole;
  action: ResourceAction;
  allowed: boolean;
  inherited: boolean; // true when value comes from a parent role
}

export interface UpdateRolePermissionArgs {
  tenantId: string;
  role: UserRole;
  action: ResourceAction;
  allowed: boolean;
}

// ---------------------------------------------------------------------------
// Types — Auth providers
// ---------------------------------------------------------------------------

export type ProviderKind =
  | 'local'
  | 'entra'
  | 'google'
  | 'okta'
  | 'oidc'
  | 'saml'
  | 'ldap';

export interface LocalProviderConfig {
  kind: 'local';
  enabled: boolean;
  passwordPolicy: {
    minLength: number;
    requireUppercase: boolean;
    requireLowercase: boolean;
    requireDigit: boolean;
    requireSpecial: boolean;
    rotationDays: number | null; // null = no forced rotation
  };
}

export interface EntraProviderConfig {
  kind: 'entra';
  enabled: boolean;
  clientId: string;
  clientSecret: string; // masked after save
  tenantId: string;
  redirectUri: string;
}

export interface GoogleProviderConfig {
  kind: 'google';
  enabled: boolean;
  clientId: string;
  clientSecret: string;
  hostedDomain: string;
}

export interface OktaProviderConfig {
  kind: 'okta';
  enabled: boolean;
  domain: string;
  clientId: string;
  clientSecret: string;
  authorizationServerId: string;
}

export interface OidcProviderConfig {
  kind: 'oidc';
  enabled: boolean;
  issuerUrl: string;
  clientId: string;
  clientSecret: string;
  scopes: string; // space-separated
  claimMappings: {
    sub: string;
    email: string;
    name: string;
    groups: string;
  };
}

export interface SamlProviderConfig {
  kind: 'saml';
  enabled: boolean;
  metadataUrl: string;
  metadataXml: string;
  entityId: string;
  acsUrl: string;
  signingCert: string;
  attributeMappings: {
    email: string;
    name: string;
    groups: string;
  };
}

export interface LdapProviderConfig {
  kind: 'ldap';
  enabled: boolean;
  host: string;
  port: number;
  bindDn: string;
  bindPassword: string; // masked after save
  baseDn: string;
  userFilter: string;
  groupFilter: string;
  attributeMappings: {
    uid: string;
    email: string;
    name: string;
    memberOf: string;
  };
}

export type ProviderConfig =
  | LocalProviderConfig
  | EntraProviderConfig
  | GoogleProviderConfig
  | OktaProviderConfig
  | OidcProviderConfig
  | SamlProviderConfig
  | LdapProviderConfig;

export interface AuthProvider {
  id: string;
  kind: ProviderKind;
  enabled: boolean;
  config: ProviderConfig;
  testedAt: string | null;
  testPassed: boolean;
}

export interface TestProviderArgs {
  tenantId: string;
  kind: ProviderKind;
  config: ProviderConfig;
}

export interface TestProviderResult {
  success: boolean;
  message: string;       // e.g. "Connected to entra.microsoft.com, 147 users visible"
  troubleshootingUrl: string | null;
}

export interface SaveProviderArgs {
  tenantId: string;
  kind: ProviderKind;
  config: ProviderConfig;
}

// ---------------------------------------------------------------------------
// Query key factories
// ---------------------------------------------------------------------------

export const usersQueryKeys = {
  all: (tenantId: string) => ['users', tenantId] as const,
  list: (tenantId: string) => ['users', tenantId, 'list'] as const,
};

export const permissionsQueryKeys = {
  all: (tenantId: string) => ['permissions', tenantId] as const,
  matrix: (tenantId: string) => ['permissions', tenantId, 'matrix'] as const,
};

export const authProvidersQueryKeys = {
  all: (tenantId: string) => ['authProviders', tenantId] as const,
  list: (tenantId: string) => ['authProviders', tenantId, 'list'] as const,
};

// ---------------------------------------------------------------------------
// Deterministic mock generators
// ---------------------------------------------------------------------------

const ROLES: UserRole[] = ['admin', 'operator', 'viewer', 'auditor'];
const STATUSES: UserStatus[] = ['active', 'active', 'active', 'suspended'];
const SSO_PROVIDERS = [null, null, 'entra', 'google', 'okta'];
const GROUPS = [
  ['security-ops'],
  ['it-admin'],
  ['security-ops', 'review-team'],
  ['auditors'],
  [],
];

function pseudoIndex(seed: string): number {
  let hash = 0;
  for (let i = 0; i < seed.length; i++) {
    hash = (hash * 31 + seed.charCodeAt(i)) | 0;
  }
  return Math.abs(hash);
}

function buildUsers(tenantId: string): TenantUser[] {
  const out: TenantUser[] = [];
  const now = Date.now();
  const offset = pseudoIndex(tenantId) % 10;
  const count = 15 + offset;
  for (let i = 0; i < count; i++) {
    out.push({
      id: `user-${tenantId}-${i.toString().padStart(3, '0')}`,
      tenantId,
      displayName: `User ${i + 1}`,
      email: `user${i + 1}@example.com`,
      role: ROLES[i % ROLES.length]!,
      groups: GROUPS[i % GROUPS.length]!,
      status: STATUSES[i % STATUSES.length]!,
      lastLoginAt:
        i % 5 === 0 ? null : new Date(now - i * 3_600_000).toISOString(),
      ssoProvider: SSO_PROVIDERS[i % SSO_PROVIDERS.length]!,
      createdAt: new Date(now - i * 86_400_000 * 10).toISOString(),
    });
  }
  return out;
}

const DEFAULT_PERMISSIONS: RolePermission[] = (
  [
    'view.live',
    'view.playback',
    'ptz.control',
    'recording.configure',
    'users.manage',
    'audit.view',
    'cameras.manage',
    'recorders.manage',
    'integrations.manage',
  ] as ResourceAction[]
).flatMap((action) =>
  (['admin', 'operator', 'viewer', 'auditor'] as UserRole[]).map((role) => ({
    role,
    action,
    allowed: role === 'admin'
      ? true
      : role === 'operator'
        ? ['view.live', 'view.playback', 'ptz.control'].includes(action)
        : role === 'viewer'
          ? ['view.live', 'view.playback'].includes(action)
          : role === 'auditor'
            ? ['view.playback', 'audit.view'].includes(action)
            : false,
    inherited: false,
  })),
);

// Mutable in-memory store so mutations are reflected in the same session.
const providerStore: Map<string, AuthProvider[]> = new Map();

function getProviders(tenantId: string): AuthProvider[] {
  if (!providerStore.has(tenantId)) {
    providerStore.set(tenantId, [
      {
        id: `${tenantId}-local`,
        kind: 'local',
        enabled: true,
        config: {
          kind: 'local',
          enabled: true,
          passwordPolicy: {
            minLength: 12,
            requireUppercase: true,
            requireLowercase: true,
            requireDigit: true,
            requireSpecial: true,
            rotationDays: null,
          },
        } satisfies LocalProviderConfig,
        testedAt: null,
        testPassed: false,
      },
      {
        id: `${tenantId}-entra`,
        kind: 'entra',
        enabled: false,
        config: {
          kind: 'entra',
          enabled: false,
          clientId: '',
          clientSecret: '',
          tenantId: '',
          redirectUri: '',
        } satisfies EntraProviderConfig,
        testedAt: null,
        testPassed: false,
      },
      {
        id: `${tenantId}-google`,
        kind: 'google',
        enabled: false,
        config: {
          kind: 'google',
          enabled: false,
          clientId: '',
          clientSecret: '',
          hostedDomain: '',
        } satisfies GoogleProviderConfig,
        testedAt: null,
        testPassed: false,
      },
      {
        id: `${tenantId}-okta`,
        kind: 'okta',
        enabled: false,
        config: {
          kind: 'okta',
          enabled: false,
          domain: '',
          clientId: '',
          clientSecret: '',
          authorizationServerId: 'default',
        } satisfies OktaProviderConfig,
        testedAt: null,
        testPassed: false,
      },
      {
        id: `${tenantId}-oidc`,
        kind: 'oidc',
        enabled: false,
        config: {
          kind: 'oidc',
          enabled: false,
          issuerUrl: '',
          clientId: '',
          clientSecret: '',
          scopes: 'openid email profile',
          claimMappings: {
            sub: 'sub',
            email: 'email',
            name: 'name',
            groups: 'groups',
          },
        } satisfies OidcProviderConfig,
        testedAt: null,
        testPassed: false,
      },
      {
        id: `${tenantId}-saml`,
        kind: 'saml',
        enabled: false,
        config: {
          kind: 'saml',
          enabled: false,
          metadataUrl: '',
          metadataXml: '',
          entityId: '',
          acsUrl: '',
          signingCert: '',
          attributeMappings: {
            email: 'http://schemas.xmlsoap.org/ws/2005/05/identity/claims/emailaddress',
            name: 'http://schemas.xmlsoap.org/ws/2005/05/identity/claims/name',
            groups: 'http://schemas.microsoft.com/ws/2008/06/identity/claims/groups',
          },
        } satisfies SamlProviderConfig,
        testedAt: null,
        testPassed: false,
      },
      {
        id: `${tenantId}-ldap`,
        kind: 'ldap',
        enabled: false,
        config: {
          kind: 'ldap',
          enabled: false,
          host: '',
          port: 636,
          bindDn: '',
          bindPassword: '',
          baseDn: '',
          userFilter: '(objectClass=person)',
          groupFilter: '(objectClass=groupOfNames)',
          attributeMappings: {
            uid: 'sAMAccountName',
            email: 'mail',
            name: 'displayName',
            memberOf: 'memberOf',
          },
        } satisfies LdapProviderConfig,
        testedAt: null,
        testPassed: false,
      },
    ]);
  }
  return providerStore.get(tenantId)!;
}

// ---------------------------------------------------------------------------
// Public API — Users
// ---------------------------------------------------------------------------

export async function listUsers(tenantId: string): Promise<TenantUser[]> {
  await Promise.resolve();
  return buildUsers(tenantId);
}

export async function inviteUser(args: InviteUserArgs): Promise<TenantUser> {
  await Promise.resolve();
  return {
    id: `user-${args.tenantId}-new-${Date.now()}`,
    tenantId: args.tenantId,
    displayName: args.email.split('@')[0] ?? args.email,
    email: args.email,
    role: args.role,
    groups: args.groups,
    status: 'active',
    lastLoginAt: null,
    ssoProvider: null,
    createdAt: new Date().toISOString(),
  };
}

export async function updateUser(args: UpdateUserArgs): Promise<void> {
  await Promise.resolve();
  void args;
}

export async function suspendUser(args: { tenantId: string; userId: string }): Promise<void> {
  await Promise.resolve();
  void args;
}

export async function deleteUser(args: DeleteUserArgs): Promise<void> {
  await Promise.resolve();
  void args;
}

// ---------------------------------------------------------------------------
// Public API — Permissions
// ---------------------------------------------------------------------------

export async function listPermissions(tenantId: string): Promise<RolePermission[]> {
  await Promise.resolve();
  void tenantId;
  return DEFAULT_PERMISSIONS;
}

export async function updateRolePermission(
  args: UpdateRolePermissionArgs,
): Promise<void> {
  // TODO(KAI-325): call PUT /api/v1/permissions/roles once KAI-225 Casbin
  //   HTTP endpoint is wired in KAI-226 gateway.
  await Promise.resolve();
  void args;
}

// ---------------------------------------------------------------------------
// Public API — Auth Providers
// ---------------------------------------------------------------------------

export async function listAuthProviders(tenantId: string): Promise<AuthProvider[]> {
  await Promise.resolve();
  return getProviders(tenantId);
}

export async function testAuthProvider(
  args: TestProviderArgs,
): Promise<TestProviderResult> {
  // TODO(KAI-325): call POST /api/v1/auth/providers/test (TestProvider RPC
  //   from KAI-222 IdentityProvider interface, HTTP gateway KAI-226).
  //   Endpoint not yet wired — using stub response.
  await new Promise((r) => setTimeout(r, 800)); // simulate network

  if (args.kind === 'local') {
    return { success: true, message: 'Local auth is always available.', troubleshootingUrl: null };
  }

  // Simulate success for well-formed configs, failure for empty required fields.
  const config = args.config;
  let missingField: string | null = null;
  if (config.kind === 'entra' && (!config.clientId || !config.tenantId)) {
    missingField = 'client_id or tenant_id';
  } else if (config.kind === 'google' && !config.clientId) {
    missingField = 'client_id';
  } else if (config.kind === 'okta' && (!config.domain || !config.clientId)) {
    missingField = 'domain or client_id';
  } else if (config.kind === 'oidc' && !config.issuerUrl) {
    missingField = 'issuer_url';
  } else if (config.kind === 'saml' && !config.metadataUrl && !config.metadataXml) {
    missingField = 'metadata_url or metadata_xml';
  } else if (config.kind === 'ldap' && (!config.host || !config.bindDn)) {
    missingField = 'host or bind_dn';
  }

  if (missingField) {
    return {
      success: false,
      message: `Connection test failed: missing ${missingField}.`,
      troubleshootingUrl: `https://docs.kaivue.com/auth/${args.kind}#troubleshooting`,
    };
  }

  const providerLabels: Record<ProviderKind, string> = {
    local: 'local',
    entra: 'entra.microsoft.com',
    google: 'accounts.google.com',
    okta: 'okta.com',
    oidc: 'OIDC provider',
    saml: 'SAML IdP',
    ldap: 'directory server',
  };

  return {
    success: true,
    message: `Connected to ${providerLabels[args.kind]}. Configuration verified successfully.`,
    troubleshootingUrl: null,
  };
}

export async function saveAuthProvider(args: SaveProviderArgs): Promise<AuthProvider> {
  await Promise.resolve();
  const providers = getProviders(args.tenantId);
  const existing = providers.find((p) => p.kind === args.kind);
  const masked = maskSensitiveFields(args.config);
  const updated: AuthProvider = existing
    ? { ...existing, enabled: true, config: masked, testedAt: new Date().toISOString(), testPassed: true }
    : {
        id: `${args.tenantId}-${args.kind}`,
        kind: args.kind,
        enabled: true,
        config: masked,
        testedAt: new Date().toISOString(),
        testPassed: true,
      };
  const idx = providers.findIndex((p) => p.kind === args.kind);
  if (idx >= 0) providers[idx] = updated;
  return updated;
}

/** Replace secret values with the masked sentinel after saving. */
function maskSensitiveFields(config: ProviderConfig): ProviderConfig {
  const MASK = '••••••••';
  if (config.kind === 'entra' && config.clientSecret) {
    return { ...config, clientSecret: MASK };
  }
  if (config.kind === 'google' && config.clientSecret) {
    return { ...config, clientSecret: MASK };
  }
  if (config.kind === 'okta' && config.clientSecret) {
    return { ...config, clientSecret: MASK };
  }
  if (config.kind === 'oidc' && config.clientSecret) {
    return { ...config, clientSecret: MASK };
  }
  if (config.kind === 'ldap' && config.bindPassword) {
    return { ...config, bindPassword: MASK };
  }
  return config;
}
