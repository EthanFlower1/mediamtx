// KAI-329: Mock data + stub API functions for system health.
//
// 3 deterministic recorders, realistic storage numbers, 5 remote sessions.
// Mutable in-memory store so mutations are reflected in the same session.

import type {
  SystemHealth,
  SystemHealthClient,
  RecorderHealth,
  CameraStatusSummary,
  StorageOverview,
  NetworkStatus,
  RemoteAccess,
  RemoteSession,
  SystemSettings,
  DayOfWeek,
} from './systemHealth';

// ---------------------------------------------------------------------------
// Deterministic mock generators
// ---------------------------------------------------------------------------

function buildRecorderHealth(tenantId: string): RecorderHealth[] {
  return [
    {
      id: `rec-${tenantId}-001`,
      name: 'Main Building NVR',
      ipAddress: '10.0.1.10',
      status: 'online',
      uptimeSeconds: 1_296_000, // 15 days
      storageUsedBytes: 5_400_000_000_000, // 5.4 TB
      storageTotalBytes: 8_000_000_000_000, // 8 TB
      lastCheckIn: '2026-04-08T10:30:00.000Z',
    },
    {
      id: `rec-${tenantId}-002`,
      name: 'Warehouse NVR',
      ipAddress: '10.0.1.11',
      status: 'degraded',
      uptimeSeconds: 432_000, // 5 days
      storageUsedBytes: 3_200_000_000_000, // 3.2 TB
      storageTotalBytes: 4_000_000_000_000, // 4 TB
      lastCheckIn: '2026-04-08T10:28:00.000Z',
    },
    {
      id: `rec-${tenantId}-003`,
      name: 'Parking Garage NVR',
      ipAddress: '10.0.1.12',
      status: 'offline',
      uptimeSeconds: 0,
      storageUsedBytes: 9_600_000_000_000, // 9.6 TB
      storageTotalBytes: 12_000_000_000_000, // 12 TB
      lastCheckIn: '2026-04-07T22:15:00.000Z',
    },
  ];
}

function buildCameraStatus(): CameraStatusSummary {
  return {
    total: 24,
    online: 18,
    offline: 3,
    degraded: 3,
  };
}

function buildStorageOverview(): StorageOverview {
  return {
    totalCapacityBytes: 24_000_000_000_000, // 24 TB
    usedBytes: 18_200_000_000_000, // 18.2 TB
    availableBytes: 5_800_000_000_000, // 5.8 TB
    retentionDaysRemaining: 42,
  };
}

function buildNetworkStatus(): NetworkStatus {
  return {
    bandwidthUtilizationPercent: 62,
    packetLossPercent: 0.02,
    latencyMs: 18,
  };
}

function buildRemoteSessions(tenantId: string): RemoteSession[] {
  void tenantId;
  return [
    {
      id: 'session-001',
      userId: 'user-admin-001',
      userDisplayName: 'Admin User',
      startedAt: '2026-04-08T09:00:00.000Z',
      endedAt: null,
      durationSeconds: 5400,
    },
    {
      id: 'session-002',
      userId: 'user-tech-002',
      userDisplayName: 'Jane Smith',
      startedAt: '2026-04-07T14:30:00.000Z',
      endedAt: '2026-04-07T15:45:00.000Z',
      durationSeconds: 4500,
    },
    {
      id: 'session-003',
      userId: 'user-tech-003',
      userDisplayName: 'Bob Johnson',
      startedAt: '2026-04-06T10:00:00.000Z',
      endedAt: '2026-04-06T10:30:00.000Z',
      durationSeconds: 1800,
    },
    {
      id: 'session-004',
      userId: 'user-admin-001',
      userDisplayName: 'Admin User',
      startedAt: '2026-04-05T16:00:00.000Z',
      endedAt: '2026-04-05T17:15:00.000Z',
      durationSeconds: 4500,
    },
    {
      id: 'session-005',
      userId: 'user-tech-004',
      userDisplayName: 'Alice Williams',
      startedAt: '2026-04-04T08:00:00.000Z',
      endedAt: '2026-04-04T08:45:00.000Z',
      durationSeconds: 2700,
    },
  ];
}

// ---------------------------------------------------------------------------
// Mutable in-memory stores
// ---------------------------------------------------------------------------

const remoteAccessStore: Map<string, RemoteAccess> = new Map();
const settingsStore: Map<string, SystemSettings> = new Map();

function getRemoteAccess(tenantId: string): RemoteAccess {
  if (!remoteAccessStore.has(tenantId)) {
    remoteAccessStore.set(tenantId, {
      enabled: true,
      portForwardingActive: true,
      vpnTunnelStatus: 'connected',
      recentSessions: buildRemoteSessions(tenantId),
    });
  }
  return remoteAccessStore.get(tenantId)!;
}

function getSettings(tenantId: string): SystemSettings {
  if (!settingsStore.has(tenantId)) {
    settingsStore.set(tenantId, {
      systemName: 'Main Office NVR System',
      timezone: 'America/New_York',
      autoUpdateEnabled: true,
      maintenanceDay: 'sunday' as DayOfWeek,
      maintenanceTime: '02:00',
    });
  }
  return settingsStore.get(tenantId)!;
}

// ---------------------------------------------------------------------------
// Mock client implementation
// ---------------------------------------------------------------------------

export const systemHealthMockClient: SystemHealthClient = {
  async getSystemHealth(tenantId: string): Promise<SystemHealth> {
    await Promise.resolve();
    const recorders = buildRecorderHealth(tenantId);
    const hasOffline = recorders.some((r) => r.status === 'offline');
    const hasDegraded = recorders.some((r) => r.status === 'degraded');
    const allOffline = recorders.every((r) => r.status === 'offline');

    return {
      overallStatus: allOffline ? 'offline' : hasOffline || hasDegraded ? 'degraded' : 'healthy',
      recorders,
      cameras: buildCameraStatus(),
      storage: buildStorageOverview(),
      network: buildNetworkStatus(),
      checkedAt: '2026-04-08T10:30:00.000Z',
    };
  },

  async getRemoteAccess(tenantId: string): Promise<RemoteAccess> {
    await Promise.resolve();
    return getRemoteAccess(tenantId);
  },

  async setRemoteAccessEnabled(tenantId: string, enabled: boolean): Promise<RemoteAccess> {
    await Promise.resolve();
    const access = getRemoteAccess(tenantId);
    access.enabled = enabled;
    access.portForwardingActive = enabled;
    access.vpnTunnelStatus = enabled ? 'connected' : 'disconnected';
    return { ...access };
  },

  async getSystemSettings(tenantId: string): Promise<SystemSettings> {
    await Promise.resolve();
    return { ...getSettings(tenantId) };
  },

  async updateSystemSettings(
    tenantId: string,
    updates: Partial<SystemSettings>,
  ): Promise<SystemSettings> {
    await Promise.resolve();
    const current = getSettings(tenantId);
    const updated = { ...current, ...updates };
    settingsStore.set(tenantId, updated);
    return { ...updated };
  },
};
