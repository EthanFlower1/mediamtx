// KAI-311: Mobile App Builds API client.
//
// Types, client interface, query-key factory, and feature-flag bootstrap
// for the Integrator Portal Mobile App Builds page.
//
// Real implementation will use Connect-Go clients generated from protos.
// Until then this file imports a deterministic mock (lazy, behind a
// feature flag) so the page is fully functional in dev.

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type BuildPlatform = 'ios' | 'android';

export type BuildStatus = 'queued' | 'building' | 'succeeded' | 'failed';

export type VersionBumpType = 'patch' | 'minor' | 'major';

export type CredentialConnectionStatus = 'connected' | 'expired' | 'missing';

export type PlayConsoleTrack = 'internal' | 'alpha' | 'beta' | 'production';

export interface BuildArtifact {
  readonly id: string;
  readonly fileName: string;
  readonly sizeBytes: number;
  readonly downloadUrl: string;
  /** IPA, APK, or AAB */
  readonly format: 'ipa' | 'apk' | 'aab';
}

export interface MobileBuild {
  readonly id: string;
  readonly integratorId: string;
  readonly platform: BuildPlatform;
  readonly version: string;
  readonly status: BuildStatus;
  readonly brandConfigName: string;
  readonly createdAtIso: string;
  readonly finishedAtIso: string | null;
  readonly artifacts: readonly BuildArtifact[];
  readonly buildLogs: string;
  readonly signingCertStatus: CredentialConnectionStatus;
  readonly distributionUploadStatus: 'pending' | 'uploaded' | 'failed' | 'n/a';
  readonly releaseNotes: string;
}

export interface BuildTriggerArgs {
  readonly integratorId: string;
  readonly platform: BuildPlatform | 'both';
  readonly brandConfigName: string;
  readonly versionBump: VersionBumpType;
  readonly releaseNotes?: string;
}

export interface AppleCredential {
  readonly teamId: string;
  readonly teamName: string;
  readonly status: CredentialConnectionStatus;
  readonly expiresAtIso: string | null;
}

export interface GoogleCredential {
  readonly serviceAccountEmail: string;
  readonly projectId: string;
  readonly status: CredentialConnectionStatus;
  readonly expiresAtIso: string | null;
}

export interface DistributionConfig {
  readonly integratorId: string;
  readonly testFlightGroupName: string;
  readonly playConsoleTrack: PlayConsoleTrack;
  readonly autoSubmit: boolean;
}

export interface MobileBuildsSnapshot {
  readonly integratorId: string;
  readonly builds: readonly MobileBuild[];
  readonly appleCredential: AppleCredential;
  readonly googleCredential: GoogleCredential;
  readonly distributionConfig: DistributionConfig;
  readonly availableBrandConfigs: readonly string[];
}

// ---------------------------------------------------------------------------
// Client interface
// ---------------------------------------------------------------------------

export interface MobileBuildsClient {
  listBuilds(integratorId: string): Promise<MobileBuildsSnapshot>;
  triggerBuild(args: BuildTriggerArgs): Promise<MobileBuild>;
}

// ---------------------------------------------------------------------------
// Query-key factory — integrator/tenant-scoped
// ---------------------------------------------------------------------------

export const mobileBuildsQueryKeys = {
  all: (integratorId: string) => ['mobileBuilds', integratorId] as const,
  list: (integratorId: string) => ['mobileBuilds', integratorId, 'list'] as const,
  detail: (integratorId: string, buildId: string) =>
    ['mobileBuilds', integratorId, 'detail', buildId] as const,
};

// ---------------------------------------------------------------------------
// Feature-flag bootstrap with lazy mock import
// ---------------------------------------------------------------------------

let _client: MobileBuildsClient | null = null;

async function ensureClient(): Promise<MobileBuildsClient> {
  if (_client) return _client;
  // Feature flag: always use mock until Connect-Go protos land.
  const mod = await import('./mobileBuilds.mock');
  _client = mod.mockMobileBuildsClient;
  return _client;
}

export async function listMobileBuilds(integratorId: string): Promise<MobileBuildsSnapshot> {
  const client = await ensureClient();
  return client.listBuilds(integratorId);
}

export async function triggerMobileBuild(args: BuildTriggerArgs): Promise<MobileBuild> {
  const client = await ensureClient();
  return client.triggerBuild(args);
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

export const CURRENT_INTEGRATOR_ID = 'integrator-001';

export const __TEST__ = {
  CURRENT_INTEGRATOR_ID,
};
