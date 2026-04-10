// KAI-326: Mock data + stub API functions for schedules.
//
// 4-5 deterministic mock schedules per tenant plus mock camera assignment.
// Mutable in-memory store so mutations are reflected in the same session.

import type {
  BulkAssignArgs,
  CreateScheduleArgs,
  DeleteScheduleArgs,
  RecordingSchedule,
  RetentionOverview,
  RetentionTier,
  ScheduleCamera,
  UpdateScheduleArgs,
  WeeklyTimeRange,
} from './schedules';

// ---------------------------------------------------------------------------
// Deterministic mock generators
// ---------------------------------------------------------------------------

const BUSINESS_HOURS: WeeklyTimeRange[] = [1, 2, 3, 4, 5].map((day) => ({
  day,
  startTime: '08:00',
  endTime: '18:00',
}));

const AFTER_HOURS: WeeklyTimeRange[] = [
  ...[1, 2, 3, 4, 5].map((day) => ({
    day,
    startTime: '18:00',
    endTime: '23:59',
  })),
  ...[1, 2, 3, 4, 5].map((day) => ({
    day,
    startTime: '00:00',
    endTime: '08:00',
  })),
  { day: 0, startTime: '00:00', endTime: '23:59' },
  { day: 6, startTime: '00:00', endTime: '23:59' },
];

function buildSchedules(tenantId: string): RecordingSchedule[] {
  const now = new Date().toISOString();
  return [
    {
      id: `sched-${tenantId}-001`,
      tenantId,
      name: '24/7 Continuous',
      type: 'continuous',
      timeRanges: [],
      retentionTier: '30d',
      cameraIds: [`cam-${tenantId}-001`, `cam-${tenantId}-002`, `cam-${tenantId}-003`],
      createdAt: now,
      updatedAt: now,
    },
    {
      id: `sched-${tenantId}-002`,
      tenantId,
      name: 'Business Hours',
      type: 'scheduled',
      timeRanges: BUSINESS_HOURS,
      retentionTier: '30d',
      cameraIds: [`cam-${tenantId}-004`, `cam-${tenantId}-005`],
      createdAt: now,
      updatedAt: now,
    },
    {
      id: `sched-${tenantId}-003`,
      tenantId,
      name: 'After Hours',
      type: 'scheduled',
      timeRanges: AFTER_HOURS,
      retentionTier: '90d',
      cameraIds: [`cam-${tenantId}-006`],
      createdAt: now,
      updatedAt: now,
    },
    {
      id: `sched-${tenantId}-004`,
      tenantId,
      name: 'Motion Only - Parking Lot',
      type: 'motion',
      timeRanges: [],
      retentionTier: '7d',
      cameraIds: [`cam-${tenantId}-007`, `cam-${tenantId}-008`],
      createdAt: now,
      updatedAt: now,
    },
    {
      id: `sched-${tenantId}-005`,
      tenantId,
      name: 'Forensic Archive',
      type: 'continuous',
      timeRanges: [],
      retentionTier: 'forensic',
      cameraIds: [`cam-${tenantId}-009`],
      createdAt: now,
      updatedAt: now,
    },
  ];
}

function buildCameras(tenantId: string): ScheduleCamera[] {
  const names = [
    'Front Entrance', 'Lobby', 'Server Room', 'Warehouse A',
    'Warehouse B', 'Parking Lot NW', 'Parking Lot SE', 'Loading Dock',
    'Elevator Hallway', 'Office Wing 2F',
  ];
  return names.map((name, i) => ({
    id: `cam-${tenantId}-${String(i + 1).padStart(3, '0')}`,
    name,
  }));
}

function buildRetention(tenantId: string): RetentionOverview[] {
  void tenantId;
  const GB = 1_073_741_824;
  return [
    { tier: '7d' as RetentionTier, storageUsedBytes: 12 * GB, storageTotalBytes: 50 * GB, cameraCount: 2, estimatedDaysRemaining: 22 },
    { tier: '30d' as RetentionTier, storageUsedBytes: 85 * GB, storageTotalBytes: 200 * GB, cameraCount: 5, estimatedDaysRemaining: 41 },
    { tier: '90d' as RetentionTier, storageUsedBytes: 42 * GB, storageTotalBytes: 100 * GB, cameraCount: 1, estimatedDaysRemaining: 125 },
    { tier: '1yr' as RetentionTier, storageUsedBytes: 0, storageTotalBytes: 500 * GB, cameraCount: 0, estimatedDaysRemaining: 365 },
    { tier: 'forensic' as RetentionTier, storageUsedBytes: 220 * GB, storageTotalBytes: 1000 * GB, cameraCount: 1, estimatedDaysRemaining: 1029 },
  ];
}

// Mutable in-memory store
const scheduleStore: Map<string, RecordingSchedule[]> = new Map();

function getSchedules(tenantId: string): RecordingSchedule[] {
  if (!scheduleStore.has(tenantId)) {
    scheduleStore.set(tenantId, buildSchedules(tenantId));
  }
  return scheduleStore.get(tenantId)!;
}

// ---------------------------------------------------------------------------
// Public API — Schedules
// ---------------------------------------------------------------------------

export async function listSchedules(tenantId: string): Promise<RecordingSchedule[]> {
  await Promise.resolve();
  return getSchedules(tenantId);
}

export async function createSchedule(args: CreateScheduleArgs): Promise<RecordingSchedule> {
  await Promise.resolve();
  const now = new Date().toISOString();
  const schedule: RecordingSchedule = {
    id: `sched-${args.tenantId}-new-${Date.now()}`,
    tenantId: args.tenantId,
    name: args.name,
    type: args.type,
    timeRanges: args.timeRanges,
    retentionTier: args.retentionTier,
    cameraIds: args.cameraIds,
    createdAt: now,
    updatedAt: now,
  };
  getSchedules(args.tenantId).push(schedule);
  return schedule;
}

export async function updateSchedule(args: UpdateScheduleArgs): Promise<void> {
  await Promise.resolve();
  const schedules = getSchedules(args.tenantId);
  const idx = schedules.findIndex((s) => s.id === args.scheduleId);
  if (idx >= 0) {
    const existing = schedules[idx]!;
    schedules[idx] = {
      ...existing,
      ...(args.name !== undefined && { name: args.name }),
      ...(args.type !== undefined && { type: args.type }),
      ...(args.timeRanges !== undefined && { timeRanges: args.timeRanges }),
      ...(args.retentionTier !== undefined && { retentionTier: args.retentionTier }),
      ...(args.cameraIds !== undefined && { cameraIds: args.cameraIds }),
      updatedAt: new Date().toISOString(),
    };
  }
}

export async function deleteSchedule(args: DeleteScheduleArgs): Promise<void> {
  await Promise.resolve();
  const schedules = getSchedules(args.tenantId);
  const idx = schedules.findIndex((s) => s.id === args.scheduleId);
  if (idx >= 0) schedules.splice(idx, 1);
}

export async function bulkAssignSchedule(args: BulkAssignArgs): Promise<void> {
  await Promise.resolve();
  const schedules = getSchedules(args.tenantId);
  const schedule = schedules.find((s) => s.id === args.scheduleId);
  if (schedule) {
    const existing = new Set(schedule.cameraIds);
    for (const id of args.cameraIds) existing.add(id);
    schedule.cameraIds = [...existing];
    schedule.updatedAt = new Date().toISOString();
  }
}

// ---------------------------------------------------------------------------
// Public API — Retention overview
// ---------------------------------------------------------------------------

export async function listRetentionOverview(tenantId: string): Promise<RetentionOverview[]> {
  await Promise.resolve();
  return buildRetention(tenantId);
}

// ---------------------------------------------------------------------------
// Public API — Cameras for schedule assignment
// ---------------------------------------------------------------------------

export async function listScheduleCameras(tenantId: string): Promise<ScheduleCamera[]> {
  await Promise.resolve();
  return buildCameras(tenantId);
}
