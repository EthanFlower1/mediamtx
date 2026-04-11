// KAI-319: Deterministic mock implementation for API Keys Management.
//
// 4 keys with mixed statuses, deterministic data so tests are stable.
// Mutable in-memory store so mutations (create, rotate, revoke) are
// reflected within the same session.

import type {
  APIKeysClient,
  APIKeyRecord,
  CreateKeyArgs,
  CreateKeyResult,
  RotateKeyArgs,
  RotateKeyResult,
  AuditLogEntry,
} from './apiKeys';

// ---------------------------------------------------------------------------
// Deterministic mock dataset
// ---------------------------------------------------------------------------

const TENANT_ID = 'tenant-001';

function buildMockKeys(): APIKeyRecord[] {
  const now = Date.UTC(2026, 3, 8, 12, 0, 0);
  return [
    {
      id: 'key-001',
      tenantId: TENANT_ID,
      name: 'Production Integration',
      keyPrefix: 'kvue_a1b',
      scopes: ['cameras:read', 'recordings:read', 'events:read'],
      status: 'active',
      createdBy: 'alice@integrator.example',
      createdAt: new Date(now - 90 * 86_400_000).toISOString(),
      expiresAt: new Date(now + 275 * 86_400_000).toISOString(),
      revokedAt: null,
      lastUsedAt: new Date(now - 300_000).toISOString(),
      rotatedFromId: null,
      graceExpiresAt: null,
    },
    {
      id: 'key-002',
      tenantId: TENANT_ID,
      name: 'Staging Environment',
      keyPrefix: 'kvue_c3d',
      scopes: ['cameras:*', 'recordings:*'],
      status: 'active',
      createdBy: 'bob@integrator.example',
      createdAt: new Date(now - 30 * 86_400_000).toISOString(),
      expiresAt: null,
      revokedAt: null,
      lastUsedAt: new Date(now - 7_200_000).toISOString(),
      rotatedFromId: null,
      graceExpiresAt: null,
    },
    {
      id: 'key-003',
      tenantId: TENANT_ID,
      name: 'Expiring Soon Key',
      keyPrefix: 'kvue_e5f',
      scopes: ['cameras:read'],
      status: 'expiring',
      createdBy: 'carol@integrator.example',
      createdAt: new Date(now - 358 * 86_400_000).toISOString(),
      expiresAt: new Date(now + 3 * 86_400_000).toISOString(),
      revokedAt: null,
      lastUsedAt: new Date(now - 86_400_000).toISOString(),
      rotatedFromId: null,
      graceExpiresAt: null,
    },
    {
      id: 'key-004',
      tenantId: TENANT_ID,
      name: 'Revoked Legacy Key',
      keyPrefix: 'kvue_g7h',
      scopes: [],
      status: 'revoked',
      createdBy: 'alice@integrator.example',
      createdAt: new Date(now - 200 * 86_400_000).toISOString(),
      expiresAt: null,
      revokedAt: new Date(now - 10 * 86_400_000).toISOString(),
      lastUsedAt: new Date(now - 15 * 86_400_000).toISOString(),
      rotatedFromId: null,
      graceExpiresAt: null,
    },
  ];
}

function buildMockAuditLog(): Map<string, AuditLogEntry[]> {
  const now = Date.UTC(2026, 3, 8, 12, 0, 0);
  const entries = new Map<string, AuditLogEntry[]>();

  entries.set('key-001', [
    {
      id: 'audit-001',
      keyId: 'key-001',
      action: 'create',
      actorId: 'alice@integrator.example',
      ipAddress: '192.168.1.100',
      userAgent: 'Mozilla/5.0',
      metadata: '{}',
      createdAt: new Date(now - 90 * 86_400_000).toISOString(),
    },
    {
      id: 'audit-002',
      keyId: 'key-001',
      action: 'authenticate',
      actorId: 'apikey:key-001',
      ipAddress: '10.0.0.50',
      userAgent: 'KaiVue-SDK/1.0',
      metadata: '{}',
      createdAt: new Date(now - 300_000).toISOString(),
    },
  ]);

  entries.set('key-004', [
    {
      id: 'audit-003',
      keyId: 'key-004',
      action: 'create',
      actorId: 'alice@integrator.example',
      ipAddress: '192.168.1.100',
      userAgent: 'Mozilla/5.0',
      metadata: '{}',
      createdAt: new Date(now - 200 * 86_400_000).toISOString(),
    },
    {
      id: 'audit-004',
      keyId: 'key-004',
      action: 'revoke',
      actorId: 'alice@integrator.example',
      ipAddress: '192.168.1.100',
      userAgent: 'Mozilla/5.0',
      metadata: '{}',
      createdAt: new Date(now - 10 * 86_400_000).toISOString(),
    },
  ]);

  return entries;
}

// Mutable in-memory stores.
const keyStore: Map<string, APIKeyRecord[]> = new Map();
const auditStore: Map<string, AuditLogEntry[]> = new Map();
let createCounter = 0;

function getKeys(tenantId: string): APIKeyRecord[] {
  if (!keyStore.has(tenantId)) {
    keyStore.set(tenantId, tenantId === TENANT_ID ? buildMockKeys() : []);
  }
  return keyStore.get(tenantId)!;
}

function getAudit(keyId: string): AuditLogEntry[] {
  if (!auditStore.size) {
    const seed = buildMockAuditLog();
    for (const [k, v] of seed) {
      auditStore.set(k, v);
    }
  }
  if (!auditStore.has(keyId)) {
    auditStore.set(keyId, []);
  }
  return auditStore.get(keyId)!;
}

function addAuditEntry(
  keyId: string,
  tenantId: string,
  action: AuditLogEntry['action'],
  actorId: string,
  metadata: string = '{}',
): void {
  const entries = getAudit(keyId);
  entries.push({
    id: `audit-${Date.now()}-${Math.random().toString(36).slice(2, 6)}`,
    keyId,
    action,
    actorId: actorId,
    ipAddress: '127.0.0.1',
    userAgent: 'KaiVue-Portal/1.0',
    metadata,
    createdAt: new Date().toISOString(),
  });
}

// ---------------------------------------------------------------------------
// Mock client implementation
// ---------------------------------------------------------------------------

export const mockAPIKeysClient: APIKeysClient = {
  async listKeys(tenantId: string): Promise<readonly APIKeyRecord[]> {
    await Promise.resolve();
    return getKeys(tenantId);
  },

  async createKey(args: CreateKeyArgs): Promise<CreateKeyResult> {
    await Promise.resolve();
    createCounter += 1;
    const now = new Date();

    let expiresAt: string | null = null;
    if (args.expiresInDays !== null && args.expiresInDays > 0) {
      expiresAt = new Date(now.getTime() + args.expiresInDays * 86_400_000).toISOString();
    }

    const rawKey = `kvue_${Array.from({ length: 40 }, () =>
      '0123456789abcdef'.charAt(Math.floor(Math.random() * 16)),
    ).join('')}`;

    const key: APIKeyRecord = {
      id: `key-new-${createCounter}`,
      tenantId: args.tenantId,
      name: args.name,
      keyPrefix: rawKey.slice(0, 8),
      scopes: [...args.scopes],
      status: 'active',
      createdBy: 'current-user@integrator.example',
      createdAt: now.toISOString(),
      expiresAt,
      revokedAt: null,
      lastUsedAt: null,
      rotatedFromId: null,
      graceExpiresAt: null,
    };

    getKeys(args.tenantId).unshift(key);
    addAuditEntry(key.id, args.tenantId, 'create', 'current-user@integrator.example');

    return { rawKey, key };
  },

  async rotateKey(args: RotateKeyArgs): Promise<RotateKeyResult> {
    await Promise.resolve();
    const keys = getKeys(args.tenantId);
    const idx = keys.findIndex((k) => k.id === args.keyId);
    if (idx < 0) throw new Error(`Key ${args.keyId} not found`);
    const oldKey = keys[idx]!;

    if (oldKey.revokedAt) throw new Error('Cannot rotate revoked key');

    const graceHours = args.gracePeriodHours || 24;
    const now = new Date();
    const graceEnd = new Date(now.getTime() + graceHours * 3_600_000);

    // Mark old key with grace period.
    keys[idx] = { ...oldKey, graceExpiresAt: graceEnd.toISOString(), status: 'grace' };

    const rawKey = `kvue_${Array.from({ length: 40 }, () =>
      '0123456789abcdef'.charAt(Math.floor(Math.random() * 16)),
    ).join('')}`;

    createCounter += 1;
    const newKey: APIKeyRecord = {
      id: `key-new-${createCounter}`,
      tenantId: args.tenantId,
      name: oldKey.name,
      keyPrefix: rawKey.slice(0, 8),
      scopes: [...oldKey.scopes],
      status: 'active',
      createdBy: 'current-user@integrator.example',
      createdAt: now.toISOString(),
      expiresAt: oldKey.expiresAt,
      revokedAt: null,
      lastUsedAt: null,
      rotatedFromId: oldKey.id,
      graceExpiresAt: null,
    };

    keys.unshift(newKey);

    const meta = JSON.stringify({
      old_key_id: oldKey.id,
      new_key_id: newKey.id,
      grace_period_seconds: graceHours * 3600,
    });
    addAuditEntry(oldKey.id, args.tenantId, 'rotate', 'current-user@integrator.example', meta);
    addAuditEntry(newKey.id, args.tenantId, 'create', 'current-user@integrator.example', meta);

    return {
      rawKey,
      newKey,
      oldKeyGraceEnd: graceEnd.toISOString(),
    };
  },

  async revokeKey(tenantId: string, keyId: string): Promise<void> {
    await Promise.resolve();
    const keys = getKeys(tenantId);
    const idx = keys.findIndex((k) => k.id === keyId);
    if (idx < 0) throw new Error(`Key ${keyId} not found`);
    const old = keys[idx]!;
    if (old.revokedAt) throw new Error('Key already revoked');

    keys[idx] = { ...old, revokedAt: new Date().toISOString(), status: 'revoked' };
    addAuditEntry(keyId, tenantId, 'revoke', 'current-user@integrator.example');
  },

  async getKeyAuditLog(
    _tenantId: string,
    keyId: string,
  ): Promise<readonly AuditLogEntry[]> {
    await Promise.resolve();
    return getAudit(keyId);
  },
};

// Test helpers
export const __TEST__ = {
  TENANT_ID,
  MOCK_KEYS: buildMockKeys(),
  resetStores: () => {
    keyStore.clear();
    auditStore.clear();
    createCounter = 0;
  },
};
