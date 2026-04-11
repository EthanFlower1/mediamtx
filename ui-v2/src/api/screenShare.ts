// KAI-469: Screen sharing + ticket integration API client stubs.
//
// Typed promise stubs for the integrator-facing screen-sharing and
// ticket integration features:
//   - initiateSession(spec)
//   - getSession(sessionId)
//   - endSession(sessionId)
//   - listSessions(integratorId, customerId?)
//   - createTicketFromContext(req)
//   - getTicketHookConfig(integratorId)
//   - upsertTicketHookConfig(config)
//
// All data is mocked here. The real implementation will be wired to
// Connect-Go clients generated from KAI-469 protos.

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
// Mock dataset
// ---------------------------------------------------------------------------

const CURRENT_INTEGRATOR_ID = 'integrator-001';

let sessionSeq = 0;
let ticketSeq = 0;

const MOCK_SESSIONS: ScreenShareSession[] = [
  {
    sessionId: 'ss-001',
    integratorId: CURRENT_INTEGRATOR_ID,
    customerId: 'cust-integrator-001-000',
    customerName: 'Acme 0 Retail',
    recorderId: 'rec-001',
    initiatedBy: 'staff@integrator.example',
    transport: 'webrtc',
    status: 'completed',
    signallingUrl: 'wss://signal.kaivue.io/v1/sessions/ss-001',
    iceServers: [{ urls: 'stun:stun.kaivue.io:3478' }],
    linkedTicketId: 'tkt-001',
    startedAtIso: '2026-04-09T10:00:00Z',
    endedAtIso: '2026-04-09T10:15:00Z',
    durationSeconds: 900,
    createdAtIso: '2026-04-09T09:58:00Z',
  },
  {
    sessionId: 'ss-002',
    integratorId: CURRENT_INTEGRATOR_ID,
    customerId: 'cust-integrator-001-001',
    customerName: 'Bcme 1 Healthcare',
    recorderId: 'rec-002',
    initiatedBy: 'tech@integrator.example',
    transport: 'rewind',
    status: 'active',
    signallingUrl: 'wss://signal.kaivue.io/v1/sessions/ss-002',
    iceServers: [{ urls: 'stun:stun.kaivue.io:3478' }],
    linkedTicketId: null,
    startedAtIso: '2026-04-10T08:30:00Z',
    endedAtIso: null,
    durationSeconds: 0,
    createdAtIso: '2026-04-10T08:28:00Z',
  },
];

const MOCK_HOOK_CONFIGS: Record<string, TicketHookConfig> = {
  [CURRENT_INTEGRATOR_ID]: {
    configId: 'hc-001',
    integratorId: CURRENT_INTEGRATOR_ID,
    provider: 'zendesk',
    apiBaseUrl: 'https://mycompany.zendesk.com',
    autoCreate: false,
    tagTemplate: 'kaivue,{{customer_id}}',
    enabled: true,
  },
};

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

export async function initiateSession(
  spec: InitiateSessionSpec,
): Promise<ScreenShareSession> {
  await new Promise((r) => setTimeout(r, 0));
  sessionSeq += 1;
  const sessionId = `ss-new-${sessionSeq}`;
  const session: ScreenShareSession = {
    sessionId,
    integratorId: spec.integratorId,
    customerId: spec.customerId,
    customerName: `Customer ${spec.customerId}`,
    recorderId: spec.recorderId,
    initiatedBy: 'current-user',
    transport: spec.transport,
    status: 'pending',
    signallingUrl: `wss://signal.kaivue.io/v1/sessions/${sessionId}`,
    iceServers: [{ urls: 'stun:stun.kaivue.io:3478' }],
    linkedTicketId: null,
    startedAtIso: null,
    endedAtIso: null,
    durationSeconds: 0,
    createdAtIso: new Date().toISOString(),
  };
  MOCK_SESSIONS.push(session);
  return session;
}

export async function getSession(sessionId: string): Promise<ScreenShareSession | null> {
  await new Promise((r) => setTimeout(r, 0));
  return MOCK_SESSIONS.find((s) => s.sessionId === sessionId) ?? null;
}

export async function endSession(sessionId: string): Promise<void> {
  await new Promise((r) => setTimeout(r, 0));
  const idx = MOCK_SESSIONS.findIndex((s) => s.sessionId === sessionId);
  if (idx >= 0) {
    const now = new Date().toISOString();
    const s = MOCK_SESSIONS[idx]!;
    MOCK_SESSIONS[idx] = {
      ...s,
      status: 'completed',
      endedAtIso: now,
      durationSeconds: s.startedAtIso
        ? Math.floor((Date.now() - new Date(s.startedAtIso).getTime()) / 1000)
        : 0,
    };
  }
}

export async function listSessions(
  integratorId: string,
  customerId?: string,
): Promise<readonly ScreenShareSession[]> {
  await new Promise((r) => setTimeout(r, 0));
  return MOCK_SESSIONS.filter(
    (s) =>
      s.integratorId === integratorId &&
      (!customerId || s.customerId === customerId),
  );
}

export async function createTicketFromContext(
  req: CreateTicketRequest,
): Promise<CreateTicketResult> {
  await new Promise((r) => setTimeout(r, 0));
  const config = MOCK_HOOK_CONFIGS[req.integratorId];
  if (!config) {
    throw new Error('No ticket hook configuration found for this integrator');
  }
  if (!config.enabled) {
    throw new Error('Ticket integration is disabled');
  }

  ticketSeq += 1;
  const ticketId = `tkt-new-${ticketSeq}`;
  let url: string | undefined;

  switch (config.provider) {
    case 'zendesk':
      url = `${config.apiBaseUrl}/agent/tickets/${ticketId}`;
      break;
    case 'freshdesk':
      url = `${config.apiBaseUrl}/a/tickets/${ticketId}`;
      break;
    case 'internal':
      url = `/command/support/tickets/${ticketId}`;
      break;
  }

  // Link to session if provided
  if (req.context.sessionId) {
    const sessIdx = MOCK_SESSIONS.findIndex((s) => s.sessionId === req.context.sessionId);
    if (sessIdx >= 0) {
      MOCK_SESSIONS[sessIdx] = { ...MOCK_SESSIONS[sessIdx]!, linkedTicketId: ticketId };
    }
  }

  return {
    ticketId,
    provider: config.provider,
    url,
  };
}

export async function getTicketHookConfig(
  integratorId: string,
): Promise<TicketHookConfig | null> {
  await new Promise((r) => setTimeout(r, 0));
  return MOCK_HOOK_CONFIGS[integratorId] ?? null;
}

export async function upsertTicketHookConfig(
  config: Omit<TicketHookConfig, 'configId'> & { configId?: string },
): Promise<TicketHookConfig> {
  await new Promise((r) => setTimeout(r, 0));
  const full: TicketHookConfig = {
    ...config,
    configId: config.configId ?? `hc-new-${Date.now()}`,
  };
  MOCK_HOOK_CONFIGS[config.integratorId] = full;
  return full;
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

// Test/fixture exports.
export const __TEST__ = {
  CURRENT_INTEGRATOR_ID,
  MOCK_SESSIONS,
  MOCK_HOOK_CONFIGS,
};
