// KAI-467: Impersonation session API client.
//
// Typed API client for the impersonation session lifecycle:
//   - listActiveSessions(tenantId) -- active impersonation sessions
//   - listAllSessions(tenantId)    -- all sessions for a tenant
//   - terminateSession(sessionId)  -- end impersonation
//   - getSessionAuditLog(tenantId) -- audit entries for a specific tenant
//   - getAllImpersonationAuditLog() -- all impersonation audit entries
//
// Wired to the backend at internal/cloud/impersonation (KAI-379).

import { API_BASE_URL } from './client';

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
// Public API
// ---------------------------------------------------------------------------

export async function listActiveSessions(
  tenantId: string,
): Promise<readonly ImpersonationSessionDetail[]> {
  return fetchJson<ImpersonationSessionDetail[]>(
    `${API_BASE_URL}/impersonation/sessions?active=true&tenantId=${encodeURIComponent(tenantId)}`,
  );
}

export async function listAllSessions(
  tenantId: string,
): Promise<readonly ImpersonationSessionDetail[]> {
  return fetchJson<ImpersonationSessionDetail[]>(
    `${API_BASE_URL}/impersonation/sessions?tenantId=${encodeURIComponent(tenantId)}`,
  );
}

export async function terminateSession(sessionId: string): Promise<void> {
  const res = await fetch(
    `${API_BASE_URL}/impersonation/sessions/${encodeURIComponent(sessionId)}/terminate`,
    { method: 'POST' },
  );
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(`${res.status} ${text}`);
  }
}

export async function getSessionAuditLog(
  tenantId: string,
): Promise<readonly AuditLogEntry[]> {
  return fetchJson<AuditLogEntry[]>(
    `${API_BASE_URL}/impersonation/sessions/${encodeURIComponent(tenantId)}/audit`,
  );
}

export async function getAllImpersonationAuditLog(): Promise<readonly AuditLogEntry[]> {
  return fetchJson<AuditLogEntry[]>(`${API_BASE_URL}/impersonation/audit`);
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
