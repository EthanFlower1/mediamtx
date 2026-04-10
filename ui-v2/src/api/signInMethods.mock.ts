// KAI-135: Deterministic in-memory mock for signInMethods.ts.
//
// Scaffolding only. The production client (KAI-399 Connect-Go) will
// register itself via `registerSignInMethodsClient` at boot, making
// this module unreachable in production bundles.

import type {
  DefaultProviderSelection,
  ProviderConfig,
  ProviderKind,
  SignInMethodsClient,
  SignInProviderSummary,
  ProviderStatus,
  LocalProviderConfig,
  OidcProviderConfig,
} from './signInMethods';
import type { TestProviderResult } from './users';

// ---------------------------------------------------------------------------
// Provider display names — deterministic, no i18n (mock only)
// ---------------------------------------------------------------------------

const DISPLAY_NAMES: Record<ProviderKind, string> = {
  local: 'Email + Password',
  entra: 'Microsoft Entra ID',
  google: 'Google Workspace',
  okta: 'Okta',
  oidc: 'Generic OIDC',
  saml: 'SAML 2.0',
  ldap: 'LDAP / Active Directory',
};

// ---------------------------------------------------------------------------
// Seed data — 2 pre-configured providers: Local + one OIDC
// ---------------------------------------------------------------------------

const FIXED_DATE = '2026-04-01T12:00:00.000Z';

function buildSeedProviders(): SignInProviderSummary[] {
  const localConfig: LocalProviderConfig = {
    kind: 'local',
    enabled: true,
    passwordPolicy: {
      minLength: 10,
      requireUppercase: true,
      requireLowercase: true,
      requireDigit: true,
      requireSpecial: false,
      rotationDays: 90,
    },
  };

  const oidcConfig: OidcProviderConfig = {
    kind: 'oidc',
    enabled: true,
    issuerUrl: 'https://idp.example.com',
    clientId: 'kaivue-client-001',
    clientSecret: 'mock-secret-value-001',
    scopes: 'openid profile email groups',
    claimMappings: {
      sub: 'sub',
      email: 'email',
      name: 'name',
      groups: 'groups',
    },
  };

  return [
    {
      id: 'provider-local-001',
      kind: 'local',
      displayName: DISPLAY_NAMES.local,
      status: 'enabled' as ProviderStatus,
      userCount: 12,
      lastSyncAt: null,
      config: localConfig,
      testedAt: FIXED_DATE,
      testPassed: true,
    },
    {
      id: 'provider-oidc-001',
      kind: 'oidc',
      displayName: DISPLAY_NAMES.oidc,
      status: 'enabled' as ProviderStatus,
      userCount: 43,
      lastSyncAt: FIXED_DATE,
      config: oidcConfig,
      testedAt: FIXED_DATE,
      testPassed: true,
    },
  ];
}

// ---------------------------------------------------------------------------
// In-memory state per tenant
// ---------------------------------------------------------------------------

interface TenantState {
  providers: SignInProviderSummary[];
  defaultKind: ProviderKind;
}

const tenantStates: Map<string, TenantState> = new Map();

function getTenantState(tenantId: string): TenantState {
  let state = tenantStates.get(tenantId);
  if (!state) {
    state = {
      providers: buildSeedProviders(),
      defaultKind: 'local',
    };
    tenantStates.set(tenantId, state);
  }
  return state;
}

// ---------------------------------------------------------------------------
// Mock client
// ---------------------------------------------------------------------------

export const signInMethodsMockClient: SignInMethodsClient = {
  async listProviders(tenantId: string): Promise<SignInProviderSummary[]> {
    await Promise.resolve();
    return getTenantState(tenantId).providers;
  },

  async getDefaultProvider(
    tenantId: string,
  ): Promise<DefaultProviderSelection> {
    await Promise.resolve();
    const state = getTenantState(tenantId);
    return {
      kind: state.defaultKind,
      updatedAt: FIXED_DATE,
      updatedBy: 'system',
    };
  },

  async setDefaultProvider(
    tenantId: string,
    kind: ProviderKind,
  ): Promise<DefaultProviderSelection> {
    await Promise.resolve();
    const state = getTenantState(tenantId);
    state.defaultKind = kind;
    return {
      kind,
      updatedAt: new Date().toISOString(),
      updatedBy: 'mock-admin',
    };
  },

  async saveProvider(
    tenantId: string,
    kind: ProviderKind,
    config: ProviderConfig,
  ): Promise<SignInProviderSummary> {
    await Promise.resolve();
    const state = getTenantState(tenantId);
    const existing = state.providers.find((p) => p.kind === kind);

    const summary: SignInProviderSummary = {
      id: existing?.id ?? `provider-${kind}-${Date.now()}`,
      kind,
      displayName: DISPLAY_NAMES[kind],
      status: 'enabled',
      userCount: existing?.userCount ?? 0,
      lastSyncAt: existing?.lastSyncAt ?? null,
      config,
      testedAt: existing?.testedAt ?? null,
      testPassed: existing?.testPassed ?? false,
    };

    if (existing) {
      const idx = state.providers.indexOf(existing);
      state.providers[idx] = summary;
    } else {
      state.providers.push(summary);
    }

    return summary;
  },

  async testProvider(
    _tenantId: string,
    _kind: ProviderKind,
    _config: ProviderConfig,
  ): Promise<TestProviderResult> {
    // Simulate a short delay then always succeed.
    await new Promise<void>((resolve) => setTimeout(resolve, 400));
    return {
      success: true,
      message: 'Connection test passed. 42 users visible.',
      troubleshootingUrl: null,
    };
  },

  async deleteProvider(
    tenantId: string,
    kind: ProviderKind,
  ): Promise<void> {
    await Promise.resolve();
    const state = getTenantState(tenantId);
    state.providers = state.providers.filter((p) => p.kind !== kind);
    if (state.defaultKind === kind) {
      state.defaultKind = state.providers[0]?.kind ?? 'local';
    }
  },
};

/** Test helper: clear the in-memory tenant state so tests are isolated. */
export function __resetSignInMethodsMockStateForTests(): void {
  tenantStates.clear();
}
