// KAI-322: Recorders API client (stub).
//
// All functions are promise-based stubs with realistic response shapes.
// The real Connect-Go generated client (KAI-226) will replace these
// function bodies — signatures stay identical, so migration is mechanical.
//
// TODO(KAI-243): POST /api/v1/pairing/tokens — plug the real endpoint response
//   in once KAI-243 lands on main. The PairingToken shape below mirrors the
//   proto schema from the KAI-243 PR description.
// TODO(KAI-226): GET/DELETE /api/v1/recorders and GET /api/v1/recorders/:id —
//   replace stub bodies with the generated Connect-Go client calls.

export type RecorderHealth = 'online' | 'degraded' | 'offline';

export interface SidecarStatus {
  name: 'mediamtx' | 'zitadel' | 'step-ca';
  healthy: boolean;
  message: string;
}

export interface StorageUsage {
  usedBytes: number;
  totalBytes: number;
}

export interface HardwareSummary {
  cpuModel: string;
  cpuCores: number;
  ramBytes: number;
  diskModel: string;
}

export interface Recorder {
  id: string;
  tenantId: string;
  name: string;
  hardware: HardwareSummary;
  health: RecorderHealth;
  cameraCount: number;
  lastCheckIn: string; // ISO-8601
  storage: StorageUsage;
  version: string;
  ipAddress: string;
}

export interface RecorderDetail extends Recorder {
  pairedCameraIds: string[];
  sidecars: SidecarStatus[];
  recentLogs: string[];
}

export interface PairingToken {
  token: string;
  expiresAt: string; // ISO-8601
  redeemed: boolean;
}

export interface RecorderListFilters {
  tenantId: string;
  search?: string;
  health?: RecorderHealth | 'all';
}

// -----------------------------------------------------------------
// Deterministic mock data
// -----------------------------------------------------------------

const HARDWARE_SAMPLES: HardwareSummary[] = [
  { cpuModel: 'Intel Core i7-12700', cpuCores: 12, ramBytes: 32 * 1024 ** 3, diskModel: 'Samsung 870 QVO 8TB' },
  { cpuModel: 'Intel Core i5-12400', cpuCores: 6, ramBytes: 16 * 1024 ** 3, diskModel: 'Seagate IronWolf 4TB' },
  { cpuModel: 'AMD Ryzen 7 5700G', cpuCores: 8, ramBytes: 64 * 1024 ** 3, diskModel: 'WD Red Pro 12TB' },
];

const HEALTH_PATTERN: RecorderHealth[] = [
  'online', 'online', 'online', 'online', 'degraded', 'offline',
];

function buildRecorders(tenantId: string, count: number): Recorder[] {
  const now = Date.now();
  return Array.from({ length: count }, (_, i) => {
    const hw = HARDWARE_SAMPLES[i % HARDWARE_SAMPLES.length]!;
    const health = HEALTH_PATTERN[i % HEALTH_PATTERN.length]!;
    const usedFraction = 0.3 + (i % 5) * 0.12;
    return {
      id: `rec-${tenantId}-${i.toString().padStart(3, '0')}`,
      tenantId,
      name: `Recorder ${(i + 1).toString().padStart(2, '0')}`,
      hardware: hw,
      health,
      cameraCount: 4 + (i % 12),
      lastCheckIn: new Date(now - (i * 2 + 1) * 60_000).toISOString(),
      storage: {
        usedBytes: Math.round(hw.diskModel.includes('12TB') ? 12e12 * usedFraction : hw.diskModel.includes('8TB') ? 8e12 * usedFraction : 4e12 * usedFraction),
        totalBytes: hw.diskModel.includes('12TB') ? 12e12 : hw.diskModel.includes('8TB') ? 8e12 : 4e12,
      },
      version: '1.0.0',
      ipAddress: `10.0.1.${10 + i}`,
    };
  });
}

// In-memory token store so revoke works within a session.
const _tokens: Map<string, PairingToken[]> = new Map();

function getTokensFor(tenantId: string): PairingToken[] {
  if (!_tokens.has(tenantId)) _tokens.set(tenantId, []);
  return _tokens.get(tenantId)!;
}

// -----------------------------------------------------------------
// Public API
// -----------------------------------------------------------------

export async function listRecorders(filters: RecorderListFilters): Promise<Recorder[]> {
  await Promise.resolve();
  let recs = buildRecorders(filters.tenantId, 6);
  if (filters.health && filters.health !== 'all') {
    recs = recs.filter((r) => r.health === filters.health);
  }
  if (filters.search && filters.search.trim().length > 0) {
    const q = filters.search.trim().toLowerCase();
    recs = recs.filter(
      (r) =>
        r.name.toLowerCase().includes(q) ||
        r.ipAddress.toLowerCase().includes(q),
    );
  }
  return recs;
}

export async function getRecorder(tenantId: string, id: string): Promise<RecorderDetail> {
  await Promise.resolve();
  const base = buildRecorders(tenantId, 6).find((r) => r.id === id);
  if (!base) throw new Error(`Recorder ${id} not found`);
  return {
    ...base,
    pairedCameraIds: Array.from({ length: base.cameraCount }, (_, i) => `cam-${tenantId}-${i.toString().padStart(3, '0')}`),
    sidecars: [
      { name: 'mediamtx', healthy: base.health !== 'offline', message: base.health !== 'offline' ? 'Running' : 'Process not found' },
      { name: 'zitadel', healthy: base.health === 'online', message: base.health === 'online' ? 'Running' : 'Unhealthy' },
      { name: 'step-ca', healthy: base.health === 'online', message: base.health === 'online' ? 'Running' : 'Unreachable' },
    ],
    recentLogs: [
      `[${new Date().toISOString()}] INFO recorder started`,
      `[${new Date(Date.now() - 60_000).toISOString()}] INFO camera stream opened`,
      `[${new Date(Date.now() - 120_000).toISOString()}] INFO health check passed`,
    ],
  };
}

export async function unpairRecorder(tenantId: string, id: string): Promise<void> {
  // TODO(KAI-226): DELETE /api/v1/recorders/:id
  await Promise.resolve();
  void tenantId;
  void id;
}

export interface CreatePairingTokenArgs {
  tenantId: string;
}

export async function createPairingToken(args: CreatePairingTokenArgs): Promise<PairingToken> {
  // TODO(KAI-243): POST /api/v1/pairing/tokens — replace with real endpoint call.
  await Promise.resolve();
  const token: PairingToken = {
    token: `kpair-${crypto.randomUUID().replace(/-/g, '').slice(0, 32)}`,
    expiresAt: new Date(Date.now() + 30 * 60_000).toISOString(), // 30-minute TTL
    redeemed: false,
  };
  getTokensFor(args.tenantId).push(token);
  return token;
}

export async function listPairingTokens(tenantId: string): Promise<PairingToken[]> {
  // TODO(KAI-243): GET /api/v1/pairing/tokens — replace with real endpoint call.
  await Promise.resolve();
  return getTokensFor(tenantId);
}

export async function revokePairingToken(tenantId: string, token: string): Promise<void> {
  // TODO(KAI-243): DELETE /api/v1/pairing/tokens/:token — replace with real endpoint call.
  await Promise.resolve();
  const list = getTokensFor(tenantId);
  const idx = list.findIndex((t) => t.token === token);
  if (idx !== -1) list.splice(idx, 1);
}

export const recordersQueryKeys = {
  all: (tenantId: string) => ['recorders', tenantId] as const,
  list: (tenantId: string, filters: Omit<RecorderListFilters, 'tenantId'>) =>
    ['recorders', tenantId, 'list', filters] as const,
  detail: (tenantId: string, id: string) =>
    ['recorders', tenantId, 'detail', id] as const,
  tokens: (tenantId: string) => ['recorders', tenantId, 'tokens'] as const,
};
