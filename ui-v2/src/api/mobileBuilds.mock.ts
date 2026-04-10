// KAI-311: Deterministic mock data for Mobile App Builds.
//
// 5 builds with mixed statuses, deterministic timestamps, and realistic
// artifact data. Used by the feature-flag bootstrap in mobileBuilds.ts.

import type {
  MobileBuild,
  MobileBuildsClient,
  MobileBuildsSnapshot,
  BuildTriggerArgs,
  AppleCredential,
  GoogleCredential,
  DistributionConfig,
} from './mobileBuilds';

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const INTEGRATOR_ID = 'integrator-001';

const MOCK_BUILDS: readonly MobileBuild[] = [
  {
    id: 'build-001',
    integratorId: INTEGRATOR_ID,
    platform: 'ios',
    version: '2.4.1',
    status: 'succeeded',
    brandConfigName: 'Acme Security',
    createdAtIso: '2026-04-07T10:00:00.000Z',
    finishedAtIso: '2026-04-07T10:18:32.000Z',
    artifacts: [
      {
        id: 'art-001',
        fileName: 'AcmeSecurity-2.4.1.ipa',
        sizeBytes: 48_200_000,
        downloadUrl: '/api/v1/builds/build-001/artifacts/art-001',
        format: 'ipa',
      },
    ],
    buildLogs:
      '[10:00:00] Starting iOS build for Acme Security v2.4.1\n' +
      '[10:02:15] Installing CocoaPods dependencies...\n' +
      '[10:05:30] Compiling Swift sources...\n' +
      '[10:12:00] Signing with distribution certificate...\n' +
      '[10:15:45] Archiving IPA...\n' +
      '[10:18:32] Build succeeded. Artifact: AcmeSecurity-2.4.1.ipa (48.2 MB)',
    signingCertStatus: 'connected',
    distributionUploadStatus: 'uploaded',
    releaseNotes: 'Bug fixes and performance improvements.',
  },
  {
    id: 'build-002',
    integratorId: INTEGRATOR_ID,
    platform: 'android',
    version: '2.4.1',
    status: 'succeeded',
    brandConfigName: 'Acme Security',
    createdAtIso: '2026-04-07T10:00:00.000Z',
    finishedAtIso: '2026-04-07T10:14:10.000Z',
    artifacts: [
      {
        id: 'art-002',
        fileName: 'AcmeSecurity-2.4.1.apk',
        sizeBytes: 35_800_000,
        downloadUrl: '/api/v1/builds/build-002/artifacts/art-002',
        format: 'apk',
      },
      {
        id: 'art-003',
        fileName: 'AcmeSecurity-2.4.1.aab',
        sizeBytes: 32_100_000,
        downloadUrl: '/api/v1/builds/build-002/artifacts/art-003',
        format: 'aab',
      },
    ],
    buildLogs:
      '[10:00:00] Starting Android build for Acme Security v2.4.1\n' +
      '[10:02:00] Resolving Gradle dependencies...\n' +
      '[10:06:30] Compiling Kotlin sources...\n' +
      '[10:10:00] Signing with upload key...\n' +
      '[10:12:45] Generating APK + AAB...\n' +
      '[10:14:10] Build succeeded.',
    signingCertStatus: 'connected',
    distributionUploadStatus: 'uploaded',
    releaseNotes: 'Bug fixes and performance improvements.',
  },
  {
    id: 'build-003',
    integratorId: INTEGRATOR_ID,
    platform: 'ios',
    version: '2.5.0',
    status: 'building',
    brandConfigName: 'SecureSight Pro',
    createdAtIso: '2026-04-08T08:30:00.000Z',
    finishedAtIso: null,
    artifacts: [],
    buildLogs:
      '[08:30:00] Starting iOS build for SecureSight Pro v2.5.0\n' +
      '[08:32:15] Installing CocoaPods dependencies...\n' +
      '[08:35:30] Compiling Swift sources...',
    signingCertStatus: 'connected',
    distributionUploadStatus: 'pending',
    releaseNotes: 'New live view feature and PTZ controls.',
  },
  {
    id: 'build-004',
    integratorId: INTEGRATOR_ID,
    platform: 'android',
    version: '2.5.0',
    status: 'failed',
    brandConfigName: 'SecureSight Pro',
    createdAtIso: '2026-04-08T07:00:00.000Z',
    finishedAtIso: '2026-04-08T07:09:45.000Z',
    artifacts: [],
    buildLogs:
      '[07:00:00] Starting Android build for SecureSight Pro v2.5.0\n' +
      '[07:02:00] Resolving Gradle dependencies...\n' +
      '[07:06:30] Compiling Kotlin sources...\n' +
      '[07:09:45] ERROR: Signing key expired. Build failed.',
    signingCertStatus: 'expired',
    distributionUploadStatus: 'failed',
    releaseNotes: 'New live view feature and PTZ controls.',
  },
  {
    id: 'build-005',
    integratorId: INTEGRATOR_ID,
    platform: 'ios',
    version: '2.5.0',
    status: 'queued',
    brandConfigName: 'WatchGuard One',
    createdAtIso: '2026-04-08T09:00:00.000Z',
    finishedAtIso: null,
    artifacts: [],
    buildLogs: '[09:00:00] Build queued. Waiting for available runner...',
    signingCertStatus: 'connected',
    distributionUploadStatus: 'n/a',
    releaseNotes: '',
  },
];

const MOCK_APPLE_CREDENTIAL: AppleCredential = {
  teamId: 'A1B2C3D4E5',
  teamName: 'Acme Integrator LLC',
  status: 'connected',
  expiresAtIso: '2027-01-15T00:00:00.000Z',
};

const MOCK_GOOGLE_CREDENTIAL: GoogleCredential = {
  serviceAccountEmail: 'builds@acme-integrator.iam.gserviceaccount.com',
  projectId: 'acme-integrator-prod',
  status: 'connected',
  expiresAtIso: null,
};

const MOCK_DISTRIBUTION_CONFIG: DistributionConfig = {
  integratorId: INTEGRATOR_ID,
  testFlightGroupName: 'Internal Testers',
  playConsoleTrack: 'beta',
  autoSubmit: false,
};

const AVAILABLE_BRAND_CONFIGS = ['Acme Security', 'SecureSight Pro', 'WatchGuard One'];

// ---------------------------------------------------------------------------
// Mock client implementation
// ---------------------------------------------------------------------------

export const mockMobileBuildsClient: MobileBuildsClient = {
  async listBuilds(integratorId: string): Promise<MobileBuildsSnapshot> {
    await Promise.resolve();
    const builds = MOCK_BUILDS.filter((b) => b.integratorId === integratorId);
    return {
      integratorId,
      builds,
      appleCredential: MOCK_APPLE_CREDENTIAL,
      googleCredential: MOCK_GOOGLE_CREDENTIAL,
      distributionConfig: MOCK_DISTRIBUTION_CONFIG,
      availableBrandConfigs: AVAILABLE_BRAND_CONFIGS,
    };
  },

  async triggerBuild(args: BuildTriggerArgs): Promise<MobileBuild> {
    await Promise.resolve();
    return {
      id: `build-new-${Date.now()}`,
      integratorId: args.integratorId,
      platform: args.platform === 'both' ? 'ios' : args.platform,
      version: '2.5.1',
      status: 'queued',
      brandConfigName: args.brandConfigName,
      createdAtIso: new Date().toISOString(),
      finishedAtIso: null,
      artifacts: [],
      buildLogs: `[${new Date().toISOString()}] Build queued. Waiting for available runner...`,
      signingCertStatus: 'connected',
      distributionUploadStatus: 'n/a',
      releaseNotes: args.releaseNotes ?? '',
    };
  },
};

// ---------------------------------------------------------------------------
// Test exports
// ---------------------------------------------------------------------------

export const __MOCK__ = {
  MOCK_BUILDS,
  MOCK_APPLE_CREDENTIAL,
  MOCK_GOOGLE_CREDENTIAL,
  MOCK_DISTRIBUTION_CONFIG,
  AVAILABLE_BRAND_CONFIGS,
};
