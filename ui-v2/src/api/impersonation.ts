// KAI-467: Impersonation session API client.
//
// Typed stubs for the impersonation session lifecycle:
//   - listActiveSessions(tenantId) — active impersonation sessions
//   - terminateSession(sessionId)  — end impersonation
//   - getSessionAuditLog(tenantId) — audit entries for impersonation events
//
// Mock data for now; real implementation will wire to the backend
// at internal/cloud/impersonation (KAI-379).

export type ImpersonationMode = 'integrator' | 'platform_support';
export type SessionStatus = 'active' | 'expired' | 'terminated';

export interface ImpersonationSessionDetail {
  readonly sessionId: string;
  readonly mode: ImpersonationMode;
  readonly impersonatingUserId: string;
  readonly impersonatingUserName: string;
  readonly impersonatingTenantId: string;
  readonly impersonatedTenantId: string;
  readonly impersonatedTenantName: string;
  readonly scopedPermissions: readonly string[];
  readonly status: SessionStatus;
  readonly reason: string;
  readonly createdAtIso: string;
  readonly expiresAtIso: string;
  readonly terminatedAtIso: string | null;
  readonly terminatedBy: string | null;
}

export interface AuditLogEntry {
  readonly id: string;
  readonly tenantId: string;
  readonly actorUserId: string;
  readonly actorUserName: string;
  readonly action: string;
  readonly resourceType: string;
  readonly resourceId: string;
  readonly result: 'allow' | 'deny';
  readonly errorCode: string | null;
  readonly impersonatingUserId: string | null;
  readonly impersonatedTenantId: string | null;
  readonly timestampIso: string;
}

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const NOW = new Date();
const THIRTY_MINUTES = 30 * 60 * 1000;

function isoAgo(ms: number): string {
  return new Date(NOW.getTime() - ms).toISOString();
}

function isoAhead(ms: number): string {
  return new Date(NOW.getTime() + ms).toISOString();
}

const MOCK_SESSIONS: ImpersonationSessionDetail[] = [
  {
    sessionId: 'imp-sess-001',
    mode: 'integrator',
    impersonatingUserId: 'integrator-user-self',
    impersonatingUserName: 'Jane Integrator',
    impersonatingTenantId: 'integrator-001',
    impersonatedTenantId: 'cust-integrator-001-005',
    impersonatedTenantName: 'Fcme 5 Finance',
    scopedPermissions: ['view.live', 'view.playback', 'cameras.edit'],
    status: 'active',
    reason: 'Investigating camera offline alert — ticket SUP-1234',
    createdAtIso: isoAgo(12 * 60 * 1000),
    expiresAtIso: isoAhead(THIRTY_MINUTES - 12 * 60 * 1000),
    terminatedAtIso: null,
    terminatedBy: null,
  },
  {
    sessionId: 'imp-sess-002',
    mode: 'integrator',
    impersonatingUserId: 'integrator-user-self',
    impersonatingUserName: 'Jane Integrator',
    impersonatingTenantId: 'integrator-001',
    impersonatedTenantId: 'cust-integrator-001-010',
    impersonatedTenantName: 'Kcme 10 Logistics',
    scopedPermissions: ['view.live', 'view.playback'],
    status: 'terminated',
    reason: 'Customer requested help configuring schedules',
    createdAtIso: isoAgo(2 * 60 * 60 * 1000),
    expiresAtIso: isoAgo(90 * 60 * 1000),
    terminatedAtIso: isoAgo(100 * 60 * 1000),
    terminatedBy: 'integrator-user-self',
  },
  {
    sessionId: 'imp-sess-003',
    mode: 'platform_support',
    impersonatingUserId: 'support-agent-007',
    impersonatingUserName: 'Support Agent',
    impersonatingTenantId: 'platform-001',
    impersonatedTenantId: 'cust-integrator-001-002',
    impersonatedTenantName: 'Ccme 2 Logistics',
    scopedPermissions: ['view.live', 'view.playback', 'audit.read'],
    status: 'expired',
    reason: 'Escalation from integrator — billing discrepancy',
    createdAtIso: isoAgo(4 * 60 * 60 * 1000),
    expiresAtIso: isoAgo(3.5 * 60 * 60 * 1000),
    terminatedAtIso: isoAgo(3.5 * 60 * 60 * 1000),
    terminatedBy: null,
  },
];

const MOCK_AUDIT_LOG: AuditLogEntry[] = [
  {
    id: 'aud-001',
    tenantId: 'cust-integrator-001-005',
    actorUserId: 'integrator-user-self',
    actorUserName: 'Jane Integrator',
    action: 'impersonation.session.start',
    resourceType: 'impersonation_session',
    resourceId: 'imp-sess-001',
    result: 'allow',
    errorCode: null,
    impersonatingUserId: 'integrator-user-self',
    impersonatedTenantId: 'cust-integrator-001-005',
    timestampIso: isoAgo(12 * 60 * 1000),
  },
  {
    id: 'aud-002',
    tenantId: 'cust-integrator-001-005',
    actorUserId: 'integrator-user-self',
    actorUserName: 'Jane Integrator',
    action: 'impersonation.session.action',
    resourceType: 'camera',
    resourceId: 'cam-front-001',
    result: 'allow',
    errorCode: null,
    impersonatingUserId: 'integrator-user-self',
    impersonatedTenantId: 'cust-integrator-001-005',
    timestampIso: isoAgo(10 * 60 * 1000),
  },
  {
    id: 'aud-003',
    tenantId: 'cust-integrator-001-010',
    actorUserId: 'integrator-user-self',
    actorUserName: 'Jane Integrator',
    action: 'impersonation.session.start',
    resourceType: 'impersonation_session',
    resourceId: 'imp-sess-002',
    result: 'allow',
    errorCode: null,
    impersonatingUserId: 'integrator-user-self',
    impersonatedTenantId: 'cust-integrator-001-010',
    timestampIso: isoAgo(2 * 60 * 60 * 1000),
  },
  {
    id: 'aud-004',
    tenantId: 'cust-integrator-001-010',
    actorUserId: 'integrator-user-self',
    actorUserName: 'Jane Integrator',
    action: 'impersonation.session.end',
    resourceType: 'impersonation_session',
    resourceId: 'imp-sess-002',
    result: 'allow',
    errorCode: null,
    impersonatingUserId: 'integrator-user-self',
    impersonatedTenantId: 'cust-integrator-001-010',
    timestampIso: isoAgo(100 * 60 * 1000),
  },
  {
    id: 'aud-005',
    tenantId: 'cust-integrator-001-002',
    actorUserId: 'support-agent-007',
    actorUserName: 'Support Agent',
    action: 'impersonation.session.start',
    resourceType: 'impersonation_session',
    resourceId: 'imp-sess-003',
    result: 'allow',
    errorCode: null,
    impersonatingUserId: 'support-agent-007',
    impersonatedTenantId: 'cust-integrator-001-002',
    timestampIso: isoAgo(4 * 60 * 60 * 1000),
  },
  {
    id: 'aud-006',
    tenantId: 'cust-integrator-001-002',
    actorUserId: 'support-agent-007',
    actorUserName: 'Support Agent',
    action: 'users.create',
    resourceType: 'user',
    resourceId: 'user-blocked-001',
    result: 'deny',
    errorCode: 'admin_action_blocked',
    impersonatingUserId: 'support-agent-007',
    impersonatedTenantId: 'cust-integrator-001-002',
    timestampIso: isoAgo(3.8 * 60 * 60 * 1000),
  },
];

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

export async function listActiveSessions(
  tenantId: string,
): Promise<readonly ImpersonationSessionDetail[]> {
  await new Promise((r) => setTimeout(r, 0));
  return MOCK_SESSIONS.filter(
    (s) =>
      (s.impersonatingTenantId === tenantId || s.impersonatedTenantId === tenantId) &&
      s.status === 'active',
  );
}

export async function listAllSessions(
  tenantId: string,
): Promise<readonly ImpersonationSessionDetail[]> {
  await new Promise((r) => setTimeout(r, 0));
  return MOCK_SESSIONS.filter(
    (s) => s.impersonatingTenantId === tenantId || s.impersonatedTenantId === tenantId,
  );
}

export async function terminateSession(sessionId: string): Promise<void> {
  await new Promise((r) => setTimeout(r, 0));
  const session = MOCK_SESSIONS.find((s) => s.sessionId === sessionId);
  if (!session) throw new Error(`Session not found: ${sessionId}`);
  if (session.status !== 'active') throw new Error(`Session not active: ${sessionId}`);
  // In mock, we mutate in place (no persistence).
  (session as { status: SessionStatus }).status = 'terminated';
  const now = new Date().toISOString();
  (session as { terminatedAtIso: string | null }).terminatedAtIso = now;
  (session as { terminatedBy: string | null }).terminatedBy = 'integrator-user-self';
}

export async function getSessionAuditLog(
  tenantId: string,
): Promise<readonly AuditLogEntry[]> {
  await new Promise((r) => setTimeout(r, 0));
  return MOCK_AUDIT_LOG.filter(
    (e) => e.impersonatedTenantId === tenantId || e.tenantId === tenantId,
  ).sort((a, b) => (a.timestampIso > b.timestampIso ? -1 : 1));
}

export async function getAllImpersonationAuditLog(): Promise<readonly AuditLogEntry[]> {
  await new Promise((r) => setTimeout(r, 0));
  return [...MOCK_AUDIT_LOG].sort((a, b) => (a.timestampIso > b.timestampIso ? -1 : 1));
}

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const IMPERSONATION_QUERY_KEY = 'impersonation' as const;

export function impersonationSessionsKey(tenantId: string) {
  return [IMPERSONATION_QUERY_KEY, 'sessions', tenantId] as const;
}

export function impersonationAuditKey(tenantId: string) {
  return [IMPERSONATION_QUERY_KEY, 'audit', tenantId] as const;
}

export function impersonationAuditAllKey() {
  return [IMPERSONATION_QUERY_KEY, 'audit', 'all'] as const;
}
