// KAI-327: AI feature-toggles API client.
//
// Scaffolding only: production file exports typed interface + bootstrap
// hooks and thin wrappers. Real Connect-Go client (KAI-399) will replace
// the lazy-loaded mock at app boot via `registerAiSettingsClient`.
//
// Split pattern (KAI-149 / KAI-323): the mock lives in a sibling module
// that is lazy-imported only when no production client has registered
// itself, so production bundles pay nothing for the mock.
//
// TODO(lead-security): confirm the final list of compliance-gated feature
// keys + the required user-acknowledgement payload for face-recognition
// enablement (EU AI Act Art 27 + GDPR Art 9).

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type AiFeatureKey =
  | 'object-detection'
  | 'face-recognition'
  | 'lpr'
  | 'behavioral-analytics'
  | 'semantic-search'
  | 'custom-models';

export interface AiFeatureState {
  key: AiFeatureKey;
  enabled: boolean;
  entitled: boolean;
  requiresEntitlement: boolean;
  updatedAt: string; // ISO-8601
  updatedBy: string;
}

/**
 * Acknowledgement payload required when enabling face-recognition.
 * The UI captures this in a modal; the backend is expected to audit-log
 * every field. See `TODO(lead-security)` above.
 */
export interface FaceRecognitionEnableAck {
  lawfulBasisConfirmed: boolean;
  aiActImpactAssessmentCompleted: boolean;
  acknowledgedAt: string;
  acknowledgedBy: string;
}

export interface TransparencyReport {
  tenantId: string;
  generatedAt: string;
  /** Opaque blob identifier — the UI treats this as a download handle. */
  downloadHandle: string;
}

export interface AiSettingsClient {
  listFeatures(tenantId: string): Promise<AiFeatureState[]>;
  setFeatureEnabled(
    tenantId: string,
    key: AiFeatureKey,
    enabled: boolean,
    ack?: FaceRecognitionEnableAck,
  ): Promise<AiFeatureState>;
  downloadTransparencyReport(tenantId: string): Promise<TransparencyReport>;
}

// ---------------------------------------------------------------------------
// Query-key factory
// ---------------------------------------------------------------------------

export const aiSettingsQueryKeys = {
  all: (tenantId: string) => ['aiSettings', tenantId] as const,
  features: (tenantId: string) => ['aiSettings', tenantId, 'features'] as const,
};

// ---------------------------------------------------------------------------
// Client bootstrap (production client registers itself at app boot)
// ---------------------------------------------------------------------------

let activeClient: AiSettingsClient | null = null;

export function registerAiSettingsClient(client: AiSettingsClient): void {
  activeClient = client;
}

export function resetAiSettingsClientForTests(): void {
  activeClient = null;
}

async function getClient(): Promise<AiSettingsClient> {
  if (activeClient) return activeClient;
  const { aiSettingsMockClient } = await import('./aiSettings.mock');
  return aiSettingsMockClient;
}

// ---------------------------------------------------------------------------
// Thin wrappers — call-sites import these, never touch getClient() directly
// ---------------------------------------------------------------------------

export async function listAiFeatures(tenantId: string): Promise<AiFeatureState[]> {
  return (await getClient()).listFeatures(tenantId);
}

export async function setAiFeatureEnabled(
  tenantId: string,
  key: AiFeatureKey,
  enabled: boolean,
  ack?: FaceRecognitionEnableAck,
): Promise<AiFeatureState> {
  return (await getClient()).setFeatureEnabled(tenantId, key, enabled, ack);
}

export async function downloadTransparencyReport(
  tenantId: string,
): Promise<TransparencyReport> {
  return (await getClient()).downloadTransparencyReport(tenantId);
}
