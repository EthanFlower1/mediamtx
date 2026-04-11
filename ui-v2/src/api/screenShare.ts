// KAI-469: Screen sharing + ticket integration API client.
//
// Typed API client for the integrator-facing screen-sharing and
// ticket integration features:
//   - initiateSession(spec)
//   - getSession(sessionId)
//   - endSession(sessionId)
//   - listSessions(integratorId, customerId?)
//   - createTicketFromContext(req)
//   - getTicketHookConfig(integratorId)
//   - upsertTicketHookConfig(config)
//
// Wired to the real backend at /api/nvr/screen-share and /api/nvr/tickets.

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type SessionTransport = 'webrtc' | 'rewind';
export type SessionStatus = 'pending' | 'active' | 'completed' | 'failed' | 'cancelled';
export type TicketHookProvider = 'zendesk' | 'freshdesk' | 'internal';

export interface ScreenShareSession {
  readonly sessionId: string;
  readonly integratorId: string;
  readonly customerId: string;
  readonly customerName: string;
  readonly recorderId: string;
  readonly initiatedBy: string;
  readonly transport: SessionTransport;
  readonly status: SessionStatus;
  readonly signallingUrl: string;
  readonly iceServers: readonly RTCIceServerConfig[];
  readonly linkedTicketId: string | null;
  readonly startedAtIso: string | null;
  readonly endedAtIso: string | null;
  readonly durationSeconds: number;
  readonly createdAtIso: string;
}

export interface RTCIceServerConfig {
  readonly urls: string;
  readonly username?: string;
  readonly credential?: string;
}

export interface InitiateSessionSpec {
  readonly integratorId: string;
  readonly customerId: string;
  readonly recorderId: string;
  readonly transport: SessionTransport;
}

export interface TicketContext {
  readonly customerId: string;
  readonly customerName: string;
  readonly recorderId: string;
  readonly sessionId?: string;
  readonly cameraPath?: string;
  readonly description: string;
}

export interface CreateTicketRequest {
  readonly integratorId: string;
  readonly subject: string;
  readonly priority: 'low' | 'normal' | 'high' | 'urgent';
  readonly context: TicketContext;
}

export interface CreateTicketResult {
  readonly ticketId: string;
  readonly externalId?: string;
  readonly provider: TicketHookProvider;
  readonly url?: string;
}

export interface TicketHookConfig {
  readonly configId: string;
  readonly integratorId: string;
  readonly provider: TicketHookProvider;
  readonly apiBaseUrl: string;
  readonly autoCreate: boolean;
  readonly tagTemplate: string;
  readonly enabled: boolean;
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const SCREEN_SHARE_BASE = '/api/nvr/screen-share';
const TICKETS_BASE = '/api/nvr/tickets';

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

export async function initiateSession(
  spec: InitiateSessionSpec,
): Promise<ScreenShareSession> {
  return fetchJson<ScreenShareSession>(`${SCREEN_SHARE_BASE}/sessions`, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(spec),
  });
}

export async function getSession(sessionId: string): Promise<ScreenShareSession | null> {
  const res = await fetch(
    `${SCREEN_SHARE_BASE}/sessions/${encodeURIComponent(sessionId)}`,
  );
  if (res.status === 404) return null;
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(`${res.status} ${text}`);
  }
  return res.json() as Promise<ScreenShareSession>;
}

export async function endSession(sessionId: string): Promise<void> {
  const res = await fetch(
    `${SCREEN_SHARE_BASE}/sessions/${encodeURIComponent(sessionId)}/end`,
    { method: 'POST' },
  );
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(`${res.status} ${text}`);
  }
}

export async function listSessions(
  integratorId: string,
  customerId?: string,
): Promise<readonly ScreenShareSession[]> {
  const params = new URLSearchParams({ integratorId });
  if (customerId) params.set('customerId', customerId);
  return fetchJson<ScreenShareSession[]>(
    `${SCREEN_SHARE_BASE}/sessions?${params.toString()}`,
  );
}

export async function createTicketFromContext(
  req: CreateTicketRequest,
): Promise<CreateTicketResult> {
  return fetchJson<CreateTicketResult>(TICKETS_BASE, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(req),
  });
}

export async function getTicketHookConfig(
  integratorId: string,
): Promise<TicketHookConfig | null> {
  const res = await fetch(
    `${TICKETS_BASE}/config?integratorId=${encodeURIComponent(integratorId)}`,
  );
  if (res.status === 404) return null;
  if (!res.ok) {
    const text = await res.text().catch(() => res.statusText);
    throw new Error(`${res.status} ${text}`);
  }
  return res.json() as Promise<TicketHookConfig>;
}

export async function upsertTicketHookConfig(
  config: Omit<TicketHookConfig, 'configId'> & { configId?: string },
): Promise<TicketHookConfig> {
  return fetchJson<TicketHookConfig>(`${TICKETS_BASE}/config`, {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(config),
  });
}

// ---------------------------------------------------------------------------
// Query keys
// ---------------------------------------------------------------------------

export const SCREEN_SHARE_QUERY_KEY = 'screenShare' as const;

export function sessionsQueryKey(integratorId: string, customerId?: string) {
  return [SCREEN_SHARE_QUERY_KEY, 'sessions', integratorId, customerId] as const;
}

export function sessionQueryKey(sessionId: string) {
  return [SCREEN_SHARE_QUERY_KEY, 'session', sessionId] as const;
}

export function ticketHookConfigQueryKey(integratorId: string) {
  return [SCREEN_SHARE_QUERY_KEY, 'hookConfig', integratorId] as const;
}
