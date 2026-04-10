// KAI-327: Deterministic in-memory mock for faceVault.ts.
//
// Produces 8-12 enrollments per tenantId with a mix of consent statuses
// and legal bases. Real data lands with KAI-282.

import type {
  ConsentStatus,
  DeleteFaceArgs,
  EnrollFaceArgs,
  EnrollmentSource,
  FaceAuditEvent,
  FaceEnrollment,
  FaceVaultClient,
  FaceVaultFilters,
  FaceVaultSummary,
  LegalBasis,
  PurgeArgs,
} from './faceVault';

const LEGAL_BASES: LegalBasis[] = [
  'consent',
  'art9-explicit-consent',
  'legitimate-interest',
  'public-task',
  'art9-public-interest',
];

const CONSENT_CYCLE: ConsentStatus[] = [
  'granted',
  'granted',
  'granted',
  'revoked',
  'expired',
  'not-required',
];

const SOURCES: EnrollmentSource[] = ['manual', 'manual', 'imported'];

// A tiny 1x1 transparent PNG data URL, used as a stub thumbnail. No real
// face data is ever generated or loaded.
const STUB_THUMBNAIL =
  'data:image/png;base64,iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNgAAIAAAUAAen63NgAAAAASUVORK5CYII=';

function hash(seed: string): number {
  let h = 0;
  for (let i = 0; i < seed.length; i++) {
    h = (h * 31 + seed.charCodeAt(i)) | 0;
  }
  return Math.abs(h);
}

interface TenantVaultState {
  enrollments: Map<string, FaceEnrollment>;
  auditEvents: Map<string, FaceAuditEvent[]>;
  lastPurgeAt: string | null;
  retentionPolicyDays: number;
}

const store: Map<string, TenantVaultState> = new Map();

function buildInitial(tenantId: string): TenantVaultState {
  const seed = hash(tenantId);
  const count = 8 + (seed % 5); // 8-12
  const now = Date.now();
  const enrollments = new Map<string, FaceEnrollment>();
  const auditEvents = new Map<string, FaceAuditEvent[]>();
  for (let i = 0; i < count; i++) {
    const id = `face-${tenantId}-${i.toString().padStart(3, '0')}`;
    const consentStatus = CONSENT_CYCLE[i % CONSENT_CYCLE.length]!;
    const legalBasis = LEGAL_BASES[i % LEGAL_BASES.length]!;
    const retentionDays = [30, 90, 180, 365][i % 4]!;
    const enrolledAt = new Date(now - i * 86_400_000 * 3).toISOString();
    const expiresAt = new Date(now + (retentionDays - i * 2) * 86_400_000).toISOString();
    enrollments.set(id, {
      id,
      tenantId,
      subjectName: `Subject ${i + 1}`,
      subjectIdentifier: `SUBJ-${(1000 + i).toString()}`,
      enrolledAt,
      enrolledBy: 'mock-admin',
      legalBasis,
      consentStatus,
      consentGrantedAt:
        consentStatus === 'granted' || consentStatus === 'expired' ? enrolledAt : null,
      consentRevokedAt:
        consentStatus === 'revoked' ? new Date(now - i * 3600_000).toISOString() : null,
      retentionDays,
      expiresAt,
      source: SOURCES[i % SOURCES.length]!,
      thumbnailUrl: STUB_THUMBNAIL,
      auditTrailId: `audit-${id}`,
    });
    auditEvents.set(id, [
      {
        id: `evt-${id}-0`,
        enrollmentId: id,
        type: 'enrollment.created',
        at: enrolledAt,
        actor: 'mock-admin',
        notes: 'Initial enrollment (mock).',
      },
    ]);
  }
  return {
    enrollments,
    auditEvents,
    lastPurgeAt: null,
    retentionPolicyDays: 90,
  };
}

function getState(tenantId: string): TenantVaultState {
  let state = store.get(tenantId);
  if (!state) {
    state = buildInitial(tenantId);
    store.set(tenantId, state);
  }
  return state;
}

function applyFilters(
  all: FaceEnrollment[],
  filters: FaceVaultFilters,
): FaceEnrollment[] {
  let out = all;
  if (filters.search) {
    const q = filters.search.trim().toLowerCase();
    if (q) {
      out = out.filter(
        (e) =>
          e.subjectName.toLowerCase().includes(q) ||
          e.subjectIdentifier.toLowerCase().includes(q),
      );
    }
  }
  if (filters.consentStatus && filters.consentStatus !== 'all') {
    out = out.filter((e) => e.consentStatus === filters.consentStatus);
  }
  if (filters.legalBasis && filters.legalBasis !== 'all') {
    out = out.filter((e) => e.legalBasis === filters.legalBasis);
  }
  if (typeof filters.expiringWithinDays === 'number') {
    const cutoff = Date.now() + filters.expiringWithinDays * 86_400_000;
    out = out.filter((e) => new Date(e.expiresAt).getTime() <= cutoff);
  }
  return out;
}

export const faceVaultMockClient: FaceVaultClient = {
  async listEnrollments(filters: FaceVaultFilters): Promise<FaceEnrollment[]> {
    await Promise.resolve();
    const state = getState(filters.tenantId);
    const all = Array.from(state.enrollments.values());
    return applyFilters(all, filters);
  },

  async getSummary(tenantId: string): Promise<FaceVaultSummary> {
    await Promise.resolve();
    const state = getState(tenantId);
    const all = Array.from(state.enrollments.values());
    const soon = Date.now() + 30 * 86_400_000;
    return {
      tenantId,
      totalEnrollments: all.length,
      activeEnrollments: all.filter(
        (e) => e.consentStatus === 'granted' || e.consentStatus === 'not-required',
      ).length,
      expiringSoonCount: all.filter((e) => new Date(e.expiresAt).getTime() <= soon).length,
      lastPurgeAt: state.lastPurgeAt,
      retentionPolicyDays: state.retentionPolicyDays,
    };
  },

  async enrollFace(args: EnrollFaceArgs): Promise<FaceEnrollment> {
    await Promise.resolve();
    const state = getState(args.tenantId);
    const id = `face-${args.tenantId}-new-${Date.now()}`;
    const now = new Date().toISOString();
    // TODO(lead-security): confirm audit payload shape when real wiring lands.
    const enrollment: FaceEnrollment = {
      id,
      tenantId: args.tenantId,
      subjectName: args.subjectName,
      subjectIdentifier: args.subjectIdentifier,
      enrolledAt: now,
      enrolledBy: 'mock-admin',
      legalBasis: args.legalBasis,
      consentStatus: args.consentGranted ? 'granted' : 'not-required',
      consentGrantedAt: args.consentGranted ? now : null,
      consentRevokedAt: null,
      retentionDays: args.retentionDays,
      expiresAt: new Date(
        Date.now() + args.retentionDays * 86_400_000,
      ).toISOString(),
      source: 'manual',
      thumbnailUrl: args.thumbnailDataUrl ?? STUB_THUMBNAIL,
      auditTrailId: `audit-${id}`,
    };
    state.enrollments.set(id, enrollment);
    state.auditEvents.set(id, [
      {
        id: `evt-${id}-0`,
        enrollmentId: id,
        type: 'enrollment.created',
        at: now,
        actor: 'mock-admin',
        notes: 'Manual enrollment via admin UI (mock).',
      },
    ]);
    return enrollment;
  },

  async revokeConsent(tenantId: string, enrollmentId: string): Promise<FaceEnrollment> {
    await Promise.resolve();
    const state = getState(tenantId);
    const existing = state.enrollments.get(enrollmentId);
    if (!existing) throw new Error(`unknown enrollment: ${enrollmentId}`);
    const now = new Date().toISOString();
    const updated: FaceEnrollment = {
      ...existing,
      consentStatus: 'revoked',
      consentRevokedAt: now,
    };
    state.enrollments.set(enrollmentId, updated);
    const events = state.auditEvents.get(enrollmentId) ?? [];
    events.push({
      id: `evt-${enrollmentId}-${events.length}`,
      enrollmentId,
      type: 'consent.revoked',
      at: now,
      actor: 'mock-admin',
      notes: 'Consent revoked via admin UI (mock).',
    });
    state.auditEvents.set(enrollmentId, events);
    return updated;
  },

  async deleteFaceEnrollment(args: DeleteFaceArgs): Promise<void> {
    await Promise.resolve();
    if (args.confirmation !== 'DELETE') {
      throw new Error('delete confirmation mismatch');
    }
    const state = getState(args.tenantId);
    state.enrollments.delete(args.enrollmentId);
    // Audit events retained per standard audit-log discipline.
  },

  async purgeFaceVault(args: PurgeArgs): Promise<void> {
    await Promise.resolve();
    // Confirmation sentinel validated by the UI; server must re-validate.
    const state = getState(args.tenantId);
    if (args.scope === 'tenant') {
      state.enrollments.clear();
    } else if (args.scope === 'dateRange' && args.fromDate && args.toDate) {
      const from = new Date(args.fromDate).getTime();
      const to = new Date(args.toDate).getTime();
      for (const [id, e] of state.enrollments) {
        const t = new Date(e.enrolledAt).getTime();
        if (t >= from && t <= to) state.enrollments.delete(id);
      }
    }
    state.lastPurgeAt = new Date().toISOString();
  },

  async listAuditEvents(
    tenantId: string,
    enrollmentId: string,
  ): Promise<FaceAuditEvent[]> {
    await Promise.resolve();
    const state = getState(tenantId);
    return state.auditEvents.get(enrollmentId) ?? [];
  },
};

/** Test helper: clear the in-memory vault store. */
export function __resetFaceVaultMockStateForTests(): void {
  store.clear();
}
