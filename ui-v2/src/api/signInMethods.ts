// KAI-135: Sign-in Methods API client.
//
// Scaffolding only: this module provides a dedicated API surface for the
// standalone Sign-in Methods page at /admin/sign-in. It wraps the auth
// provider types from api/users.ts and adds page-specific concerns
// (user counts, last sync, default provider selection, delete).
//
// The production Connect-Go client (KAI-399) will replace the
// lazy-loaded mock at app boot via `registerSignInMethodsClient`.

import type {
  AuthProvider,
  ProviderConfig,
  ProviderKind,
  TestProviderResult,
} from './users';

// Re-export provider types for convenience — call-sites only need to
// import from this module.
export type {
  AuthProvider,
  ProviderConfig,
  ProviderKind,
  TestProviderResult,
  LocalProviderConfig,
  EntraProviderConfig,
  GoogleProviderConfig,
  OktaProviderConfig,
  OidcProviderConfig,
  SamlProviderConfig,
  LdapProviderConfig,
} from './users';

// ---------------------------------------------------------------------------
// Types — Sign-in Methods page-specific
// ---------------------------------------------------------------------------

export type ProviderStatus = 'enabled' | 'disabled' | 'error';

export interface SignInProviderSummary {
  id: string;
  kind: ProviderKind;
  displayName: string;
  status: ProviderStatus;
  userCount: number;
  lastSyncAt: string | null; // ISO-8601
  config: ProviderConfig;
  testedAt: string | null;
  testPassed: boolean;
}

export interface DefaultProviderSelection {
  kind: ProviderKind;
  updatedAt: string;
  updatedBy: string;
}

export interface SignInMethodsClient {
  listProviders(tenantId: string): Promise<SignInProviderSummary[]>;
  getDefaultProvider(tenantId: string): Promise<DefaultProviderSelection>;
  setDefaultProvider(
    tenantId: string,
    kind: ProviderKind,
  ): Promise<DefaultProviderSelection>;
  saveProvider(
    tenantId: string,
    kind: ProviderKind,
    config: ProviderConfig,
  ): Promise<SignInProviderSummary>;
  testProvider(
    tenantId: string,
    kind: ProviderKind,
    config: ProviderConfig,
  ): Promise<TestProviderResult>;
  deleteProvider(
    tenantId: string,
    kind: ProviderKind,
  ): Promise<void>;
}

// ---------------------------------------------------------------------------
// Query-key factory
// ---------------------------------------------------------------------------

export const signInMethodsQueryKeys = {
  all: (tenantId: string) => ['signInMethods', tenantId] as const,
  providers: (tenantId: string) =>
    ['signInMethods', tenantId, 'providers'] as const,
  defaultProvider: (tenantId: string) =>
    ['signInMethods', tenantId, 'default'] as const,
};

// ---------------------------------------------------------------------------
// Client bootstrap (production client registers itself at app boot)
// ---------------------------------------------------------------------------

let activeClient: SignInMethodsClient | null = null;

export function registerSignInMethodsClient(
  client: SignInMethodsClient,
): void {
  activeClient = client;
}

export function resetSignInMethodsClientForTests(): void {
  activeClient = null;
}

async function getClient(): Promise<SignInMethodsClient> {
  if (activeClient) return activeClient;
  const { signInMethodsMockClient } = await import('./signInMethods.mock');
  return signInMethodsMockClient;
}

// ---------------------------------------------------------------------------
// Thin wrappers — call-sites import these, never touch getClient() directly
// ---------------------------------------------------------------------------

export async function listSignInProviders(
  tenantId: string,
): Promise<SignInProviderSummary[]> {
  return (await getClient()).listProviders(tenantId);
}

export async function getDefaultProvider(
  tenantId: string,
): Promise<DefaultProviderSelection> {
  return (await getClient()).getDefaultProvider(tenantId);
}

export async function setDefaultProvider(
  tenantId: string,
  kind: ProviderKind,
): Promise<DefaultProviderSelection> {
  return (await getClient()).setDefaultProvider(tenantId, kind);
}

export async function saveSignInProvider(
  tenantId: string,
  kind: ProviderKind,
  config: ProviderConfig,
): Promise<SignInProviderSummary> {
  return (await getClient()).saveProvider(tenantId, kind, config);
}

export async function testSignInProvider(
  tenantId: string,
  kind: ProviderKind,
  config: ProviderConfig,
): Promise<TestProviderResult> {
  return (await getClient()).testProvider(tenantId, kind, config);
}

export async function deleteSignInProvider(
  tenantId: string,
  kind: ProviderKind,
): Promise<void> {
  return (await getClient()).deleteProvider(tenantId, kind);
}
