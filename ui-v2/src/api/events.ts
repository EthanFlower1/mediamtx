// KAI-324: Stub events API client.
//
// Returns typed mock AI detection events for the Customer Admin
// Events page. Real Connect-Go generated clients will replace these
// functions when the proto pipeline lands (KAI-238). Function
// signatures are promise-based so migration is mechanical. All
// queries are tenant-scoped via the session store.

export type EventSeverity = 'info' | 'low' | 'medium' | 'high' | 'critical';

export type EventType =
  | 'person.detected'
  | 'vehicle.detected'
  | 'package.detected'
  | 'face.matched'
  | 'lpr.plate'
  | 'anomaly.behavioral'
  | 'line.crossed'
  | 'intrusion.zone';

export interface AiEvent {
  id: string;
  tenantId: string;
  cameraId: string;
  cameraName: string;
  type: EventType;
  severity: EventSeverity;
  timestamp: string; // ISO-8601
  summary: string;
  thumbnailUrl: string;
  clipStartSec: number;
  clipDurationSec: number;
}

export interface EventListFilters {
  tenantId: string;
  cameraIds?: readonly string[];
  types?: readonly EventType[];
  severities?: readonly EventSeverity[];
  fromIso?: string;
  toIso?: string;
  search?: string;
  semantic?: boolean;
}

// ---------------------------------------------------------------------
// Deterministic mock data generators
// ---------------------------------------------------------------------

const EVENT_TYPES: readonly EventType[] = [
  'person.detected',
  'vehicle.detected',
  'package.detected',
  'face.matched',
  'lpr.plate',
  'anomaly.behavioral',
  'line.crossed',
  'intrusion.zone',
];

const SEVERITIES: readonly EventSeverity[] = [
  'info',
  'info',
  'low',
  'low',
  'medium',
  'medium',
  'high',
  'critical',
];

const CAMERA_COUNT = 10;

function buildCameras(tenantId: string): readonly { id: string; name: string }[] {
  const out: { id: string; name: string }[] = [];
  for (let i = 0; i < CAMERA_COUNT; i++) {
    out.push({
      id: `cam-${tenantId}-${i.toString().padStart(3, '0')}`,
      name: `Camera ${(i + 1).toString().padStart(3, '0')}`,
    });
  }
  return out;
}

function buildEvents(tenantId: string, count: number): AiEvent[] {
  const cameras = buildCameras(tenantId);
  const now = Date.now();
  const out: AiEvent[] = [];
  for (let i = 0; i < count; i++) {
    const type = EVENT_TYPES[i % EVENT_TYPES.length]!;
    const severity = SEVERITIES[i % SEVERITIES.length]!;
    const camera = cameras[i % cameras.length]!;
    const ts = new Date(now - i * 90_000).toISOString();
    out.push({
      id: `evt-${i.toString().padStart(4, '0')}`,
      tenantId,
      cameraId: camera.id,
      cameraName: camera.name,
      type,
      severity,
      timestamp: ts,
      summary: buildSummary(type, camera.name),
      // Deterministic placeholder thumbnail URL — real thumbnails land
      // with the Multi-Recorder timeline API (KAI-262).
      thumbnailUrl: `/thumbnails/${camera.id}/${i}.jpg`,
      clipStartSec: i * 90,
      clipDurationSec: 12,
    });
  }
  return out;
}

function buildSummary(type: EventType, cameraName: string): string {
  // The summary text is deterministic seed data — the UI displays
  // translated type labels separately, this is a free-text excerpt
  // the detector would emit. Left in English in the mock because the
  // real backend emits source-language text plus a structured code.
  switch (type) {
    case 'person.detected':
      return `Person detected on ${cameraName}`;
    case 'vehicle.detected':
      return `Vehicle detected on ${cameraName}`;
    case 'package.detected':
      return `Package left at ${cameraName}`;
    case 'face.matched':
      return `Face match on ${cameraName}`;
    case 'lpr.plate':
      return `License plate read on ${cameraName}`;
    case 'anomaly.behavioral':
      return `Behavioral anomaly on ${cameraName}`;
    case 'line.crossed':
      return `Tripwire crossed on ${cameraName}`;
    case 'intrusion.zone':
      return `Intrusion zone breach on ${cameraName}`;
  }
}

// ---------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------

export async function listEvents(filters: EventListFilters): Promise<AiEvent[]> {
  await Promise.resolve();
  const all = buildEvents(filters.tenantId, 50);
  let filtered: AiEvent[] = all;

  if (filters.cameraIds && filters.cameraIds.length > 0) {
    const set = new Set(filters.cameraIds);
    filtered = filtered.filter((e) => set.has(e.cameraId));
  }
  if (filters.types && filters.types.length > 0) {
    const set = new Set(filters.types);
    filtered = filtered.filter((e) => set.has(e.type));
  }
  if (filters.severities && filters.severities.length > 0) {
    const set = new Set(filters.severities);
    filtered = filtered.filter((e) => set.has(e.severity));
  }
  if (filters.fromIso) {
    const fromMs = Date.parse(filters.fromIso);
    filtered = filtered.filter((e) => Date.parse(e.timestamp) >= fromMs);
  }
  if (filters.toIso) {
    const toMs = Date.parse(filters.toIso);
    filtered = filtered.filter((e) => Date.parse(e.timestamp) <= toMs);
  }
  if (filters.search && filters.search.trim().length > 0) {
    const q = filters.search.trim().toLowerCase();
    if (filters.semantic) {
      // KAI-324: semantic search is mocked client-side — the real
      // backend will call pgvector + CLIP embeddings (KAI-292). For
      // the scaffold, semantic mode loosely matches on any token
      // that appears in the summary OR the event type, which is
      // sufficient to exercise the UI path.
      const tokens = q.split(/\s+/).filter((tok) => tok.length > 0);
      filtered = filtered.filter((e) =>
        tokens.some(
          (tok) =>
            e.summary.toLowerCase().includes(tok) ||
            e.type.toLowerCase().includes(tok),
        ),
      );
    } else {
      filtered = filtered.filter(
        (e) =>
          e.summary.toLowerCase().includes(q) ||
          e.cameraName.toLowerCase().includes(q),
      );
    }
  }
  return filtered;
}

export interface ExportPdfArgs {
  tenantId: string;
  eventIds: readonly string[];
}

export interface ExportPdfResult {
  downloadUrl: string;
  pageCount: number;
}

export async function exportEventsPdf(args: ExportPdfArgs): Promise<ExportPdfResult> {
  // The real PDF renderer runs in the backend because it needs the
  // original video frames. The scaffold returns a deterministic stub
  // so the UI can render a "download ready" state.
  await Promise.resolve();
  return {
    downloadUrl: `/api/v1/tenants/${args.tenantId}/events/export/pdf?ids=${args.eventIds.join(',')}`,
    pageCount: Math.max(1, Math.ceil(args.eventIds.length / 4)),
  };
}

// ---------------------------------------------------------------------
// CSV generator — client-side, pure, tested.
// ---------------------------------------------------------------------

export function buildEventsCsv(events: readonly AiEvent[]): string {
  const header = [
    'id',
    'timestamp',
    'camera',
    'type',
    'severity',
    'summary',
    'clipStartSec',
    'clipDurationSec',
  ];
  const rows: string[] = [header.join(',')];
  for (const e of events) {
    rows.push(
      [
        csvCell(e.id),
        csvCell(e.timestamp),
        csvCell(e.cameraName),
        csvCell(e.type),
        csvCell(e.severity),
        csvCell(e.summary),
        String(e.clipStartSec),
        String(e.clipDurationSec),
      ].join(','),
    );
  }
  return rows.join('\n');
}

function csvCell(value: string): string {
  // Quote if the value contains comma, quote, newline, or carriage return.
  if (/[",\r\n]/.test(value)) {
    return `"${value.replace(/"/g, '""')}"`;
  }
  return value;
}

// Return the list of distinct cameras seen in a batch of events.
// Used by the filters component to populate the camera multi-select
// without a second API round trip.
export function distinctCameras(
  events: readonly AiEvent[],
): readonly { id: string; name: string }[] {
  const seen = new Map<string, string>();
  for (const e of events) {
    if (!seen.has(e.cameraId)) {
      seen.set(e.cameraId, e.cameraName);
    }
  }
  return Array.from(seen.entries())
    .map(([id, name]) => ({ id, name }))
    .sort((a, b) => a.name.localeCompare(b.name));
}

// TanStack Query key factory — tenant-scoped.
export const eventsQueryKeys = {
  all: (tenantId: string) => ['events', tenantId] as const,
  list: (tenantId: string, filters: Omit<EventListFilters, 'tenantId'>) =>
    ['events', tenantId, 'list', filters] as const,
};
