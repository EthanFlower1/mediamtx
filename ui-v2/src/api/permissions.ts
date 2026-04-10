// KAI-315: Permissions API client — types, query-key factory, feature-flag bootstrap.
//
// Follows the API split pattern: this file has types + client interface +
// query-key factory + feature-flag bootstrap with lazy mock import.
// The deterministic mock implementation lives in permissions.mock.ts.
//
// The real Connect-Go implementation will replace the mock import when
// KAI-238 protos generate. This file is the single seam.

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/** Tri-state permission value. */
export type PermissionState = 'enabled' | 'disabled' | 'inherited';

/** Top-level permission categories. */
export type PermissionCategoryId =
  | 'cameras'
  | 'recordings'
  | 'ai_features'
  | 'users'
  | 'billing'
  | 'api_access'
  | 'white_label'
  | 'alerts';

/** Sub-permission within a category. */
export interface SubPermission {
  readonly id: string;
  readonly labelKey: string; // i18n key
  readonly state: PermissionState;
}

/** A top-level permission category with optional sub-permissions. */
export interface PermissionCategory {
  readonly id: PermissionCategoryId;
  readonly labelKey: string; // i18n key
  readonly state: PermissionState;
  readonly subPermissions: readonly SubPermission[];
}

/** A single tenant row in the permissions matrix. */
export interface TenantPermissionRow {
  readonly tenantId: string;
  readonly tenantName: string;
  readonly planTier: string;
  readonly categories: readonly PermissionCategory[];
}

/** A single override change for the diff preview. */
export interface PermissionOverride {
  readonly tenantId: string;
  readonly tenantName: string;
  readonly categoryId: PermissionCategoryId;
  readonly subPermissionId: string | null; // null = top-level category
  readonly oldState: PermissionState;
  readonly newState: PermissionState;
}

/** Audit trail entry for permission changes. */
export interface PermissionAuditEntry {
  readonly id: string;
  readonly timestamp: string; // ISO-8601
  readonly actor: string;
  readonly tenantId: string;
  readonly tenantName: string;
  readonly categoryId: PermissionCategoryId;
  readonly subPermissionId: string | null;
  readonly oldState: PermissionState;
  readonly newState: PermissionState;
}

/** Full matrix snapshot returned from the API. */
export interface PermissionMatrix {
  readonly integratorId: string;
  readonly rows: readonly TenantPermissionRow[];
  readonly auditTrail: readonly PermissionAuditEntry[];
}

/** Request to save permission changes. */
export interface SavePermissionsRequest {
  readonly integratorId: string;
  readonly overrides: readonly PermissionOverride[];
}

// ---------------------------------------------------------------------------
// Client interface
// ---------------------------------------------------------------------------

export interface PermissionsClient {
  getMatrix(integratorId: string): Promise<PermissionMatrix>;
  savePermissions(request: SavePermissionsRequest): Promise<void>;
}

// ---------------------------------------------------------------------------
// Query-key factory (integrator-scoped)
// ---------------------------------------------------------------------------

export const PERMISSIONS_QUERY_KEY = 'permissions' as const;

export function permissionsQueryKey(integratorId: string) {
  return [PERMISSIONS_QUERY_KEY, integratorId] as const;
}

// ---------------------------------------------------------------------------
// Feature-flag bootstrap with lazy mock import
// ---------------------------------------------------------------------------

let clientInstance: PermissionsClient | null = null;

async function loadMockClient(): Promise<PermissionsClient> {
  const mod = await import('./permissions.mock');
  return mod.mockPermissionsClient;
}

async function getClient(): Promise<PermissionsClient> {
  if (!clientInstance) {
    // TODO: replace with real Connect-Go client when KAI-238 protos land
    clientInstance = await loadMockClient();
  }
  return clientInstance;
}

export async function getPermissionMatrix(integratorId: string): Promise<PermissionMatrix> {
  const client = await getClient();
  return client.getMatrix(integratorId);
}

export async function savePermissions(request: SavePermissionsRequest): Promise<void> {
  const client = await getClient();
  return client.savePermissions(request);
}

// Test/fixture exports
export const __TEST__ = {
  CURRENT_INTEGRATOR_ID: 'integrator-001',
  /** Reset client instance for test isolation. */
  resetClient: () => { clientInstance = null; },
};
