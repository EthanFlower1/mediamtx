// KAI-324 follow-up (KAI-149): mock implementation of the events API.
//
// Separated from events.ts so the real Connect-Go client (KAI-238)
// can be swapped in by flipping VITE_USE_MOCK_API without disturbing
// types, query keys, or the pure CSV/helpers consumed by the UI.
//
// All data is deterministic from the tenantId so tests stay stable.

import type {
  AiEvent,
  EventListFilters,
  EventSeverity,
  EventType,
  EventsClient,
  ExportPdfArgs,
  ExportPdfResult,
} from './events';

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

async function listEventsMock(filters: EventListFilters): Promise<AiEvent[]> {
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

async function exportEventsPdfMock(args: ExportPdfArgs): Promise<ExportPdfResult> {
  // The real PDF renderer runs in the backend because it needs the
  // original video frames. The scaffold returns a deterministic stub
  // so the UI can render a "download ready" state.
  await Promise.resolve();
  return {
    downloadUrl: `/api/v1/tenants/${args.tenantId}/events/export/pdf?ids=${args.eventIds.join(',')}`,
    pageCount: Math.max(1, Math.ceil(args.eventIds.length / 4)),
  };
}

export const eventsMockClient: EventsClient = {
  listEvents: listEventsMock,
  exportEventsPdf: exportEventsPdfMock,
};
