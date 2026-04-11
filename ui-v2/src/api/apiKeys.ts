// KAI-319: API Keys Management client (Integrator Portal).
//
// Typed API client for the integrator-facing API key lifecycle:
//   - listKeys(tenantId)
//   - createKey(args)
//   - rotateKey(args)
//   - revokeKey(tenantId, keyId)
//   - getKeyAuditLog(tenantId, keyId)
//
// Wired to the real backend endpoints at /api/v1/api-keys.

import { API_BASE_URL } from './client';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type KeyStatus = 'active' | 'expiring' | 'expired' | 'revoked' | 'grace';

export interface APIKeyScope {
  readonly value: string;
  readonly label: string;
}

export interface APIKeyRecord {
  readonly id: string;
  readonly tenantId: string;
  readonly name: string;
  readonly keyPrefix: string;
  readonly scopes: readonly string[];
  readonly status: KeyStatus;
  readonly createdBy: string;
  readonly createdAt: string;
  readonly expiresAt: string | null;
  readonly revokedAt: string | null;
  readonly lastUsedAt: string | null;
  readonly rotatedFromId: string | null;
  readonly graceExpiresAt: string | null;
}

export interface CreateKeyArgs {
  readonly tenantId: string;
  readonly name: string;
  readonly scopes: readonly string[];
  readonly expiresInDays: number | null; // null = no expiry
}

export interface CreateKeyResult {
  readonly rawKey: string;
  readonly key: APIKeyRecord;
}

export interface RotateKeyArgs {
  readonly tenantId: string;
  readonly keyId: string;
  readonly gracePeriodHours: number; // default 24
}

export interface RotateKeyResult {
  readonly rawKey: string;
  readonly newKey: APIKeyRecord;
  readonly oldKeyGraceEnd: string;
}

export interface AuditLogEntry {
  readonly id: string;
  readonly keyId: string;
  readonly action: 'create' | 'rotate' | 'revoke' | 'authenticate' | 'auth_fail';
  readonly actorId: string;
  readonly ipAddress: string;
  readonly userAgent: string;
  readonly metadata: string;
  readonly createdAt: string;
}

// ---------------------------------------------------------------------------
// Available scopes (mirrors Go backend publicapi.APIKey.Scopes)
// ---------------------------------------------------------------------------

export const AVAILABLE_SCOPES: readonly APIKeyScope[] = [
  { value: 'cameras:read', label: 'apiKeys.scopes.camerasRead' },
  { value: 'cameras:write', label: 'apiKeys.scopes.camerasWrite' },
  { value: 'cameras:*', label: 'apiKeys.scopes.camerasAll' },
  { value: 'recordings:read', label: 'apiKeys.scopes.recordingsRead' },
  { value: 'recordings:write', label: 'apiKeys.scopes.recordingsWrite' },
  { value: 'recordings:*', label: 'apiKeys.scopes.recordingsAll' },
  { value: 'events:read', label: 'apiKeys.scopes.eventsRead' },
  { value: 'events:*', label: 'apiKeys.scopes.eventsAll' },
  { value: 'users:read', label: 'apiKeys.scopes.usersRead' },
  { value: 'users:write', label: 'apiKeys.scopes.usersWrite' },
  { value: 'users:*', label: 'apiKeys.scopes.usersAll' },
  { value: 'streams:read', label: 'apiKeys.scopes.streamsRead' },
  { value: 'streams:*', label: 'apiKeys.scopes.streamsAll' },
];

// ---------------------------------------------------------------------------
// Client interface
// ---------------------------------------------------------------------------

export interface APIKeysClient {
  listKeys(tenantId: string): Promise<readonly APIKeyRecord[]>;
  createKey(args: CreateKeyArgs): Promise<CreateKeyResult>;
  rotateKey(args: RotateKeyArgs): Promise<RotateKeyResult>;
  revokeKey(tenantId: string, keyId: string): Promise<void>;
  getKeyAuditLog(tenantId: string, keyId: string): Promise<readonly AuditLogEntry[]>;
}

// ---------------------------------------------------------------------------
// Query key factory (tenant-scoped)
// ---------------------------------------------------------------------------

export const API_KEYS_QUERY_KEY = 'apiKeys' as const;

export const apiKeysQueryKeys = {
  all: (tenantId: string) => [API_KEYS_QUERY_KEY, tenantId] as const,
  list: (tenantId: string) => [API_KEYS_QUERY_KEY, tenantId, 'list'] as const,
  auditLog: (tenantId: string, keyId: string) =>
    [API_KEYS_QUERY_KEY, tenantId, 'audit', keyId] as const,
};

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

async function fetchJson<T>(url: string, init?: RequestInit): Promise<T> {
  const res = await fetch(url, init);
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(`${res.status} ${text}`);
  }
  return res.json() as Promise<T>;
}

// ---------------------------------------------------------------------------
// Public API -- direct fetch calls to real endpoints
// ---------------------------------------------------------------------------

export async function listKeys(tenantId: string): Promise<readonly APIKeyRecord[]> {
  return fetchJson<APIKeyRecord[]>(
    `${API_BASE_URL}/api-keys?tenantId=${encodeURIComponent(tenantId)}`,
  );
}

export async function createKey(args: CreateKeyArgs): Promise<CreateKeyResult> {
  return fetchJson<CreateKeyResult>(`${API_BASE_URL}/api-keys`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(args),
  });
}

export async function rotateKey(args: RotateKeyArgs): Promise<RotateKeyResult> {
  return fetchJson<RotateKeyResult>(
    `${API_BASE_URL}/api-keys/${encodeURIComponent(args.keyId)}/rotate`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(args),
    },
  );
}

export async function revokeKey(tenantId: string, keyId: string): Promise<void> {
  const res = await fetch(
    `${API_BASE_URL}/api-keys/${encodeURIComponent(keyId)}/revoke`,
    {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ tenantId }),
    },
  );
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(`${res.status} ${text}`);
  }
}

export async function getKeyAuditLog(
  tenantId: string,
  keyId: string,
): Promise<readonly AuditLogEntry[]> {
  return fetchJson<AuditLogEntry[]>(
    `${API_BASE_URL}/api-keys/${encodeURIComponent(keyId)}/audit?tenantId=${encodeURIComponent(tenantId)}`,
  );
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

export function deriveKeyStatus(key: APIKeyRecord): KeyStatus {
  if (key.revokedAt) return 'revoked';
  if (key.graceExpiresAt) {
    const graceEnd = new Date(key.graceExpiresAt);
    if (graceEnd > new Date()) return 'grace';
    return 'expired';
  }
  if (key.expiresAt) {
    const expires = new Date(key.expiresAt);
    const now = new Date();
    if (expires < now) return 'expired';
    // Within 7 days of expiry
    const weekFromNow = new Date(now.getTime() + 7 * 24 * 60 * 60 * 1000);
    if (expires < weekFromNow) return 'expiring';
  }
  return 'active';
}

// Test constants
export const CURRENT_TENANT_ID = 'tenant-001';

export const __TEST__ = {
  CURRENT_TENANT_ID,
  /** No-op: lazy mock client removed; kept for test compatibility. */
  resetClient: () => {},
};
