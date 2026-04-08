// KAI-320: Stub dashboard API client.
//
// Returns typed mock data for the Customer Admin Dashboard. Real
// Connect-Go generated clients will replace these functions when
// the proto pipeline lands (KAI-310). The public function surface
// (`fetchDashboardSummary`, `ackAlert`, etc.) is intentionally
// promise-based so the migration to generated code is mechanical.

export type CameraStatus = 'online' | 'offline' | 'warning';

export interface CameraSummary {
  total: number;
  online: number;
  offline: number;
  warning: number;
  recentlyAdded: number;
}

export type EventSeverity = 'info' | 'warning' | 'critical';

export interface AiEvent {
  id: string;
  cameraId: string;
  cameraName: string;
  eventType: string;
  severity: EventSeverity;
  timestamp: string; // ISO-8601
  summary: string;
}

export type AlertState = 'active' | 'acknowledged';

export interface Alert {
  id: string;
  title: string;
  description: string;
  severity: EventSeverity;
  state: AlertState;
  createdAt: string;
}

export type HealthStatus = 'healthy' | 'degraded' | 'unhealthy';

export interface StorageHealth {
  usedBytes: number;
  totalBytes: number;
  status: HealthStatus;
}

export interface NetworkHealth {
  uplinkMbps: number;
  status: HealthStatus;
  packetLossPct: number;
}

export interface SidecarHealth {
  totalSidecars: number;
  healthySidecars: number;
  status: HealthStatus;
}

export interface RecorderHealth {
  lastCheckinAt: string;
  status: HealthStatus;
}

export interface SystemHealth {
  storage: StorageHealth;
  network: NetworkHealth;
  sidecars: SidecarHealth;
  recorder: RecorderHealth;
}

export interface DashboardSummary {
  tenantId: string;
  tenantName: string;
  cameras: CameraSummary;
  events: AiEvent[];
  alerts: Alert[];
  health: SystemHealth;
}

// --- mock generators ---------------------------------------------------

function buildCameraSummary(): CameraSummary {
  return { total: 25, online: 20, offline: 3, warning: 2, recentlyAdded: 2 };
}

function buildEvents(count: number): AiEvent[] {
  const types = ['person.detected', 'vehicle.detected', 'motion.detected', 'line.crossed'];
  const severities: EventSeverity[] = ['info', 'info', 'info', 'warning', 'critical'];
  const now = Date.now();
  const events: AiEvent[] = [];
  for (let i = 0; i < count; i++) {
    const type = types[i % types.length]!;
    const severity = severities[i % severities.length]!;
    events.push({
      id: `evt-${i.toString().padStart(4, '0')}`,
      cameraId: `cam-${(i % 25).toString().padStart(3, '0')}`,
      cameraName: `Camera ${(i % 25) + 1}`,
      eventType: type,
      severity,
      timestamp: new Date(now - i * 60_000).toISOString(),
      summary: `${type} on Camera ${(i % 25) + 1}`,
    });
  }
  return events;
}

function buildAlerts(): Alert[] {
  const now = Date.now();
  return [
    {
      id: 'alert-001',
      title: 'Camera offline >5 min',
      description: 'Camera 7 has been offline for 12 minutes',
      severity: 'warning',
      state: 'active',
      createdAt: new Date(now - 12 * 60_000).toISOString(),
    },
    {
      id: 'alert-002',
      title: 'Storage usage high',
      description: 'Primary volume at 87% capacity',
      severity: 'critical',
      state: 'active',
      createdAt: new Date(now - 45 * 60_000).toISOString(),
    },
  ];
}

function buildHealth(): SystemHealth {
  return {
    storage: {
      usedBytes: 870 * 1_000_000_000,
      totalBytes: 1000 * 1_000_000_000,
      status: 'degraded',
    },
    network: { uplinkMbps: 940, packetLossPct: 0.02, status: 'healthy' },
    sidecars: { totalSidecars: 4, healthySidecars: 4, status: 'healthy' },
    recorder: {
      lastCheckinAt: new Date(Date.now() - 30_000).toISOString(),
      status: 'healthy',
    },
  };
}

// --- public API --------------------------------------------------------

export interface FetchDashboardArgs {
  tenantId: string;
}

export async function fetchDashboardSummary(
  args: FetchDashboardArgs,
): Promise<DashboardSummary> {
  // Simulate a trivial async hop — tests don't rely on real latency.
  await Promise.resolve();
  return {
    tenantId: args.tenantId,
    tenantName: 'Sample Customer',
    cameras: buildCameraSummary(),
    events: buildEvents(50),
    alerts: buildAlerts(),
    health: buildHealth(),
  };
}

export interface AckAlertArgs {
  tenantId: string;
  alertId: string;
}

export async function ackAlert(args: AckAlertArgs): Promise<Alert> {
  await Promise.resolve();
  return {
    id: args.alertId,
    title: '',
    description: '',
    severity: 'info',
    state: 'acknowledged',
    createdAt: new Date().toISOString(),
  };
}

// TanStack Query key factory — tenant-scoped.
export const dashboardQueryKeys = {
  all: (tenantId: string) => ['dashboard', tenantId] as const,
  summary: (tenantId: string) => ['dashboard', tenantId, 'summary'] as const,
};
