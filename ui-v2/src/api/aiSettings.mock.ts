// KAI-327: Deterministic in-memory mock for aiSettings.ts.
//
// Scaffolding only. The production client (KAI-399 Connect-Go) will
// register itself via `registerAiSettingsClient` at boot, making this
// module unreachable in production bundles.

import type {
  AiFeatureKey,
  AiFeatureState,
  AiSettingsClient,
  FaceRecognitionEnableAck,
  TransparencyReport,
} from './aiSettings';

const FEATURE_KEYS: AiFeatureKey[] = [
  'object-detection',
  'face-recognition',
  'lpr',
  'behavioral-analytics',
  'semantic-search',
  'custom-models',
];

const REQUIRES_ENTITLEMENT: Record<AiFeatureKey, boolean> = {
  'object-detection': false,
  'face-recognition': true,
  lpr: true,
  'behavioral-analytics': true,
  'semantic-search': true,
  'custom-models': true,
};

function pseudoIndex(seed: string): number {
  let h = 0;
  for (let i = 0; i < seed.length; i++) {
    h = (h * 31 + seed.charCodeAt(i)) | 0;
  }
  return Math.abs(h);
}

const tenantStates: Map<string, Map<AiFeatureKey, AiFeatureState>> = new Map();

function buildDefaults(tenantId: string): Map<AiFeatureKey, AiFeatureState> {
  const now = new Date().toISOString();
  const offset = pseudoIndex(tenantId);
  const entitlementBitmap = offset % 8;
  const map = new Map<AiFeatureKey, AiFeatureState>();
  FEATURE_KEYS.forEach((key, i) => {
    const requiresEntitlement = REQUIRES_ENTITLEMENT[key];
    const entitled = !requiresEntitlement || ((entitlementBitmap >> i) & 1) === 0;
    map.set(key, {
      key,
      // Privacy-preserving default: face-recognition is OFF by default.
      enabled: key === 'object-detection',
      entitled,
      requiresEntitlement,
      updatedAt: now,
      updatedBy: 'system',
    });
  });
  return map;
}

function getTenantState(tenantId: string): Map<AiFeatureKey, AiFeatureState> {
  let state = tenantStates.get(tenantId);
  if (!state) {
    state = buildDefaults(tenantId);
    tenantStates.set(tenantId, state);
  }
  return state;
}

export const aiSettingsMockClient: AiSettingsClient = {
  async listFeatures(tenantId: string): Promise<AiFeatureState[]> {
    await Promise.resolve();
    const state = getTenantState(tenantId);
    return FEATURE_KEYS.map((k) => state.get(k)!);
  },

  async setFeatureEnabled(
    tenantId: string,
    key: AiFeatureKey,
    enabled: boolean,
    ack?: FaceRecognitionEnableAck,
  ): Promise<AiFeatureState> {
    await Promise.resolve();
    const state = getTenantState(tenantId);
    const current = state.get(key);
    if (!current) {
      throw new Error(`unknown ai feature key: ${key}`);
    }
    // TODO(lead-security): enforce that enabling face-recognition without a
    // fully populated FaceRecognitionEnableAck is rejected server-side; the
    // UI already blocks the flow but defence in depth is required.
    if (key === 'face-recognition' && enabled && !ack) {
      throw new Error('face-recognition enable requires acknowledgement');
    }
    const updated: AiFeatureState = {
      ...current,
      enabled,
      updatedAt: new Date().toISOString(),
      updatedBy: 'mock-admin',
    };
    state.set(key, updated);
    return updated;
  },

  async downloadTransparencyReport(tenantId: string): Promise<TransparencyReport> {
    await Promise.resolve();
    return {
      tenantId,
      generatedAt: new Date().toISOString(),
      downloadHandle: `mock-report-${tenantId}-${Date.now()}`,
    };
  },
};

/** Test helper: clear the in-memory tenant state so tests are isolated. */
export function __resetAiSettingsMockStateForTests(): void {
  tenantStates.clear();
}
