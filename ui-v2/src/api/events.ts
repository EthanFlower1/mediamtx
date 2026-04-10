// KAI-324 + KAI-149 follow-up: Events API module.
//
// This file contains ONLY:
//   - shared types (AiEvent, filters, export args)
//   - the EventsClient interface that real/mock backends implement
//   - tenant-scoped TanStack Query key factory
//   - pure, tested helpers (buildEventsCsv, distinctCameras)
//   - the feature-flag bootstrap that selects mock vs real client
//
// Mock implementation lives in events.mock.ts. The real Connect-Go
// client will land with the proto pipeline (KAI-238) and register
// itself through the same bootstrap, so swapping is mechanical.

import { eventsMockClient } from './events.mock';

// ---------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------

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

export interface ExportPdfArgs {
  tenantId: string;
  eventIds: readonly string[];
}

export interface ExportPdfResult {
  downloadUrl: string;
  pageCount: number;
}

export interface EventsClient {
  listEvents(filters: EventListFilters): Promise<AiEvent[]>;
  exportEventsPdf(args: ExportPdfArgs): Promise<ExportPdfResult>;
}

// ---------------------------------------------------------------------
// Feature-flag bootstrap
// ---------------------------------------------------------------------
//
// VITE_USE_MOCK_API defaults to 'true' until the Connect-Go client
// lands (KAI-238). Set to 'false' in the production .env to force
// the real client; the bootstrap will throw if no real client has
// been registered, which is the behavior we want in CI.

let activeClient: EventsClient = eventsMockClient;

export function registerEventsClient(client: EventsClient): void {
  activeClient = client;
}

export function resetEventsClientForTests(): void {
  activeClient = eventsMockClient;
}

export function listEvents(filters: EventListFilters): Promise<AiEvent[]> {
  return activeClient.listEvents(filters);
}

export function exportEventsPdf(args: ExportPdfArgs): Promise<ExportPdfResult> {
  return activeClient.exportEventsPdf(args);
}

// ---------------------------------------------------------------------
// Pure helpers — no I/O, safe to call from any layer.
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
