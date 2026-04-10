// KAI-326: Stub schedules + retention API client.
//
// Mock-only deterministic data. Real Connect-Go generated client
// (KAI-226) will replace these. Signatures are promise-based so
// migration is mechanical. All queries are tenant-scoped.

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type ScheduleType = 'continuous' | 'motion' | 'scheduled';

export type RetentionTier = '7d' | '30d' | '90d' | '1yr' | 'forensic';

export interface WeeklyTimeRange {
  /** 0 = Sunday, 6 = Saturday */
  day: number;
  /** HH:mm (24h) */
  startTime: string;
  /** HH:mm (24h) */
  endTime: string;
}

export interface RecordingSchedule {
  id: string;
  tenantId: string;
  name: string;
  type: ScheduleType;
  /** Only populated when type === 'scheduled' */
  timeRanges: WeeklyTimeRange[];
  retentionTier: RetentionTier;
  cameraIds: string[];
  createdAt: string;
  updatedAt: string;
}

export interface CreateScheduleArgs {
  tenantId: string;
  name: string;
  type: ScheduleType;
  timeRanges: WeeklyTimeRange[];
  retentionTier: RetentionTier;
  cameraIds: string[];
}

export interface UpdateScheduleArgs {
  tenantId: string;
  scheduleId: string;
  name?: string;
  type?: ScheduleType;
  timeRanges?: WeeklyTimeRange[];
  retentionTier?: RetentionTier;
  cameraIds?: string[];
}

export interface DeleteScheduleArgs {
  tenantId: string;
  scheduleId: string;
}

export interface BulkAssignArgs {
  tenantId: string;
  scheduleId: string;
  cameraIds: string[];
}

export interface RetentionOverview {
  tier: RetentionTier;
  storageUsedBytes: number;
  storageTotalBytes: number;
  cameraCount: number;
  estimatedDaysRemaining: number;
}

export interface ScheduleCamera {
  id: string;
  name: string;
}

// ---------------------------------------------------------------------------
// Query key factories
// ---------------------------------------------------------------------------

export const schedulesQueryKeys = {
  all: (tenantId: string) => ['schedules', tenantId] as const,
  list: (tenantId: string) => ['schedules', tenantId, 'list'] as const,
  retention: (tenantId: string) => ['schedules', tenantId, 'retention'] as const,
  cameras: (tenantId: string) => ['schedules', tenantId, 'cameras'] as const,
};

// ---------------------------------------------------------------------------
// Feature-flag bootstrap
// ---------------------------------------------------------------------------

export const SCHEDULES_FEATURE_FLAG = 'schedules.enabled';

export function isSchedulesEnabled(): boolean {
  // TODO(KAI-326): wire to real feature-flag service
  return true;
}
