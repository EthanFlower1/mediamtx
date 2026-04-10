// KAI-327: Face Vault API client — compliance-sensitive scaffolding.
//
// Face recognition is regulated under:
//   - EU AI Act Art 5 (prohibited/high-risk AI) + Art 13 (transparency)
//     + Art 27 (fundamental-rights impact assessment)
//   - GDPR Art 9 (special-category biometric data)
//
// All destructive operations require an explicit typed confirmation in
// the UI. This module provides scaffolding only — no real image upload,
// no real embeddings, no real audit-log wiring. Real enrollment lands
// with KAI-282 (face recognition core) and the production Connect-Go
// client will register itself at boot via `registerFaceVaultClient`.
//
// TODO(lead-security): validate this type surface against KAI-282/294
// conformity requirements before wiring a real client.

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

/**
 * Legal basis for processing biometric data.
 * Combines GDPR Art 6 (general bases) + Art 9 (special-category exemptions).
 */
export type LegalBasis =
  | 'consent'
  | 'legitimate-interest'
  | 'vital-interest'
  | 'legal-obligation'
  | 'public-task'
  | 'art9-explicit-consent'
  | 'art9-vital-interest'
  | 'art9-public-interest';

export type ConsentStatus = 'granted' | 'revoked' | 'expired' | 'not-required';

export type EnrollmentSource = 'manual' | 'imported';

export type PurgeScope = 'tenant' | 'camera' | 'dateRange';

export interface FaceEnrollment {
  id: string;
  tenantId: string;
  subjectName: string;
  subjectIdentifier: string;
  enrolledAt: string;
  enrolledBy: string;
  legalBasis: LegalBasis;
  consentStatus: ConsentStatus;
  consentGrantedAt: string | null;
  consentRevokedAt: string | null;
  retentionDays: number;
  expiresAt: string;
  source: EnrollmentSource;
  /**
   * Stubbed data URL placeholder. Real thumbnails never round-trip through
   * this scaffold — KAI-282 will replace this with a signed URL.
   * TODO(lead-security): confirm thumbnail display policy
   * (full / blurred / embedding-only).
   */
  thumbnailUrl: string;
  auditTrailId: string;
}

export interface FaceVaultFilters {
  tenantId: string;
  search?: string;
  consentStatus?: ConsentStatus | 'all';
  expiringWithinDays?: number | null;
  legalBasis?: LegalBasis | 'all';
}

export interface FaceVaultSummary {
  tenantId: string;
  totalEnrollments: number;
  activeEnrollments: number;
  expiringSoonCount: number;
  lastPurgeAt: string | null;
  retentionPolicyDays: number;
}

export interface EnrollFaceArgs {
  tenantId: string;
  subjectName: string;
  subjectIdentifier: string;
  legalBasis: LegalBasis;
  consentGranted: boolean;
  retentionDays: number;
  /** Base64 data URL or null when the UI has not attached an image yet. */
  thumbnailDataUrl: string | null;
}

export interface PurgeArgs {
  tenantId: string;
  scope: PurgeScope;
  cameraId?: string;
  fromDate?: string;
  toDate?: string;
  /**
   * Typed confirmation string; the server must re-validate the exact
   * string matches the expected `PURGE-TENANT-{tenantName}` sentinel.
   * TODO(lead-security): confirm two-person approval rule vs single-admin
   * strong-confirm per lead-security decision.
   */
  confirmation: string;
}

export interface DeleteFaceArgs {
  tenantId: string;
  enrollmentId: string;
  /** Must equal the literal string "DELETE". */
  confirmation: string;
}

export interface FaceAuditEvent {
  id: string;
  enrollmentId: string;
  type:
    | 'enrollment.created'
    | 'enrollment.deleted'
    | 'consent.granted'
    | 'consent.revoked'
    | 'consent.expired'
    | 'enrollment.matched'
    | 'vault.purged';
  // TODO(lead-security): confirmed event-type list pending.
  at: string;
  actor: string;
  notes: string;
}

export interface FaceVaultClient {
  listEnrollments(filters: FaceVaultFilters): Promise<FaceEnrollment[]>;
  getSummary(tenantId: string): Promise<FaceVaultSummary>;
  enrollFace(args: EnrollFaceArgs): Promise<FaceEnrollment>;
  revokeConsent(tenantId: string, enrollmentId: string): Promise<FaceEnrollment>;
  deleteFaceEnrollment(args: DeleteFaceArgs): Promise<void>;
  purgeFaceVault(args: PurgeArgs): Promise<void>;
  listAuditEvents(tenantId: string, enrollmentId: string): Promise<FaceAuditEvent[]>;
}

// ---------------------------------------------------------------------------
// Query-key factory
// ---------------------------------------------------------------------------

export const faceVaultQueryKeys = {
  all: (tenantId: string) => ['faceVault', tenantId] as const,
  summary: (tenantId: string) => ['faceVault', tenantId, 'summary'] as const,
  list: (tenantId: string, filters: Omit<FaceVaultFilters, 'tenantId'>) =>
    ['faceVault', tenantId, 'list', filters] as const,
  audit: (tenantId: string, enrollmentId: string) =>
    ['faceVault', tenantId, 'audit', enrollmentId] as const,
};

// ---------------------------------------------------------------------------
// Client bootstrap
// ---------------------------------------------------------------------------

let activeClient: FaceVaultClient | null = null;

export function registerFaceVaultClient(client: FaceVaultClient): void {
  activeClient = client;
}

export function resetFaceVaultClientForTests(): void {
  activeClient = null;
}

async function getClient(): Promise<FaceVaultClient> {
  if (activeClient) return activeClient;
  const { faceVaultMockClient } = await import('./faceVault.mock');
  return faceVaultMockClient;
}

// ---------------------------------------------------------------------------
// Thin wrappers
// ---------------------------------------------------------------------------

export async function listFaceEnrollments(
  filters: FaceVaultFilters,
): Promise<FaceEnrollment[]> {
  return (await getClient()).listEnrollments(filters);
}

export async function getFaceVaultSummary(
  tenantId: string,
): Promise<FaceVaultSummary> {
  return (await getClient()).getSummary(tenantId);
}

export async function enrollFace(args: EnrollFaceArgs): Promise<FaceEnrollment> {
  return (await getClient()).enrollFace(args);
}

export async function revokeConsent(
  tenantId: string,
  enrollmentId: string,
): Promise<FaceEnrollment> {
  return (await getClient()).revokeConsent(tenantId, enrollmentId);
}

export async function deleteFaceEnrollment(args: DeleteFaceArgs): Promise<void> {
  return (await getClient()).deleteFaceEnrollment(args);
}

export async function purgeFaceVault(args: PurgeArgs): Promise<void> {
  return (await getClient()).purgeFaceVault(args);
}

export async function listFaceAuditEvents(
  tenantId: string,
  enrollmentId: string,
): Promise<FaceAuditEvent[]> {
  return (await getClient()).listAuditEvents(tenantId, enrollmentId);
}
