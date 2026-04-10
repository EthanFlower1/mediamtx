// KAI-329: System Health + Remote Access + Quick Settings API client.
//
// Scaffolding only: production file exports typed interfaces + bootstrap
// hooks and thin wrappers. Real Connect-Go client (KAI-399) will replace
// the lazy-loaded mock at app boot via `registerSystemHealthClient`.
//
// Split pattern (same as KAI-327): the mock lives in a sibling module
// that is lazy-imported only when no production client has registered
// itself, so production bundles pay nothing for the mock.

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type OverallStatus = 'healthy' | 'degraded' | 'offline';

export type RecorderStatus = 'online' | 'offline' | 'degraded';

export interface RecorderHealth {
  id: string;
  name: string;
  ipAddress: string;
  status: RecorderStatus;
  uptimeSeconds: number;
  storageUsedBytes: number;
  storageTotalBytes: number;
  lastCheckIn: string; // ISO-8601
}

export interface CameraStatusSummary {
  total: number;
  online: number;
  offline: number;
  degraded: number;
}

export interface StorageOverview {
  totalCapacityBytes: number;
  usedBytes: number;
  availableBytes: number;
  retentionDaysRemaining: number;
}

export interface NetworkStatus {
  bandwidthUtilizationPercent: number;
  packetLossPercent: number;
  latencyMs: number;
}

export interface SystemHealth {
  overallStatus: OverallStatus;
  recorders: RecorderHealth[];
  cameras: CameraStatusSummary;
  storage: StorageOverview;
  network: NetworkStatus;
  checkedAt: string; // ISO-8601
}

export interface RemoteSession {
  id: string;
  userId: string;
  userDisplayName: string;
  startedAt: string; // ISO-8601
  endedAt: string | null; // ISO-8601 or null if active
  durationSeconds: number;
}

export interface RemoteAccess {
  enabled: boolean;
  portForwardingActive: boolean;
  vpnTunnelStatus: 'connected' | 'disconnected' | 'connecting';
  recentSessions: RemoteSession[];
}

export type DayOfWeek = 'monday' | 'tuesday' | 'wednesday' | 'thursday' | 'friday' | 'saturday' | 'sunday';

export interface SystemSettings {
  systemName: string;
  timezone: string;
  autoUpdateEnabled: boolean;
  maintenanceDay: DayOfWeek;
  maintenanceTime: string; // HH:mm (24h)
}

// ---------------------------------------------------------------------------
// Client interface
// ---------------------------------------------------------------------------

export interface SystemHealthClient {
  getSystemHealth(tenantId: string): Promise<SystemHealth>;
  getRemoteAccess(tenantId: string): Promise<RemoteAccess>;
  setRemoteAccessEnabled(tenantId: string, enabled: boolean): Promise<RemoteAccess>;
  getSystemSettings(tenantId: string): Promise<SystemSettings>;
  updateSystemSettings(tenantId: string, settings: Partial<SystemSettings>): Promise<SystemSettings>;
}

// ---------------------------------------------------------------------------
// Query-key factory
// ---------------------------------------------------------------------------

export const systemHealthQueryKeys = {
  all: (tenantId: string) => ['systemHealth', tenantId] as const,
  health: (tenantId: string) => ['systemHealth', tenantId, 'health'] as const,
  remoteAccess: (tenantId: string) => ['systemHealth', tenantId, 'remoteAccess'] as const,
  settings: (tenantId: string) => ['systemHealth', tenantId, 'settings'] as const,
};

// ---------------------------------------------------------------------------
// Feature-flag bootstrap
// ---------------------------------------------------------------------------

export const SYSTEM_HEALTH_FEATURE_FLAG = 'systemHealth.enabled';

export function isSystemHealthEnabled(): boolean {
  // TODO(KAI-329): wire to real feature-flag service
  return true;
}

// ---------------------------------------------------------------------------
// Client bootstrap (production client registers itself at app boot)
// ---------------------------------------------------------------------------

let activeClient: SystemHealthClient | null = null;

export function registerSystemHealthClient(client: SystemHealthClient): void {
  activeClient = client;
}

export function resetSystemHealthClientForTests(): void {
  activeClient = null;
}

async function getClient(): Promise<SystemHealthClient> {
  if (activeClient) return activeClient;
  const { systemHealthMockClient } = await import('./systemHealth.mock');
  return systemHealthMockClient;
}

// ---------------------------------------------------------------------------
// Thin wrappers — call-sites import these, never touch getClient() directly
// ---------------------------------------------------------------------------

export async function getSystemHealth(tenantId: string): Promise<SystemHealth> {
  return (await getClient()).getSystemHealth(tenantId);
}

export async function getRemoteAccess(tenantId: string): Promise<RemoteAccess> {
  return (await getClient()).getRemoteAccess(tenantId);
}

export async function setRemoteAccessEnabled(
  tenantId: string,
  enabled: boolean,
): Promise<RemoteAccess> {
  return (await getClient()).setRemoteAccessEnabled(tenantId, enabled);
}

export async function getSystemSettings(tenantId: string): Promise<SystemSettings> {
  return (await getClient()).getSystemSettings(tenantId);
}

export async function updateSystemSettings(
  tenantId: string,
  settings: Partial<SystemSettings>,
): Promise<SystemSettings> {
  return (await getClient()).updateSystemSettings(tenantId, settings);
}
